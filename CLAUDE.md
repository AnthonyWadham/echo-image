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
  Run the tests at 60 Hz too: `go test -tags rate60 ./...`, and in `client/`
  `GOFLAGS=-tags=rate60 bun run test` (see tick-rate independence). `scripts/check.sh` runs every
  check at both rates (gofmt, vet, wasm vet, Go and client tests); `git config core.hooksPath
  .githooks` makes each push run it first (on the working tree, not the pushed commits). CI
  (`.github/workflows/check.yml`) runs its Go half, `scripts/check.sh go`, on amd64 and arm64 for
  every push and pull request. The client half joins once the SDK's MessagePack codec is on npm and
  `client/package.json` pins it in place of the `../../go-ecs` alias.
  The world tests in `shards/game` run real ticks through `main.go`'s `register` with
  `cardinal.NewTestWorld`, so a missing registration fails there; they also pin the client-sync,
  prediction, ability, shot, Echo, area, damage and effect rules below.
- **Mutation checks:** `go run ./scripts/mutate` breaks each rule in `scripts/mutate/mutants.json` once,
  in a temporary copy (never the working tree), and fails if a test misses one or its old text is gone.
  Add a mutant with each new rule; run it when a rule changes.
- **Random play:** `TestRandomPlayIsPredicted` plays 2,000 random but honest ticks from a new seed
  each run, and checks prediction every tick. A failure names its seed; `-seed` reruns it.
  `FuzzFlyAtViewMatchesFly` (`go test -fuzz`) fuzzes where a shot meets a target its client drew.
- **Tick cost:** `BenchmarkTick` times a 3v3 tick (`go test -run '^$' -bench Tick ./shards/game`, with
  `-tags rate60` for 60 Hz). `TestTickAllocations` fails when that tick allocates past its ceiling for
  the rate. Allocation counts don't depend on the machine; raise a ceiling only for a known reason.
- **Test files:** a test file mirrors one source file or one feature package (`player_test.go`,
  `projectile_test.go`, … in `shards/game`). One harness: shared helpers live in `main_test.go`. Never
  a topical one-off file or a second harness. Unit and fuzz tests sit beside the code they test.
- **The client needs Go too.** `bun run dev` and `bun run build` (in `client/`) first build
  `predictor.wasm` and copy that toolchain's `wasm_exec.js` (`client/scripts/predictor.mjs`, both
  gitignored). Vitest runs the same script as its global setup, so its tests never use a stale build.
  It stamps the wasm with the tree's sim version (see client sync), so after a Go or content change
  that reaches the shard, restart `bun run dev` too.
- **world-engine is pinned** in `go.mod` to `v0.22.0-beta`, the commit of PR #1109 (codegen
  reverted). To move it: `go get github.com/argus-labs/world-engine@<commit or tag>`. No
  `vendor/`: the shard's Docker build downloads modules itself.
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
  catalog.go                    every content file, loaded at boot, and the IDs across files checked
  content/   package content     embedded JSON config (go:embed), namespaced content/<feature>/*.json,
                                  and Load, the one strict loader
  internal/
    interact/ package interact    the rule vocabulary: Kind (the Box2D category bits), Tags, Relation,
                                  Side, Filter, First — standard library only
    physics/  package physics    plugin config (physics.go) + Match (categories.go)
                                  + NewBody2D, WorldPoint helpers
      mover/  package mover      Box2D's geometric mover (walls stop a circle, it slides along them)
                                  and the landing rule: pure functions over *box2d.World, no Cardinal
                                  import
      timestep/ package timestep the tick rate the server, the physics step and the predictor share
                                  (Rate, Seconds, Ticks) — imports only math
    ref/      package ref         Spawn{Serial} and Next: tells an entity from a newer one with its ID — a
                                  component-only leaf (imports only cardinal)
    team/     package team        Team and Owner allegiance components + SameTeam, SideOf — a
                                  component-only leaf (imports only cardinal and interact)
    combat/   package combat      damage: damage_system.go (the one writer of Health), death_system.go
                                  (removes what died, after the syncs)
      component/  package component   Health, DamageIntent, HealIntent, Died — combat's leaf; any damage or
                                      heal source emits an intent without importing combat's systems
    area/     package area        areas: area_system.go (each area acts on what overlaps its holder,
                                  by rules), tuning.go (area kinds), doc.go
      component/  package component   Area — area's leaf; a constructor gives its entity an area
    player/   package player      player Body: spawn_system.go (the one Body constructor),
                                  input_system.go, movement_system.go, input_command.go,
                                  sync_system.go, tuning.go (champions)
      component/  package component   Player, Champion, InputQueue + InputFrame, MoveSpeed,
                                      BodyClaimed — player's leaf
      motion/     package motion      how a Body moves each tick: Step (the server's move, and the
                                      predictor's), Unit, Blockers — no Cardinal import
    echo/     package echo        the Echo replay: spawn_system.go, replay_system.go, sync_system.go,
                                  elimination_system.go, tuning.go, doc.go
      component/  package component   Echo, PathHistory — echo's leaf
    ability/  package ability     casting: equip_system.go (cooldowns on claim) → cast_system.go (the
                                  gate; the one writer of cooldowns) → cast_hook_system.go (runs each
                                  cast's hook, turns its requests into intents), sync_system.go,
                                  tuning.go (AbilityTuning: cast.Tuning plus its kit-size check),
                                  doc.go
      component/  package component   Abilities (cooldowns by ability ID), AbilityCast + Cause —
                                      ability's leaf
      cast/       package cast        the cast gate (Gate), Cooldowns, Tuning (kits, abilities, their
                                      bound hooks) — no Cardinal import
      hooks/      package hooks       what abilities do: one hook per name (launch_hook.go,
                                      dash_hook.go), Bind, Out — standard library and timestep only
    projectile/ package projectile  shots: launch_system.go (the one shot constructor) →
                                  flight_system.go (flies each shot, a predicted one at its client's
                                  view, acts on what it meets), history_system.go (each target's
                                  recent rows), target_sync_system.go (each tick's rows, for
                                  predictors), sync_system.go, tuning.go (ProjectileTuning =
                                  flight.Tuning), doc.go
      component/  package component   Projectile, LaunchIntent, ShotEnded, TargetHistory — projectile's leaf
      flight/     package flight      one tick of a shot's flight and what it meets first: Tuning (shot
                                      types, the default rules, the rewind window), Launch, Fly, and
                                      FlyAtView with its view rule (Row, AtView, View) — no Cardinal
                                      import
    effect/   package effect      timed effects on any entity: resolve_system.go (the one writer of
                                  Effects), tuning.go (EffectTuning = status.Tuning), doc.go
      component/  package component   Effects, EffectIntent — effect's leaf; any feature reads a status
                                      or asks for an effect without importing effect's systems
      status/     package status      the rules: Entry, Fold, Sweep, Tuning (what each kind does),
                                      Intent, Apply — standard library and interact only
    netsync/  package netsync     who may receive what: Viewers, Items (audience + seenBy). No events
                                  of its own; each feature sends its own
    predict/  package predict     the client's own Body ahead of the server: Reset from a body_sync,
                                  See each sync's Bodies and Echoes, Step per frame, new or replayed
                                  (cast gate, move, hooks, its shots at the frame's view, its
                                  effects) — no Cardinal import
  predictor/  package main        js/wasm only: predict for the browser, with content.FS embedded
                                  (client/scripts/predictor.mjs)
    version/  package main        prints echoimage.SimVersion, which predictor.mjs stamps into the wasm
```

The repo root holds one Go package, `echoimage` (`sim.go`): `SimVersion`, which build of the sim a
binary runs.

Current dependency direction (must stay acyclic): a feature's behavior package depends on **physics**,
the **team**, **interact** and **ref** primitives, **netsync**, and any **`<feature>/component`** leaves
it reads — never another feature's behavior package (`player → {physics, interact, team, ref, netsync,
player/component, player/motion, effect/component, effect/status, ability/component, ability/cast,
projectile/component}`, `ability → {ability/hooks, ability/cast, team, netsync, player/component,
ability/component, effect/component, effect/status, projectile/component}`, `projectile → {physics,
interact, team, ref, netsync, combat/component, effect/component, player/component,
projectile/component, projectile/flight}`, `area → {physics, interact, team, area/component,
combat/component, effect/component}`, `combat → {combat/component, ability/component}`, `echo →
{physics, interact, team, ref, netsync, echo/component, area/component, combat/component,
player/component, effect/component, projectile/component}`, `effect → {effect/component,
effect/status}`).
`netsync` imports only `team` and `player/component`, so any feature can sync through it. Every
`component` leaf and `team` import only cardinal (and the physics2d component types; effect's leaf
also imports `effect/status`, ability's imports `ability/cast` for `Cooldown`, team imports `interact`,
combat's and projectile's leaves import ability's leaf for `Cause`, and projectile's imports
`projectile/flight` for `Row`), so they're dependency *sinks*
— a feature can read another's components without ever closing a cycle.
The shared sim code imports no Cardinal: `physics/timestep` imports only math, `physics/mover` only box2d;
`interact`, `effect/status` and `ability/hooks` only the standard library (and `interact`, and
`physics/timestep` for hooks); and
`ability/cast`, `projectile/flight`, `player/motion` and `predict` only those and box2d. So the client
predictor builds them for wasm, and any feature may import them like a leaf. Player's leaf imports
`player/motion` for `InputFrame.Move`. `content` is a leaf too, standard library only (imported by
package main, `predict` and the predictor for `FS` and `Load`); feature packages never import it — they
name their JSON file by path string in `JSONFile()`.

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
3. **Descriptive package names, no stutter.** `physics.NewPlugin` not `physics.NewPhysicsPlugin`;
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
   load-bearing** — then the file boundary documents the pipeline. ability's `cast_system.go` (the
   gate, emits `AbilityCast`) and `cast_hook_system.go` (runs the hooks) are separate *because* the
   gate runs in PreUpdate and the hooks in Update; likewise projectile's `launch_system.go` →
   `flight_system.go`. Don't split just because a file feels big.
4. **A cross-cutting vocabulary many features import gets its own home, even when small**
   (`interact` — the kinds, tags and relations every rule uses).
5. **The package's front-door doc comment has one home:** a dedicated `doc.go` (`echo`, `ability`),
   or the namesake file for a component-only package (`team.go`). Never leave it on whichever file
   sorts first.

---

## Architectural decisions

### Tick-rate independence (fixed-timestep sim)
The simulation must be **tick-rate agnostic**: changing `TickRate` changes only how finely we sample
time — never how fast the game plays or how hard anything hits. A build at 60 Hz must be the *same
game* as one at 20 Hz, just smoother. Every system honors this:
- **Rates are per-second, scaled by `dt` at the point of use.** `dt = 1 / physics.ServerTickRateHz`
  (`physics/timestep`, which the client predictor shares).
  Never bake a per-*tick* constant. A damage-over-time of 25/s is `25 * dt` this tick, not a hardcoded
  `1.25`. Movement already complies: `Velocity2D` is world-units-per-*second* and
  `player.MovementSystem` moves a Body by `velocity * dt`, so units/second is constant across tick
  rates — a faster tick rate must never move anything faster.
- **Durations & delays are configured in seconds and converted to ticks** by `timestep.Ticks`
  (`round(seconds * Rate)`), the one place that converts — never stored as a raw tick count, the
  snapshot interval (`SnapshotRate`, 0.5 s) included. The Echo delay, cooldowns, and any ring-buffer
  sizing all derive from seconds so they hold their wall-clock meaning when the rate changes.
- **Discrete vs continuous.** A discrete event (a projectile hit, a spawn) applies a fixed amount and is
  already rate-independent. A continuous effect (DoT, regen, movement) must scale by `dt`.
- **Integer HP vs sub-unit-per-tick.** `dt`-scaled continuous damage is fractional; if `Health.HP`
  stays an int, a continuous source must accumulate the fraction and apply whole points as it crosses
  them — never truncate per tick, or a DoT rounds toward zero at high tick rates. (Switching HP/damage
  to a float or fixed-point removes the accumulator; decide per the combat model.)
- **One source of truth for the rate.** Read `physics.ServerTickRateHz` (or a `physics` `dt` helper);
  never hardcode `20`. `main.go` already feeds it to both the Cardinal `WorldOptions.TickRate` and the
  physics2d step. A build tag picks it: `timestep.Rate` is 20, or 60 built with `-tags rate60`
  (`rate20.go`, `rate60.go`), and the sim version says which.
- **Tests run at both rates.** They state times in seconds (`ticks(seconds)` in `main_test.go`), and
  each rate has its own prediction recording. The client converts seconds to frames with body_sync's
  `TickSeconds` (`InputStream`), and its tests run at 20 and 60 Hz.

### Rules: what happens when X meets Y (`interact`)
- **Every "X meets Y" is data: a filter plus a response, and the first rule that matches wins.** The
  vocabulary is `interact`: `Kind` (what a thing is — the Box2D category bits), `Tags` (states effects
  grant, e.g. `stunned`), `Relation` (`own`, `ally`, `enemy`, `neutral`, from each side's `team.Owner`
  and `team.Team`), and `Filter{Kinds, Relations, Require, Forbid}` (an empty field matches anything).
  Content names them in JSON lists; an unknown name fails the boot.
- **The GDD's rules are default content, not code.** A shot type's own `meets` rules are checked first,
  then `defaults` in `content/projectile/tuning.json` (walls and bounds stop, own and ally pass, `out`
  Bodies pass, Bodies stop, enemy Echoes take the payload). So "some shots pass Bodies" is one rule on that shot type. Order
  is the only priority; the rule index breaks ties between things met at the same point. A rule with a
  payload must name its relations, since a shot type's own rules come before the defaults' own and ally
  pass: friendly fire is written out, never a default.
- **Executors switch on responses and options, never on content IDs.** `projectile.FlightSystem` (shot
  `meets`: `stop`, `pass`, payload yes or no) and `area.AreaSystem` (area rules) are the executors
  today. A new response is an enum value with a JSON name plus one branch in its executor, tested
  against the others.
- **Tags are a closed Go list, append only** (snapshots keep the bits). Add one only when several
  features read it; state one champion alone reads stays in its own component. What a tag does to
  other effects is data: an effect kind's `ends` (the holder's entries it removes as it lands) and
  `blocked_by` (holder tags that stop it), and the abilities' `blocked_by` (the cast gate).
- **Areas test who is inside every tick.** An `area/component.Area{Kind, Start}` makes its entity's
  first shape act on what overlaps it: one Box2D circle overlap (`Physics.Engine().OverlapShape`), then
  the first matching rule per thing. Bodies and Echoes mask `interact.Area` so the query finds them. Anti-camp is
  the Echo's area (`content/area/tuning.json`, kind `anticamp`), and its `Start` is the warmup.

### Collision handling: deferred intents, rule-driven shots, per-feature contact reactions
- **Damage is deferred.** A damage source emits `DamageIntent`, and a heal `HealIntent` (both in the
  `combat/component` leaf, so a source never imports combat's systems). `combat.DamageResolveSystem`
  (Update, registered *after* every emitter) drains them and is the **only** place `Health` changes.
  Sources today: `projectile.FlightSystem` (shot hits) and `area.AreaSystem` (anti-camp); no heals yet.
- **HP stays between 0 and `MaxHP`.** For each target the resolver takes the tick's damage, then its
  heals, and writes HP once, never above `MaxHP`. Overkill counts, so a small heal can't save a target
  from a much bigger hit. Intents are summed in sorted order, so the order they arrive in — or which
  source emitted first — never changes the result, and there's **no per-type event fan-out**.
- **A death is timed.** Each `DamageIntent` says when in the tick it lands (`Until`: a shot's
  `flight.End.Until`, 1 for damage over the tick). At 0 or below the target dies: HP 0 and a `Died`
  with the time of the hit that took it there. `combat.DeathSystem` removes it after the syncs, so each
  sync sends it once more.
- **An Echo's death puts its player out for the round** (GDD §8). `echo.EliminationSystem` gives the
  owner's Body the Echo tuning's `death_effect` (`out`) for `status.Forever` ticks. The cast gate's
  `blocked_by` and a default shot rule read the `out` tag, so the player neither casts nor shields. (Rampage re-emitted a typed event per
  collision kind to control ordering; deferral makes that unnecessary.)
- **Credit vs cause.** An intent's `Credit` is the Body credited with it (kills, lifesteal): the
  `team.Owner` of what dealt it. Its `Cause` is the cast that started it
  (`ability/component.Cause{Caster, Ability, Tick, Seq}`, which `AbilityCast` embeds), carried by
  shots. `Seq` is the input frame that cast it, or 0 for a cast no frame made (an Echo's). A reflect
  would change the shot's Owner and keep Cause. An area's damage has no cause.
- **Shots don't use physics contacts.** A projectile has no Box2D body. `projectile.FlightSystem` moves
  each shot every tick and resolves the **first** thing on its path that its rules don't pass, with one
  Box2D shape cast (`flight.Fly`) on `Physics.Engine()` over the map kinds its rules stop at, and the
  Bodies and Echoes for a shot no frame cast. The cast's callback judges each shape found by its
  owner's relation and tags (team isn't a collision bit) and keeps the nearest it doesn't pass. Bodies
  and Echoes mask `interact.Shot` so the cast finds them. A shot's `Type` names its shot type, whose
  rules are looked up each tick; its numbers are copied at launch.
  - First hit wins, which is what makes body-blocking exact, and a swept path can't tunnel at any
    speed. At the same point the earlier rule wins, then the map, then the lower entity ID, so by
    default a Body standing on its own Echo shields it.
  - **Every end is one fact.** `flight.Fly` (and `flight.FlyAtView`) returns a `flight.End`
    straight from the sweep: where the shot's center was, when in the tick (`Until`), and what it met.
    `FlightSystem` emits it as `component.ShotEnded` for every end, its range included. Damage stays a
    `DamageIntent`; the sync reads `ShotEnded`, as impacts and on-end rules will.
  - `pass` with a payload is rejected at load: with no hit list yet, it would hit the same thing every
    tick. Pierce, bounce and the rest come with their first users and a `Hits` list.
  - Cost is one cast per shot per tick, O(log N + K·R) for N shapes, K found and R rules. A predicted
    shot's is one map cast, one tree query, and one pair cast per Body or Echo found, plus O(T·W) a
    tick to build the tree of T targets keeping W rows.
- **A predicted shot meets Bodies and Echoes where its client drew them (favor the shooter, by server
  rewind).** A shot a frame cast is `Projectile.Predicted`: its caster's client flies it in the
  predictor. Each tick `FlightSystem` flies it with `flight.FlyAtView` at its caster's view, with no
  claim from the client:
  - The view is the applied frame's `ViewTick`, moved on by the ticks since on a tick with no new frame
    (`InputQueue.View`), then held to the views the rows kept are drawn from (`flight.Tuning.View`): no
    older than `rewind_seconds`, no newer than the newest row, the tick before. An older view (a high
    ping) is judged at the oldest, so it must lead its shots but still hits.
  - A view never goes back: a frame's `ViewTick` below the largest one applied, or not a number, is
    taken at that one (`InputQueue.ViewFloor`, from 0, kept across sessions). A client can lag, but
    can't pick an older moment for each shot. An honest view only moves on (the render clock never
    steps back), so the predictor has no copy of this rule.
  - The map is now. It is never rewound.
  - Each Body and Echo is where the view drew it (`TargetHistory`, `flight.AtView`). `flight.Reach`
    finds them: a Box2D tree built each tick, one box per target around its kept rows and where it is
    now, so no speed bound (a dash, blink or landing stays in its box). Each is pair-cast at its view.
    The caster's own Body is where it is now.
  - The first thing met ends the shot now, as in `Fly`, and a payload becomes a `DamageIntent`.
    History is never rewritten.
  - The pair cast is in the target's own frame, as a world cast does it, so it gets the predictor's
    bits: no margin (`TestFlyAtViewMatchesFlyBitForBit`).
  - A shot no frame cast (an Echo's) meets Bodies and Echoes where they are now.
  - Plan and limits: `docs/research/favor-the-shooter.md`.
- **Contact reactions** (pickups, triggers; none exist yet): `physics2d` emits `ContactBegin/End` +
  `TriggerBegin/End` system events each tick (on PreUpdate). Blocking and sliding aren't reactions
  (a Body's happen in `player.MovementSystem`, see Movement and walls); these events carry category
  bits, not meaning. **A feature that reacts to contacts declares its own receiver** over that stream
  and does a **single pass** for *its* kinds only, matching by **category bits** (`physics.Match` —
  integer compare, *never* a reverse ECS type lookup) and **emitting intents**. It is **not** a global
  router.
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
- **Every collision or overlap check is a Box2D query. Never hand-write one.** No distance or overlap
  math between entities, and no loop over every target: ask Box2D with a cast, overlap or ray, through
  physics2d or `Physics.Engine()`, and decide pass or stop in its callback. Box2D's tree is what keeps
  it scalable, and every shape is already in it. A Box2D pair cast (`box2d.ShapeCastCircle`) on a shape
  the tree found counts too, as does one on what a component saved, found by its own
  `box2d.DynamicTree` (`flight.Reach`: a predicted shot's targets at its view).
- **Category bits describe what a shape IS (its kind, `interact.Kind`)** — `Body`, `Echo`, `Shot`,
  `Wall`, `Bounds` (the map edge), `Area` (an area query) — never whose side it's on. Snapshots keep
  the bits, so never renumber one.
- **A map shape's category is its map kind, and its mask is all bits.** Each query's mask lists the
  kinds that stop it (`motion.Blockers`, or the map kinds a shot's rules stop at). A per-Body rule,
  like a pass through walls, changes that Body's query mask, never a shape's bits.
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
- **Owner vs team.** Anti-camp self-damage (Echo overlapping its OWN Body) is an **owner** match
  (relation `own`); friendly fire (a shot passing through everything on its own team) is a **team**
  match (`own` or `ally`). Bodies (themselves), Echoes and shots carry `team.Owner` and the owner's
  `Team`, and `team.SideOf` turns them into an `interact.Side` for `Relation`. A "same-team = don't
  collide" bit/`groupIndex` scheme is therefore wrong for damage — it would suppress the owner-match
  self-collision.
- `groupIndex` can't express same-team pass-through for movement: the mover uses Box2D queries, and
  their filter ignores `groupIndex`. If Bodies ever block each other, decide that in the mover's planes,
  and skip the Body's own shape: queries don't, and the mover would push itself away from where it was.
  Never use it for damage rules.

### Movement and walls
- **Bodies are `BodyTypeManual`, and `player.MovementSystem` moves them itself.** Box2D never pairs a
  kinematic or Manual body with a static or kinematic one, so its step can't stop a Body.
- **One tick's move is `motion.Step`,** which the server and the client predictor both run. It
  lands a Body that's inside a blocker, picks its velocity and runs the mover.
- **The move is Box2D's geometric mover,** `physics/mover.Move`: up to 5 passes of collide → solve
  planes → cast, then the velocity is clipped. It writes `Transform2D`. Walls stop a Body and it
  slides along them.
- **The mover's query filter decides what blocks a Body:** `{Category: interact.Body, Mask:
  motion.Blockers &^ passes}`. The filter is two-way, which is why map shapes mask all bits. The Body's own
  `MaskBits` doesn't drive the mover.
- **Effects feed the mover and never move a Body.** `MovementSystem` folds the Body's `Effects` each
  tick: a forced velocity (dash, push) replaces input, the folded speed scales input (slows; a stun is
  zero), and a pass drops map kinds from the mask (`"passes": ["wall"]`). Bounds have no pass name, so
  nothing passes them.
- **Bodies don't block each other, for now,** and walk through Echoes (anti-camp needs that overlap).
  Rules for what blocks a Body (`blocked_by`) come with the first rule beyond walls and bounds.
- **`Velocity2D` is the Body's velocity this tick** in units per second, clipped against the walls it
  touched. Coasting through a late frame uses the last frame, never `Velocity2D`, so a dash doesn't leak
  into the coast. The physics plugin also needs the component on every physics body.
- **A Body gets no `ContactBegin/End`** with walls or other Bodies. A static sensor still fires
  `TriggerBegin/End` for it.
- **A Body whose center is inside a blocker lands first** (`mover.Land`: the nearest spot on a 0.25 m
  grid within `mover.LandReach` where the whole circle is clear, and the straight way there doesn't
  cross the map edge). The mover can't push a center out, so it would walk out the far side. This
  covers a phase ending inside a wall. Map walls must be thinner than twice `LandReach`, with no sealed
  pockets.
- **The mover is shared sim code** the client predictor runs too, so it must stay deterministic: no
  Cardinal import, FMA-free arithmetic (`float64(a*b)`, or box2d helpers like `box2d.MulAdd`), and
  planes sorted by `(Normal.X, Normal.Y, Offset)`, never by ShapeID. So must the cast gate, hooks,
  shot flight and effect rules. `TestSharedCoreIsDeterministic` checks every package the predictor
  links from this module and world-engine (`go list -deps`, so a new import is scanned the day it
  lands): no Cardinal import, no FMA instruction on arm64, amd64/v3 or wasm, and no `math` call but the
  exact ones (floor, mod, …).
  **Never `math.Hypot`:** it fuses on arm64. Use `box2d.Length`.

### System registration ordering (`main.go`)
- The physics plugin registers on `Init` (rebuild) and `PreUpdate` (step), so its events are ready
  before `Update` game logic runs.
- **System events are same-tick and cleared at tick end**, so the **emitter must be registered
  before the receiver**: `player.InputSystem` (emits `BodyClaimed`) before `echo.SpawnSystem` and
  `ability.EquipSystem`, `ability.CastSystem` (emits `AbilityCast`) before `ability.CastHookSystem`,
  which emits `LaunchIntent` before `projectile.LaunchSystem`; every `DamageIntent` and `HealIntent`
  emitter (`projectile.FlightSystem`, `area.AreaSystem`) before `combat.DamageResolveSystem`; and every
  `EffectIntent` emitter (`ability.CastHookSystem`, and `echo.EliminationSystem`, which reads `Died`
  after `combat.DamageResolveSystem`) before `effect.ResolveSystem`.
- **Every receiver sorts its intents by a full key before it acts,** because emit order can change
  after a restore (`LaunchSystem` sorts launches, so new shots get the same entity IDs).
- **`player.InputSystem`, `ability.EquipSystem`, `ability.CastSystem`, then `player.MovementSystem`
  run in PreUpdate, registered before the physics plugin.** Movement moves each Body by the frame Input
  just applied, so a frame moves the Body in the same tick its `InputAck` is sent. Client prediction
  depends on this (`TestFrameMovesBodySameTickAsAck`). The predictor steps in the same order: input,
  cast gate, move, then the hooks and shots from where the Body moved. Effects a cast starts apply
  from the next tick.
  - Before the plugin, the step sees each Body where it moved to this tick. The mover sees walls as of
    the last step, so a wall created this tick blocks from the next one.
  - `physics.NewPlugin()` builds the plugin first so Movement can hold it. Where `world.RegisterPlugin`
    is called is what places its step.
- **`echo.ReplaySystem` runs in PreUpdate, after Movement and before the physics plugin,** so Box2D
  holds this tick's Echo poses when shots and areas query it (`TestShotMeetsEchoWhereItIsNow`). An
  Echo spawned this tick joins Box2D next tick.
- **Hooks, shots and areas run in Update:** `ability.CastHookSystem` → `projectile.LaunchSystem` →
  `projectile.FlightSystem` → `area.AreaSystem`. A Body can cast the tick it is claimed (Equip runs
  before Cast), and a shot launched this tick moves this tick, its path starting at the shooter.
- **`effect.ResolveSystem` runs after every system that reads status, just before the syncs.** What it
  adds applies from the next tick, and it drops what is over by then, so a whole tick reads one status.
- **`projectile.HistorySystem` runs after `effect.ResolveSystem`, before the syncs.** Each Body's and
  Echo's row is exactly what this tick's syncs show of it, its tags read for the next tick and its
  `Until` from its `Died`, so a predicted shot meets what its client drew. `FlightSystem` runs before
  it, so a view can use rows up to the tick before (`TestTargetHistoryIsWhatSyncsShow`).
- **Sync systems run last** (`player`, `echo`, `ability`, `projectile`, then
  `projectile.TargetSyncSystem`), so each sends the tick's final state. Only `combat.DeathSystem` comes
  after them: it removes what died this tick, once each sync has sent it.

### Game data / definitions (`content.Load`)
- **One loader, `content.Load[T](fsys)`,** which the shard and the client predictor share. It decodes
  T's file strictly (an unknown key fails, so a typo never loads as 0), then runs T's `Bind` and
  `Validate`. Any error fails the boot. **Never hand-roll another JSON loader.** (world-engine's data
  plugin, which this replaced, is gone upstream, #1133.)
- **A definition is a Go type implementing `JSONFile()`** (`content.File`). Optional: `Bind()`
  (binds what decoding can't, e.g. `cast.Tuning` binds each ability's hook) and `Validate()` (one
  file's invariants).
- **The shard loads every file once at boot** (`loadCatalog` in `shards/game/catalog.go`), then checks
  the IDs one file names in another: each ability's shots and effect kinds (hooks list theirs in
  `Refs`), and each champion's kit, Echo and Echo area. A champion with no Echo could never be
  eliminated. `TestBadContentFailsTheBoot` pins both kinds of failure.
- **Each system that reads config holds its file** in a typed `Tuning` field, set in `main.go` from
  the catalog. Content never changes after boot, so a system reads it directly, never a copy per
  entity.
- **Tests change content with `withContent`** (`main_test.go`): it deep-merges JSON patches into the
  shipped files and boots the real `register` from that `fs.FS`; `patched` gives a predictor the same.
  Use it for test-only shot types and champions; never add test content to the shipped files.
- **Every test Body comes from `spawnBody`.** A test that needs more players patches the dev roster
  (`dev_spawns` in `content/player/tuning.json`, e.g. `twoVsTwo`) and adds them with `join`. Players
  claim free Bodies in roster order.
- **The definition TYPE lives in the feature package that owns it** (e.g. `projectile.ProjectileTuning`),
  **never a central `types/` grab-bag.** Shared sim code keeps the type in its own package
  (`flight.Tuning`), and the feature names it with an alias, or embeds it to add a check
  (`AbilityTuning{cast.Tuning}` adds the kit-size check). Rampage centralizes all definitions in
  `pkg/datadef/types` and 40 feature files import it — that's the layered bucket we reject.
  Definitions reference each other **by string ID**, checked at boot, so feature-owned types create no
  import cycles.
- **JSON content lives under `content/<feature>/<file>.json`**, one `embed.FS` (`content.FS`) —
  `go:embed` can't reach across directories. `JSONFile()` returns the path within that FS (e.g.
  `"combat/tuning.json"`).
- **Content is code.** It is embedded and hashed into `SimVersion`, so a shard resumes a snapshot on
  the content its build carries; changing content is a new build.
- **Prefer typed per-feature config structs** over a single stringly-typed global settings bag
  (rampage's `game_settings.json` uses `Parameter string / Value json.RawMessage` — untyped, avoid).

### Client sync (`netsync`)
- **Events are the only way to push data to clients.** Cardinal has no client queries (removed in
  world-engine #915) and no automatic component sync.
- **Each feature sends its own sync event** from its own `SyncSystem`, registered last:
  `player.BodySync`, `echo.EchoSync`, `ability.AbilitySync`, `projectile.ProjectileSync` and
  `projectile.TargetSync`. Each goes to every player with `w.SendTo` every tick and is complete — an
  entity missing from it is gone or hidden — so joining or reconnecting needs nothing else. Clients
  interpolate each stream on `Tick * TickSeconds`. A new synced feature adds its own event and
  `SyncSystem`; `netsync` doesn't change.
- **One rule draws and judges every view.** View v is drawn from the states at ticks ⌈v⌉ − 1 and ⌈v⌉,
  v − (⌈v⌉ − 1) of the way through the newer one's tick, so a whole tick is its own end; without both,
  nothing is drawn (`Timeline.sample`, then `blend`). The server and the predictor place targets by the
  same rule (`flight.RowTicks`, `flight.AtView`), and `testdata/view.json` pins the two sides together,
  to the bit (`TestViewFixture`, `entity_views.test.ts`). The render clock draws `InterpolationDelay`
  behind the newest tick every timeline has, never past it: in a stall it waits there. It never steps
  back, and eases by time, not by frame (`Timelines.renderTick`). So the view a frame sends always has
  both its rows, in the predictor and on the server.
- **An entity that ends partway through a tick is sent once more, where and when it ended.** A shot
  comes in `ProjectileSync` on the tick it ended, `Ended`, at its end with `Until` (the part of the tick
  it lived), through `netsync.Items.AddGone`. The engine's `EntityViews` draws every entity with
  `blend`: one with `Until` below 1 moves at its own pace to its end, then goes, so it touches what it
  hit. An Echo killed in a tick comes in `EchoSync` and `TargetSync` the same way: at HP 0, with `Until`
  from its `Died`, so it goes when the shot reaches it. `flight.AtView` follows the same rule, so your
  predictor drops it then too. Add `Until` to another entity's state when it needs the same.
- **A shot that ended on a Body or Echo ends on it on every screen.** The hit was judged where the
  shooter's view had the target, but the victim's screen draws their Body ahead, and others draw it
  later. `flight.Hit.Offset` is the end from the target's center as judged; `ProjectileState` sends it
  (`OnTarget`, `TargetSerial`, `OffsetX`, `OffsetY`), and so does the predictor for your own shots. Each
  screen ends the shot at the target where it draws it this frame, plus the offset (`endOn`, from
  `EntityViews.at`; shots draw after Bodies and Echoes). A target the screen doesn't draw keeps `X, Y`.
- **A shot says where its flight this tick began** (`PrevX`, `PrevY`: its launch point on its first
  tick), in `ProjectileState` and `ShotState`. Every screen draws its tick from there, never from an
  older sync (`Placed.PrevX`), so a new shot leaves its launcher. Your predictor keeps it on a reset, so
  your shot flies on when nothing replays.
- **An entity's serial names it across ticks; its EntityID only within one tick.** Cardinal gives a
  freed ID out again at once, so a shot launched the tick after another ended takes its ID. Every
  constructor stamps `ref.Spawn{Serial}` from the world's one counter (`ref.Next`, saved with the world,
  from 1), and `BodyState`, `EchoState` and `ProjectileState` carry it. `EntityViews` keys views and
  blends by `Serial`, the predictor keys its sights and bodies by it, and a hit names its target's
  (`flight.Hit.Serial`). Same-tick links (`You.Body`, `Caster`, `Owner`) stay EntityIDs.
- **Other screens get content IDs, not only rule bits.** `BodyState.Champion` names a Body's champion,
  and `BodyState.Effects` and `EchoState.Effects` list the kinds active from the next tick on, each
  with its last `End` (`status.Shown`), never their numbers or sources. `ProjectileState.Type` names a
  shot's type. A client keys its visuals by these IDs, so a new champion or effect kind needs no new
  field or tag.
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
- **Clients predict their own Body with the server's own code.** Each tick a client sends
  `Input{Session, Frames}` with its recent unacked frames — each frame is move, aim, buttons and the
  render tick on screen (`ViewTick`).
  `shards/game/predictor` builds `predict` (and through it `motion.Step` and world-engine's `box2d`) to
  wasm with the server's Go toolchain. Each frame the client sends runs one tick of it at once, in the
  server's order: the cast gate, the move, each cast's hook, your shots, then the effects your casts
  put on you. Each `BodySync` restarts it from your Body and `You` (`MoveSpeed`, `Champion`,
  `Cooldowns`, `Effects` with `EffectsNext`, never the folded status, `Shots`, and `EchoDelay` with
  `EchoPath`) at `Tick`, and replays the frames newer than `You.InputAck` (when `InputSession` is its
  own). A replay that lands elsewhere is a correction: your Body, Echo and each shot (by key) keep the
  path they're drawn on and fade to their new spots, or jump past 3 units, and a new shot starts as far
  off as your Body (`client/src/game/player/prediction.ts`). The HUD counts them.
  - **Every input is predicted:** a shot leaves the frame you click, from where that frame moves your
    Body, a dash starts the tick after, and the HUD starts a cooldown at once.
    `ProjectileState.Predicted` and `Caster` say whose predictor draws a shot: a client draws its own
    predicted shots from the predictor (`LocalPlayer.drawsShot`) and the rest from `ProjectileSync`.
    `LocalPlayer.you` is undefined before the first body_sync, since entity 0 is a Body too.
  - **Your Echo is predicted too,** so the gap between it and your Body on screen is the real one. Its
    next spots are already in the server's `PathHistory` ring (Echo(T) is Body(T − delay)), so
    `You.EchoPath` sends your Echo this tick, then its spot on each next tick, up to
    `echo_path_seconds` (`content/player/tuning.json`) or its delay, and `You.EchoDelay` the delay. The
    predictor walks the path a spot per step (`Predictor.Echo`). Past the delay, your Echo is where it
    predicted your Body that many steps back; past the path but inside the delay, it waits at the
    path's end. The client draws your Echo from it (`LocalPlayer.drawnEcho`), others' from `EchoSync`.
    The tick your Echo dies, `EchoPath` is empty, since it is gone the next.
  - **Your shots stop where your screen shows them hit, as the server judges them.** A target's row
    (`flight.Row`: kind, side, circle, tags and `Until`) is its one record: `TargetHistory` keeps its
    rows, and `TargetSync` sends each tick's, which is all your predictor reads to place targets. The
    predictor keeps as many ticks of them as the server keeps rows (`KeptRows`) (`See`), and a body for
    each target that lived all of the newest sync's tick: one that died in it is gone on the server
    before your next frame. The render clock waits for `TargetSync` like any timeline
    (`LocalPlayer.targets`), and it feeds the predictor only once a body_sync says the shard runs your
    sim. Shot code names no target kind but to find your own Echo, which your predictor places itself.
    Each step places them at its `ViewTick`, held by the server's rule (`flight.Tuning.View`), with
    `flight.AtView` (the rule `EntityViews` draws with), and your Body and Echo where they are now, as
    your screen draws them (your Echo with its rows' tags). A shot that stops on one ends there, as it
    will on the server. `Step` returns the shots that ended (`predict.Ended`, with their `flight.End`),
    drawn with the same `blend` as everyone's. A replay after each body_sync is the same `Step`, so it
    ends a shot where the step did; a shot the server flew on comes back from `You.Shots`, and the end
    your frame drew for it is dropped (by key, when nothing replays). Views the predictor has no syncs
    for (frames over `rewind_seconds` ahead of your newest sync: a high ping or a deep queue) pass
    Bodies and Echoes, while the server may still hit there.
  - **The predictor embeds `content.FS`** and loads it with `predict.Load`, through the shard's own
    `content.Load`.
  - **A client predicts only for a shard of its own build.** `echoimage.SimVersion` is the Go release,
    the tick rate and a hash of `go.mod`, `go.sum`, `pkg/` and `shards/`, without tests or testdata. The
    shard hashes the copy it embeds (its Docker build has no `.git`) and sends it as body_sync's
    `SimVersion`.
    `predictor.mjs` stamps the same into the wasm (`-X main.simVersion`), which never embeds the
    source, so players don't download it. When they differ, `LocalPlayer` doesn't play and the HUD
    says to reload (in dev, restart `bun run dev` first). `TestSimVersionCoversWhatTheSimIsBuiltFrom`
    checks every file the shard and the predictor build from is hashed.
  - **Exact, unless the network surprises it.** `TestPredictionMatchesServer` checks the predictor lands
    on the server's bits, each shot's end and your Echo included, after a step and after a replay. Its
    golden is the run's text digest, `testdata/prediction-20hz.txt` (`-60hz` at 60 Hz, a line a step):
    `-update` rewrites it, so review that diff. The wasm build must land on the same run: vitest's setup
    has the test write it (`-record`, a gitignored build output) and `predictor.test.ts` replays it.
    The run plays its own pinned content (`recorded` in `prediction_test.go`), so a balance change
    never moves it. Its bad network (`badNetwork`: lost, held
    and bunched Inputs, a full queue dropping frames) checks each replay of several unacked frames
    lands there too once its frame applies, unless the server coasted or dropped a frame first, and
    that a correction is at most a tick of walking per such surprise. The recording keeps those
    replays. `TestPredictedEchoMatchesServer` checks your Echo over ten frames from one state, and
    `TestShotMeetsYourEchoWhereYouDrawIt` a shot that stops on it.
    Corrections still come from what the client can't know: a late frame (the server repeats the last
    one, at a view moved on), effects others cause, and a target someone else kills first. Pacing
    keeps late frames rare.
  - The predictor's world has no map yet. Walls come with the arena, built exactly as the server builds
    them. Until then a shot the server stops at a wall flies on past it on your screen, until a
    body_sync shows it gone, about a round trip later.
  - The client aims from where the frame moves its Body, since the server casts from there.
- **The server queues frames and applies exactly one per Body per tick**, in Seq order
  (`player.InputSystem`, `player/component.InputQueue`). Duplicates and applied frames are dropped, and
  a Body with no input for `input_timeout_seconds` stops. One step per tick keeps the Body, and the
  Echo replaying it, smooth when frames arrive late or bunched. A new `Session` (a restarted client)
  starts the stream over.
- **The client paces its frames by the server's queue** (Overwatch's time dilation).
  `YouState.InputQueued` is how many frames wait for later ticks. `InputStream` averages it, in
  seconds, over 0.5 s and runs its frame clock 5% faster per 0.05 s below 0.05 s queued (a tick at
  20 Hz), or slower above, by at most 10%: each frame lands about 0.05 s before it's applied, so the
  queue neither starves (a coast) nor grows (latency). This also absorbs a server or client clock that runs a few percent off.
  `input_buffer_seconds` (0.5 s) is only an abuse cap: past it the oldest frames drop.
- **An Input carries at most the queue's frames** (`YouState.InputCap`, `input_buffer_seconds` of
  ticks). `InputSystem` drops a bigger one whole, before it claims a Body (Source's "too many cmds"),
  so merging an Input costs O(cap²), never O(frames²). The client sends its newest 0.4 s of unacked
  frames, at most `InputCap`, and one before the first body_sync says the cap.
- **Systems act on input through `InputQueue.Applied(tick)`**, so each frame acts once: movement sets
  velocity from it, abilities cast from it. Buttons ride in the frame (like a Source or Overwatch
  user command), so a shot leaves from where the client predicted its Body when it fired.
- **Cooldowns are server ticks** (`Cooldown.ReadyTick`), never a client count, so a client can't speed
  them up. Its predicted kit reads them from `You.Cooldowns`.

### Abilities and their effects
- **A champion is an ID, not a type.** A Body's `player/component.Champion` names the champion it
  plays, and each feature keeps that champion's numbers under the ID: `champions` (move speed, radius)
  in `content/player/tuning.json`, `kits` in `content/ability/tuning.json`, and its Echo (delay, area)
  in `content/echo/tuning.json`. A new champion is JSON entries, never new code.
- **The cast gate decides whether a cast happens.** A Body casts from its champion's kit.
  `cast.Gate`, which `ability.CastSystem` and the predictor both run, checks the button, cooldown and
  aim, and that the Body has no tag in the abilities' `blocked_by` (`stunned`, `out`), then starts the cooldown; `CastSystem` emits
  `AbilityCast{Cause, Aim}`. Cooldowns are kept per ability ID, so a cooldown
  follows its ability into whichever slot holds it.
- **What an ability does is its hook.** Every ability row names a hook in `ability/hooks` and gives
  its `params` (`{"hook": "launch", "params": {"shot": "bolt"}}`). A hook reads its `Cast` and params
  and appends requests to `Out` (`Launches`, `Effects`). `cast.Run`, which the server and the predictor
  both call, runs it and hands each request to a `cast.Sink`: launches numbered within the cast, from
  the caster, and effects as intents from it. `ability.CastHookSystem`'s sink emits them as intents with
  the cast's Cause and the caster's Owner and Team; the predictor's keeps your shots and the effects on
  your Body. Hooks never write state, never import Cardinal, and never branch on a champion or another
  ability's ID.
  - Shared hooks make common abilities JSON-only: `launch` (one shot along the aim) and `dash`. Make a
    shared hook at the third copy of an idea; a unique hook is named after its ability.
  - **Never assume the caster:** a hook gets the caster and its owner apart, so an Echo can cast too
    (`TestAnyCasterCastsEveryAbility`).
  - A new request type in `Out`, or a new method a hook may read, comes with its first user. A request
    type is a `Sink` method too, so the predictor can't miss it.
- **A shot is a shot type.** `content/projectile/tuning.json` keys shots by type (`shots.bolt`), and a
  hook names the type it launches. A new shot ability is a shot type (with its own `meets` only where it
  differs from the defaults) plus an ability row.
- **A dash is a forced-move effect.** The `dash` hook turns the cast into one along the aim, covering
  `distance` in `seconds` (whole ticks). It runs from the tick after the cast, walls stop it, and an
  effect whose kind `ends` `ends_on_cc` (a stun) ends it (the `dash` kind has `ends_on_cc`). A slow
  doesn't.
- **Timed effects (stun, slow, pass, forced move) belong to `effect`.** A feature asks with
  `EffectIntent{Target, Kind, Ticks, Value, VelX, VelY, Source}`; what each kind does (tags, passes,
  stat, stacking, ends, blocked_by) is content in `content/effect/tuning.json`, and the load fails on a name it doesn't
  know. An intent of an unknown kind is dropped with an error log. A refresh keeps what is strongest at
  every tick: a new entry is dropped only if one with its refresh key (its kind, plus its source for
  `per_source`) is as strong and lasts as long, and it replaces those it beats on both. Otherwise both
  stay and the fold counts the strongest of each key. So a short stun never cuts a long one short, and
  a weak, long slow never erases a strong one. Bodies and Echoes carry `Effects` from their constructors. Read a status with
  `Effects.Status(tick)`; the rules, and what each kind does (`status.Tuning`), live in `effect/status`
  so a predictor can run them.
- **Foundation for champions, effects and movement abilities:** `docs/research/character-foundation.md`
  (build steps 1-8 built). Read it before adding a champion or a movement ability. Abilities never move
  a Body; they feed the one mover a forced move, a speed change or a pass.
- **The architecture for every system:** `docs/research/system-architecture-design.md` (migration
  steps 1-9 built). Read it before adding a shot behavior, an area, a hook or a game mode; it says what
  comes with which first user.
- **`InputFrame.Buttons` bit N casts kit slot N**, so a new ability never changes the input type. The
  client sends left click as slot 0 and Space as slot 1. Each frame takes the buttons held and those
  pressed since a frame last took them (`Controls.takeButtons`, Source's latched buttons), so a tap
  between two frames still casts, in one frame. A press made while you can't play is dropped.
  `TestContentIDsLineUp` checks the content IDs line up across files.

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
- **Research findings** (not source of truth; this file and the code win) live in `docs/research/`.
  Map system: [`docs/research/map-system-findings.md`](docs/research/map-system-findings.md).
- World Engine source (incl. the `physics2d` plugin) lives at `../world-engine`; this project builds
  against that checkout, through the `replace` in `go.mod` (see Build & run).
