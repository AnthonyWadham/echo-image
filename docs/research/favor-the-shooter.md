# Favor the shooter: hits that match your screen

Plan, 2026-10-26. Not built yet.

## Summary

- Your Body and your shots are predicted, so they're drawn **ahead** of the server. Enemies are drawn
  **behind** it (interpolated, about 0.1 s). The server checks your shot against enemies where they are
  *now*. So on your screen the shot flies past the enemy until the server's hit arrives: an overshoot
  of shot speed × your lead (about 2 units locally at 15 units/s), which grows with ping.
- The fix shooters use (Overwatch, Valorant, Photon Fusion, Unity Netcode): **your screen decides the
  hit, and the server agrees** by checking your shot against the targets **as you saw them** (rewind).
- The server and the predictor run the same Go code on the same recorded positions. So when your frames
  reach the server on time, they reach the **same** answer, not a close one. (Walls aside: the
  predictor has none yet.)
- The victim pays: now and then they're hit "after dodging", bounded by a rewind cap (0.4 s).

| Who decides | What |
|---|---|
| Your screen (predicted) | your moves, casts, cooldowns, where your shot stops |
| The server, rewound to your view | whether your shot hit, and the damage |
| The server, now | everything else: others' shots, Echo casts, effects, deaths |

## Problems this solves

1. **Overshoot.** Your shot passes through an enemy and vanishes a few ticks later.
2. **Disagreement online.** Without a rewind, a hit on your screen can be a miss on the server (the
   target moved during your lead), so the shot would pop back.
3. **Respawn on a rare miss.** Each body_sync rebuilds your shots, so a shot your screen ended would
   reappear if the server disagreed.

## Changes

### 1. The view tick rides in each input frame

- **What:** `InputFrame.ViewTick` (float64): the render tick on screen when the frame was made.
- **How:** the client sends the last drawn render tick (frames are made before the next one is
  drawn). The server ignores a bad value (NaN, ±Inf: no rewind) and clamps it to
  `[tick − cap, tick − 1]`. It never rewinds past the cap, or to a tick it hasn't recorded yet.
- **Why:** it is the moment your screen showed. Unity Netcode and Photon Fusion send the same.

### 2. Targets keep a short history on the server (new feature: `rewind`)

- **What:** each Body and Echo keeps its shot-target row (center, radius, tags) for the last
  `cap + 1` ticks, in a component. A record system writes it at the end of each tick, after
  everything that moves or tags a target and before the syncs.
- **How:** packaged like `effect`: `rewind` (systems), `rewind/component` (the `History` leaf, added by
  the Body and Echo constructors), `rewind/history` (pure: `Row`, `At`).
- **Why:** the server must know where a target was when you saw it. A component is snapshot-safe. On
  the entity, the history dies with it, so a dead target, or a new entity reusing its ID, can't be hit
  through a rewind.
- **Cost:** O(targets) writes per tick, about 9 rows per target.

### 3. One rule for "targets as seen", shared by the server and the predictor

- **What:** `history.At(rows, view)`. It gives the targets that exist at `ceil(view)`. Positions are
  lerped from `floor(view)` to `ceil(view)`; when the earlier row is missing, the later position is
  used. Tags and radius come from `ceil(view)`. That is exactly what the client draws
  (`EntityViews`).
- **The shooter's own Body is never rewound.** It is taken at its current center (server) or predicted
  center (client), since that is how your screen draws it.
- **Determinism:** FMA-free lerp, added to the shared-core scan.

### 4. Shots check rewound targets on the server

- **What:** `FlightSystem` gives a shot cast by a Body with input the targets `At(history, view)`, plus
  the caster's own Body now. `view` is the caster's applied frame's `ViewTick`. Shots cast by Echoes
  have no view, so they use current targets, as today.
- **Late frame:** when no frame was applied this tick, the last frame's view advanced by the ticks
  since. That keeps the rewind distance steady; the clamp bounds it.
- **Cost:** target lists cached per caster per tick: O(shots + casters × targets).

### 5. TargetSync: the server sends what it recorded

- **What:** a new per-tick event with the rows recorded this tick, for the targets the viewer may see
  (through `netsync`, like `BodySync`).
- **Why:** the predictor then holds the same rows as the server's history, so `At(view)` returns
  identical targets on both sides.
- **Cost:** one more event per player per tick. With about 12 targets that's 0.5 KB per tick, about
  10 KB/s per player.
- **Rejected alternative:** adding tags to `BodyState`, and tags, owner and radius to `EchoState`, is
  cheaper. But the client would then rebuild targets from two event types: a second definition of
  "target" that drifts when a collider changes (an offset, a second circle).

### 6. The predictor stops your shots where you see them

- **What:** the predictor keeps recent TargetSync rows (back to the cap behind the last body_sync).
  Each step flies your shots against `At(rows, frame.ViewTick)` plus your predicted Body, with the same
  view clamp, `flight.Fly` and meet rules as the server.
- **High ping:** when the rows for the view haven't arrived (ping above the cap), it flies past targets
  and waits for the server, as Overwatch does.
- **Replays:** frames carry their view tick and the rows are kept, so a replay reaches the same hits.

### 7. A rare miss never respawns the shot

- **What:** each shot gets a key (cast tick, ability, launch index), sent in `ShotState`. The client
  remembers the shots it ended on a target, and on reconcile drops server shots with those keys until
  the server's shot ends.
- **Needs:** a launch index on `LaunchIntent` and `Projectile`: the hook's launch order.
- **Damage and HP stay server-only,** so a disagreement looks like "impact, no damage" (as in
  Overwatch).
- **Rare case:** a target that died on the server after your view still stops your shot on your screen.
  The server's shot flies on, hidden from you, and can hit something behind.

### 8. Optional: an impact flash where your shot ended

The predictor already knows the contact point. Without the flash the shot simply stops on the target.

## Tuning

A new rewind tuning file: `{"max_seconds": 0.4}`. The predictor loads it too.

The rewind needed is ping + input wait (0 to 0.1 s) + interpolation delay (0.1 s). So 0.4 s covers pings
up to at least 200 ms. Overwatch stops predicting impacts at about 220 ms. Above the cap the server
clamps, and the client waits for the server.

## Left for later

- **Escapes** (i-frames, blink): the server refuses a rewound hit on a target that is escaping now.
  Comes with the first such ability.
- **Hidden targets:** rewind only the targets the shooter could see at that tick. Comes with the first
  hidden info.
- **Walls in the predictor** come with the map step. Until then, predicted shots pass walls.
- **Adaptive input pacing** shrinks the input wait, so the rewind (and the victim's cost) shrinks too.

## Cheating

The client picks its view tick, so a cheater can claim any view within the cap. Every favor-the-shooter
game accepts that bound. The server never rewinds past the cap, or to a tick it hasn't recorded.

## Tests

- `history.At`: lerp, a missing earlier row, targets that exist only later, the own Body excluded.
- Server, favor the shooter: bob walks out of the bolt's path. Alice's frames claim a view where bob was
  in it, so the bolt stops on bob's Body. Against bob's Echo it also deals damage. A view older than the
  cap is clamped (miss). A NaN view gives the current targets.
- Prediction: `TestPredictionMatchesServer` with a lagged view. The predictor, fed each tick's
  TargetSync, ends the bolt on the same tick as the server, on bob's Body and on bob's Echo. The
  recording is updated; `predictor.test.ts` checks the wasm against it.
- No respawn: a locally ended shot stays hidden while a sync still lists it (`prediction.test.ts`).
- Wire fixtures: `TargetSync`, `ViewTick`, the shot key.
- Mutation checks over the new paths.

## Build order

1. `rewind/history` core and its tests.
2. Server: `ViewTick`, `History` + record system, rewound `FlightSystem`, tuning. Server tests.
3. `TargetSync` and the shot key.
4. Predictor, wasm glue, client wiring (view tick, rows, suppression).
5. The prediction recording, docs (CLAUDE.md client sync), and removing the `DEBUG(shots)` logs.

New components and fields mean one `world purge`.

## Appendix: where things live

- Client interpolation: `client/src/engine/sync/timeline.ts` (`InterpolationDelay` 0.1 s, `sample`,
  `renderTick`) and `client/src/engine/render/entity_views.ts`. `client/src/main.ts` builds frames
  before computing the render tick.
- Server targets: `collectTargets` in `shards/game/internal/projectile/flight_system.go`. Body and Echo
  are one centered circle each (`shards/game/internal/player/spawn_system.go`,
  `shards/game/internal/echo/spawn_system.go`).
- Meet rules: `shards/game/content/projectile/tuning.json` (own and ally pass; Bodies stop shots;
  Echoes stop them and take the payload).
- Casts and launches: `shards/game/internal/ability/cast_hook_system.go` (`LaunchIntent{Cause, …}`
  from `out.Launches`) and `shards/game/internal/projectile/component` (`Projectile.Cause`).
- Input: `shards/game/internal/player/component/input.go` (`InputQueue.Applied`, `Last`, `LastTick`);
  `input_buffer_seconds` in `shards/game/content/player/tuning.json`.
- Predictor: `shards/game/internal/predict`, `shards/game/predictor/main.go`,
  `client/src/game/player/predictor.ts`.
- New: `shards/game/internal/rewind/…` and `shards/game/content/rewind/tuning.json`.
