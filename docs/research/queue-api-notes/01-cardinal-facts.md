# 01 — Cardinal engine facts (source-verified)

Repo: world-engine at `main` f7d4a52b (2026-11-19). Read with cat/grep/git only.
Paths are relative to `pkg/cardinal/` unless they start with `pkg/`.
Probes (tests/benchmarks) ran only in a `git archive` copy in the session scratchpad, never in the repo.

Legend: (V) read in source this session · (S) secondary/unread primary · (I) my inference.

## Sources

1. `system.go` — public System, Phase, WithPhase, Search, SearchResult.Single, Slice/Map aliases.
2. `world.go` — World struct, run loop, step, restore, Register*, events, system events, search.
3. `internal/ecs/world.go` — ecs.World, Init, Tick, Schedules.
4. `internal/ecs/system.go` — Phase consts, RegisterSystem.
5. `internal/ecs/system_event.go` — system event manager and queues.
6. `internal/ecs/ecs.go` — IterEntities, MatchExact/Contains.
7. `internal/ecs/world_state.go` — entity IDs, free list, toProto/fromProto.
8. `internal/ecs/archetype.go` — swap-remove, moveEntity.
9. `internal/command/{command,queue}.go` — client command queues.
10. `internal/event/event.go` — client event manager.
11. `snapshot/snapshot.go`, `proto/worldengine/cardinal/v1/snapshot.proto` — snapshot envelope.
12. `testworld.go` — TestWorld, RequireDeterministic.
13. `dst.go` — RunDST, verifySnapshotRoundtrip.
14. `debug.go` — introspection schedules.
15. `pkg/immutable/slice.go`, `map.go`, `README.md` — Slice/Map semantics and costs.
16. `git show 57c2eafd` (#1177, merged 2026-11-19 02:25 +0700) — Slice rewrite.
17. `docs/cardinal/{ecs,system-events}.mdx` — official docs.
18. Probe A: `internal/ecs` test, live vs restored iteration order (scratch copy).
19. Probe B: `pkg/immutable` benchmarks, queue patterns (scratch copy, i9-11900K, Go 1.27.4).

---

## 1. Phases and order

- (V) Four phases: `PreUpdate=0, Update=1, PostUpdate=2, Init=3` [4] `internal/ecs/system.go:11-20`.
- (V) Tick runs PreUpdate, Update, PostUpdate in order; each phase in registration order [3] `internal/ecs/world.go:49-53`:
  `for _, phase := range []Phase{PreUpdate, Update, PostUpdate} { for _, system := range w.systems[phase] { system.fn() } }`
- (V) Registration = append to a per-phase list. No sorting, no before/after, no sets [4] `system.go:28-38`.
- (V) Docs say the same: "Registered systems run sequentially within each tick phase, in registration order." [17] `ecs.mdx:316`.
- (V) Init runs once, in `w.init()`, which also sets `started=true` (closes registration) [2] `world.go:256-267`.
- (V) Full tick frame (`step`) [2] `world.go:280-356`:
  1. Drain client commands into per-type read buffers (`w.commands.Drain()`, :284).
  2. `run()` = ecs Tick (all three phases).
  3. `dispatchEvents` (client events + inter-shard sends, :331).
  4. Encode snapshot if `height % SnapshotRate == 0` (or debug on) (:335-350).
  5. `currentTick.height++` (:353).
- (V) `TickHeight()` returns `currentTick.height` = the tick now running [2] `world.go:569-571`.
- (I) There is no engine slot "before PreUpdate". "Start of tick" today = the first system registered in PreUpdate. That depends on plugin registration order.

## 2. System events (the "one-tick event")

- (V) Cleared by `defer w.systemEvents.clear()` at the end of ecs Tick, i.e. after PostUpdate, before client-event dispatch and snapshot [3] `internal/ecs/world.go:45-46`.
- (V) Storage: one Go slice per type, `append` on emit, `s.events[:0]` on clear [5] `system_event.go:126-140, 193-206`. Not a component, so never in a snapshot.
- (V) Readers get every event emitted *so far* this tick, in emit order [2] `world.go:788-804`. Docs: "If a receiver runs before its emitter, it will not receive any system events for that tick" [17] `system-events.mdx:66-76`.
- (V) Unregistered type: panic in release too [2] `world.go:790-817`.
- (V) `Init()` does NOT clear system events [3] `internal/ecs/world.go:30-38`. So events emitted by Init systems are seen by the first tick. (I) After a restore they are still there too (restore does not clear them, see §4).
- (V) `TestWorld.RunSystem` clears after the one system [12] `testworld.go:188-195`.
- (V) Test-only tap: `systemEventTap` sees each emit, set only by TestWorld [2] `world.go:57, 815-817`. There is no matching tap on reads.

## 3. RegisterSystem options

- (V) Only option is `WithPhase(phase)`; `WithHook` is a deprecated alias [1] `system.go:39-78`. `systemConfig` has one field: `phase`.
- (V) Default phase is Update [2] `world.go:531`.
- (V) System name = `fmt.Sprintf("%T", s)`; no dedupe of names [2] `world.go:536`, [4] `system.go:28-32`.
- (V) Panics after start (`ErrWorldStarted`) and on nil systems [2] `world.go:520-529`.
- (V) `runSystem` just wraps `s.Run(w)` in an optional span. The engine does not record "which system is running" anywhere except the trace ctx [2] `world.go:552-562`.
- (V) System interface: `Run(w *World)` [1] `system.go:24-26`. No declared reads/writes/emits.

## 4. Snapshot contents and restore

- (V) Envelope: `tick_height`, `timestamp`, `world_state`, `version` [11] `snapshot.proto:12-20`.
- (V) WorldState: `next_id`, component name table, and per entity `{id, component indices, msgpack payloads}` [7] `world_state.go:342-379`. Entities written in ascending ID order. Free list is not stored; it is "every ID below next_id with no entity" [11] `snapshot.proto:38-40`.
- (V) Not saved: system events, pending client commands, pending client events, archetype layout, row order. (Only `ToProto` state goes in; [2] `world.go:380-392`.)
- (V) Restore sets `height = snap.tick_height + 1` [2] `world.go:423`. So an absolute due tick stored in a component stays meaningful after restore. (I)
- (V) Restore does not restore `timestamp` (only height) [2] `world.go:418-423`.
- (V) Order at boot: `w.init()` (Init systems) THEN `w.restore()` [2] `world.go:190-192`. Restore replaces the whole entity state [7] `world_state.go:467-468`.
  - (I) Pitfall: a singleton (e.g. a new queue component) created by an Init system is wiped if the loaded snapshot predates it. `Single()` then returns `ErrSingleNoResult`. Needs `world purge` or a lazy "create if missing".
- (V) DST checks the restored world re-encodes to the same bytes [13] `dst.go:307-322`. It checks state bytes, not iteration order.
- (V) `RequireDeterministic` compares two fresh runs; it does not compare live vs restored [12] `testworld.go:290-316`.

## 5. Entity IDs: allocation, reuse, generations

- (V) `EntityID` is `uint32`; no generation bits [7] `world_state.go:15-16`.
- (V) Destroy pushes the ID on a min-heap free list; Create reuses the *smallest* free ID [7] `world_state.go:71-81, 130-171`:
  "free is a min-heap, so newEntity always reuses the smallest id. A snapshot stores only the gaps and restore rebuilds them ascending, so reuse order must not depend on the order ids were freed."
- (V) So which ID the next Create gets is the same live and after restore (Probe A: `next id live=5 restored=5`) [18].
- (V) IDs are stable across snapshot restore: restore re-creates each entity at its saved ID [7] `world_state.go:442-460, 509`.
- (V) Public doc warns: "It is reused as soon as the entity is destroyed, so an ID kept past that point refers to whichever entity is created next in its place." [1] `system.go:15-18`.
- (I) Pitfall for queues: a request that targets an entity by ID can hit a *new* entity if the target died and an ID was reused before the drain (same tick or a due tick later). Requests that wait (due ticks, late arrivals) need a game-owned ID or a generation check at drain time.

## 6. Search iteration order (determinism)

- (V) No Go map in the iteration path. Exact = one archetype's `entities` slice; Contains = every archetype in creation order, each in row order [6] `ecs.go:341-384`.
- (V) Row order changes by swap-remove on destroy and on archetype move [8] `archetype.go:84-116, 122-149`.
- (V) Restore builds archetypes in the order their first entity (by ascending ID) is seen, rows ascending by ID [7] `world_state.go:442-460, 507-510`.
- (V, Probe A [18]) Same world, live vs restored, `Contains[A]` order:
  - live: `[3 4 2 1 0]`
  - restored: `[0 1 2 3 4]`
- Conclusion: iteration order is deterministic for one unbroken run (same inputs → same order), but it is NOT the same after a snapshot restore. (V)
- (I) This is direct evidence for rule 7: any queue drain or request generation that depends on search order must sort by a stable key before applying. Two-run `RequireDeterministic` cannot catch this; a live-vs-restore check would.
- (V) Docs warn against structural changes during iteration: "Collect handles before making structural changes while iterating a query, so moving rows cannot skip other matches." [17] `ecs.mdx` ("Adding and Removing Components").

## 7. Singletons

- (V) Pattern `w.Exact[struct{ Q Queue }]().Iter().Single()` works: `Exact` resolves the struct fields to a component bitmap (cached per `reflect.Type`) [2] `world.go:856-893, 931-933`.
- (V) `Single()` returns `(Entity, error)`; `ErrSingleNoResult` / `ErrSingleMultipleResult` [1] `system.go:164-224`.
- (V) Cost per lookup: one map lookup on `reflect.Type`, plus `archExact` = linear scan over all archetypes comparing bitmaps [7] `world_state.go:552-560`. (I) Cheap, but O(#archetypes); cache-free by design.
- (V) There is no engine "singleton/resource" type; a singleton is just an entity [2] (no such API in the World method list).

## 8. cardinal.Slice: Insert vs Append — the brief's fact is OUT OF DATE

- (V) Before #1177 (`57c2eafd^`): `func (s Slice[T]) Append(items ...T) Slice[T] { return Slice[T]{items: slices.Concat(s.items, items)} }` — a full copy per call; `Insert` used `slices.Insert` (in-place when capacity allowed). That matches the brief and the memory note [16].
- (V) Since #1177 (merged today, 2026-11-19): Slice is a persistent 32-way trie + tail; methods are pointer-receiver mutators [15] `slice.go:52-57, 279-314`.
  - `Append` "copies the last chunk of up to 32 elements, so appending in a loop is linear" `slice.go:277-294`.
  - `Insert(i, …)` with `i == Len` just calls `Append` `slice.go:303-306`. So Insert(Len,x) and Append now cost the same.
  - Insert/Delete in the middle and front-delete over 32 elements rebuild: O(n) `slice.go:39-44, 312-313, 355-368`.
  - `Reset()` is O(1): `*s = Slice[T]{}` `slice.go:446-448`.
  - `SortStableFunc` only (no unstable sort, on purpose for determinism) `slice.go:450-462`.
  - README: "a queue of thousands: `Append`, then `Delete(0, 1)`" is a listed mistake; use a head index or a Map keyed by sequence [15] `README.md:150, 529-534`.
- (V, Probe B [19]) 1,000 requests of a 32-byte struct:

  | Pattern | Time | Allocs | Bytes |
  |---|---|---|---|
  | 1,000 × `Append(x)` then `Reset` | ~220–239 µs | 1,061 | 701 KB |
  | 1,000 × `Insert(Len, x)` | ~216 µs | 1,061 | 701 KB |
  | buffer in `[]T`, one `Append(buf...)` | ~20 µs | 93 | 51 KB |
  | `SortStableFunc` on 1,000 | ~105 µs | 36 | 51 KB |
  | `DeleteFunc` keep half (due-tick filter) | ~10 µs | 19 | 42 KB |

- (I) The user's measured "+116 µs for 1,000 req/tick" was likely taken before #1177 (Insert(Len) amortized in place). Re-measure on current main. Single-element Append now costs ~220 ns + ~700 B each.
- (I) API consequence: senders should not each `Get → Append → Set` the owner's queue component. Better: append to a per-tick Go buffer (engine-held, like system events), then the engine or the owner moves leftovers into the component in one batched `Append` before the snapshot.
- (V) `immutable.Map[K,V]` is sorted by key; keys are ints/strings only; iteration and encoding in key order; `Range(lo,hi)` is O(log n + k) [15] `map.go:19-60`, `README.md:512-525`. (I) A Map keyed by packed `(dueTick, seq)` uint64 is a natural due-tick queue: drain = `Range(0, now+1)` then delete.

## 9. Go 1.27 generic methods

- (V) `go 1.27.4` in `go.mod:7` and `.prototools`.
- (V) Used widely: `func (w *World) Commands[T Command]()`, `SystemEvents[T]`, `EmitSystemEvent[T]`, `Create[T any]`, `Entity.Get[T]`, `systemEventManager.register[T]` [2] `world.go:629, 790, 809, 908`; [5] `system_event.go:89`.
- (S) Generic methods cannot satisfy interface methods (interfaces cannot declare type-parameterised methods). I did not read the spec text this session.
- (I) So `w.Request[Owner, Req](...)`-style helpers are possible as World methods; a plugin-level `queue.Send[T](w, r)` free function works too.

## 10. Hooks for startup checks

- (V) What exists:
  - `Plugin.Register(w)` before start [1] `plugin.go`.
  - Init-phase systems (run once, before restore) [2] `world.go:190, 256-267`.
  - `started` flag; every `Register*` panics after it [2] `world.go:28-33, 507, 521, 605, 688, 778, 844`.
  - `ecs.World.Schedules()` lists systems per phase in order with names [3] `internal/ecs/world.go:69-85`; exposed only through the debug introspection RPC [14] `debug.go:85-117`. Not public on `cardinal.World`.
  - `RunDST`, `RequireDeterministic`, `TestWorld` for tests [12][13].
- (V) What does not exist: no public "list systems" API, no "after registration, before first tick" callback, no per-system declared reads/writes/emits, no read-side tap for system events, no "current system" field.
- (I) A reader-after-sender check therefore needs engine help. Options seen in the code shape:
  - a. Declarative: system options `WithEmits[T]()`, `WithReads[T]()` (systemConfig gains fields) + a check at `init()` over the ordered lists. Catches unexercised paths.
  - b. Observed: TestWorld records (system index, event type) for each emit (tap exists) and each `SystemEvents[T]()` call (new tap), then fails if a reader index < an emitter index. Only sees paths a test runs.

## 11. Existing scheduling/timer/queue helpers

- (V) None in `pkg/cardinal` or `pkg/immutable`. `grep -rniE "timer|delay|cooldown|due ?tick|scheduled|deferred"` over `pkg/` (non-test) hits only box2d, micro client, service, telemetry, physics2d e2e harness.
- (V) Closest mechanisms:
  - client command queue: mutex + Go slice, drained at tick start, arrival order (network order, not deterministic) [9] `queue.go:45-77`, `command.go:177-193`.
  - client event manager: buffered channel + overflow slice, dispatched after the tick [10] `event.go:59-127`.
  - system events: per-type Go slice, cleared at tick end [5].
- (V) `Commands[T]()` is read-only for the whole tick; every system sees the same list [2] `world.go:629-652`.

## 12. What the engine would need to add (I unless noted)

| Need | Today | Smallest engine addition |
|---|---|---|
| Owner queue that survives tick + snapshot | Component Slice works, but single Appends cost ~220 ns each (V, Probe B) | Per-tick Go-side send buffer per request type, merged into the owner's component with one batched Append (or drained directly the same tick). Or keep it purely in a plugin. |
| Due ticks | `TickHeight()` absolute and restored as height+1 (V) | Nothing in core. Store absolute due tick. Use Map keyed by (due, seq) or Slice + DeleteFunc. |
| Start-of-tick delayed-event drain | No slot before PreUpdate (V) | Either a reserved first phase (e.g. `First`) or a guaranteed-first system registered by the queue plugin with a startup check that it is index 0 of PreUpdate. |
| Order check | Schedules() internal (V); no reads/emits metadata (V) | Public read-only schedule API + `WithReads/WithEmits` options or a read tap in `SystemEvents[T]`; check at `init()` or in TestWorld. |
| Deterministic apply order | Search order differs after restore (V, Probe A) | Nothing in core; queue drain sorts by stable key (due, source key, seq). Optionally a live-vs-restore determinism helper in TestWorld. |
| Stale targets | No generations (V) | Game-owned IDs in requests, or a generation component checked at drain. |
| New singleton vs old snapshot | Init runs before restore (V) | Lazy create-if-missing in the plugin, or `world purge`. |
| Monotonic seq for sorting | none | A counter in the queue singleton component (snapshotted). |

## Could not find / not done

- No public API to read the system schedule from game code.
- Did not benchmark singleton lookup or a full owner-queue system in a World (only the Slice ops).
- Did not read the Go 1.27 spec text for generic method limits (marked S).
- Did not check how `reset()` (debug) interacts with a queue plugin beyond: it clears state, commands and client events, then re-runs Init (V `world.go:461-482`).
