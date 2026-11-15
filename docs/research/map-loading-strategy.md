# Map loading strategy (2026-11-13)

> **A proposal, not the source of truth.** Steps 0 to 6 are built (the mover's chain rule, the core
> in `pkg/level`, the Cardinal side in `pkg/levelstate`, the game's `arena` and its open default
> map, movement, shots and the predictor on the map's world, spawns from the map, and the map rules
> at boot). Code,
> [`CLAUDE.md`](../../CLAUDE.md)
> (how) and [`GDD.md`](../GDD.md) (what) win when they disagree with this file. It replaces the
> proposal in [`map-system-findings.md`](map-system-findings.md) §6-§7. The facts in that file's §2
> still hold unless §12 here says otherwise.
>
> Tags: **[code]** = checked in code. **[test]** = proven by an experiment in §12.
> **[R1]**-**[R6]** = research, sources in Appendix B. **[open]** = not proven yet.

## 1. Verdict

- **Build one small, generic map library, in echo_image for now.** A core with no Cardinal in it,
  plus a thin Cardinal side. The same split as `box2d` and `physics2d`. It is written so it can move
  to world-engine as a plugin, or serve as an example other games copy from (7.1).
- **A map is one baked file.** Whole numbers on a fixed grid. Final shapes. No angles. The server
  and the client load the same bytes.
- **Walls are content, never entities.** One function builds them into a plain Box2D world. The
  shard and the wasm predictor call the same function.
- **Game state holds only "which map".** One small component. One system rebuilds the walls after
  boot, restore, map change or reload.
- **The game injects all meaning:** kind names, object types, rules. The library never names a
  wall, a team or a spawn.
- **Maps are drawn in Tiled.** A Go baker turns Tiled files into baked files. No game reads a
  Tiled file at runtime.
- **Bad content fails loudly,** at bake and at boot. No warnings. No fallbacks.
- This is how Quantum, Source, Unreal and Unity DOTS do it [R1]. Three designers, working alone,
  picked this same architecture.
- **How wall pieces meet is settled (step 0, built).** Solid pieces that touch hook a Body. So a
  wall is one chain outline plus fill pieces, and the mover treats chains the way Box2D does. Nothing
  stalls. See 7.4.

## 2. Key terms

| Plain words | Working code name | What it is |
|---|---|---|
| the core | `level` | Reads a baked file, checks it, builds Box2D shapes. No Cardinal. |
| the baker | `level/bake` | Offline tool. Tiled file in, baked file out. |
| the plugin | `levelstate` | Cardinal side. Keeps "which map" in state and the built walls in memory. |
| baked file | `<id>.level.json` | The one file the server and the client load. |
| solid | | A convex wall piece: polygon, circle or capsule. |
| edge | | The map border: one closed loop. |
| kind | | A name on a shape (`wall`, `bounds`). The game maps it to Box2D bits. |
| object | | A typed point placed in the map (a spawn). |
| the game side | `arena` | echo_image's own package: its kinds, its spawn type, its rules. |

Names are placeholders. See §11.

## 3. How it works today

- No map ships. Walls exist only in tests, as ECS entities built by physics2d. [code]
- Movement and shots already take a plain Box2D world. Today they get physics2d's. [code]
- The predictor has its own Box2D world. It is empty. A shot the server stops at a wall flies on,
  on your screen. [code]
- Spawn points are two pairs of numbers in player tuning. [code]
- Nothing owns "the match", so nothing picks a map.

## 4. Problems

Problems 1-3 have one cause. Today a wall reaches the server's Box2D world only as an ECS entity,
which physics2d copies in. The tests do this, and the earlier plan did too. Box2D itself needs no
Cardinal and no entities. The design loads walls from the file straight into Box2D (7.3).

1. **The predictor can't build them.** physics2d reads ECS components, so it needs Cardinal, which
   the predictor can't link. It also rotates shapes with Go's cos and sin, which can give other bits
   on arm64 than in the browser (finding F8).
2. **Cost for nothing.** Every tick, physics2d copies and compares each entity, and every snapshot
   saves it. A wall never changes.
3. **Stale walls after a restart.** Entities come back from the snapshot, not from the build, so an
   edited map keeps its old walls.
4. Nothing checks a map: a spawn in a wall, a sealed pocket, a wall too deep to land out of (F26).
5. Nothing in this repo can be imported by another game. [code]

## 5. Goals and non-goals

**Goals**
- One library any World Engine game can import. No game words inside it.
- The server and the client collide with the same bits.
- One path for boot, restore, map change and reload.
- A designer adds a wall without a programmer.
- The format can grow (regions, nav, vision, several maps) without breaking old files.

**Not now**
- Streamed or chunked worlds. Procedural generation. Tile maps. Bushes and vision. Destructible
  walls. Art.
- Any content the GDD does not have.

## 6. The four layers

```
Tiled file ──bake──▶ baked file ──load──▶ Box2D walls + typed objects ──▶ the game
 (author)   (offline)  (content)   (boot)
```

1. **Author.** Draw in Tiled.
2. **Bake.** Offline. Convert, split, check, write.
3. **Baked file.** Frozen. Lives in content. Hashed.
4. **Load.** At boot. The same code on the shard and in the wasm.

Every mature engine has these four layers. None parses an editor file at runtime. [R1]

## 7. The decisions

### 7.1 Home: echo_image, for now

**Decided (2026-11-13).** Build it in echo_image first, in small reviewable steps: the core in
`pkg/level/`, the baker in `pkg/level/bake/`, the Cardinal side in `pkg/levelstate/`. The aim is a
solid reference that can become a world-engine plugin, or an example a World Engine skill builds
other games from.

**Why here first.** The design still moves. A world-engine PR and a pin bump per change is slow
while it does.

**Rules that keep a move a copy, not a rewrite.**
- The core imports only `box2d` and the standard library. No Cardinal, no game words.
- echo_image's own words (kinds, the spawn type, rules) live in its game package and are injected.
- Follow world-engine's rules from day one. [code] Its lint bans `encoding/json` outside tests, so
  use `goccy/go-json` and test its strict mode. Maths the predictor links uses Box2D's FMA-free
  helpers. The Cardinal component's name is a long-term contract: renaming it breaks every snapshot.

**Cost.** No other game can import it yet. echo_image's module path can't be fetched. [code]

**When a second game needs it, move it; don't copy it.** An example is enough for game policy. Code
that must give the same bits on every platform drifts when copied. On the move, world-engine's
determinism checks (hard-wired to `pkg/box2d`) must add the package. [code]

**Rejected.** Its own public module now (two Box2D pins, a second release train).

### 7.2 The baked file

**What.** One JSON file per map.

| Field | Holds |
|---|---|
| `format` | A whole number. A reader refuses a number it does not know. |
| `solids` | Wall pieces. Each has `src` (the drawn object it came from), `kind`, and one shape: polygon (3-8 points), circle or capsule. |
| `outlines` | Closed loops around walls, counter-clockwise: they keep things out. Each has `src`, `kind` and its points. |
| `edges` | Closed loops around the play area, clockwise: they keep things in. |
| `objects` | Typed points, such as spawns. Each has `id` (unique in the map), `type`, `at` and `props`. |

**Rules.**
- Every coordinate is a whole number. One unit is 1/1024 m.
- Shapes are final and in world space. No rotation and no trig at load.
- One canonical byte form, one shape per line, so a diff shows which shape changed. The loader re-encodes what it read and refuses the file if the bytes
  differ. This check is what makes decoding strict: Go's JSON decoder alone accepts duplicate keys,
  wrong-case keys, `-0` and `null`. [test]
- Each polygon is written in the point order Box2D's own hull function returns, so "the hull
  equals the input" is an exact check. [test]
- The map's ID is the file name. Its hash is sha256 of the file's bytes.
- An unknown key, kind or type is an error.

**Why.**
- A whole number divided by 1024 is exact in Go, JavaScript and C#. No float parser to trust.
- Whole numbers make the baker's geometry exact.
- Same bytes, same shapes. Quantum says its own bake is not deterministic, so every peer loads the
  same baked asset. [R1]
- Text diffs stay readable at arena size.

**Growth.** A new section (regions, derived data) arrives with its first user and raises `format`.
Newer readers still read older files. A change of meaning means "rebake everything"; the Tiled
sources are committed. Never an up-converter.

**Rejected.** Decimal floats. A binary format. Reading Tiled JSON at runtime. Bits in the file. A
`requires` list (strict decoding plus one number already refuse the unknown).

### 7.3 Walls are content

**The rule.** Only what changes during play is state (an entity). A wall never changes, so it is
content, loaded from the file. Which map is live can change, so that one small thing is state.

**What.** The core builds every shape, in file order, on one static body, in a Box2D world that is
never stepped. On the shard the plugin owns that world. The predictor builds its own with the same
function.

**In echo_image.** Movement and shots query the level's world, not physics2d's. Areas stay on
physics2d: they only ever meet Bodies and Echoes. [code]

**Why.**
- Same bytes + same code + same order = same walls on both sides.
- Zero cost per tick. Zero bytes in snapshots.
- A restart rebuilds walls from the build, not from old state.
- It is the standard: Quantum's static colliders, Source's world brushes. [R1]

**Cost.** The shard holds two Box2D worlds (physics2d's and the map's).

**A game whose Box2D step must push bodies off walls** fills physics2d's world with the same build
function, through one small new entry point in physics2d. That is a world-engine change. It comes
with the first such game.

**Rejected.** Walls as ECS entities. Bodies added straight to physics2d's world (unsupported).
A physics2d-owned path as the only path (the predictor cannot link physics2d).

### 7.4 Shapes

**Settled.**
- The map edge is one closed loop, wound so the play area is inside. From inside it stops Bodies,
  shots and rays at every corner. [test]
- The loader checks the loop's winding. A loop wound the wrong way still stops slow walking, so a
  playtest looks fine, but shots and landing rays pass through it. [test]
- Ties: when a shot meets two map shapes at the same moment, it stops at the same point either
  way. Only the hit's kind (wall or edge) can differ: Box2D reports the shapes in build order. 3.3%
  of test shots tied, and the shape changed with build order. [test] No tie rule is built: the shard
  and the predictor build the same file in file order, so they agree, and nothing reads a map hit's
  kind yet. The rule (the earlier shape in the file wins) comes with the first rule that does. [code]
- Landing needs something solid to ask "is this point in a wall?". A hollow loop always says
  no. [code]

**How wall pieces meet: settled by step 0.** All three designs said "split a concave wall into
convex pieces; pieces may touch". The seam test said no. [test]

| Layout | Result |
|---|---|
| One convex shape, apart from the others | Clean |
| Two pieces sharing a straight face, or overlapping | A Body hooks in one direction. Slower from 45°. Stuck for good from about 83° |
| An L split along its own diagonal | No stall. Up to 39% slower around the outer corner |

The cause: where a Body rests on one piece, Box2D also reports the corner of the next piece. That
"ghost" corner blocks it.

**The fix: outline + fill.** Box2D's own cure for ghost collisions is the chain shape [R4].
- Each wall is one closed chain outline, counter-clockwise, so it blocks from outside. It stops
  movement. A straight run has no seams at all.
- Convex fill pieces under their own kind bit answer "is this point in a wall?" for landing and
  shots. A hollow outline can't. Not built yet: the fill kind comes with the first concave wall.
  Until then, a wall to land out of is a convex solid, and a Body in a hollow outline walks out.
  The map rules still refuse a hollow outline a Body fits in: open space no Body walks to (7.11).
- The mover meets a chain as Box2D's chains collide: from the front only, and with ghost vertices
  (where the Body is over the next segment's face, the joint's corner plane is dropped). The pinned
  `CollideMover` does neither, so echo_image's mover adds it. Built and tested:
  `physics/mover`, `TestChainSlidesLikeTheSolidItOutlines`. [test]

**What the test shows.** A chain outline against the solid it outlines, at press angles 0° to
89.9°, radii 0.3 to 1 m, 5 and 12.5 m/s, 20 and 60 Hz. [test]

| Case | Result |
|---|---|
| A straight wall in three segments, both ways | Never stalls |
| L and T: round outer corners, into and out of inner corners | Never stalls |
| Ridges of 2° to 70° (convex bends) | Never stalls. After a ridge of 20° or more the Body can end a little behind: up to 1.3 cm walking at 20 Hz, 1 cm at 60 Hz, 16 cm dashing (12.5 m/s) at 20 Hz. The cast stops at each joint, which costs one of the mover's 5 passes |
| Valleys of 2° to 70° (concave bends) | Never stalls while there is room to slide |

**Limits that stay.**
- Steps under 1 cm (2 × LinearSlop) still hook. The bake must flatten them. [test]
- Pressed into a valley under about 10°, the Body is wedged, and Box2D's plane solver (20 rounds)
  doesn't settle two near-parallel planes. It jitters up to 4.7 cm a tick, on a chain or on solid
  pieces alike. A mover limit, not a chain one. [test]

**Rejected.** A rotated-box shape (trig at load). A third-party polygon-union library in the
runtime.

### 7.5 Kinds

**What.** Each shape has one kind name. The game hands the library a table: name → category bits
and mask bits. The library copies the bits. It never reads them. The kind table and the object
types (7.6) together are the game's schema (`level.Schema`), which Decode, Encode and Build all take.

**In echo_image.** `wall` and `bounds`, from `interact`, with mask = all bits. That is today's rule.

**Why.** The bits are the game's: they sit in snapshots and must never be renumbered. What a kind
blocks stays where it is today, in each query's mask and in content rules. Source works the same
way: contents on surfaces, masks on queries. [R4]

**Rejected.** The library assigning bits. Bits or masks in the file. Built-in kinds such as "wall".

### 7.6 Objects

**What.** An object is `{id, type, at, props}`. The game registers a Go struct per type
(`level.Types{"spawn": reflect.TypeFor[Spawn]()}`), one struct per type. Props decode strictly into
it, and are written as Go's JSON encoder writes that struct, so every field is in the file. The game
reads typed lists: `level.Objects[Spawn](m)` returns the spawns in file order. Built in step 1b.

**In echo_image, day one.** One type: `spawn {team}`. The dev spawn system takes positions from
the map. Nothing else. Built in step 5: a map holds one spawn per player, and each team's Bodies
take its spawns in file order, as team games' maps do (one start per slot). The boot fails if any
map has fewer spawns for a team than the roster puts on it. Picking a free spawn, as engines do for
a respawn or a late join, comes with the first of those.

**Later, with the first map-placed entity.** A spawn hook (type → constructor), and a tag on what
it spawned, so a map change removes exactly that set. Quantum does this. [R1] Designed, not built.

**Why.** Every engine uses a name → factory registry filled by game code: Quake, Source, Unity
bakers, Quantum. [R1]

**Rejected.** Warn and drop an unknown type (Quake and Source do; we fail). Untyped property bags.
Entity spawning now (no user).

### 7.7 Which map is live

**State holds** one singleton component, `Current`: map ID, the file's SHA-256, an epoch (a counter
that rises on every change), and `Next`, the map a change asked for.

**Derived, in plugin memory:** every map's built Box2D world, and which one is live.

**One system, "reconcile",** runs first in every tick, and once at Init:
1. A change was requested → take the new ID, raise the epoch.
2. The ID is unknown to this build → stop: "world purge".
3. The hash in state differs from this build's → take the build's hash, raise the epoch.
4. The built world does not match state → swap to the right one.
5. The epoch rose, in a tick → emit one `Changed` event (previous ID, new ID, epoch).

That one path covers boot, restore, map change and reload. Every map is loaded and checked at boot,
so a change is a swap. A restore of another map swaps memory without `Changed`: state did not change.
A snapshot from before the plugin has no `Current`, so the tick creates it, and that is a change.

**The API.** `levelstate.NewPlugin(Config{Maps, Schema, Default})` decodes and builds every map, or
returns the first error. `Register` it before every other PreUpdate system. Systems read
`Engine()` (the live map's Box2D world, queries only) and `Map()` each tick. Built in step 2.

**Who picks the map.** Content, for now (`default_map`). Later the match owner calls
`Change(w, id)`. The map is live from the next tick, so a whole tick reads one map. No client
command can pick a map.

**Why.**
- Cardinal has no hook after a restore, and Init runs before it. A first-in-tick reconcile is the
  engine's own idiom; physics2d does it. [code]
- Running first means movement sees the right walls on the first tick after a restore.

**Cost per tick.** One lookup and two compares.

**Three engine facts that shape it.** [code]
- The plugin must be registered above the input system. Order within a hook is registration order.
- `Changed` is emitted only from the tick, never from Init. Init's events leak into the first tick.
- A panic in a system is not recovered, and the shard stops. So "unknown map" stops in the tick,
  where a purge clears it. Never at Init, where a purge cannot.

**Open.** Step 3 is your call. See §11.

### 7.8 Content in echo_image

- `content/arena/tuning.json`: the list of maps and `default_map`. Loaded by `content.Load`.
- `content/arena/<id>.level.json`: baked files. Loaded by the library's own strict decoder.
- `maps/` at the repo root: Tiled sources. Not embedded. Not in SimVersion.
- Tests declare small maps in code and pass them in before boot, through the content overlay.

**Why.** Today's embed pattern and SimVersion already cover these files, so the web client provably
holds the same bytes. An explicit list makes "which maps ship" a reviewed line.

**Rule-book change.** One sentence: map files are decoded only by the level library.

### 7.9 Clients

- **Web.** One new wasm export returns the current map's shapes. The client draws grey boxes from
  it. There is no TypeScript parser, so nothing can drift.
- **Camera.** Follows your Body and clamps to the map edge.
- **Wire.** The map ID rides in body_sync (`MapID`), next to SimVersion, which already pins the
  file's bytes. Hash and epoch join when a client needs them, such as a Unity client that reads the
  file itself. Geometry never goes on the wire.
- **Unity.** Reads the same baked file to draw. It gets bit-identical prediction only if it runs
  the Go sim. A second collision builder in C# is refused.
- **Art.** Later: a client-only file keyed by `src` IDs. The gameplay file never names art. League
  and Dota keep the two apart. [R6]

### 7.10 Authoring and bake

- **Editor.** Tiled 1.12, object layers only. It is the one maintained 2D editor with polygons,
  typed classes and a project file. [R2]
- **Schema.** The Tiled project's types are generated from the game's Go code. A test fails when
  they drift.
- **Baker.** Go. It reads Tiled's JSON directly, so CI needs no editor.
- **Stages.** Read → fill defaults → convert (pixels to meters, y up, rotation) → snap to grid →
  split concave shapes → encode → load with the runtime's own code → check → write.
- **Stale files.** A test rebakes every map and compares bytes.
- **Conventions.** 1 tile = 1 m. Origin = canvas centre. Hidden objects are still baked.

**Tiled traps the baker must handle.** Checked in Tiled's source. [test]
- Tiled 1.12 writes `"version": "1.11"`. Gate on `tiledversion`.
- Object IDs change on cut and paste, and two git branches can merge into duplicate IDs. Tiled
  accepts both silently. The baker rejects duplicates, and nothing may treat an ID as permanent.
- Tiled 1.10 and 1.11 load a capsule as a rectangle and save it that way. Everyone uses 1.12.
- An ellipse may not be a circle. Templates leave out most fields. The baker rejects both.
- Tiled does not reload a regenerated project file while it is open.

**Rejected.** LDtk (no polygons, frozen). An exporter written as a Tiled script (a second language,
and the editor in CI). Tile layers in the first version.

### 7.11 Validation

One tier: errors.

| Runs at | Checks |
|---|---|
| every load (shard, wasm, bake) | format known; canonical bytes; kinds and types known; props strict; each polygon is its own convex hull; each loop has 4+ points and the right winding; segments and capsules longer than LinearSlop; every point inside Box2D's world |
| bake and shard boot | the game's rules, on the built world, through Box2D queries, for each champion radius: spawns clear of walls; every spawn reachable from the first; map sealed; every clear cell reachable; every wall cell a pass reaches lands where Bodies walk (F26). Built at boot in step 6 |
| bake only | Tiled rules: every object has a class; no tiles, templates or infinite maps; no polygon crosses itself |

A message names the file, the object, its ID and its position.

**How the boot rules run (step 6).** `level.Grid` samples the map on 0.25 m cells (the landing
step) for a circle of the Body's radius: one Box2D AABB query per 8 x 8 block, then one circle
overlap per cell of a block with a shape near. A scanline flood from the first spawn, and one from
the ring around the grid, answer reach, sealed and clear cells. Landing runs the mover's own search
(`mover.LandWhere`) with the grid's cells as its "does it fit" answers: the same queries, made once.
A gap narrower than a cell can slip through, as in every sampled check. A landing the rules missed
is logged at run time. Cost, measured: about 2 ms a boot on the 100 x 100 m open map, about 5 ms
with two walls; the world tests pay about 0.2 s in all (first draft: 11.6 ms and 35 ms).

**Why one tier.** Engines have a strict mode for CI. We make it the only mode. [R1]

### 7.12 Seams for later

None of these is built now. Each comes with its first user.

| Capability | How it enters | What it breaks |
|---|---|---|
| Map-placed entities | spawn hook + tag (7.6) | nothing |
| Regions (zones, brush) | objects with a shape | nothing (`format` +1) |
| Nav or vision data | a bake pass the game registers, plus a section | nothing (`format` +1) |
| Tile maps | a bake-only importer | nothing |
| Box2D-stepped games | static entry point in physics2d (7.3) | a world-engine change |
| Several maps at offsets | levels placed at whole-number offsets | state shape: one purge |
| Destructible wall | a "removed" set in state; the world is rebuilt from it | state shape: one purge |

### 7.13 Tests

- **Library.** Golden files. One failing file per rule. Byte round trip. A digest of the built
  shapes plus a fixed battery of queries, compared on amd64, arm64 and wasm. A shuffle test for
  order independence.
- **echo_image.** The ten wall tests declare maps before boot; the ECS wall helpers go. The
  prediction recording gains walls, a seam, the edge and a landing, at both rates. The wasm
  replays it.

## 8. Who owns what

| The library | The game (echo_image's `arena`) |
|---|---|
| file format, strict decode, hash | the kind table (from `interact`) |
| building Box2D shapes | the `spawn` type |
| the "which map" component and reconcile | rules with game numbers (champion radius, landing reach) |
| baker, Tiled reader, schema export | the maps list and the default map |
| rule helpers (a flood fill over Box2D queries) | what a map change does to a match |

- **The library never holds:** a kind name, a team, a spawn, a tick rate, a command, a wire event,
  an art path.
- **The game never holds:** a map parser, shape building, hashing, Tiled parsing.

## 9. Is it generic?

Walked on paper through three other genres taken from shipped games. [R5]

| Genre | Needs beyond the first version | Breaks format or API? |
|---|---|---|
| Platformer (Box2D pushes bodies off walls) | physics2d static entry; chain pieces for smooth ground | No. Additive. |
| Room dungeon built at runtime | tile importer; rooms at offsets | Format no. State shape once. |
| Lane map with vision and nav | regions; derived data sections | No. `format` +1 each. |

**Honest limit.** This is argued, not shown. Only the first user proves anything.

## 10. Build order

All in echo_image (7.1), one reviewable step at a time.

| # | Where | Step |
|---|---|---|
| 0 | mover | **Done.** The mover meets chains as Box2D does, so outlines don't hook (7.4) |
| 1 | `pkg/level` | **Done.** The core: format, strict decode and encode, kinds, build into Box2D |
| 1b | `pkg/level` | **Done.** Objects and their types, and the game's schema |
| 2 | `pkg/levelstate` | **Done.** The Cardinal side: component, reconcile, `Changed` event |
| 3 | game | **Done.** `arena` package. An open default map (a 100 x 100 m edge, two spawns where they are today). Cardinal side registered first. Map ID in body_sync |
| 4 | game | **Done.** One change: movement, shots and the predictor use the live map's world. Wall tests declare maps before boot (`onMap`). The recording meets walls, the edge's joints, a landing and a range end, at both rates. No tie rule yet (7.4) |
| 5 | game | **Done.** Spawns come from the map: one per player, in file order per team. The boot checks every map has enough, through the plugin's rules hook (`Config.Check`), which step 6 fills |
| 6 | game | **Done.** The map rules at boot (`arena.CheckMap` over `level.Grid`, 7.11), once per champion radius. A failed landing is logged. Test maps start from the shipped map, so they keep its edge and spawns |
| 7 | client | wasm export, greybox view, camera |
| 8 | `pkg/level/bake` | Baker, Tiled reader, project generator |
| 9 | game | Tiled sources, generated project, staleness test, the first real map |

- Step 4 must not be split. Server walls without predictor walls make every wall stop a correction (F8).
- Another game can use it only once it moves to world-engine (7.1).
- No step needs a purge. A snapshot without the new component restores cleanly, and the reconcile
  creates it. [code]

## 11. Decisions for you

| # | Decision | My pick |
|---|---|---|
| 1 | **Home.** | **Decided:** echo_image for now (7.1) |
| 2 | **A restored snapshot whose map bytes changed.** Treat as a map change, or refuse and purge | Map change |
| 3 | **How wall pieces meet** (7.4) | **Decided:** outline + fill. Step 0 proved it |
| 4 | **Names.** `level`, `levelstate`, `arena` | `arena` is one letter from `area`: pick another if that hurts |
| 5 | **The first map.** Size, walls, spawns (GDD §8 is open) | Yours. I invent none |
| 6 | **Camera** follows and clamps | Yes |

On 2: shipped games refuse [R3]. I pick "map change" because echo_image already resumes a snapshot
on the content its build carries, and refusing means a purge after every map edit. A Body left
inside a new wall lands out by the existing rule.

## 12. What is proven, what is not

Experiments ran on a private copy of this repo, with the real mover, on 2026-11-13. Step 0's test
is in the repo (`physics/mover/mover_test.go`).

**Proven**

| Claim | Result |
|---|---|
| A chain outline slides like the solid it outlines | Never stalls at any angle, radius, speed or rate tested (7.4) |
| Walls collide with the same bits natively and in wasm | Same digest on amd64, amd64 v3 and wasm: 54 shapes, 10,000 queries. First proof with walls |
| A world that is never stepped answers every query | Yes, right after the shapes are made |
| Build order does not change results | 0 of 1,050,000 move, land and point results differ over 21 orders. Shots need the file-order tie rule |
| Whole numbers / 1024 are exact | All 33.5 million values, same bits in wasm |
| "Polygon equals its own hull" is a sound check | Yes, if the baker writes the hull's own point order |
| The edge loop holds from inside | 0 of 1,800 random runs escape. All shots and rays stop |
| It is fast enough | 1,000 solids load in 4.4 ms (10.7 ms in wasm). All game rules on a 40 x 24 m map take 62 ms at shard boot. In wasm they take 0.22 s, so the client skips them and trusts SimVersion |
| Content packaging | Today's embed pattern matches the files. One changed byte changes SimVersion. The test overlay serves a map |
| Cardinal order | Init, then restore, then ticks. The restore replaces Init's singleton |

**Disproven**

| Claim | Result |
|---|---|
| Wall pieces may touch | No. See 7.4 |
| A directory listing can be the map catalog | No. The test overlay cannot list files. Hence the explicit list in 7.8 |
| Tiled's `version` field tells the version | No. See 7.10 |
| Tiled object IDs are permanent | No. See 7.10 |

**New problems found**

- **Landing can put a Body outside the map.** From a centre exactly on the edge, the landing ray
  ran along the edge line and missed it. This belongs with finding F26.
- **A Body that starts outside the edge can tunnel in** at dash speed. Nothing puts a Body there
  today.
- **Formula code outside Box2D can still fuse** (FMA). The probe's own grid math did. Rule helpers
  the predictor links must use Box2D's helpers.

**Not tested**

- arm64. It is the other platform that fuses by default. CI runs it.
- Keeping solid pieces and dropping the ghost corner in the mover instead. Outline + fill works, so
  it wasn't tried.
- A real snapshot restore. Cardinal's test world has none, so tests can only imitate one.
- Whether world-engine's maintainers take the packages when they move.
- **No critic pass.** Three critics were started and then stopped to save usage. Nobody has
  attacked this merged document yet.

## 13. Honest costs

- **No other game can import it** until it moves to world-engine (7.1).
- **World-engine may say no** to the move. Then it becomes its own public module.
- **No hot reload at first.** A map edit means: rebake, reload the shard, rebuild the client.
- **Freeform departs from the genre's files.** League, Dota and Brawl Stars use grids of flag
  cells [R6]. A grid gives cheap symmetry and vision checks. We pay with a splitter, seams, and
  flood-fill checks that sample every 0.25 m and can miss a smaller gap.
- **The biggest new code is offline:** the splitter and the Tiled reader.
- **Unity** draws from the same file, but predicts with the same bits only if it runs the Go sim.
- **Thin research.** No primary source for Riot's or Blizzard's map internals. Little on hot reload.
  Nothing on the collision of freeform top-down games (Battlerite, Supervive).

## 14. Out of scope

Streaming and chunks. Procedural generators. Encounter and wave tables. Moving platforms and doors
(they are entities, not map geometry). Art and rendering layers. Symmetry tools. A map editor of
our own.

## Appendix A. Where the code lives

**Today**

| What | Where |
|---|---|
| The mover and landing | `shards/game/internal/physics/mover/` |
| One tick's move | `shards/game/internal/player/motion/motion.go` |
| The shot's map cast | `shards/game/internal/projectile/flight/flight.go` |
| Systems that pass a world | `internal/player/movement_system.go`, `internal/projectile/flight_system.go` |
| The predictor's maps | `shards/game/internal/predict/predict.go` (`loadMaps`, `Reset`) |
| Content loader and embed | `shards/game/content/embed.go` |
| Catalog and wiring | `shards/game/shard/catalog.go`, `shards/game/shard/shard.go` |
| SimVersion | `sim.go`, `sim_test.go` |
| Kind bits | `shards/game/internal/interact/interact.go` |
| Dev spawns | `shards/game/internal/player/spawn_system.go`, `content/player/tuning.json` (the roster), `internal/arena/arena.go` (`Spawns`, `CheckSpawns`) |
| Test maps | `shards/game/test/harness_test.go` (`onMap`, `mapBox`, `mapLoop`), `recordedMap` in `prediction_test.go` |

**Proposed**

| What | Where |
|---|---|
| The core | `pkg/level/` |
| The baker and Tiled reader | `pkg/level/bake/` |
| The Cardinal side | `pkg/levelstate/` |
| The game side | `shards/game/internal/arena/` |
| Baked maps and tuning | `shards/game/content/arena/` |
| Tiled sources | `maps/` |
| Client map view | `client/src/game/arena/`, camera in `client/src/engine/render/stage.ts` |

## Appendix B. How this was made, and sources

Made on 2026-11-13 by 9 research agents, 3 independent designers and 4 verification agents (two
ran experiments). The critic pass was cut.

- **[R1] Engine pipelines.** Quantum maps: `doc.photonengine.com/quantum/current/manual/maps/overview`,
  `.../maps/map-baking`, `.../maps/dynamic-map`. Unity baking:
  `docs.unity3d.com/Packages/com.unity.entities@1.3/manual/baking-overview.html`. Unreal cooking and
  Map Check: `dev.epicgames.com/documentation/en-us/unreal-engine/cooking-content-in-unreal-engine`.
  Quake: `github.com/id-Software/Quake` (`sv_main.c`). TrenchBroom manual.
- **[R2] Editors and formats.** Tiled releases and docs: `github.com/mapeditor/tiled/releases`,
  `doc.mapeditor.org/en/latest/reference/json-map-format/`, `.../manual/custom-properties/`.
  `github.com/adrien-bon/bevy_ecs_tiled`. LDtk: `ldtk.io/api/`. RFC 8785.
- **[R3] Networking.** Quake 3 `sv_init.c`, `sv_client.c`. Agones allocation:
  `agones.dev/site/docs/reference/gameserverallocation/`. Unreal travel. Unity NGO scene
  management. Quantum replay.
- **[R4] Collision.** `box2d.org/documentation/md_simulation.html`,
  `box2d.org/posts/2024/08/determinism/`. Unreal collision overview. Source `bspflags.h`.
- **[R5] Other genres.** Tiled worlds, LDtk JSON doc, Godot tilesets, Spelunky generation
  (`tinysubversions.com/spelunkyGen/`), Dead Cells level design (`deepnight.net`).
- **[R6] Top-down practice.** Riot: `riotgames.com/en/news/story-fog-and-war`. Blizzard on greybox:
  `news.blizzard.com/en-gb/article/23674944/2`. Stunlock on baking:
  `unity.com/resources/stunlock-studios-v-rising`. LoL patch 14.1 notes.

The full research reports, the three designs, the experiment reports and the test code are kept
outside the repo, in `~/echo-map-strategy-research/`. The seam test is in `probes-x1/`. Its files
are `_test.go` files for a package under `shards/game/internal/`.
