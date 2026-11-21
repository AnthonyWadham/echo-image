---
paths:
  - "shards/game/internal/netsync/**"
  - "shards/game/internal/predict/**"
  - "shards/game/predictor/**"
  - "shards/game/internal/history/**"
  - "shards/game/internal/player/**"
  - "shards/game/**/sync_system.go"
  - "shards/game/test/prediction_test.go"
  - "client/src/**"
---

# Client sync (`netsync`)
- **Events are the only way to push data to clients.** Cardinal has no client queries (removed in
  world-engine #915) and no automatic component sync.
- **Each feature sends its own sync event** from its own `SyncSystem`, registered last:
  `player.BodySync`, `echo.EchoSync`, `ability.AbilitySync`, `projectile.ProjectileSync` and
  `history.TargetSync`. Each goes to every player with `w.SendTo` every tick and is complete — an
  entity missing from it is gone or hidden — so joining or reconnecting needs nothing else. Clients
  interpolate each stream on `Tick * TickSeconds`. A new synced feature adds its own event and
  `SyncSystem`; `netsync` doesn't change.
- **One rule draws and judges every view.** View v is drawn from the states at ticks ⌈v⌉ − 1 and ⌈v⌉,
  v − (⌈v⌉ − 1) of the way through the newer one's tick, so a whole tick is its own end; without both,
  nothing is drawn (`Timeline.sample`, then `blend`). The server and the predictor place targets by the
  same rule (`past.RowTicks`, `past.AtView`, and `past.Across` for a tick of views), and
  `testdata/view.json` pins the two sides together, to the bit (`TestViewFixture`,
  `entity_views.test.ts`). The render clock draws the window's `interpolation_seconds`
  (`Timelines.interpolationSeconds`) behind the newest tick every timeline has, never past the tick
  before it: in a stall it waits there. It never steps back, and eases by time, not by frame
  (`Timelines.renderTick`). So the views a frame's shots fly across always have their rows, in the
  predictor and on the server.
- **An entity that ends partway through a tick is sent once more, where and when it ended.** A shot
  comes in `ProjectileSync` on the tick it ended, `Ended`, at its end with `Until` (the part of the tick
  it lived), through `netsync.Items.AddGone`. The engine's `EntityViews` draws every entity with
  `blend`: one with `Until` below 1 moves at its own pace to its end, then goes, so it touches what it
  hit. An Echo killed in a tick comes in `TargetSync` the same way, with `Until` from its `Died`, so it
  goes when the shot reaches it, and in `EchoSync` at HP 0. `past.AtView` follows the same rule, so your
  predictor drops it then too. Add `Until` to another entity's state when it needs the same.
- **A shot that ended on a Body or Echo ends on it on every screen.** The hit was judged where the
  shooter's view had the target, but the victim's screen draws their Body ahead, and others draw it
  later. `flight.Hit.Offset` is the end from the target's center as judged; `ProjectileState` sends it
  (`OnTarget`, `TargetSerial`, `OffsetX`, `OffsetY`), and so does the predictor for your own shots. Each
  screen ends the shot at the target where it draws it this frame, plus the offset (`endOn`, from
  `EntityViews.at`; shots draw after Bodies and Echoes). A target the screen doesn't draw keeps `X, Y`.
- **Every Body and Echo is drawn from one timeline, `LocalPlayer.targets`** (each `TargetSync`). Each
  kind's views pick their rows by kind (`EntityViews` `shows`), so a new kind of target is one more view,
  with no new timeline or sync. Your own things are drawn as predicted by serial alone
  (`LocalPlayer.drawnOwn`: your Body's from the same tick's body_sync, your Echo's `You.EchoSerial`),
  never by kind, owner or EntityID: an older Echo of yours, or a newer Body that took your ID, is drawn
  from its rows.
- **A shot says where its flight this tick began** (`PrevX`, `PrevY`: its launch point on its first
  tick), in `ProjectileState` and `ShotState`. Every screen draws its tick from there, never from an
  older sync (`Placed.PrevX`), so a new shot leaves its launcher. Your predictor keeps it on a reset, so
  your shot flies on when nothing replays.
- **An entity's serial names it across ticks; its EntityID only within one tick.** Cardinal gives a
  freed ID out again at once, so a shot launched the tick after another ended takes its ID. Every
  constructor stamps `ref.Spawn{Serial}` from the world's one counter (`ref.Next`, saved with the world,
  from 1), and `TargetState`, `ProjectileState`, `BodyState` and `EchoState` carry it. `EntityViews`
  keys views and blends by `Serial`, the predictor keys each thing's rows by it, and a hit names its
  target's (`flight.Hit.Serial`). Links (`You.Body`, `Caster`, `Owner`) stay EntityIDs. `You.Body` is
  same-tick, but a shot's `Cause.Caster` and a thing's `team.Owner` are stored for its life, so a
  stale one can name a newer entity: F22 in `docs/research/foundation-review.md` (generational IDs
  in Cardinal, outside this repo).
- **Each thing's state is sent once.** A Body's or Echo's row in `TargetSync` is its only spot, size,
  team, owner, tags and `Until`. `BodyState` and `EchoState` send only what rows lack (player, look,
  HP), joined to the row by `Serial`. Your own row comes in `You` too (`Team`, `X`, `Y`, `Radius`),
  which your predictor starts from.
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
  sight; today everything is seen). Lists are built per team, since allies share vision. An item goes
  to a team only if it sees the entity and every entity the state gives away, which `Add` lists (a
  shot's caster; at its end, its target). A gone one (`AddGone`) is judged by where it last was
  (`netsync.Last`). seenBy follows `team.Owner` itself: an invisible Body's Echo replays its path.
- **Cost.** Cardinal writes one stream message per event, so each synced feature adds one message per
  player per tick. Serialization repeats once per player; bandwidth equals a broadcast.
- **Player IDs are only trustworthy with Argus auth.** DEV auth trusts the `X-Player-ID` header.
- **Clients predict their own Body with the server's own code.** Each tick a client sends
  `Input{Session, Frames}` with its recent unacked frames — each frame is move, aim, buttons and the
  render tick on screen (`ViewTick`).
  `shards/game/predictor` builds `predict` (and through it `motion.Step` and world-engine's `box2d`) to
  wasm with the server's Go toolchain. Each frame the client sends runs one tick of it at once, in the
  server's order: the cast gate, the move, each cast's hook, your shots, then the effects your casts
  put on you. The predictor keeps its state after each frame. A `BodySync` of your session whose state
  is what your frame `You.InputAck` left (`predict.Agrees`: everything, your Echo up to your newest
  frame, and no frame since stepped without its view's target syncs) changes nothing, but your Echo's
  path takes the server's. Any other restarts it from your Body and `You` (`MoveSpeed`, `Champion`,
  `Cooldowns`, `Effects` with `EffectsNext`, never the folded status, `Shots`, and `EchoDelay` with
  `EchoPath`) at `Tick`, and replays the frames newer than `You.InputAck` (when `InputSession` is its
  own). So a clean link replays nothing, and a body_sync costs a compare, not a replay of every
  unacked frame. A replay that lands elsewhere is a correction: your Body, Echo and each shot (by key) keep the
  path they're drawn on and fade to their new spots over the champion's `fade_seconds`, or jump past
  its `snap_seconds` of move speed (`content/player/tuning.json`, sent in `YouState`), and a new shot
  starts as far off as your Body (`client/src/game/player/prediction.ts`). The HUD counts replays and
  corrections.
  - **Every input is predicted:** a shot leaves the frame you click, from where that frame moves your
    Body, a dash starts the tick after, and the HUD starts a cooldown at once.
    A client draws its own predicted shots from the predictor and the rest from `ProjectileSync`,
    which leaves a player's own predicted shots out (Unreal's COND_SkipOwner): `You.Shots` carries
    them for the predictor. `LocalPlayer.you` is undefined before the first body_sync, since entity 0
    is a Body too.
  - **Your Echo is predicted too,** so the gap between it and your Body on screen is the real one. Its
    next spots are already in the server's `PathHistory` ring (Echo(T) is Body(T − delay)), so
    `You.EchoPath` sends your Echo this tick, then its spot on each next tick, up to
    `echo_path_seconds` (`content/player/tuning.json`) or its delay, and `You.EchoDelay` the delay. The
    predictor walks the path a spot per step (`Predictor.Echo`). Past the delay, your Echo is where it
    predicted your Body that many steps back; past the path but inside the delay, it waits at the
    path's end. The client draws your Echo from it (`LocalPlayer.drawnOwn`), others' from their rows.
    The tick your Echo dies, `EchoPath` is empty, since it is gone the next.
  - **Your shots stop where your screen shows them hit, as the server judges them.** A target's row
    (`past.Row`: kind, side, circle, tags and `Until`) is its one record: its `History` keeps its
    rows, and `TargetSync` sends each tick's, which is all your predictor reads to place targets. The
    predictor keeps as many ticks of them as the server keeps rows (`Window.KeptRows`), in a ring per
    serial (`See`), and builds the same `past.Scene` on a step with a shot, holding only
    targets that lived all of the newest sync's tick: one that died in it is gone on the server before
    your next frame. The render clock waits for `TargetSync` like any timeline
    (`LocalPlayer.targets`), and it feeds the predictor only once a body_sync says the shard runs your
    sim. Shot code names no target kind: your Body (its ID) and your Echo (`You.EchoSerial`) are
    predicted things in the scene. Each step meets them across its `ViewTick` and the next
    (`past.Across`, from the rule `EntityViews` draws with), held by the server's rule
    (`past.Window.Clamp`), and your Body and Echo moving from where they were a step ago to where they
    are now, as your screen draws them (with their rows' tags). A shot that stops on one ends there, as it will on the server. `Step` returns the shots that ended (`predict.Ended`, with their `flight.End`),
    drawn with the same `blend` as everyone's. A replay after each body_sync is the same `Step`, so it
    ends a shot where the step did; a shot the server flew on comes back from `You.Shots`, and the end
    your frame drew for it is dropped (by key, when nothing replays). Views the predictor has no syncs
    for (frames over the window ahead of your newest sync: a high ping or a deep queue) pass
    Bodies and Echoes, while the server may still hit there.
  - **The predictor embeds `content.FS`** and loads it with `predict.Load`, through the shard's own
    `content.Load`.
  - **A client predicts only for a shard of its own build.** `echoimage.SimVersion` is the Go release,
    the build tags (from the binary's build info; `rate60` picks 60 Hz) and a hash of what the shard and
    the predictor build from: `go.mod`, `go.sum`, `shards/game`'s code, content and predictor, and
    `pkg/level` and `pkg/levelstate`, without tests. The shard hashes the copy it embeds (its Docker build has no `.git`) and sends it as
    body_sync's `SimVersion`.
    `predictor.mjs` stamps the same into the wasm (`-X main.simVersion`), which never embeds the
    source, so players don't download it. When they differ, `LocalPlayer` doesn't play and the HUD
    says to reload (in dev, restart `bun run dev` first). `TestSimVersionCoversWhatTheSimIsBuiltFrom`
    checks every file the shard and the predictor build from is hashed, testdata isn't embedded, and no
    `replace` points at a local directory.
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
    replays. A second predictor there replays only when `Agrees` says no, lands where they do every
    tick, and never replays without a surprise; the main run replays only after its two. `TestPredictedEchoMatchesServer` checks your Echo over ten frames from one state, and
    `TestShotMeetsYourEchoWhereYouDrawIt` a shot that stops on it.
    Corrections still come from what the client can't know: a late frame (the server repeats the last
    one, at a view moved on), effects others cause, and a target someone else kills first. Pacing
    keeps late frames rare.
  - The predictor walks the map body_sync names (`MapID`). `predict.Load` builds every map
    `content/arena` lists, as the shard's map plugin does, and `Reset` takes the state's. A shard
    names only maps its build carries, and SimVersion pins those, so `Reset` panics on any other.
    The recording meets wall 1 (her dash, a phase and a landing), wall 2 (an outline her first bolt
    ends on) and the edge (her slide over its joints, a bolt on it), on its own map (`recordedMap`).
  - The client aims from where the frame moves its Body, since the server casts from there.
- **The server queues frames and applies exactly one per Body per tick**, in Seq order
  (`player.InputSystem`, `player/component.InputQueue`). Duplicates and applied frames are dropped, and
  a Body with no input for `input_timeout_seconds` stops. One step per tick keeps the Body, and the
  Echo replaying it, smooth when frames arrive late or bunched. A new `Session` (a restarted client)
  starts the stream over.
- **The client paces its frames by the server's queue** (Overwatch's time dilation).
  `YouState.InputQueued` is how many frames wait for later ticks, and `YouState.InputTarget` how many
  the server wants waiting. `InputStream` averages the queue, in seconds, over 0.5 s, and runs its
  frame clock faster below the target and slower above, in proportion, by at most 10% (an empty queue
  gets the whole 10% at any rate). So the queue neither starves (a coast) nor grows (latency). This
  also absorbs a server or client clock that runs a few percent off.
- **The server sets the target from coasts** (`InputQueue.Target`, `TargetAt`). It starts at a tick.
  Each time a Body's queue runs dry while its client plays (the first dry tick after an applied frame),
  it rises `queue_step_seconds`, up to `input_buffer_seconds`. It falls that much for each
  `queue_calm_seconds` without one (`content/player/tuning.json`), worked out when read, so a calm Body
  costs no write. A clean link keeps a tick of delay; a jittery one buys only the margin it needs.
  `input_buffer_seconds` (0.5 s) is otherwise only an abuse cap: past it the oldest frames drop.
- **An Input carries at most the queue's frames** (`YouState.InputCap`, `input_buffer_seconds` of
  ticks). `InputSystem` drops a bigger one whole, before it claims a Body (Source's "too many cmds"),
  so merging an Input costs O(cap²), never O(frames²). The client sends its newest 0.4 s of unacked
  frames, at most `InputCap`, and one before the first body_sync says the cap.
- **Input trouble is counted, never logged** (`InputQueue.Health`, `player.InputMetrics`). Coasts,
  frames dropped past the cap, oversized Inputs and floored views add to the Body's counts, which
  `YouState.InputHealth` shows on its HUD, and to the shard's otel counters (`echo.input.*`, plus a
  queue-depth histogram) on the meter Cardinal installs. No per-player labels: unbounded.
- **Systems act on input through `InputQueue.Applied(tick)`**, so each frame acts once: movement sets
  velocity from it, abilities cast from it. Buttons ride in the frame (like a Source or Overwatch
  user command), so a shot leaves from where the client predicted its Body when it fired.
- **Cooldowns are server ticks** (`Cooldown.ReadyTick`), never a client count, so a client can't speed
  them up. Its predicted kit reads them from `You.Cooldowns`.
