# Owner queue and delayed-event API for Cardinal

Researched 2026-11-19. Research only: no code changed then. "Note NN" means `queue-api-notes/NN-*.md`.
The benchmarks and an analyzer prototype are in the same folder. The brief is at the end.

> **These are findings, not the source of truth.** Code and [`CLAUDE.md`](../../CLAUDE.md) win when
> they disagree with this file.

## Status (2026-11-20)

Echo built a smaller version: [`owner-queues.md`](owner-queues.md). How it differs from this report:

| This report | Built in echo |
| --- | --- |
| `Req[R]` wraps each request with a `Header` (source, cause, depth, flags) made by `Root` and `Child` | Plain request structs. Each keeps its own fields |
| Sort by (Due, Target.ID, Source.ID, Seq) | Sort by (Due, the owner's compare) |
| `cardinal.Handle` with a generation on every request | Not yet: no request in echo outlives its tick |
| A depth cap | Dropped ([`owner-queues.md`](owner-queues.md) §3) |
| `MaxLen` and `MaxHorizon` limits | None yet |
| `Drain` yields (entity, request) pairs | `Drain` returns the sorted requests |
| Senders batch into one `Send` | Senders send one at a time. On echo's pinned World Engine, an append is in place |
| `DeclareDelayed[E](w)`; `Schedule(w, at, subject, e)` with `at` in [now+1, now+MaxHorizon] | `DeclareDelayed[E](w, compare)`; `Schedule(w, tick, e)`. A past tick fires at the next start |
| `CancelFunc` over `Pending[E]` | `CancelFunc` over the event |
| Package `pkg/cardinal/queue` | `pkg/queue` in echo, built to move to World Engine |

Kept as this report says: one owner system drains once a tick, due times are absolute ticks, a typed
`Ticket[E]` is never reused, and delayed events fire at the start of the tick.

---

Final report, 2026-11-19. Built from notes 01–08 in [`queue-api-notes/`](queue-api-notes/), with the fixes from the critique (09). Note 08 (verification) wins where notes disagree. All Cardinal signatures were re-checked against WE `main` f7d4a52b.

Marks:
- **(V)** read in primary source (code or official doc), and not contradicted by note 08.
- **(S)** secondary source, a search snippet, a summary tool, or a user measurement not re-run.
- **(I)** my inference or my proposal.

All Go signatures in this report are proposals, so they are (I). The Cardinal calls they use are real (V) [1]: `Entity.Get[T]()`, `Entity.Set(c)`, `Entity.ID()`, `World.TickHeight() uint64`, `World.Contains[T]()`, `World.RegisterSystem(s, opts...)`, `WithPhase`, `EmitSystemEvent`, `SystemEvents[T]() iter.Seq[T]`, `cardinal.Slice[T]`, `EntityID = uint32`.

---

## 1. Verdict

### 1.1 In short

- Keep the architecture. Most rules have a shipped precedent (§2). Two do not: a per-chain depth field and firing delayed events at tick start. Those two are our own choice. (V for the search, I for the choice)
- Use **one generic queue type**, `queue.Queue[R]`. One singleton per request type, never one per entity. (I)
- Sort key for requests: **(Due, Target.ID, Source.ID, Seq)**. For delayed events: **(Due, Subject.ID, Source.ID, Seq)**. Seq is a per-queue `uint64`, saved in the component, never reset. (I)
- Store every due time as an **absolute tick** (`uint64`). Never seconds. Never a countdown. (V: Doom 3, Source, Quantum [38][39][29]; (S) Mover [26])
- A drain works on a **frozen copy**. Requests sent during the drain wait for the next drain. So no loop can hang a tick. (I)
- Senders **batch**: collect into a local slice, then call `Send` once, before the system returns. No Go-side buffer outlives the sending system. One-by-one appends are about 11x slower on WE main. (V) [4]
- Headers come only from `queue.Root` and `Header.Child`. `Cause == 0` and a zero `Handle` are invalid. An analyzer flags hand-made `Header{}` literals. (I)
- Limits (`MaxLen`, `MaxHorizon`) are code on the payload type, never saved. The depth cap is one constant, because a chain crosses queue types. (I)

### 1.2 Three gaps the API must close (not a redesign)

| # | Gap | Evidence | Fix |
|---|---|---|---|
| G1 | A waiting request or delayed event can hit the **wrong entity**. Cardinal reuses an `EntityID` at once and has no generation. | `EntityID` is `uint32`, no generation; free list is a min-heap; doc warns of reuse (V) [1]. Doom 3 and Source use index + serial handles (V) [38][39]; Unreal timers auto-cancel (S) [45]. | Engine adds a generation. Requests carry `cardinal.Handle{ID, Gen}` for Target, Source and Subject. Drain and fire drop stale ones. (I) |
| G2 | **Depth resets through events.** Request → event → reader sends a new request. If the reader uses `Root`, depth is 0 again. The loop runs forever, one hop per tick. | No engine has a per-chain depth field (V absence) [6][15][24][34]. | Every request, event and delayed event carries `queue.Header`. Readers build `ev.Child(src)`. `Schedule` checks depth too. Repeats are owner state, not re-sent events. (I) |
| G3 | The brief's cost fact is **stale**. Since PR #1177, `Insert(Len, x)` just calls `Append`. Both copy the last chunk (up to 32 items). | `slice.go:277-306` (V) [2]; note 08 row 8. | Design for batched `Append`. The user's "+116 µs" was likely measured before #1177; re-measure. (I) |

Also worth knowing (not gaps):
- Rule 7 needs **stable IDs before Seq** in the key. Search order differs after a restore (probe: live `[4 3 2 0]` vs restored `[0 2 3 4]`). So a Seq stamped inside a search loop is not restore-stable on its own. (V) [1][3]
- Rule 6 (fire at tick start) has **no engine slot today**. "Start of tick" is just the first PreUpdate system. (V) [1] Doom 3 and Source fire after thinks (V) [38][39]. Tick start is still a good choice: every reader runs after the fire by construction. (I)

### 1.3 Owner queue: Go API

```go
package queue // proposed: pkg/cardinal/queue

// Header rides on every request and every event.
type Header struct {
	Due    uint64          // absolute tick. 0 = now (Send stamps it).
	Seq    uint64          // stamped by Send/Schedule. Unique per queue, never reused.
	Source cardinal.Handle // the entity the sending loop is on.
	Cause  uint32          // game id of the first trigger. 0 = invalid.
	Depth  uint8           // hops since the first trigger.
	Flags  uint16          // game bits.
}

const MaxDepth = 16 // one cap: a chain crosses queue types

func Root(source cardinal.Handle, cause uint32) Header // starts a chain, Depth 0
func (h Header) Child(source cardinal.Handle) Header   // same Cause, Depth+1, Due/Seq cleared
func (h Header) At(tick uint64) Header                 // sets Due
func (h Header) Head() Header                          // lets events embed Header (1.4)

// Limits are code, never saved, so a restore cannot bring back old caps.
type Limits struct {
	MaxLen     uint32 // items in one queue; fan-out guard
	MaxHorizon uint64 // max ticks ahead for a due tick
}

// Payload is a request body. Plain value struct, numbers only.
type Payload interface {
	Name() string // also names the component; must match [a-zA-Z_][a-zA-Z0-9_]*
	Limits() Limits
}

// Req is one request. Header is a field, not a method: 18x faster to read in a generic drain.
type Req[R Payload] struct {
	Hdr    Header
	Target cardinal.Handle // engine addition (G1)
	Body   R
}

// Queue is the component. One singleton per request type.
type Queue[R Payload] struct {
	Items   cardinal.Slice[Req[R]]
	NextSeq uint64 // starts at 1
	NextDue uint64 // smallest Due in Items; lets Drain return early
	Drained uint64 // last drained tick + 1 (0 = never)
}

// One name per R. A plain method on Queue would give every R the same name.
func (Queue[R]) Name() string { var r R; return "queue_" + r.Name() }

// Declare registers Queue[R] and an Init system that creates its singleton.
func Declare[R Payload](w *cardinal.World)

// Send does one singleton lookup, one Append and one Set for the whole batch.
// Checks: Cause != 0, Source and Target non-zero, Depth <= MaxDepth, Len <= MaxLen,
// Due in [now, now+MaxHorizon]. Fails panic in TestWorld. In production: bad Due is
// clamped, the rest dropped; all logged and counted.
func Send[R Payload](w *cardinal.World, reqs ...Req[R])

// Drain yields due requests in key order. Stale targets are dropped and counted.
func Drain[R Payload](w *cardinal.World) iter.Seq2[cardinal.Entity, Req[R]]
```

Drain, step by step (I):
1. Panic if this queue was already drained this tick.
2. Return at once if `NextDue > now`.
3. Copy out items with `Due <= now`. That set is frozen. Put the rest back.
4. Sort by (Due, Target.ID, Source.ID, Seq).
5. Resolve each Target. Stale = drop + count. Alive = yield.
6. On `break`, unvisited items go back.

Anything sent during the drain, even by the owner to itself, waits for the next drain.

### 1.4 Delayed-event queue: Go API

```go
// Event is a system event that embeds queue.Header (Due/Seq stay 0; Pending holds them).
type Event interface {
	Name() string // also names the component, same rule as Payload
	Head() Header // promoted from the embedded Header
	Limits() Limits
}

type Pending[E Event] struct {
	Due     uint64          // fire tick
	Seq     uint64          // also the ticket
	Subject cardinal.Handle // the entity it is about; stale at fire time = dropped
	Event   E
}

// Delayed is the component. One singleton per event type, owned by the sender feature.
type Delayed[E Event] struct {
	Items   cardinal.Slice[Pending[E]]
	NextSeq uint64 // starts at 1
	NextDue uint64
}

func (Delayed[E]) Name() string { var e E; return "delayed_" + e.Name() }

type Ticket[E Event] uint64 // = Seq. 0 = none. Typed: cannot cancel another event type.

// DeclareDelayed registers Delayed[E], the system event E, and one fire system in First.
func DeclareDelayed[E Event](w *cardinal.World)

// Schedule fires e at the start of tick `at`. Same checks as Send, with at in [now+1, now+MaxHorizon].
func Schedule[E Event](w *cardinal.World, at uint64, subject cardinal.Handle, e E) Ticket[E]

// Cancel returns false if the item already fired, was cancelled, or is unknown.
func Cancel[E Event](w *cardinal.World, t Ticket[E]) bool
func CancelFunc[E Event](w *cardinal.World, del func(Pending[E]) bool) int
```

Fire system (one per type, in `First`): if `NextDue > now`, return. Else take the due items, sort by (Due, Subject.ID, Source.ID, Seq), drop stale subjects (counted), and `EmitSystemEvent` each. (I)

Cancel timing: events fire at tick start, before any system can cancel. So a cancel on the due tick is too late and returns false. Fire wins on the due tick. (I)

### 1.5 Usage: owner queue

```go
// package combat — owns Health and the damage queue.
type DamageReq struct{ Amount int32 }

func (DamageReq) Name() string         { return "damage_req" }
func (DamageReq) Limits() queue.Limits { return queue.Limits{MaxLen: 4096, MaxHorizon: 1200} }

func (Plugin) Register(w *cardinal.World) {
	queue.Declare[DamageReq](w)
	w.RegisterSystem(applyDamage{}, cardinal.WithDrains[queue.Queue[DamageReq]]())
}

func (applyDamage) Run(w *cardinal.World) {
	for e, r := range queue.Drain[DamageReq](w) {
		h := e.Get[Health]()
		h.HP -= r.Body.Amount
		e.Set(h)
		if h.HP <= 0 {
			w.EmitSystemEvent(Died{Header: r.Hdr.Child(e.Handle()), Who: e.Handle()})
		}
	}
}

// package weapons — a sender (registered with cardinal.WithSends[...]()). Collect, then send once.
func (hitScan) Run(w *cardinal.World) {
	var out []queue.Req[combat.DamageReq]
	for p := range w.Contains[struct{ B Bullet }]().Iter() {
		target, ok := firstHit(w, p) // a Box2D query
		if !ok {
			continue
		}
		out = append(out, queue.Req[combat.DamageReq]{
			Hdr:    queue.Root(p.Handle(), CauseBullet), // contact = a true start
			Target: target,
			Body:   combat.DamageReq{Amount: 10},
		})
	}
	queue.Send(w, out...)
}
```

### 1.6 Usage: delayed event

```go
// package ability — owns the wind-up and its done event.
type WindUpDone struct {
	queue.Header
	Ability uint16
}

func (WindUpDone) Name() string         { return "wind_up_done" }
func (WindUpDone) Limits() queue.Limits { return queue.Limits{MaxLen: 1024, MaxHorizon: 20 * 60} }

// Register:
queue.DeclareDelayed[WindUpDone](w)

// Start a 10-tick wind-up; keep the ticket in the owner's own component:
ab.WindUp = queue.Schedule(w, w.TickHeight()+10, caster.Handle(),
	WindUpDone{Header: queue.Root(caster.Handle(), CauseAbility), Ability: id})
caster.Set(ab)

// Stunned: cancel. false = it already fired at the start of this tick.
queue.Cancel(w, ab.WindUp)

// Reader (registered with cardinal.WithReads[WindUpDone]()):
for ev := range w.SystemEvents[WindUpDone]() {
	// react; build any new request with ev.Child(src)
}
```

Rule of thumb (I, from Source think slots and Quake `nextthink` (V) [39]): if an entity has **at most one** pending thing per purpose, a `Due` field in the owner's component is simpler than a queue. Cancel = overwrite. Repeats (regen, ticks of a DoT) also live there. Use the delayed queue when there can be many.

---

## 2. Engine table

| Engine | API shape | How it's drained | Delays | Determinism | Known pitfalls |
|---|---|---|---|---|---|
| Bevy 0.20 Commands | Opaque closures in a per-system queue (V) [9] | `ApplyDeferred` sync points, system index order (V) [9] | — | Order changes when systems are added (V) [9] | Despawned target = error (V) [9]; observer commands not depth-first, #19569 open (V) [10] |
| Bevy Messages | Typed double buffer, per-reader cursor (V) [6] | Buffer swap in `First` (V) [6] | — | Not snapshotted; bevy_ggrs says don't use in rollback (V) [12] | Unread messages "dropped silently" (V) [6] |
| Bevy DelayedCommands | `commands.delayed().secs(f)` (V) [7] | PreUpdate, default clock (V) [7] | `Duration`, not ticks (V) [7] | Fires in query order, no sort (V) [7] | No cancel (PR #24179 closed) (V) [8]; `reflect(ignore)` = not saved (V) [7] |
| Flecs 4.2 | Explicit stage; command records the enqueuing system (V) [14][15] | Merge at sync points; nested queues depth-first (V) [15] | Timers / rate filters as components (V) [17] | Rate filters (ints) deterministic, float intervals not (V) [17] | Observer order undefined (V) [16]; "a basic event queue is always going to outperform observers" (V) [16] |
| Unity ECB | Commands with a required `sortKey` arg (V) [18] | ECB systems at group start/end (V) [18] | — | Min-heap merge on sortKey only, no tie-break (V) [19] | Same key from two threads interleaves (I) [19]; multi-playback deprecated in 6.6 (S, unverified) [23] |
| Unity Netcode | Per-entity input ring keyed by tick, size 64 (V) [21] | Read, not consumed (V) [21] | Tick-stamped (V) [21] | "not a deterministic package" (V) [21] | Partial rollback breaks owner-applies (V) [21]; stale RPC watchdog 4 frames (V) [21] |
| Unreal Mass | Command buffer (lambdas); payload-less signals (V) [24] | End of each phase; flush loop max 5 then error log (V) [24] | `DelaySignal` in float seconds (V) [24] | Batched by op type, record order lost (V) [24] | `RemoveAtSwap` reorders delays (V) [24]; payload by position went to wrong entities (V) [25] |
| Unreal GAS | Effect spec + context (instigator, causer) (S) [28] | Applied at once into target's ASC (S) [28] | Duration effects, seconds (S) [28] | Add/Mul commutative; Override = last wins (S) [28] | Scope locks + pending lists still had bugs (S) [28] |
| Unreal Mover | Queue instant effects / layered moves / modifiers (S) [26] | Next sim frame (S) [26] | `ExecutionServerFrame` int on the request (S) [26] | Mixer by priority + mix mode (S) [26] | Off-by-one on last frame (V forum) [26] |
| Photon Quantum | Signals = sync calls; commands per player per tick (V) [30] | Signals run at once in system order (V) [30] | `FrameTimer` = target frame, 2 ints (V) [29] | All state in Frame; fixed system order (V) [30] | `HasStoppedThisFrame` is edge-only (I) [29] |
| SpatialOS GDK | One flat list per command type (V) [32] | Owner system reads `GetRequests`; cleared each frame (V) [32] | 5 s deadline max (V) [31] | Sort by EntityId only, unstable (V) [32] | Timeout ≠ "not applied" (V) [31] |
| SS14 / Robust | Owner method `ChangeDamage(ent, dmg, origin, flags…)` (V) [35] | Sync; queued events loop until empty, no cap (V) [34] | Absolute `TimeSpan` fields; DoAfter (V) [35] | Single thread, fixed system order (I) | Closure `Timer` discouraged (V) [34]; RS2008 owner-write compile error (V) [34] |
| Overwatch | Per-entity `ModifyHealthQueue`; `Contact` singleton (V translation) [37] | Later in the same frame (V translation) [37] | — | Fixed system list (V translation) [37] | Deferred create cost a frame; changed after launch (V translation) [37] |
| Doom 3 | `PostEventMS`; global sorted list (V) [38] | After thinks; due-now posts fire same pass (V) [38] | Absolute ms (V) [38] | FIFO ties; saved in list order (V) [38] | 4,096 per frame → hard error (V) [38] |
| Source | `CEventQueue::AddEvent(..., activator, caller)`; think slots (V) [39] | After thinks (V) [39] | Float seconds (I/O), ticks (think) (V) [39] | FIFO ties (V) [39] | Missing target dropped with DevMsg (V) [39]; float round trip on restore (I) |
| Go libs | Donburi bus in a component; Ark observers; GoWorld timers as data (V) [47][48][49] | Loop until empty, no cap (V) [47][49] | GoWorld absolute `FireTime` (V) [49] | Ark observer order undefined (V) [48] | GoWorld saves timers by map range (random) (V) [49] |

---

## 3. The seven questions

### Q1. How do shipped engines expose request queues, deferred commands and scheduled events?

What they do:
- Deferred commands are **opaque closures** in Bevy and Mass. (V) [9][24] They cannot be saved, inspected or cancelled well. (V) [7][8]
- Request lists with **data**: SpatialOS GDK (flat list per type), Overwatch (`Contact`, `ModifyHealthQueue`), Unity ECB `AppendToBuffer`. (V) [32][37][20]
- Owner-only writes: SpatialOS (runtime), SS14 (compile error) (V) [31][34]; Mover ("not directly modifiable externally") (S) [26].
- SS14 uses **sync owner methods**, not queues. "Method Events are *prohibited*." (V) [33]
- Delays: almost all store an **absolute due time** on data: Doom 3, Source, Quantum, GoWorld, SS14 (V) [38][39][29][49][35]; Mover (S) [26].
- Named delayed-event queues: Doom 3 `idEvent`, Source `CEventQueue`, Mass `DelaySignal`. (V) [38][39][24] Doom 3 and Source fire after thinks; Mass fires from its own subsystem tick. (V) None fires at tick start.

Pick: typed data requests in an owner queue, absolute due tick, data-only delayed events. (I) It keeps the parts that are saveable and sortable, and drops closures.

### Q2. What makes these APIs easy to use right and hard to use wrong?

| Lesson | Evidence | Our API |
|---|---|---|
| One name per mechanism | Bevy 0.17 split `Event` (observed) from `Message` (buffered): "fundamentally very different things" (S) [13] | `Send`/`Drain` for requests, `EmitSystemEvent` for events, `Schedule` for delayed. (I) |
| Make the order key a required argument | Unity ECB: sortKey is the first arg of every call (V) [18] | `Source`, `Target`, `Subject` required; zero Handle rejected. (I) |
| Build the context with a helper | GAS `MakeEffectContext` (S) [28]; Source passes activator + caller as required args (V) [39] | `Root` / `Child`; `Header{}` literal flagged by analyzer. (I) |
| Fail loud on setup errors | Cardinal panics on unregistered events "in release builds too" (V) [1]; Unity throws on cycles (V) [19]; Mass "cuts the chain at an arbitrary location" and only logs (V) [24] | Startup panics (Q6). (I) |
| Normal gameplay failures are counted, not panics | Bevy checks the entity at apply and errors (V) [9]; Flecs discards commands to dead entities (V) [14] | Dead target / over depth = drop + count; panic only in TestWorld and DST. (I) |
| No silent loss | Bevy: "dropped silently" (V) [6] | Requests wait for the next drain; events need the order check. (I) |
| No loop-until-empty | Robust, Donburi, GoWorld loop with no cap (V) [34][47][49]; Doom 3 hard-errors at 4,096 (V) [38] | Drain a frozen copy. (I) |
| Payload travels with its target | Mass: payload by position went to wrong entities after reorder (V) [25] | Each request holds its own `Target`. (I) |
| Level check, not edge | Quantum `HasStoppedThisFrame` (V) [29] | `Due <= now`. (I) |
| Define the boundary | Mover off-by-one on the last frame (V forum) [26] | Test: `Due == now` runs this tick. (I) |
| Don't defer create | Overwatch: deferred create cost a frame; fixed after launch (V translation) [37] | Owner creates entities at once in its drain. Only cross-feature spawn requests take a tick. (I) |
| Convention alone leaks | Overwatch: rule-breaking code is "the main source of complexity" (V translation) [37]; SS14 made it a compile error (V) [34] | Analyzers (Q6). (I) |

### Q3. Owner queue API: one generic type or one type per owner?

What engines do:
- SpatialOS GDK: one list **per command type**, all entities share it. (V) [32]
- Mover: three separate queues by kind. (S) [26]
- Overwatch: per-entity queue component (V translation) [37]. Unity: dynamic buffer per entity, or a singleton (V) [20].
- Flecs author: per-entity event queues are "inefficient/difficult" (V) [16]. User measured +130 µs/tick for a queue on 500 entities, even empty (S: user measurement).

Pick (I):
- **One generic `Queue[R]`, one singleton per request type.** An owner may own several (damage, heal) and drains them in a fixed order in its one drain system.
- Why not a hand-written type per owner: same header, sort and tests, written once.
- Why not one mixed queue: Go needs an interface payload, and components cannot hold interfaces (V) [1].
- The component name comes from the payload (`"queue_" + R.Name()`). A fixed name would make the second instantiation panic at registration (V) [1].

| Step | Pick | Why |
|---|---|---|
| Declare | `queue.Declare[R](w)` + `cardinal.WithDrains[Queue[R]]()` on the drain system | One drain per type, checked at startup. Option naming follows `WithPhase` (V) [1]. (I) |
| Add | `queue.Send(w, reqs...)` | Batched path is the easy path (V cost) [4] |
| Drain | `for e, r := range queue.Drain[R](w)` | Same shape as `SystemEvents` and `Commands` (V) [1]. `break` keeps the rest: Overwatch-style budgeting (V translation) [37] |
| Sort | Copy out (the drain does anyway), `slices.SortFunc` on the total key | Key is total, so unstable = stable result, 0 allocs (V cost) [4] |
| Due ticks | `Due <= now` yields; others go back; `NextDue` skips idle ticks; Send range-checks Due | Absolute tick (V) [38][29]; skip when idle like Bevy `run_if` (V) [6] |
| Source / cause / depth | `Root`, `Child`; `Cause 0` invalid | Context built by helper (S) [28] |
| Depth cap | `Depth > MaxDepth` = drop + count + log; TestWorld and DST panic | Per-chain cap is finer than Mass's global 5-pass cap (V) [24] |
| Target gone | Drain resolves `Handle`; stale = drop + count | Doom 3 `idEntityPtr`, Source EHANDLE (V) [38][39] |
| Timing | Only "applied by the next drain" is promised | A sender before the drain gets same-tick apply; after it, next tick. Moving a system changes timing. The startup check prints each sender's place. (I) |

### Q4. Delayed-event queue: schedule, cancel, target gone, pauses, growth

| System | Storage | Ties | Cancel | Target gone | Bound |
|---|---|---|---|---|---|
| Doom 3 | Sorted linked list, O(n) insert (V) [38] | FIFO (V) | By (object, event) scan (V) | Cancel on destroy + spawnId check (V) | Pool 4,096; 4,096/frame → error (V) |
| Source I/O | Sorted list (V) [39] | FIFO (V) | By caller, by (target, input) (V) | Resolve at fire; drop with DevMsg (V) | none found |
| Unreal timers | Handle-based (S) [45] | — | `ClearTimer(handle)` (S) | "canceled automatically" (S) | — |
| Linux wheel | 64 buckets/level, non-cascading (V) [40] | LIFO in a bucket (V) | `timer_delete`; `timer_shutdown` blocks re-arm (V) | — | Force-expire past max (V) |
| Netty | Hashed wheel with rounds (V) [41] | — | `Timeout.cancel()`: flag + lazy removal (V) | — | `maxPendingTimeouts`; 100,000/tick (V) |
| Kafka | Hierarchical wheel + bucket DelayQueue (V) [42] | — | O(1) unlink (V) | — | Old lazy design hit OOM (V) |
| Go runtime | 4-ary heap per P (V) [43] | Random on purpose in fake time (V) | Zombie flag; purge at >25% (V) | `seq` squelches stale fires (V) | — |
| Quantum | `FrameTimer` field in a component (V) [29] | — | Overwrite | — | — |

Per-handle cancel exists: Unreal (S), GoWorld `CancelTimer` (V) [49], Netty (V). Doom 3 and Source delayed queues cancel by scan (V).

Pick (I):
- **Schedule:** absolute tick, `at` in [now+1, now+MaxHorizon]. Outside = panic in TestWorld; clamp + log + count in production. Netty and Source clamp past times silently (V) [41][39]; we count them.
- **Storage:** one `cardinal.Slice` per event type, unsorted. Sort only the due part at fire time. `NextDue` skips idle ticks. Delays are short (1–100 ticks), so wheels and heaps don't pay off. Measure before changing.
- **Upgrade path past about 10k items of one type:** per-tick buckets (Varghese & Lauck Scheme 4 (V) [44]). Not now.
- **Cancel:** by typed `Ticket[E]` or by predicate. Eager delete, so no zombies to purge (Kafka's OOM shows why flag-cancel needs a bound (V) [42]). Tickets are never reused, so an old ticket returns false. A cancel on the due tick is too late (§1.4); add a test.
- **Who cancels:** the owner of the event, directly (it owns the queue). Cancel of another feature's work = a request to that owner.
- **Target gone:** resolve `Subject` at fire time and drop if stale, like Doom 3 / Source / Go `seq` (V) [38][39][43]. Eager cancel on destroy would need every destroyer to know every queue; that breaks rule 1.
- **Pause:** whole world paused = no tick = nothing fires (Doom 3 `g_stopTime` (V) [38]). Per-feature pause: the owner keeps remaining ticks in its component. Quantum has `FrameTimer` (ignores time scale) and `GameTimer` (pauses and slows with `GameTimeScale`) (V) [29]. Add a second clock only if a game needs it.
- **Growth:** `MaxLen` per type; over it = drop + error (Netty, Doom 3 caps (V) [41][38]).

### Q5. Determinism and snapshots

What engines do:
- Doom 3 and Source save the queue in list order, so ties survive a load. (V) [38][39]
- Quantum: all state in the Frame; "Writing gameplay state into the frame context will cause desyncs." (V) [30]
- bevy_ggrs: `Messages` and `Local` are not snapshotted; use components. "Always sort queries that mutate state." (V) [12]
- GoWorld saves timers by ranging a Go map (random order) and re-makes IDs on restore. (V) [49]
- Factorio: "saving, exiting, and loading the save shouldn't change any observable behavior." (V) [46] Heavy mode saves and loads every tick (S) [46].

Cardinal facts:
- Snapshots save components only. Not saved: system events, client commands, row order. (V) [1]
- Restore sets `height = snapshot + 1`. Absolute due ticks stay valid. (V) [1]
- Entity IDs are restored exactly; next-ID choice is the same live and restored. (V) [1]
- Search row order is **not** the same after restore. (V) [1][3]
- Init runs before restore. A new singleton made by Init is wiped by an older snapshot. (V) [1]
- `Init()` does not clear system events. Events emitted by Init systems reach the first tick after every restart. (V) [1]

Who is hit by restore divergence: DST snapshot round trips, a shard restarting from a snapshot, and any client that re-runs server code from a snapshot. `RequireDeterministic` compares two fresh runs, so it misses this. (V) [1]

Pick (I):
- **Store:** `Queue[R]` and `Delayed[E]` with full headers, `NextSeq`, `NextDue`, `Drained`. Entity generations, including those of free IDs (else a restore resets them and a stale Handle matches again).
- **Don't store:** Go scratch slices (empty when a system returns), limits (code), one-tick events (cleared before the snapshot).
- **Sort keys:** requests `(Due, Target.ID, Source.ID, Seq)`; delayed events `(Due, Subject.ID, Source.ID, Seq)`.
  - Due and IDs survive a restore unchanged. (V) [1]
  - Seq may only break ties between requests one sender makes in program order, never between search rows. Rule: `Source` = the entity your loop is on.
  - Why not `(Due, Seq)`: a Seq stamped in a search loop follows row order, which changes on restore (V) [1]; note 08 contradiction 2.
  - Why not a per-tick Seq: it repeats across ticks, so the key is not total (note 08 contradiction 3).
- **Limit:** the queue cannot fix order inside a sender. "Hit the first 5 targets" over a search still differs after restore. That is a Cardinal-wide search-order issue.
- **Old snapshot without the singleton:** fail loud: the `Single()` error says "run `world purge`". No lazy create.

### Q6. Testing and enforcement

What engines do:
- SS14: `[Access]` + analyzer → compile error RS2008 on outside writes. (V) [34]
- Bevy CI asserts zero schedule ambiguities for its main and render apps (V) [11]. Debug `shuffle_seed` finds order that holds by chance (V) [11].
- Unity throws on system cycles; Mass logs and cuts. (V) [19][24]
- Netcode warns on RPCs unconsumed for 4 frames. (V) [21]
- SS14 save-load-save test and "run 5 ticks, nothing changed". (V) [36]
- Factorio heavy mode. (S) [46]

Pick (I):

| Check | Catches | Kind |
|---|---|---|
| Owner-write analyzer (prototype, 44 lines: flags `e.Set(h)`, `e.Set[T](h)`, `e.Remove[T]()` from another package; `Create`/`Destroy` not yet; its `TestUnusedWrite` is red because it reuses the owner `want` comments (V) [5]) | Rule 1 broken | `go vet -vettool`, built with Go 1.27 (V: installed golangci builds are older) [5] |
| Header analyzer (same skeleton): `Header{}` literal outside `queue`; `Root` inside a loop over `SystemEvents` or `Drain` | Hand-made depth; depth reset in a reader | Lint |
| TestWorld: a system declared `WithReads`/`WithDrains` that calls `Root` | Depth reset through a helper | Test-mode panic (needs "current system") |
| `WithDrains[T]`: exactly one system per queue type | Two owners, or none | Startup panic |
| `WithEmits`/`WithReads`/`WithSends`: every reader after every emitter (phase, then index); print senders after the drain | Reader too early; timing surprises | Startup panic / print |
| TestWorld: observed `Drain`/`Send`/`Emit`/`SystemEvents` calls must match the declarations | Undeclared use | Test-mode panic |
| Second drain in one tick | Draining twice | Runtime panic |
| `Cause == 0`, zero Handle, depth or Due out of range in `Send`/`Schedule` | Missing helper, runaway chain, tick bugs | Panic in TestWorld and DST, counted in prod |
| Two instantiations (`Queue[A]`, `Queue[B]`) in one world | Name clash | Unit test |
| Restore determinism: snapshot + restore every tick, compare to an unbroken run | Bad sort key, state outside components | Engine test helper |
| Boundary: `Due == now` fires this tick; cancel on the due tick returns false | Off-by-one, cancel race | Unit test |

Not caught by `unusedwrite`: "forgot `Set`" after `Get` (V) [5]. `Send` owns its Get/Set, so the queue itself cannot hit it.

exhaustruct is no help: it is off in WE (V) [1], and an explicit `Depth: 0` passes it anyway.

### Q7. Cost in a Go ECS with value components

Measured on WE main (48-byte pointer-free request, 1,000 requests, i9-11900K) (V) [4]:

| Operation | Time | Bytes | Allocs |
|---|---|---|---|
| Plain `[]Req`, reused | 2.7 µs | 0 | 0 |
| `immutable.Slice` Append one by one | ~250 µs | 832 KB | 1,061 |
| `immutable.Slice` `Insert(Len)` one by one | ~250 µs | 832 KB | 1,061 |
| One `Append(xs...)` | ~20 µs | 59 KB | 93 |
| 10 senders × `Append(100…)` | ~22 µs | 65 KB | 101 |
| `slices.SortFunc` | 81 µs | 0 | 0 |
| `immutable.Slice.SortStableFunc` (with build) | 131 µs | 111 KB | 71 |
| Header via method on type param / via field | 6.8 µs / 0.38 µs | – | – |

Other facts:
- `immutable.Slice` copy is a pointer copy (0.9 ns). (V) [4]
- Front `Delete(0,1)` as a queue is a listed mistake: 27 µs at 4,096. (V) [2]
- Per-entity empty queue: +130 µs at 500 entities. (S: user measurement)
- Singleton lookup = map lookup + linear scan over archetypes. (V) [1]
- Pointer-free values are cheaper for the GC. (V) [52]

Pick (I):
- Singleton per type. Batched `Send`. Field header in a generic wrapper.
- Drain: copy out to a scratch slice, `SortFunc`, apply, then put leftovers back with one `Append`.
- Rough budget at 1,000 requests/tick: ~20 µs send + ~81 µs sort + copy and apply, plus one singleton lookup per `Send` and the owner's own `Get`/`Set`. About 0.2–0.3% of a 50 ms tick. (I, not measured in a World)

---

## 4. Alternatives considered

| Alternative | Seen in | Why it lost |
|---|---|---|
| Sync owner methods | SS14 (V) [33] | Works at scale, but no fixed drain point, no due tick, re-entry risk. GAS needed scope locks and still had bugs (S) [28]. Rule 2 is decided. |
| Observers / immediate triggers | Bevy, Flecs, Ark (V) [10][16][48] | Order undefined; no depth cap; Flecs author prefers a queue for sorting and peeking (V) [16]. |
| Opaque closures | Bevy Commands, Mass lambdas (V) [9][24] | Not saveable, not cancellable, not inspectable (V) [7][8]. |
| Per-entity queue component | Overwatch, Unity buffers (V) [37][20] | +130 µs/tick empty at 500 entities (S: user). |
| One mixed queue for all types | — | Needs interface payloads; components can't hold them (V) [1]. |
| `(Due, Seq)` key only | notes 03, 06 | Breaks after restore when senders loop over a search (V) [1][3]. |
| Per-tick Seq; `uint32` Seq | notes 07, 02 | Not total across ticks; `uint32` wraps in ~60 h at 1,000/tick, 20 Hz (note 08). |
| `immutable.Map` keyed by packed (due, seq) | note 01 | Key has no room for Target and Source, so it has the restore problem; Map `Set`/`Delete` worst case O(n) (V) [2]. |
| Heap or timing wheel | Go, Linux, Netty, Kafka (V) [40]–[43] | Built for many long, mostly cancelled timers. Ours are short and few. Go heap and Linux buckets have no tie order (V). |
| Go send buffer flushed at tick end | note 01 | No hook runs between the last system and the snapshot (V) [1]. A late request in the buffer is lost on restore. Batched `Send` gets most of the win (V cost) [4]. |
| Spawn-serial component in the plugin | note 06 | Every target needs the component; one without it is a hole. Only queues get the fix; IDs stored in other components keep the reuse bug. |
| Delay input (`after Ticks`) | Doom 3, Source (V) [38][39] | A second time model next to absolute `Due`. The range check catches the same bugs (underflow, past ticks). |
| Per-queue depth cap | notes 02, 07 | A chain crosses queue types, so caps per type don't bound it. One constant. |
| Callback `Drain(w, func)` | — | Iterator matches `SystemEvents` and `Commands`, and `break` gives budgeting. |
| Depth-first resolution in one flush | Flecs nested queues (V) [15] | Fights fixed drain points; harder to cap. Owner decides "one hit per projectile" in its drain. |
| Lazy create of a missing singleton | note 01 | Compat code for old snapshots; a structural change mid-loop. Fail loud instead. |
| Global per-drain cap only | Mass 5 passes, Doom 3 4,096 (V) [24][38] | Stops unrelated work. Per-chain depth cuts only the runaway chain. `MaxLen` stays as the fan-out guard. |

---

## 5. Engine vs plugin vs game

| Who | Writes | Status |
|---|---|---|
| Engine (Cardinal) | Entity generation: `Handle{ID, Gen}`, `Entity.Handle()`, `World.Resolve(Handle) (Entity, bool)`. Gen starts at 1, so a zero Handle is never valid. Gens of all IDs below `next_id` (free ones too) go in the snapshot. (G1) | New (I) |
| Engine | System options `WithEmits[T]()`, `WithReads[T]()`, `WithDrains[T]()`, `WithSends[T]()` + startup order check | New (I). Today `systemConfig` has only `phase` (V) [1] |
| Engine | "Current system" index, for test-mode checks | New (I). Not tracked today (V) [1] |
| Engine | `First` phase before PreUpdate, reserved for fire systems (Bevy has a `First` schedule for message update (V) [6]) | New (I). No slot today (V) [1] |
| Engine | Panic on `EmitSystemEvent` during Init (closes the replay after restart) | New (I). Check existing Init emitters first |
| Engine | `TestWorld` restore-determinism helper | New (I) |
| Queue plugin (`pkg/cardinal/queue`) | `Header`, `Root`/`Child`/`At`, `Limits`, `Req`, `Queue`, `Declare`, `Send`, `Drain`, `Pending`, `Delayed`, `Ticket`, `Schedule`, `Cancel`, fire systems, drop counters | New (I) |
| Tools | Owner-write analyzer (from the prototype); Header analyzer | Owner prototype works (V) [5]; Header analyzer new (I) |
| Game | Payload types and limits; owner drain systems; typed helpers like `combat.Damage(...)`; cause ids; folding, immunity and rejects inside the drain | Game code (I) |

Modifiers (armor, shields) are **component leaves the owner reads in its drain**, not edits to the request. SS14 does this with mid-apply events instead (V) [35]; our events fire after the fact. (I)

---

## 6. Not found, unverified, open

Not found or unverified:
- Unity 6.6 "ECB multi-playback deprecated": forum returned 403 to the verifier. (S)
- Unreal Mover, GAS and timer handles: Epic source is gated; only docs, API snippets and GASDocumentation. All (S).
- Overwatch: which system drains `ModifyHealthQueue`, when, and in what order. Not in the translation.
- No engine with a **per-request chain depth** field. Per-drain count caps exist: Mass `MaxIterations = 5`, Doom 3 4,096 then a hard error, Netty 100,000 per tick. (V)
- No engine that fires delayed events at **tick start**.
- No primary per-command cost numbers for Unity ECB or Bevy Commands.
- The user's "+116 µs" was not re-measured on current main.
- No benchmark of `Send` + `Drain` inside a Cardinal World (only Slice ops and sorts).
- Probe A's exact orders differ between the two probes; the mechanism (restore reorders rows) is confirmed.
- Factorio forum and Hive Workshop threads: Cloudflare/403; snippets only. (S)
- Bevy 0.17 release notes read through a summary tool. (S)
- Go 1.27 spec text on generic methods not read; the repo uses them widely. (V use, S spec)
- Whether `unusedwrite` is on by default in vet/gopls: not checked.

Open (I):
- How a sender learns the outcome (target gone, rejected): a one-tick event from the owner, carrying the header.
- Entity creation stays immediate inside the owner; only cross-feature spawn requests take one tick.
- Same-tick vs next-tick apply depends on registration order (Q3). If that proves confusing, the fix is "always next tick", at one tick of latency.

---

## 7. Sources

Local (WE `main` f7d4a52b):
1. Cardinal source: `pkg/cardinal/{system.go, world.go, entity.go, testworld.go, dst.go}`, `internal/ecs/{world.go, system.go, world_state.go, archetype.go, system_event.go, component.go}`, `.golangci.yaml`. `world-engine/pkg/cardinal/`
2. `pkg/immutable/{slice.go, map.go, README.md, DESIGN.md}`. `world-engine/pkg/immutable/`
3. Restore-order probes (notes 01 Probe A, 08 re-probe), scratch copies only.
4. Benchmarks: [`queue-api-notes/benchmarks.md`](queue-api-notes/benchmarks.md).
5. Owner-write analyzer prototype: [`queue-api-notes/ownercheck.md`](queue-api-notes/ownercheck.md).

Bevy v0.20.1:

6. Messages: https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/message/messages.rs ; `First` schedule: https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_app/src/sub_app.rs
7. DelayedCommands: https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/delayed_commands.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/lib.rs
8. PR #23090 https://github.com/bevyengine/bevy/pull/23090 ; issue #15129 https://github.com/bevyengine/bevy/issues/15129 ; PR #24179 https://github.com/bevyengine/bevy/pull/24179
9. Commands and executors: https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/system/commands/mod.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/executor/mod.rs
10. Issues #19569 https://github.com/bevyengine/bevy/issues/19569 ; #14890 https://github.com/bevyengine/bevy/issues/14890
11. Ambiguity test: https://github.com/bevyengine/bevy/blob/v0.20.1/tests/ecs/ambiguity_detection.rs ; schedule.rs https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/schedule.rs
12. bevy_ggrs pitfalls: https://github.com/gschup/bevy_ggrs/blob/main/docs/pitfalls.md
13. Bevy 0.17 notes https://bevy.org/news/bevy-0-17/ ; 0.16 notes https://bevy.org/news/bevy-0-16/

Flecs v4.2.0:

14. Manual (deferred ops): https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/Manual.md ; release https://github.com/SanderMertens/flecs/releases/tag/v4.2.0
15. Command queue: https://github.com/SanderMertens/flecs/blob/v4.2.0/src/commands.c ; https://github.com/SanderMertens/flecs/blob/v4.2.0/src/commands.h
16. Observers manual: https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/ObserversManual.md
17. Timers: https://github.com/SanderMertens/flecs/blob/v4.2.0/include/flecs/addons/timer.h ; Systems: https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/Systems.md

Unity:

18. ECB playback (6.5): https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entity-command-buffer-playback.html ; automatic playback: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entity-command-buffer-automatic-playback.html
19. Entities 1.5.0 source (`EntityCommandBuffer.cs`, `ComponentSystemSorter.cs`): https://download.packages.unity.com/com.unity.entities/-/com.unity.entities-1.5.0.tgz
20. Dynamic buffers: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-buffer-introducing.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-buffer-command-buffer.html
21. Netcode 7.2.0 source: https://download.packages.unity.com/com.unity.netcode/-/com.unity.netcode-7.2.0.tgz ; docs https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/command-stream.html ; https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/prediction-details.html
22. Forums: https://discussions.unity.com/t/entities-as-events-yes-or-no/764755 ; https://discussions.unity.com/t/event-component-data/816934
23. Unity 6.6 ECS status (unverified): https://discussions.unity.com/t/ecs-development-status-for-unity-6-6/1724480

Unreal:

24. UE 5.6.1 Mass source copy (branch `master`): https://github.com/Viktor4221/ue5.6.1/tree/master/UnrealEngine-5.6.1-release/Engine (MassCommandBuffer, MassEntityManager, MassSignalSubsystem, MassProcessorDependencySolver)
25. Forum: https://forums.unrealengine.com/t/masscommandaddfragmentinstances-adding-fragments-to-incorrect-entities/731341 ; https://forums.unrealengine.com/t/question-about-fmassbatchedcommand-best-practices/2684678
26. Mover: https://dev.epicgames.com/documentation/en-us/unreal-engine/comparing-mover-and-character-movement-component-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/mover-features-and-concepts-in-unreal-engine ; https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/FScheduledInstantMovementEffect ; forum https://forums.unrealengine.com/t/mover-layeredmoves-end-one-frame-in-advance/2720552
27. GAS effects: https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-effects-for-the-gameplay-ability-system-in-unreal-engine
28. GASDocumentation (community): https://github.com/tranek/GASDocumentation

Others:

29. Quantum Frame Timer: https://doc.photonengine.com/quantum/v3/concepts-and-patterns/frame-timer
30. Quantum Systems / DSL / Commands / Frames: https://doc.photonengine.com/quantum/current/manual/quantum-ecs/systems ; https://doc.photonengine.com/quantum/current/manual/quantum-ecs/dsl ; https://doc.photonengine.com/quantum/current/manual/commands ; https://doc.photonengine.com/quantum/v3/manual/frames
31. SpatialOS docs (archived): https://web.archive.org/web/20210928052112/https://documentation.improbable.io/spatialos-overview/docs/commands ; https://web.archive.org/web/20210928045444/https://documentation.improbable.io/spatialos-overview/docs/authority-and-interest
32. SpatialOS GDK for Unity: https://github.com/spatialos/gdk-for-unity/tree/master/workers/unity
33. SS14 conventions: https://docs.spacestation14.com/en/general-development/codebase-info/conventions.html
34. RobustToolbox @0fb119a9: https://github.com/space-wizards/RobustToolbox/tree/0fb119a9258ed22a4f0d9bcef9f4521feff989ed
35. SS14 content @75a85474: https://github.com/space-wizards/space-station-14/tree/75a8547472043e8d258f0e577563954f19f1b4f7
36. SS14 SaveLoadSaveTest: https://github.com/space-wizards/space-station-14/blob/master/Content.IntegrationTests/Tests/SaveLoadSaveTest.cs
37. Overwatch GDC 2017 (Chinese translation): https://www.sohu.com/a/148848770_466876
38. Doom 3: https://github.com/id-Software/DOOM-3/blob/master/neo/game/gamesys/Event.cpp
39. Source SDK 2013: https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/server/cbase.cpp ; https://github.com/ValveSoftware/source-sdk-2013/tree/master/src/game
40. Linux timer wheel: https://github.com/torvalds/linux/blob/master/kernel/time/timer.c
41. Netty: https://github.com/netty/netty/blob/4.1/common/src/main/java/io/netty/util/HashedWheelTimer.java
42. Kafka: https://github.com/apache/kafka/blob/trunk/server-common/src/main/java/org/apache/kafka/server/util/timer/TimingWheel.java ; https://www.confluent.io/blog/apache-kafka-purgatory-hierarchical-timing-wheels/
43. Go 1.25.5 runtime timers: https://github.com/golang/go/blob/go1.25.5/src/runtime/time.go ; https://github.com/golang/go/blob/go1.25.5/src/container/heap/heap.go
44. Varghese & Lauck, timing wheels: https://cseweb.ucsd.edu/users/varghese/PAPERS/twheel.ps.Z
45. Unreal Gameplay Timers: https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-timers-in-unreal-engine
46. Factorio: https://factorio.com/blog/post/fff-270 ; https://wiki.factorio.com/Desynchronization ; https://lua-api.factorio.com/latest/classes/LuaBootstrap.html
47. Donburi events: https://github.com/yohamta/donburi/blob/v1.15.8/features/events/events.go
48. Ark events: https://github.com/mlange-42/ark/blob/v0.8.3/docs/content/events/index.md
49. GoWorld: https://github.com/xiaonanln/goworld/blob/master/engine/entity/Entity.go ; https://github.com/xiaonanln/goworld/blob/master/engine/post/post.go
50. go/analysis: https://pkg.go.dev/golang.org/x/tools/go/analysis ; exhaustruct: https://github.com/GaijinEntertainment/go-exhaustruct
51. Generics cost: https://planetscale.com/blog/generics-can-make-your-go-code-slower
52. Go GC guide: https://go.dev/doc/gc-guide

## Appendix: the brief

```markdown
# Research: the best API for owner request queues and a delayed-event queue in a Go ECS

Research only. Don't edit or build anything in any repo. Don't base anything on existing game code: the standard is shipped engines and well-known libraries.

## Context (facts, check them in the source)
- Engine: World Engine's Cardinal, a Go ECS. Source: world-engine/pkg/cardinal
- Fixed tick (20 Hz). Phases PreUpdate, Update, PostUpdate. Systems run in registration order inside each phase.
- System events (`w.EmitSystemEvent`, `w.SystemEvents[T]()`) are cleared at the end of every tick.
- Snapshots save components only. Anything that must survive a restore must be a component.
- Components are plain structs. Lists inside them use `cardinal.Slice[T]` (pkg/immutable). `Insert(Len, x)` appends in place; `Append` copies.
- Singletons are found with a search (`w.Exact[struct{...}]().Iter().Single()`).
- Go 1.27 with generic methods (e.g. `e.Get[T]()`).
- Measured: a shared queue on a singleton costs about the same as events (1,000 requests per tick: events +73 µs, queue +116 µs, in a 50 ms tick). A queue component on every entity adds about +130 µs per tick even when empty (500 entities).

## The architecture (decided; design the API for it)
1. Every component has one owner feature. Only the owner writes it.
2. To change another feature's data, add a request to that owner's queue component. The owner drains it at one fixed point in the tick. A request that arrives after the owner ran waits for next tick, so nothing is lost.
3. Each request can carry a due tick (0 = as soon as possible). This is also how delays work (wind-ups, staggered hits).
4. Each request carries source, cause, depth (steps since the first trigger) and flags. A depth cap stops runaway chains.
5. To announce that something happened, the owner sends a one-tick event. Many systems can read it. A test checks that every reader runs after every sender.
6. To announce something later, the owner puts the event in a delayed-event queue. It fires at the very start of the tick, so every reader runs after it.
7. Requests are applied in a sorted, deterministic order, so a restored match gives the same result.

If your research shows a real flaw in this architecture, say so plainly, with evidence. Otherwise don't redesign it.

## Questions
1. How do shipped engines expose request queues, deferred commands and scheduled events? Cover at least: Bevy (Commands, Messages, observers), Unity Entities (EntityCommandBuffer, DynamicBuffer, request entities), Flecs, Unreal Mass (deferred commands, signals), Unreal GAS (pending effects, cues), Unreal Mover (queued moves), Photon Quantum (signals, events), SpatialOS (commands to the owner), Space Station 14 / RobustToolbox, and Overwatch (Tim Ford, GDC 2017; Chinese translation at https://www.sohu.com/a/148848770_466876). Add others you find.
2. What makes these APIs easy to use right and hard to use wrong? Find the known mistakes and complaints.
3. Owner queue API: how to declare a queue, add a request, drain it, sort it, apply due ticks, carry source/cause/depth, and enforce the depth cap. One generic type, or one type per owner?
4. Delayed-event queue API: how to schedule, cancel, handle a target that's gone, handle pauses, and stop it growing forever. What do timer queues in games and in Go libraries do (timer wheels, heaps, per-tick buckets)?
5. Determinism and snapshots: what must be stored, and what sort key makes results stable after a restore?
6. Testing and enforcement: how do engines or teams stop misuse (lint rules, tests, startup checks)? What would catch a wrong drain point or a missing depth?
7. Cost: what is cheap and what is expensive for these shapes in a Go ECS with value-type components?

## Sources
- Prefer primary sources: official docs, engine source code, talks by the people who built it.
- Include sources that criticize these patterns, and post-mortems.
- Mark every claim as verified (you read the primary source), secondary (a summary), or inferred.

## Output
- Save it as Markdown at REPORT.md (now this file).
- Verdict first: the recommended API, as Go signatures and one short usage example per queue.
- One table: engine, API shape, how it's drained, delays, determinism, known pitfalls.
- For each question: what shipped engines do, then the pick and why.
- Alternatives considered, and why they lost.
- What the API needs from the engine vs what the game writes.
- A "not found or unverified" list, then sources with links.
- Style: short sentences, bullets, small tables, no long paragraphs.
```
