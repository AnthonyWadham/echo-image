---
paths:
  - "shards/game/shard/**"
---

# System registration ordering (`shard.go`)
- **The map plugin (`pkg/levelstate`) registers first,** so its system runs at Init and first in
  PreUpdate: the map state names is live before any system reads it (`TestMapIsLiveFirstInTheTick`).
- The physics plugin registers on `Init` (rebuild) and `PreUpdate` (step), so its events are ready
  before `Update` game logic runs.
- **System events are same-tick and cleared at tick end**, so the **emitter must be registered
  before the reader**: `player.InputSystem` (emits `BodyClaimed`) before `echo.SpawnSystem`,
  `ability.CastSystem` (emits `AbilityCast`) before `ability.CastHookSystem`, and
  `combat.DamageResolveSystem` (emits `Died`) before `echo.EliminationSystem`.
- **An intent's senders register before its queue's drain,** so it lands the tick it's sent:
  `ability.CastHookSystem` (sends `LaunchIntent`) before `projectile.LaunchSystem`; every
  `DamageIntent` and `HealIntent` sender (`projectile.FlightSystem`, `area.AreaSystem`) before
  `combat.DamageResolveSystem`; and every `EffectIntent` sender (`ability.CastHookSystem`,
  `echo.EliminationSystem`) before `effect.ResolveSystem`.
- **`TestSendersRunBeforeTheirReceivers` checks both orders,** and that each queue has one drain, with
  `queuetest.CheckOrder`. It reads each system's own file, so write each send and emit as a literal (`T{…}`) or a conversion
  (`T(x)`), give each drain a compare that is a named func or a literal (its first parameter is the
  type), and keep one system per file.
- **Every receiver sorts what it gets by a full key before it acts,** because send and emit order can
  change after a restore. `queue.Drain` sorts by the owner's compare (`LaunchSystem`'s sorts
  launches, so new shots get the same entity IDs). A system event's reader sorts too, unless every
  event touches its own entity and what it sends a drain sorts (`echo.SpawnSystem`,
  `ability.CastHookSystem`).
- **`player.InputSystem`, `ability.CastSystem`, then `player.MovementSystem`
  run in PreUpdate, registered before the physics plugin.** Movement moves each Body by the frame Input
  just applied, so a frame moves the Body in the same tick its `InputAck` is sent. Client prediction
  depends on this (`TestFrameMovesBodySameTickAsAck`). The predictor steps in the same order: input,
  cast gate, move, then the hooks and shots from where the Body moved. Effects a cast starts apply
  from the next tick.
  - Before the plugin, the step sees each Body where it moved to this tick.
  - `physics.NewPlugin()` builds the plugin first so `area.AreaSystem` can hold it. Where
    `world.RegisterPlugin` is called is what places its step.
- **`echo.ReplaySystem` runs in PreUpdate, after Movement and before the physics plugin,** so Box2D
  holds this tick's Echo poses when shots and areas query it (`TestShotMeetsEchoWhereItIsNow`). An
  Echo spawned this tick joins Box2D next tick.
- **`history.SceneSystem` runs in Update after `echo.SpawnSystem`, before `ability.CastHookSystem`,** so
  a just-spawned Echo is in this tick's scene (`TestShotMeetsAnEchoTheTickItSpawns`) and every judge
  after it reads one scene. Nothing between
  them moves a recorded thing. The scene is derived, never saved: shard.go hands the one `*past.Scene`
  to `history.SceneSystem` and to each judge (`projectile.FlightSystem`), as it does the physics plugin.
  That shared pointer is the one place the leaf rule bends.
- **Hooks, shots and areas run in Update:** `ability.CastHookSystem` → `projectile.LaunchSystem` →
  `projectile.FlightSystem` → `area.AreaSystem`. A Body can cast the tick it is claimed (its constructor
  gives it Abilities), and a shot launched this tick moves this tick, its path starting at the shooter.
- **`effect.ResolveSystem` runs last in Update.** What it adds applies from the next tick, and it
  drops what is over by then, so a whole tick reads one status.
- **Update writes the tick; PostUpdate reads it.** PostUpdate runs after every Update system, so no
  Update order can put a reader before a writer. A new writer goes in Update; a new recorder or sync
  goes in PostUpdate (`cardinal.WithHook(cardinal.PostUpdate)`), which `TestReadersRunInPostUpdate`
  checks from the systems a tick ran. System events (`Died`) still show there.
- **`history.RecordSystem` runs first in PostUpdate.** Each Body's and Echo's row is exactly what this
  tick's syncs show of it, its tags read for the next tick and its `Until` from its `Died`, so a
  predicted shot meets what its client drew. Judges run in Update, before it, so a view can use rows
  up to the tick before (`TestHistoryIsWhatSyncsShow`).
- **Sync systems come next** (`player`, `echo`, `ability`, `projectile`, then `history.SyncSystem`), so
  each sends the tick's final state. **`combat.DeathSystem` runs last**, the one PostUpdate system that
  changes the world: it removes what died this tick, once each sync has sent it.
