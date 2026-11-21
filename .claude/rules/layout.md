---
paths:
  - "shards/game/**"
---

## Repo layout — **package by feature**

```
shards/game/
  main.go                       makes the world and calls shard.Register, nothing else
  shard/     package shard       the wiring: Register, and RegisterWith for a test's content and metrics
    shard.go                    registration wiring ONLY
    catalog.go                  every content file, loaded at boot, and the IDs across files checked
  test/      package test        the world tests (only _test.go files) and their testdata
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
    team/     package team        Team and Owner allegiance components + SideOf — a
                                  component-only leaf (imports only cardinal and interact)
    combat/   package combat      damage: damage_system.go (the one writer of Health), death_system.go
                                  (removes what died, after the syncs)
      component/  package component   Health, DamageIntent, HealIntent, Died — combat's leaf; any damage or
                                      heal source sends an intent without importing combat's systems
    area/     package area        areas: area_system.go (each area acts on what overlaps its holder,
                                  by rules), tuning.go (area kinds), doc.go
      component/  package component   Area — area's leaf; a constructor gives its entity an area
    arena/    package arena       the game's side of its maps: Schema (the kinds wall, fill and bounds, the spawn
                                  type), Spawns and CheckSpawns, CheckMap (rules.go: the map rules a
                                  Body needs), and ArenaTuning (the maps that ship)
                                  — no Cardinal import
    player/   package player      player Body: spawn_system.go (the one Body constructor),
                                  input_system.go, movement_system.go, input_command.go,
                                  sync_system.go, tuning.go (champions)
      component/  package component   Player, Champion, InputQueue + InputFrame, MoveSpeed,
                                      BodyClaimed — player's leaf
      motion/     package motion      how a Body moves each tick: Step (the server's move, and the
                                      predictor's), Land, Unit, Blockers — no Cardinal import
    echo/     package echo        the Echo replay: spawn_system.go, replay_system.go, sync_system.go,
                                  elimination_system.go, tuning.go, doc.go
      component/  package component   Echo, PathHistory — echo's leaf
    ability/  package ability     casting: cast_system.go (the
                                  gate; the one writer of cooldowns) → cast_hook_system.go (runs each
                                  cast's hook, turns its requests into intents), sync_system.go,
                                  tuning.go (AbilityTuning: cast.Tuning plus its kit-size check),
                                  doc.go
      component/  package component   Abilities (cooldowns by ability ID), AbilityCast + Cause —
                                      ability's leaf
      cast/       package cast        the cast gate (Gate), Cooldowns, Tuning (kits, abilities, their
                                      bound hooks) — no Cardinal import
      hooks/      package hooks       what abilities do: one hook per name (launch_hook.go,
                                      dash_hook.go), Bind, Out — standard library, timestep and status only
    projectile/ package projectile  shots: launch_system.go (the one shot constructor) →
                                  flight_system.go (flies each shot, a predicted one at its client's
                                  view, acts on what it meets), sync_system.go, tuning.go
                                  (ProjectileTuning = flight.Tuning), doc.go
      component/  package component   Projectile, LaunchIntent, ShotEnded — projectile's leaf
      flight/     package flight      one tick of a shot's flight and what it meets first: Tuning (shot
                                      types, the default rules), Launch, and Fly — no Cardinal import
    history/    package history       lag compensation: scene_system.go (this tick's Scene, before every
                                  judge), record_system.go (each recorded thing's row, once a tick),
                                  sync_system.go (sends them as TargetSync), tuning.go (HistoryTuning =
                                  past.Tuning), doc.go
      component/  package component   History (the ring of a thing's rows; a constructor adds it) —
                                      history's leaf
      past/       package past        the one record, ring and view rule (Row, RowAt, RowTicks, AtView, Across), the
                                      Window a view may lag (Ticks, KeptRows, Clamp), and the Scene
                                      every judge queries (Thing, Actor, Query, Contact, Cast) — no
                                      Cardinal import
    effect/   package effect      timed effects on any entity: resolve_system.go (the one writer of
                                  Effects), tuning.go (EffectTuning = status.Tuning), doc.go
      component/  package component   Effects, EffectIntent — effect's leaf; any feature reads a status
                                      or asks for an effect without importing effect's systems
      status/     package status      the rules: Entry, Fold, Sweep, Tuning (what each kind does),
                                      Intent, Apply — standard library and interact only
    netsync/  package netsync     who may receive what: Viewers, Items (audience + seenBy). No events
                                  of its own; each feature sends its own
    predict/  package predict     the client's own Body ahead of the server: Reset from a body_sync,
                                  See each target_sync's rows, Step per frame, new or replayed
                                  (cast gate, move, hooks, its shots at the frame's view, its
                                  effects), through the map it names — no Cardinal import
  predictor/  package main        js/wasm only: predict for the browser, with content.FS embedded
                                  (client/scripts/predictor.mjs)
    version/  package main        prints echoimage.SimVersion, which predictor.mjs stamps into the wasm
```

The repo root holds one Go package, `echoimage` (`sim.go`): `SimVersion`, which build of the sim a
binary runs.

Current dependency direction (must stay acyclic): a feature's behavior package depends on **physics**,
the **team**, **interact** and **ref** primitives, **netsync**, and any **`<feature>/component`** leaves
it reads — never another feature's behavior package (`player → {physics, physics/timestep, interact,
team, ref, netsync, player/component, player/motion, effect/component, effect/status, ability/component,
ability/cast, combat/component, echo/component, projectile/component, history/component,
pkg/levelstate, arena}`, `arena → {interact, pkg/level, physics/mover, player/motion}`, `ability → {ability/hooks, ability/cast, team, ref, netsync, player/component,
ability/component, effect/component, effect/status, projectile/component}`, `projectile → {physics,
team, ref, netsync, combat/component, projectile/component, projectile/flight, history/past,
pkg/levelstate}`, `history →
{interact, team, ref, netsync, combat/component, effect/component, player/component, history/component,
history/past}`, `area → {physics, interact, team, area/component,
combat/component, effect/component}`, `combat → {combat/component, ability/component}`, `echo →
{physics, physics/timestep, interact, team, ref, netsync, echo/component, area/component,
combat/component, player/component, effect/component, effect/status, history/component}`, `effect → {effect/component,
effect/status}`).
`netsync` imports only `team` and `player/component`, so any feature can sync through it. Every
`component` leaf and `team` import only cardinal (and the physics2d component types; effect's leaf
also imports `effect/status`, ability's imports `ability/cast` for `Cooldown`, team imports `interact`,
combat's and projectile's leaves import ability's leaf for `Cause`, projectile's imports
`projectile/flight` for `End`, and history's imports `history/past` for `Row`), so they're dependency *sinks*
— a feature can read another's components without ever closing a cycle.
The shared sim code imports no Cardinal: `physics/timestep` imports only math, `physics/mover` only box2d;
`interact`, `effect/status` and `ability/hooks` only the standard library (and `interact`, and
`physics/timestep` and `effect/status` for hooks); and
`history/past`, `ability/cast`, `projectile/flight`, `player/motion` and `predict` only those and box2d. So the client
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
   `physics.NewBody2D` not `physics.NewPhysicsBody2D`. This keeps `shard.go` import-alias-free. The
   rule targets verb/function stutter; an eponymous type in a single-concept package (`team.Team`,
   like stdlib `list.List`) is fine.
4. **A component/event lives in the package that owns it.** Promote to a *cross-cutting* `shared`/`core`
   package only when ownership is genuinely ambiguous — not preemptively. (This is about ambiguous
   *ownership*; a feature's OWN `component/` leaf is a different, deliberately-early split — see File
   layout rule 2.)
5. **`shard.go` is wiring only** — register plugins/systems in order, no logic.
6. **Each feature registers its own types** in `register.go`: `RegisterTypes(w)` registers every
   component (including its `component/` leaf's), command, system event and queue the feature owns.
   `shard.go` calls each feature's `RegisterTypes` before registering systems.

### File layout within a package

The five rules above govern *directories*; these govern the *files inside* a package, and cover two
things that must both hold as a feature grows: you can tell a file's role at a glance, and one feature
can read another's data without risking an import cycle.

1. **Role goes in the filename suffix.** A behavior file is `<concept>_<role>.go` —
   `movement_system.go`, `input_command.go`, `spawn_system.go`, `damage_intent.go`. `ls` alone
   tells you what each file is; you never open `move.go` and `movement.go` to find out one's a command
   and the other's a system. Config definitions are `tuning.go` (`<concept>_tuning.go` when a feature
   has several); the package doc is `doc.go` or the namesake file. A search Row / query projection
   lives in the system file that uses it, not its own file.
2. **Components live in a `component/` leaf sub-package, so any feature can read them without importing
   systems.** ECS data is a dependency *sink*: `player/component` and `combat/component` (both `package
   component`) import only the engine. That's what lets a future `enemy` read `component.Player` or
   attach `component.Health` with **zero** risk of an import cycle — the failure that bites hard the
   first time two features reference each other's state. The leaf holds component *types* only — rows,
   systems, client events, commands, and tunings stay in the behavior package. One exception: a type
   another feature sends or reads lives in the owner's leaf too — an intent sent to its queue
   (`combat/component.DamageIntent`) or a system event others react to (`player/component.BodyClaimed`,
   `ability/component.AbilityCast`) — so no feature imports another's systems.
   - **This is NOT the "split by layer" anti-pattern.** Separating *data* (a genuine leaf) from
     *behavior* is a real dependency boundary; splitting *behavior* into `systems/`/`events/` dirs is
     the cosmetic layering we still reject (directory rule 2).
   - **A component-only feature skips the sub-package** — the feature package *is* the leaf
     (`team.Team`; `interact.Kind`, the category bits). Components graduate into `component/` only
     once the feature also owns systems a cross-feature importer would otherwise drag in.
   - **Cross-importing two features' components** takes a one-word import alias (both are `package
     component`): `import playercomp "…/player/component"`. That alias lives in the importing *system*
     file — `shard.go` imports feature packages, never their component leaves, so it stays alias-free.
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
