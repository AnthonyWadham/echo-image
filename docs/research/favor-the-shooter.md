# Favor the shooter: shots stop where you see them hit

Built 2026-10-26, all seven steps. It replaces the earlier plan, where the server rewound every shot.

## Summary

- **The problem.** Your shots are predicted, so your screen draws them ahead of the server. Enemies
  are drawn about 0.1 s behind it. The server hits shots against enemies where they are now. So on
  your screen a shot flies through the enemy you see, then vanishes: about 3 units late locally (bolt
  15 units/s, 20 Hz), more with ping.
- **The fix: your screen finds the hit, and the server checks it.**
  - The predictor (the server's Go code, built to wasm) keeps a Box2D world of every Body and Echo,
    each placed where your screen drew it. It flies your shots through that world with `flight.Fly`,
    the cast the server uses.
  - When your shot stops on a Body or Echo, the input frame that did it carries a **hit claim**. Every
    frame carries its **view tick**: the render tick on screen.
  - The server keeps 0.4 s of each Body's and Echo's rows: where it was, and its tags. It checks a
    claim with Box2D: a pair cast at your view, a live cast for walls, and a check that nothing else
    came first.
- **Shots your Body casts stop on Bodies and Echoes only through accepted claims.** Walls and the map
  edge still stop them live. Echo casts stay live, as today.
- **A shot your screen ended stays hidden** until the server drops it. It never respawns.
- Damage, HP and effects stay on the server.

| Who | Decides |
|---|---|
| Your predictor | where your shot stops on a Body or Echo, at your view |
| The server, at your view | whether that holds: the path touches the target there, and nothing it can't pass came first |
| The server, now | walls and the map edge, damage, effects, deaths, Echo shots |

## Problems

1. **Overshoot.** A shot passes through the enemy on your screen and vanishes a few ticks later.
2. **Disagreement.** The server judges against where enemies are now, your screen against where they
   were. A hit you saw can be a miss on the server, and the other way round.
3. **Respawn.** Each body_sync rebuilds your shots from the server's. A shot your screen ended would
   come back while the server still had it.

## Changes

### 1. Every shot has a key

- **What:** a shot's key is the input frame `Seq` that cast it, the ability, and its launch index
  (its place among that cast's launches). `Cause` gains `Seq`. `LaunchIntent` and `Projectile` gain
  `Launch`. `ShotState` and the predictor's shots carry all three.
- **How:** `CastSystem` puts the applied frame's Seq in the cast's `Cause`. A cast no frame made (an
  Echo's) has Seq 0. `CastHookSystem` numbers each cast's launches.
- **Why:** the client knows its frames' Seqs for sure, while a frame's tick shifts when it arrives
  late. The ability keeps the key unique when two abilities launch in one frame.

### 2. Input frames carry a view tick and hit claims

- **What:** `InputFrame.ViewTick` (float64) and `InputFrame.Claims`. A claim is
  `{Seq, Ability, Launch, Target}`: this frame's step ended that shot on that Body or Echo.
- **How:** the client sends the last drawn render tick. The page builds frames before it draws, so
  that is what was on screen. Claims are made once, when the client first predicts the frame. Resends
  repeat them, and the server keeps the first copy it gets.
- **Why:** the view says which moment the screen showed. The claim says which shot hit what. One frame
  can end several shots.

### 3. Bodies and Echoes keep their recent rows

- **What:** every Body and Echo carries `TargetHistory`: its last 8 rows (0.4 s at 20 Hz), oldest
  first. A row is its tick, center, radius and tags.
- **How:** `projectile.HistorySystem` (Update, after `effect.ResolveSystem`, before the syncs) writes
  this tick's row, which is exactly what this tick's syncs show. Tags are read from the effect list
  after the tick, as the next tick's rules read them. The Body and Echo constructors add the
  component, and the system keeps as many rows as the tuning's window.
- **Why:** the server must know where a target was when you saw it. A component is snapshot-safe and
  dies with its entity, so a dead target can't be claimed. It lives in `projectile`, its only reader;
  a feature of its own comes with a second reader. It isn't called "rewind", because that is
  Echo-Swap's planned Echo rewind.
- **Cost:** one row write per target per tick.

### 4. One rule for "where the screen drew it"

- **What:** `flight.AtView`. A target at view v, between ticks a = floor(v) and a + 1, is its row at
  a + 1, placed partway from its row at a: `lerp(row a ?? row a+1, row a+1, v − a)`. A target with no
  row at a + 1 isn't there.
- **How:** it is the rule `EntityViews` draws with (`lerp(before ?? s, s, t)`), FMA-free, in shared
  code. The server runs it on a target's `TargetHistory`, and the predictor on its buffer of syncs.
- **Why:** both sides get the same bits from the same rows, so the predictor's hit is the server's.

### 5. Syncs carry what a target needs

- **What:** `BodyState.Tags`, and `EchoState.Owner`, `EchoState.Radius` and `EchoState.Tags`. A Body's
  owner is itself.
- **Why:** the predictor builds its targets from body_sync and echo_sync. These are the same values
  `TargetHistory` saves.

### 6. The server checks each claim

- **What:** a shot with a Seq flies against map kinds only. When its caster's frame applied this tick
  claims it, `FlightSystem` checks the claim. Accepted: the shot stops on the target now, and a
  payload rule becomes a `DamageIntent`. Rejected: the shot flies on. History is never rewritten.
- **How:** a claim is rejected when any of these holds:
  1. the view isn't a finite number, is more than 8 ticks old, or needs a row the server hasn't saved
     yet;
  2. the shot (by key, among the caster's) or the target (with rows at the view) doesn't exist;
  3. a pair cast (`box2d.ShapeCast`) of the shot's path this tick misses the target at the view; or
     the shot's rules pass the target, judged with its tags at the view;
  4. the live map cast stops the shot first;
  5. another Body or Echo that the rules don't pass comes first at the view. One live `OverlapShape`
     finds the candidates: a capsule along the path, grown by `max_target_speed` × the view's age.
     Each candidate is pair-cast at its row at the view. The caster's own Body is taken where it is
     now, which is where the predictor has it.
- **Ties** go by `Fly`'s order: the earlier rule, then the map, then the lower ID.
- **The same bits as the predictor, so no margin.** A world cast meets each circle in that circle's
  own frame: it moves the shot by `−center`, then pair-casts. The server's pair cast does exactly
  that. Doing it in world space instead gives other last bits: in 1,000,000 bolt casts grazing an
  Echo, hit or miss never changed, but 99% of the hit fractions did, and those decide who came first.
  In the target's own frame all of them matched. A margin would only add slack: a real mismatch (a
  late frame) moves a shot 0.75 units, not millimeters. `TestPredictionMatchesServer` guards the
  match.
- **Why:** server work grows with claimed hits, not with shots × ticks. Every check is still a Box2D
  query: a pair cast on a saved shape counts, and the CLAUDE.md rule is reworded to say so.
- **Cost:** per claim, one pair cast, one overlap, and one pair cast per candidate. Player shots now
  cast only the map each tick.

### 7. The predictor finds the hits

- **What:** the predictor keeps one Box2D world alive. Each Body and Echo gets a body when a sync first
  shows it, and loses it when the newest sync no longer does. Your Body is in it too, at your
  predicted spot, with your own tags.
- **How:**
  - It keeps a short per-tick buffer of the Bodies and Echoes in each body_sync and echo_sync: the
    last 0.4 s.
  - Each new frame places each target at the frame's view with `flight.AtView`, then flies your shots
    with `flight.Fly`, through a `flight.Targets` lookup built from its own shape → target map. A stop
    on a Body or Echo is a claim, and the shot ends.
  - A view more than 8 ticks old, or without both ticks' syncs (ping above the cap), places no
    targets. Then there is no claim, and your shot flies on.
  - Replays never make claims. Each body_sync resets the predictor and replays your unacked frames,
    and each replayed frame ends the shots it claimed, as the server will.
- **Why:** the same rows, the same rule and the same cast as the server, so the claim is the one the
  server accepts.

### 8. A shot your screen ended stays hidden

- **What:** the client keeps the keys of the shots it claimed. A key stays while an unacked frame
  claims it or the server still lists the shot. Each reset leaves those shots out.
- **Why:** a rejected claim (rare) can't bring the shot back. The server's copy flies on unseen and
  never claims again: it passes Bodies and Echoes, and stops at a wall or its range.
- **Its last tick:** `Step` returns each shot that ended with its `flight.End`: where, and the
  sweep's own part of the tick (`Until`). The client draws it with the engine's `blend` up to there,
  then hides it, so it touches what it hit. A body_sync in that tick keeps it. Others' shots come the
  same way, from `ProjectileSync` (`Ended`, `Until`).
- **A target that dies:** the hit's `Until` times the death (`combat.Died`). The Echo is sent once
  more with it and drawn with the same `blend`, so it goes as the shot reaches it. `flight.AtView`
  uses the rule too, so a shot fired at it later in that tick passes.

### 9. Content

- **What:** `rewind_seconds: 0.4` and `max_target_speed: 20` in the projectile tuning. The predictor
  loads them too.
- **How:** `max_target_speed` is the fastest a Body or Echo moves: the dash, 6 m in 0.3 s.
  `TestContentIDsLineUp` checks it covers every champion's move speed and every dash.
- **Why:** 0.4 s covers pings up to about 200 ms (view age = round trip + input wait + 0.1 s of
  interpolation). A landing (up to 6 m in one tick, when a phase ends inside a wall) isn't covered:
  a blocker that just landed can be missed, which only makes the check lenient.

### 10. Cleanup and docs

- Remove both `DEBUG(shots)` logs.
- CLAUDE.md: client sync and prediction, the Box2D rule, the shots section, system order, layout and
  dependencies.
- A new component and changed shapes (`Cause`, `Projectile`, `InputFrame`) need one `world purge`.

## Limits

- **Ping above the cap.** There is no live fallback. When your view is more than 0.4 s old, your shots
  can't hit Bodies or Echoes. Walls still stop them.
- **Late or dropped frames.** A frame applied a tick off from the prediction has its claim checked on
  a shifted path. Most claims still land, since a path that starts inside the target counts. Glancing
  ones are rejected: the shot stays hidden and does no damage. Adaptive input pacing (later) makes
  this rarer.
- **No walls in the predictor yet.** A claim behind a wall is rejected. Until walls come, your screen
  can show a shot stopping on a target behind a wall.
- **A target that died after your view:** the claim is rejected, and your shot stays hidden.
- **The victim pays.** A hit can land up to 0.4 s after they dodged on their screen.
- **Cheating.** A client may claim any view within the cap. The server still checks the real path,
  the walls now, and the blockers at that view.
- **Only this tick's path is checked.** A client can claim the target a tick after its shot passed a
  blocker. An honest predictor claims the blocker instead, so only a cheater gains.

## Tests

- **Server:**
  - an accepted claim stops the shot and damages an Echo;
  - a claim still hits at the claimed view after the target moved away;
  - a view older than the cap is rejected;
  - a wall in front is rejected;
  - an enemy Body in front of the claimed Echo is rejected, and so is one standing on it;
  - a player shot with no claim passes enemies;
  - Echo shots still hit live.
  - The tests that relied on live hits from player shots move to claims: damage carries its cast,
    point blank, a shot type's rules come first, rules see tags, and allies are passed.
  - Content: `max_target_speed` covers every champion and dash. Wire fixtures: the new fields.
- **Prediction:** `TestPredictionMatchesServer` gains bob walking across alice's second bolt, at a
  fractional view. The predictor's claim is the one the server accepts, on the same tick. The run
  grows so her first bolt still flies out its range. The recording is re-made with `-update`, and the
  wasm test checks against it, claims included.
- **Client:** claims ride on their frame; an ended shot stays hidden while a frame claims it or the
  server lists it, then is forgotten; ViewTick is sent; the wire types have the new fields.
- **Mutation checks:** break each new rule once and confirm a test fails.

## Build order

All seven are built, and each ended green.

1. The shot key: `Cause.Seq`, `Launch`, the `ShotState` fields.
2. Syncs (tags, the Echo's owner and radius), `TargetHistory` and `HistorySystem`, `flight.AtView`.
3. Frames: `ViewTick` and `Claims`.
4. Server: map-only player shots and the claim check, with the server tests moved and added.
5. Predictor: the target buffer and world, claims and replays, the wasm glue; the prediction test and
   its recording.
6. Client: the view tick, claims on frames, hidden ended shots, the echo feed; the client tests and
   build.
7. CLAUDE.md, this doc, the `DEBUG(shots)` logs; mutation checks.

## Appendix: where things live

- Shot flight and rules: `shards/game/internal/projectile/flight/flight.go` (`Fly`, `Targets`, `Hit`)
  and `shards/game/internal/projectile/flight/tuning.go`.
- Shot systems: `shards/game/internal/projectile/flight_system.go`,
  `shards/game/internal/projectile/launch_system.go`, and the new
  `shards/game/internal/projectile/history_system.go`.
- Shot components: `shards/game/internal/projectile/component/` (`Projectile`, `LaunchIntent`, the new
  `TargetHistory`).
- Casts: `shards/game/internal/ability/cast_system.go`,
  `shards/game/internal/ability/cast_hook_system.go`,
  `shards/game/internal/ability/component/ability_cast_event.go` (`Cause`).
- Input: `shards/game/internal/player/component/input.go`,
  `shards/game/internal/player/input_system.go`.
- Syncs: `shards/game/internal/player/body_sync_event.go`,
  `shards/game/internal/player/sync_system.go`, `shards/game/internal/echo/echo_sync_event.go`,
  `shards/game/internal/echo/sync_system.go`.
- Constructors: `shards/game/internal/player/spawn_system.go`,
  `shards/game/internal/echo/spawn_system.go`.
- Wiring: `shards/game/main.go`. Content: `shards/game/content/projectile/tuning.json`.
- Predictor: `shards/game/internal/predict/predict.go`, `shards/game/predictor/main.go`.
- Client: `client/src/main.ts`, `client/src/game/player/predictor.ts`,
  `client/src/game/player/prediction.ts`, `client/src/game/player/local_player.ts`,
  `client/src/game/player/input_stream.ts`, `client/src/game/player/input_command.ts`,
  `client/src/game/player/body_sync_event.ts`, `client/src/game/echo/echo_sync_event.ts`.
- Client drawing: `client/src/engine/sync/timeline.ts` (`InterpolationDelay`, `sample`, `renderTick`),
  `client/src/engine/render/entity_views.ts`.
- Tests: `shards/game/main_test.go`, `shards/game/testdata/`, and the `*.test.ts` files above.
