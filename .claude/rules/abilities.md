---
paths:
  - "shards/game/internal/ability/**"
  - "shards/game/internal/effect/**"
  - "shards/game/content/ability/**"
  - "shards/game/content/effect/**"
  - "shards/game/content/player/**"
---

# Abilities and their effects
- **A champion is an ID, not a type.** A Body's `player/component.Champion` names the champion it
  plays, and each feature keeps that champion's numbers under the ID: `champions` (move speed, radius)
  in `content/player/tuning.json`, `kits` in `content/ability/tuning.json`, and its Echo (delay, area)
  in `content/echo/tuning.json`. A new champion is JSON entries, never new code.
- **The cast gate decides whether a cast happens.** A Body casts from its champion's kit.
  `cast.Gate`, which `ability.CastSystem` and the predictor both run, checks the button, cooldown and
  aim, and that the Body has no tag the ability is blocked by (its own `blocked_by`, or the file's: `stunned`, `out`), then starts the cooldown; `CastSystem` emits
  `AbilityCast{Cause, Aim}`. Cooldowns are kept per ability ID, so a cooldown
  follows its ability into whichever slot holds it.
- **What an ability does is its hook.** Every ability row names a hook in `ability/hooks` and gives
  its `params` (`{"hook": "launch", "params": {"shot": "bolt"}}`). A hook reads its `Cast` and params
  and appends requests to `Out` (`Launches`, `Effects`). `cast.Run`, which the server and the predictor
  both call, runs it and hands each request to a `cast.Sink`: launches numbered within the cast, from
  the caster, and effects as intents from it. `ability.CastHookSystem`'s sink sends them as intents with
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
- **Timed effects (stun, slow, pass, forced move) belong to `effect`.** A feature sends
  `EffectIntent{Target, Kind, Ticks, Value, VelX, VelY, Source}` to effect's queue; what each kind does (tags, passes,
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
