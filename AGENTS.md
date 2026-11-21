# echo_image — agent guide

A Cardinal (Argus **World Engine**) authoritative game backend in Go. Game logic lives in
**shards** (each shard = one Cardinal `World` process). The gameplay shard is `shards/game`.

This file is the always-loaded index: build rules, the rules that hold everywhere, and where each
system's decisions live. **Game design** (the *what*) is [`docs/GDD.md`](docs/GDD.md); this is the *how*.
Update the matching file when we make a new decision.

---

## Build & run

> **⛔ Do NOT run the game or touch the MCP.** Never run `world start` / `world stop` / `world purge`
> / `world reload`, and never call the Cardinal MCP tools (`send_command`, `query`, `introspect`,
> `get_shard_logs`, …). **The user starts, stops, and runs the game manually.** Only use the MCP
> *after* the user explicitly says the game is running. Building/vetting the Go code locally
> (`go build ./...`) is fine; spinning up the cluster is the user's job.

- **From the repo root, with moon:** `moon run client:dev` (serve the client), `client:test`,
  `client:build`, `client:typecheck`, `echo:check` (`scripts/check.sh`), `echo:check-go`,
  `echo:mutate -- <ids>`. `moon tasks` lists them.
- **Plain local build/vet/test:** `go build ./...` / `go vet ./shards/... ./pkg/...` / `go test ./...`.
  Run the tests at 60 Hz too: `go test -tags rate60 ./...`, and in `client/`
  `GOFLAGS=-tags=rate60 bun run test` (see tick-rate independence). `scripts/check.sh` runs every
  check at both rates (gofmt, vet, wasm vet, Go and client tests); `git config core.hooksPath
  .githooks` makes each push run it first (on the working tree, not the pushed commits). CI
  (`.github/workflows/check.yml`) runs its Go half, `scripts/check.sh go`, on amd64 and arm64 for
  every push and pull request. The client half joins once the SDK's MessagePack codec is on npm and
  `client/package.json` pins it in place of the `../../go-ecs` alias.
  The world tests in `shards/game/test` run real ticks through `shard.Register` with
  `cardinal.NewTestWorld`, so a missing registration fails there; they also pin the client-sync,
  prediction, ability, shot, Echo, area, damage and effect rules in `.claude/rules/`.
- **Mutation checks:** `go run ./scripts/mutate` breaks each rule in `scripts/mutate/mutants.json` once,
  in a temporary copy (never the working tree), and fails if a check fails with no mutant, a test misses
  one, or its old text is gone. `scripts/check.sh` runs `go run ./scripts/mutate -anchors`, which checks
  only the old texts. Add a mutant with each new rule; run it when a rule changes.
- **Test files:** a test file mirrors one source file or one feature package (`player_test.go`,
  `projectile_test.go`, … in `shards/game/test`). One harness: shared helpers live in
  `harness_test.go`. Never a topical one-off file or a second harness. Unit and fuzz tests sit beside the code they test.
- **world-engine is pinned** in `go.mod`: that line is the version, so no doc repeats it. To move it:
  `go get github.com/argus-labs/world-engine@<commit or tag>`, in a commit of its own that says why.
  No `vendor/`: the shard's Docker build downloads modules itself.
- `world.toml` declares the shards.
- **No codegen.** Commands, events, components and system events are encoded as MessagePack by
  reflection; a type only needs `Name()` and a registration.
- More test, client-build and toolchain details: `.claude/rules/build-and-test.md`,
  `client-build.md`, `kit-packages.md`.

---

## Layout in one look

- `shards/game/main.go` makes the world; `shard/shard.go` is wiring only; `shard/catalog.go` loads content.
- `shards/game/internal/<feature>/` = one package per feature (systems, commands, events, `tuning.go`,
  `doc.go`), with its components in a `<feature>/component` leaf. Never a layer bucket (`system/`, …).
- `shards/game/content/<feature>/*.json` = embedded data; `shards/game/test/` = the world tests, one harness.
- `shards/game/predictor/` = the wasm predictor; `client/` = the three.js client.
- `pkg/level`, `pkg/level/bake`, `pkg/levelstate`, `pkg/queue` = no game words, built to move to world-engine.
- Full tree, dependency direction and file-layout rules: `.claude/rules/layout.md`.

## Rules that hold everywhere (detail in the file named)

- **Tick-rate independent.** Rates are per second, scaled by `dt`; durations are seconds, converted by
  `timestep.Ticks`; never hardcode `20`. Tests run at 20 and 60 Hz. → `tick-rate.md`
- **Every collision or overlap check is a Box2D query.** No hand-written distance math, no loop over
  every target. → `collision.md`
- **Shared sim code is deterministic:** no Cardinal import, FMA-free, never `math.Hypot`
  (`TestSharedCoreIsDeterministic`). → `movement.md`
- **A change to state another feature owns is an intent to its owner's queue** (`pkg/queue`); only the
  owner writes it (`combat.DamageResolveSystem` is the one writer of `Health`). → `owner-queues.md`, `collision.md`
- **Emitters register before readers, senders before drains; PostUpdate only reads; every receiver sorts
  by a full key** (`TestSendersRunBeforeTheirReceivers`). → `registration-order.md`
- **Content is data:** one loader (`content.Load`), never another JSON loader; a champion, shot type or
  rule is JSON, not code; the GDD's rules are default content. → `content.md`, `interact-rules.md`, `abilities.md`
- **Never `Broadcast` game state;** `netsync` decides who may receive what. → `netsync.md`
- **Abilities never move a Body;** they feed the one mover a forced move, a speed change or a pass.
  → `abilities.md`, `movement.md`

## Index: where each system's decisions live

Files under `.claude/rules/` load by themselves in Claude Code when you touch matching paths. Other
agents: read the file before changing that system.

| System | Read | Also |
|---|---|---|
| Repo layout, conventions, file layout | `.claude/rules/layout.md` | |
| Tick rate | `.claude/rules/tick-rate.md` | `internal/physics/timestep` |
| X meets Y rules (`interact`) | `.claude/rules/interact-rules.md` | |
| Owner queues, delayed events | `.claude/rules/owner-queues.md` | `pkg/queue/doc.go`, `queuetest.CheckOrder`, `docs/research/owner-queues.md` |
| Collision, shots, damage, teams, lag compensation | `.claude/rules/collision.md` | `docs/research/favor-the-shooter.md` |
| Movement and walls | `.claude/rules/movement.md` | |
| System registration order | `.claude/rules/registration-order.md` | `shards/game/shard/shard.go` |
| Content, maps, test content | `.claude/rules/content.md` | `pkg/level/doc.go`, `docs/research/map-loading-strategy.md` |
| Client sync, input, prediction | `.claude/rules/netsync.md` | `docs/research/predictor-on-every-client.md` |
| Abilities and effects | `.claude/rules/abilities.md` | `docs/research/character-foundation.md`, `system-architecture-design.md` |
| Tests, random play, tick cost | `.claude/rules/build-and-test.md` | |
| Client build, toolchain, Unity pairing | `.claude/rules/client-build.md` | |
| `pkg/` packages (world-engine rules) | `.claude/rules/kit-packages.md` | each package's `doc.go` |

---

## Engine cheat-sheet (Cardinal)

- **Systems** are structs with `Run(w *cardinal.World)`, registered in `shard.go` with
  `world.RegisterSystem(&feature.FooSystem{...}, cardinal.WithHook(...))`. Dependencies (content,
  plugins) are struct fields set there. Log with `w.Logger()`.
- **Search rows:** `w.Contains[Row]()` / `w.Exact[Row]()`, where `Row` is a struct whose fields are
  component types. `.Iter()` yields `cardinal.Entity` handles; `.GetByID(id)` fetches one (error if it
  doesn't match). `w.Create[Row]()` spawns an entity with those components, zeroed. On a handle:
  `e.Get[T]()` (a copy), `e.Set(v)`, `e.Has[T]()`, `e.ID()`, `e.Destroy()`; `w.Entity(id)` binds a
  stored id. `Contains` matches any archetype that includes the row's components; `Exact` matches only
  that set.
- **Commands & system events:** `for cmd := range w.Commands[T]()` (`cmd.Payload`; `cmd.Player()` is
  the sender's player ID, false if a shard sent it);
  `w.EmitSystemEvent(v)`; `for e := range w.SystemEvents[T]()`.
- **Intents (`pkg/queue`):** the owner calls `queue.Declare[T](w)` in `RegisterTypes`; any system
  sends with `queue.Send(w, T{…})` (`queue.SendAt` for a later tick); the owner's one system takes them
  with `queue.Drain(w, compare)`, once a tick (Go infers T from `compare`).
- **Registration is explicit.** Every component, command, event and system event must be registered
  before `StartGame` (the owning feature's `RegisterTypes`); an unregistered one panics on first use.
- **Components, commands & system events** implement `Name() string` — unique, stable across runs, and
  `[A-Za-z_][A-Za-z0-9_]*` (no hyphens/dots/spaces).
- **Field rules:** values only — no Go slices, maps, pointers or interfaces in a component, command
  or event (a plain slice shares its backing array, so writes skip `Set`). Use `cardinal.Slice[T]`
  for a variable-length list (`With` writes through, so always `Set` the component after). Component
  leaves use the plugin's own types (`physics2d/component`), so they don't import the plugin.
- **Physics body = a trio of components:** `Transform2D` (pose, authoritative) + `Velocity2D` +
  `PhysicsBody2D` (Box2D body + collider shapes). Use `physics.NewBody2D` for correct Box2D defaults
  (bare `PhysicsBody2D{}` literals are inactive/asleep with zero gravity scale — almost never right).
- **System hooks:** `cardinal.Init` (once), `cardinal.PreUpdate`, `cardinal.Update` (default),
  `cardinal.PostUpdate`. Registration order within a hook = execution order.
- **Commands** are MessagePack on the wire. Route commands by the authenticated `cmd.Player()`, not a
  client-supplied id.

---

## Reference

- `rampage-backend` (`../rampage-backend`) is a mature World Engine game we mine for patterns. It's a
  useful reference but **not a style authority** — we deliberately diverge from it (see the
  package-per-feature vs. its `system/` junk drawer, and deferred intents vs. its typed-event
  fan-out). Compare against general best practice, not just what rampage did.
- **Research findings** (not source of truth; this file and the code win) live in `docs/research/`.
  Map loading: [`docs/research/map-loading-strategy.md`](docs/research/map-loading-strategy.md) (it
  replaces the proposal in [`map-system-findings.md`](docs/research/map-system-findings.md) §6-§7).
  The predictor on Unity, phones and consoles:
  [`docs/research/predictor-on-every-client.md`](docs/research/predictor-on-every-client.md).
  How features affect each other (events and owner queues):
  [`docs/research/event-architecture.md`](docs/research/event-architecture.md), its survey
  [`gameplay-coupling-survey.md`](docs/research/gameplay-coupling-survey.md), the queue API
  [`queue-api.md`](docs/research/queue-api.md), and what echo built:
  [`owner-queues.md`](docs/research/owner-queues.md).
- **Backlog of ideas and decisions for later:** [`docs/backlog.md`](docs/backlog.md). Read it when
  asked what to build next, and add to it when asked to save an idea.
- World Engine source (incl. the `physics2d` plugin) lives at `../world-engine`, for reading. The build
  uses the pinned module (see Build & run): a local `replace` would leave its code out of `SimVersion`.
