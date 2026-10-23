# echo_image — Claude Code guide

A Cardinal (Argus **World Engine**) authoritative game backend in Go. Game logic lives in
**shards** (each shard = one Cardinal `World` process). The gameplay shard is `shards/game`.

This file records the conventions and architectural decisions we've committed to, so systems get
built the same way as the game grows. Read it before adding a system; update it when we make a new
decision.

**Game design** — what we're actually building and why — lives in [`docs/GDD.md`](docs/GDD.md).
Read it for gameplay context (the Echo mechanic, win conditions, etc.); this file is the *how*, that
one is the *what*.

---

## Build & run

> **⛔ Do NOT run the game or touch the MCP.** Never run `world start` / `world stop` / `world purge`
> / `world reload`, and never call the Cardinal MCP tools (`send_command`, `query`, `introspect`,
> `get_shard_logs`, …). **The user starts, stops, and runs the game manually.** Only use the MCP
> *after* the user explicitly says the game is running. Building/vetting the Go code locally
> (`go build ./...`) is fine; spinning up the cluster is the user's job.

- **Plain local build/vet/test:** `go build ./...` / `go vet ./shards/... ./pkg/...` / `go test ./...`.
  `shards/game/main_test.go` runs real ticks through `main.go`'s `register` with `cardinal.NewTestWorld`,
  so a missing registration fails there; it also pins the client-sync, prediction, ability and shot
  rules below.
- **world-engine is the local checkout** (`replace => ../world-engine` in `go.mod`), currently a
  branch where codegen is reverted. The shard's Docker build only mounts this project, so it can't
  see `../world-engine`: `world start` needs a fresh `go mod vendor` first (and a re-vendor whenever
  world-engine changes), or a published world-engine release in place of the `replace`.
- **Toolchain:** Go 1.27.1 (`GOTOOLCHAIN=auto` to auto-fetch if the local go is older).
- `world.toml` declares the shards.
- **No codegen.** Commands, events, components and system events are encoded as MessagePack by
  reflection; a type only needs `Name()` and a registration.
- **Client pairing.** The Unity client (`/mnt/z/Echo`) still uses `dev.world.sdk` v0.6.0 and its
  generated protobuf C#, which doesn't match the MessagePack payloads. It needs updating before it
  can talk to this shard again.

---

## Repo layout — **package by feature**

```
shards/game/
  main.go                       registration wiring ONLY
  content/   package content     embedded JSON config (go:embed), namespaced content/<feature>/*.json
  internal/
    physics/  package physics    plugin wiring (physics.go) + collision category bits
                                  (categories.go — kind, never team) + NewBody2D helper
    team/     package team        Team allegiance component + SameTeam — a component-only leaf
                                  (imports only cardinal; safe to read from any feature)
    combat/   package combat      damage: anticamp_system.go (an intent emitter) → damage_system.go
                                  (the one writer of Health), tuning.go
      component/  package component   Health, DamageIntent — combat's leaf; any damage source emits
                                      DamageIntent without importing combat's systems
    player/   package player      player Body: spawn_system.go (the one Body constructor),
                                  input_system.go, movement_system.go, input_command.go,
                                  sync_system.go, tuning.go
      component/  package component   Player, InputQueue + InputFrame, MoveSpeed, BodyClaimed — player's
                                      leaf
    echo/     package echo        the Echo replay: spawn_system.go, replay_system.go, sync_system.go,
                                  tuning.go, doc.go
      component/  package component   Echo, PathHistory — echo's leaf
    ability/  package ability     whether a Body can cast: equip_system.go (default kit on claim) →
                                  cast_system.go (the one writer of cooldowns), sync_system.go,
                                  tuning.go, doc.go
      component/  package component   Abilities (kit + cooldowns), AbilityCast — ability's leaf
    projectile/ package projectile  shots: launch_system.go (the one shot constructor) →
                                  flight_system.go (move + first hit), sync_system.go, tuning.go, doc.go
      component/  package component   Projectile — projectile's leaf
    netsync/  package netsync     who may receive what: Viewers, Items (audience + seenBy). No events
                                  of its own; each feature sends its own
```

Current dependency direction (must stay acyclic): a feature's behavior package depends on **physics**,
the **team** primitive, **netsync**, and any **`<feature>/component`** leaves it reads — never another
feature's behavior package (`player → {physics, team, netsync, player/component}`, `ability →
{physics, team, netsync, player/component, ability/component}`, `projectile → {physics, team, netsync,
ability/component, combat/component, projectile/component}`, `combat → {physics, data,
combat/component, echo/component, player/component}`). `netsync` imports only `team` and
`player/component`, so any feature can sync through it. Every `component` leaf and `team` import only
cardinal (and the physics2d component types), so they're dependency *sinks* — a feature can read
another's components without ever closing a cycle. `content` is a leaf too (imported only by
`main.go`); feature packages never import it — they name their JSON file by path string in
`JSONFile()`.

### Conventions when adding systems

1. **One package per feature, named after the feature.** New enemy code → `internal/enemy`
   (`package enemy`); player code → `internal/player`. **Never** create a top-level *layer* bucket —
   no shared `system/`, `component/`, `event/`, or `command/` dir. That's the exact anti-pattern we're
   avoiding: rampage has a 32-file `system/` junk drawer *and* a top-level `component/`, so one concept
   like "game phase" ends up smeared across `component/game_phase.go`, `event/game_phase_events.go`,
   and two files in `system/` — four files in three dirs, with no `internal/gamephase/` package at
   all. Every component, system, event, and command lives in its owning feature package.
2. **Colocate a feature's components, systems, and events in that one package.** Split into
   sub-packages only when a package genuinely gets large (~10+ files), and split **by sub-feature**
   (`combat/status`), **never by layer** (no `components/` `systems/` `events/` trio upfront).
3. **Descriptive package names, no stutter.** `physics.Register` not `physics.RegisterPhysics`;
   `physics.NewBody2D` not `physics.NewPhysicsBody2D`. This keeps `main.go` import-alias-free. The
   rule targets verb/function stutter; an eponymous type in a single-concept package (`team.Team`,
   like stdlib `list.List`) is fine.
4. **A component/event lives in the package that owns it.** Promote to a *cross-cutting* `shared`/`core`
   package only when ownership is genuinely ambiguous — not preemptively. (This is about ambiguous
   *ownership*; a feature's OWN `component/` leaf is a different, deliberately-early split — see File
   layout rule 2.)
5. **`main.go` is wiring only** — construct the world, register plugins/systems in order, no logic.
6. **Each feature registers its own types** in `register.go`: `RegisterTypes(w)` registers every
   component (including its `component/` leaf's), command and system event the feature owns.
   `main.go` calls each feature's `RegisterTypes` before registering systems.

### File layout within a package

The five rules above govern *directories*; these govern the *files inside* a package, and cover two
things that must both hold as a feature grows: you can tell a file's role at a glance, and one feature
can read another's data without risking an import cycle.

1. **Role goes in the filename suffix.** A behavior file is `<concept>_<role>.go` —
   `movement_system.go`, `input_command.go`, `spawn_system.go`, `damage_intent_event.go`. `ls` alone
   tells you what each file is; you never open `move.go` and `movement.go` to find out one's a command
   and the other's a system. Config definitions are `tuning.go` (`<concept>_tuning.go` when a feature
   has several); the package doc is `doc.go` or the namesake file. A search Row / query projection
   lives in the system file that uses it, not its own file.
2. **Components live in a `component/` leaf sub-package, so any feature can read them without importing
   systems.** ECS data is a dependency *sink*: `player/component` and `combat/component` (both `package
   component`) import only the engine. That's what lets a future `enemy` read `component.Player` or
   attach `component.Health` with **zero** risk of an import cycle — the failure that bites hard the
   first time two features reference each other's state. The leaf holds component *types* only — rows,
   systems, client events, commands, and tunings stay in the behavior package. One exception: a system
   event another feature emits or reads lives in the owner's leaf too — an intent sent into it
   (`combat/component.DamageIntent`) or a fact others react to (`player/component.BodyClaimed`,
   `ability/component.AbilityCast`) — so no feature imports another's systems.
   - **This is NOT the "split by layer" anti-pattern.** Separating *data* (a genuine leaf) from
     *behavior* is a real dependency boundary; splitting *behavior* into `systems/`/`events/` dirs is
     the cosmetic layering we still reject (directory rule 2).
   - **A component-only feature skips the sub-package** — the feature package *is* the leaf
     (`team.Team`; the bits in `physics/categories.go`). Components graduate into `component/` only
     once the feature also owns systems a cross-feature importer would otherwise drag in.
   - **Cross-importing two features' components** takes a one-word import alias (both are `package
     component`): `import playercomp "…/player/component"`. That alias lives in the importing *system*
     file — `main.go` imports feature packages, never their component leaves, so it stays alias-free.
   - **Yes, the leaf goes in up front, before a second importer exists — deliberately.** This is the one
     place we promote data *preemptively*: in ECS a component being read cross-feature is near-certain,
     and retrofitting the leaf later means touching every importer. The leaf is one file and the
     boundary is real, so we pay it early. (Contrast the `shared`/`core` promotion in directory rule 4,
     which stays on-demand because cross-cutting *ownership* is genuinely uncertain until it happens.)
3. **Split a concept's system into its own file only when two systems' registration ORDER is
   load-bearing** — then the file boundary documents the pipeline. `anticamp_system.go` (emit
   `DamageIntent`) and `damage_system.go` (drain → the one place `Health` mutates) are separate
   *because* `AntiCampSystem` must register before `DamageResolveSystem`; likewise projectile's
   `launch_system.go` → `flight_system.go`. Don't split just because a file feels big.
4. **A cross-cutting vocabulary many features import gets its own file, even when small**
   (`physics/categories.go` — the collision category bits).
5. **The package's front-door doc comment has one home:** a dedicated `doc.go` (`echo`, `ability`),
   or the namesake file for a component-only package (`team.go`). Never leave it on whichever file
   sorts first.

---

## Architectural decisions

### Tick-rate independence (fixed-timestep sim)
The simulation must be **tick-rate agnostic**: changing `TickRate` changes only how finely we sample
time — never how fast the game plays or how hard anything hits. A build at 60 Hz must be the *same
game* as one at 20 Hz, just smoother. Every system honors this:
- **Rates are per-second, scaled by `dt` at the point of use.** `dt = 1 / physics.ServerTickRateHz`.
  Never bake a per-*tick* constant. A damage-over-time of 25/s is `25 * dt` this tick, not a hardcoded
  `1.25`. Movement already complies: `Velocity2D` is world-units-per-*second* and Box2D integrates at
  `dt`, so units/second is constant across tick rates — a faster tick rate must never move anything
  faster.
- **Durations & delays are configured in seconds and converted to ticks** (`ticks = round(seconds *
  TickRate)`) — never stored as a raw tick count. The Echo delay, cooldowns, and any ring-buffer
  sizing all derive from seconds so they hold their wall-clock meaning when the rate changes.
- **Discrete vs continuous.** A discrete event (a projectile hit, a spawn) applies a fixed amount and is
  already rate-independent. A continuous effect (DoT, regen, movement) must scale by `dt`.
- **Integer HP vs sub-unit-per-tick.** `dt`-scaled continuous damage is fractional; if `Health.HP`
  stays an int, a continuous source must accumulate the fraction and apply whole points as it crosses
  them — never truncate per tick, or a DoT rounds toward zero at high tick rates. (Switching HP/damage
  to a float or fixed-point removes the accumulator; decide per the combat model.)
- **One source of truth for the rate.** Read `physics.ServerTickRateHz` (or a `physics` `dt` helper);
  never hardcode `20`. `main.go` already feeds it to both the Cardinal `WorldOptions.TickRate` and the
  physics2d step, so a single edit re-tunes the whole sim.

### Collision handling: deferred intents, first-hit shots, per-feature contact reactions
- **Damage is deferred.** A damage source emits `DamageIntent` (in the `combat/component` leaf, so a
  source never imports combat's systems). `combat.DamageResolveSystem` (Update, registered *after*
  every emitter) drains them and is the **only** place `Health` changes. Sources today:
  `projectile.FlightSystem` (shot hits) and `combat.AntiCampSystem`.
- **Why deferral:** because all mutation is deferred to the resolver and damage sums commutatively, the
  order hits arrive in — or which source emitted first — is irrelevant, so there's **no per-type event
  fan-out**. (Rampage re-emitted a typed event per collision kind to control ordering; deferral makes
  that unnecessary.)
- **Shots don't use physics contacts.** A projectile has no Box2D body. `projectile.FlightSystem` moves
  each shot every tick and resolves the **first** thing on its path: walls with the physics2d
  `CircleSweep` (category `Environment`, which must mask `CategoryProjectile`), Bodies and Echoes with
  its own swept-circle test. Box2D's sweep returns only the closest hit and can't skip allies (team
  isn't a collision bit), so the team-aware part lives in gameplay code. Kind still comes from category
  bits: `CategoryBody` blocks, `CategoryEcho` takes damage.
  - First hit wins, which is what makes body-blocking exact, and a swept path can't tunnel at any
    speed. A Body wins a tie with an Echo, so a Body standing on its own Echo shields it.
  - Cost is O(S·T) per tick (shots × Bodies and Echoes). If counts grow, gather candidates with
    `OverlapAABB` first.
- **Contact reactions** (pickups, triggers; none exist yet): `physics2d` emits `ContactBegin/End` +
  `TriggerBegin/End` system events each tick (on PreUpdate). Physics resolves the *physical* contact
  (blocking, sliding) on its own; these events carry category bits, not meaning. **A feature that
  reacts to contacts declares its own receiver** over that stream and does a **single pass** for *its*
  kinds only, matching by **category bits** (`physics.Match` — integer compare, *never* a reverse ECS
  type lookup) and **emitting intents**. It is **not** a global router.
- **Know the cost of that model:** the engine keeps ONE shared event queue per type, so each reactor
  re-scans the *whole* contact stream — N reactors × C contacts/tick = **O(N·C)** `Match` calls. Fine
  while reactors are few. Because no layer sees the whole stream, a contact two reactors both match is
  handled by both (safe *only* because effects are deferred commutative intents), and a contact **no**
  reactor claims is silently dropped. To confirm masks during bring-up, register a **Debug-level
  contact logger in `physics`** only while debugging — never a catch-all inside a feature.
- **Why the reaction lives in the feature, not physics:** deciding a contact's *meaning* needs gameplay
  state — `Team` (friend/foe), owner, tuning. `physics` is a reusable, gameplay-blind plugin and must
  never import `team`/`combat`. Physics knows *that* shapes touched; the feature knows what that touch
  *does*.
- **Corollaries:** add a new collision *effect* as **new intents + resolver logic**, not an inline
  mutation; add a new *kind* of contact reaction as a **new feature reactor**.

### Collision filtering & teams
- **Category bits describe what a shape IS (its kind)** — `Body`, `Echo`, `Projectile`,
  `Environment` — never whose side it's on.
- **Filter at the source with mask bits** so pairs you'll never act on don't even generate events —
  cheaper than classify-and-discard. Two shapes interact only if each one's category is in the
  other's mask.
- **Team allegiance is NOT a category bit.** "Enemy" is relational (team A's enemies are teams B and
  C, who aren't allies of each other) and teams are assigned dynamically at match start — you can't
  bake that into a fixed bit. Friend/foe is decided **in gameplay code** from `Team`/owner components
  at collision time — via the shared `team.SameTeam(search, a, b)` helper. Note `SameTeam` is a
  friendly-fire *suppression* predicate (unknown allegiance ⇒ not allied); enemy *selection*
  (damage-all-enemies) is **not** just `!SameTeam` — it needs its own predicate so teamless neutral
  geometry isn't wrongly treated as a target. This is *not* the "reverse type lookup" the classification
  rule forbids — that
  rule is about KIND (use bits); team is genuine game state (read the component).
- **Owner vs team.** Anti-camp self-damage (Echo overlapping its OWN Body) is an **owner** match;
  friendly fire (a shot passing through everything on its own team) is a **team** match. Echoes and
  projectiles carry their owner (`Echo.Owner`, `Projectile.Source`) and the owner's `Team`. A
  "same-team = don't collide" bit/`groupIndex` scheme is therefore wrong for damage — it would
  suppress the owner-match self-collision.
- `groupIndex` *may* later express purely-physical same-team pass-through (movement), never damage rules.

### System registration ordering (`main.go`)
- The physics plugin registers on `Init` (rebuild) and `PreUpdate` (step), so its events are ready
  before `Update` game logic runs.
- **System events are same-tick and cleared at tick end**, so the **emitter must be registered
  before the receiver**: `player.InputSystem` (emits `BodyClaimed`) before `echo.SpawnSystem` and
  `ability.EquipSystem`, `ability.CastSystem` (emits `AbilityCast`) before `projectile.LaunchSystem`,
  and every `DamageIntent` emitter (`projectile.FlightSystem`, `combat.AntiCampSystem`) before
  `combat.DamageResolveSystem`.
- **`player.InputSystem` then `player.MovementSystem` run in PreUpdate, registered before the physics
  plugin**, so a frame moves the Body in the same tick its `InputAck` is sent. Client prediction
  depends on this (`TestFrameMovesBodySameTickAsAck`).
- **Abilities and shots run after the Echo replay:** `ability.EquipSystem` → `ability.CastSystem` →
  `projectile.LaunchSystem` → `projectile.FlightSystem`, after `echo.ReplaySystem`, so shots test
  against this tick's Echo poses. A Body can cast the tick it is claimed, and a shot launched this tick
  moves this tick, its path starting at the shooter.
- **Sync systems run last** (`player`, `echo`, `ability`, `projectile`), so each sends the tick's final
  state.

### Game data / definitions (data plugin)
- **Use the engine's `pkg/plugin/data` plugin** for all data-driven config — it gives a typed
  catalog, `data.Get[T](plugin)` lookup, load-time `Validate()`, and snapshot content-hash reconcile so
  an in-flight game resumes on the exact config it was running. **Never hand-roll a JSON loader.**
- **A definition is a Go type implementing `Name()` + `JSONFile()`** (the `data.Definition`
  contract); the JSON file unmarshals straight into it. Optional interfaces: `Validate()`
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
  `data.Register[T](p)` for **every** kind, then `world.RegisterPlugin(p)` — all before
  `StartGame`. A system that reads config has a `Data *data.Plugin` field, set in `main.go`.
- **Read config once at system entry** (`tuning := data.Get[T](s.Data)` at the top of `Run`), not
  per-entity inside hot loops.
- **Prefer typed per-feature config structs** over a single stringly-typed global settings bag
  (rampage's `game_settings.json` uses `Parameter string / Value json.RawMessage` — untyped, avoid).

### Client sync (`netsync`)
- **Events are the only way to push data to clients.** Cardinal has no client queries (removed in
  world-engine #915) and no automatic component sync.
- **Each feature sends its own sync event** from its own `SyncSystem`, registered last:
  `player.BodySync`, `echo.EchoSync`, `ability.AbilitySync`, `projectile.ProjectileSync`. Each goes to
  every player with `w.SendTo` every tick and is complete — an entity missing from it is gone or
  hidden — so joining or reconnecting needs nothing else. Clients interpolate each stream on
  `Tick * TickSeconds`. A new synced feature adds its own event and `SyncSystem`; `netsync` doesn't
  change.
- **`netsync` decides who may receive what, never what is sent.** `netsync.Viewers(w)` lists the
  players with a Body. `netsync.Items[S]` collects a feature's per-entity state with an **audience**
  and filters it once per viewer team (`Lists`). It imports only `team` and `player/component`.
- **Audience:** `Everyone` (every team `seenBy` allows) or `Allies` (only the entity's own team, e.g.
  ability cooldowns — enemies never receive them).
- **Never `Broadcast` game state.** Any client can subscribe to any event name, so a broadcast is
  public. Anything the server sends, a cheater can read.
- **Hidden info is decided in one place, `netsync.seenBy(entity, team)`** (invisibility, line of
  sight). Lists are built per team, since allies share vision. That includes what reveals a hidden
  entity indirectly, e.g. an invisible Body's Echo replaying its path 5s later.
- **Cost.** Cardinal writes one stream message per event, so each synced feature adds one message per
  player per tick. Serialization repeats once per player; bandwidth equals a broadcast.
- **Player IDs are only trustworthy with Argus auth.** DEV auth trusts the `X-Player-ID` header.
- **Clients predict their own Body.** Each tick a client sends `Input{Session, Frames}` with its recent
  unacked frames — each frame is move, aim and buttons — and replays the frames newer than
  `BodySync.You.InputAck` (when `InputSession` is its own).
- **The server queues frames and applies exactly one per Body per tick**, in Seq order
  (`player.InputSystem`, `player/component.InputQueue`). Duplicates and applied frames are dropped, at
  most `input_buffer_seconds` of frames are kept, and a Body with no input for `input_timeout_seconds`
  stops. One step per tick keeps the Body, and the Echo replaying it, smooth when frames arrive late
  or bunched. A new `Session` (a restarted client) starts the stream over.
- **Systems act on input through `InputQueue.Applied(tick)`**, so each frame acts once: movement sets
  velocity from it, abilities cast from it. Buttons ride in the frame (like a Source or Overwatch
  user command), so a shot leaves from where the client predicted its Body when it fired.
- **Cooldowns are server ticks** (`Slot.ReadyTick`), never a client count, so a client can't speed
  them up. Shots aren't predicted on the client yet; they appear when the server confirms them.

### Abilities and their effects
- **`ability` only decides whether a cast happens.** It holds each Body's kit (equipped from
  `default_kit` when the Body is claimed), checks the button, cooldown and aim, starts the cooldown,
  and emits `AbilityCast{Caster, Ability, Aim}`. It never does the effect.
- **An effect lives in the feature it changes and reacts to `AbilityCast` by ability ID,** with its
  own numbers under that ID: `launchers.bolt` in `content/projectile/tuning.json` is the shot `bolt`
  launches. A new shot ability is one JSON entry in each file. A new kind of effect (dash, shield) is a
  new reactor in its own feature. Neither changes `ability`.
- **`InputFrame.Buttons` bit N casts kit slot N**, so a new ability never changes the input type.
  `TestLaunchersAreAimedAbilities` checks the content IDs line up.

---

## Engine cheat-sheet (Cardinal)

- **Systems** are structs with `Run(w *cardinal.World)`, registered in `main.go` with
  `world.RegisterSystem(&feature.FooSystem{...}, cardinal.WithHook(...))`. Dependencies (the data
  plugin) are struct fields set there. Log with `w.Logger()`.
- **Search rows:** `w.Contains[Row]()` / `w.Exact[Row]()`, where `Row` is a struct whose fields are
  component types. `.Iter()` yields `cardinal.Entity` handles; `.GetByID(id)` fetches one (error if it
  doesn't match). `w.Create[Row]()` spawns an entity with those components, zeroed. On a handle:
  `e.Get[T]()` (a copy), `e.Set(v)`, `e.Has[T]()`, `e.ID()`, `e.Destroy()`; `w.Entity(id)` binds a
  stored id. `Contains` matches any archetype that includes the row's components; `Exact` matches only
  that set.
- **Commands & system events:** `for cmd := range w.Commands[T]()` (`cmd.Payload`; `cmd.Player()` is
  the sender's player ID, false if a shard sent it);
  `w.EmitSystemEvent(v)`; `for e := range w.SystemEvents[T]()`.
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
- World Engine source (incl. the `physics2d` plugin) lives at `../world-engine`; this project builds against the published release, not that checkout.
