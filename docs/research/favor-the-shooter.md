# Favor the shooter: shots stop where you see them hit

Built 2026-10-26 with hit claims. Rebuilt 2026-10-31 as server rewind, after a review found holes in
the claim model (problem 3).

## Summary

- **The problem.** Your shots are predicted, so your screen draws them ahead of the server. Enemies
  are drawn about 0.1 s behind it. If the server hit shots against enemies where they are now, a shot
  on your screen would fly through the enemy you see, then vanish: about 3 units late locally (bolt
  15 units/s, 20 Hz), more with ping.
- **The fix: the server judges your shot where your screen drew its targets (server rewind).**
  - Every input frame carries its **view**: the render tick on screen.
  - The server keeps 0.4 s of each Body's and Echo's rows: where it was, and its tags.
  - Each tick, the server flies each of your shots with `flight.Fly`: the map now, and each Body and
    Echo as your views drew it across the tick, from its rows (`flight.Across`).
  - Your predictor (the server's Go code, built to wasm) flies the same shot with the same code, from
    the same rows: the same end, to the bit.
- **The client sends no hits.** The server finds every hit itself, every tick.
- Echo shots stay live. Damage, HP, effects and deaths stay on the server.

| Who | Decides |
|---|---|
| Your predictor | where your shot ends, at your view, so it shows at once |
| The server, at your view | where your shot ends on a Body or Echo: the same answer |
| The server, now | walls and the map edge, damage, effects, deaths, Echo shots |

## Problems

1. **Overshoot.** A shot passes through the enemy on your screen and vanishes a few ticks later.
2. **Disagreement.** The server would judge against where enemies are now, your screen against where
   they were. A hit you saw could be a miss on the server, and the other way round.
3. **The first build trusted claims, and claims had holes.** The client sent "my shot hit X", and the
   server checked only that, on that tick's path:
   - a late sync let an honest claim pass a Body that blocked it;
   - a lost or late frame lost the hit;
   - past 0.4 s of view age, no shot could hit at all;
   - a rejected claim's shot stayed hidden while it flew on, unseen, on the server.

   Mature engines don't take a hit list from the client. Source (lag compensation) and Overwatch
   rewind the targets to what the shooter saw and run the server's own query. This rebuild does that.
4. **The screen and the judge picked rows by two rules** (fixed 2026-11-01). In a stall the screen
   held its newest state, while the predictor placed nothing there, so a shot passed what you saw.
   After the stall the render clock stepped back. At a whole tick the two read different rows, and
   nothing compared them.

## Changes

### 1. Every shot has a key (unchanged)

- **What:** a shot's key is the input frame `Seq` that cast it, the ability, and its launch index.
  `Cause` has `Seq`; `LaunchIntent` and `Projectile` have `Launch`. `ShotState` and the predictor's
  shots carry all three. `cast.Run` numbers a cast's launches for the server and the predictor alike.
- **Why:** the client knows its frames' Seqs for sure, while a frame's tick shifts when it arrives
  late.

### 2. Input frames carry a view

- **What:** `InputFrame.ViewTick` (float64), and no claims.
- **How:** the client sends the last drawn render tick. The page builds frames before it draws, so
  that is what was on screen. The render clock never passes the newest tick every sync has: in a stall
  the screen waits there. It never steps back, and eases by time, not by frame, so every refresh rate
  draws the same. So a view's two rows are always in the predictor and on the server.
- **Why:** the view says which moment the screen showed.

### 3. Targets keep their recent rows

- **What:** every target (each Body and Echo) carries `TargetHistory`: its last 9 rows (`KeptRows`),
  oldest first: those every view 0.4 s back (8 ticks at 20 Hz) is drawn from. A row (`flight.Row`) is
  its tick, kind, side (owner and team), center, radius, tags and `Until`. It is the target's one
  record: the server judges by it, and the target sync sends it.
- **How:** `projectile.HistorySystem` (Update, after `effect.ResolveSystem`, before the syncs) writes
  this tick's row, which is exactly what this tick's syncs show, with `Until` from `Died`.
- **Why:** the server must know where a target was when you saw it. A component is snapshot-safe and
  dies with its entity.

### 4. One rule for "where the screen drew it"

- **What:** view v is drawn from the rows at ticks b − 1 and b, for b = ⌈v⌉: it is v − (b − 1) of the
  way through tick b, so a whole tick is its own end. A target is its row at b, placed partway from its
  row at b − 1, or as it is when new at b. Without both ticks, nothing is drawn or placed.
  `flight.RowTicks` picks the ticks and `flight.AtView` places it; the client's `Timeline.sample` and
  `blend` do the same.
- **How:** `testdata/view.json` (`TestViewFixture`) holds targets over a few ticks and where views
  across them place each, as Go computes it. The client's `EntityViews` must draw them there, to the
  bit.
- **Why:** FMA-free shared code gives both sides the same bits from the same rows, and the fixture
  fails the side that drifts.

### 5. One target sync

- **What:** `projectile.TargetSync` sends each player every target it may see, each as this tick's row
  (`TargetState`), every tick. Your predictor reads only it to place targets.
- **How:** `projectile.TargetSyncSystem` runs after `HistorySystem`, with the other syncs, through
  `netsync`. The client's render clock waits for it like any timeline (`LocalPlayer.bodyRows`, `echoRows`), so the
  predictor has the rows of every view you draw. It feeds the predictor once a body_sync says the shard
  runs your sim.
- **Why:** the predictor gets the server's own rows, with no field rebuilt from body_sync and echo_sync,
  and one stream to wait for. A new kind of target needs no predictor, glue or shot code: a thing is a
  target because it keeps rows, and `Fly`, `Across`, `seenAt` and the predictor's `sights` use the kinds
  the rows name. Only the own-Echo rule names a kind, since your predictor places your Echo itself. A
  row is one circle: a target of another shape first gives `Row` a shape (F19).
- **Cost:** each target's spot is sent twice a tick, in its own sync for drawing and here.

### 6. The server flies predicted shots at their client's view

- **What:** a shot an input frame cast is `Projectile.Predicted`. Each tick `FlightSystem` flies it
  with `flight.Fly` at its caster's view. Any other shot (an Echo's) flies at this tick's views, as
  every screen draws it.
- **How:**
  - The view is the frame applied this tick. On a tick with no new frame, it is the last one moved on
    by the ticks since (`InputQueue.View`), as the client keeps drawing.
  - A view never goes back. A frame's view below the largest one applied, or NaN, is taken at that one
    (`InputQueue.ViewFloor`, from 0, kept across sessions). Your render clock never steps back, so an
    honest view never trips it, and the predictor has no copy of it.
  - `flight.Tuning.View` holds it to the views whose tick's views the rows kept are drawn from: no
    older than 8 ticks, no newer than the tick − 2, whose views run to the newest row. An older view (a
    high ping) is judged at the oldest: you must lead your shots, but you still hit. NaN and the
    infinities can't leave the window.
  - The map is cast now. It is never rewound.
  - One query of `flight.Reach` finds the Bodies and Echoes that could be on it at the view: a Box2D
    tree (`box2d.DynamicTree`) built each tick, one box per target around its kept rows and where it is
    now. There is no speed bound: a dash, a blink or a landing stays inside its box (2026-11-01). Each
    is pair-cast in its frame as it moves across the views v to v + 1 (`flight.Across`): a frame's
    shots fly a tick while its screen's view moves a tick (F2, 2026-11-01). The caster's own Body and
    Echo move from where they were a tick ago to where they are now, as its predictor has them and its
    screen draws them. The Echo keeps its rows' tags, which both sides have.
  - The first thing met ends the shot, in `Fly`'s order: the nearer, then the earlier rule, then the
    map, then the lower ID. A payload becomes a `DamageIntent`. History is never rewritten.
- **The same bits as the predictor, so no margin.** Both run `flight.Fly` with `flight.Across` from
  the same rows. `TestPredictionMatchesServer` guards it.
- **Why:** the server's own query decides every hit, on every tick's path, so no hit hangs on a
  message arriving, and blockers are always checked.
- **Cost:** per shot per tick, one map cast, one tree query, and up to two pair casts per Body or
  Echo found. Building the tree is O(T·W) a tick for T targets keeping W rows,
  as `HistorySystem` already pays.

### 7. The predictor flies your shots the same way

- **What:** the predictor keeps one Box2D world with a body for each target that lived all of the
  newest target sync's tick. One that died in that tick is gone on the server before your next frame,
  so the predictor drops it at once.
- **How:**
  - It keeps as many ticks of target sync rows as the server keeps (`KeptRows`).
  - Each step places each target at the frame's view, held by the same `View` rule, with `AtView`,
    and your Body and Echo where it predicts them. Then it flies your shots with `flight.Fly`.
  - A view without both ticks' syncs places no targets, and your shot flies on.
  - A replay after a body_sync is the same step, so it ends a shot where the first step did.
- **Why:** the same rows, rule and cast as the server.

### 8. Your shots' ends are drawn as they happen

- **What:** `Step` returns each shot that ended with its `flight.End`: where, and the sweep's part of
  the tick (`Until`). The client draws it with the engine's `blend` up to there, then hides it. Others'
  shots come the same way, from `ProjectileSync` (`Ended`, `Until`).
- **On every screen, an end on a target is drawn on it** (2026-10-31). The server judged it where your
  view had the target, but the victim's screen draws their Body ahead, and third screens draw it later.
  `flight.Hit.Offset` is the end from the target's center as judged. `ProjectileSync` sends it with
  `OnTarget` and `TargetSerial`, and your predictor gives it for your own shots. Each screen ends the
  shot at the target where it draws it that frame, plus the offset (`endOn`), so everyone sees it touch.
- **When the server disagrees:** each body_sync rebuilds your shots from the server's, so a shot the
  server flew on comes back on the server's flight, and the end your frame drew for it is dropped (by
  its key). That is a correction, and rare.
- **Corrections fade per thing** (2026-11-01). Your Body, your Echo and each shot (by key) keep the
  path they're drawn on through a body_sync, then fade to their true spots as your Body always did;
  past the champion's snap line (`snap_seconds` of its move speed, 3 units for the starter) they jump. A new shot starts as far off as your Body, so it leaves where your Body is
  drawn. An ending shot keeps its drawn start but ends where it ended. A shot drawn ending that flies
  on starts on the server's flight, since its drawn path stopped partway through the tick.
- **A target that dies:** the hit's `Until` times the death (`combat.Died`). The Echo is sent once
  more and drawn with the same `blend`, so it goes as the shot reaches it. It is removed at that tick's
  end, so no later shot meets it, on either side.

### 9. Content (unchanged)

- **What:** `rewind` in the projectile tuning, a budget (2026-11-01): `interpolation_seconds` 0.1 +
  `queue_seconds` 0.05 + `ping_seconds` 0.1, so 0.25 s (it was a flat 0.4 s). The predictor loads it
  too, and the client's render clock draws `interpolation_seconds` behind.
- `max_target_speed` is gone (2026-11-01): each target's rows bound how far it moved, so no speed
  limit has to cover every move, and no test copies the dash formula.

### 10. One world purge

- `Projectile` gains `Predicted` (and `Prev`, its start-of-tick point, 2026-10-31), and `InputFrame`
  (kept in `InputQueue`) loses `Claims`, so running this needs one `world purge`.

## Limits

- **Ping above the window.** A view older than the rewind (0.25 s) is judged at the oldest row. When
  your frames run over the rewind ahead of your newest sync (a high ping or a deep queue), that row is newer than any
  sync your predictor has, so it can't show those hits ahead: they come with the next body_sync.
- **Late frames.** A frame applied a tick late moves the server's shot a step from the predicted one.
  The server still judges every tick, so a late frame shifts a hit; it doesn't drop it. Input pacing
  keeps late frames rare.
- **No walls in the predictor yet.** A shot the server stops at a wall flies past it on your screen
  until a body_sync shows it gone, about a round trip later.
- **A target someone else kills first.** Your screen shows your hit; the server's shot flies on. The
  next body_sync corrects it.
- **The victim pays.** A hit can land up to the rewind (0.25 s) after they dodged on their screen.
- **Cheating.** A client may lag as far back as the window, but its view never goes back, so it can't
  try each past moment for each shot. It gains at most the rewind, as an honest player at the favored
  ping does.
  The server still runs its own query: the walls now, the blockers at that view.

## Tests

- **Flight:** `View` holds views to the rows kept, NaN and infinities included, and every view in
  (a, a + 1] is drawn from rows a and a + 1 (`RowTicks`). `Across` shows a tick of views as they draw
  it. `Fly` meets each target as the tick shows it, over named cases (moved off or onto the path,
  blinked 40 units off it, the nearer first, allies passed, a wall first, a Body on its Echo, a last
  tick, crossing the path, leaving it, a dash across it) and a sweep and fuzz of moving targets, each met
  when the two first touch. A target is met where the tick showed it, at its size there, however far it
  went since (`TestFlyMeetsTargetsWhereverTheyWent`).
- **Server:** a hit where the view had the target after it walked off, which his own sync ends on him
  (`OnTarget`, the offset from where the view had him); partway between two rows; an enemy Body in
  front at the view blocks; a Body on its Echo shields it; a view too old, too new or not a number is
  held and still hits; an older view than one already sent is judged at that one, in a new session or
  after NaN too; a wall in front stops the shot, on no target; Echo shots hit live; allies are passed; a
  shot that stops on your own Echo (test content) meets it where you draw it, now, on the server and in
  your predictor alike. Each target's newest row is what its syncs show, kind, side and death time
  included, and `TargetSync` sends exactly that row (`TestTargetHistoryIsWhatSyncsShow`,
  `TestKilledEchoLastsUntilTheShotReachesIt`).
- **Prediction:** `TestPredictionMatchesServer` checks each step's and each replay's ends against the
  server's `ShotEnded`, bit for bit, with bob walking across alice's second bolt at a fractional view.
  On a bad network (lost, held and bunched Inputs, a full queue dropping frames), each replay of
  several frames lands on the server's bits, its ends included, once its frame applies, unless the
  server coasted or dropped a frame first; a correction is at most a tick of walking per such surprise.
  `TestPredictorPlacesOnlyWhatSyncsShow` covers missing syncs, a target gone or dead, and views held by
  the window from each step's own tick, its oldest row included. The wasm build checks the same run,
  those replays included. All of it runs at 20 and 60 Hz (`-tags rate60`), with a recording each.
- **Client:** frames carry the view; `TargetSync` feeds the predictor only once the shard runs your sim,
  and the render clock waits for it; replays end shots like steps; a shot the server flew on shows;
  through a correction your Body, Echo and shots stay where they're drawn and fade together, a new shot
  leaves your drawn Body, and far ones jump;
  only your predicted shots are drawn from the predictor, and none before the first body_sync; an end
  on a target is drawn on it where the screen draws it (`endOn`, `EntityViews`, `Prediction.shots`);
  the render clock waits at the newest tick every timeline has, never steps back, and runs alike at 60
  and 240 Hz; `EntityViews` draws `testdata/view.json`'s targets where Go placed them, to the bit.
- **Mutation checks:** each new rule broken once fails a test (`go run ./scripts/mutate`, list in
  `scripts/mutate/mutants.json`).

## Appendix: where things live

- Shot flight and rules: `shards/game/internal/projectile/flight/flight.go` (`Fly`),
  `shards/game/internal/projectile/flight/view.go` (`Row`, `AtView`, `Across`, `View`, `Fly`),
  `shards/game/internal/projectile/flight/tuning.go`.
- Shot systems: `shards/game/internal/projectile/flight_system.go`,
  `shards/game/internal/projectile/launch_system.go`,
  `shards/game/internal/projectile/history_system.go`,
  `shards/game/internal/projectile/target_sync_system.go` (`TargetSync` in `target_sync_event.go`).
- Shot components: `shards/game/internal/projectile/component/` (`Projectile`, `LaunchIntent`,
  `ShotEnded`, `TargetHistory`).
- Input: `shards/game/internal/player/component/input.go` (`InputFrame`, `InputQueue.View`).
- Syncs: `shards/game/internal/player/sync_system.go`, `shards/game/internal/projectile/sync_system.go`,
  `shards/game/internal/echo/sync_system.go`.
- Predictor: `shards/game/internal/predict/predict.go`, `shards/game/predictor/main.go`.
- Client: `client/src/main.ts`, `client/src/engine/sync/timeline.ts`,
  `client/src/engine/render/entity_views.ts`, `client/src/game/player/predictor.ts`,
  `client/src/game/player/prediction.ts`, `client/src/game/player/local_player.ts`,
  `client/src/game/player/input_command.ts`, `client/src/game/projectile/projectile_sync_event.ts`,
  `client/src/game/projectile/target_sync_event.ts`.
- Tests: `shards/game/*_test.go`, `shards/game/internal/projectile/flight/view_test.go`,
  `shards/game/testdata/`, and the client `*.test.ts` files above.
