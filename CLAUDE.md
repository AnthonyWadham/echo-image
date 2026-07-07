# echo_image — Claude Code guide

A Cardinal (Argus **World Engine**) authoritative game backend in Go. Game logic lives in
**shards** (each shard = one Cardinal `World` process). The gameplay shard is `shards/game`.

This file records the conventions and architectural decisions we've committed to, so systems get
built the same way as the game grows. Read it before adding a system; update it when we make a new
decision.

---

## Build & run

- **Build from the repo root:** `go build -mod=mod ./...`
- **⚠️ Do NOT use a `vendor/` directory.** The `physics2d` plugin compiles Box2D via cgo using
  **relative `#include` paths** (`../../third_party/box2d/...`) that don't survive `go mod vendor`
  — a vendored build fails with `box2d/box2d.h: No such file or directory`. Because a bare
  `go build` defaults to `-mod=vendor` when `vendor/` exists, always build with **`-mod=mod`**
  (or delete `vendor/`). Keep `vendor/` out of the repo.
- **cgo is required** (physics2d): `CGO_ENABLED=1` and a C compiler (gcc) on PATH.
- **Toolchain:** Go 1.26 (`GOTOOLCHAIN=auto` to auto-fetch if the local go is older).
- `world.toml` declares the shards; the `world-engine` dependency is a local `replace` to
  `../world-engine`.

---

## Repo layout — **package by feature**

```
shards/game/
  main.go                     registration wiring ONLY
  content/    package content   embedded JSON config (go:embed), namespaced content/<feature>/*.json
  internal/
    physics/  package physics  physics2d plugin wiring, collision categories, body helpers
    combat/   package combat    Health, collision routing (DamageIntent), damage resolution,
                                CombatTuning (data.Definition)
    demo/     package demo       throwaway spawn scene (delete when real spawners land)
```

Current dependency direction (must stay acyclic): **demo → combat → physics**. `content` is a
leaf (imported only by `main.go`); feature packages never import it — they name their JSON file by
path string in `JSONFile()`.

### Conventions when adding systems

1. **One package per feature, named after the feature.** New enemy code → `internal/enemy`
   (`package enemy`); player code → `internal/player`. **Never** create a shared `system/` grab-bag
   — that junk drawer is the exact anti-pattern we're avoiding (see rampage's 32-file `system/`).
2. **Colocate a feature's components, systems, and events in that one package.** Split into
   sub-packages only when a package genuinely gets large (~10+ files), and split **by sub-feature**
   (`combat/status`), **never by layer** (no `components/` `systems/` `events/` trio upfront).
3. **Descriptive package names, no stutter.** `physics.Register` not `physics.RegisterPhysics`;
   `physics.NewBody2D` not `physics.NewPhysicsBody2D`. This keeps `main.go` import-alias-free.
4. **A component/event lives in the package that owns it.** Promote to a `shared`/`core` package
   only when ownership is genuinely ambiguous — not preemptively.
5. **`main.go` is wiring only** — construct the world, register plugins/systems in order, no logic.

---

## Architectural decisions

### Collision handling: deferred intents, single classification pass
- `physics2d` emits `ContactBegin/End` + `TriggerBegin/End` system events each tick (on PreUpdate).
- **One** system (`combat.CollisionSystem`, Update) does a **single pass**, classifies each contact
  by **Box2D category bits** (integer compare — *never* a reverse ECS type lookup), and **emits
  intents** (`combat.DamageIntent`). It never mutates game state.
- `combat.DamageResolveSystem` (Update, registered *after*) drains intents and is the **only** place
  `Health` changes.
- **Why:** because all mutation is deferred to one resolver and damage sums commutatively, the order
  contacts arrive in (and whether solid contacts and sensor triggers interleave) is irrelevant. So
  we process everything in one interleaved pass with **no per-type event fan-out**. (Rampage
  re-emitted a typed event per collision kind to control ordering; deferral makes that unnecessary.)
- **Corollary:** add new collision effects as **new intents + resolver logic**, not inline
  mutations, to preserve this order-independence.

### Collision filtering
- Category bits are the collision vocabulary (`physics.CategoryPlayer/Enemy/Projectile/...`).
- **Filter at the source with mask bits** so pairs you'll never act on don't even generate events —
  cheaper than classify-and-discard. Two shapes interact only if each one's category is in the
  other's mask.

### System registration ordering (`main.go`)
- The physics plugin registers on `Init` (rebuild) and `PreUpdate` (step), so its events are ready
  before `Update` game logic runs.
- **System events are same-tick and cleared at tick end**, so the **emitter must be registered
  before the receiver**: `combat.CollisionSystem` (emit) before `combat.DamageResolveSystem` (drain).

### Game data / definitions (data plugin)
- **Use the engine's `pkg/plugin/data` plugin** for all data-driven config — it gives a typed
  catalog, `data.Get[T]()` lookup, load-time `Validate()`, and snapshot content-hash reconcile so
  an in-flight game resumes on the exact config it was running. **Never hand-roll a JSON loader.**
- **A definition is a Go type implementing `Name()` + `JSONFile()`** (the `data.Definition`
  contract). Optional interfaces: `SingleObject()` (one JSON object, not an array), `Validate()`
  (load-time invariants — a returned error **panics the boot**, fail loud), `Resolve()` (second-
  stage load of designer-bundled assets like tilemaps).
- **The definition TYPE lives in the feature package that owns it** (e.g. `combat.CombatTuning`),
  **never a central `types/` grab-bag.** Rampage centralizes all definitions in `pkg/datadef/types`
  and 40 feature files import it — that's the layered bucket we reject. Definitions reference each
  other **by string ID resolved through the catalog**, so feature-owned types create no import
  cycles.
- **JSON content lives under `content/<feature>/<file>.json`**, one `embed.FS` (`content.FS`) —
  the plugin takes a single FS and `go:embed` can't reach across directories. `JSONFile()` returns
  the path within that FS (e.g. `"combat/tuning.json"`).
- **Wiring:** in `main.go`, `data.NewPlugin(data.Config{EmbeddedFS: content.FS})`, then
  `data.Register[T](p)` for **every** kind, then `cardinal.RegisterPlugin` — all before
  `StartGame`. Read anywhere afterward with `data.Get[T]()`.
- **Read config once at system entry** (`tuning := data.Get[T]()` at the top of the system), not
  per-entity inside hot loops.
- **Prefer typed per-feature config structs** over a single stringly-typed global settings bag
  (rampage's `game_settings.json` uses `Parameter string / Value json.RawMessage` — untyped, avoid).

---

## Engine cheat-sheet (Cardinal)

- **Search rows:** `cardinal.Contains[Row]` / `cardinal.Exact[Row]`, where `Row` is a struct of
  `cardinal.Ref[Component]` fields. `.Create() (EntityID, Row)` spawns an entity with those
  components; `.GetByID(eid)` fetches by id; `.Iter()` ranges matches; `.Destroy(eid)` removes.
  `Contains` matches any archetype that includes the row's components; `Exact` matches only that set.
- **Components & system events** implement `Name() string` — unique, stable across runs, and
  `[A-Za-z_][A-Za-z0-9_]*` (no hyphens/dots/spaces).
- **System state structs** embed `cardinal.BaseSystemState` and declare their dependencies as
  fields: `cardinal.Ref`, search rows, `WithCommand[T]`, `WithEvent[T]`,
  `WithSystemEventEmitter[T]`, `WithSystemEventReceiver[T]`. Components/commands/events auto-register
  from these fields — no manual registration call.
- **Physics body = a trio of components:** `Transform2D` (pose, authoritative) + `Velocity2D` +
  `PhysicsBody2D` (Box2D body + collider shapes). Use `physics.NewBody2D` for correct Box2D defaults
  (bare `PhysicsBody2D{}` literals are inactive/asleep with zero gravity scale — almost never right).
- **System hooks:** `cardinal.Init` (once), `cardinal.PreUpdate`, `cardinal.Update` (default),
  `cardinal.PostUpdate`. Registration order within a hook = execution order.

---

## Reference

- `rampage-backend` (`../rampage-backend`) is a mature World Engine game we mine for patterns. It's a
  useful reference but **not a style authority** — we deliberately diverge from it (see the
  package-per-feature vs. its `system/` junk drawer, and deferred intents vs. its typed-event
  fan-out). Compare against general best practice, not just what rampage did.
- World Engine source (incl. the `physics2d` plugin) lives at `../world-engine`.
