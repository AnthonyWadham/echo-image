---
description: Deep multi-agent review of the current uncommitted changes, tuned to this project's standards (ECS/tick-model correctness, snapshot safety, scaling, architecture, clean code).
argument-hint: "[optional: PR#, branch, or path — defaults to the working-tree diff]"
---

Do a thorough code review of the **current uncommitted working-tree changes** for this Go + Cardinal (Argus **World Engine**) authoritative ECS game server. If an argument is given, review that target instead: $ARGUMENTS

Invoke the **`code-review` skill at `high` effort** (its find → adversarially-verify multi-agent methodology). Pass it the block below **verbatim** as the review focus, so every finder/verifier angle applies this project's standards. Rank findings correctness-first.

---
high — Review the current changes for correctness, game-architecture fitness, and scalability in a Go + Cardinal (Argus World Engine) authoritative ECS server. **Read `CLAUDE.md` first — it is the source of truth for this project's conventions.** Lenses, in priority order:

1. **ECS & TICK-MODEL CORRECTNESS (the firewall for this project — this is what silently breaks the sim).**
   - **System registration order.** System events are same-tick and cleared at tick end, so an emitter MUST be registered before its receiver in `main.go`. Flag a receiver registered before (or without) its emitter, and any code assuming an event survives into a later tick.
   - **Hooks.** `Init` (once), `PreUpdate` (physics step + data reconcile run here), `Update` (game logic), `PostUpdate`. Flag logic in the wrong hook — e.g. consuming contact events before the physics PreUpdate step produced them, or spawning in a hook that races the physics rebuild.
   - **Single-writer mutation.** Game state changes go through the deferred-intent → one-resolver pattern (collision emits `DamageIntent`; only `DamageResolveSystem` writes `Health`). Flag inline state mutation in a classifier/handler that should emit an intent, and any component written by more than one system.
   - **Order-independence.** Because we deliberately do NOT batch by type, contact/intent handling must not depend on the order contacts or intents arrive within a tick. Flag any logic that only works if X is processed before Y.
   - **Iteration & lookup safety.** Entity `Create`/`Destroy` during a `Search.Iter()` over the same set; `Contains` vs `Exact` misuse; unchecked `GetByID` (the target may have despawned this tick).

2. **SNAPSHOT / STATE AUTHORITY.** Cardinal snapshots and restores world state every N ticks. ALL mutable authoritative state must live in ECS components (stable `Name()`, registered) or a plugin's own snapshot-resident state — NEVER in package-level `var` globals or maps, which silently reset or diverge on restore. The data-plugin catalog and physics config are legitimately global (immutable config); mutable *runtime* state is not. Flag: game state held in globals/singletons outside ECS; a `cardinal.Slice` derived (`With`, etc.) without a `Set` after; a `PhysicsBody2D` built as a bare struct literal instead of `physics.NewBody2D` (bare literals are inactive/asleep and break on reconcile).

3. **SCALING & COMPLEXITY (the server pays per tick × per player — this is a scalability-first project).** Name the Big-O of every per-tick system loop, search, and physics query, and the entity count where it stops being acceptable; call out anything O(n²) or worse. Flag: reverse ECS type lookups per contact (must classify on Box2D **category bits** — integer compare); missing **mask-bit filtering at the source** (pairs you will never act on should never generate events, not be classified-and-discarded); per-tick heap allocations in hot loops; `data.Get`/config lookups repeated per-entity instead of once at system entry; unbounded entity growth (e.g. projectiles with no lifetime/cleanup system).

4. **ARCHITECTURE / ALTITUDE.** Is each change at the right depth, or a bandaid special-cased onto shared infra? Build-vs-buy: does Cardinal or the `physics2d` / `data` plugins already provide this — the physics step, spatial queries (`OverlapAABB` / `Raycast` / `CircleSweep`), contact/trigger events, config catalog + hot reload? Flag reinventing a plugin feature. Is new code in the feature package that owns it, or bolted onto a grab-bag? Flag single-source-of-truth violations and authoritative inputs sourced from the wrong layer. New collision effects must be added as **new intents + resolver logic**, not inline mutations (preserves order-independence). Note where the approach diverges from how shipped authoritative-server ECS games solve it.

5. **DUCT TAPE — fixes it today, breaks or needs a rewrite tomorrow.** For each change ask: is it still right with 10× the content (maps, kinds, abilities, players), a second mode, or the next planned step (see `docs/research/`)? Flag:
   - **Special-casing today's only case:** hardcoded IDs, names, counts or positions, `if kind == "x"` branches, or one map/ability/team assumed to be the only one, where data or a general rule fits.
   - **Fallback chains and silent defaults:** try A else B, an unknown ID that loads a default, skip-and-continue on bad data. Want one root-cause path that fails loud at load.
   - **Symptom patches:** clamps, retries, sleeps, bumped tolerances or re-syncs that hide a wrong mechanism instead of fixing it.
   - **Feature coupling:** importing another feature's behavior package, needing another feature's components to work, or a test listing other features' names, so an unrelated change breaks this one.
   - **Two sources of truth for one fact:** server and client computing the same thing separately, duplicated constants, copied files. These drift.
   - **Costs that grow with content:** fine at today's counts, but they scale with maps, kinds or players (O(P²), snapshot bloat, per-tick rebuilds).
   - **Unguarded invariants:** a rule that holds only because current content happens to satisfy it, with no validation or test.
   For each: what breaks, at which future change; the root-cause fix; how shipped games handle it. Do NOT flag a deliberate "for now" decision recorded in `CLAUDE.md`/GDD unless the code makes the later step hard. Do NOT ask for speculative abstractions or plugin points: simple and explicit beats "extensible". Flag only what will need rewriting.

6. **CORRECTNESS.** Inverted/wrong conditions, off-by-one on boundaries, unhandled errors (`GetByID`, data lookups), sign/component errors in vector math, collision-filter bugs (asymmetric category/mask that can never collide, or `CategoryAll` where a specific mask was intended), resource underflow (HP below zero unhandled), removed guards with no replacement. **Determinism:** server-only game logic does NOT need it, so don't flag plain `float` math or map iteration there. Code the client will also run to predict (the movement/mover function, and anything building map collision shapes) MUST be deterministic. Flag: map-iteration order or unsorted data feeding results; mover collision planes not sorted by a stable geometric key (not ShapeID); a `*` result feeding a `+`/`-` without `float64(...)` rounding or `box2d.MulAdd`-style helpers (Go may fuse it into an FMA on arm64/GOAMD64=v3 but never on wasm); and a Cardinal import in that shared code.

7. **PACKAGE STRUCTURE & PLACEMENT (this is a DIFF review — see `CLAUDE.md`).** Organization must follow the package-by-feature rules. Above all, **flag every newly-added file placed in the wrong package**, and pre-existing smells when the change touches that area:
   - **One package per feature, named after the feature** (`internal/enemy` → `package enemy`). Flag any new shared `system/` / `util/` / `helpers/` grab-bag — that junk drawer is the exact anti-pattern being avoided.
   - **Colocate a feature's components, systems, and events in that one package.** Split into sub-packages only when a package genuinely gets large (~10+ files), and by **sub-feature** (`combat/status`), **never by layer** (no `components/` `systems/` `events/` trio).
   - **Data definitions.** The `Definition` type lives in the feature package that owns it (`projectile.ProjectileTuning`), never a central `types/` grab-bag; JSON lives under `content/<feature>/*.json`; feature packages must not import `content`. Flag a central types bucket, or a definition missing its `data.Register` line in `main.go` (`data.Get` on an unregistered kind panics).
   - **No stutter** in package API names (`physics.Register`, `physics.NewBody2D` — not `RegisterPhysics`/`NewPhysicsBody2D`).
   - **Acyclic deps** (current: `player → {physics, team}`, `combat → {physics, team, data}`; `team` imports only cardinal). Flag a new import cycle or a reversed direction (e.g. `physics` importing `combat`).
   - **`main.go` is wiring only** — construct the world, register plugins/systems in order, no logic.

8. **GO / CARDINAL GOTCHAS.** Package-level mutable globals (snapshot risk + the tick loop is single-threaded — no goroutines mutating world state); every component, command and system event needs a line in its feature's `RegisterTypes` (an unregistered one panics on first use); per-entity/per-contact `Info` logging in the tick loop (`Debug` for bring-up only); **build hygiene**: follow `CLAUDE.md`'s "Build & run" notes on the world-engine dependency. Today it's a deliberate local `replace => ../world-engine` plus a `vendor/` that `world start` needs, so don't flag their existence. Do flag a change that edits `vendor/` by hand, adds another local `replace`, or changes world-engine code without re-vendoring (Docker shard builds see only the vendored copy); panic discipline — a loud panic at registration/boot is fine, but runtime tick code must not panic on bad client/command input.

9. **CLEANLINESS / MAINTAINABILITY.** New code that re-implements an existing helper (name it), dead code, unused/misleading parameters, magic numbers that should move to a data-driven tuning struct (the `ProjectileTuning` pattern), duplicated idioms that want one shared home.

Report each finding with `file:line`, a concrete failure scenario (specific inputs/state → wrong result / crash / cost / divergent restore), and a concrete fix. ECS/tick-model + snapshot + correctness outrank cleanup when the output must be trimmed.
---

When the review returns, present the surviving findings to me most-severe first, and **ask which to fix — do not apply fixes unless I say so.**
