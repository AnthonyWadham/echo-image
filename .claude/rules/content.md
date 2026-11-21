---
paths:
  - "shards/game/content/**"
  - "shards/game/shard/**"
  - "shards/game/internal/arena/**"
  - "shards/game/test/**"
  - "pkg/level/**"
  - "pkg/levelstate/**"
  - "maps/**"
---

# Game data / definitions (`content.Load`)
- **One loader, `content.Load[T](fsys)`,** which the shard and the client predictor share. It decodes
  T's file strictly (an unknown key fails, so a typo never loads as 0), then runs T's `Bind` and
  `Validate`. Any error fails the boot. **Never hand-roll another JSON loader.** (world-engine's data
  plugin, which this replaced, is gone upstream, #1133.)
- **A definition is a Go type implementing `JSONFile()`** (`content.File`). Optional: `Bind()`
  (binds what decoding can't, e.g. `cast.Tuning` binds each ability's hook) and `Validate()` (one
  file's invariants).
- **The shard loads every file once at boot** (`loadCatalog` in `shards/game/shard/catalog.go`), then checks
  the IDs one file names in another: each ability's shots and effect kinds (hooks list theirs in
  `Refs`), and each champion's kit, Echo and Echo area. A champion with no Echo could never be
  eliminated. `TestBadContentFailsTheBoot` pins both kinds of failure.
- **Each system that reads config holds its file** in a typed `Tuning` field, set in `shard.go` from
  the catalog. Content never changes after boot, so a system reads it directly, never a copy per
  entity.
- **Tests change content with `withContent`** (`harness_test.go`): it deep-merges JSON patches into the
  shipped files and boots the real `shard.RegisterWith` from that `fs.FS`; `patched` gives a predictor the same.
  Use it for test-only shot types and champions; never add test content to the shipped files.
  A test with walls declares its map before boot: `onMap` writes a test map into the files, and
  `duelOn` and `fxDuelOn` are `duel` and `fxDuel` on one. The shard's own world has no wall to add.
  World tests play the shipped open map: `patched` lists only it (`onOpen`), so a new shipped map moves
  none of them. A test map starts from it (`openMap`, `openWith`), so it keeps the edge and spawns the
  map rules need.
  A test that reads metrics passes its own meter provider to `shard.RegisterWith`. Never swap otel's global
  one: it stays wired into every later world.
- **Every test Body comes from `spawnBody`.** A test that needs more players patches the dev roster
  (`dev_spawns` in `content/player/tuning.json`), gives it a map with a spawn for each (`twoVsTwo`),
  and adds them with `join`. Players claim free Bodies in roster order.
- **The definition TYPE lives in the feature package that owns it** (e.g. `projectile.ProjectileTuning`),
  **never a central `types/` grab-bag.** Shared sim code keeps the type in its own package
  (`flight.Tuning`), and the feature names it with an alias, or embeds it to add a check
  (`AbilityTuning{cast.Tuning}` adds the kit-size check). Rampage centralizes all definitions in
  `pkg/datadef/types` and 40 feature files import it — that's the layered bucket we reject.
  Definitions reference each other **by string ID**, checked at boot, so feature-owned types create no
  import cycles.
- **JSON content lives under `content/<feature>/<file>.json`**, one `embed.FS` (`content.FS`) —
  `go:embed` can't reach across directories. `JSONFile()` returns the path within that FS (e.g.
  `"combat/tuning.json"`).
- **Map files are decoded only by the level library** (`pkg/level`), never `content.Load`.
  `content/arena/tuning.json` lists the maps that ship (`content/arena/<id>.level.json`) and the
  default; the catalog builds each into the `levelstate` plugin, so a bad map fails the boot.
- **Maps are drawn in Tiled 1.12+ and baked.** `maps/<id>.tmj` is each map's source, and
  `maps/echo.tiled-project` the project generated from the schema: its classes are the kinds and object
  types. `moon run echo:bake` rebakes `content/arena` and the project (`pkg/level/bake`, with the boot's
  own `arena.Rules`); `TestMapsAreBaked` fails when one is stale or a listed map has no source. A tile is
  a meter and the canvas's centre is (0, 0); objects bake in layer order, so each team's spawns go in
  that order; the `bounds` class draws the edge. A wall polygon Box2D takes as no one shape (concave, over 8
  corners, a corner on a line) bakes as its outline plus triangles of fill (`bake.Config.Fills`, the
  splitter in `pkg/level/bake/split.go`): draw an L as one polygon. A wall too thin to cut into pieces
  about 1 cm wide is refused: add a corner across from the thin part, or draw two walls. A loop (an edge, an outline) can't cross itself, or bring two segments
  within 5 mm (Box2D's `LinearSlop`): `pkg/level` refuses it at every load, with Box2D's own
  `SegmentDistance`. `courtyard` (40 x 24 m) is the default map.
- **A map holds a spawn for each player.** Each team's Bodies take its spawns in file order, one each
  (`arena.Spawns`), as team games' maps do. The boot fails if any map has fewer spawns for a team than
  the dev roster puts on it: the game's rules run on every map through `levelstate.Config.Check`.
- **The map rules** (`arena.CheckMap`, once per champion radius) refuse a map where a spawn touches a
  wall, a Body can't walk between spawns, it walks off the map, open space can't be walked to (a
  walled-off room, or a hollow outline a Body fits in), or a wall cell a pass reaches can't land where
  Bodies walk. They sample a 0.25 m grid (`level.Grid`) of Box2D queries, so a gap narrower than a
  cell can slip through. They cost about 2 ms a boot on the open map; keep them free of allocations
  per cell.
- **The client draws the map its predictor decoded.** The wasm's `shapes(mapID)` gives the map file
  as `pkg/level` writes it, in units, so Go alone checks a map and TypeScript only `JSON.parse`s it
  (`MapFile`). `client/src/game/arena/map_view.ts` draws it grey, in meters, again only when a
  body_sync of your sim names another map. The camera follows your drawn Body and stops 1 m past the
  map's bounds (`Stage.follow`). Go owns that box (`level.Map.Bounds`, sent with the shapes): no
  TypeScript works it out.
- **Content is code.** It is embedded and hashed into `SimVersion`, so a shard resumes a snapshot on
  the content its build carries; changing content is a new build.
- **Prefer typed per-feature config structs** over a single stringly-typed global settings bag
  (rampage's `game_settings.json` uses `Parameter string / Value json.RawMessage` — untyped, avoid).
