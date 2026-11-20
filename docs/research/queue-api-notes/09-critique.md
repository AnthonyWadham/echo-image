# 09 — Critique of the queue API report

Critic pass, 2026-11-19. Read notes 01–08 and Cardinal source at WE `main` f7d4a52b. No repo edited.

## Verdict up front
- **REPORT.md does not exist.** Checked at 22:42 and again later: the research folder has only `notes/`, `bench/`, `ownercheck/`, `raw/`, `src/`. No REPORT.md anywhere under `$HOME` or the scratchpad newer than the notes.
- So no brief requirement can be marked met. Issues 2–23 are what the report must fix when it is written. Each one comes from the notes, the verification (08), or the Cardinal source.
- Count: 3 blocker, 13 major, 7 minor.

Severity: **blocker** = the design breaks or the report is unusable. **major** = a wrong or missing claim a reader would act on. **minor** = accuracy or style.

---

## Issues

### 1. Report missing — blocker
- **Location:** `REPORT.md` (now [`../queue-api.md`](../queue-api.md)).
- **Problem:** File not written. Not one brief item can be checked: verdict first with Go signatures and one example per queue, the table, "engines do / pick / why" for each question, alternatives, engine vs game split, not-found list, numbered sources with links, a V/S/I mark on every claim, plain style.
- **Fix:** Run the synthesis step. Then run this critique again on the real file. Build it from the fixes below, not from notes 02–07 as written. Those notes hold the stale and wrong claims listed in 08.

### 2. Sort key is not stable across restore — blocker
- **Location:** 03 §11 ("append order is already deterministic"), 06 Synthesis (`(Due, Seq)`), 02 §11 (`(Due, Target, Seq)`), 07 Summary 4. 08 Contradiction 2 says the same.
- **Problem:** Seq is stamped when the request is sent. A sender that loops over a search sends in row order. Row order differs live vs restored (Probe A: live `[3 4 2 1 0]`, restored `[0 1 2 3 4]`; 08 re-ran it with the same kind of result). So `(Due, Seq)` gives a different apply order after a restore. `(Due, Target, Seq)` still ties when one target gets two requests from different senders' loops.
- **Fix:** Use the key `(Due, Target.ID, Source.ID, Kind, Seq)`. Every field before Seq must be restore-stable (IDs are restore-stable, 01 §5). State the rule: Seq may only break ties between requests that one sender makes in program order, never between search rows. Add a TestWorld helper that restores from a snapshot every tick and compares a state digest with an unbroken run (Factorio heavy mode, 06). Name it in the engine split. Also say plainly that the queue cannot fix order-dependent logic *inside* a sender (for example "first 5 targets hit"). That is a Cardinal-wide search-order issue.

### 3. A generic queue component breaks on the second instantiation — blocker
- **Location:** any signature shaped like `type Queue[R any] struct{...}` used as a component (02 §11 sketch, 07 `Queued[P]`).
- **Problem:** A component needs `Name() string` (`internal/ecs/component.go:21`, `schema.Serializable`). A method on a generic type has one body for every R. So it returns the same name for `Queue[Damage]` and `Queue[Heal]`. Registering the second one panics with "already registered with a different type" (`component.go:414`). Names also cannot hold dots, so `fmt.Sprintf("%T")` fails the name rule.
- **Fix:** Make the payload name itself and build the queue name from it:
  ```go
  type Named interface{ Name() string }
  type Queue[R Named] struct { Items cardinal.Slice[Req[R]]; NextSeq uint64 }
  func (Queue[R]) Name() string { var r R; return "queue_" + r.Name() }
  ```
  Do the same for the delayed-event store (`Delayed[E]`). Add a test that registers two instantiations in one world.

### 4. Seq width and scope — major
- **Location:** 02 §11 and 04 (`Seq uint32`), 07 ("per-tick seq"). 08 Contradiction 3.
- **Problem:** A uint32 at 1,000 requests per tick and 20 Hz wraps after about 59.6 hours (2^32 / 20,000 per second). After the wrap, Seq order is wrong, and it cannot be a cancel handle. A per-tick Seq repeats across ticks. Two delayed requests sent on different ticks but due on the same tick can then tie, so the key is not total.
- **Fix:** Use `Seq uint64`, one counter per queue, kept in the queue component and never reset. Write it in the signatures.

### 5. The 02 sketch is not valid Go and invites "forgot Set" — major
- **Location:** 02 §11: `func Send[R any](w, q *Queue[R], m Meta, target EntityID, body R)`.
- **Problem:** `w, q *Queue[R]` gives `w` the type `*Queue[R]`. `EntityID` is unqualified. Passing `*Queue[R]` means the caller does `Get`, then `Send`, then must call `Set` itself. That is the "forgot Set" mistake. `unusedwrite` does not catch it (07, 08 row 16).
- **Fix:** Let the helper own the Get/Append/Set:
  ```go
  func Send[R Named](w *cardinal.World, parent Header, target Ref, delay Ticks, body R)
  func SendAll[R Named](w *cardinal.World, parent Header, reqs []Out[R]) // one batched Append
  func Drain[R Named](w *cardinal.World, apply func(Req[R]) )
  ```
  Check every signature in the report against `pkg/cardinal` (`World.Exact[T any]`, `Entity.Get[T ecs.Component]`, `TickHeight() uint64`, `EntityID = uint32`).

### 6. "Sender system index" in the key cannot be built — major
- **Location:** 07 Summary and §(a) ("key derived from (due tick, sender system index, per-tick seq) is automatic").
- **Problem:** Cardinal does not record which system is running (`world.go:552-562`; 01 §3). So a plugin cannot read a sender index.
- **Fix:** Drop it from the key. If the report keeps the idea, list "current system index" as an engine addition in the engine vs game split, and mark it (I).

### 7. Stale Slice cost claim — major
- **Location:** 02 §11 (`// Insert(Len, x): in-place append`), 04 "Concrete API ideas", 05 §E.2, 06 Synthesis.
- **Problem:** Since #1177, `Insert(Len)` calls `Append`. Both copy the tail of up to 32 elements. Cost is about 220–250 µs per 1,000 one-by-one sends; one batched Append is about 20 µs (01 §8, 07, 08 rows 8 and 14).
- **Fix:** Remove every "Insert(Len) in place" line. Quote one set of numbers with its struct size: 01 used 32 B, 07 used 48 B. Mark the user's "+116 µs" as measured before #1177 (I), and say to re-measure.

### 8. Batched sends have no flush point before the snapshot — major
- **Location:** 01 §8 and §12 ("per-tick Go buffer ... moved into the component ... before the snapshot").
- **Problem:** No plugin hook runs between `run()` and `encodeSnapshot` (`world.go:330-350`). A "last PostUpdate system" flush depends on registration order. A late request held in a Go buffer at snapshot time is lost on restore. That breaks rule 2 ("late = next tick").
- **Fix:** Pick one path: no Go-side buffer that lives past the sending system. Batching is a local `[]Out[R]` that the same system flushes with `SendAll` before it returns. Add a test that asserts no plugin-held buffer is non-empty at tick end. Leave any engine-held buffer out of the design.

### 9. The depth cap can be bypassed silently — major
- **Location:** 07 §(a) (`hdr.Child()`, exhaustruct), 02 §11 ("repeats ... do not inherit depth"), 05 §E.4.
- **Problem:** Four bypasses:
  - a. Header fields must be exported for the snapshot, so `Header{Depth: 0}` compiles anywhere. exhaustruct is off in WE (`.golangci.yaml:139`), and an explicit `Depth: 0` passes it anyway.
  - b. One-tick events carry no header. A listener that sends a new root request resets depth. Request → event → request loops forever, one hop per tick.
  - c. 02's "repeat flag that does not inherit depth" is a bypass by design.
  - d. Delayed events re-sending themselves.
- **Fix:**
  - Every request, one-tick event and delayed event carries `Header`.
  - The only ways to get a Header are `parent.Child()` and `Root[C](cmd cardinal.CommandContext[C], source Ref)`. A root therefore needs a client command.
  - An analyzer (same skeleton as `ownercheck`) flags any `Header` literal outside the queue package.
  - Repeats are owner state (a `NextTick` field in the owner's own component), not re-sent requests.
  - At the cap: panic in TestWorld and DST, and log an error plus count a metric in production.
  - If systems with no command must start chains (AI, Init), name that root function and its allowlist explicitly.

### 10. Due-tick handling can be bypassed silently — major
- **Location:** 06 Bounds ("Due < now → treat as asap"), 02/04 (`Due uint64` written by the caller).
- **Problem:** If callers write `Due` themselves, mistakes stay silent. A seconds-for-ticks mix-up or a `now - x` underflow gives a huge Due: the item never fires and only a length cap might notice. A Due in the past is clamped with no signal.
- **Fix:** Callers pass `delay Ticks` (a named uint32 type). The helper sets `Due = w.TickHeight() + delay` and is the only writer of Due. Check a max delay at send: panic in tests, error log plus clamp in production. Count late or clamped items. Drain uses `Due <= now`, never `==` (04, Quantum edge note).

### 11. Target gone: pick one fix that has no hole — major
- **Location:** 06 "The gap" (spawn serial in a component), 01 §5 (I), 05 §E.3.
- **Problem:** EntityIDs are reused at once with no generation (`world_state.go`; `system.go:15-18`). 06's fix puts a serial component on targetable entities. If an entity without it is treated as serial 0, a reused ID made by plain `w.Create` would match a stored serial-0 ref. Cancel-by-source has the same reuse problem for `Source`.
- **Fix:** Use `type Ref struct{ ID cardinal.EntityID; Serial uint64 }` for both Target and Source. Serial comes from a saved counter in the queue plugin's singleton. `Send` panics if the target has no `Spawn` component. The drain drops a request unless the target is alive, has `Spawn`, and `Spawn.Serial == ref.Serial`. Count the drop; do not panic (07: target death is normal gameplay). List "engine-level generation in EntityID" as the rejected alternative, with why.

### 12. Cancellation semantics left open — major
- **Location:** 06 Cancellation; 05 §E.3; 08 Contradiction 6.
- **Problem:** "Cancel is a request applied before the due drain" does not say what happens when the cancel arrives after the drain on the tick the item is due. Cancel by Seq needs a unique uint64 (issue 4). Cancel by source needs a Ref (issue 11).
- **Fix:** Write the drain order in the report:
  1. apply cancels;
  2. freeze the due set (`Due <= now` at entry);
  3. apply in key order;
  4. keep the rest.

  Anything sent during the drain, including the owner sending to itself, waits for the next tick. A cancel that lands after the drain is too late, and the report says so in one line. Add a test for it.

### 13. Restore holes: Init events and new singletons — major
- **Location:** 01 §2 and §4.
- **Problem:**
  - a. `Init()` does not clear system events, and boot runs `init()` then `restore()` (`world.go:190-192`). Events emitted by Init systems are replayed on the first tick after every restart.
  - b. A queue singleton made in Init is wiped by an older snapshot. Then `Single()` returns `ErrSingleNoResult`. 01 suggests lazy create-if-missing. That is compat code for old snapshots, which the project rules reject.
- **Fix:**
  - a. Engine split: clear system events after `restore()`, or panic on `EmitSystemEvent` during Init. Pick one; panicking is the louder choice.
  - b. Drop lazy create. Say "`world purge` after adding a queue type". Make the drain's `Single()` error message say that.

### 14. Delayed events "at tick start" have no engine slot — major
- **Location:** 01 §1 and §12; 06 Delayed events; 08 Contradiction 7.
- **Problem:** Nothing runs before PreUpdate. "First PreUpdate system" depends on plugin registration order. No public schedule API exists to check it (`Schedules()` is internal, 01 §10). No surveyed engine fires at tick start (Doom 3 and Source fire after thinks), so it is Cardinal's own choice.
- **Fix:** Engine split: add a reserved first phase that only the delayed-event drain may use. Mark "tick start" as (I), with no prior art. Do not cite Mass or Doom 3 as support.

### 15. Order check needs engine help, and request latency depends on registration — major
- **Location:** 07 §(b) Startup checks; 01 §10; 02 §11.5.
- **Problem:**
  - a. The reader-after-sender check needs emit and read sets. Cardinal has none (01 §10). A declared list can drift from the code. A TestWorld tap only sees paths a test runs.
  - b. A sender placed before the owner's drain gets same-tick delivery; one placed after gets next tick. Moving a system changes game timing with no error.
- **Fix:**
  - a. One mechanism: `WithEmits[T]()`, `WithReads[T]()` and `WithSends[R]()` system options, checked at `init()`. A TestWorld tap fails when an observed emit, read or send is not declared.
  - b. Say in the API doc that same-tick delivery is incidental. Only "applied by the next drain" is guaranteed. The startup check prints each sender's position relative to the drain.

### 16. Overclaims carried in the notes — major
- **Location:** 05 Verdict, §E.4; 02 §11.6; 06 Top finding 4. 08 Contradictions 5 and 6.
- **Problem:** "No engine has a cap" and "no game engine gives per-request cancel handles" are both false as written.
- **Fix:**
  - Write: "No engine read has a per-request chain-depth field. Per-drain count caps exist: Mass `MaxIterations = 5`, Doom 3 4,096 then a hard error, Netty 100,000 per tick."
  - Write: "Doom 3 and Source delayed queues cancel by scan. Per-handle cancel exists in Unreal `FTimerHandle` (S), GoWorld `CancelTimer` (V) and Netty `Timeout.cancel()` (V)."

### 17. V/S/I marks that are wrong — minor
- **Location:** 03 §9 (Unity 6.6 ECB multi-playback deprecation), 04 (all Mover/GAS internals), 07 ("Go 1.27 has generic methods [24 spec]"), 07 (analyzer "catches all call forms").
- **Problem:**
  - Unity 6.6: the forum returned 403, so this is unverifiable (08 row 27).
  - Mover/GAS: Epic source is gated, so 04 itself says these are (S).
  - Go spec: 01 says the spec was not read, so (S). The generic methods themselves are (V) from WE code.
  - Analyzer: it covers `Set`/`Remove` only, not `Create`/`Destroy`, and `TestUnusedWrite` is red (08 row 16).
- **Fix:** Mark all of these (S), except the analyzer. For the analyzer write "flags `Set`/`Remove`; `Create`/`Destroy` not yet" and note the red test.

### 18. Link and version nits — minor
- **Location:** 03 Mass links; 02 `flecs.h` line; 06 Go links; 06 Pause.
- **Problem:** Mass links use branch `main`, but the repo branch is `master` (404). The flecs define is at line 367, not ~362. Go is cited at `master`, but 1.25.5 was checked. The Quantum `GameTimer` (scaled and paused by `GameTimeScale`) is missing from the pause section (08 row 33).
- **Fix:** Correct the links to `master`, cite `flecs.h:367`, pin Go 1.25.5 links, and add one line on `GameTimer` under pause.

### 19. Map keyed by packed (due, seq) inherits issue 2 — minor
- **Location:** 01 §8 and §12.
- **Problem:** A packed uint64 `(dueTick, seq)` key leaves out Target and Source, so it has the restore problem from issue 2. Packing also splits 64 bits between due and seq. `immutable.Map` Set/Delete is O(n) in the worst case (08 row 11).
- **Fix:** If the report offers a Map, put it under Alternatives. Rejected because the key cannot hold the full sort key; or keep a Slice and sort once per drain.

### 20. Full send path not measured — minor
- **Location:** 01 "Could not find"; 07 §(c).
- **Problem:** Only Slice operations were benchmarked. Each `Send` also pays a singleton lookup (O(number of archetypes), 01 §7) plus Get/Set.
- **Fix:** Do not quote a per-request total. Mark the full cost (I), or benchmark `Send` + `Drain` in a TestWorld before quoting.

### 21. Say why restore divergence matters — minor
- **Location:** 01 §6.
- **Problem:** Readers may think restore order is only theoretical. `RequireDeterministic` compares two fresh runs and misses it (`testworld.go:290-316`).
- **Fix:** One line naming who is hit: DST snapshot round-trips, shard restart from a snapshot, and any client that re-runs server code from a snapshot. Point to the helper in issue 2.

### 22. Stable or unstable sort: pick one — minor
- **Location:** 07 Summary 4 vs 06 and 01. 08 Contradiction 4.
- **Problem:** The notes point two ways. `immutable.Slice` only offers stable sort.
- **Fix:** With the total key from issue 2, both give the same result. Pick `SortStableFunc` on the Slice, which needs no copy out. Mention `slices.SortFunc` on a plain `[]T` only if the drain already copies out.

### 23. Alternatives and gaps belong in the report — minor
- **Location:** 05 (request outcome; modifiers; Overwatch deferred-create trap), 03 §11 (per-entity queues), 06 (wheel/heap storage).
- **Problem:** The brief wants alternatives and an engine vs game split. Project style is one pick per question, not option menus.
- **Fix:** For each question, give one pick, then "considered and rejected" rows with one-line reasons:
  - per-entity queue (+130 µs/tick empty at 500 entities, user measurement);
  - heap or wheel (not needed at 1–100 tick delays);
  - Go-side buffer (issue 8);
  - engine-level EntityID generation (issue 11).

  Put three gaps in the not-found or open list:
  - how a requester learns the outcome (target gone or rejected): one-tick event with the header;
  - modifiers like armor are component leaves the owner reads during its drain;
  - entity creation stays immediate inside the owner; only cross-feature spawn requests take one tick.
