---
paths:
  - "shards/game/internal/projectile/**"
  - "shards/game/internal/area/**"
  - "shards/game/internal/combat/**"
  - "shards/game/internal/history/**"
  - "shards/game/internal/echo/**"
  - "shards/game/internal/physics/**"
  - "shards/game/internal/interact/**"
  - "shards/game/internal/team/**"
---

# Collision handling: deferred intents, rule-driven shots, per-feature contact reactions
- **Damage is deferred.** A damage source sends `DamageIntent`, and a heal `HealIntent`, to combat's
  queue (both in the `combat/component` leaf, so a source never imports combat's systems).
  `combat.DamageResolveSystem` (Update, registered *after* every sender) drains them and is the
  **only** place `Health` changes.
  Sources today: `projectile.FlightSystem` (shot hits) and `area.AreaSystem` (anti-camp); no heals yet.
- **HP stays between 0 and `MaxHP`.** For each target the resolver takes the tick's damage, then its
  heals, and writes HP once, never above `MaxHP`. Overkill counts, so a small heal can't save a target
  from a much bigger hit. Intents are summed in sorted order, so the order they arrive in — or which
  source sent first — never changes the result, and there's **no per-type event fan-out**. (Rampage
  re-emitted a typed event per collision kind to control ordering; deferral makes that unnecessary.)
- **A death is timed.** Each `DamageIntent` says when in the tick it lands (`Until`: a shot's
  `flight.End.Until`, 1 for damage over the tick). At 0 or below the target dies: HP 0 and a `Died`
  with the time of the hit that took it there. `combat.DeathSystem` removes it after the syncs, so each
  sync sends it once more.
- **An Echo's death puts its player out for the round** (GDD §8). `echo.EliminationSystem` gives the
  owner's Body the Echo tuning's `death_effect` (`out`) for `status.Forever` ticks. The cast gate's
  `blocked_by` and a default shot rule read the `out` tag, so the player neither casts nor shields.
- **Credit vs cause.** An intent's `Credit` is the Body credited with it (kills, lifesteal): the
  `team.Owner` of what dealt it. Its `Cause` is the cast that started it
  (`ability/component.Cause{Caster, CasterSerial, Ability, Tick, Seq}`, which `AbilityCast` embeds),
  carried by shots. `CasterSerial` tells the caster from a newer entity with its ID. `Seq` is the input
  frame that cast it, or 0 for a cast no frame made (an Echo's). A reflect
  would change the shot's Owner and keep Cause. An area's damage has no cause.
- **Shots don't use physics contacts.** A projectile has no Box2D body. `projectile.FlightSystem` moves
  each shot every tick and resolves the **first** thing on its path that its rules don't pass
  (`flight.Fly`): one Box2D shape cast on the live map's world (`Levels.Engine()`, from
  `pkg/levelstate`) over the map kinds its rules stop at, and
  one Box2D pair cast per recorded thing the scene's tree query finds (`past.Scene.Cast`), judged by its
  owner's relation and tags (team isn't a collision bit). A shot's `Type` names its shot type, whose rules are looked up
  each tick; its numbers are copied at launch.
  - First hit wins, which is what makes body-blocking exact, and a swept path can't tunnel at any
    speed. At the same point the earlier rule wins, then the map, then the lower entity ID, so by
    default a Body standing on its own Echo shields it.
  - **Every end is one fact.** `flight.Fly` returns a `flight.End`
    straight from the sweep: where the shot's center was, when in the tick (`Until`), and what it met.
    `FlightSystem` emits it as `component.ShotEnded` for every end, its range included. Damage stays a
    `DamageIntent`; the sync reads `ShotEnded`, as impacts and on-end rules will.
  - `pass` with a payload is rejected at load: with no hit list yet, it would hit the same thing every
    tick. Pierce, bounce and the rest come with their first users and a `Hits` list.
  - Cost is one map cast, one tree query, and up to two pair casts per Body or Echo found, per shot
    per tick: O(log N + K·R) for N shapes, K found and R rules, plus O(T·W) a tick to build the tree
    of T targets keeping W rows.
- **A shot meets each Body and Echo as it moves through the tick, as screens draw it.** A frame's
  shots fly a tick while its screen's view moves a tick, so a shot judged at view v meets each target
  across views v to v + 1 (`past.Across`): a span per pair of rows those views are drawn from,
  sliding as `AtView` places it, gone past its `Until`. The pair cast is in the target's frame as it
  moves (the shot's move less the target's), so a dash across the path meets the shot where the two
  paths cross, at any tick rate. A shot no frame cast (an Echo's) is judged at view T − 1: this tick,
  as every screen draws it, from the target's row a tick ago to where it is now, with its tags now.
  The hit's `Offset` is from the target's center when they met.
- **A predicted shot meets Bodies and Echoes where its client drew them (favor the shooter, by server
  rewind).** A shot a frame cast is `Projectile.Predicted`: its caster's client flies it in the
  predictor. Each tick `FlightSystem` flies it with `flight.Fly` at its caster's view, with no claim
  from the client:
  - The view is the applied frame's `ViewTick`, moved on by the ticks since on a tick with no new frame
    (`InputQueue.View`), then held to those whose tick's views are drawn from the rows kept
    (`past.Window.Clamp`): no older than the window, no newer than two ticks before, whose views run
    to the newest row. An older view (a high ping) is judged at the oldest, so it must lead its shots
    but still hits.
  - **The window is a budget, not a round number:** what an honest view lags, added up, in
    `content/history/tuning.json` (`window`): `interpolation_seconds` (how far behind the newest tick the client
    draws, which its render clock reads through the predictor, so both read one value), `queue_seconds`
    and the `ping_seconds` it favors. A cheat can't claim more than an honest player at that ping. The
    load fails unless the ping adds at least a tick.
  - A view never goes back: a frame's `ViewTick` below the largest one applied, or not a number, is
    taken at that one (`InputQueue.ViewFloor`, from 0, kept across sessions). A client can lag, but
    can't pick an older moment for each shot. An honest view only moves on (the render clock never
    steps back), so the predictor has no copy of this rule.
  - The map is now. It is never rewound.
  - Each recorded thing is where the views drew it (its `History`, `past.Across`). `history.SceneSystem`
    builds one `past.Scene` a tick, before every judge: each thing's kept rows and its row now, one
    Box2D tree box around all of them, so no speed bound (a dash, blink or landing stays in its box),
    and each client's view, held once. A judge acts as an actor: `Scene.Client(id, serial, side)` for
    a client while its Body is still the one with that serial (`Cause.CasterSerial`), else
    `Scene.Present(side)`, this tick as every screen draws it. It queries the scene (`Scene.Cast`),
    never the tree.
  - **One placement rule** (`Scene.spans`): a thing its owner's client predicts (`History.Predicted`)
    moves as that client draws it, from its row a tick ago to now, for that client's predicted actions
    only; its tags and life come from its rows at the view, like everything else. That covers your
    Body and your Echo by identity, not by kind. Contacts sort by fraction, rank, then ID, so the
    tree's visit order never matters.
  - The first thing met ends the shot now, and a payload becomes a `DamageIntent`. History is never
    rewritten.
  - The predictor flies your shots with the same `flight.Fly` and `past.Scene`, built from the target
    syncs when a step's first judge looks, so it gets the server's bits: no margin. A judge that looks
    without its view's syncs makes the step blind, which forces a replay. `End.View` is the view each
    end was judged at, which `TestPredictionMatchesServer` compares.
  - Plan and limits: `docs/research/favor-the-shooter.md`.
- **Contact reactions** (pickups, triggers; none exist yet): `physics2d` emits `ContactBegin/End` +
  `TriggerBegin/End` system events each tick (on PreUpdate). Blocking and sliding aren't reactions
  (a Body's happen in `player.MovementSystem`, see Movement and walls); these events carry category
  bits, not meaning. **A feature that reacts to contacts declares its own receiver** over that stream
  and does a **single pass** for *its* kinds only, matching by **category bits** (`physics.Match` —
  integer compare, *never* a reverse ECS type lookup) and **sending intents**. It is **not** a global
  router.
- **Know the cost of that model:** the engine keeps ONE shared event queue per type, so each reactor
  re-scans the *whole* contact stream — N reactors × C contacts/tick = **O(N·C)** `Match` calls. Fine
  while reactors are few. Because no layer sees the whole stream, a contact two reactors both match is
  handled by both (safe *only* because effects are deferred commutative intents), and a contact **no**
  reactor claims is silently dropped. To confirm masks during bring-up, register a **Debug-level
  contact logger in `physics`** only while debugging — never a catch-all inside a feature.
- **Why the reaction lives in the feature, not physics:** deciding a contact's *meaning* needs gameplay
  state — `Team` (friend/foe), owner, tuning. `physics` is a reusable, gameplay-blind plugin and must
  never import `team`/`combat`. Physics knows *that* shapes touched; the feature knows what that touch
  *does*.
- **Corollaries:** add a new collision *effect* as **new intents + resolver logic**, not an inline
  mutation; add a new *kind* of contact reaction as a **new feature reactor**.

### Collision filtering & teams
- **Every collision or overlap check is a Box2D query. Never hand-write one.** No distance or overlap
  math between entities, and no loop over every target: ask Box2D with a cast, overlap or ray, through
  physics2d, `Physics.Engine()` (Bodies and Echoes) or `Levels.Engine()` (the live map's walls and
  edge), and decide pass or stop in its callback. Box2D's tree is what keeps
  it scalable, and every shape is already in it. A Box2D pair cast (`box2d.ShapeCastCircle`) on a shape
  the tree found counts too, as does one on what a component saved, found by its own
  `box2d.DynamicTree` (`past.Scene`: the recorded things at an actor's view).
- **Category bits describe what a shape IS (its kind, `interact.Kind`)** — `Body`, `Echo`, `Shot`,
  `Wall`, `Bounds` (the map edge), `Area` (an area query), `Fill` (a wall's inside) — never whose side
  it's on. Snapshots keep the bits, so never renumber one.
- **A wall that isn't convex is an outline plus fill.** Its outline (kind `wall`, a chain) stops a walk.
  Convex `fill` pieces say where its inside is. A walk never asks for fill: pieces under one face hook a
  Body at their seam. What rests or flies asks with `interact.Solid(kinds)`, which adds the fill of the
  walls among them: where a Body lands (`motion.Step`), the map rules, a shot's cast. A pass through
  walls passes their fill, and a shot that ends on fill ends on a wall. No content names `fill`.
- **A map shape's category is its map kind, and its mask is all bits.** Each query's mask lists the
  kinds that stop it (`motion.Blockers`, or the map kinds a shot's rules stop at). A per-Body rule,
  like a pass through walls, changes that Body's query mask, never a shape's bits.
- **Filter at the source with mask bits** so pairs you'll never act on don't even generate events —
  cheaper than classify-and-discard. Two shapes interact only if each one's category is in the
  other's mask.
- **Team allegiance is NOT a category bit.** "Enemy" is relational (team A's enemies are teams B and
  C, who aren't allies of each other) and teams are assigned dynamically at match start — you can't
  bake that into a fixed bit. Friend/foe is decided **in gameplay code** from `Team`/owner components
  at collision time: `team.SideOf` reads a thing's side, and a rule matches the `interact.Relation`
  between two sides (own, ally, enemy, neutral), so teamless neutral geometry is never an ally or an
  enemy. This is *not* the "reverse type lookup" the classification
  rule forbids — that
  rule is about KIND (use bits); team is genuine game state (read the component).
- **Owner vs team.** Anti-camp self-damage (Echo overlapping its OWN Body) is an **owner** match
  (relation `own`); friendly fire (a shot passing through everything on its own team) is a **team**
  match (`own` or `ally`). Bodies (themselves), Echoes and shots carry `team.Owner` and the owner's
  `Team`, and `team.SideOf` turns them into an `interact.Side` for `Relation`. A "same-team = don't
  collide" bit/`groupIndex` scheme is therefore wrong for damage — it would suppress the owner-match
  self-collision.
- `groupIndex` can't express same-team pass-through for movement: the mover uses Box2D queries, and
  their filter ignores `groupIndex`. If Bodies ever block each other, decide that in the mover's planes,
  and skip the Body's own shape: queries don't, and the mover would push itself away from where it was.
  Never use it for damage rules.
