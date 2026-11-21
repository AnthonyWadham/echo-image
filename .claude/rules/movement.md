---
paths:
  - "shards/game/internal/player/**"
  - "shards/game/internal/physics/**"
---

# Movement and walls
- **Bodies are `BodyTypeManual`, and `player.MovementSystem` moves them itself.** Box2D never pairs a
  kinematic or Manual body with a static or kinematic one, so its step can't stop a Body.
- **One tick's move is `motion.Step`,** which the server and the client predictor both run. It
  lands a Body that's inside a blocker, picks its velocity and runs the mover. It queries only the
  live map's world (`Levels.Engine()`), which no step touches: walls and the edge are the map's.
- **The move is Box2D's geometric mover,** `physics/mover.Move`: up to 5 passes of collide → solve
  planes → cast, then the velocity is clipped. A pass under 1 cm ends it, unless it met a wall it had
  no plane for yet, so a Body slides on the tick it meets a wall. It writes `Transform2D`. Walls stop a
  Body and it slides along them.
- **A chain wall meets a Body as Box2D's chains collide:** from its front only (a counter-clockwise
  loop blocks from outside), and smoothly where segments join: the mover drops a segment's end plane
  while the Body is over the next segment's face (Box2D's ghost vertices). The pinned `CollideMover`
  does neither, so this is the one place the mover reads a shape's geometry.
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
  covers a phase ending inside a wall. The boot's map rules make sure every wall cell a pass reaches
  lands; `Step` says `stuck` if one doesn't, and `MovementSystem` logs it as an error.
- **The mover is shared sim code** the client predictor runs too, so it must stay deterministic: no
  Cardinal import, FMA-free arithmetic (`float64(a*b)`, or box2d helpers like `box2d.MulAdd`), and
  planes sorted by `(Normal.X, Normal.Y, Offset)`, never by ShapeID. So must the cast gate, hooks,
  shot flight and effect rules. `TestSharedCoreIsDeterministic` checks every package the predictor
  links from this module and world-engine (`go list -deps`, so a new import is scanned the day it
  lands): no Cardinal import, no FMA instruction on arm64, amd64/v3 or wasm, and no `math` call but the
  exact ones (floor, mod, …).
  **Never `math.Hypot`:** it fuses on arm64. Use `box2d.Length`.
