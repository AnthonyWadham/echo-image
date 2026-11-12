# Lag compensation for every system: one record, one scene, one query

Planned 2026-11-02. Planning only: no code has changed. It replaces the projectile-only rewind that
favor-the-shooter.md describes. Base: design B (entity record first). Grafts from A and C, and fixes for
every blocker the two critiques raised.

## Summary

- **Verdict: move lag compensation out of `projectile` into one new feature, `history`.** It owns each
  thing's public record. Every screen draws it, the predictor reads it, and every judge queries it.
- **One record per thing.** A Body or Echo opts in with one component, `historycomp.History`: a ring of its
  rows, written once a tick, after every writer, with 0 allocations.
- **One sync.** The record still goes out as `target_sync`, byte for byte as today. Only its owner moves.
- **One scene a tick.** `history.SceneSystem` builds one Box2D tree over each thing's kept rows, and
  resolves each client's view once. Shots, and later melee, rays, picks and bursts, all query it.
- **One query service** in the pure package `history/past`: `Cast` (swept or instant), `Overlap` and
  `Pose`. Each judges "as actor A saw it" and returns contacts in one total order.
- **One placement rule** replaces the own-Body and own-Echo cases written twice today.
- Geometry and hit-rule tags are judged at the view. The map and every outcome are judged now.
- The server and the wasm predictor run the same code on the same rows. Recordings stay byte-identical
  through every step.
- One world purge, in step 2. Every later field (jump, unfavored, seen, shape) is additive: no purge.

| Package | New or moved | Owns |
|---|---|---|
| `history` | new feature | `SceneSystem`, `RecordSystem`, `SyncSystem`; `TargetSync` (moved, same wire name); tuning |
| `history/component` | new leaf | `History`: the ring, and the opt-in |
| `history/past` | new, pure (wasm) | `Row`, the view rule, `Window`, `Scene`, `Actor`, `Query`, `Contact`, `Cast`, `Overlap`, `Pose` |
| `projectile/flight` | shrinks | shot types, meet rules, `Fly` on top of `past` |
| `projectile` | shrinks | launch and flight only; no history, no target sync, no player import |
| `predict` | changed | a per-serial mirror of the record; a scene when a step's first judge looks |
| client `game/history` | moved | the `target_sync` decoder (from `game/projectile`) |

## Problems

1. **All four layers (record, publish, view, judge) live in `projectile`.** Any new judge must import shot code.
2. **Bodies and Echoes become targets through projectile's leaf.** A new target kind imports the shot feature.
3. **Bodies' and Echoes' only position stream is projectile's `TargetSync`.** Drawing depends on the shot feature.
4. **The window and the client's interpolation delay are shot tuning.** Other judges must read shot content to clamp.
5. **`Reach` is unexported; only `Fly` can query rewound things.** No melee, ray, overlap or pick can be built.
6. **Own-Body and own-Echo cases are written twice, by kind** (server `seenAt`, predictor `sights`). Every judge must copy them.
7. **A target needs a Team and an Owner, and a row is one circle.** Neutral things get no rows and are never drawn.
8. **History is a shifting slice.** Two copies and 5-7 allocations per target per tick.
9. **The tree is rebuilt inside `FlightSystem` only, and the predictor builds one every step,** even with no shot.
10. **`viewOf` finds the caster by raw EntityID (F22).** A reused ID judges a shot at another player's view.
11. **"After every writer" is a comment.** Order lives in registration order and nothing enforces it.
12. **Teleports will streak (F21), and no rule says whether a shot is favored across one.** Blink and Echo-Swap are planned.
13. **Docs drift.** favor-the-shooter.md still says 0.4 s and 9 rows; the content says 0.25 s (5 rows at 20 Hz, 15 at 60 Hz).

## Changes

### 1. A neutral feature owns each thing's record

- **What:** a new feature, `history`. It owns the record, the recorder, the scene, the sync and the window.
  Judges import only the pure `history/past`, never the feature.
- **How:** constructors opt in. `player.bodyRow` and `echo.echoRow` hold `historycomp.History{Predicted: true}`
  instead of `projectilecomp.TargetHistory`. A future barrier adds `History{}`.
- **Why:** every mature library puts history in one module, and targets opt in with its component
  (Fusion HitboxRoot, Netick HitShapeContainer, lightyear LagCompensationHistory, L4D2 `LagCompensate`).
  Valve's own TF2 code calls the players-only manager a mistake. The record also drives drawing, as
  Overwatch's MovementState does, so no judge may own it.
- **Name:** `history`, because the record is a thing's public history. "rewind" is Echo-Swap's Echo-path
  rewind. "lagcomp" names one judge's concern, not the drawing record. It was `track` until 2026-11-12,
  which did not say what it holds.
- **Rejected:** keeping history in `projectile` and exporting `Reach` (the drawn record stays with its
  first judge); the row in a judging package like `lagcomp` (the same coupling, one package over).

### 2. One recorder, a ring, after every writer

- **What:** `history.RecordSystem` replaces `projectile.HistorySystem`. It runs first in PostUpdate.
- **How:**
  - The ring holds `KeptRows` slots; tick t sits at slot `t % KeptRows`. `With` writes in place: O(1),
    0 allocations. It is sized at a thing's first row (1 allocation per life). A window change resizes
    once and places the old rows by `Tick`.
  - `rowOf` is unchanged (shape 0, the Transform, tags for tick + 1, `Until` from `combat.Died`).
    Deaths go into a reused, sorted scratch slice, not a map.
  - PostUpdate holds the recorder, the five syncs, then `combat.DeathSystem`. System events last until
    PostUpdate ends, and physics2d registers nothing there (both verified).
- **Why:** one recorder after the tick's state is final (Source `FrameUpdatePostEntityThink`, Unlagged
  `G_StoreHistory`, Fusion after physics, lightyear after the solver). PostUpdate makes the order
  structural, not a comment.
- **Rejected:** a `Head` index like `PathHistory` (`t % len` needs no extra field); sharing Echo
  `PathHistory` (another owner, length and content, and Echo-Swap will rewrite it).

```go
package component // history/component: imports cardinal and history/past

// History is a thing's rows from its last ticks, as each tick's target sync showed it. Its constructor
// adds it; then every screen draws the thing and every judge finds it. Only history.RecordSystem writes it.
type History struct {
	Rows      cardinal.Slice[past.Row] // a ring: tick t at t % len
	Predicted bool                     // its owner's client predicts it (a Body, an Echo)
}

func (History) Name() string { return "history" }

func (t History) At(tick uint64) (past.Row, bool)                     // O(1); checks Row.Tick
func (t History) Record(r past.Row, keep int) History                   // With at r.Tick % keep
func (t History) AppendRows(dst []past.Row, before uint64) []past.Row // oldest first, ticks < before
```

### 3. The record is the one sync

- **What:** `TargetSyncSystem`, `TargetSync` and `TargetState` move from `projectile` to `history`
  unchanged. The wire name stays `target_sync`.
- **How:** same `netsync.Items` (`Everyone`, `seenBy`), one `SendTo` per viewer, complete every tick.
  The client decoder moves to `game/history` with the same types. The client keeps one timeline of
  targets keyed by serial; Body and Echo views filter it by kind. The draw-own rule is by serial
  alone: your Body's (from the same tick's body_sync) or `You.EchoSerial`, not a kind or EntityID check.
- **Why:** no surveyed library sends lag-compensation history as its own stream. Each peer builds it
  from replicated state, and Netick draws and judges from the same From/To/Alpha. Keeping the wire name
  keeps the wire fixture, the client decode and their mutants unchanged.
- **Rejected:** renaming the wire to `track_sync` (churn, no behaviour); positions in `BodySync` and
  `EchoSync` (two records per thing, F18); a per-viewer history like GoldSrc's (O(viewers·T·W) memory).

### 4. One window for drawing and judging

- **What:** the budget moves from projectile content to history content, as `past.Window`. One value
  drives the render clock, the ring size and the clamp.
- **How:** `interpolation_seconds` 0.1 + `queue_seconds` 0.05 + `ping_seconds` 0.1 = 0.25 s. The
  wasm `interpolationSeconds` reads it. `Validate` keeps today's check: the ping adds at least a tick.
- **Why:** one cap shared by all systems, or results differ by weapon (06 patterns). Valorant caps at
  140 ms, CS2 and Fusion at 0.2 s, Overwatch about 220 ms.
- **Rejected:** per-action caps: the same drawn frame would give different results per action.

```go
package past // history/past: imports box2d, interact, physics/timestep and std only

// Window is how far back an actor's view may be: what an honest view lags, added up.
type Window struct {
	InterpolationSeconds float64 `json:"interpolation_seconds"`
	QueueSeconds         float64 `json:"queue_seconds"`
	PingSeconds          float64 `json:"ping_seconds"`
}

func (w Window) Ticks() uint64                           // timestep.Ticks of the sum
func (w Window) KeptRows() int                           // Ticks(): the oldest view reads the row Ticks back
func (w Window) Clamp(view float64, tick uint64) float64 // today's flight.Tuning.View, unchanged

type Tuning struct{ Window Window `json:"window"` } // the history content

func (Tuning) JSONFile() string { return "history/tuning.json" }
func (t Tuning) Validate() error

// Row (Tick, Kind, Side, Center, Radius, Tags, Until: 64 B), RowTicks, AtView, along, lerp, Span and
// Across move here from projectile/flight, unchanged.
```

### 5. One scene a tick, for every judge

- **What:** `history.SceneSystem` builds `*past.Scene` once a tick. It runs in Update right after
  `echo.SpawnSystem` and before `ability.CastHookSystem`.
- **How:**
  - Per thing with `History` and a shape: `Now` = the live row at T, tags at T (today's `seenAt` row);
    `From` = its row at T − 1, or `Now`; `To` = `Now`; its kept rows, oldest first.
  - One tree box per thing over every kept row, `Now`, `From` and `To`, padded by the largest radius
    (today's `Reach.Add`). Category = its kind. Inserted in ID order. `Reset` destroys last tick's
    proxies, which keeps the node pool.
  - Per Body with an `InputQueue` and a frame: `AddClient(id, serial, q.View(T))`, clamped once.
  - After `echo.SpawnSystem`, so a just-spawned Echo is in it, as `reachOf` saw it today. Nothing
    between it and the judges moves a recorded thing: in Update only spawn sets a Body's Transform.
  - Derived, never saved: after a restore the first tick builds it. The shared pointer is wired like
    the physics plugin: the one place the leaf rule bends, said once in CLAUDE.md.
- **Why:** a broad phase over the whole window (Overwatch's union volume, lightyear's envelope, Fusion's
  BVH). Building once lets every judge share it, and resolving views once means no judge copies `viewOf`.
- **Rejected:**
  - A persistent tree with `MoveProxy`: proxy IDs to keep across ticks and restores, for no gain at this T.
  - A lazy scene on the first query: a cache, for a build that is small and allocation-free.
  - Move-and-restore in the live world (Source, FishNet): mutates what the predictor must match; one shooter at a time.
  - Whole-world clones (Unity NfE) or a second posed `box2d.World`: O(scene) a tick, static poses only.

```go
// Thing is one recorded thing in a scene.
type Thing struct {
	ID        uint32
	Serial    uint64
	Now       Row        // at the scene's tick: live (shard), or the newest sync (predictor)
	Predicted bool       // its owner's client predicts it
	From, To  box2d.Vec2 // where its owner's screen draws it as the tick starts and ends
}

type Scene struct{ /* tick, window, things, rows, clients, tree, scratch: all private */ }

func NewScene(w Window) *Scene                                    // w holds its clients' views
func (s *Scene) Reset(tick uint64)                                // empty for tick; keeps its memory
func (s *Scene) Add(t Thing, kept []Row)                          // kept: rows before Now, oldest first
func (s *Scene) AddClient(id uint32, serial uint64, view float64) // held by Window.Clamp
func (s *Scene) Build()                                           // sort by ID; one tree box per thing
```

### 6. Actors: a client's view, or present

- **What:** an `Actor` is whose action is judged and at which view. A client actor comes from the scene.
  A server actor is `Present`.
- **How:**
  - `Scene.Client(id, serial, side)` is false when the client is missing or `id` has another serial
    now; the caller then uses `Present`. That closes F22 for every delayed judge, in one place.
  - `Cause` gains `CasterSerial`, stamped where the one `Cause` is built (`ability.CastSystem`).
  - `Present(side)` is view T − 1, this tick as every screen draws it: Echo-cast shots, reflected shots,
    later AI. It carries no ID, so no sentinel (entity 0 is a real Body).
  - The raw view and its floor stay with the input (`ViewTick`, `InputQueue.View`, `ViewFloor`). The
    clamp is the window's. An instant cast on its applied tick reads exactly that frame's `ViewTick`.
  - Delayed effects use the view of the tick being judged: a shot in flight, its caster's current view
    (today; Fusion, lightyear); a wind-up, its landing tick's view (TF2 `Smack` after `SmackDelay`).
- **Why:** the view travels with each input, clamped (Source, Valorant, Netick, Fusion). An explicit
  present mode for server actors is Fusion's tick overload "for server AI".
- **Departure: no latency cross-check.** Source falls back to its own estimate when the claim is off by
  200 ms. Cardinal has no transport RTT, and a client-reported RTT can be inflated by the same cheat.
  The floor plus the window already cap a liar at an honest player at `ping_seconds`. RTT is an engine ask.
- **Rejected:** an `ActorOf(w, ...)` helper in the leaf (design A): world-reading logic in a types-only leaf.

```go
// Actor is whose action a scene judges, and the view its screen had as the tick began.
type Actor struct {
	Side      interact.Side
	View      float64 // a client's clamped view, or T − 1 when present
	Predicted bool    // its client predicts this action
}

func (s *Scene) Client(id uint32, serial uint64, side interact.Side) (Actor, bool)
func (s *Scene) Present(side interact.Side) Actor

// ability/component. New: CasterSerial, the caster's ref.Spawn serial (another serial on Caster: gone).
type Cause struct{ Caster cardinal.EntityID; CasterSerial uint64; Ability string; Tick uint64; Seq uint32 }
```

### 7. One placement rule

- **What:** one private function places a thing for an actor. `Cast`, `Overlap` and `Pose` all use it,
  on the server and in the predictor.
- **How,** first match wins:
  1. **Own and predicted:** `a.Predicted`, `t.Predicted` and the same owner. Spans, tags and life come
     from `Across(a.View)`. Positions move `From` → `To`. This covers your Body and your Echo, by no kind.
  2. **Present:** `t` carries the `Unfavored` tag in any row from the view's older row to now, or (with
     vision) was not sent to `a`'s team at the view. Then use `Across(T − 1)` and mark the contact
     `Present`. Both are specified now and built with their first user (change 11).
  3. **At the view:** `Across(a.View)`.
- **In the predictor,** rule 2 finds no rows at T − 1 and T, so the thing meets nothing. That is "not
  predicted", never "blind": it forces no replay.
- **Why:** one rule, so a new predicted thing changes one function on each side. Rule 2 is the industry
  policy (Source stops at a jump; Overwatch does not favor dashes and translocates) as per-content data.
- **Behaviour change (invisible today):** your own Body's tags and life now come from its rows at the
  view, like your Echo's. Today they are "now". Every shipped and recorded rule passes own things first,
  so no test or recording can see it.

### 8. One query service

- **What:** three read-only queries on the scene. The tree stays private. No judge writes its own
  narrow phase or tie-break.
- **How:**
  - **Broad phase:** one tree `Query` over the query's swept box, mask = `q.Kinds`. Candidates go into
    scene scratch through a package-level callback with the scene as context: no closure escapes.
  - **`Cast`:** per candidate, per span: `meet` first (rule before cast, as today), then one
    `box2d.ShapeCast` in the thing's moving frame. The frame is pinned to today's `hitOn`: the thing at
    the origin, the query's points offset by `from − center`, the move less the thing's. For a circle
    that is today's `ShapeCastCircle` call, to the bit.
  - **Instants:** `From == To` freezes things at that part of the tick (hitscan, a still swing).
  - **`Overlap`:** `box2d.ShapeDistance` (UseRadii) in the same frame. Touching when the distance is at
    most `0.1·LinearSlop`, the tolerance `World.OverlapShape` uses.
  - **Order:** each thing gives its first contact. Then one sort by `Contact.Before`: fraction, then
    distance, then rank, then ID. Ties stay by ID, as today, so recordings don't move.
  - **Self** passes by the meet rule (a GDD default), so there is no exclude flag. **The map** is never
    in the scene: a judge casts the live world now and compares fractions (Unity's two queries, Fusion
    `IncludePhysX`).
- **Why:** Fusion, Netick and lightyear ship the query, not the tree. Sorting once makes order-free
  results structural: the tree's visit order differs between shard and predictor.
- **Built now:** `Cast` (shots use it). `Overlap` and `Pose` land with their first users, with these signatures.
- **Rejected:** exposing `Scene.Query` plus a copied snippet per judge (design B); an interface `Rule`
  (escapes; a func value doesn't); separate `CastAt` and `Near` methods (one `Cast` with a time range covers them).

```go
// Query is a shape moving over part of the tick.
type Query struct {
	Shape      box2d.ShapeProxy // around its own origin
	Start, End box2d.Vec2       // its origin's path: equal for a still shape
	From, To   float64          // the part of the tick it covers; From == To: one instant
	Kinds      interact.Kind    // what it may meet: the tree mask
}

// Meet is the judge's rule for a thing as placed: its rank for ties, or ok false to pass it.
type Meet func(r Row) (rank int, ok bool)

// Contact is one thing a query met, with the row and view it was judged at.
type Contact struct {
	ID       uint32
	Serial   uint64
	Row      Row     // as the actor saw it; Center where it was at the contact
	Fraction float64 // Cast: of the query's path
	Distance float64 // Overlap: from the query's shape
	Rank     int
	View     float64
	Present  bool // judged as it is now (rule 2), not at the view
}

func (c Contact) Before(o Contact) bool // fraction, distance, rank, ID: the one total order

// Cast appends the first contact with each thing q meets, sorted. 0 allocations with room in out.
func (s *Scene) Cast(a Actor, q Query, meet Meet, out []Contact) []Contact

// Overlap appends each thing shape touches at part at of the tick, nearest first. (first user)
func (s *Scene) Overlap(a Actor, at float64, shape box2d.ShapeProxy, kinds interact.Kind, meet Meet,
	out []Contact) []Contact

// Pose is thing id as a saw it at part at of the tick, or false. (first user)
func (s *Scene) Pose(a Actor, at float64, id uint32, serial uint64) (Row, bool)
```

### 9. Shots use the service; projectile policy is content

- **What:** `Fly` takes an actor and the scene. `End` gains the view it was judged at, for tests.
  `projectile` drops `player/component`, `viewOf`, `seenAt`, `reachOf`, `Reach`, `Seen` and `Sights`.
- **How:** per tick, `Fly` casts the map now, then `scene.Cast` with the shot's circle over
  `[0, part]` of the tick and the shot's meet rules. `before` picks between the first contact and the
  map, as today (fraction, rule, map, ID).
- **Projectile policy:** `Shot.ViewSeconds` (`*float64`), built with the first shot that isn't whole-flight.
  - Omitted: the whole flight at the caster's view (today; Fusion, lightyear, Unity staff advice).
  - `0`: never; judged present (Valve, Unlagged: "you can dodge those").
  - `x`: the first x seconds at the view, then present (spawn-only when about a tick: TF2 Dragon's Fury).
  - Age is `T − Cause.Tick` through the timestep, so it means the same at 20 and 60 Hz.
  - The predictor then needs the shot's cast tick (`predict.Shot.Tick`, `ShotState.Tick`), with that user.
- **Children** (split, bounce, pierce) copy the parent's `Cause`, so they share its actor and policy.
- **Reflect:** the reflect hit is judged at the shooter's view. Then `Owner` becomes the reflector,
  `Cause` stays and `Predicted` clears. From the next tick it is `Present`. No client predicted the new path.
- **Why:** every shipped answer for projectiles is per content (none, spawn-only, whole flight).
- **Rejected:** a `launch` tick enum (designs B and C): one tick is 50 ms of flight at 20 Hz and 17 ms at
  60 Hz. Server fast-forward (UT4, FishNet): a second sim the predictor can't match.

```go
// projectile/flight
func (t Tuning) Fly(world *box2d.World, def Shot, f Flight, a past.Actor, scene *past.Scene,
	dt float64) (next Flight, end End, over bool)

// End{At, Until, Hit} gains View float64: the view its tick was judged at.

// projectile.FlightSystem gains Scene *past.Scene. Per shot:
side := team.SideOf(shot)
a, ok := s.Scene.Client(uint32(p.Cause.Caster), p.Cause.CasterSerial, side)
if !p.Predicted || !ok {
	a = s.Scene.Present(side)
}
next, end, over := tuning.Fly(world, def, f, a, s.Scene, physics.SecondsPerTick)
```

### 10. The predictor mirrors the record

- **What:** the predictor keeps the record per serial, as the shard keeps it per thing, and builds the
  same `past.Scene` when a step's first judge looks.
- **How:**
  - `See` writes each synced row into its serial's ring (slot `tick % KeptRows`). Serials sit in a
    slice sorted by serial, with a binary-search insert. No Go map, so no map iteration (F36). A serial
    absent for `KeptRows` ticks is dropped. A small ring of synced ticks keeps today's blind check.
  - When a step's first judge looks: `Reset`, then one `Thing` per serial in the newest sync with
    `Until ≥ 1` (today's rule). `Now` is its newest synced row. `Predicted` marks your Body
    (`id == Body`) and your Echo (`serial == EchoSerial`). Their `From`/`To` are what the step predicted.
  - The actor is `Actor{you, Window.Clamp(f.ViewTick, s.Tick), true}`. Blind (forces a replay) only
    when a judge looks and the syncs for its view are missing. One call gives the scene and the mark,
    so a new judge (melee, hitscan) can't skip it; a list of today's judges could.
  - `YouState` gains `EchoSerial`, so your Echo is found by identity, not by kind.
- **Why:** history built from replicated state, on each peer (Netick, Unity `ClientHistorySize`).
  The same rows and the same code give the server's bits.
- **Rejected:** a ring of whole syncs (design B): each build would search every slot per row. Moving
  own proxies each step (design A): a second motion path.

```go
type Content struct {
	Abilities cast.Tuning
	Shots     flight.Tuning
	Effects   status.Tuning
	History     past.Tuning // the window: clamp, ring size, render clock
}

type Sight struct{ ID uint32; Serial uint64; past.Row } // past.Row, was flight.Row

func (p *Predictor) See(tick uint64, sights []Sight)  // O(T log T); 0 allocations warm
func (p *Predictor) Forget()
```

### 11. Specified now, built with their first user

None of these has a user today. Each is an added field, so none needs a purge: a snapshot decodes a
missing field as zero (verified in world-engine's schema decode).

| Field | First user | Rule |
|---|---|---|
| `Row.Jump bool` | blink, spawn teleport, Echo-Swap | Geometry, both sides: never lerp across it. A view before the jump holds the older row; `Across` gives that pair a still span; the client blend snaps the same way. The view fixture gains jump cases. Fixes F21's streak. |
| `interact.Unfavored` tag | a movement or a barrier whose content is not favored | Rule 2 (change 7): judged now. Its content lists the tag (an effect kind's `tags`). The scene keeps each thing's newest unfavored tick, so the check is O(1). |
| `Row.Seen` team bits | the first vision feature | The recorder asks `netsync.seenBy`; the sync sends what `Seen` allows. Rule 2: a thing not sent to the actor's team is judged now (Source rewinds only what was sent). |
| `Row.Points [8]box2d.Vec2`, `Row.Count uint8` | the first non-circle barrier (F19) | Local to `Center`, `Radius` its rounding. `Cast` and `Overlap` use them in the same frame. `TargetState.Points` is sent only when not empty. |
| A neutral `Side` | the first thing with no Team or Owner | The record reads Team and Owner only when present; `Relation` returns `Neutral`. Today a recorded thing needs both. |

- **Unfavored and exact prediction:** the predictor applies rule 2 to the rows it has. When the tag
  sits in rows it doesn't have yet (the last ping and queue), it predicts a hit the server won't give.
  That is a correction, the class of "someone else killed it first". No content sets the tag, so the
  default keeps the bit-exact rule. Turning it on is a game call (decisions 2 and 4).

## View vs now

| Field | Judged | Why |
|---|---|---|
| Map (walls, bounds) | Now: a live Box2D cast | Static. Unity, Fusion and Netick use two queries. |
| Others' geometry (center, radius, later shape) | At the view | What the screen drew |
| Motion inside the tick | Views v to v + 1 (`Across`) | A frame's shot flies a tick while the view moves a tick |
| Kind, side | Rows at the view | Static per thing |
| Existence | Alive now **and** shown at the view (`Until`) | No hits on the dead (Source `LC_ALIVE`) or the undrawn |
| Hit-rule tags (`pass`, `stop`, payload filters) | At the view | Drawn and judged alike; bit-exact |
| Defensive outcomes (invulnerable, shields) | Now, in combat's resolver | The victim's defence wins (Overwatch, Reaper's wraith) |
| Damage, heals, effects, knockback, death | Now, in their resolvers | Never predicted (Overwatch, GAS, Gambetta) |
| Your own predicted things | Position: as predicted. Tags and life: at the view | As your screen draws them |
| Unfavored things (content) | Now | Not favored across escapes (Source, Overwatch) |
| Hidden things (later) | Now, for actors not sent them | Source rewinds only what was sent |
| The action itself (shot, swing origin) | Now | It is the actor's own predicted state |

**Body-block:** a Body blocks where the shooter's screen drew it, like every thing. One time frame per
query. The duel keeps its rules: the first thing met ends the shot, and an enemy Body stops it. Only the
timing follows the shooter, by at most the window (0.25 s).

- Mixing frames by accident (blockers now, victims at the view) is TF2's airblast trap, and it can't be
  predicted. So it is never the default.
- A deliberate, per-content exception is allowed: a barrier or an escape movement whose content sets
  `Unfavored` is judged now. That is a game call (decisions 1, 2 and 4).
- The lever for "late blocks" is the window's `ping_seconds`, not mixed time.

## Every user

| User | Rewound? | Calls |
|---|---|---|
| Predicted shot (bolt) | Yes | `Client` → `Fly` → map `CastShape` now + `Cast(Query{circle, pos, to, 0, part, meetable})` |
| Echo-cast or unpredicted shot | No | `Present(side)` → the same `Fly` |
| Pierce, bounce, split, return | Inherited | `Cast` each tick. Pierce: every contact before the map, not in `Hits`. Bounce: `Cast` from the bounce fraction. Split: children copy `Cause`. Return: `Pose` of your Body (rule 1). |
| Homing or chain pick | Yes (geometry) | `Overlap(a, at, circle(range), ...)`, first = nearest; steering by `Pose`. `lowest_hp` reads HP now, so it is shard-only. |
| Reflect | The hit | `Cast` at the shooter's actor; then `Present(reflector's side)` |
| Hitscan | Yes | Map `CastRayClosest` now; `Cast(Query{point, muzzle, muzzle+dir·range, 0, 0})`; first before the wall |
| Beam or channel | Yes, each tick | Hitscan each tick, with that tick's `Client` |
| Melee swing | Yes | One tick: `Cast(Query{sector, origin, origin, 0, 1})` (things sweep, the swing is still). Several ticks: the same per tick, with `Hits`. Rotating: `box2d.TimeOfImpact` with that user. |
| Targeted acquisition | Yes | A ray `Cast` or an `Overlap` pick at the cast frame's view; a new `Sink` method on both sides; the effect applies now |
| Heal on your own Echo | No | A filter: your Echo is present and predicted. An ally picked by aim is acquisition. |
| AoE placed at the aim | Placement no; burst yes | Placement: the frame's point, map checked now. Burst: `Overlap(a, 0, area shape, ...)`. |
| Zone pulses, anti-camp, pickups | No | Live `OverlapShape` (`area`). Contested grabs resolve in sorted order. |
| Barrier as a target | Yes, as a thing (or now: decision 4) | Its constructor adds `History{}`; a non-circle one needs the row shape |
| Knockback, slow, stun on hit | The hit only | Outcomes, now; direction from the contact |
| Own walk, dash, blink, landing | No | The mover against the live map. A blink sets `Row.Jump`. |
| Echo-Swap | No | Your Echo is present. It sets `Jump` for your Body. Its path rewind is `PathHistory`, unrelated. |
| Body blocks movement; damage, heals, death, immunity | No | The live mover; their resolvers, now |
| Vision (`seenBy`) | No | Live, per viewer team; later it fills `Row.Seen` |
| Shot vs shot | No | Shots keep no history |
| NPCs, summons, turrets, bots (labelled stress cases only) | Targets yes; actors present | The constructor adds `History`. AI uses `Present`. A bot that sends input frames is a client. |

## Costs

T = recorded things, W = `KeptRows` (5 at 20 Hz, 15 at 60 Hz), P = clients, K = candidates per query.

| Piece | Today | Design | Big-O per tick |
|---|---|---|---|
| Record | Shifting slice: 2 copies, 5-7 allocs per target. 4.5 µs / 60 allocs (W6, T12); 29 µs / 168 (W16, T24). Measured, scratch bench. | Ring with `With`: 0.1-0.2 µs, 0 allocs (scratch bench of the same pattern) | O(T) |
| Broad phase | `Reach` on shot ticks, in `FlightSystem` only: 5.0 µs / 51 allocs (W6, T12); 17.9 µs / 125 (W16, T24). Measured. | The scene, every tick, for every judge. Buffers and node pool reused. **Estimate** 1-5 µs, 0 allocs warm. | O(T·W + T log T + P log P) |
| Per shot | 1 map cast, 1 tree query, ≤ 2 pair casts per candidate, plus ECS reads and a status fold per candidate | The same casts. Rows come from the scene. Actor lookup O(log P). Sort O(K log K). 0 allocs. | O(log T + K·R + K log K) |
| Sync | O(T) + one list per team, 1 send per viewer | Unchanged | Unchanged |
| Snapshot | T·W rows | T·W rows, plus a bool per thing | — |
| Wire | `target_sync` every tick | Unchanged. `body_sync` gains `You.EchoSerial` (~10 B). No history stream. | — |

- **TestTickAllocations (3v3):** measured 3,300 at 20 Hz (ceiling 4,000) and 4,976 at 60 Hz (ceiling
  6,000). History and `Reach` make up about 111 and 147. Expect about 3,190 and 4,830 after step 3.
  Ceilings stay. `BenchmarkSceneBuild` lands in step 3; the scene numbers above are estimates until then.
- **Per second:** the scene copy is O(T·W·rate) = O(T·rate²), since W grows with the rate. At 60 Hz and
  24 things that is 384 rows (about 25 KB) a tick. 100 stress-case things: 1,600 rows (about 100 KB).
- **No O(P²):** each system visits each thing once. Queries touch only tree candidates.
- **Wasm:** today every step and replay builds a map and a tree, even with no shot. Design: 0 on idle
  steps; O(T·W + T log T), 0 allocs warm, on a step a judge looks (**estimate** tens of µs at 15-25×
  native).
  `See` is O(T log T). The JS decode of `target_sync` (~180 µs per 12 things, F12) still dominates.

## Migration

Each step is one PR and ends green: `go test` at 20 and 60 Hz, the client tests, and the mutation run.
The 20 Hz and 60 Hz recordings and the view fixture are the oracle: **they must stay byte-identical
through steps 1-5.** Any diff blocks the step. The mutation tool fails when a mutant's old text is
gone, so none is lost silently: count them before and after each step (226 today). A client mutant
moves only if its text moves.

| Step | Purge | Behaviour | Recordings / fixtures |
|---|---|---|---|
| 1. Pure core and content | No | None | Digests same; view fixture same; SimVersion changes |
| 2. `history` feature: record and sync | **WORLD PURGE** | None | Digests same; `target_sync` wire fixture same |
| 3. Scene, actor, query service, predictor mirror | No | Own Body tags at the view (invisible) | Digests same; `body_sync` fixture regenerated |
| 4. PostUpdate | No | None | Same |
| 5. Client: one timeline, draw-own by identity | No | None | Same |

**Step 1: pure core and content.**
- `history/past` gets the view rule from `flight` (`Row` to `Across`), and `Window` from `flight.Rewind`,
  `RewindTicks`, `KeptRows` and `View`. History content holds the window; projectile content loses
  `rewind`. The catalog, `predict.Load`, the wasm `interpolationSeconds` and the pinned `recorded`
  content follow.
- Tests: the three view tests move to a `past` view test (it mirrors the moved source).
  `TestViewFixture` calls `past.AtView`. `TestBadContentFailsTheBoot` adds a bad window. The client
  predictor test reads history content.
- Mutants re-pointed (paths only): 6-G1, 6-G2, 6-G3, F2-A1, F2-A2, F2-A3 (view rule); 6-G4, X6-G3,
  F9-S1, F9-V1 (window); F9-W1 (wasm interpolation).

**Step 2: the `history` feature. WORLD PURGE.**
- `history` gets `RecordSystem` (from `HistorySystem`), `SyncSystem` and `TargetSync` (same wire name).
  `history/component` gets `History` (the ring). The Body and Echo constructors hold
  `historycomp.History{Predicted: true}`; `TargetHistory` goes. `seenAt` and `reachOf` read `History` for
  one step. The client decoder moves to `game/history`.
- **Purge reason:** the component name changes from `target_history` to `history` (by way of `track`),
  and a restore fails on an unknown name an entity uses (verified). F3's purge already landed (commit 1c0e968), so there is
  nothing to bundle with. Keeping the old name forever was rejected as a lasting mismatch.
- Tests: `TestTargetHistoryIsWhatSyncsShow` becomes `TestHistoryIsWhatSyncsShow`, in a new server test
  file that mirrors the new feature. It adds the ring size at both rates, a window change, and "every
  `History` holder is in the sync". The harness's `rowOf` and `echoOf` re-point.
- Mutants re-pointed: 12-G1, 12-G2, 12-G3, 6-G5 (record); 12-G4, 12-G12 (sync). New: ring slot off by
  one; a resize drops a row.

**Step 3: scene, actor, query service, predictor mirror. No purge.**
- Changes 5-10 land: the scene and `SceneSystem`, actors and `Cause.CasterSerial`, placement, `Cast`,
  the new `Fly` and `End.View`, and the predictor mirror with `YouState.EchoSerial`.
  `viewOf`, `seenAt`, `reachOf`, `Reach`, `Seen`, `Sights`, `sights` and the `seen` map go.
- The one behaviour change: own Body tags and life at the view. No shipped or recorded rule can see it.
- Tests: the `Fly` tests build a `past.Scene`. `TestFlyMeetsEachTargetAsTheTickShowsIt`'s sweep also adds
  things in reverse order and wants the same ends. `TestPredictorPlacesOnlyWhatSyncsShow` adds a sync
  older than the ring. `TestPredictionMatchesServer` also compares `End.View`. A new scene test (mirrors
  the new source) holds `BenchmarkSceneBuild` and a stale-serial `Client` case. `TestTickAllocations`
  should fall by about 111 / 147.
- Mutants re-pointed: 12-G5, 17-R1, 17-R2, 8-G9, 8-G10, F2-S1, F2-S2 (flight system); 12-G11, 17-F1,
  17-F2, 17-F3, 17-F5, F2-H1, F2-H2, F2-H3 (tree box and pair cast); 12-G6, 12-G7, 12-G8, 6-G6, 8-G12,
  8-G13, 8-G14, X5-N2, F12-A3, F2-P1, F2-P2 (predictor).
- New mutants: `Client` ignores the serial; `Present` at T, not T − 1; `Reset` keeps last tick's things;
  rule 1 for any owner; `Before` ignores the ID; things inserted unsorted.

**Step 4: PostUpdate. No purge.**
- `RecordSystem`, the five syncs and `DeathSystem` register with `cardinal.WithHook(cardinal.PostUpdate)`.
- New mutant: `RecordSystem` back in Update before `FlightSystem`. `TestHistoryIsWhatSyncsShow` catches it.

**Step 5: client. No purge.**
- One targets timeline keyed by serial; Body and Echo views filter by kind; draw-own by `You.Body` and
  `You.EchoSerial`.
- Mutants re-pointed: 12-T1, 12-T2, F18-R1, F18-R2 (timelines). New: draw-own ignores `EchoSerial`.

**Docs (with each step):** CLAUDE.md (layout, dependency lists, ordering, client sync, the
favor-the-shooter bullets, the shared scene pointer as the one leaf-rule bend). favor-the-shooter.md
(new names; 0.25 s, 5 and 15 rows; this view-vs-now table). foundation-review.md (F18, F19, F21, F22
status). The options doc's homing helper is renamed from `track`.

**Later, each with its first user (none needs a purge):** `Overlap` and `Pose` (homing, chain, melee,
acquisition, burst); `ViewSeconds` (first non-whole-flight shot); `Row.Jump` (first teleport, with a
view fixture case); the `Unfavored` tag (decision 2 or 4); `Row.Seen` (vision); the row shape (first
non-circle barrier). Each adds its tests to the existing files and its mutants to the list.

## Limits

- **No latency cross-check.** Cardinal has no RTT. The window caps a liar at an honest player at the favored ping.
- **Above the window the shooter must lead.** Both sides clamp the same way, so the predictor's answer is
  still the server's. So no Overwatch-style "stop predicting at 220 ms" ladder: ours isn't wrong there.
- **Unfavored and hidden things are never predicted.** Their hits come with the next body_sync.
- **The map is never rewound.** A wall that moves or breaks would need rows of its own.
- **No rotating sweeps** until a user needs `TimeOfImpact`. **No shot vs shot:** shots keep no history.
- **The broad phase is the query's box.** A long diagonal ray among many things over-collects. Bench with
  the first hitscan; the tree's own `RayCast` is the drop-in.
- **Only shots can show a hit on the target where each screen draws it.** A new judge's sync must carry
  `TargetSerial` and `Offset` like `ProjectileState` does.
- **Hidden info leaks through shot ends** (F38) until `seenBy` is real.
- **NPCs, turrets and bots are stress cases only.** No content adds them.

## Decisions for Anthony

Game-design calls only. All eight were decided on 2026-11-03.

1. **Whose view judges a body-block?** Decided 2026-11-03: the shooter's view (today). One frame per
   shot, exact prediction. If defenders feel blocks land late, lower `ping_seconds` rather than mix time.
2. **Are shots favored across teleports and escapes (blink, Echo-Swap)?** Decided 2026-11-03: no. Their
   content sets `Unfavored`, so they are judged now. Every verified source stops the rewind at a teleport
   (Source 64 units, GoldSrc 128, Unlagged, MVDSV), and Overwatch doesn't favor translocates. Cost: hits
   on a target that just escaped aren't predicted, so the shooter sees a correction.
3. **Are shots favored at dashing targets?** Decided 2026-11-03: yes for now (today). Overwatch says no. Revisit
   after playtests; the dash's content would only add the tag.
4. **Barriers: judged at the shooter's view, or now?** Decided 2026-11-03: now (`Unfavored`). A barrier is a
   defensive state, and a shot passing a wall the defender just placed reads as broken. Cost: hits on
   barriers aren't predicted.
5. **Defensive abilities: outcome or hit rule?** Decided 2026-11-03: outcomes (combat drops the damage now), like
   Overwatch's wraith. Use a pass tag (judged as drawn) only for a deliberate dodge-through feel.
6. **The window.** Decided 2026-11-03: keep 0.25 s until playtests at real ping. Valorant caps at 140 ms.
7. **Per-shot favor.** Decided 2026-11-03: whole flight for the bolt. Set `view_seconds` per shot type when slow,
   lobbed or long-lived shots arrive.
8. **Invisible things (when stealth arrives).** Decided 2026-11-03: judged now for shooters who weren't sent them.

## Engine asks

None is a dependency; every step above works on v0.22.0-beta.

1. **Generational EntityIDs (F22).** They would retire `Cause.CasterSerial`, `You.EchoSerial` and the ID-plus-serial keys.
2. **Per-connection RTT.** It would allow Source's view sanity check.
3. **`DynamicTree.Clear` that keeps the node pool.** `Reset` destroys proxies one by one today.
4. **Checked system ordering** (before/after constraints at registration). "Scene before judges" would be enforced.
5. **`box2d.ShapeCast` doc fix.** An initial overlap returns a hit at fraction 0; the comment says miss.

## Appendix: where things live, and sources

### Paths

Under `shards/game/` unless they start with `client/`, `docs/` or `pkg/` (world-engine v0.22.0-beta).

| What | Path (planned paths are new) |
|---|---|
| Pure core | `internal/history/past/view.go` (from `internal/projectile/flight/view.go`), `window.go`, `scene.go`, with `view_test.go`, `scene_test.go` |
| Leaf | `internal/history/component/history.go` (replaces `internal/projectile/component/target_history.go`) |
| Feature | `internal/history/record_system.go` (from `internal/projectile/history_system.go`), `scene_system.go`, `sync_system.go`, `target_sync_event.go` (from `internal/projectile/`), `register.go`, `doc.go`, `tuning.go` |
| Content | `content/history/tuning.json` (from `content/projectile/tuning.json` `rewind`) |
| Changed | `internal/projectile/flight/flight.go`, `tuning.go`, `internal/projectile/flight_system.go`, `internal/ability/component/ability_cast_event.go`, `internal/ability/cast_system.go`, `internal/player/spawn_system.go`, `internal/echo/spawn_system.go`, `internal/player/body_sync_event.go`, `internal/predict/predict.go`, `predictor/main.go`, `main.go` |
| Input (unchanged) | `internal/player/component/input.go`, `internal/player/input_system.go` |
| Client | `client/src/game/history/target_sync_event.ts` (from `client/src/game/projectile/`), `client/src/game/player/local_player.ts`, `body_view.ts`, `client/src/game/echo/echo_view.ts`, `client/src/game/ui/hud.ts`, `client/src/engine/render/entity_views.ts` |
| Tests, fixtures | `main_test.go`, `projectile_test.go`, `prediction_test.go`, `wire_test.go`, new `history_test.go`; `testdata/prediction-20hz.txt`, `prediction-60hz.txt`, `view.json`, `wire/target_sync.msgpack`, `wire/body_sync.msgpack`; `scripts/mutate/mutants.json` |
| Docs | `docs/research/favor-the-shooter.md`, `foundation-review.md`, `system-architecture-options.md`; `CLAUDE.md` |
| Engine facts | `pkg/box2d/dynamic_tree.go`, `distance.go`, `world_query.go` (overlap tolerance); `pkg/immutable/slice.go` (`With`); `pkg/cardinal/internal/ecs/world.go` (hooks, events); `pkg/cardinal/internal/schema/schema.go` (missing fields are zero); `pkg/cardinal/internal/ecs/world_state.go` (unknown names fail a restore) |

### Sources

Verified = read in a fetched source during the research. Not verified = search snippet or secondary only.

| Claim | Source | Status |
|---|---|---|
| Source lag compensation: view = tick − lerp, 200 ms check, 64-unit teleport stop, `sv_maxunlag` 1 s | https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/server/player_lagcompensation.cpp | Verified |
| TF2: rewinds only players sent to the shooter; teammates for medigun and melee | https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/server/tf/tf_player.cpp | Verified |
| TF2 comment: "all entities could opt-in" | https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/shared/tf/tf_fx_shared.cpp | Verified |
| TF2 melee lands in `Smack` after `SmackDelay` | https://github.com/ValveSoftware/source-sdk-2013/blob/be57e3d01ac7d7aa6a2cacbfef187c0b46616db0/src/game/shared/tf/tf_weaponbase_melee.cpp#L374 | Verified |
| Alien Swarm/L4D2 opt-in manager | https://github.com/ReactiveDrop/reactivedrop_public_src/blob/9231d40422194dfecca8b698698df4af7e0d99f1/src/game/server/player_lagcompensation.cpp | Verified |
| Bernier: Half-Life doesn't compensate projectiles | https://www.gamedevs.org/uploads/latency-compensation-in-client-server-protocols.pdf | Verified |
| Unlagged history, reset, "you can dodge those" | https://icculus.org/~zakk/q3run/unlagged-2.01-src-gpl.zip ; https://github.com/smokin-guns/SmokinGuns/blob/HEAD/Unlagged.txt | Verified |
| MVDSV antilag (lagged traces, projectiles at the owner's view) | https://github.com/QW-Group/mvdsv/blob/master/src/sv_phys.c | Verified |
| Overwatch MovementState, 0.5 s union volume, 220 ms, wraith judged on the server | https://www.sohu.com/a/148848770_466876 | Verified (Chinese translation of the GDC 2017 talk) |
| Overwatch: no favor at dashing or translocating targets | https://www.gamesear.com/news/overwatch-devs-have-released-an-update-on-the-recent-hit-registration-problems | Verified (quote of a dead forum post) |
| Valorant: client sends viewed time; cap 200 → 140 ms | https://www.riotgames.com/en/news/peeking-valorants-netcode ; https://playvalorant.com/en-us/news/game-updates/valorant-patch-notes-6-02/ | Verified |
| Fusion: queries per player, `IncludePhysX`, 200 ms buffer, AI tick overload | https://doc.photonengine.com/fusion/current/manual/lag-compensation | Verified |
| Fusion Projectiles Essentials: a lag-compensated ray every flight tick | https://doc.photonengine.com/fusion/v2/technical-samples/projectiles-essentials | Verified |
| Netick: queries by input source; same From/To/Alpha as rendering | https://netick.net/docs/2/articles/lag-compensation.html | Verified |
| lightyear: `LagCompensationHistory`, AABB envelope, per-shot timestamps as the refinement | https://github.com/cBournhonesque/lightyear/blob/main/crates/integration/avian/src/lag_compensation/history.rs | Verified |
| Unity NfE: history capacity 32, clamp to oldest, `ClientHistorySize` | https://docs.unity3d.com/Packages/com.unity.netcode@1.13/api/Unity.NetCode.PhysicsWorldHistory.html | Verified |
| FishNet projectiles fast-forward by passed time | https://fish-networking.gitbook.io/docs/guides/features/lag-compensation/projectiles | Verified |
| GAS: no damage prediction; prediction keys | https://raw.githubusercontent.com/tranek/GASDocumentation/master/README.md | Verified |
| CS2: rewinds to the fractional render time sent in the usercmd | https://github.com/SteamDatabase/Protobufs/blob/c445e8b11ba40ffca79dc16a7c58990601e4026d/csgo/cs_usercmd.proto | Not verified |
| Overwatch rewinds targets during projectile flight | https://gdcvault.com/play/1024041/Networking-Scripted-Weapons-and-Abilities | Not verified (not relied on) |
