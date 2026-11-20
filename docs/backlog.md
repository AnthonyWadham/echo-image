# Backlog: ideas and decisions for later

Things we talked through but didn't build yet. When asked "what next?", read this list, check each
entry against the code (it may be done or stale), and suggest one.

Each entry has a status:
- **idea**: not decided
- **decided**: agreed, not built
- **done**: built (keep a line, or delete)

---

## Loop guard for triggers

- **Status:** idea
- **Added:** 2026-11-20
- **Today:** nothing in echo can loop. A death triggers "out", and "out" triggers nothing. A depth cap
  on every intent (`queue.Chain`) was built and taken out before use.

**Why not a depth cap:** a loop that branches (a split, a fork) makes 65,536 copies by step 16 at two
children a step, so a cap stops it far too late. A guard that is a design rule ("an on-crit strike
can't crit again") stops at step 1.

**What games do** (from memory): a rule per mechanic, carried on the child. Dota flags reflected
damage so it can't be reflected again. Risk of Rain 2 records which items already fired in a chain.
Rampage has four hand-made guards of this kind (`docs/research/owner-queues.md` §3).

**Plan:** with the first trigger that can fire itself (on-hit, on-death, reflect, split), add that
mechanic's rule. When a second one comes, share one "already fired" flag set across them.

---

## Units that block movement, and prediction

- **Status:** idea
- **Added:** 2026-11-02
- **Today:** only walls and bounds block a Body (`motion.Blockers = Wall | Bounds`). Bodies pass
  through each other. The predictor builds every map's walls and walks the live map, like the server.

**How it would work**
1. Each frame, place each blocking unit in the predictor's Box2D world as an obstacle. Don't simulate
   it; we don't know its input.
2. Run the same `motion.Step` as now. The mover already casts against whatever is in the world.
3. Add the unit's kind to `Blockers` on server and client, so both run the same code.

**The catch:** the server collides your move at tick T with the unit at tick T. The client only knows
that unit's past position.

| Unit | Prediction |
|---|---|
| Static (totem, turret, wall) | Exact: same bits as the server |
| Predictable (known path, like an Echo) | Good: compute where it will be |
| Another player | Guess: collisions will sometimes snap |

**What real games do**
- Static and scripted blockers go in the client world.
- Players vs players: most fast games avoid hard collision (Overwatch and Valorant teammates pass
  through each other). MOBAs use a soft push over a few ticks, so a wrong guess blends in.
- If hard collision is needed: place others at their last known position, or extrapolate by
  velocity, and accept small corrections.

**Recommendation**
- Static and predictable blockers: add them to the predictor world.
- Player blocking player: decide the design first. If needed, use soft push, not hard collision.

---

## Touching walls: bake them into one outline

- **Status:** decided (step 1 not built)
- **Added:** 2026-11-15
- **Today:** two walls drawn as separate shapes that touch bake as two solids. The mover can stop at
  the seam between them. Measured with the mover tests' slide harness (two boxes (-6,-2)-(0,0) and
  (0,-2)-(6,0) against one box): sliding +x along the top, pressed at 84° or steeper, it stops at the
  seam in 46 of 144 runs (12 press angles x 3 radii x 2 speeds x 20 and 60 Hz). Sliding -x never stops.
  Touching walls are common in maps, so refusing them at bake is the wrong fix.

**Why:** a mover sliding over two separate shapes catches the inner corner where they meet: Box2D's
"ghost collision". Our mover is already Box2D's own (`CollideMover`, `SolvePlanes`, `CastMover`), so
the fix belongs in the geometry, not the mover.

**What real engines do**
- Box2D: chain shapes for level geometry; ghost vertices smooth the joins. Our outlines are chains.
- Unity: Composite Collider 2D merges child colliders into one, as outlines or polygons.
- Godot 4.5: tile layers merge their collision shapes automatically, in chunks; its docs call bumps on
  shared tile edges a known issue.

**Plan**
1. In the baker, merge same-kind polygons that share an edge (no overlap) into one outline, then let
   the existing outline + fill path cut it. Whole-number points, so exact maths; no library (bake
   imports only box2d, the standard library and level). About 250 lines and 100 test lines, 1-2 days.
   Circles and capsules stay separate.
2. Only if a map needs it: overlapping shapes and holes (a ring of walls). About a week.
