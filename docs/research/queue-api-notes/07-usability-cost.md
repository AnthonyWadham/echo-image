# 07 — API usability, enforcement and cost

Status: DONE 2026-11-19 (researcher 07)

Marks: (V) read primary source this session, (S) secondary/unread primary, (I) my inference.

## Summary (verdict first)
1. Ownership can't come from Go's type system here: snapshotted component fields must be exported [2]. Write ownership needs a checker. In Cardinal it is small: `Get` returns a copy, so only `Set/Remove/Create/Destroy` write. A ~40-line go/analysis prototype catches all call forms [27]. (V)
2. Queue type can be hidden with `internal/` (compiler-enforced); others call the owner's typed helper. Header built by `parent.Child()` (Source/GAS style: source and cause are required args, not hand-typed fields). (I, from [23][25])
3. Biggest cost is not the sort, it is one-by-one `immutable.Slice.Append`: ~250 us, 832 KB, 1,061 allocs per 1,000 requests on WE main; one batched Append ~20 us. Make batching the default path. (V) [18]
4. Use `slices.SortFunc` with a unique seq tie-breaker (81 us/1,000, 0 allocs; ~1.75x faster than sort.Slice). Read header as a field, never via a method on a type param (18x slower). (V) [18]
5. Tests: SS14 save-load-save + "run N ticks, state unchanged" [9]; Bevy CI asserts zero ambiguities [8]. Cardinal needs declared emit/read sets for the reader-after-sender check. `unusedwrite` does not catch "forgot Set" [27]. (V)
6. Brief correction: on WE main (#1177) `Insert(Len,x)` == `Append`; both copy the ≤32-element tail. The "Insert appends in place" note is pre-#1177. (V) [1]

## Sources
- [1] world-engine pkg/immutable DESIGN.md, README.md, slice.go at main f7d4a52b (PR #1177 "add immutable.Map and make Slice and Map safe to copy"). Local: world-engine/pkg/immutable/
- [2] world-engine pkg/cardinal entity.go, world.go, system.go at main f7d4a52b. Local: world-engine/pkg/cardinal/

- [3] RobustToolbox AccessAttribute.cs, https://github.com/space-wizards/RobustToolbox/blob/master/Robust.Shared/Analyzers/AccessAttribute.cs (engine source)
- [4] RobustToolbox AccessAnalyzer.cs, https://github.com/space-wizards/RobustToolbox/blob/master/Robust.Analyzers/AccessAnalyzer.cs (engine source)
- [5] RobustToolbox ByRefEventAnalyzer.cs, https://github.com/space-wizards/RobustToolbox/blob/master/Robust.Analyzers/ByRefEventAnalyzer.cs (engine source)
- [6] SS14 codebase conventions, https://docs.spacestation14.com/en/general-development/codebase-info/conventions.html (official docs)
- [7] Bevy ScheduleBuildSettings docs (bevy 0.20.1), https://docs.rs/bevy/latest/bevy/ecs/schedule/struct.ScheduleBuildSettings.html (official docs)
- [8] Bevy tests/ecs/ambiguity_detection.rs, https://github.com/bevyengine/bevy/blob/main/tests/ecs/ambiguity_detection.rs (engine source)
- [9] SS14 SaveLoadSaveTest.cs, https://github.com/space-wizards/space-station-14/blob/master/Content.IntegrationTests/Tests/SaveLoadSaveTest.cs (engine source)

- [10] go/analysis package docs, https://pkg.go.dev/golang.org/x/tools/go/analysis (official docs)
- [11] golangci-lint Module Plugin System, https://golangci-lint.run/docs/plugins/module-plugins/ (official docs); register.go https://github.com/golangci/plugin-module-register/blob/main/register/register.go (source)
- [12] exhaustruct README, https://github.com/GaijinEntertainment/go-exhaustruct (tool docs)
- [13] world-engine .golangci.yaml (local, main f7d4a52b)
- [14] Eli Bendersky, "Faster sorting with Go generics", 2022-04-02, https://eli.thegreenplace.net/2022/faster-sorting-with-go-generics/ (Go team member's blog)
- [15] Vicent Marti, "Generics can make your Go code slower", PlanetScale, 2022-03-30, https://planetscale.com/blog/generics-can-make-your-go-code-slower
- [16] sort package docs, https://pkg.go.dev/sort (official docs)
- [17] Go GC guide, https://go.dev/doc/gc-guide (official docs)

- [18] My micro-benchmarks, this session. Code + raw output: [`benchmarks.md`](benchmarks.md) (bench_test.go, batch_test.go, bench-out.txt, bench-batch-out.txt). Go 1.27.4, i9-11900K under WSL2, `-benchtime=200ms -count 3`. immutable package copied from WE main f7d4a52b. Request = 48-byte pointer-free struct {Header{Due,Source uint64; Cause,Seq uint32; Depth,Flags uint8}; Target uint64; Amount int64}.

- [19] Bevy 0.17 release notes, https://bevy.org/news/bevy-0-17/ (official)
- [20] Bevy 0.16 release notes (error handling), https://bevy.org/news/bevy-0-16/ (official)
- [21] Bevy `Messages` docs 0.20.1, https://docs.rs/bevy/0.20.1/bevy/ecs/message/struct.Messages.html (official)
- [22] Unity Entities 1.3 manual, ECB playback, https://docs.unity3d.com/Packages/com.unity.entities@1.3/manual/systems-entity-command-buffer-playback.html (official)
- [23] GASDocumentation (tranek), Gameplay Effect Context section, https://github.com/tranek/GASDocumentation#concepts-ge-context (widely used community doc; Epic API page returned empty)
- [24] Go 1.27.4 go/types api.go `Info.Instances` doc + my probe test [`ownercheck.md`](ownercheck.md) (type-instance probe)

- [25] Source SDK 2013 eventqueue.h and cbase.cpp (CEventQueue), https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/server/eventqueue.h ; https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/server/cbase.cpp (engine source)
- [26] x/tools unusedwrite docs, https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/unusedwrite (official docs)
- [27] My analyzer prototype, this session: [`ownercheck.md`](ownercheck.md) (ownercheck.go, ownercheck_test.go, testdata/). x/tools v0.52.0, Go 1.27.4.

## Cardinal facts that shape the API (from [1][2])
- (V) `Entity.Get[T]() T` returns a copy; `Entity.Set[T](c T)` writes it back. Panics if absent/unregistered. [2] entity.go: "Get returns a copy of a component."
- (V) Startup check already exists: `RegisterComponent` panics on slice/map/pointer/interface fields: "a slice, map, pointer, or interface, which would share memory with the stored component". Lists must be fixed arrays, `immutable.Slice` or `immutable.Map`. [2] world.go
- (V) System events: `RegisterSystemEvent[T]`, `EmitSystemEvent[T](e)`, `SystemEvents[T]() iter.Seq[T]`. Unregistered emit panics "in release builds too: an unregistered emit dropped silently would leave every later SystemEvents reader empty." [2] world.go — precedent for fail-loud registration.
- (V) **Brief correction**: on WE main (#1177) `immutable.Slice` is a persistent 32-way trie (Bagwell/Clojure vector), not a flat slice. `Insert(Len, x)` just calls `Append`. `Append` "copies the last chunk of up to 32 elements, so appending in a loop is linear" [1] slice.go. So "Insert(Len) in place, Append copies O(n)" is the OLD (pre-#1177) behavior. Check which WE version the game pins. (I)
- (V) Copies and Get are free (pointer copy). "Reads and copies allocate nothing ... an edit allocates once per node it copies: once for a small value ... 3 for a Set in a 5,000-element Slice." [1] DESIGN.md
- (V) Queue pitfall stated in [1] README: "Long queues need a head index. Dropping the front ... about 2 µs at 256 elements ... O(n) per pop: 27 µs at 4,096." Recommends head index or "A Map keyed by a sequence number ... removes its smallest key in O(log n)."
- (V) Sorting: "Sorting is always stable. There is no unstable sort, so the result never depends on the sort algorithm a Go release uses." [1] README. Insert/Delete in middle and sort = O(n) rebuild.
- (V) Common-mistake table in [1] README: "editing c.Items and never calling Set" → must `entity.Set(c)`; `entity.Get[Inventory]().Items.Append(x)` does not compile (pointer receiver on non-addressable value) — compile-time guard by design.
- (V) [1] DESIGN: no in-place write possible "Go cannot tell: it has no reference counts ... and no ownership rules"; transient/builder sessions (Clojure transients, go-immutable-radix Txn) considered, "none built".

## (a) Usability
### Lessons from engines
- (V) One name for two mechanisms confused users. Bevy 0.17: the old Event trait covered observed and buffered events; "they are also fundamentally very different things functionally." Split into `Message` (buffered; `MessageWriter`/`MessageReader`) and `Event` (observed; `world.trigger()`, `On<E>`). [19] (I) Our design has requests (queue to owner), one-tick events and delayed events. Give each its own Go type and verb (`Request`, `Emit`, `EmitAt`) — never one `Send` for all three.
- (V) Silent loss is the top buffered-event complaint. Bevy: "If messages are not handled by the end of the frame after they are updated, they will be dropped silently"; buffers "grow indefinitely if update is never called"; "If no ordering is applied between writing and reading systems, there is a risk of a race condition." [21] (I) Our rule 2 (late request waits for next tick) avoids loss for requests. One-tick events still lose late readers → rule 5's order test is the guard.
- (V) Panicking shorthand APIs. Bevy 0.16: "shorthand" panicking APIs "require less typing than unwrapping results"; errors from systems/observers/commands now go to `GLOBAL_ERROR_HANDLER`, which "defaults to panicking", advice: develop with panic, then log in production. [20] (I) Cardinal already panics on misuse (unregistered event, absent entity) [2]. A request whose target entity died before drain should be a counted drop, not a panic — it is normal gameplay, not a bug.
- (V) Deterministic order needs keys independent of scheduling. Unity ECB: "the order of the commands is non-deterministic because they depend on job scheduling"; fix is "Record an int sort key passed as the first argument to each ECB method"; "If the recorded sort keys are independent from the scheduling, then the sorting makes the playback order deterministic." Suggested key: `ChunkIndexInQuery`. [22] (I) Unity makes the key a required *positional argument* — impossible to forget. In our serial engine, a key derived from (due tick, sender system index, per-tick seq) is automatic; the user should not pick it.
- (V) Source/cause carried by a context object built by a helper, not by hand. GAS: `GameplayEffectContext` "holds information about a GameplayEffectSpec's instigator and TargetData"; callers write `Source->MakeEffectContext()`; projects subclass it to carry extra data. [23] (I) Mirror: `ctx := hdr.Child()` copies source/cause from the request being handled and adds 1 to depth. Users never type `Depth:`.

### API shape: what makes misuse hard (Go-specific)
- (V) Go generics can name the target by type: Go 1.27 has generic methods [24 spec], and `go/types` records each instantiation, explicit `e.Set[Health](...)` or inferred `e.Set(h)`, in `Info.Instances` keyed by the method identifier (probe output: `Set TypeArg=p.Health pkg=p` for all three call forms). [24] So a typed `Queue[R]` / `Request[R]` costs nothing in clarity and is checkable by tools.
- (V) Cardinal snapshots cannot hold unexported state: `RegisterComponent` refuses "a non-empty struct with no exported fields, which a snapshot cannot hold" and "an embedded field of an unexported type ... which a snapshot stores but a restore cannot write back". [2] (I) So "ownership via unexported fields" (classic Go encapsulation) does NOT work for components: any snapshotted field must be exported, hence writable by anyone who can name the type. Package visibility can hide *helper functions* and *constructors*, not component fields. Write ownership must come from a checker (like SS14 RS2008), not the type system.
- (I) Good split by Go visibility:
  - Owner package exports the component type (readable by all via `Get`).
  - Owner package exports typed request helpers: `combat.RequestDamage(w, target, amount, hdr)`. The queue component type itself can live in an `internal/` sub-package of the owner, so other features cannot even name it — they can only use the helper. `internal/` is enforced by the Go compiler, so no linter is needed for the queue.
  - Header type with all fields exported (snapshot) but built only via `hdr.Child()` / `queue.Root(src)`; exhaustruct `//exhaustruct:enforce` on the header catches hand-built literals [12].
- (I) Struct literal vs builder: Go struct literals silently zero missing fields (`Depth` 0 = "first trigger", `Due` 0 = "asap" — both are valid values, so a forgotten field is not detectable at runtime). That argues for a constructor that takes the parent header, or exhaustruct. Functional-options builders hide the same zero defaults and allocate closures; not worth it here.
- (I) Batched sends: since one-by-one `immutable.Slice.Append` is ~11x the batched cost [18], the helper should accept variadic requests, or the system should collect in a plain local `[]R` and flush once. A plain local slice that is always empty when the system returns is snapshot-safe.
- (I) Header access: keep header as a concrete field of a generic wrapper (`Queued[P]{Hdr Header; P P}`), not a method on P; field read is ~18x faster inside a generic drain [18] and needs no per-type boilerplate.
## (b) Enforcement
### Go tooling for custom checks
- (V) go/analysis [10]: an `Analyzer` has Name, Doc, Run, Requires, ResultType, FactTypes. "Facts are analogous to type export data in a build system" — lets one package's analysis use results from imported packages (e.g. "component C is owned by package P" exported as a fact from P, read when checking package Q). Runners: unitchecker (go vet), multichecker, singlechecker.
- (V) golangci-lint module plugins [11]: `.custom-gcl.yml` lists plugin modules (proxy or local `path:`), `golangci-lint custom` builds `./custom-gcl`. Plugin implements `LinterPlugin { BuildAnalyzers() ([]*analysis.Analyzer, error); GetLoadMode() string }` with load mode `"syntax"` or `"typesinfo"`.
- (V) exhaustruct [12]: "finds structures with uninitialized fields". Explicit mode: "only checks structs that are explicitly marked ... via //exhaustruct:enforce directive". Field-level `//exhaustruct:optional`. (I) Mark the request header type `//exhaustruct:enforce` → any literal missing `Depth:`/`Source:` fails lint. Note: `Depth: 0` written explicitly still passes, so it forces a *decision*, not a correct value.
- (V) In world-engine's own lint config exhaustruct is OFF: `#- exhaustruct_v5 # [highly recommend to enable] checks if all structure fields are initialized` [13]. depguard, forbidigo, gocritic, govet are on.
### SS14 / RobustToolbox: owner-only writes as a compile error (C# Roslyn)
- (V) `[Access(typeof(MySystem))]` on a component. Three user groups: Self, Friend, Other. Defaults: Self and Friend = ReadWriteExecute, Other = Read. [3] `OtherDefaultPermissions = AccessPermissions.Read`.
- (V) Violation = compile error: "Error RS2008: Tried to perform write access to member 'Counter' in type 'MyComponent', despite read access." [3]
- (V) How it detects a write [4]: registers on FieldReference/PropertyReference/MethodReference/Invocation; walks to the parent op. `IAssignmentOperation` where target is the member → Write; invocation → Execute unless method has `[Pure]` → Read; else Read. Severity `DiagnosticSeverity.Error`.
- (V) Convention [6]: "Components should specify their access restrictions whenever possible, usually only allowing the entity systems that wrap them to modify their data." And: no setter logic in properties; "create a setter method in your entity system, and apply the [Friend(...)] attribute to the component so only that system can modify it."
- (V) Event shape enforced too [5][6]: "Events should always be structs, not classes, and should always be raised by ref." Analyzer errors: "Tried to raise a by-ref event '{0}' by value", "Tried to subscribe to a by-ref event '{0}' by value", "Tried to raise a value event '{0}' by-ref". Lesson (I): when an API has two shapes that both compile, the team adds a checker for the wrong one.
- (V) Public API rule [6]: owner system exposes methods `Foo(Entity<FooComponent?> ent, args...)`, first line `Resolve`. Other systems call the owner's method, not the component. (I) Closest analogue in our design: owner package exports a typed `Request(w, ...)` helper; the queue component type itself stays writable only by owner.
- (I) Go mapping: Go has no attributes, but a `//cardinal:owner` doc directive or the package boundary itself can play the role of `[Access]`. Go analyzers can read comments/directives via `ast.File.Comments`.

### Prototype: owner-only writes as a Go analyzer [27] (V = I built and ran it)
- (V) ~40 lines. Loop over `pass.TypesInfo.Instances`; keep ids whose `Uses[id]` is a `*types.Func` named `Set`/`Remove` with receiver `ecs.Entity`; take `inst.TypeArgs.At(0)` as the component; report if `comp.Obj().Pkg() != pass.Pkg`.
- (V) analysistest passes: flags all three forms in package `ai` — inferred `e.Set(h)`, explicit `e.Set[combat.Health](h)`, `e.Remove[combat.Health]()` — and does not flag owner package `combat`. Message: "Entity.Set writes Health, owned by package combat; send a request to its owner".
- (I) Why it is this small in Cardinal: `Get` returns a copy [2], so the only ways to change stored state are `Entity.Set`, `Entity.Remove`, `World.Create`/archetype creation, and `Destroy`. One choke point, unlike SS14 which must inspect every field assignment [4]. Extend the method list to those, plus an allowlist (tests, a `//cardinal:friend pkg` directive on the component type read from the AST, like SS14's `Friend`).
- (I) Owner = declaring package by default. Facts [10] are only needed if ownership is declared somewhere other than the type's own package.
- (V) Gap found: x/tools `unusedwrite` ("reports instances of writes to struct fields and arrays that are never read" [26]) did NOT flag `h := e.Get[combat.Health](); h.HP--` with no `Set` in my test [27]. The "forgot Set" mistake (listed in [1] README) needs its own check, e.g. "a value from `Get[T]` whose fields are written, with no later `Set` of that variable in the function" (I).
- (I) How to run it: standalone `singlechecker`/`multichecker` binary via `go run ./tools/...` or `go vet -vettool=`, built with the pinned Go. A golangci module plugin [11] makes `golangci-lint custom` compile golangci-lint from source — the cost WE removed in f2a50582 ("stop compiling golangci-lint on every lint run"). The installed prebuilt golangci-lint binaries here say "built with go1.26.0" / "go1.25.5" (I ran `version`); type-checking Go 1.27 generic methods needs a 1.27-built tool. (V for versions; I for consequence, matches user memory note on IDE phantom errors.)
- (I) Other cheap checks with the same skeleton: (1) a request literal built outside the queue package must come from `hdr.Child()` (or exhaustruct `enforce`); (2) `EmitSystemEvent[T]` / `SystemEvents[T]` call sites → per-package emit/read sets, which feed the order test below.

### Bevy: order-ambiguity detection
- (V) `ScheduleBuildSettings.ambiguity_detection`: "Determines whether the presence of ambiguities (systems with conflicting access but indeterminate order) is only logged ... Defaults to LogLevel::Ignore." [7] So off by default.
- (V) Bevy CI test asserts zero ambiguities for DefaultPlugins [8]: `assert_eq!(main_app_ambiguities.total(), 0, "Main app has unexpected ambiguities ...")`. It sets `auto_insert_apply_deferred: false` because auto-inserted sync points "can cause two ambiguous systems to become accidentally ordered" — "We don't just want no ambiguities - we also don't want changes ... from 'creating' new ambiguities". (V) [8]
- (I) Cardinal runs systems serially in registration order, so there is no "ambiguous" order. The analogous check for our design is rule 5: "every reader of event E is registered after every sender of E", plus "every owner drain of queue Q runs at its declared point; writers after it get next-tick". That needs systems to *declare* what they emit/read/enqueue (Bevy gets it from typed system params; Cardinal systems take `*World`, so access is not declared). See design note below.

### Startup checks (I, drawn from Cardinal precedent [2] and Bevy [8])
- Cardinal already fails at registration for unstorable component fields and for unregistered events "in release builds too" [2]. Same style fits:
  - Every queue type registered with exactly one owner system and one drain phase/position; a second `RegisterQueue[R]` or a drain from a different system panics at startup.
  - Order check (rule 5): systems declare `Emits[E]()` / `Reads[E]()` at registration (or the analyzer extracts them). Startup verifies each reader's registration index > every emitter's. Bevy needs typed system params to know access; Cardinal systems take `*World`, so this must be declared or extracted.
  - Depth cap and max-due-horizon as world config, checked once.
- Runtime counters instead of panics for gameplay-normal cases: requests dropped at depth cap, requests whose target died. Expose as metrics (Cardinal has metrics.go).

### Source engine precedent for the header and delays [25]
- (V) `CEventQueue::AddEvent(target, action, Value, float fireDelay, CBaseEntity *pActivator, CBaseEntity *pCaller, int outputID = 0)` — delay, activator (≈ root source) and caller (≈ immediate cause) are *required positional args*, so they cannot be forgotten. [25]
- (V) Insert keeps the list sorted by fire time; the loop breaks on `m_flFireTime > newEvent->m_flFireTime`, so equal times stay in insertion order (FIFO). O(n) linked-list insert. [25]
- (V) The queue has `Save`/`Restore` and `CancelEvents(CBaseEntity *pCaller)` (removes all pending events from that caller). [25] (I) Cancellation by source is the usability feature delayed requests need (interrupted wind-up). In our design: owner drops due requests whose source is dead/stunned, or the request carries a token the owner checks.
- (V) `ServiceEvents` fires while `m_flFireTime <= gpGlobals->curtime`. [25]

### Save-load-save tests
- (V) SS14 [9]: "Tests that a grid's yaml does not change when saved consecutively." Save → load → save, `Assert.That(two, Is.EqualTo(one))`. Also `LoadSaveTicksSaveBagel`: "Loads the default map, runs it for 5 ticks, then assert that it did not change." And a test that loads the same map "at slightly different times" and checks the saves are equal.
- (I) For us: save mid-tick-sequence with queued requests and delayed events pending → restore → run N ticks → compare state digest with the unrestored run. This directly tests rule 7 (sorted apply) and rule 6 (delayed events live in components, so they survive). Cardinal snapshots save components only, so any request/event state kept outside components (e.g. system events, Go globals) is a restore bug; the test catches it.
## (c) Cost in Go
### Measured this session [18] (V = I ran it; numbers are per tick-sized batch)
| What (1,000 requests) | time | bytes | allocs |
| --- | --- | --- | --- |
| plain `[]Req`, reused (`q = q[:0]`), append one by one + drain | 2.7 us | 0 | 0 |
| `immutable.Slice` Append one by one + drain | ~250 us | 832 KB | 1,061 |
| `immutable.Slice` Insert(Len) one by one | ~250 us | 832 KB | 1,061 |
| `immutable.Slice` one `Append(src...)` | ~20 us | 59 KB | 93 |
| `immutable.Slice` 10 senders x `Append(100...)` | ~22 us | 65 KB | 101 |
| sort.Slice | 142 us | 120 B | 3 |
| sort.SliceStable | 383 us | 120 B | 3 |
| slices.SortFunc | 81 us | 0 | 0 |
| slices.SortStableFunc | 171 us | 0 | 0 |
| immutable.Slice SortStableFunc (incl. SliceOf build) | 131 us | 111 KB | 71 |
| sum `Due` via constraint method `r.Hdr()` in generic func | 6.8 us | – | – |
| sum `Due` via field, concrete or generic `Queued[P]{H Header; P P}` | 0.38 us | – | – |
| copy component value: immutable.Slice (1,000 elems) / [16]Req / [64]Req | 0.9 ns / 10.5 ns / 48 ns | – | – |

At 100 requests: plain 0.25 us; immutable one-by-one 24 us/80 KB/105 allocs; one Append 1.8 us. At 10: plain 23 ns; one-by-one 0.9 us; one Append 0.17 us.

Takeaways:
- (V) On WE main, one-by-one `Append` onto `immutable.Slice` is ~100x a reused plain slice and allocates ~830 bytes per request (each Append copies the 32-slot tail: 32 x 48 B = 1.5 KB worst case). This is the dominant cost of a queue component, larger than the sort.
- (V) Batching appends (one `Append(xs...)` per sender per tick) cuts it ~11x. (I) So the API should make the batched path the easy path: e.g. `Send(w, reqs ...R)` or a sender-local buffer flushed once, not a per-request `Get→Append→Set` round trip.
- (V) `slices.SortFunc` ~1.75x faster than `sort.Slice` and 0 allocs; stable sorts ~2x the unstable ones. With a unique `Seq` tie-breaker the unstable sort is deterministic, so stability buys nothing. (I)
- (V) Reading the header through a method on a type parameter is ~18x slower than a field read (dictionary call, no inlining) — matches [15]. Generic wrapper `Queued[P]{H Header; P P}` costs nothing extra. (I) → put the header in a generic wrapper struct owned by the queue package, not a `Header()` method each request type implements.
- (I) The user's measured "+116 us for 1,000 req/tick" fits between my one-by-one (250 us) and batched (22 us) numbers; the gap suggests their test either batched partly or ran on the pre-#1177 Slice. Worth re-measuring on the pinned WE version.
- (I) Fixed arrays copy on every Get/Set (48 ns for 64 requests); immutable.Slice copy is a pointer copy. For small bounded queues a fixed array is alloc-free; for unbounded queues the Slice wins on Get/Set but loses on append.

### Sorting
- (V) Go docs on every sort.Slice/Sort/Stable: "Note: in many situations, the newer slices.SortFunc function is more ergonomic and runs faster." [16] `sort.Stable` makes "O(n*log(n)*log(n)) calls to data.Swap". `sort.Slice` "is not guaranteed to be stable".
- (S) sort.Slice builds a reflect swapper (`reflectlite.Swapper(x)`) because it takes `any`; slices.SortFunc is generic and needs none. (from search snippet of go/src/sort/slice.go; not read in full)
- (V) Bendersky [14], bubble sort of 1000 strings: interface 9.60 ms/op, generic 7.43 ms/op ("over 20% faster"), func-param 8.58 ms/op. Reason: interface `Less` is dynamic dispatch, not inlined, bounds checks kept; generic stenciled code calls `runtime.cmpstring` directly.
- (I) Determinism: an unstable sort (sort.Slice, slices.SortFunc) gives same output for same input on the same Go version, but equal keys can land in any order and the algorithm may change between Go releases. So either (1) the sort key is total (ties impossible: add a unique sequence number), or (2) use the stable sort. Cardinal's immutable.Slice only offers stable sort [1] for exactly this reason.

### Generics overhead
- (V) Go uses "GCShape stenciling with Dictionaries"; "All pointers to objects belong to the same GCShape" [15]. Slowdowns are from calling *methods* through a type param that is a pointer or interface: Escape benchmark monomorphized 5.06 us, Iface 6.85, GenericWithPtr 7.18, GenericWithExactIface 9.68, GenericWithSuperIface 17.6 [15].
- (V) Recommendations [15]: "Use generics for data structures, replacing interface{} storage"; "Don't ... Pass interfaces to generic functions"; "Don't use generics to de-virtualize or inline method calls."
- (I) For us: `Queue[R]` with R a plain value struct is the "data structure" case (cheap). Cost appears only if the drain calls methods on R through a constraint (e.g. `R interface{ Key() uint64 }`) — then each call goes through the dictionary. Prefer a concrete header struct field (`r.Hdr.Due`) read directly, not a method.

### GC and allocation
- (V) "The GC's CPU costs are modeled as a fixed cost per cycle, and a marginal cost that scales proportionally with the size of the live heap." "the allocation rate of a Go program is a major factor in GC frequency" [17].
- (V) "Pointer-free values are segregated from other values ... it may be advantageous to eliminate pointers from data structures that do not strictly need them" [17]. `go build -gcflags=-m=3` shows escape reasons.
- (I) A request struct with only ints/IDs (no string, no slice) is pointer-free → cheaper to scan. A `Cause string` field or `[]byte` payload makes every queued request a pointer-bearing object. Prefer numeric cause/source IDs.
## Could not find
- No primary measurement of Unity ECB / Bevy Commands per-command cost comparable to ours.
- No Go ECS (Ark, Donburi, Cardinal) doc or issue about write-ownership linting; no published Go analyzer for ECS component ownership. SS14 (C#) is the only shipped one found.
- No engine post-mortem specifically about a request/depth header (depth caps) API; the closest are Source's activator/caller args and GAS's effect context.
- Epic's official FGameplayEffectContext API page rendered empty; used GASDocumentation instead (community).
- `sort.Slice` reflect-swapper allocation was not read in source; my benchmark shows 3 allocs/120 B per call [18].
- Did not verify whether `unusedwrite` is enabled in go vet/gopls by default.
- Did not measure Cardinal `Entity.Get/Set` of a singleton per request (user already measured totals); my numbers are for the Slice alone.
