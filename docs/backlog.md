# Backlog: ideas and decisions for later

Things we talked through but didn't build yet. When asked "what next?", read this list, check each
entry against the code (it may be done or stale), and suggest one.

Each entry has a status:
- **idea**: not decided
- **decided**: agreed, not built
- **done**: built (keep a line, or delete)

---

## Units that block movement, and prediction

- **Status:** idea
- **Added:** 2026-11-02
- **Today:** only walls and bounds block a Body (`motion.Blockers = Wall | Bounds`). Bodies pass
  through each other. The predictor's Box2D world is empty, so it can't see walls yet either.

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
- Do the map walls in the predictor first (see `map-system-findings.md`). Same mechanism, and it fixes
  today's wall mispredict.
