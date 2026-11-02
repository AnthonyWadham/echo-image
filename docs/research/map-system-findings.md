# Map system — research findings (2026-10-23/24)

> **These are findings, not the source of truth.** They record what research found and what it
> proposed. Code, [`CLAUDE.md`](../../CLAUDE.md) (how) and [`GDD.md`](../GDD.md) (what) win
> whenever they disagree with this file. Line refs point at the code as of world-engine `d8b0db58`
> (branch `ryan/revert`) and echo_image `3c6edfb`, and may have moved. Re-check a fact before you
> build on it.

## 1. Design answers from the user (2026-10-24)

| Question | Answer |
|---|---|
| Grid-snapped or freeform maps? | **Freeform** shapes |
| Do Bodies block each other's movement? | **No, for now** |
| One map per match, or a new map each round? | **One map per match, for now** |
| Bushes / line of sight in v1? | **Not now** (keep the design able to add it) |

These aren't in the GDD yet. §8 "Map design" there is still Open.

## 2. Verified engine facts

Each one was checked by reading the code and, where marked *tested*, by running scratch Go programs.

**Movement vs walls**
- Box2D never pairs kinematic bodies with static or kinematic bodies. Only dynamic proxies query
  those trees (world-engine `pkg/box2d/broad_phase.go:406-421`). physics2d maps both Kinematic and
  Manual to `box2d.KinematicBody` (`pkg/plugin/physics2d/internal/create.go:125-136`).
- So echo_image's player Body (`shards/game/internal/player/spawn_system.go:51-57`, kinematic) walks
  through walls and through other Bodies. *Tested* on a copy of the real game: 0 contacts, full travel.
  The comment "solid vs. other bodies and walls" at `spawn_system.go:56` is false.
- Sensors are the exception: a static sensor *does* fire TriggerBegin/End for kinematic and Manual bodies.
- Box2D's geometric character mover is ported: `World.CastMover` and `World.CollideMover`
  (`pkg/box2d/world_query.go:546`, `:656`), plus `SolvePlanes` and `ClipVector` (`pkg/box2d/mover.go:47`, `:92`).
  The port was taken from upstream main `4996f6e2` (2026-03-28), not the v3.2.0 tag. The algorithm
  is the same, but some API names differ.
- Box2D docs (https://box2d.org/documentation/md_character.html): "I recommend the geometric mover for
  networked games where rollback is needed."

**Mover behaviour** (*tested*, scratch `wf1-spike`, `M-mover`)
- It stops flush at a wall, resting 5 mm (LinearSlop) inside the face. It slides along a wall
  cleanly and rounds corners.
- **Seams:** when two boxes share an edge, the mover gets stuck before the seam whenever input presses
  into the wall at 85° or more. At smaller angles it slows for one tick. One merged shape never sticks.
  The current mover passes that seam at 84-89° at both rates (foundation review probe, 2026-11-01), so
  this reason to reject flush seams no longer holds; decide the seam rule when the arena lands.
  Chain loops with a collinear vertex at the seam behave the same as two boxes, because chains don't
  help the mover.
- **Deep overlap:** once a circle's centre is on or inside a wall, `CollideMover` returns 0 planes and
  the mover walks out the far side (`world_query.go:641-642`, upstream "todo handle deep overlap").
  Spawns and teleports must never land inside a wall.
- **Plane order:** `SolvePlanes` and `ClipVector` depend on plane order, and that order follows the
  static tree's build order. Sorting planes by `(Normal.X, Normal.Y, Offset)` gave bit-identical
  results across 5 world variants and across amd64 vs js/wasm. Don't sort by ShapeID, which differs
  between worlds.
- Queries do not skip sensors or the mover's own body. Filter by category/mask, for example
  mask = Environment only.
- Queries called inside a Box2D callback (world locked) silently return "no hit". Calling them from a
  Cardinal system is safe, since the world is unlocked outside Step.
- The query filter is two-way (`pkg/box2d/shape.go:1908-1910`), so a wall's MaskBits must include the
  query's category: CategoryBody for movement, CategoryProjectile for shots.
- Cost: about 0.5 µs per player per tick with 400 boxes.
- The upstream sample's loop: up to 5 passes of (CollideMover → SolvePlanes → CastMover → move),
  stopping when a step is under 0.01 m, then ClipVector on the velocity. Pogo, gravity and ground
  logic can be dropped for top-down.

**Shapes**
- Convex polygons go through `box2d.ComputeHull` (`internal/create.go:206`), so a concave polygon
  silently becomes its hull. Nothing rejects it. A polygon holds at most 8 vertices.
- Chains are one-sided. A CCW loop blocks from outside and a CW loop from inside. Rays and sweeps are
  one-sided too. CastMover is one-sided against chains, while CollideMover is two-sided.
- Chains can't be sensors. In-place filter/material changes skip chain slots.
- A 2-point open chain panics the tick, and a 3-point open chain has 0 segments.
- Shapes with zero CategoryBits or MaskBits are invisible to every query.
- **One bad shape blocks the rest:** `ReconcileFromECS` returns at the first failing entity
  (`internal/reconcile.go:42-46`), so every entity with a higher ID never gets a body, and the error is
  logged every tick. If the bad shape exists at Init, the shard panics. Validation must catch bad
  geometry before it reaches physics2d.

**Lifecycle**
- Init systems run first, then the snapshot restore swaps in the whole entity table
  (`pkg/cardinal/world.go:184-189`). Walls built only at Init are replaced by the snapshot's
  (possibly old) walls after a restart. `world reload` keeps state unless `--purge` is passed.
- After a restore, physics2d heals on the first PreUpdate through `ReconcileFromECS`.
- The data plugin: when a restored snapshot's config hash differs from the embedded config, it keeps
  the embedded config, warns and rewrites the manifest (`pkg/plugin/data/system/state.go:165-185`).
  Files fetched by a Resolve hook are not version-pinned and not in the manifest, so a change to one is
  silent (`system/definition.go:33-38`). `EmbedSource.Fetch` returns the sha256 hex of the bytes
  (`system/source.go:34-41`).
- Entity IDs are reused smallest-first and carry no generation, so never store an ID across ticks.
- Restore aborts if a snapshot entity holds a component this build doesn't register. Renaming a map
  component means a `world purge`.
- An Init system registered **after** the physics plugin creates walls that aren't in Box2D during
  tick 0's pre-pipeline PreUpdate.

**Cost of walls as ECS entities** (*tested*, scratch `wf1-bench`, i9-11900K)

| Layout | Per tick | Per snapshot |
|---|---|---|
| One entity per wall | ~0.27 µs per wall | 862 B per wall (uncompressed) |
| One entity, N shapes | ~31 ns per shape (about 9× cheaper) | ~602 B per shape |

- With 200 per-wall entities, a snapshot is about 172 KB. At SnapshotRate 10 that's about 345 KB/s per shard.
- Locally, `world start` runs with Debug and writes a snapshot every tick, so 200 wall entities add
  about 1 ms per tick there.
- Almost all of this cost is physics2d's ECS mirror (gather, shadow compare, orphan check). The Box2D
  step itself is about 2%.
- Cardinal does not compress snapshots.

**Determinism**
- `pkg/box2d` forbids FMA, and `TestNoFusedMultiplyAdd` enforces it. Game code *outside* box2d (for
  example `pos + v*dt`) can still be fused on arm64 or GOAMD64=v3. Shared server/client sim code
  must round its own products (`float64(a*b)`, or `box2d.MulAdd` and friends).

## 3. Rampage lessons

Sources: rampage-backend `shards/gameplay/system/map_loader.go`, rampage-client `TiledShapeColliderBuilder.cs`.

| Bug class seen in rampage | Evidence |
|---|---|
| Two map loaders (Go server, C# client) drifted | Client skipped rectangle obstacles for about 3 months, so dash prediction tunnelled through walls (client `b3df40414`). The loaders still differ on layer-name matching, centring and rotation |
| `.tmj` hand-copied between repos | Drift windows of 4, 11 and 18 days, found in git history |
| Origin derived from painted tiles | One edit moved every obstacle (+3, −0.5) tiles |
| Tiled visibility used as gameplay data | Hidden objects lost their spawn zones (`a2d20fc2d`) |
| Map loading only ever adds | Invisible walls carried across matches (`284e95b76`, RAM-705) |
| Whole map sent over the wire | 78,608 B `.tmj`, broadcast up to 7 times per join. Tower floors once reached ~19 MB, past the NATS cap |
| Two "blocked" models (Box2D vs nav grid) | Pits, chests and spawns placed inside walls (RAM-1036, RAM-1231, RAM-1178) |
| Silent fallbacks | An unknown map ID loads neo_street. Zero-size rectangles are skipped with no log |

Not a real bug: "off-centre rectangle chains failed to block on the server". This did not reproduce,
and the actual cause was the client skipping rectangles.

## 4. How shipped games do it

Sources were gathered by a research agent. Confidence tags: HIGH = primary source.

- **The static map is baked, immutable data on both sides, never snapshot state.** Photon Quantum
  bakes static colliders into the Map asset. They are "stateless and immutable at runtime" and are not
  entities (https://doc.photonengine.com/quantum/v3/manual/physics/statics, HIGH). Source moves
  `prop_static` out of the entity list, so it is never networked (source-sdk-2013 `vbsp/staticprop.cpp`, HIGH).
- **Both sides check that they loaded the same map.** Source's `CSVCMsg_ServerInfo` carries `map_name`
  and `map_crc` (csgo-demoinfo `netmessages_public.proto`, HIGH).
- **Each tile type flags what it blocks.** Brawl Stars `tiles.csv` has BlocksMovement,
  BlocksProjectiles, IsDestructible and HidesHero (data-mined, MED). LoL's navgrid has Wall, Brush and
  TransparentWall flags (MED).
- **Vision is server-side, and hidden enemies are not sent** (Riot, VALORANT fog-of-war post, HIGH).
  For later.
- **A map change is an explicit event.** Quantum sets `frame.Map` only on a verified frame (HIGH).
- **Box2D FAQ:** many boxes for terrain snag characters on internal corners, so use chains for dynamic
  bodies. For the mover, merged shapes are what matter (see §2).

## 5. Tools

- **Tiled 1.12.2** (May 2026): active, with typed classes and enums in the `.tiled-project`, capsule
  objects (new in 1.12), and CLI export (`tiled --export-map json`; needs `xvfb-run` on Linux).
  Its JSON uses pixels with y pointing down, and rotation is clockwise around (x, y). Flipping y
  reverses polygon winding.
- **LDtk:** no release since 1.5.3 (January 2024) and no polygons. Rejected.
- **Go parsers:** `lafriks/go-tiled` reads only `.tmx` (XML), not `.tmj`. Write a few small structs
  with `encoding/json` instead.
- **Blender → GLB:** only for art-led dressing later. Custom properties arrive as glTF extras, and
  `qmuntal/gltf` reads them in Go.
- **three.js:** InstancedMesh or BatchedMesh, both working on WebGPU in r186. For static shadows, set
  `shadow.autoUpdate = false`. Free CC0 kits: KayKit Prototype Bits, Kenney Mini Arena.
- **Vite:** files imported from `src` get hashed URLs, while `public/` files do not.

## 6. Proposed design (a proposal, not a decision)

- **One baked map file per map**, in our own format: world units, +y up, explicit origin.
  - It holds bounds, spawn points per team, and elements, each with a `kind` and a shape (box, circle,
    convex polygon ≤ 8 vertices, capsule).
  - It lives in `shards/game/content/arena/maps/`. The server embeds it, and the client imports the
    same file.
  - Neither side ever reads Tiled's format.
- **Each kind says what it blocks** (Body, projectile, later sight).
  - Every shape gets CategoryBits = Environment, and MaskBits = the channels its kind blocks.
  - **Superseded (2026-10-24):** a shape's CategoryBits = its kind bit, and its MaskBits = all bits. Each query's mask
    lists the kinds that block it. Channels can't combine two pass effects. See `character-foundation.md`.
  - The kind table is data, so a new kind needs no code.
- **Validation at bake time and at load**: convex, 3–8 non-collinear points, non-zero bits, spawns not
  inside or touching a blocking shape, no flush seams between blocking shapes. Landing's preconditions
  too (F26 in `foundation-review.md`): a flood fill of clear cells from the spawns rejects any clear
  cell it doesn't reach (a sealed pocket) and any blocked cell farther than `mover.LandReach` from a
  reached one (a wall too deep to land out of). Any failure is a loud error. With freeform maps, the simplest seam rule is to *reject* two blocking shapes that share a
  collinear edge, and tell the designer to make one shape. Merge them with a polygon union only if
  that rule becomes annoying.
- **Bounds:** a chain loop wound to block from inside (CW).
- **Server `arena` feature:**
  - **The map is content, not state** (F8 in `foundation-review.md`, 2026-11-01; this replaces a map
    entity and its rebuild check). One Cardinal-free `arena` package loads the baked map from
    `content.FS` and builds a static `box2d.World` with box2d's public builders (`MakeOffsetBox`,
    `MakeRot`, `ComputeHull`, `CreateChain`), which the FMA scan already covers; add the package to its
    shared core. The shard and the predictor build it from the same file, so every wall stop, slide and
    shot stop is predicted to the bit. The mover, `Land` and a shot's map cast query it; Bodies and
    Echoes stay out of it, as shots meet them from their rows. It never enters snapshots, and a new map
    is new content, so a new build with its own `SimVersion`.
  - A failed landing is then a bug: `motion.Step` reports it, and `MovementSystem` logs it.
  - Destructibles, if they ever exist, are entities, not arena geometry.
  - `ArenaSync{MapID, Hash}` is sent each tick, so join and reconnect need nothing else.
  - The dev spawn system reads its spawn points from the map.
- **Movement:**
  - Bodies become `BodyTypeManual`. MovementSystem (PreUpdate, before the plugin) runs the mover
    against the arena's world and writes Transform2D.
  - It sorts planes and does FMA-free arithmetic. The mover is a pure function with no Cardinal
    import, so a wasm predictor can reuse it later.
- **Client `game/arena/`:**
  - Builds wall meshes from the same shapes, so the art can't drift from collision.
  - The camera follows the player and is clamped to the map bounds.
  - It checks the sha256 of the raw file text against `ArenaSync.Hash`, and shows "reload" on a mismatch.
- **Authoring:**
  - Tiled object layers with typed classes.
  - The converter is used only by a Go test that rebakes every map and fails if the committed baked
    file is stale (`-update` rewrites it), like `TestWireFixtures`.
  - Unknown classes, unhandled rotation and hidden gameplay objects are errors.

## 7. Steps

1. **Movement collides with walls:** the mover, Manual Bodies, and tests.
2. **`arena` feature:** map format, validation (Land's preconditions included), the arena world the
   shard and the predictor both build, `ArenaSync`, spawns from the map, one hand-written greybox map,
   and wall cases in the prediction recording. Server and predictor walls ship in this one step.
   Record the decisions in CLAUDE.md and the GDD.
3. **Client:** map view, camera follow and clamp, hash check.
4. **Tiled converter:** `.tiled-project` with classes, converter, staleness test.
5. **Prediction in wasm** comes with step 2: the predictor builds the same arena world from the same
   file, with no world-engine export.

## 8. Doc traps found (not fixed)

- physics2d `README.md:121` says Manual uses Box2D "only for contact detection", but Manual bodies get no contacts with static or kinematic bodies.
- physics2d `component/collider.go:18-20` says Box2D validates convexity and panics. It does neither.
- `pkg/box2d/oracle_misc_test.go:14-18` says `mover.c` is not ported, but `mover.go` exists.

## 9. Scratch experiments

These may be deleted. They live in `/tmp`.

`/tmp/claude-1000/-home-doglightning-world-engine/aebc2c7f-0598-488d-a49d-ac19fd2e46ae/scratchpad/`:
`wf1-spike` (mover and kinematic tests, `run.sh`), `wf1-bench` (wall cost), `seamtest`, `staticbench`.
