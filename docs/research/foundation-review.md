# Foundation review

Date: 2026-11-01. Scope: echo_image as it stands, with fixes 1-14, 17, X3 (repo part), X5, X6 and X8 in the tree, uncommitted.

## Verdict

The foundation is sound. Its core matches mature practice:

- One Go sim runs on the server and, as wasm, in the browser. It matches bit for bit at 20 and 60 Hz, and on arm64 too.
- The server judges every hit by rewinding to the shooter's view. Clients never claim hits.
- Every overlap is a Box2D query. Per-tick cost is linear: game systems take 0.12-0.21 ms a tick at 3v3.
- Each component has one writer, receivers sort, and no state lives outside components.

No finding stayed high. The skeptic downgraded both highs to medium. The gaps sit at the edges:

- **Match flow:** a player whose Echo died keeps shooting and blocking.
- **Hit judging:** a moving target is judged at one pose per tick, unlike the screen.
- **Content:** the loader was removed upstream, and a typo key loads as zero.
- **Safety net:** no CI, a test that guards nothing, no random play, no restore test.
- **Cost:** bandwidth, replays and snapshots grow 3-7x at 60 Hz. And 60 Hz can't deploy yet.

## Top 10

| # | Problem | Verdict | Fix | Cost |
|---|---|---|---|---|
| [F1](#f1) | A player whose Echo died keeps shooting and blocking. Nothing owns the match. | bug | Echo death gives the Body an `out` effect. Later, a Match owns seats and rounds. | S now. M and a purge later. |
| [F2](#f2) | A moving target is judged at one pose per tick, unlike the screen. Model: 30% of walking and 77% of dashing crossings differ at 20 Hz. | bug | Sweep each shot against the target's motion over the tick the screen shows. | M+. Re-record both rates. |
| [F3](#f3) | Content loads through a plugin world-engine has deleted. A typo key loads as 0. | not standard | One strict loader, shared by the server and the predictor. | M. Purge. |
| [F4](#f4) | Nothing runs the checks: the hook is off, there is no CI, and nothing runs on arm64. | duct tape | CI runs check.sh, plus an arm64 job. The Go half needs no secrets today. | S-M. Client half needs an SDK release. |
| [F5](#f5) | Tests hand-build extra Bodies. The ally pass-through test guards nothing. | bug | Every test Body comes from spawnBody, through a dev roster. Add a 3v3 benchmark. | S |
| [F6](#f6) | No test plays randomly. A short random run finds your dead Echo mispredicted. | not standard | One seeded random test in main_test.go. YouState skips a dead Echo. | S |
| [F7](#f7) | Pacing aims at a fixed 0.05 s queue. Packet loss turns into coasts (sim: 9-57 a minute at 1% loss). | not standard | The server raises the target after coasts and sends it. | S |
| [F8](#f8) | The map plan can't give the predictor bit-identical walls. | not standard | The map is content: one Cardinal-free arena package, built the same on both sides. | M |
| [F9](#f9) | Any client can claim the full 0.4 s rewind on every shot. | not standard | Size the rewind from a budget: interpolation + queue + the highest ping you favor. | S |
| [F10](#f10) | Every tick resends all state as named msgpack: 1.4-2.9 Mbit/s per player at 60 Hz. | won't scale | Compress each player's event stream in Cardinal. | M. World-engine. |

## How this review was made

- Six reviewers, one per area: ECS; input and netcode; prediction with lag compensation (areas 3+4); content and rules; physics with the client engine (areas 6+7); tests and tooling.
- Lean run. A skeptic tried to refute only the high findings. Both were downgraded to medium, none refuted.
- Medium and low findings were checked by their own reviewer only. The lead re-read the key lines of each top-10 claim in the real tree.
- Every probe ran in a scratch copy. The real tree was fingerprinted before and after. It is unchanged, apart from this file.
- Claims are marked **verified** (read at file:line, or measured) or **summarized**. Engine comparisons are marked **sourced** (with a link) or **from memory**.
- Cost of the run: 8 agents, 2.2M tokens, 946 tool calls, 69 minutes.

## Contents

- [Findings](#findings): F1-F27 medium, F28-F39 low, most severe first.
- [Recent calls re-examined](#recent-calls-re-examined)
- [Docs vs code](#docs-vs-code)
- [Solid as is](#solid-as-is)
- Appendices: [A. Mechanisms](#appendix-a-mechanisms-enumerated), [B. Earlier review status](#appendix-b-earlier-review-status), [C. Probes](#appendix-c-probes-run), [D. Skeptic coverage](#appendix-d-skeptic-coverage-and-refuted-findings)

Each finding lists its source ID from the review run (for example `netcode-3`). Paths are relative to the repo root unless they start with `world-engine@`, which means the v0.22.0-beta module.

## Findings

Most severe first. F1-F27 are medium, F28-F39 low. No high finding survived the skeptic.

<a id="f1"></a>

### F1. A player whose Echo died keeps playing, because nothing owns the match

**bug**, medium. From `content-1, ecs-2`.

- **Where:** `shards/game/internal/combat/death_system.go:16-23`, `shards/game/internal/ability/cast_system.go:15-21`, `shards/game/internal/echo/spawn_system.go:51-55`, `shards/game/internal/echo/spawn_system.go:72`, `docs/GDD.md:157-162`, `shards/game/internal/player/spawn_system.go:42-46`, `shards/game/internal/player/input_system.go:108-151`, `shards/game/internal/ability/equip_system.go:20-28`, `shards/game/internal/ability/cast_system.go:31-43`, `shards/game/main.go:87-89`
- **What breaks:**
  - GDD §8 is Locked: you are out for the round when your Echo dies.
  - In code, Died only destroys the Echo. The Body keeps moving, casting and body-blocking.
  - Probe: a player whose Echo had died shot the enemy Echo from 100 to 75 HP.
  - In 2v2 or 3v3, a dead player's Body still shields its allies' Echoes.
  - Root cause: no system owns the match. DevSpawn makes two Bodies at Init. A player's first command claims the next free Body, so join order picks the team.
  - Echo spawn and kit equip fire only on BodyClaimed, once per Body ever. Nothing resets shots, effects or cooldowns between rounds, and rounds can't hand out new Echoes.
  - Each mode or round feature touches 5 or more systems.
- **Measured:** Probe (fxDuel, scratch copy): alice's Echo killed by a DamageIntent, then alice fired at bob's Echo: 100 → 75 HP. Died has 3 readers (HistorySystem, echo SyncSystem, DeathSystem); none marks the player out. The lifecycle lives in 4 systems (DevSpawn, Input claim, Equip, echo.Spawn), and 0 lines handle rounds, seats or 'out'. A 3v3 probe had to hand-add 4 Bodies.
- **Mature engines:** Unreal's GameMode owns match flow (players join, RestartPlayer spawns a pawn, match state changes), and GameState replicates it [from memory]. Quantum raises a player-added signal, and one system builds the player entity from a prototype [from memory]. Overwatch and Valorant stop a dead player's inputs and switch to a spectator view; League's Arena, the GDD's reference, benches a dead player until the next round [from memory]. GAS stops an ability from starting while its owner has a listed tag (ActivationBlockedTags) [sourced: https://raw.githubusercontent.com/tranek/GASDocumentation/master/README.md]. In all of them the mode drives spawning and resets, not a player's first input.
- **Fix:**
  - One owner for the match lifecycle, built in two slices.
  - Slice 1, now: when an Echo dies, its owner Body gets an `out` effect until the round ends. The cast gate refuses `out` Bodies, and a default shot rule passes them. A dead player then neither shoots nor shields. ([F17](#f17)'s `blocked_by` data would make that gate a content row.)
  - Slice 2, with rounds: the planned Match owns seats from the lobby (persona, team), and spawnBody builds one Body per seat. A RoundSystem sends each owner the planned SpawnEchoIntent, a kit reset and a shot clear, instead of features reacting to BodyClaimed. Tests get the same seats through [F5](#f5)'s dev roster.
- **Rejected:**
  - *Destroy the Body when its Echo dies.* The client follows its own Body to spectate. Rounds would then rebuild and re-claim Bodies, and reused IDs alias stored refs ([F22](#f22)).
  - *Each system checks whether the caster's Echo is alive.* Spreads one rule over CastSystem, FlightSystem and AreaSystem, and each must look up another feature's entity.
  - *Remove the Body's InputQueue.* Changes the archetype and stops the acks the client waits for. The Body still stands in the field as a shield.
  - *More dev Bodies, keep first-come claims.* Join order still picks teams, and nothing resets between rounds.
  - *Spawn an Echo whenever a Body has none.* Revives a dead player mid-round, which the GDD forbids, and scans every Body each tick.
- **Cost:**
  - Size: slice 1 S (one small mode system, one tag, one gate check, one default rule, tests). Slice 2 M (Input claim, DevSpawn, echo.Spawn, Equip, Death, a new RoundSystem, test helpers).
  - Risk: slice 1 low: the recorded run has no out tag. Slice 2 medium: it changes how every test gets a Body.
  - Purge: slice 1 no (adds a tag bit, an effect kind and a system). Slice 2 yes: snapshots hold first-come claimed Bodies with no seat.
  - World-engine: no
- **Earlier review:** new. Design step 22 plans the mode, but not seats, resets or 'out'.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check (`content-1`): Rounds and modes are planned (design step 22), so this is partly unbuilt work. But the rule is Locked in the GDD, the spawn comment already claims 'Echo death = elimination', and the probe shows a dead player dealing damage today. Medium, not high: no match flow exists yet.
  - Reviewer's check (`ecs-2`): It is labeled scaffolding (main.go:87-88), and the design already plans Match and SpawnEchoIntent. Fixing it before matchmaking exists may guess wrong. But the plan misses seats, resets and 'out', and every feature built now on BodyClaimed adds to the rework.
  - Lead check: Re-read death_system.go:16-23: Died only destroys the Echo. cast_system.go's casterRow (input, champion, abilities, effects) never reads the Echo. Holds.
- **Evidence:**
  - Verified: DeathSystem only destroys the entity that died (shards/game/internal/combat/death_system.go:16-23)
  - Verified: CastSystem casts for any Body with input, a champion, cooldowns and effects. Nothing reads its Echo. (shards/game/internal/ability/cast_system.go:15-21)
  - Verified: Died is read only by history, echo sync and DeathSystem (shards/game/internal/projectile/history_system.go:40, shards/game/internal/echo/sync_system.go:38, shards/game/internal/combat/death_system.go:18)
  - Verified: Echoes spawn once per Body, on BodyClaimed, 'for the whole match' (shards/game/internal/echo/spawn_system.go:51-55, 72)
  - Verified: GDD §8 [Locked]: out for the round when your Echo dies; best-of-N rounds (docs/GDD.md:157-162)
  - Verified: Mode with round play is planned as migration step 22 (docs/research/system-architecture-design.md:259-263, 524)
  - Verified: DevSpawnSystem makes two fixed Bodies at Init (shards/game/internal/player/spawn_system.go:42-46)
  - Verified: First command claims the next free Body; Persona is never cleared (shards/game/internal/player/input_system.go:108-151)
  - Verified: Echo spawn and Equip run only on BodyClaimed (shards/game/internal/echo/spawn_system.go:72; shards/game/internal/ability/equip_system.go:22)
  - Verified: CastSystem gates on InputQueue and cooldowns, not on a living Echo (shards/game/internal/ability/cast_system.go:31-43)
  - Verified: GDD: 1v1 and 2v2 confirmed, 3v3 possible; out for the round when the Echo dies [Locked]; best-of-N (docs/GDD.md:152-163)
  - Verified: Design plans Match, RoundSystem and SpawnEchoIntent, but not seats, resets or 'out' (docs/research/system-architecture-design.md:241,259-263)
  - Verified: main.go labels the dev spawn as scaffolding (shards/game/main.go:87-89)
  - Verified: Tests hand-build ally and viewer Bodies for team cases (shards/game/main_test.go:630-638,990-1008)

<a id="f2"></a>

### F2. Shots meet a moving target at one pose per tick, not as the screen draws it

**bug**, medium (reviewer said high; the skeptic downgraded it). From `prediction-1`.

- **Where:** `shards/game/main.go:76-82`, `shards/game/main.go:105`, `shards/game/internal/projectile/flight/flight.go:170`, `shards/game/internal/projectile/flight/view.go:100-140`, `client/src/engine/render/entity_views.ts:27-28`, `shards/game/main_test.go:555-557`, `shards/game/main_test.go:1541-1544`
- **What breaks:**
  - Targets move in PreUpdate. Shots fly in Update. So each tick, a shot sweeps its segment against one target pose.
  - The screen draws both sliding. A target that crosses the bolt inside a tick is drawn overlapping it but judged a miss, and the reverse.
  - Results change with tick rate.
  - Every gameplay shot today is cast from a frame, so FlyAtView judges it at the pose the view drew (view.go:121-140). The live rule (flight.go:170, the end pose) covers only shots no frame cast, and test content.
  - Until 0 means the target at the view already overlaps the shot's start. The shooter's screen draws that overlap, then the bolt vanishes: an Until-0 tick draws nothing (entity_views.ts:27-28).
- **Measured:**
  - Model of today's rule against exact relative motion (bolt 15 u/s, radius 0.15; target radius 0.5 crossing 6 u out; 4,000 offsets): outcomes differ on 18.2% (20 Hz) and 6.1% (60 Hz) of exact hits for a walking target (5 u/s), and 46.2% and 15.4% for a dash (20 u/s).
  - Against what the screen draws, the gap is larger. The skeptic drove the real LocalPlayer, Timelines and Prediction: view v pairs with the shot's tick start. Today's mismatch is then 30.3% walking and 76.7% dashing at 20 Hz, 60 fps.
  - Head-on, Until 0 happens 25% of the time for a walker and 50-62% for a dasher, at any rate.
  - Real code, the own-Echo test: Until 0.807 at 20 Hz but Until 0 at 60 Hz, with the bolt 0.067 u inside the 0.65 radius sum.
- **Mature engines:** Box2D's own bullets solve time of impact against the other body's sweep, so moving bodies meet where their paths cross [sourced: world-engine@v0.22.0-beta pkg/box2d/solver_continuous.go:143-180, checked by the skeptic]. Source and Unreal step projectiles against other actors at their current pose [from memory; the skeptic found no source]. But at 64-128 Hz a player moves about 10% of its width per tick. Here a dash moves a full body width per 20 Hz tick, so freezing costs far more.
- **Fix:**
  - Sweep each shot against each target's motion over the tick the shooter's screen shows. Cast in the target's frame: the shot's move minus the target's move. The pair cast already works in that frame.
  - For predicted shots, sweep AtView(v) to AtView(v+1). This is the earlier review's D5 pairing. The reviewer proposed v−1 to v; the skeptic showed that is a tick off, and worse than today.
  - Define what happens when row v+1 doesn't exist yet. In a stall the render clock waits at the newest tick, so the predictor has no row v+1. This view-contract change is why the fix is bigger than M.
  - Land [F21](#f21)'s jump flag first, so a blink or a landing is never swept as a slide.
  - Then put touching (Until > 0) back in the own-Echo test.
- **Rejected:**
  - *Sweep v−1 to v (the reviewer's first fix).* A tick off: the screen pairs view v with the tick's start. Skeptic's measure at 20 Hz, 60 fps: walkers 48.5% (today 30.3%, D5's pairing 12.0%); dashers 122.8% (today 76.7%, D5's 30.5%).
  - *Fly shots before targets move.* Flips the bias: shots then meet start-of-tick poses, so targets leaving the path get hit. Still rate-dependent, still drawn ≠ judged.
  - *Sub-step shots (several casts a tick).* Cuts the error by the sub-step count but never removes it, and multiplies cast cost on the server and in wasm.
  - *Rely on 60 Hz.* The error drops about 3x but stays: dashes still differ on 15% of crossing hits. 20 Hz stays the default.
  - *Keep the own-Echo test on the offset only.* Hides the symptom.
- **Cost:**
  - Size: M+. The skeptic says more than M, because of the view contract.
  - Risk: medium: it changes which shots hit. Recordings at both rates must be re-made and their diffs reviewed.
  - Purge: no, per the reviewer: no component changes shape.
  - World-engine: no: ShapeCastCircle and DynamicTree in v0.22.0-beta already do it.
- **Earlier review:** carried over from D5 (rated low then, still open) and D3 (the Until-0 draw gap, still open). X6's own-Echo test change worked around the Until-0 symptom.
- **Skeptic:**
  - Downgraded from high to medium. Real: each tick meets one target pose, and the skeptic's probes reproduce the numbers (18.3/6.0/46.2/15.4%; Until 0.807 vs 0).
  - Not high: there is no desync. Server and predictor agree to the bit, and endOn draws every hit on its target. It is the earlier D5 (rated low) plus D3's open Until-0 draw gap.
  - The reviewer's fix was a tick off. The fix above uses the skeptic's corrected pairing.
  - A relative sweep would sweep a blink or a landing (mover.Land) as if the target slid, so the jump flag must land first.
  - The Source/Unreal claim has no source. A search only confirmed that Unreal sweeps each frame against the scene.
- **Evidence:**
  - Verified: MovementSystem and Echo ReplaySystem run in PreUpdate; FlightSystem runs in Update after them (shards/game/main.go:76-82,105)
  - Verified: Live shots cast against the Box2D world after targets moved (shards/game/internal/projectile/flight/flight.go:170)
  - Verified: Predicted shots pair-cast against one target pose, the one at the view (shards/game/internal/projectile/flight/view.go:121-140)
  - Verified: Every frame cast is Predicted (skeptic) (shards/game/internal/projectile/launch_system.go:57)
  - Verified: blend returns nothing when t > until, so an Until-0 tick draws nothing (skeptic's reading) (client/src/engine/render/entity_views.ts:27-28)
  - Verified: Touching needs Until > 0; the own-Echo test now checks only End.At − Offset against the Echo, within 1e-9 (shards/game/main_test.go:555-557,1541-1544)
  - Verified: Box2D bullets use relative-motion time of impact (world-engine@v0.22.0-beta/pkg/box2d/solver_continuous.go:143-180)
  - Verified: CLAUDE.md says first hit wins makes body-blocking exact and a swept path can't tunnel (CLAUDE.md:304)
  - Verified: Probe numbers above: crossing.py, lagged.py and headon.py models, the real-client alignment probe, and a t.Logf in the own-Echo test at both rates (scratch copies only)

<a id="f3"></a>

### F3. Content loads through a plugin world-engine has deleted, and a typo key loads as zero

**not standard**, medium. From `ecs-5, content-2`.

- **Where:** `shards/game/main.go:42-49`, `shards/game/internal/predict/predict.go:36-49`, `CLAUDE.md:468-499`, `shards/game/internal/predict/predict.go:38-60`, `shards/game/main_test.go:1351-1396`, `shards/game/internal/echo/spawn_system.go:86-90`, `world-engine@v0.22.0-beta/pkg/plugin/data/system/state.go:72`, `world-engine@v0.22.0-beta/pkg/plugin/data/system/state.go:236-242`, `world-engine@v0.22.0-beta/pkg/plugin/data/system/source.go:45-47`
- **What breaks:**
  - Tuning keys decode loosely. A typo loads as 0: a 0 s cooldown, 0 anti-camp damage, a slow with no stat.
  - A champion missing from echo tuning gets no Echo, so it can never be eliminated.
  - Cross-file IDs are checked only by a test (TestContentIDsLineUp), and nothing runs the tests automatically ([F4](#f4)).
  - world-engine main deleted pkg/plugin/data (#1133). 13 systems hold `Data *data.Plugin` (15 files, 27 data.Get calls). Any upgrade past v0.22.0-beta breaks the build, and generational IDs ([F22](#f22)) and WebSocket input ([F24](#f24)) both need one.
  - The predictor already loads the same JSON a second way (predict.Load). That is two loaders.
  - The plugin's hash reconcile does nothing here. The embed is the only source, so a changed hash just logs a warning.
- **Measured:** Probe: `cooldown_secnds: 2` loaded as cooldown 0. `damage_per_secnd` loaded as 0. `stats` loaded as no stat. A champion with `aera` booted, and its Echo stayed at 100/100 HP after 3 s on its own Body. 13 of 20 systems take Data. Decoder parity: goccy (shard) and encoding/json (predictor) agreed on all 6 shipped files and 10 edge cases.
- **Mature engines:** Quantum loads data assets into one asset database that the sim reads the same way on every client and server [from memory]. Unity bakes authoring data into blob assets shared by client and server worlds [from memory]. Unreal validates assets on save through its Data Validation subsystem [sourced: https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/DataValidation/UEditorValidatorSubsystem]. GAS tags must be declared before use, so a misspelled tag can't load [sourced: https://raw.githubusercontent.com/tranek/GASDocumentation/master/README.md]. In-house: rampage replaced this plugin with gamedata.Load, which decodes, validates, then checks cross-file pairs at boot [verified: rampage-backend/shards/gameplay/internal/gamedata/gamedata.go:55-103].
- **Fix:**
  - One Cardinal-free loader, shared by the server and the predictor.
  - It decodes each file strictly (DisallowUnknownFields), runs Bind and Validate, then runs the cross-file checks TestContentIDsLineUp holds today. Any error fails the boot.
  - main.go calls it once and gives each system a typed tuning field. predict.Load becomes the same function.
  - Remove the data plugin and its ConfigManifest in the same change, riding the purge already pending for fixes 1, 5, 8 and 12. Rewrite CLAUDE.md's data-plugin section.
- **Rejected:**
  - *Keep the plugin and add a strict UnmarshalJSON to each tuning type.* The plugin is gone upstream, so the next engine bump breaks the build anyway. And it means six hand-written decoders.
  - *Stay pinned to v0.22.0-beta.* Blocks every later engine fix, including generational IDs and WebSocket input.
  - *Copy the plugin into echo_image.* Keeps two loaders and a reconcile that does nothing here. Keys still decode loosely.
  - *Keep both loaders and test that they agree.* Two paths to keep equal by hand. A test proves it only for the content it loads.
  - *JSON Schema files beside the Go types.* A second source of truth that drifts from the structs. The Go types already name every key.
- **Cost:**
  - Size: M: mechanical wiring in 15 files, one loader, and tests move from data.Source to fs.FS (the overlay already has Open).
  - Risk: low: same bytes and the same Validate. The parity probe covers the decoder switch.
  - Purge: yes: the data_config_manifest component leaves the snapshots, and restore fails on an unregistered component.
  - World-engine: no: main already removed the plugin. echo_image just stops importing it.
- **Earlier review:** carried over from S9 (the two loaders still match). New: the plugin is deleted upstream. system-architecture-design.md:592 already lists strict keys as not done.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check (`ecs-5`): Nothing breaks while the pin holds, and the edit is mechanical. An upgrade may not be planned soon. But F22's fix needs one, and CLAUDE.md still tells authors to use a plugin that no longer exists.
  - Reviewer's check (`content-2`): Checked: the design doc defers strict keys, and the reconcile would matter with a versioned source, but PickSource only returns the embed. Both decoders agree, so there is no desync today. Still real: the typo probe booted with zeros, and main has no data plugin.
  - Lead check: plugin state.go:72 and predict.go:51-60 both decode with plain json.Unmarshal. Only hooks.go:94 calls DisallowUnknownFields. Holds.
- **Evidence:**
  - Verified: 13 systems take Data *data.Plugin (shards/game/main.go:73-129)
  - Verified: predict.Load decodes, binds and validates 3 tunings (shards/game/internal/predict/predict.go:36-49)
  - Verified: World Engine main removed the plugin (world-engine commit 134dcc5d 'refactor: remove data plugin (#1133)'; pkg/plugin now holds lobby, physics2d)
  - Verified: Restore errors on a component this build does not register (world-engine@v0.22.0-beta/pkg/cardinal/internal/ecs/world_state.go:460-462)
  - Verified: Plugin reconcile only warns and resumes on changed content (world-engine@v0.22.0-beta/pkg/plugin/data/system/state.go:236-242)
  - Verified: CLAUDE.md mandates the plugin and bans hand-rolled loaders (CLAUDE.md:468-499)
  - Verified: The plugin decodes with json.Unmarshal and never checks for unknown keys (world-engine@v0.22.0-beta/pkg/plugin/data/system/state.go:72)
  - Verified: PickSource always returns the embed, and a changed snapshot hash only logs a warning (world-engine@v0.22.0-beta/pkg/plugin/data/system/source.go:45-47; state.go:236-242)
  - Verified: world-engine main deleted pkg/plugin/data (1787 lines) (/home/doglightning/world-engine commit 134dcc5d (#1133))
  - Verified: Cross-file IDs are checked only in a test (shards/game/main_test.go:1351-1396)
  - Verified: A champion missing from echo tuning logs an error and gets no Echo (shards/game/internal/echo/spawn_system.go:86-90)
  - Verified: Hook params already decode strictly (shards/game/internal/ability/hooks/hooks.go:94)
  - Verified: The design doc lists strict tuning keys as not done (docs/research/system-architecture-design.md:592)
  - Verified: Rampage's gamedata.Load decodes, validates and checks cross-file pairs at boot (rampage-backend/shards/gameplay/internal/gamedata/gamedata.go:55-103)

<a id="f4"></a>

### F4. Nothing runs the checks: hook is off, no CI, no arm64 run

**duct tape**, medium. From `tests-3`.

- **Where:** `scripts/check.sh:1-19`, `.githooks/pre-push:3`, `CLAUDE.md:27-28`, `client/vite.config.ts:7`, `client/tsconfig.json:19`, `shards/game/main_test.go:1351`
- **What breaks:** core.hooksPath is unset, so no push runs check.sh. When on, the hook tests the working tree, not the pushed commits (89 uncommitted files today), and --no-verify skips it. Cross-file content IDs (50+ abilities a year, often JSON only) are checked only by TestContentIDsLineUp, which nothing runs. Determinism is claimed for arm64, but no test runs there.
- **Measured:** check.sh in a scratch copy: 31.1 s cold, 7.4 s warm (Go results cached). The Go half needs nothing private: proxy.golang.org serves world-engine v0.22.0-beta. npm has only @argus-labs/sdk 0.1.0 (2025-06-03), without the MessagePack codec; the codec is on go-ecs branch Anthony/sdk-ts-msgpack with no remote-tracking ref. GOARCH=arm64 go test ./... under this machine's qemu binfmt: all pass, 2m03s at 20 Hz and 2m29s at 60 Hz, amd64 recordings included.
- **Mature engines:** GitHub offers ubuntu-24.04-arm runners to private repos, at 2 vCPU [sourced: https://docs.github.com/en/actions/reference/runners/github-hosted-runners]. A second private checkout needs a PAT or deploy key [sourced: https://github.com/actions/checkout/]. Unreal's Gauntlet runs test sessions across platforms and validates results [sourced: https://dev.epicgames.com/documentation/unreal-engine/gauntlet-automation-framework-overview-in-unreal-engine]. Studios gate merges on CI, not local hooks [from memory].
- **Fix:** Release the SDK's MessagePack codec to npm and pin it in client/package.json, as go.mod pins world-engine; drop the ../../go-ecs alias. Then one workflow runs scripts/check.sh on every push and PR, plus an ubuntu-24.04-arm job running go test at both rates. The Go jobs need no secret, so they can start before the SDK ships.
- **Rejected:**
  - *Opt-in pre-push hook only.* Off today, skipped by --no-verify, tests the working tree not the pushed commits, one machine, no arm64.
  - *CI checkout of go-ecs with a token.* Needs a PAT into an org repo, a pushed branch, and a monorepo bun install (catalog: and workspace: deps) on every run.
  - *git submodule of go-ecs.* Same token and push needs, plus cloning the whole monorepo.
  - *Vendor a pinned SDK copy.* Forks 332 KB of SDK source and about ten dependencies; drifts from releases.
- **Cost:**
  - Size: medium: a ~40-line workflow, plus an SDK release in Argus-Labs/monorepo
  - Risk: low
  - Purge: no
  - World-engine: no; the SDK lives in Argus-Labs/monorepo (go-ecs) and needs a release there
- **Earlier review:** carried over from X3 (repo part done; CI still open)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: One author, and check.sh passes. But the hook is off, CLAUDE.md overstates the blocker (Go needs nothing private), and content-ID typos are caught only by tests. The earlier skeptic rated X3 medium; still medium.
  - Lead check: core.hooksPath is unset, and there is no .github/ directory. The client's go-ecs checkout is on branch ryan/revert at 0ec6eb05c, the codec commit, and no remote branch contains it. Holds.
- **Evidence:**
  - Verified: check.sh runs gofmt, vet, wasm vet and go test at both rates, then tsc and vitest twice (scripts/check.sh:7-19)
  - Verified: The hook execs check.sh, which checks the working tree (.githooks/pre-push:3)
  - Verified: core.hooksPath is not set (git config --get core.hooksPath (exit 1))
  - Verified: Only the client aliases go-ecs (client/vite.config.ts:7)
  - Verified: Cross-file content IDs fail only in a test (CLAUDE.md:476-479)
  - Verified: The SDK codec commit 0ec6eb05c has no remote-tracking ref in the go-ecs clone (go-ecs: git branch -r --contains 0ec6eb05c (empty))
  - Verified: npm lists only @argus-labs/sdk 0.1.0 (https://registry.npmjs.org/@argus-labs/sdk)

<a id="f5"></a>

### F5. Tests hand-build extra Bodies, so the ally pass-through test guards nothing

**bug**, medium. From `ecs-3, tests-5`.

- **Where:** `shards/game/main_test.go:986-1025`, `shards/game/internal/projectile/flight_system.go:121-124`, `shards/game/content/projectile/tuning.json:4`, `shards/game/internal/player/spawn_system.go:42-45`, `shards/game/main_test.go:1000-1009`, `shards/game/main_test.go:636`, `shards/game/main_test.go:381`
- **What breaks:**
  - TestShotPassesThroughAllies builds carol by hand, with no ref.Spawn. Since fix 8, seenAt skips any target without Spawn. So the shot never meets carol, whatever the rules say.
  - Probe: remove `ally` from the pass rule, and every Go test stays green. Add a Spawn to carol, and the test fails as it should.
  - Ally pass-through is needed for 2v2, a confirmed mode, and has no working guard.
  - Root cause: the only spawn path makes exactly two Bodies. Three tests hand-build a third player with fewer parts than spawnBody gives. One carol is Kinematic with mask Body|Wall|Shot. A real Body is Manual with Wall|Area|Shot, plus Spawn, InputQueue and Champion.
  - No benchmark exists, so a per-tick cost rise at 3v3 or at 60 Hz shows up only in play.
- **Measured:** Pass rule set to own only: 0 FAIL in go test ./shards/game/... at 20 Hz, and the game package passes at 60 Hz. Same edit plus ref.Spawn on carol: 'shots = [], want one past carol'. Probe 1v1 benchmark through TestWorld: 369 µs, 186 KB and 1,096 allocs a tick at 20 Hz; 488 µs, 374 KB and 1,659 allocs at 60 Hz.
- **Mature engines:** Quantum spawns entities from prototypes and Unity from prefabs or baked archetypes, so a test entity carries the same components as a live one [from memory]. In archetype ECSs a missing component drops an entity from a query without error, so tests build entities through the game's own spawn path [from memory]. Unreal's Gauntlet launches multi-client sessions such as 5v5 plus a server [sourced: https://dev.epicgames.com/documentation/unreal-engine/gauntlet-automation-framework-overview-in-unreal-engine]. Allocation counts are deterministic, so they make a stable cost guard where wall times don't [from memory].
- **Fix:**
  - Every test Body comes from the one constructor.
  - Make the dev roster content: player/tuning.json lists dev spawns (team, position), and DevSpawnSystem calls spawnBody for each. withContent then builds a real 2v2 or 3v3. Delete the hand-built carols.
  - Add a 3v3 BenchmarkTick at both rates, and a test that fails when allocations per tick pass a ceiling.
  - Later, the Match's seats ([F1](#f1)) feed the same spawnBody.
- **Rejected:**
  - *Add ref.Spawn to carol's row.* Fixes one row. The next Body component (a seat, an out tag) silently drops it again.
  - *Let seenAt accept targets without Spawn.* A fallback that hides wrongly built rows. The predictor keys targets by serial, so server and client would disagree.
  - *Wait for matchmaking.* No 3v3 test or cost number until then, though 3v3 is the target.
  - *Wall-time thresholds in tests.* Noisy on shared machines and CI runners. Allocation counts are not.
- **Cost:**
  - Size: S: a content list, a loop in DevSpawnSystem, three test rows rebuilt, a benchmark and a guard test.
  - Risk: low
  - Purge: no: Init-time dev spawn and content only.
  - World-engine: no
- **Earlier review:** new: a side effect of fix 8, which made seenAt require ref.Spawn.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check (`ecs-3`): The rule itself works, and live play has one Body per team, so no ally exists yet. The tests reviewer may flag this too. But the guard does not work today, and 2v2 is a confirmed mode.
  - Reviewer's check (`tests-5`): A 1v1 costs about 0.5 ms of a 16.7 ms tick at 60 Hz, so no budget breaks today. The carol tests pass because today's rules read only Team and shape bits. Medium, because 3v3 is the target and nothing measures it.
  - Lead check: carol's row (main_test.go:990-1001) has no ref.Spawn, and seenAt (flight_system.go:121-124) returns false without one. Holds.
- **Evidence:**
  - Verified: carol's row has no ref.Spawn (shards/game/main_test.go:990-1008)
  - Verified: seenAt returns false for a target without TargetHistory or Spawn (shards/game/internal/projectile/flight_system.go:121-124)
  - Verified: Content's pass rule covers own and ally (shards/game/content/projectile/tuning.json:4)
  - Verified: Rule edit passes all tests; with Spawn on carol it fails (copy only: rule and test edited, go test run, both files restored (diff -q identical))
  - Verified: DevSpawnSystem spawns exactly two Bodies (shards/game/internal/player/spawn_system.go:42-45)
  - Verified: A test hand-builds a Kinematic carol with mask Body|Wall|Shot (shards/game/main_test.go:990-1009)
  - Verified: Real Bodies are Manual with mask Wall|Area|Shot and carry Spawn, InputQueue, Champion (shards/game/internal/player/spawn_system.go:20-33)
  - Verified: Two more tests add a carol with only Player, Team and Transform (shards/game/main_test.go:376-382)
  - Verified: No Benchmark function exists (grep -rn 'func Benchmark' (no match))

<a id="f6"></a>

### F6. No random or soak test; a quick soak finds your dead Echo mispredicted

**not standard**, medium. From `tests-1`.

- **Where:** `shards/game/main_test.go:2443`, `shards/game/internal/projectile/flight/view_test.go:173`, `shards/game/internal/player/input_system.go:68`, `shards/game/internal/player/sync_system.go:94-96`, `shards/game/internal/predict/predict.go:278-279`, `world-engine@v0.22.0-beta/pkg/cardinal/dst.go:94`
- **What breaks:** Every test feeds hand-picked inputs. Nothing samples random but honest play, so whole classes of state go unchecked. A seeded random soak (two players, 4,000 ticks) breaks the 'exact' prediction rule in 10 of 10 seeds at 20 Hz and 5 of 10 at 60 Hz. On the tick after your Echo dies, the predictor still predicts it, because body_sync sends EchoPath for a dead Echo. No scripted test kills your own Echo.
- **Measured:** Probe TestSoakProbe in a scratch copy (deleted after): random moves, aims, buttons 0-3, 1 in 40 frames NaN or zero; requirePredicted and requireEnded every tick. Fails at her Echo's death tick: 10/10 seeds at 20 Hz, 5/10 at 60 Hz (4,000 ticks). Skipping that one tick, all 20 runs pass. Fuzz probes: FlyAtView vs Fly 8.6M execs in 90 s, enqueue invariants 2.1M execs in 45 s, no failure. cardinal.RunDST(register): 8 seeds x 3,000 ticks pass in 0.5-1.3 s, but only 2-4 of 6,000 Body-ticks get a new frame.
- **Mature engines:** Go fuzzing is in the toolchain: f.Add seeds and testdata/fuzz run in plain go test, and failures are saved as regressions [sourced: https://go.dev/doc/security/fuzz/]. Riot built determinism for 'fast, repeatable tests driven from a set of recordable inputs' [sourced: https://www.riotgames.com/en/news/determinism-league-legends-introduction]. Unreal's Gauntlet runs automated multi-client sessions, like 5v5 plus a server [sourced: https://dev.epicgames.com/documentation/unreal-engine/gauntlet-automation-framework-overview-in-unreal-engine]. Bot soaks with invariant checks are routine [from memory].
- **Fix:** Add one seeded soak test to main_test.go: both clients send random honest frames for 2,000 ticks at each rate, and the existing requirePredicted and requireEnded check every tick. Log the seed so a failure reruns. Turn view_test.go's 60-point sweep into a fuzz target with those points as seeds. The dead-Echo bug itself goes to the prediction area: YouState skips an Echo that died this tick.
- **Rejected:**
  - *cardinal.RunDST as is.* Each command comes from a new random player, so only 2-4 of 6,000 Body-ticks get a frame. It checks ECS structure, not game rules.
  - *Property-test libraries (rapid, gopter).* An extra dependency for what go test -fuzz and a seeded loop already do.
  - *More hand-written scripted cases.* Fixed samples miss whole classes; the dead-Echo tick hid from all 69 scripted world tests.
- **Cost:**
  - Size: small: ~60 lines of test, plus a one-line YouState fix in the prediction area
  - Risk: low
  - Purge: no: tests only; the YouState fix changes an event, not a component
  - World-engine: no (optional: a fixed player pool for RunDST would make it useful here)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Checked whether CLAUDE.md accepts this correction: it lists late frames, others' effects and targets others kill. Your own dead Echo is already in the sync the predictor starts from, so it isn't accepted. Impact is cosmetic today (own shots pass own Echo), so medium.
  - Lead check: sync_system.go:84-96 sends EchoPath for every Echo entity, and DeathSystem destroys a dead Echo only after the syncs. predict.go:278-280 predicts any non-empty path. Holds.
- **Evidence:**
  - Verified: YouState sends EchoPath for any Echo with PathHistory, a dead one included (shards/game/internal/player/sync_system.go:85-96)
  - Verified: The predictor predicts an Echo whenever EchoPath is non-empty (shards/game/internal/predict/predict.go:278-279)
  - Verified: On the failing tick the sync it reset from shows her Echo at HP 0, with EchoDelay 100 and 11 path spots (probe TestSoakProbe seed 2 tick 1948 (scratch copy))
  - Verified: CLAUDE.md accepts corrections only from late frames, others' effects and targets others kill (CLAUDE.md:614-616)
  - Verified: The Fly vs FlyAtView property is checked on 8 cases plus a fixed 60-point sweep (shards/game/internal/projectile/flight/view_test.go:134-181)
  - Verified: RunDST sends each command from a random 8-letter player (world-engine@v0.22.0-beta/pkg/cardinal/dst.go:94)
  - Verified: The repo has no Benchmark or Fuzz function (grep -rn 'func Benchmark|func Fuzz' (no match))

<a id="f7"></a>

### F7. Input pacing aims at a fixed 0.05 s queue and ignores coasts

**not standard**, medium. From `netcode-3`.

- **Where:** `client/src/game/player/input_stream.ts:8`, `client/src/game/player/input_stream.ts:54`, `client/src/game/player/input_stream.ts:95`, `shards/game/internal/player/input_system.go:96`, `shards/game/internal/player/sync_system.go:122`
- **What breaks:** The target is a TS constant: one tick at 20 Hz, three at 60 Hz. The loop steers the average queue, so a TCP stall from one lost packet still empties it, and each empty tick is a coast and a correction. Speed-up tops out at 5% (gain 1/s × 0.05 s), not the 10% clamp, so a client clock more than 5% slow coasts steadily. On clean links the 50 ms is pure added delay.
- **Measured:** Real InputStream (bun) against a simulated server queue on one in-order link, 40 ms each way, 5 min per run. No loss: 0 coasts. 0.2% loss (100 ms recovery): 2.2 coasts/min at 20 Hz, 15.8 at 60 Hz. 1% loss: 9.4 and 56.6/min. The queue stays at 52–66 ms and the rate near 1.0: the target never rises. Server clock 6% fast: the rate caps at 1.049, with 18 and 54 coasts/min. Out-of-order jitter up to 60 ms: at most 0.6/min, because the resend covers it.
- **Mature engines:** In Overwatch the server spots input starvation and tells the client to dilate time and build a buffer, then drains it once things are stable. The client leads by half the RTT plus one 16 ms command frame [sourced: https://edgegap.com/blog/game-backend-deep-dive-overwatch-2016-netcode-architecture-rollback]. Rocket League grows or shrinks its input buffer with measured jitter: 25 ms of jitter wants about 25 ms of buffer [sourced: https://devtrackers.gg/u/Psyonix_Cone].
- **Fix:** Let the server set the target from starvation. InputSystem counts each Body's coasts while its client is sending. The target rises a step after each coast and decays slowly when none come, staying between one tick and input_buffer_seconds. YouState sends it. The client steers its queue to that target with a gain that can reach the 10% clamp. The step, decay and bounds live in content/player/tuning.json.
- **Rejected:**
  - *Raise the fixed target to 0.1 s.* Adds 50 ms for every player on every link, and a longer stall still empties the queue.
  - *Estimate jitter on the client.* The client can't see when frames reach the server, and the policy would be duplicated in TS.
  - *Only raise MaxDilation.* Fixes clock skew, not the setpoint. Packet loss still causes coasts.
- **Cost:**
  - Size: S
  - Risk: low: the client loop plus one YouState field, tested in input_stream.test.ts and main_test.go
  - Purge: no: a new InputQueue counter decodes as 0
  - World-engine: no
- **Earlier review:** carried over from S3 (still open): fix 2's setpoint and gain are TS constants
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Clean links coast 0 times a minute in the simulation, and in-order jitter turns into delay, not starvation. The 100 ms loss recovery is my model, not a capture. Each coast costs at most a tick of walking. So medium.
  - Lead check: input_stream.ts:54-57: rate = 1 + clamp(1 × (0.05 − queued), ±0.1). queued is never below 0, so speed-up tops out at 5%. CLAUDE.md:626-630 says 'by at most 10%'. Holds.
- **Evidence:**
  - Verified: The target, gain and clamp are TS constants (client/src/game/player/input_stream.ts:8-11)
  - Verified: rate = 1 + clamp(1 × (0.05 − average queued)), so speed-up is at most 5%; the tests pin 1.05 and 0.9 (client/src/game/player/input_stream.ts:54-57; client/src/game/player/input_stream.test.ts:124-139)
  - Verified: The server reports only the queue length; a coast is silent (shards/game/internal/player/sync_system.go:122; shards/game/internal/player/input_system.go:96-98)
  - Verified: A coast repeats the last frame's move (shards/game/internal/player/movement_system.go:56-63)
  - Summarized: Each coast becomes a correction of up to a tick of walking (CLAUDE.md:609-616)

<a id="f8"></a>

### F8. Planned map walls can't be built bit-identically in the predictor

**not standard**, medium. From `physics-client-1`.

- **Where:** `docs/research/map-system-findings.md:188-189`, `docs/research/map-system-findings.md:214-216`, `shards/game/internal/predict/predict.go:122-123`, `world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/create.go:9,192,268-270`
- **What breaks:** Nothing today: no map ships. The plan stores the map as one ECS entity that physics2d builds. That builder is internal, imports Cardinal, and rotates polygons, chains and capsules with math.Cos/Sin and unguarded a*b−c*d. The predictor can neither call it nor copy it safely. If the arena ships first, every wall stop, slide and shot stop on screen becomes a correction a round trip later.
- **Measured:** No map ships, so nothing breaks yet. Cost of the ECS plan (snapshots off, 6 Bodies): physics2d tick 10.5 µs bare, 14 µs with 50 shapes in one entity, 21 µs with 50 wall entities; per-tick snapshots add 140-175 µs for 50 shapes. A physics2d component leaf pulls in about 737 packages (main_test.go:2862). Mover over 50 walls: 0.35-0.40 µs a move.
- **Mature engines:** Photon Quantum bakes static colliders into the map asset; they are immutable and not entities [sourced: https://doc.photonengine.com/quantum/v2/manual/physics/statics]. Source loads world collision from the map file on both sides and never networks static props [from memory]. Box2D v3 got cross-platform determinism by avoiding platform-specific float math [sourced: https://box2d.org/posts/2024/08/determinism/]; its rotations use its own cos/sin approximation [from memory].
- **Fix:** Make the map content, not state. Add one Cardinal-free arena package that loads the baked map file and builds a static box2d.World with box2d's public builders (MakeOffsetBox, MakeRot, ComputeHull, CreateChain), already FMA-scanned. Server and predictor build it from the same file. The mover, Land and the shot cast query it. Add it to sharedCore, add wall cases to the prediction recording, and ship server and predictor walls in one change.
- **Rejected:**
  - *Export physics2d's shape builder from world-engine (S5's fix).* Needs a world-engine change and release first. The map stays snapshot state with a per-tick rebuild check, and a rebuilt map reaches Box2D only after the mover ran.
  - *Copy physics2d's builder into echo_image, plus a bit-compare test.* Two builders of one map drift, as rampage's client and server colliders did; the test only covers the shapes it lists.
  - *Send wall shapes to the client in a sync event.* Still needs a client builder with the same bits, and costs join bytes; rampage's map sync was 78 KB.
  - *Add arena bodies straight into physics2d's Engine() world.* Relies on physics2d leaving unknown bodies alone through rebuilds, which no contract promises; one engine change silently deletes the map.
- **Cost:**
  - Size: M: one arena package and loader (~200 lines), flight's cast run over two worlds (map, targets) with its tie rules kept, wall cases in the recording. Replaces the planned map entity and rebuild check.
  - Risk: Medium: shot ties across two casts must match the one-cast rule (earlier rule, then map, then lower target ID); existing flight and recording tests cover it.
  - Purge: No: nothing stored changes, and the map never enters snapshots.
  - World-engine: No: box2d's public API only. S5's alternative needs a world-engine export.
- **Earlier review:** carried over from S5 (still open); revises G10, which judged the ECS-entity plan right
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: If the team takes S5's world-engine export, the ECS plan also gets bit-identical walls, with one world and no flight change. The per-tick cost I measured is small: about 3.5 µs at 50 shapes.
  - Lead check: predict.go:122-123 says walls come with the arena. physics2d's create.go imports cardinal (line 9) and rotates with math.Cos/Sin (line 268). Holds.
- **Evidence:**
  - Verified: Predictor world has no map; 'walls come with the arena'. (shards/game/internal/predict/predict.go:122-123)
  - Verified: Plan: all permanent geometry in one static ECS entity; predictor walls later through a world-engine export. (docs/research/map-system-findings.md:188-189, 214-216)
  - Verified: Same doc's research: the static map is baked data, never snapshot state. (docs/research/map-system-findings.md:134-137)
  - Verified: physics2d rotates polygon points with math.Cos/Sin and p.X*c - p.Y*s; boxes use MakeRot; the file imports cardinal and is internal. (world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/create.go:9,192,268-270)
  - Verified: The FMA scan covers box2d but no physics2d code. (shards/game/main_test.go:2853-2855)
  - Verified: The shot cast stops at Wall and Bounds in the same physics2d world as targets. (shards/game/internal/projectile/flight/flight.go:142-171; shards/game/internal/projectile/flight_system.go:50)
  - Verified: The mover queries physics2d's world in PreUpdate before the plugin reconciles ECS into Box2D. (shards/game/internal/player/movement_system.go:37; shards/game/main.go:63-77; world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/system/pipeline.go:74-90)
  - Summarized: physics2d tick and snapshot costs at 50 shapes. (audit-copy plugin benchmark against a world-engine copy with snapshots off (not re-run after the context reset))

<a id="f9"></a>

### F9. Any client can claim the full 0.4 s rewind on every shot

**not standard**, medium. From `prediction-3`.

- **Where:** `shards/game/content/projectile/tuning.json:8`, `shards/game/internal/projectile/flight/tuning.go:61-66`, `shards/game/internal/projectile/flight/view.go:88-94`, `shards/game/internal/player/input_system.go:100-103`, `shards/game/internal/player/component/input.go:57-59`, `client/src/engine/sync/timeline.ts:9`
- **What breaks:** The server trusts the client's ViewTick, clamped only to [tick−R, tick−1]. An honest view lags RTT + 0.05 s queue + 0.1 s interpolation. A cheat can always send the oldest allowed, 0.4 s. At 20 ms ping that is 0.23 s extra: a walking target is hit 1.15 u after it left, more than a body width. The 0.1 s lives only in TS, so the server cannot tell honest from stale. ViewFloor only stops views going back.
- **Measured:** R = round(0.4 × rate) = 8 ticks at 20 Hz, 24 at 60 Hz; KeptRows 9 / 25. Honest view lag = RTT + 0.05 + 0.1 s: 0.17 s at 20 ms, 0.25 s at 100 ms, 0.35 s at 200 ms. So 0.4 s fully favors pings up to 250 ms and gives a low-ping cheat ~0.23 s extra (1.15 u walking, 4.6 u dashing). Per shot the cost is small and flat in R: FlyAtView 0.28-0.54 µs (0-6 candidates), live Fly ~0.8 µs (go test -run XXX -bench 'FlyLive|FlyAtView' -benchtime=200000x -count=3 ./shards/game/internal/predict/). Per tick: Reach build 2.1-3.0 µs at T=12 (W 9/25), 5.3-7.6 µs at T=24; TargetHistory copies (C7, HistoryCopies bench in ./shards/game/internal/projectile/) 20 µs / 57 KB at W=9, 41 µs / 121 KB at W=25.
- **Mature engines:** Source rewinds at most sv_maxunlag (1 s) and, if the client's claimed time is more than 0.2 s off its measured latency, uses the measured one [sourced: https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/player_lagcompensation.cpp]. Valorant cut its rewind limit from 200 to 140 ms [sourced: https://www.pcgamesn.com/valorant/patch-notes-6-02]. Overwatch stops favoring the shooter past about 220 ms RTT [sourced: https://edgegap.com/blog/game-backend-deep-dive-overwatch-2016-netcode-architecture-rollback] (secondary).
- **Fix:** Size the rewind from a budget, not a round number: interpolation delay (moved from TS into content, so client and server read one value) + input queue + the highest ping you choose to favor. 0.1 + 0.05 + 0.1 s gives 0.25 s. Players above it shoot at a clamped view, as today, with no cliff. Tuning validation checks rewind ≥ interpolation + queue + one tick.
- **Rejected:**
  - *Check the claimed view against measured latency, like Source.* Cardinal has no transport RTT; any lag the server infers from client messages can be inflated by the same cheat.
  - *Average the view lag and clamp outliers.* A steady cheat has no outliers; it just sits at the cap.
  - *Reject shots whose view is older than the budget.* Brings back the cliff T3/G4 removed: high-ping players lose shots instead of shooting at a clamped view.
- **Cost:**
  - Size: S
  - Risk: low-medium: pings above the budget lose some favor; tune by playtest
  - Purge: no: content only; TargetHistory trims to the new window on the next tick
  - World-engine: no
- **Earlier review:** new (Z2 fixed by fix 10 and T3's cliff by fix 1; this is the cap that is left)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: favor-the-shooter.md:197-199 accepts this on purpose: a cheat gains no more than an honest 250 ms player, and Source allows 1 s. But Source checks claims against measured lag; here nothing does, so the cap is the only bound and 0.4 s is derived from nothing.
  - Lead check: rewind_seconds is 0.4 (content/projectile/tuning.json:8). View clamps into [tick−R, tick−1] (view.go:86-94). InterpolationDelay = 0.1 lives only in timeline.ts:9. Holds.
- **Evidence:**
  - Verified: rewind_seconds is 0.4 (shards/game/content/projectile/tuning.json:8)
  - Verified: R = round(rewind × rate); KeptRows = R + 1 (shards/game/internal/projectile/flight/tuning.go:61-66)
  - Verified: View clamps the claimed view into [tick−R, tick−1] (shards/game/internal/projectile/flight/view.go:88-94)
  - Verified: ViewFloor raises a lower view and lets equal views pass (shards/game/internal/player/input_system.go:100-103)
  - Verified: InterpolationDelay 0.1 s exists only in TS (client/src/engine/sync/timeline.ts:9)
  - Verified: The doc accepts a cheat taking the full window, as an honest high ping does (docs/research/favor-the-shooter.md:197-199)
  - Verified: Source caps at sv_maxunlag and checks claimed vs measured lag within 0.2 s (https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/player_lagcompensation.cpp)
  - Summarized: Valorant cut its rewind cap from 200 to 140 ms (https://www.pcgamesn.com/valorant/patch-notes-6-02)

<a id="f10"></a>

### F10. Every tick resends all state as named msgpack: 1.4-2.9 Mbit/s per player at 60 Hz

**won't scale**, medium (reviewer said high; the skeptic downgraded it). From `netcode-1`.

- **Where:** `shards/game/internal/player/sync_system.go:98`, `shards/game/internal/echo/sync_system.go:66`, `shards/game/internal/ability/sync_system.go:46`, `shards/game/internal/projectile/sync_system.go:53`, `shards/game/internal/projectile/target_sync_system.go:41`, `shards/game/internal/player/sync_system.go:96`, `shards/game/internal/projectile/projectile_sync_event.go:20`, `world-engine@v0.22.0-beta/pkg/cardinal/internal/schema/schema.go:18`, `world-engine@v0.22.0-beta/pkg/cardinal/service.go:633`, `world-engine@v0.22.0-beta/pkg/cardinal/service.go:705`
- **What breaks:**
  - Five syncs go to each player every tick, complete. Each is a msgpack map with field names, float64 numbers and 9-byte uint64s. A shot costs 230 B, a Body 123 B.
  - 72-75% of each tick's bytes repeat the last tick.
  - Your Echo path is resent whole every tick and grows with the rate squared: 716 B a tick at 60 Hz.
  - Shipped content, fire held: 0.40-0.88 Mbit/s per player at 20 Hz (1v1 to 3v3), and 1.4-2.9 Mbit/s at 60 Hz. A 3v3 match sends 5.3 Mbit/s at 20 Hz and 17.2 at 60 Hz.
  - Busier content (20 shots in flight) reaches 4.9 Mbit/s per player at 60 Hz.
- **Measured:**
  - Probe content (bolt cooldown 0.05 s, range 5000), down kbit/s per player at 20/60 Hz: 2P0S 332/1218, 2P20S 1327/4202, 6P0S 711/2354, 6P20S 1552/4878.
  - One 6P20S tick at 60 Hz: projectile_sync 4,610 B, body_sync 2,393 B (Echo path 716 B), target_sync 1,584 B, echo_sync 965 B, ability_sync 344 B.
  - Skeptic, shipped content: 404/881 kbit/s per player (1v1/3v3) at 20 Hz, 1,432/2,862 at 60 Hz.
  - One deflate stream per player, flushed per event: 12-56% of raw. Flushed once per tick, or at level 9: 2-4.5x better again. Deflate costs 100-230 µs a tick and about 0.7 MB per player stream.
  - msgpack as_array saves 45% raw (59% with compact ints) but compresses about the same. Serializing all 30 events takes 196-256 µs a tick. TLS and TCP framing are not counted.
- **Mature engines:** Quake 3 keeps each client's last 32 snapshots and deltas against the last acked one [sourced: https://fabiensanglard.net/quake3/network.php]. Source delta-compresses its snapshots, and cl_updaterate defaults to 20 [sourced via search; the Valve wiki fetch returned 403]. Glenn Fiedler's 901 cubes cost 17.37 Mbit/s raw at 60 Hz. He sets a 256 kbit/s target and works toward it with quantization and deltas against acked baselines [sourced: https://gafferongames.com/post/snapshot_compression/; the article gives no final total]. Unreal Iris deltas against acked, quantized state [sourced: https://dev.epicgames.com/documentation/unreal-engine/API/Runtime/IrisCore/FNetSerializeDeltaArgs]. For scale, CS:GO's default client rate is about 1.57 Mbit/s [community sources only].
- **Fix:**
  - In world-engine, compress each player's event stream with one deflate context, flushed once per tick. On a reliable, ordered stream this works as a delta against recent ticks, with no acks and no game code. Browsers decode it natively.
  - connect-go flushes after every Send, so a per-tick flush means Cardinal sends a tick's events as one batch. A per-event flush still helps, just less.
  - Measured: shipped 3v3 at 60 Hz falls to 0.48 Mbit/s per player with a per-event flush, and 0.27 with a per-tick flush.
  - The same change carries over to item 18's WebSocket as permessage-deflate ([F24](#f24)).
- **Rejected:**
  - *Delta against a baseline per viewer (Quake 3, Source).* Best ratio. On a reliable stream it needs no acks, only a last-sent baseline per viewer (skeptic). It still loses: it needs code per sync, and it breaks the rule that every sync is complete.
  - *msgpack as_array plus compact ints.* Saves 45–59% raw but compresses about the same. It needs a hand-kept field order per TS type, and uint64 would stop decoding as bigint.
  - *Quantize positions (float32 or fixed point).* Breaks bit-exact prediction: TargetSync rows and YouState must carry the server's float64 bits.
  - *Relevancy or a priority budget (Unreal, Halo).* There are at most 6 players on one small arena, all visible. Nothing to cull or rank.
  - *omitempty on fields set only when a shot ends.* Saves about 73 B per shot per tick, but every TS reader must treat a missing field as zero. Compression removes the zeros anyway.
  - *One combined game-side sync event.* Saves four envelopes a tick but breaks package-by-feature syncs. One flush per tick in the engine gets the same saving.
- **Cost:**
  - Size: M
  - Risk: low: transport only. About 5% of a core and 0.7 MB per player stream for a 3v3 match at 60 Hz, at level 6.
  - Purge: no: only wire bytes change.
  - World-engine: yes: Cardinal's event stream gets one compressor per player stream, flushed once per tick.
- **Earlier review:** carried over from C6 and C4 (both still open).
- **Skeptic:**
  - Downgraded from high to medium. The cost is real, and the byte counts reproduce exactly.
  - The reviewer's top figures (4.9 Mbit/s, 29 Mbit/s per 3v3, 2.2 GB per player-hour) came from probe content: a 0.05 s bolt cooldown and range 5000. The shipped bolt has a 2 s cooldown and a 1.33 s life, so each Body has at most one shot in flight. This report quotes shipped numbers first.
  - No wrong result and no written budget. The fix is transport-only in world-engine, so waiting adds no game rework. 20 Hz stays under CS:GO's default client rate.
  - Cardinal's path flushes per event, not per tick, so the per-tick numbers need batching.
  - Fiedler's article sets a 256 kbit/s target but gives no final total.
- **Evidence:**
  - Verified: Each feature sends its complete sync to every player every tick (shards/game/internal/player/sync_system.go:98; shards/game/internal/echo/sync_system.go:66; shards/game/internal/ability/sync_system.go:46; shards/game/internal/projectile/sync_system.go:53; shards/game/internal/projectile/target_sync_system.go:41)
  - Verified: Cardinal encodes events with plain msgpack.Marshal: maps keyed by field names, float64, 8-byte uint64 (world-engine@v0.22.0-beta/pkg/cardinal/internal/schema/schema.go:17-23)
  - Verified: Each event is its own stream message that carries the shard address and the event name (world-engine@v0.22.0-beta/pkg/cardinal/service.go:633-716)
  - Verified: YouState.EchoPath sends 1 + echo_path_seconds × rate poses every tick: 11 at 20 Hz, 31 at 60 Hz (shards/game/internal/player/sync_system.go:90-96; shards/game/internal/echo/component/echo.go:36-46)
  - Verified: A shot is 17 named fields (230 B); fixes 1, 4, 5 and 8 added 8 of them (shards/game/internal/projectile/projectile_sync_event.go:20-35)
  - Verified: Byte counts, compression ratios and serialize cost (copy: shards/game/probe_netcode_test.go; scratchpad/bandwidth-20hz.txt and bandwidth-60hz.txt)
  - Summarized: Go sets TCP_NODELAY by default, so each flushed event can become its own TCP segment (Go net package default)
  - Verified: Shipped content, fire held: 404/881 kbit/s per player (1v1/3v3) at 20 Hz, 1432/2862 at 60 Hz; connect-go flushes after every Send (skeptic's TestSkepticShipped and TestSkepticFlushSplit (scratch copy); connect@v1.21.0 protocol_connect.go:807-808)

<a id="f11"></a>

### F11. A silently dead event stream freezes play while the HUD says Connected

**not standard**, medium. From `physics-client-2`.

- **Where:** `go-ecs/pkg/sdk-ts/src/shard/create-stream-manager.ts:95-136`, `client/src/engine/net/shard_connection.ts:40-46`, `client/src/game/ui/hud.ts:83-88`, `client/src/game/player/input_stream.ts:6,85`
- **What breaks:** Wi-Fi handoff, sleep or a NAT drop can kill the TCP path without a FIN. The stream's for-await then just waits: no idle timer, and the 30 s keepalives are read but never timed. Syncs stop, the render clock holds at the newest tick, everyone freezes, and the HUD still says Connected until a TCP-level timeout fires, likely minutes later. Inputs unacked past 6.4 s are dropped from the queue with no signal.
- **Measured:** Not run: the game is off limits. From code: no timer around the for-await, keepalive responses skipped untimed, no timeout on the Connect client. Render-clock sim with the real Timelines: a 300 ms stall freezes the view 167-208 ms, then catches up at up to 1.48x; with no new ticks the clock holds indefinitely.
- **Mature engines:** gRPC clients send keepalive pings and close the connection when no ack arrives within a timeout [sourced: https://github.com/grpc/proposal/blob/master/A8-client-side-keepalive.md]. Quake and Source clients warn of a connection problem after about a second without packets and drop at a timeout [from memory]. Browser script can't send HTTP/2 pings, so web clients time the server's own heartbeats [from memory].
- **Fix:** In the SDK stream manager, add an idle timeout: if no response, keepalive included, arrives within it, abort the call and reconnect through the existing backoff, which already reports 'Reconnecting' to the HUD. Default it above the 30 s keepalive; the game passes about 2 s, since it gets a sync every tick. This is a monorepo pkg/sdk-ts change; ask first.
- **Rejected:**
  - *A game-level watchdog in shard_connection.ts.* Every SDK user has the same hole. The stream manager owns the call and its abort, so a game watchdog duplicates the timer and fights its reconnect.
  - *A shorter server keepalive alone.* Pings that nobody times detect nothing; the client still waits on the dead socket.
  - *Rely on TCP keepalive.* Script can't set it; Chromium's fixed 45 s idle plus probes takes minutes to notice [from memory].
- **Cost:**
  - Size: S: a timer reset per response and one option in create-stream-manager.ts; the game passes 2 s.
  - Risk: Low: the idle abort must surface as a retryable code (Canceled), never Aborted, which is terminal there; 2 s leaves 40 ticks of slack at 20 Hz.
  - Purge: No: client only.
  - World-engine: No: monorepo pkg/sdk-ts, the client's local SDK checkout. Ask first.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: If inputs share the stream's HTTP/2 connection, their retransmission failures may kill it sooner, in tens of seconds depending on the OS. I did not run the game, so the freeze length is inferred from code, not observed.
- **Evidence:**
  - Verified: The stream loop has no timer; keepalives carry no event and are skipped. (go-ecs/pkg/sdk-ts/src/shard/create-stream-manager.ts:101-115)
  - Verified: Reconnect uses 0.5-8 s backoff; Aborted, Unauthenticated, PermissionDenied, InvalidArgument are terminal. (go-ecs/pkg/sdk-ts/src/shard/create-stream-manager.ts:7-16,129-135,158)
  - Verified: The Connect client sets no timeout. (go-ecs/pkg/sdk-ts/src/shard/create-connect-client.ts (grep: no timeout))
  - Verified: Cardinal sends a keepalive every 30 s to beat ALB idle timeouts. (world-engine@v0.22.0-beta/pkg/cardinal/service.go:484-486)
  - Verified: HUD shows 'Connected as ...' from the stream state alone; 'Reconnecting: ...' while connecting. (client/src/game/ui/hud.ts:83-93)
  - Verified: Unacked inputs past 6.4 s are shifted out; nothing acts on it. (client/src/game/player/input_stream.ts:6,85)
  - Verified: The render clock never passes the newest tick, so missing syncs freeze the view. (client/src/engine/sync/timeline.ts:22-27)

<a id="f12"></a>

### F12. Every body_sync replays all unacked frames, even when nothing was wrong

**won't scale**, medium. From `prediction-2`.

- **Where:** `client/src/game/player/local_player.ts:127`, `client/src/game/player/prediction.ts:83-120`, `shards/game/internal/predict/predict.go:230`, `shards/game/internal/predict/predict.go:338`, `shards/game/predictor/main.go:95-183`
- **What breaks:** Each body_sync resets the predictor and replays every unacked frame through wasm, then reads shots, Echo and kit back. Steps per second = rate × (unacked + 1), so cost grows with rate times latency. At 60 Hz and 200 ms RTT that is about 1,000 steps and 67-84 ms of main-thread CPU a second on a fast desktop, mostly re-deriving answers that were already right. Slower laptops and phones lose frames first.
- **Measured:** In P/echo_image: go test -run XXX -bench . -benchtime=2000x ./shards/game/internal/predict/ (and -tags rate60). Native Step 2-7 µs, 9-20 allocs (a map per step at predict.go:230). Wasm built into P/wasm, timed in node 24 with P/bench/wasm_bench.mjs on an i9-11900K (quietest run): one reconcile at 20 Hz, 4 frames/3 shots 391 µs, 6 frames 468 µs; at 60 Hz, 10 frames 706 µs, 16 frames 921 µs, 16 frames/6 shots 1212 µs. Fixed parts: reset 78 µs, shots+echo+kit 90 µs; seeTargets(12) ~180 µs per target_sync (10.8 ms/s more at 60 Hz). Wasm ≈ 15-25× native. Steps/s: 20 Hz ~100 (100 ms RTT) / ~140 (200 ms); 60 Hz ~660 / ~1020. CPU: 11-13 ms/s at 20 Hz; ~54 ms/s at 60 Hz/100 ms; 67-84 ms/s at 60 Hz/200 ms. One step is O(T + S·(log T + C)): T targets placed, S shots, C candidates. Machine shared (load up to 50); loaded runs were 1.5-3× slower.
- **Mature engines:** Source predicts from the last acked state but, with cl_pred_optimize 2, skips the replay when that state matches what it predicted [sourced: https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/client/prediction.cpp]. Overwatch keeps each predicted frame and resimulates only after a misprediction [sourced: https://edgegap.com/blog/game-backend-deep-dive-overwatch-2016-netcode-architecture-rollback] (secondary source). Quake 3 replays every unacked command each frame, but at 20 Hz with tiny steps [from memory].
- **Fix:** Keep the predicted state after each Seq. On body_sync, compare the acked state with the stored one for that Seq, bit for bit (same Go code, so equal means right). Replay only when they differ, or when rows a frame used arrived after it was predicted. Otherwise drop the acked entries and keep going. Client only, no purge. Add a test that a clean run replays nothing; keep the bad-network recordings as the oracle.
- **Rejected:**
  - *Make each step cheaper (no map per step, fewer js.Value reads).* Worth doing anyway, but only a 2-3× constant. The step count still grows with rate × RTT.
  - *Replay only the last few frames.* Older unacked frames still shape the present; skipping them leaves a stale or wrong pose.
  - *Move the predictor to a Web Worker.* Moves the CPU off the main thread but does not remove it, and adds message latency to your own movement.
  - *Ship only at 20 Hz.* Hides the cost; CLAUDE.md says the same game runs at 60 Hz and the recordings test both.
- **Cost:**
  - Size: M
  - Risk: medium: a wrong equality would skip a needed replay; the bad-network recordings catch that
  - Purge: no: client only; server state unchanged
  - World-engine: no
- **Earlier review:** new (T9 judged reset-and-replay fine at 20 Hz; this is its 60 Hz cost, now measured)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: At 20 Hz the cost is ~12 ms a second, which is fine, and desktops cope even at 60 Hz. But CLAUDE.md says the same game runs at 60 Hz, and phones are likely several times slower than this i9. Checked T9's reasoning and the per-step allocations.
- **Evidence:**
  - Verified: reconcile runs on every body_sync with all unacked frames (client/src/game/player/local_player.ts:127)
  - Verified: reconcile resets, then steps each unacked frame (client/src/game/player/prediction.ts:83-120)
  - Verified: place allocates a map every step (shards/game/internal/predict/predict.go:230)
  - Verified: Step places targets and flies shots on every frame, replays included (shards/game/internal/predict/predict.go:338)
  - Verified: the glue reads each field through js.Value; uint64 travels as decimal strings (shards/game/predictor/main.go:95-183)
  - Verified: Source cl_pred_optimize 2 skips repredicting when there were no errors (https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/client/prediction.cpp)
  - Summarized: Overwatch resimulates only after a misprediction (https://edgegap.com/blog/game-backend-deep-dive-overwatch-2016-netcode-architecture-rollback)

<a id="f13"></a>

### F13. SnapshotRate is a raw tick count; seven separate seconds-to-ticks conversions

**won't scale**, medium. From `ecs-4`.

- **Where:** `shards/game/main.go:24`, `shards/game/internal/ability/hooks/hooks.go:28`, `shards/game/internal/projectile/flight/tuning.go:62`, `shards/game/internal/echo/tuning.go:34`, `shards/game/internal/player/tuning.go:35,41,46`, `shards/game/internal/ability/cast/tuning.go:36`
- **What breaks:** SnapshotRate 10 is a snapshot every 0.5 s at 20 Hz but every 0.17 s at 60 Hz, while the rings it saves triple. Measured 3v3 payload: 67 KB at 20 Hz, 160 KB at 60 Hz, so 132 KB/s becomes 940 KB/s per match (7.1x). Dev runs debug by default, which encodes every tick. Cause: seven hand-written seconds-to-ticks conversions, and SnapshotRate was missed. The next duration can be missed too.
- **Measured:** TestProbeSnapshotSize, 6 Bodies, 6 Echoes, 21 shots: 67,375 B at 20 Hz, 160,375 B at 60 Hz. PathHistory 34.6 KB to 103 KB (64% at 60 Hz); TargetHistory 13.9 KB to 38.5 KB. pprof over 4,000 TestWorld ticks: snapshot encode is 29% of bytes allocated at 20 Hz, 48% at 60 Hz.
- **Mature engines:** Unity Netcode sets snapshot cadence apart from the sim rate: NetworkTickRate vs SimulationTickRate [sourced: https://docs.unity3d.com/Packages/com.unity.netcode@1.9/api/Unity.NetCode.ClientServerTickRate.html], and calls snapshot building often the largest CPU cost [sourced: https://docs.unity3d.com/Packages/com.unity.netcode@1.9/manual/client-server-worlds.html]. Source keeps lag-compensation history in seconds (sv_maxunlag, 1 s) [sourced: https://developer.valvesoftware.com/wiki/Lag_Compensation] and converts time with TIME_TO_TICKS [from memory].
- **Fix:** Add one timestep.Ticks(seconds) beside timestep.Rate and route every duration through it, SnapshotRate included (0.5 s: 10 ticks at 20 Hz, 30 at 60 Hz). Every wall-clock meaning then survives a rate change, and a review can grep for any new math.Round(x * Rate). Ring sizes stay what content's seconds ask for.
- **Rejected:**
  - *Set CARDINAL_SNAPSHOT_RATE per build.* main.go's value overrides the env (config.go:67-69), and it is a second place to keep in step with the rate.
  - *Sample PathHistory at 20 Hz to shrink the rings.* Cuts bytes once but blurs the Echo replay, and the snapshot cadence stays tick-bound.
  - *A second hand-set SnapshotRate for 60 Hz builds.* Another raw constant per rate; the same miss returns at the next rate.
- **Cost:**
  - Size: S: one helper, 8 call sites
  - Risk: low: identical values at 20 Hz
  - Purge: no: snapshot format unchanged
  - World-engine: no; optional: Cardinal could accept the interval in seconds
- **Earlier review:** carried over from C8 and C11 (SnapshotRate part still open). Raised from low: bytes now measured, 7.1x.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: At 20 Hz, the shipped rate, 132 KB/s is fine, and 60 Hz is only a build tag. Debug encoding is dev-only. But tests already run at 60 Hz, and this is the kind of miss CLAUDE.md forbids.
- **Evidence:**
  - Verified: SnapshotRate is 10, a tick count (shards/game/main.go:22-25; world-engine@v0.22.0-beta/pkg/cardinal/config.go:21)
  - Verified: Code options override the env value (world-engine@v0.22.0-beta/pkg/cardinal/config.go:67-69,154)
  - Verified: A snapshot is due at height % SnapshotRate; debug encodes every tick (world-engine@v0.22.0-beta/pkg/cardinal/world.go:318-319)
  - Verified: world start defaults to debug on (world-engine@v0.22.0-beta/cli/commands/root/start.go:29; world-engine@v0.22.0-beta/cli/pkg/docker/service/cardinal.go:133)
  - Verified: Seven separate seconds-to-ticks conversions (hooks.go:28, flight/tuning.go:62, echo/tuning.go:34, player/tuning.go:35,41,46, cast/tuning.go:36)
  - Verified: Payload 67,375 B vs 160,375 B (copy only: TestProbeSnapshotSize at default and -tags rate60)

<a id="f14"></a>

### F14. 60 Hz recording lost replay-ending shots; a loosened TS guard hides it

**duct tape**, medium. From `tests-2`.

- **Where:** `client/src/game/player/predictor.test.ts:52-54`, `shards/game/main_test.go:2735-2737`, `shards/game/main_test.go:2745`, `shards/game/main_test.go:2642-2645`, `shards/game/testdata/prediction-60hz.txt`
- **What breaks:** X5 made the recording prove that a replay of several frames ends shots, out of range and on bob, in Go and in the wasm. At 60 Hz no recorded multi-frame step ends a shot, and Go's badNetwork has no deep replay ending on bob. Fix X6 swapped the exact TS guard for a set check, so 60 Hz passes anyway. A 60 Hz-only bug in replays that end shots would pass.
- **Measured:** Parsed both digests. Ends as (tick, several frames, on target): 20 Hz (26,n,n) (54,n,y) (104,y,n: 8 frames) (137,y,y: 3 frames); 60 Hz (79,n,n) (158,n,y) (406,n,y). Counter probe in badNetwork (a scratch copy): 20 Hz exact 137, deep 78, hits 3, deep hits 2; 60 Hz exact 687, deep 520, hits 1, deep hits 0. X5's exact guard (old copy in mutX5b/backup) fails the 60 Hz recording; today's guard passes it.
- **Mature engines:** Riot's Delta Checker drives repeatable tests from recorded inputs [sourced: https://www.riotgames.com/en/news/determinism-league-legends-introduction]. A recorded scenario is worth what it provably covers, so coverage asserts stay exact per scenario, and a gap is fixed by changing the scenario, not the assert [from memory].
- **Fix:** Assert coverage where the run is made: badNetwork also requires a replay of several frames that ends on bob, and the recording keeps one. Time the held burst from the bolt's flight time to bob, in seconds, so both rates produce it. In TS, require all four (several frames, on target) pairs as a set: rate-agnostic and still exact.
- **Rejected:**
  - *Keep the set guard.* It passes a 60 Hz recording with no multi-frame ends, so X5's coverage is gone at 60 Hz with no failing test.
  - *Per-rate expected tables in TS.* Two hard-coded tables tied to tick counts; any scenario edit breaks both.
  - *Record only 20 Hz.* Drops the wasm check at 60 Hz that X6 added.
- **Cost:**
  - Size: small: ~20 lines and a re-record at both rates
  - Risk: low
  - Purge: no: tests only
  - World-engine: no
- **Earlier review:** fix X6 is duct tape: it loosened X5's exact ends guard (predictor.test.ts:52-54), so the 60 Hz recording passes with no multi-frame replay ending a shot
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: The Go code is the same at both rates and 20 Hz covers the path, so only a rate-specific bug slips through. Checked Go's 60 Hz counters: deep hits are 0, so Go doesn't cover it either. Medium, not high.
- **Evidence:**
  - Verified: The TS guard checks only the set of OnTarget values and that some step has several frames (client/src/game/player/predictor.test.ts:52-54)
  - Verified: X5's guard required the pairs [F,F] [F,T] [T,F] [T,T] in order (earlier scratchpad mutX5b/backup/client__src__game__player__predictor.test.ts:51-59)
  - Verified: hits counts any exact replay ending on a target, one frame or several (shards/game/main_test.go:2735-2737)
  - Verified: The vacuity check needs deep and hits, but no deep hit (shards/game/main_test.go:2745)
  - Verified: 60 Hz digest ends at ticks 79, 158, 406, all single-frame steps (shards/game/testdata/prediction-60hz.txt)
  - Summarized: Fix log: 'TS ends guard now rate-agnostic' (fix-progress.md:136 (earlier scratchpad))

<a id="f15"></a>

### F15. No test restores a snapshot, though the shard snapshots every 10 ticks

**not standard**, medium. From `tests-4`.

- **Where:** `shards/game/main.go:24`, `CLAUDE.md:440-441`, `shards/game/main_test.go:2143`, `world-engine@v0.22.0-beta/pkg/cardinal/testworld.go:158-221`, `world-engine@v0.22.0-beta/pkg/cardinal/dst.go:128-129`, `docs/research/system-architecture-options.md:182`
- **What breaks:** A crash or deploy restores the last snapshot mid-match. Several rules exist only for that: full-key sorts, the saved serial counter, ViewFloor kept across sessions, all state in components. Nothing checks that a restored match continues bit for bit like one that never stopped. A field kept outside components, or a physics rebuild that differs, would desync every predictor or crash-loop the shard, seen first in production.
- **Measured:** grep: no snapshot or restore test in echo_image; TestEffectsAreDeterministic runs two fresh worlds. Cardinal TestWorld (v0.22.0-beta) has Command, RunSystem, Tick, StartGame, Events, Emitted, ShardCommands: no restore. RunDST's restart and restore ops are commented out in v0.22.0-beta and on world-engine main (6fbcaa7e).
- **Mature engines:** Riot's Chronobreak restores a live server to an exact earlier state, which works because the server is deterministic [sourced: https://www.riotgames.com/en/news/determinism-league-legends-introduction]. Rollback netcode tests save, load and resimulate for equality [from memory].
- **Fix:** World-engine: give TestWorld a restore step that encodes the state and rebuilds a fresh world from it, as a snapshot restore does (or switch on RunDST's restore op). Then echo_image adds one test: a script with shots, deaths, effects, an Echo and queued frames, restored partway, must match the uninterrupted run at every later tick, using RequireDeterministic's digests.
- **Rejected:**
  - *Hand-rolled snapshot code in echo_image.* Reaches into Cardinal internals the game can't import.
  - *RunDST as is.* Its restore op is off, and it checks ECS structure, not that a match continues the same.
  - *Rely on review-changes.md's snapshot lens.* A review reads code; it can't prove a restored match continues bit for bit.
- **Cost:**
  - Size: medium: a TestWorld method in world-engine, ~40 lines of test here
  - Risk: low
  - Purge: no
  - World-engine: yes: a TestWorld restore step, or RunDST's restore op turned on
- **Earlier review:** new (the earlier review disputed 'deterministic when emit order changes' on this point, with no ID)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: The design looks restore-safe: state is in components and receivers sort by full keys. But physics2d rebuilds Box2D on restore, and nothing tests the whole path. Restores are rare, but they hit every player at once: medium.
- **Evidence:**
  - Verified: The shard snapshots every 10 ticks (shards/game/main.go:24)
  - Verified: Receivers sort by a full key because emit order can change after a restore (CLAUDE.md:440-441)
  - Verified: The only determinism test runs two fresh worlds, no restore (shards/game/main_test.go:2143-2167)
  - Verified: TestWorld has no restore method (world-engine@v0.22.0-beta/pkg/cardinal/testworld.go:158-221)
  - Verified: RunDST's restart and restore ops are commented out (world-engine@v0.22.0-beta/pkg/cardinal/dst.go:128-129)
  - Verified: The options doc planned a snapshot-restore-step test (docs/research/system-architecture-options.md:182)

<a id="f16"></a>

### F16. Effect refresh ignores strength: a weaker, longer effect erases a stronger one

**won't scale**, medium. From `content-3`.

- **Where:** `shards/game/internal/effect/status/status.go:95-117`, `shards/game/internal/effect/status/status.go:70-75`, `shards/game/internal/effect/status/tuning.go:67-75`, `shards/game/content/effect/tuning.json:3-6`
- **What breaks:** Add refreshes an entry of the same kind (and the same source, for per_source) by its end tick alone. A stronger, shorter effect is dropped, and a weaker, longer one replaces a stronger one. Fold says only the strongest slow counts, but the strong slow is already gone. The design puts the numbers on the intent, so across 50+ abilities one kind will carry many strengths. The planned slowing shot plus slow zone hits this first.
- **Measured:** Probe on status.Add in a scratch copy, base speed 5. A 60% slow for 20 ticks, then a 20% slow for 40 from the same source: speed 4.00 at tick 11, where the strongest-slow rule says 2.00. A 20% slow for 40, then a 60% slow for 5: 4.00 (want 2.00). Merge: a 50% haste, then a 10% haste from another source: 5.50 (want 7.50).
- **Mature engines:** In League, the strongest slow applies until it expires, then the next slow still active takes over [sourced: https://lol.fandom.com/wiki/Rylai%27s_Crystal_Scepter]. GAS keeps each application as its own instance unless the effect stacks. Stacks aggregate by source or by target, with policies for duration refresh and expiry [sourced: https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/GameplayAbilities/EGameplayEffectStackingType] [sourced: https://raw.githubusercontent.com/tranek/GASDocumentation/master/README.md].
- **Fix:** Make a refresh compare strength as well as the end tick. A new entry replaces an old one only if it is at least as strong and ends at least as late. It is dropped only if the old one is both. Otherwise keep both, and Fold counts the strongest active entry per refresh key (kind, plus source for per_source). Stuns carry no number, so they behave as today.
- **Rejected:**
  - *Make every kind that carries a number instanced.* A slow zone re-applies every tick, so the list grows every tick, and summed hastes count the same ability twice.
  - *Newest always replaces, like a Dota refresh.* A short stun would then cut a long one short, which is exactly what today's rule exists to stop.
  - *A content rule: one kind per strength.* The numbers move from the applier into kinds, so 50 abilities need 50 kinds. That breaks the design's rule that the applier sends the numbers.
- **Cost:**
  - Size: S: status.Add, Fold and unit tests
  - Risk: low to medium: predicted code. The recorded run has no overlapping effects with numbers, so the recordings should not change.
  - Purge: no: Entry keeps its fields
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: No shipped ability sends a Value (hooks.Effect has none), and tests use one strength, so play is fine today. But the design puts numbers on the intent, and the planned slows on hit and slow zone reuse one kind at different strengths.
- **Evidence:**
  - Verified: A refresh replaces only when the new End is later, and otherwise drops the new entry (shards/game/internal/effect/status/status.go:105-110)
  - Verified: Fold keeps only the strongest slow (shards/game/internal/effect/status/status.go:70-75)
  - Verified: The applier sends Value; the kind holds no number (shards/game/internal/effect/status/tuning.go:72)
  - Verified: No hook can send a Value yet: hooks.Effect has none, and cast.Run never sets it (shards/game/internal/ability/hooks/hooks.go:44-49; shards/game/internal/ability/cast/cast.go:104-105)
  - Verified: Planned: slows on hit, and a per-source slow zone where the strongest wins (docs/research/system-architecture-design.md:448; docs/research/character-foundation.md:108)

<a id="f17"></a>

### F17. Tag rules are Go code, so any cc, even a slow, ends a dash

**not generic**, medium. From `content-4`.

- **Where:** `shards/game/internal/effect/status/status.go:95-101`, `shards/game/internal/ability/cast/cast.go:55`, `shards/game/content/effect/tuning.json:4-6`, `shards/game/internal/interact/interact.go:33-38`, `docs/research/character-foundation.md:84`
- **What breaks:** status.Add hard-codes two rules: any cc removes every ends_on_cc entry, and unstoppable blocks any cc. The gate hard-codes stunned. The shipped slow has cc, so a slow ends a dash (probe), though the foundation doc says only a stun or root does. Content can't say 'unstoppable blocks it, but it doesn't interrupt'. Silence, root, cleanse and similar effects each need an edit to predicted core code, plus a re-record.
- **Measured:** Probe on status.Add: a dash (ends_on_cc), then a 10% slow (cc) one tick later. Result: forced=false, velX=0, one entry left. Tags in use: 4 of 64. Tag meanings live in 2 Go places (status.Add and cast.Gate).
- **Mature engines:** GAS keeps these relations as data on each ability: CancelAbilitiesWithTag, BlockAbilitiesWithTag and ActivationBlockedTags. Effects grant immunity by tag [sourced: https://raw.githubusercontent.com/tranek/GASDocumentation/master/README.md]. Dota keeps a closed list of unit states with fixed meanings, like echo_image's tags, but splits them finely. A root stops movement without touching move speed, and it interrupts casts [sourced: https://liquipedia.net/dota2/Root].
- **Fix:** Keep tags a closed Go list, but move their relations into effect-kind data. Each kind lists `ends` (tags whose entries it removes when it lands) and `blocked_by` (holder tags that stop it). Ability rows get one default `blocked_by` for the gate. Then stun ends ends_on_cc and is blocked by unstoppable, while slow is only blocked by unstoppable. A new interaction becomes a content row.
- **Rejected:**
  - *Split cc into hard_cc and soft_cc in Go.* That fixes slows, but silence, root and cleanse would each still need another Go rule in predicted code.
  - *Drop cc from slow in content.* Then unstoppable stops blocking slows, which breaks the CC-immunity meaning of the tag.
  - *Keep it and document the rule.* The planned slowing shots and zones would cancel dashes, unlike every reference game.
- **Cost:**
  - Size: S: status.Add, KindDef, the gate, content and unit tests
  - Risk: low: predicted code, but the recorded stun still ends the dash, so the recordings should hold
  - Purge: no: the new kind fields are content, and Entry state is unchanged
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: No shipped ability applies a slow, so play isn't hit yet, and a closed tag list is the design's stated choice. But that choice covers the list, not the relations, and the shipped content contradicts the foundation doc's 'a stun or root ends your dash'.
- **Evidence:**
  - Verified: Any cc removes ends_on_cc entries, and unstoppable blocks cc (shards/game/internal/effect/status/status.go:96-101)
  - Verified: The gate blocks casts only for stunned (shards/game/internal/ability/cast/cast.go:55)
  - Verified: The shipped slow carries cc, and dash carries ends_on_cc (shards/game/content/effect/tuning.json:4, 6)
  - Verified: The foundation doc says a stun or root ends your own dash (docs/research/character-foundation.md:84)
  - Verified: The design chose a closed tag list in Go (docs/research/system-architecture-design.md:546)

<a id="f18"></a>

### F18. One synced or predicted field means 9-15 hand edits

**not generic**, medium. From `netcode-4, prediction-4 (glue part)`.

**Status (2026-11-07):** open. Lag compensation moved `TargetSync` to `track` (same wire name) and added
`You.EchoSerial` by hand at each site, now with a glue test. The paths below predate it.

- **Where:** `shards/game/internal/projectile/target_sync_event.go:21`, `shards/game/internal/player/body_sync_event.go:64`, `shards/game/internal/echo/echo_sync_event.go:20`, `shards/game/internal/player/sync_system.go:72`, `client/src/main.ts:74`, `client/src/game/player/predictor.ts:94`, `shards/game/predictor/main.go:126`, `shards/game/internal/predict/predict.go:105`, `shards/game/main_test.go:2275`, `shards/game/internal/predict/predict.go:62-79`, `shards/game/predictor/main.go:126-183`, `client/src/game/player/predictor.ts:98-121`, `client/src/game/player/prediction.ts:55-59`
- **What breaks:**
  - TargetSync sends each Body's and Echo's row. BodySync and EchoSync send the same spot, radius, team, tags and serial again.
  - Your own shots go out twice: in YouState.Shots and in projectile_sync, which main.ts drops.
  - Each field is typed again in the Go events, the TS types, predictor.ts, the wasm glue, predict and main_test.
  - A YouState field touches 9 places. A row field like the jump bit touches about 15. A new predicted request type touches about 12.
  - A field the glue skips stays zero, and only a recording that uses it notices.
- **Measured:** 6P20S at 60 Hz: target_sync (1584 B) repeats Bodies (737 B) and Echoes (943 B). Own shots in projectile_sync cost 916 B a tick; at 2P20S, 2290 B, which is 27% of the tick. YouState.Shots costs 160 B a shot. A YouState field touches 9 places: body_sync_event.go, you() in sync_system.go, body_sync_event.ts, reset in predictor.ts, state() in predictor/main.go, predict.State, predictFrom in main_test, and the Go and TS wire fixtures. The input frame type is written 5 times: component.InputFrame, predict.Frame, the TS InputFrame, the wasm frame() reader and predictFrame in main_test. A predicted request type: predict.go State 62-79, Reset 259-264, Step 321-341, asks 343-357; predictor/main.go 126-183; predictor.ts 98-121; prediction.ts 55-59; the server sync; tests; recordings at both rates (counted by reading).
- **Mature engines:** Unity Netcode for Entities serializes and rolls back predicted components from one [GhostField] definition [from memory]. Unreal replicates the same UPROPERTY structs on both ends, and Iris builds its state descriptors from them [from memory]. Neither copies a field by hand between transport, client and predictor. Unreal's COND_SkipOwner keeps the owner from getting what it simulates itself [from memory]. Overwatch runs the same ECS systems on client and server, so prediction covers whatever those systems touch [from memory].
- **Fix:**
  - One record per thing, decoded by one Go type on both sides.
  - Clients draw Bodies and Echoes from TargetSync's rows. BodySync and EchoSync keep only what rows lack (player, nickname, HP). A viewer's projectile_sync leaves out its own predicted shots.
  - The SDK hands the wasm each sync's raw msgpack bytes. The wasm decodes them into the server's own event types, kept Cardinal-free (cardinal.Slice is immutable.Slice).
  - A new field is then one Go edit plus its TS type.
- **Rejected:**
  - *Generate the TS types and glue from Go.* CLAUDE.md chose no codegen, and a generator is one more build step to keep. Raw bytes into the server's own types need neither.
  - *Keep the duplicates and let compression hide the bytes.* Fixes the bytes only. The 9–15 edits per field and the silent zero fields stay.
  - *Re-encode the decoded objects in TS for the wasm.* Decodes, encodes and decodes again every tick, though the SDK already holds the raw bytes.
  - *Keep wiring by hand with a checklist.* Each new field or request repeats 9-15 edits, and risks a site the bit-exact tests don't cover.
- **Cost:**
  - Size: M
  - Risk: medium: the client draw path and the wasm boundary change. Wire fixtures and recordings catch drift. Whether the wasm accepts immutable.Slice's dependencies is untested.
  - Purge: no: events only, no components change
  - World-engine: yes (go-ecs SDK): a callback that hands over the raw payload; everything else is in echo_image
- **Earlier review:** carried over from CV1, S4 and G2 (still open).
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check (`netcode-4`): Fix 12 chose a separate TargetSync on purpose and notes the double send. Wire fixtures catch Go↔TS drift, but not glue drift unless the recording uses the field. Compression would hide the bytes, not the edits. Medium holds.
  - Reviewer's check (`prediction-4`): Two kinds and one predicted request exist today; generalizing early has its own cost, and a new shape still needs its cast. But barriers as targets are planned, and favor-the-shooter.md:100-102 already promises new kinds for free. Checked each site by reading it.
- **Evidence:**
  - Verified: TargetState repeats the spot, radius, team, tags and serial that BodyState and EchoState send (shards/game/internal/projectile/target_sync_event.go:21-31; shards/game/internal/player/body_sync_event.go:64-73; shards/game/internal/echo/echo_sync_event.go:20-32)
  - Verified: Own predicted shots are sent twice, and the client drops one copy (shards/game/internal/player/sync_system.go:72-84; shards/game/internal/projectile/sync_system.go:24-41; client/src/main.ts:73-75)
  - Verified: The predictor glue copies each field by hand (client/src/game/player/predictor.ts:91-127; shards/game/predictor/main.go:105-183; shards/game/main_test.go:2275-2310)
  - Verified: predict.Frame duplicates component.InputFrame (shards/game/internal/predict/predict.go:105-112; shards/game/internal/player/component/input.go:14-20)
  - Verified: cardinal.Slice is an alias for immutable.Slice (world-engine@v0.22.0-beta/pkg/cardinal/system.go:209)
  - Summarized: Fix 12 chose a separate TargetSync on purpose and notes that spots are sent twice (fix-progress.md, item 12)
  - Verified: the glue reads every predicted field by name through js.Value (shards/game/predictor/main.go:126-183)

<a id="f19"></a>

### F19. A non-circle or new target kind needs about 9 edits

**not generic**, medium. From `prediction-4 (target shape part)`.

**Status (2026-11-07):** kinds fixed, shape open. A thing is a target because it carries `Track`, and
your own Body and Echo are one placement rule (`Track.Predicted`) on both sides. A row is still one
circle. The code below moved to `track/past`.

- **Where:** `shards/game/internal/predict/predict.go:204-212`, `shards/game/internal/predict/predict.go:236-249`, `shards/game/internal/projectile/flight_system.go:121-143`, `shards/game/internal/projectile/flight_system.go:165`, `shards/game/internal/projectile/flight/tuning.go:50-52`, `shards/game/internal/projectile/flight/view.go:14-22`, `shards/game/internal/projectile/flight/view.go:129-134`
- **What breaks:**
  - Row is one circle. ensure makes circles. The pair cast is ShapeCastCircle.
  - targetKinds is a const naming Body and Echo, and targetsOf requires Effects.
  - Your own Body and your own Echo are two more special cases.
  - So a box barrier, which the design plans as a target, touches about 9 places.
  - favor-the-shooter.md:100-102 promises a new kind of target needs no predictor, glue or shot code.
- **Measured:** Counted by reading: Row view.go:14-22, AtView 36-50, Reach.Add 62-82, hitOn 129-134, ensure predict.go:204-212, place 236-249, seenAt flight_system.go:121-143, targetsOf 165, targetKinds tuning.go:52. Special cases: own Body predict.go:236-240 and flight_system.go:126-130; own Echo predict.go:247-249 and flight_system.go:139-141.
- **Mature engines:** Unreal hit results carry the hit component and its own shape, not a fixed circle [from memory].
- **Fix:** Give Row a shape (circle, capsule, box) and cast by the row's shape. A thing is a target because it carries TargetHistory, not because a const lists its kind.
- **Rejected:**
  - *Keep wiring by hand with a checklist.* Fine for two kinds; each new one repeats 9-12 edits and risks a site the bit-exact tests don't cover.
  - *Run Cardinal itself in wasm.* Pulls about 737 packages into the predictor, breaks the shared-core scan, and bloats a 5.1 MB wasm.
- **Cost:**
  - Size: M for this part. The reviewer's L covered [F18](#f18)'s glue too.
  - Risk: medium: it touches the bit-exact path, and recordings are re-made at both rates.
  - Purge: yes: TargetHistory rows gain a shape field, so old snapshots won't decode.
  - World-engine: no
- **Earlier review:** carried over from G6 (fix 12 made kinds one const and rows generic; a row is still one circle).
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Two kinds and one predicted request exist today; generalizing early has its own cost, and a new shape still needs its cast. But barriers as targets are planned, and favor-the-shooter.md:100-102 already promises new kinds for free. Checked each site by reading it.
- **Evidence:**
  - Verified: Row holds one circle (Center, Radius) (shards/game/internal/projectile/flight/view.go:14-22)
  - Verified: the pair cast is ShapeCastCircle (shards/game/internal/projectile/flight/view.go:129-134)
  - Verified: targetKinds is a const naming Body and Echo (shards/game/internal/projectile/flight/tuning.go:50-52)
  - Verified: own Body and own Echo are special-cased in the predictor and the server (shards/game/internal/predict/predict.go:236-249; shards/game/internal/projectile/flight_system.go:126-141)
  - Verified: the doc says a new kind of target needs no predictor, glue or shot code (docs/research/favor-the-shooter.md:100-102)
  - Summarized: barriers are planned as targets that join targetKinds (docs/research/system-architecture-design.md:238-255)

<a id="f20"></a>

### F20. Other screens get rule bits, not content: no champion, most effects invisible

**not generic**, medium. From `content-5`.

- **Where:** `shards/game/internal/player/body_sync_event.go:64-73`, `shards/game/internal/player/body_sync_event.go:29-39`, `shards/game/internal/echo/echo_sync_event.go:20-31`, `CLAUDE.md:643-646`
- **What breaks:** BodyState has no Champion; only your own You.Champion does. With a second champion, other screens can't tell the heroes apart, so 'a new champion is JSON, never code' fails at the client. Other players' effects arrive only as folded Tags. Slow, haste, phase and dash have no tag of their own, so they are invisible on other screens. Each new visible state needs a new rule tag or a new field. There is no cue channel.
- **Measured:** msgpack bytes in a scratch copy: a per-entity {Kind, End} list costs 7 B empty, then about 24 B per active kind. A Champion field costs about 16 B per Body. That makes about 0.4 KB a tick per player at 3v3 with one kind each (22 KB/s at 60 Hz). For reference, the wire fixtures are 1058 B (body_sync) and 337 B (echo_sync).
- **Mature engines:** GAS GameplayCues carry sounds and particles. They replicate to clients as multicast events, and they are predicted [sourced: https://raw.githubusercontent.com/tranek/GASDocumentation/master/README.md]. Overwatch's Statescript automates the replication of ability state, so new heroes need little new code [sourced: https://gdcvault.com/play/1024041/Networking-Scripted-Weapons-and-Abilities]. Dota and League show every visible unit's hero and its buff effects to all players [from memory].
- **Fix:** Sync content identity, not only rule bits. BodyState gets Champion. BodyState and EchoState get their active effect kinds as {Kind, End}, filtered by netsync's seenBy so hidden effects stay hidden. ProjectileState already sends Type. The client maps champion, shot type and effect kind to visuals in its own content table, keyed by those IDs.
- **Rejected:**
  - *Add a tag per visible state (slowed, phased).* It fills the closed rule-tag list with cosmetic entries, the Deadlock problem the design doc warns about.
  - *Send full effect entries to everyone.* It leaks Source and exact numbers to enemies and costs more bytes than kinds.
  - *Let the client infer states from motion.* Phase, haste and slow can't be told apart from motion, and a champion can't be inferred at all.
- **Cost:**
  - Size: S: two event fields, the syncs, the client decoders and the wire fixtures on both sides
  - Risk: low: events only, about +15% on per-entity sync bytes
  - Purge: no: these are events, not components
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Today there is one champion and no effect visuals, and tags cover stunned and unstoppable, the states that change play. But the second champion and each visible effect need identity on other screens, and CLAUDE.md's 'never new code' claim fails at the client.
- **Evidence:**
  - Verified: BodyState has no Champion field (shards/game/internal/player/body_sync_event.go:64-73)
  - Verified: Only YouState carries Champion and effect entries (shards/game/internal/player/body_sync_event.go:36-38)
  - Verified: EchoState sends folded Tags only (shards/game/internal/echo/echo_sync_event.go:28)
  - Verified: Slow, dash and phase grant no tag of their own (shards/game/content/effect/tuning.json:3-6)
  - Verified: The client reads a champion only from your own You state (client/src/game/player/predictor.ts:99)

<a id="f21"></a>

### F21. Teleports will be drawn and judged along a streak

**won't scale**, medium. From `prediction-5`.

**Status (2026-11-07):** open, to land with the first teleport (`Row.Jump`). `AtView` moved to
`track/past`, and `Scene.Build` now boxes every kept row.

- **Where:** `shards/game/internal/projectile/flight/view.go:36-50`, `shards/game/internal/projectile/flight/view.go:74-82`, `client/src/engine/render/entity_views.ts:27-31`, `client/src/game/player/prediction.ts:31-34`
- **What breaks:** AtView always slides between the two rows around a view, and Reach boxes span every kept row. Nothing marks a jump. When blink, spawn teleports or Echo-Swap land, a target that jumped 10 u is drawn sliding across the gap, and a predicted shot can hit it in the gap where it never was. The server agrees, because both sides use AtView, so it is consistent but wrong. The Echo path slides the same way.
- **Measured:** Not probed: nothing teleports yet. From code: AtView lerps with no jump check (view.go:36-50); Reach.Add unions every kept row's box (view.go:74-82), so one jump makes a long box and extra candidates, which the pair cast then tests at the lerped pose. The client blend lerps too (entity_views.ts:27-31). A jump stays in the rows for KeptRows ticks (0.45 s at 20 Hz). Planned users: blink, spawn teleports, Echo-Swap (system-architecture-design.md:262, 328, 381; GDD.md:93).
- **Mature engines:** Source's lag compensation stops rewinding a player whose position jumped more than 64 units between records, treating it as a teleport [sourced: https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/player_lagcompensation.cpp]. Source clients skip interpolation for entities flagged EF_NOINTERP [from memory]. Unreal snaps instead of smoothing past a no-smooth distance [sourced: https://qiita.com/mintiatian/items/ab990c4cb7a53c63172c].
- **Fix:** Add a jump flag to Row, set by the system that moves a thing without walking (blink, swap, spawn). AtView holds the older row instead of sliding when the newer row jumped. Reach.Add boxes each run of rows between jumps on its own. The client blend and the Echo path read the same flag and snap. One flag, one rule, both sides. Land it with the first teleport.
- **Rejected:**
  - *Detect jumps by speed between rows.* Dashes are fast too (20 u/s); any threshold misreads dashes or short blinks, and shifts with tick rate.
  - *A fixed teleport distance, like Source's 64 units.* Same guess in other units; a short blink under it still streaks.
  - *Drop the rows before a jump.* Breaks views inside the rewind window: the shooter can't hit what they saw before the jump.
- **Cost:**
  - Size: S
  - Risk: low: nothing teleports yet, so no recording changes
  - Purge: yes: Row gains a field, so TargetHistory snapshots change shape
  - World-engine: no
- **Earlier review:** carried over from G5 (fix 16, still not done)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Nothing teleports today, so nothing breaks yet, and the flag costs a purge with no current user. But blink and Echo-Swap are planned, and the fix is small if it lands with the first one. Checked view.go, the client blend and the plan docs.
- **Evidence:**
  - Verified: AtView always slides between the rows around a view (shards/game/internal/projectile/flight/view.go:36-50)
  - Verified: Reach.Add boxes every kept row (shards/game/internal/projectile/flight/view.go:74-82)
  - Verified: the client blend slides with no jump check (client/src/engine/render/entity_views.ts:27-31)
  - Summarized: blink, spawn teleports and Echo-Swap are planned (docs/research/system-architecture-design.md:262,328,381; docs/GDD.md:93)
  - Verified: Source stops backtracking across a 64-unit jump (https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/player_lagcompensation.cpp)

<a id="f22"></a>

### F22. Stored entity IDs have no generation; a dead caster's ID names a new entity

**not generic**, medium. From `ecs-1`.

**Status (2026-11-07):** shots fixed, the rest open. `Cause.CasterSerial` and `Scene.Client`'s serial
check judge a shot whose caster's ID now names another entity at the present view. `team.Owner.Body`
and status `Entry.Source` still hold raw IDs; the engine fix stands.

- **Where:** `shards/game/internal/team/team.go:28-30`, `shards/game/internal/ability/component/ability_cast_event.go:9`, `shards/game/internal/effect/status/status.go:40,105`, `shards/game/internal/projectile/flight_system.go:95,109`, `world-engine@v0.22.0-beta/pkg/cardinal/internal/ecs/world_state.go:16,128-131`
- **What breaks:** Cardinal reuses a freed ID at once. team.Owner.Body, Projectile.Cause.Caster and status Entry.Source keep raw IDs across ticks. When the holder dies, its ID names the next new entity. Probe: a shot cast by Echo 5 kept Caster=5 after the Echo died; ID 5 was then a live shot. Harmless today: Bodies never die; only Bodies cast in play. Once a Body can go (a leave, a round rebuild), shots rewind to another player's view and credit them.
- **Measured:** TestProbeCasterIDAliases (copy): shot 7 kept Cause.Caster=5 after Echo 5 died; w.Entity(5) then held a live shot. 3 stored-ID fields; no reader compares a serial.
- **Mature engines:** Bevy's Entity is index plus generation, so a recycled index is told apart from an old handle [sourced: https://docs.rs/crate/bevy_ecs/0.9.1/source/src/entity/mod.rs]. Unity Entities pairs Index with Version, bumped on destroy [sourced: https://docs.unity.cn/Packages/com.unity.entities@1.1/api/Unity.Entities.Entity.Index.html]. Quantum's EntityRef holds Index and Version; Frame.Exists checks a ref stored in a component [sourced: https://doc-api.photonengine.com/en/quantum/v2/group___entity_api.html]. Unreal's FWeakObjectPtr keeps ObjectIndex plus ObjectSerialNumber [sourced: https://dev.epicgames.com/documentation/unreal-engine/API/Runtime/CoreUObject/FWeakObjectPtr].
- **Fix:** Make Cardinal IDs generational: EntityID = index + generation, bumped on destroy and saved in snapshots; w.Entity on a stale ID reports not found. Then every stored ref is safe by construction, and ref.Spawn/ref.Next retire (syncs send the full ID). This is a world-engine change; echo_image then deletes its serial plumbing.
- **Rejected:**
  - *Game-level Ref{ID, Serial} on every stored reference (design rule 10).* Every reader in every feature must remember the serial check. Fix 8's Spawn check already voided a test silently (F5). One engine check covers all.
  - *Never destroy Bodies; reuse them across rounds.* Echoes and shots still die and still sit in Caster and Entry.Source. It hides the bug for one kind only.
  - *Re-check the archetype on read, as viewOf does with InputQueue.* A new entity of the same kind passes the check. It narrows the window; it does not close it.
- **Cost:**
  - Size: M in world-engine (ID type, free list, snapshot field, lookups); S in echo_image (drop ref plumbing)
  - Risk: medium: every Cardinal lookup and the snapshot format change
  - Purge: likely yes: snapshots store bare uint32 IDs; restore could default generation 0 to avoid it
  - World-engine: yes: generational EntityID in pkg/cardinal/internal/ecs (world_state.go)
- **Earlier review:** new: extends C1/GV1. Fix 8 closed those for syncs; stored refs were outside it.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: No wrong result today: Bodies never die, only Bodies cast in play, and the one effect (dash) targets its own caster. Rounds may teleport Bodies, not rebuild them. But Echo casters already die in tests, and Cause.Caster already breaks rule 10.
- **Evidence:**
  - Verified: EntityID is a bare uint32 (world-engine@v0.22.0-beta/pkg/cardinal/internal/ecs/world_state.go:16)
  - Verified: Freed IDs go to a min-heap; the smallest is reused next (world-engine@v0.22.0-beta/pkg/cardinal/internal/ecs/world_state.go:128-131)
  - Verified: team.Owner stores a raw Body EntityID (shards/game/internal/team/team.go:28-30)
  - Verified: Cause.Caster is a raw EntityID carried in every Projectile (shards/game/internal/ability/component/ability_cast_event.go:9)
  - Verified: Entry.Source is a raw uint32 set to the caster; PerSource stacking compares it (shards/game/internal/effect/status/status.go:40,105; shards/game/internal/ability/cast/cast.go:104-105)
  - Verified: viewOf resolves the caster by raw ID each tick; damage credit uses the shot's Owner.Body (shards/game/internal/projectile/flight_system.go:95,108-116)
  - Verified: Design rule 10 and the Ref section ask for Ref{ID, Serial} on stored refs to things that can die (docs/research/system-architecture-design.md:40,271)
  - Verified: Probe: Caster=5 names a live shot after Echo 5 died (copy only: shards/game/zz_probe_bench_test.go TestProbeCasterIDAliases)

<a id="f23"></a>

### F23. SimVersion ships tests in the shard binary and knows only the rate tag; 60 Hz can't deploy

**won't scale**, medium. From `netcode-5, tests-9`.

- **Where:** `sim.go:18`, `sim.go:24`, `sim.go:49`, `sim_test.go:86`, `shards/game/main.go:23`, `shards/game/main.go:125`, `client/scripts/predictor.mjs:21`, `world-engine@v0.22.0-beta/cli/pkg/docker/cardinal.Dockerfile:10`, `world-engine@v0.22.0-beta/cli/pkg/docker/service/constants.go:36`, `world-engine@v0.22.0-beta/pkg/cardinal/world.go:69`, `sim.go:18-19`, `sim.go:31-36`
- **What breaks:**
  - The embed takes all of shards/, so tests and testdata ship in the shard binary: +2.17 MB, growing with each recording. Only 240 KB of it is hashed.
  - The rate is a parameter, because the root package can't import internal/. Any other build tag goes unhashed.
  - `world build` can't pass tags, and main.go's TickRate overrides world.toml's tick_rate. So a 60 Hz shard can't be deployed.
  - runtime.Version() is in the hash. If the local Go differs from the CLI's golang:1.27.1 builder, every deployed shard refuses the client.
  - A local replace would drop world-engine from the hash.
- **Measured:** In the copy, the shard binary is 64,418,109 B with the embed and 62,243,157 B embedding only go.mod: +2,174,952 B (3.4%). The embedded shards/ is 2.11 MB, of which testdata is 1.78 MB and tests 146 KB; only 240 KB is hashed. SimVersion() takes 0.35–0.37 ms a call (once at boot, and once per test world). go list EmbedFiles: 120 files, 2,163,582 B, of which tests and testdata are 1,924,569 B.
- **Mature engines:** Unreal compares a network version checksum at login and turns away a mismatched client [from memory]. Source checks map CRCs and, on pure servers, file hashes [from memory]. Both take the version from the build itself, not from a copy of the source shipped inside the server binary [from memory]. Builds usually stamp versions with -ldflags -X or debug.ReadBuildInfo [from memory].
- **Fix:**
  - Hash and embed exactly what both builds compile, and read the rest from the build itself.
  - List the source dirs in go:embed (go.mod, go.sum, pkg, shards/game/main.go, content, internal, predictor), so tests and testdata stay out. TestSimVersionCoversWhatTheSimIsBuiltFrom already fails if a built file drops out.
  - Read the build's tags from debug.ReadBuildInfo and drop the rate parameter.
  - Make sim_test fail on any local replace. Pin the toolchain in go.mod to the CLI's builder image.
  - Ask world-engine for a build-tags field in world.toml, so 60 Hz can deploy.
- **Rejected:**
  - *Stamp the VCS revision.* Docker builds get no .git (defaultBuildIgnores), and a dirty tree shares its commit's revision.
  - *Serve predictor.wasm from the shard.* Cardinal's Dockerfile runs a single go build, so the shard can't build or serve the wasm today.
  - *Drop the Go release from the hash.* Another compiler can change float code, and the FMA scan only checks the local toolchain.
  - *-ldflags -X stamp.* The world CLI's Docker build sets the flags; needs a world-engine change.
  - *Leave as is.* The production binary grows with every recording.
- **Cost:**
  - Size: S
  - Risk: low: the go list test guards the embed.
  - Purge: no
  - World-engine: yes, for 60 Hz deploys only: a build-tags field in world.toml passed to the Dockerfile. The rest is in echo_image.
- **Earlier review:** new. Fix 7's SimVersion (S1, G9) works; these are its gaps.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check (`netcode-5`): Today the local Go is 1.27.1, there is no replace and no other tag, so nothing mismatches. The 2.2 MB is 3.4% of the binary. Medium only because 60 Hz can't deploy and the traps are silent.
  - Reviewer's check (`tests-9`): 1.9 MB is small for a Go server binary, and players never get it. So low. But it grows with every recording, and the fix is one line.
- **Evidence:**
  - Verified: The embed takes all of shards/, tests and testdata included (sim.go:18)
  - Verified: The hash skips tests and testdata (sim.go:27-45)
  - Verified: The rate is a parameter that each caller passes (sim.go:21-24; shards/game/predictor/version/main.go:13; shards/game/main.go:125)
  - Verified: The version string includes runtime.Version() (sim.go:49)
  - Verified: The shard's Docker build uses golang:1.27.1-bookworm and runs go build without tags (world-engine@v0.22.0-beta/cli/pkg/docker/cardinal.Dockerfile:1-10; world-engine@v0.22.0-beta/cli/pkg/docker/service/constants.go:36)
  - Verified: The TickRate set in code overrides world.toml's tick_rate env (world-engine@v0.22.0-beta/pkg/cardinal/world.go:68-69; world-engine@v0.22.0-beta/pkg/cardinal/config.go:61-63; shards/game/main.go:23)
  - Verified: The coverage test checks only main-module packages (sim_test.go:86)
  - Verified: go.mod has no replace, and go.sum pins world-engine v0.22.0-beta (go.mod:5-8; go.sum:46)
  - Verified: sim.go embeds go.mod go.sum pkg shards (sim.go:18-19)
  - Verified: The hash skips testdata and _test.go files (sim.go:31-36)
  - Verified: The shard sends this package's SimVersion (shards/game/main.go:125)

<a id="f24"></a>

### F24. Each input frame is its own HTTP POST that resends up to 0.4 s

**not standard**, medium. From `netcode-2`.

- **Where:** `client/src/engine/net/shard_connection.ts:60`, `client/src/game/player/input_stream.ts:86`, `client/src/game/player/input_stream.ts:7`, `client/src/main.ts:95`, `go-ecs/pkg/sdk-ts/src/shard/shard-client-factory.ts:61`, `world-engine@v0.22.0-beta/proto/worldengine/cardinal/v1/cardinal.proto:15`, `world-engine@v0.22.0-beta/pkg/cardinal/service.go:296`
- **What breaks:** Every new frame is its own unary POST: 60 a second per player at 60 Hz, and 9 at once after a 0.15 s hitch. Each repeats all unacked frames, up to 0.4 s, at about 80 B a frame. At 60 Hz that is 0.29–0.63 Mbit/s up, and 87–94% of it was already sent. Over HTTP/1.1, as through the Vite dev proxy, 5 free connections saturate above 83 ms RTT at 60 Hz. One lost TCP packet stalls inputs and events together.
- **Measured:** Client encoder (the SDK's @msgpack/msgpack plus the SendCommandRequest proto, in node): 1 frame 133 B, 4 frames 371 B, 10 frames 845 B, 16 frames 1321 B, 24 frames 1953 B (about 80 B a frame). Real InputStream in a simulation: 3/4/6 frames per Input at 20 Hz and 7/9/16 at 60 Hz, for RTT 50/80/200 ms. Up: 47–85 kbit/s at 20 Hz, 293–634 kbit/s at 60 Hz, and 937 kbit/s at the 24-frame cap. HTTP headers are not counted. Server path per POST, in-process (otelconnect, protovalidate, auth, msgpack decode): 21 µs for 1–10 frames, 33 µs for 24.
- **Mature engines:** Overwatch bundles every input since the last acknowledged state into one UDP packet [sourced: https://edgegap.com/blog/game-backend-deep-dive-overwatch-2016-netcode-architecture-rollback]. Source sends command packets at cl_cmdrate, with several commands in each [sourced: https://developer.valvesoftware.com/wiki/Source_Multiplayer_Networking]. Resending pays off on UDP, where packets get dropped. On an ordered stream, each frame is sent once. Browser games use one WebSocket or WebTransport connection, not a request per input [from memory].
- **Fix:** Build world-engine item 18: one persistent, ordered stream per player (a WebSocket in Cardinal, the gateway and the SDK) that carries commands in order. The client then sends each frame once, as one message per animation frame, and resends unacked frames only after a reconnect. Input{Session, Frames} and InputSystem stay as they are; the design doesn't block this.
- **Rejected:**
  - *Shrink ResendSeconds on today's HTTP.* It keeps a POST per frame and its cost. Unary requests can fail or race through the gateway, so frames would drop.
  - *Connect client streaming from the browser.* connect-web throws 'The fetch API does not support streaming request bodies' (connect-transport.js:147).
  - *WebTransport datagrams now.* It is the only cure for TCP head-of-line stalls, but it needs HTTP/3 at the gateway and wider browser support [from memory]. The same messages can move there later.
  - *HTTP/2 everywhere.* Removes only the 6-connection cap. The per-request cost, the resending and TCP head-of-line blocking stay.
- **Cost:**
  - Size: L
  - Risk: medium: a new transport, and reconnect paths need tests
  - Purge: no
  - World-engine: yes: a WebSocket stream for commands (and events) in Cardinal, the gateway and sdk-ts
- **Earlier review:** carried over from T8 (still open)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Over HTTP/2 the 6-connection cap is gone, and local RTT is about 1 ms, so dev is fine. Resending is right for unary requests that can race or fail. Medium holds: upstream is bounded and nothing breaks today.
- **Evidence:**
  - Verified: Each Input is one fire-and-forget unary sendCommand (client/src/engine/net/shard_connection.ts:60-65; go-ecs/pkg/sdk-ts/src/shard/shard-client-factory.ts:61-70)
  - Verified: Cardinal offers only unary commands; only events stream (world-engine@v0.22.0-beta/proto/worldengine/cardinal/v1/cardinal.proto:13-28)
  - Verified: One Input per new frame, each carrying min(0.4 s, cap) of unacked frames (client/src/game/player/input_stream.ts:7, 81-87; client/src/main.ts:95)
  - Verified: connect-web can't stream request bodies (go-ecs/pkg/sdk-ts/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js:147)
  - Verified: The dev client goes through Vite's /world proxy (client/vite.config.ts:20-25; client/src/main.ts:42)
  - Summarized: Chrome allows 6 HTTP/1.1 connections per host, and the event stream holds one (browser behavior)

<a id="f25"></a>

### F25. No production signal for coasts, dropped frames, queue depth or corrections

**not standard**, medium. From `netcode-6`.

- **Where:** `shards/game/internal/player/input_system.go:84`, `shards/game/internal/player/input_system.go:96`, `shards/game/internal/player/input_system.go:49`, `client/src/game/ui/hud.ts:67`, `world-engine@v0.22.0-beta/pkg/cardinal/metrics.go:19`, `world-engine@v0.22.0-beta/pkg/cardinal/world.go:520`
- **What breaks:** The server trims frames past the cap and coasts on empty ticks without a log line or a count. An oversized Input logs one warning per command, so a bad client floods the logs. Corrections and frames in flight show only on the player's own HUD. In production nobody can see why players get corrections, which links or tick rates hurt, or what each event costs in bytes.
- **Measured:** grep finds no otel, metric or meter use under shards/. The game logs only claims, oversized Inputs, no free Body and unknown kinds. Cardinal v0.22.0-beta exports cardinal.tick.duration, tick.overruns, tick.commands and events.sent when OTEL_EXPORTER_OTLP_METRICS_ENDPOINT is set. Per-system spans exist only when the tick span is recording. Each event publish gets a span with its name, recipient and subscriber count, but no byte size.
- **Mature engines:** Unreal's Replication Graph reports per-connection saturation (bWasConnectionSaturated) [sourced: https://dev.epicgames.com/documentation/en-us/unreal-engine/API/Plugins/ReplicationGraph/UReplicationGraph]. Overwatch's server notices input starvation per client and acts on it [sourced: https://edgegap.com/blog/game-backend-deep-dive-overwatch-2016-netcode-architecture-rollback]. Source's net_graph shows loss and choke live [from memory].
- **Fix:** Count input health per Body on the server and export it through the meter Cardinal already installs (otel.Meter). Counters: coasts, frames dropped by the cap, Inputs dropped as oversized, and views floored, plus a histogram of queue depth. Keep the counts in InputQueue so YouState can show them on the HUD too. Replace the per-command warning with a count. Ask world-engine to record event bytes on events.sent.
- **Rejected:**
  - *Log each coast or drop.* Up to 60 lines a second per bad link. Logs cost more than counters and can't be graphed.
  - *Keep it on the client HUD.* It never leaves the player's screen, so ops can't see it.
  - *Per-player metric labels.* Unbounded cardinality. Per-shard counters plus per-Body counts in state are enough.
- **Cost:**
  - Size: S
  - Risk: low
  - Purge: no: new InputQueue fields decode as 0
  - World-engine: optional: a byte count on cardinal.events.sent
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Tick duration and overrun metrics exist once OTel is on. But per-system cost needs traces, and there is no input-health signal at all. It hides problems rather than causing them, so medium, not high.
- **Evidence:**
  - Verified: The cap trim and the coast are silent (shards/game/internal/player/input_system.go:84-86, 96-98)
  - Verified: Each oversized Input logs a warning (shards/game/internal/player/input_system.go:47-52)
  - Verified: Corrections and inputs in flight appear only on the local HUD (client/src/game/ui/hud.ts:67-68)
  - Verified: Cardinal's metrics: tick duration, overruns, commands, events sent (world-engine@v0.22.0-beta/pkg/cardinal/metrics.go:19-24)
  - Verified: Per-system spans exist only when the tick span is recording (world-engine@v0.22.0-beta/pkg/cardinal/world.go:519-526)
  - Verified: The publish span carries no byte size (world-engine@v0.22.0-beta/pkg/cardinal/service.go:629-630)

<a id="f26"></a>

### F26. Land fails silently at the map edge and on unchecked deep walls

**duct tape**, medium. From `physics-client-3`.

- **Where:** `shards/game/internal/player/motion/motion.go:47-50`, `shards/game/internal/physics/mover/land.go:10-12,49-57,71-78`, `docs/research/character-foundation.md:93-97`
- **What breaks:** motion.Step drops Land's ok, so a failed landing is silent. Landing goes wrong when a wall is deeper than 12 m or a sealed pocket is nearest (the Body is trapped); no check exists, though the docs say map tests check both. It fails when a center is inside a solid Bounds shape (every ray starts inside it, which box2d counts as a hit), and a Body outside a chain-loop edge is never Inside. Failed ticks re-run the search.
- **Measured:** Land per call (re-run): 14 µs and 212 allocations typical; 0.36-0.42 ms (up to 0.75 ms in earlier runs), 5,379 allocations and 289 KB with no spot. Six stuck Bodies: 2.1-4.5 ms a tick (4-9% of a 20 Hz tick, 13-27% at 60 Hz). Probes: center 0.3 or 0.7 m into a 2 m solid Bounds box gives ok=false, center unchanged. Chain loop, either winding: Body at x=12 has Inside=false; walking in stops at x=10.495.
- **Mature engines:** Quake 3's PM_CorrectAllSolid tries a 3x3x3 grid of one-unit nudges and otherwise gives up, so play relies on maps and teleports never leaving a player in solid [sourced: https://raw.githubusercontent.com/id-Software/Quake-III-Arena/master/code/game/bg_pmove.c]. Source's vbsp refuses to finish a map that leaks [from memory]. Aimed blinks trace toward the target and stop at walls [from memory].
- **Fix:** Keep the grid search; make its preconditions checked facts. The arena loader flood-fills clear cells from the spawns and rejects any unreached clear cell (a sealed pocket) and any blocked cell over LandReach from a reached one. Every teleport casts its path against Bounds and stops at the edge, as the mover does. motion.Step logs ok=false as a bug. Add the map tests the docs promise.
- **Rejected:**
  - *Minimum-translation push-out, like Unreal's ResolvePenetration.* Box2D's mover has no deep-overlap solve ('todo handle deep overlap'); a nearest-face exit from a deep wall can still land in a sealed pocket.
  - *Bigger LandReach or a finer grid.* Moves the limit without checking it; doubling reach quadruples cells, and a no-spot search already costs up to 0.75 ms.
  - *Snap back to the Body's last clear position.* A second, stateful rule beside Land: stored per Body, saved in snapshots, and wrong after a teleport whose start was also blocked.
- **Cost:**
  - Size: M: loader validation (~100 lines plus map tests), a Bounds cast in each teleport hook, one log line.
  - Risk: Low: validation runs once at load; a strict rule may reject a map a designer wants, which is the point.
  - Purge: No: no stored state changes.
  - World-engine: No.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: No shipped content reaches these cases: phase passes only Wall, the mover stops at Bounds, and no map or blink exists. If the arena and blink designs already include these checks, this is a doc gap, not a code gap.
- **Evidence:**
  - Verified: Land's ok is discarded. (shards/game/internal/player/motion/motion.go:49)
  - Verified: Land returns the old center and false when no grid spot qualifies; crosses() is a ray cast against Bounds. (shards/game/internal/physics/mover/land.go:49-57,71-78)
  - Verified: A polygon ray starting inside reports an initial-overlap hit. (world-engine@v0.22.0-beta/pkg/box2d/geometry.go:910-916)
  - Verified: The wall-depth limit is only a comment. (shards/game/internal/physics/mover/land.go:10-12)
  - Verified: Docs say Land covers phase, blinks and restore lag, and map tests check depth and pockets; no map, blink or map test exists (walls appear only in tests). (docs/research/character-foundation.md:93-97; shards/game/main_test.go:1710-1729)
  - Verified: Phase can pass only Wall, never Bounds. (shards/game/internal/effect/status/tuning.go:101)
  - Verified: Box2D's mover leaves deep overlap unhandled. (world-engine@v0.22.0-beta/pkg/box2d/world_query.go:641)

<a id="f27"></a>

### F27. Mutation checks live in another session's /tmp and edit the real tree

**not standard**, medium. From `tests-6`.

- **Where:** `/tmp/claude-1000/-home-doglightning-world-engine/6a855a41-562e-4a86-b097-fde3ba6e9d68/scratchpad/mut12/run.py:2`, `docs/research/favor-the-shooter.md:234`, `docs/research/system-architecture-design.md:572`
- **What breaks:** The 140 hand-written mutants are Python scripts in another session's scratchpad. Each edits a file in the real working tree, runs the tests, then copies the file back. A kill mid-run leaves a planted bug among 89 uncommitted files. No one else can rerun them, /tmp may be cleared, and 11 of 140 no longer apply after a day. The docs cite them as each rule's proof.
- **Measured:** 14 runners, all with ROOT = /home/doglightning/echo_image. Dry run on today's code: 129 of 140 anchors apply. Re-run in a second copy, never the real tree: 127 caught, 1 missed (G6, known unobservable), 10 not applied, 2 not run (old file names). A Go mutant costs at least 6 s; 5.7 s of it is the FMA scan.
- **Mature engines:** StrykerJS mutates a sandbox copy by default; in-place is opt-in, with backups [sourced: https://stryker-mutator.io/docs/stryker-js/configuration/]. Its incremental mode reruns only changed mutants [sourced: https://stryker-mutator.io/docs/stryker-js/incremental/]. Gremlins tests only covered mutants and can gate CI [sourced: https://gremlins.dev/latest/]. Go's -overlay flag builds with replaced files without touching the tree [from memory].
- **Fix:** Commit the mutant list as data (file, old text, new text, check that must fail) with a small runner that applies each in a temporary copy, never the working tree. A mutant whose old text no longer matches fails the run, so the list can't rot unseen. Run it on demand and whenever a rule changes.
- **Rejected:**
  - *Keep the /tmp scripts.* They rot (11 of 140 in a day), vanish with /tmp, and edit the uncommitted tree in place.
  - *gremlins or go-mutesting alone.* Generic operators, not the project's rules; each Go mutant pays 6 s or more, so a full run takes hours.
  - *Stryker alone.* Covers only the TypeScript half; most rules live in Go.
- **Cost:**
  - Size: small: one data file and a ~60-line runner
  - Risk: low
  - Purge: no
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: The runners back up and cmp-check each file after restoring, and 127 of 128 applicable mutants still fail a test today. So the evidence is real. The risk is an interrupted run and a lost list: bounded, medium.
- **Evidence:**
  - Verified: Every runner sets ROOT to the real tree and edits files in place, then restores and cmp-checks (earlier scratchpad mut*/run.py:2 (14 files))
  - Verified: The docs cite mutation checks as each new rule's proof (docs/research/favor-the-shooter.md:234)
  - Verified: Built steps 'ran the full tests plus mutation checks' (docs/research/system-architecture-design.md:572)
  - Verified: 127 caught, 1 missed, 10 not applied in a re-run on a copy (probe rerun_mutants.py (scratch copy))

<a id="f28"></a>

### F28. Phase-landing test passes by sitting exactly on two ties

**duct tape**, low. From `physics-client-6`.

- **Where:** `shards/game/main_test.go:1857-1877`
- **What breaks:** The Body stops exactly at mid-wall, where both sides are 1.25 m away, so grid order picks −x; the comment calls it 'the side she came from'. The landed edge sits exactly on the 0.25 m bound, and at 60 Hz rounding puts it 1.78e-15 m past, so a 1e-9 slack was added. A Body that came from +x also lands on −x, and the test can't tell.
- **Measured:** Probe, re-run: 20 Hz pre-land center −3 exactly, landed edge −3.75, exactly the bound. 60 Hz center −3.0000000000000018, edge 1.78e-15 m past the bound. Centers at mid-wall ±1e-9 all land at x=0.75 (−x); a Body from +x at mid-wall also lands −x.
- **Mature engines:** Box2D v3 checks determinism by hashing exact end transforms across platforms, not by ranges with slack [sourced: https://box2d.org/posts/2024/08/determinism/]. Unit tests usually keep inputs off tie points unless the tie rule itself is under test [from memory].
- **Fix:** Stop a quarter of the way in (0.25 m, reachable at 20 and 60 Hz), read the center before the phase ends, and assert the landed X equals that center minus 1.0, the nearest clear cell, exactly. Fix the comment: Land takes the nearest clear grid cell, ties in grid order, with no notion of direction. Drop the 1e-9 slack.
- **Rejected:**
  - *Keep the slack and widen the window.* Hides which side and which cell Land chose, so a tie-order or grid change passes unnoticed.
  - *Make Land prefer the side the Body came from.* Adds motion history to a pure function and a rule nobody designed; nearest cell is the documented rule.
- **Cost:**
  - Size: XS: one test.
  - Risk: None.
  - Purge: No.
  - World-engine: No.
- **Earlier review:** fix X6 is duct tape: its 1e-9 slack lets the phase-landing test pass at 60 Hz instead of asserting the exact cell
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: The test does prove the main point: a phase ending in a wall lands clear of it. Its slack is 1e-9 m, far below anything visible, and it still fails if Land lands on the far side.
- **Evidence:**
  - Verified: The test moves to mid-wall and checks the edge in [face−0.25−1e-9, face). (shards/game/main_test.go:1862,1874)
  - Verified: The comment claims 'the side she came from' and blames 1/60 s rounding. (shards/game/main_test.go:1871-1872)
  - Verified: The grid is sorted by i²+j², then i, then j, so −x wins ties. (shards/game/internal/physics/mover/land.go:17-37)
  - Verified: The slack came with X6's 60 Hz runs. (earlier review fix-progress.md:132)
  - Verified: Probe outputs above. (audit-copy probes TestProbeLandingSlack and TestProbeLandSide, re-run this session)

<a id="f29"></a>

### F29. Mover's 1 cm stop rule drops a tick of slide on first contact

**not standard**, low. From `physics-client-4`.

- **Where:** `shards/game/internal/physics/mover/mover.go:16`, `shards/game/internal/physics/mover/mover.go:37-50`
- **What breaks:** If a Body starts a tick within about 2 mm of a wall it isn't touching, the first pass finds no plane, its cast stops under 1 cm, and the loop ends. That tick's slide is lost: 0.17 m at 20 Hz, 0.05 m at 60 Hz, so the rates end 0.118 m apart. That breaks 'same game at any rate'. Prediction still matches, since both sides share the code.
- **Measured:** Probe, re-run (45° push for 1 s, wall face at x=1.5): gap 2 mm gives 20 Hz y=3.3658, 60 Hz y=3.4836, exact 3.5355; gaps 5-50 mm are exact at both rates. First pass at 2 mm: planes=0, fraction 0.0396, step 0.0099 m; at 4 mm the step is 0.0127 m and nothing is lost. With the proposed break, both rates were exact and all Go tests passed at 20 and 60 Hz.
- **Mature engines:** Quake 3's PM_SlideMove runs up to four bumps and ends early only when a trace covers the whole move [sourced: https://raw.githubusercontent.com/id-Software/Quake-III-Arena/master/code/game/bg_slidemove.c]. Source's TryPlayerMove does the same [sourced: https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/game/shared/gamemovement.cpp]. Box2D v3's mover sample, which this ports, has the same 1 cm break [from memory]; its docs describe the collide-solve-cast loop [sourced: https://box2d.org/documentation/md_character.html].
- **Fix:** Break only when the step is under 1 cm and the pass either had planes (a real corner) or its cast went the full way (nothing left to move). A pass that met a new wall with no plane yet continues, collects it and slides. Add a 2 mm-gap case to the existing wall tests, which run at both rates.
- **Rejected:**
  - *Lower minStep.* Shrinks the window but never closes it: any threshold above zero loses the slide when the first cast stops just short of it.
  - *Always run all 5 passes.* Up to 5x the cost on open ground, where one pass suffices, to fix a case one extra pass covers.
- **Cost:**
  - Size: XS: one condition in mover.go, one test case.
  - Risk: Low: probed with all Go tests passing at both rates; prediction recordings may need regenerating if a recorded path crosses the window.
  - Purge: No: no stored format changes; ship server and predictor together (simVersion).
  - World-engine: No.
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: The window is narrow: a start 0-2 mm from a wall, pushing within about 60° of its normal. The loss is one tick of slide, identical in prediction, so players may never notice. It is a faithful port of Box2D's own sample.
- **Evidence:**
  - Verified: minStep is 1 cm: a pass moving less ends the move. (shards/game/internal/physics/mover/mover.go:16)
  - Verified: Loop: collide, sort, solve, cast, break on a small step; velocity clipped by the last planes. (shards/game/internal/physics/mover/mover.go:37-50)
  - Verified: Rate rule: a 60 Hz build is the same game as 20 Hz, just smoother. (CLAUDE.md:230-233)
  - Verified: Probe numbers above. (audit-copy probes TestProbeMinStepContact and TestProbeMinStepPasses, re-run this session)

<a id="f30"></a>

### F30. A shot type's payload rule without relations hits your own Echo

**not standard**, low. From `content-6`.

- **Where:** `shards/game/internal/projectile/flight/tuning.go:88-99`, `shards/game/internal/projectile/flight/tuning.go:104-112`, `shards/game/content/projectile/tuning.json:2-7`
- **What breaks:** A shot type's own meets run before the defaults. So for anything its own rule matches, the default 'own and ally pass' never runs. A rule like {kinds: [echo], do: stop, payload: true} with no relations damages your own Echo and your allies' Echoes. In the probe, the shooter's own Echo went from 100 to 75 HP. With 20+ shot types a year, this is an easy slip, and nothing checks for it.
- **Measured:** Probe in a scratch copy: alice walked 5 m from her Echo and fired back across it. With the bolt's default rules, her Echo stayed at 100. With own meets [{kinds: [echo], do: stop, payload: true}], it dropped to 75.
- **Mature engines:** Dota abilities declare which team they target in their data (AbilityUnitTargetTeam), so friendly fire is always written down [from memory]. GAS applies an effect only to the targets that the ability's target data picks [from memory].
- **Fix:** Reject at load any payload rule that names no relations. Add the check to validMeets, and to area rules once they can damage other things. Friendly fire must then be written out, for example relations [enemy], or [ally] for a heal shot. That is one check in validMeets plus a test.
- **Rejected:**
  - *Put the own/ally pass above every shot type's rules.* That blocks planned shots that should hit allies, like an Echo-Medic heal shot.
  - *Write a test per shot type.* Content added later without a test slips through. A load check covers every row.
- **Cost:**
  - Size: XS: one check in validMeets plus a test
  - Risk: low
  - Purge: no
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Order as priority is the design's choice, and a shot that hits allies may be wanted. The check only makes the author write that down. The probe shows how silently it goes wrong without one.
- **Evidence:**
  - Verified: A shot type's own rules are checked before the defaults (shards/game/internal/projectile/flight/tuning.go:104-112)
  - Verified: validMeets checks do, pass with a payload, and kinds, but not relations (shards/game/internal/projectile/flight/tuning.go:88-99)

<a id="f31"></a>

### F31. Equip-before-Cast claim-tick ordering is documented but no test pins it

**not standard**, low. From `ecs-6`.

- **Where:** `CLAUDE.md:456-457`, `shards/game/main.go:74-75`, `shards/game/main_test.go:473-485`
- **What breaks:** CLAUDE.md says a Body can cast the tick it is claimed because Equip runs before Cast. Swapping the two (mutation M1) passes every test, because the test claim helper sends only move(0,0). A reorder would silently drop a player's first fire on joining, and nothing would flag it.
- **Measured:** 17 order mutations in the copy: 13 fail tests. 4 pass: M1 Cast before Equip (the documented edge), M12 Cast after Movement, M15 Flight after Area, M18 Echo spawn after hooks. The last three commute (no shared write, or a sorted receiver).
- **Mature engines:** Bevy orders systems with explicit before/after constraints and can report system pairs with conflicting access and no set order; the error is opt-in [sourced: https://doc.qu1x.dev/bevy_trackball/bevy_ecs/schedule/struct.ScheduleBuildSettings.html]. Cardinal has only registration order, so a test per documented edge is the only guard.
- **Fix:** In main_test.go, add the fire button to the claim frame in an existing fire test and require the shot that same tick. Then M1 fails like every other documented edge.
- **Rejected:**
  - *Delete the claim from CLAUDE.md.* The behavior is real: players fire on join. Dropping the line hides a lost first input.
  - *Fold Equip into CastSystem.* Mixes setup into the gate; Equip moves to round start anyway (F1).
- **Cost:**
  - Size: S: a few test lines
  - Risk: low
  - Purge: no
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: One lost click on joining is minor, and F1 may move Equip to round start, which changes this edge anyway. Still, CLAUDE.md states it as a guarantee, and every other documented edge has a test.
- **Evidence:**
  - Verified: CLAUDE.md promises a cast on the claim tick (CLAUDE.md:456-457)
  - Verified: Equip is registered before Cast (shards/game/main.go:74-75)
  - Verified: The claim helper sends move(0,0) only (shards/game/main_test.go:473-485)
  - Verified: M1 passes all tests (copy only: scratchpad mutate_order.py M1)

<a id="f32"></a>

### F32. Correction snap distance is a TS constant tied to no speed

**not generic**, low. From `prediction-6`.

- **Where:** `client/src/game/player/prediction.ts:9-10`, `client/src/game/player/prediction.ts:16`, `client/src/game/player/prediction.ts:135`
- **What breaks:** SnapDistance 3 u and FadeSeconds 0.1 s are hard-coded in TS. 3 u is 0.6 s of walking but 0.15 s of dashing, so a new champion speed or knockback changes what counts as a big error with no code noticing. A 2.9 u correction slides for about 0.3 s; a 3.1 u one jumps. The fade is exp(−tick/0.1) per step, so it is rate-safe.
- **Measured:** From the gap analysis, not a browser probe: effects others cause land about RTT + 0.05 s late, giving 0.75-1.25 u corrections walking (5 u/s) and 3-5 u on a dash cut short (20 u/s); a late frame gives up to one walk step (0.25 u at 20 Hz). So the 3 u line splits dash corrections: some fade, some snap.
- **Mature engines:** Unreal's character movement smooths corrections by mode (exponential, linear or none) [sourced: https://docs.unrealengine.com/4.27/API/Runtime/Engine/Engine/ENetworkSmoothingMode/index.html] and snaps past a no-smooth distance [sourced: https://qiita.com/mintiatian/items/ab990c4cb7a53c63172c]. Defaults are about 0.1 s, 256 cm and 384 cm, set per movement component, not global [from memory].
- **Fix:** Move both numbers into each champion's movement tuning, which the wasm predictor already embeds, and read them through kit(). State the snap line in seconds of that champion's top speed (for example 0.3 s), so a faster champion gets a longer line with no TS change.
- **Rejected:**
  - *Keep the constant and tune by hand.* Each new speed or knockback needs a TS edit someone must remember.
  - *Never snap, always fade.* Big errors (respawn, swap) would slide across the map.
- **Cost:**
  - Size: S
  - Risk: low
  - Purge: no: content and client only
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: One champion with one dash exists, and 3 u is a sane line for it. This is cleanup before content grows, not a bug.
- **Evidence:**
  - Verified: FadeSeconds 0.1 and SnapDistance 3 are hard-coded (client/src/game/player/prediction.ts:9-10)
  - Verified: near compares Math.hypot against SnapDistance (client/src/game/player/prediction.ts:16)
  - Verified: the fade factor is exp(−tickSeconds/FadeSeconds) per step (client/src/game/player/prediction.ts:135)
  - Summarized: move 5 u/s; dash 6 u in 0.3 s (shards/game/content (champion tuning))

<a id="f33"></a>

### F33. Timeline caps states by count (64), not by seconds

**not standard**, low. From `physics-client-5`.

- **Where:** `client/src/engine/sync/timeline.ts:3`, `client/src/engine/sync/timeline.ts:58`
- **What breaks:** Nothing at 20 or 60 Hz: 64 states hold 3.2 s and 1.07 s, and drawing reaches at most 0.35 s back. It breaks the project rule that buffer sizes derive from seconds, and fails silently above about 180 Hz, where 64 states no longer cover MaxDrift plus InterpolationDelay and sample() finds no pair.
- **Measured:** 64 states = 3.2 s at 20 Hz, 1.07 s at 60 Hz. Drawing needs about 0.35 s x rate + 2 states, so the cap fails from about 178 Hz. Memory is trivial at 64 states x 32 entities.
- **Mature engines:** Gaffer on Games sizes the interpolation buffer as a delay in time, not a packet count [sourced: https://gafferongames.com/post/snapshot_interpolation/]. Source sets its interpolation window in seconds (cl_interp, or cl_interp_ratio over the update rate) [sourced: https://developer.valvesoftware.com/wiki/Interpolation].
- **Fix:** Prune by time, not count: on add, drop states older than the newest tick minus (MaxDrift + InterpolationDelay) in ticks, plus the one-tick pair, using TickSeconds. MaxStates goes away and the buffer holds the same 0.35 s at any rate, hidden tab included.
- **Rejected:**
  - *Raise MaxStates to 256.* Still a tick count: it moves the break to about 700 Hz and keeps the rule broken.
  - *Prune in renderTick below the drawn pair.* Hidden tabs stop animation frames, so states would pile up without bound while hidden.
- **Cost:**
  - Size: XS: one prune rule in Timeline.add, a test at both rates.
  - Risk: Low.
  - Purge: No: client only.
  - World-engine: No.
- **Earlier review:** new; X6's fix kept MaxStates as a count cap on purpose
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: No planned rate comes near 180 Hz, and a count cap is the simplest memory bound while a tab is hidden. Converting it to seconds changes nothing players see at 20 or 60 Hz.
- **Evidence:**
  - Verified: MaxStates = 64 is a memory cap; views are drawn at most 0.35 s back. (client/src/engine/sync/timeline.ts:3)
  - Verified: add() shifts out the oldest state past 64. (client/src/engine/sync/timeline.ts:58)
  - Verified: Rule: ring-buffer sizing derives from seconds. (CLAUDE.md:240-242)
  - Verified: X6's fix kept MaxStates as a memory cap on purpose. (earlier review fix-progress.md:135)

<a id="f34"></a>

### F34. Binary recordings are derived files, rewritten at two rates on each predicted change

**not standard**, low. From `tests-7`.

- **Where:** `shards/game/main_test.go:2576-2629`, `shards/game/testdata/prediction-20hz.msgpack`, `shards/game/testdata/prediction-60hz.msgpack`, `client/src/game/player/predictor.test.ts:26-28`, `client/scripts/predictor.mjs:24-34`
- **What breaks:** Any change on the predicted path (predictor code, or a shot, ability or effect number in the scenario) rewrites 4 files, 1.77 MB raw, by two commands. Two branches that both do it conflict in binary; the fix is to re-record after the merge. A bolt speed tweak changes 45-47% of digest lines. Each new mechanic must extend the one scenario, so the files grow with content.
- **Measured:** Raw/gzip -9: 20 Hz msgpack 297,195/14,849 B, txt 76,556/7,746; 60 Hz msgpack 1,116,646/48,748, txt 281,763/24,438. In a scratch git repo, a bolt speed 15 to 16 re-record adds 28 KiB to an aggressively packed repo (98 to 126 KiB), about 96 KB as loose objects. At least 6 re-records noted during the fixes. move_speed 5 to 5.5 passes with no re-record; bolt speed fails. Re-recording today's code reproduces every golden byte for byte. At 2-4 a week: about 3-6 MB a year packed.
- **Mature engines:** Go's golden-file pattern keeps expected output in testdata and rewrites it with -update [sourced: https://www.github.com/xorcare/golden]. Riot records inputs and recomputes state from them [sourced: https://www.riotgames.com/en/news/determinism-league-legends-introduction]. Git LFS counts every version ever pushed against storage, with 10 GiB free [sourced: https://docs.github.com/en/billing/concepts/product-billing/git-lfs].
- **Fix:** Keep the text digests as the only goldens. Make the msgpack a build output: vitest's global setup (predictor.mjs) runs the Go test with a flag that writes it to a gitignored path, at the same tags. Go checks its run against the digest; the wasm checks against a fresh native run. Git keeps one reviewed text file per rate.
- **Rejected:**
  - *Keep as is.* Four files and two commands per change, binary merge conflicts, and a stale SimVersion inside every recorded sync.
  - *Smaller scenario.* Fewer bytes but the same churn and conflicts, and less coverage.
  - *Store a hash.* No reviewable diff; X8's digest exists to show what changed.
  - *Git LFS.* Solves only size, which isn't a problem at 28 KiB a change; adds LFS setup to every clone and CI; still conflicts.
- **Cost:**
  - Size: small: a flag, a predictor.mjs step, one .gitignore line, two files removed
  - Risk: low: the digest keeps the baseline
  - Purge: no
  - World-engine: no
- **Earlier review:** new (fix X8's digest is the right golden; the msgpack twin it kept is the cost)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Repo growth is small (about 28 KiB packed per re-record), and only predicted-path changes need one. One author means no conflicts yet. So the cost is churn and review noise, not size: low.
- **Evidence:**
  - Verified: -update writes msgpack and txt and returns; a normal run checks the digest, then replays natively (shards/game/main_test.go:2576-2629)
  - Verified: The wasm test reads the recording by its rate (client/src/game/player/predictor.test.ts:26-28)
  - Verified: Each recorded step stores the whole BodySync, SimVersion included (shards/game/main_test.go:2201-2203)
  - Summarized: Re-records during the fixes (3, 12, X8, X5, X6) (fix-progress.md:19, 91-92, 116, 123-124, 133-134 (earlier scratchpad))

<a id="f35"></a>

### F35. Test-only effect kinds ship in content; the recording reads shipped balance

**not standard**, low. From `content-7`.

- **Where:** `shards/game/content/effect/tuning.json:3-6`, `shards/game/main_test.go:2443-2467`, `CLAUDE.md:480-482`
- **What breaks:** phase, stun and slow ship in content/effect/tuning.json, but no ability uses them; 13 tests do. CLAUDE.md says test content never goes in shipped files. The coupling also runs the other way: the prediction recording (both rates) is built from the shipped bolt, dash, slow and stun. A designer's balance tweak breaks it until a programmer re-records with -update. With 50+ abilities a year, balance passes will keep churning it.
- **Measured:** grep: phase is used by 0 abilities and 4 tests, stun by 0 abilities and 10 tests, slow by 0 abilities and 5 tests (13 distinct tests). TestPredictionMatchesServer patches only the Echo delay and loads shipped content for both the server and the predictor.
- **Mature engines:** Golden-file tests pin their own inputs, so only a code change moves the golden file [from memory]. Unreal and Unity projects keep test maps and test assets apart from shipped content [from memory].
- **Fix:** Move phase, stun and slow into the patches of the tests that use them (withContent, and patched() for a predictor). Pin the recorded run's whole kit (the bolt, dash, slow and stun numbers) in its own patch, so balance changes never move the recording. Shipped content then holds only what abilities use.
- **Rejected:**
  - *Keep them as foundation kinds.* They wait for abilities the GDD hasn't planned, and the tests stop designers from changing them.
  - *Re-record on every balance change.* It turns each balance tweak into a programmer task and a binary diff review.
- **Cost:**
  - Size: S: test patches, then one re-record
  - Risk: low
  - Purge: no
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: These kinds are the built effect foundation, meant for the first abilities, and tests that read shipped numbers adapt on their own. But a binary recording can't adapt, and no GDD ability needs phase, stun or slow yet.
- **Evidence:**
  - Verified: Only the dash kind is named by an ability (shards/game/content/ability/tuning.json:8)
  - Verified: CLAUDE.md says never to add test content to the shipped files (CLAUDE.md:480-482)
  - Verified: The recorded run uses shipped content plus a 1 s Echo delay (shards/game/main_test.go:2445-2449)
  - Verified: The recorded run applies the shipped slow and stun as surprises (shards/game/main_test.go:2464-2467)

<a id="f36"></a>

### F36. Determinism scan covers a hand-written package list, not the predictor's imports

**not standard**, low. From `prediction-7`.

- **Where:** `shards/game/main_test.go:2852-2855`, `shards/game/main_test.go:2866-2896`, `scripts/check.sh:12-16`
- **What breaks:** TestSharedCoreIsDeterministic compiles the packages in sharedCore for arm64, amd64/v3 and wasm and fails on fused multiply-adds or inexact math calls. That check is strong, but only for listed packages: -gcflags=-S prints only the packages named. A new package the predictor imports is not scanned until someone adds it by hand. A miss shows up as steady small corrections on clients of an arm64 server, not a desync. No CI runs it yet.
- **Measured:** go list -deps ./shards/game/predictor in the copy: the shared packages it links are the 10 in sharedCore (box2d included) plus content and main, so the list matches today. check.sh runs go test at default and rate60 on the host (amd64) only; there is no .github/workflows, and the pre-push hook is opt-in. What the scan leaves: compiler drift is covered by SimVersion (Go release in the hash, sim.go:24-49); sqrt is an exact instruction; box2d's atan2 and sin/cos are plain arithmetic inside the scan; NaN bits only matter if compared; map order in predict is result-free today (meetShape returns 1, full-key ties) but only the recordings guard it.
- **Mature engines:** Box2D v3 tests cross-platform determinism in CI on x64 and ARM [sourced: https://box2d.org/posts/2024/08/determinism/]. Fiedler: the same binary is safe; across compilers and CPUs it takes care and testing [sourced: https://gafferongames.com/post/floating_point_determinism/]. Photon Quantum avoids floats with fixed point [sourced: https://doc.photonengine.com/quantum/v2/manual/quantum-ecs/fixed-point]. GGPO games run sync tests that re-simulate and compare checksums [from memory].
- **Fix:** Scan what the predictor actually links: take the list from go list -deps ./shards/game/predictor, keep this module's packages plus box2d, and drop the hand list. Then a new import is scanned the day it lands. Run check.sh in CI so the scan runs on every change.
- **Rejected:**
  - *Replay the recordings on arm64 under qemu.* Catches the same fused-math class the scan already finds in compiled code, and adds minutes to every run.
  - *Fixed-point math, like Quantum.* Rewrites Box2D and every shared package; the float path already matches bit for bit.
- **Cost:**
  - Size: S
  - Risk: low: test-only
  - Purge: no
  - World-engine: no
- **Earlier review:** new (S10 judged determinism fine; still true for today's imports; S5 shares this root)
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: The list matches the predictor's imports today. Servers build for default amd64, which never fuses, so a missed package changes nothing until a server moves to arm64 or v3. Insurance, not a live bug.
- **Evidence:**
  - Verified: sharedCore is a hand-written list of 10 packages, box2d included (shards/game/main_test.go:2852-2855)
  - Verified: the scan builds the listed packages with -gcflags=-S for three targets and flags FMA and inexact math calls (shards/game/main_test.go:2866-2896)
  - Verified: check.sh runs vet and tests at default and rate60 on the host only (scripts/check.sh:12-16)
  - Verified: no CI workflows exist (.github/workflows (absent))
  - Verified: SimVersion hashes non-test source with the Go release and rate (sim.go:24-49)
  - Summarized: Box2D v3 tests determinism across x64 and ARM (https://box2d.org/posts/2024/08/determinism/)

<a id="f37"></a>

### F37. No per-player command budget or request size cap at the shard

**not standard**, low. From `netcode-7`.

- **Where:** `world-engine@v0.22.0-beta/pkg/cardinal/internal/command/queue.go:26`, `world-engine@v0.22.0-beta/pkg/cardinal/service.go:154`, `shards/game/internal/player/input_system.go:49`, `shards/game/internal/player/input_system.go:129`, `world-engine@v0.22.0-beta/pkg/cardinal/service.go:999`
- **What breaks:** Client fields are well guarded, but the edges are not. Cardinal accepts any request size (connect's default) and queues any number of commands per player per tick. Each valid Input costs 1–6 µs in the tick, so about 3000 in one tick would stall a 60 Hz shard. Each oversized Input writes a warning. Any authenticated player can claim a free Body with one Input, and DEV auth trusts X-Player-Id.
- **Measured:** enqueue benchmark in the copy: 1.0 µs at cap 10 (20 Hz), 5.8 µs at cap 30 (60 Hz), 2.7 µs for a reversed Input into an empty queue. A 16.7 ms tick fits about 2900 Inputs. Outside the tick, the server path costs 21–33 µs per POST.
- **Mature engines:** Source caps how many user commands it runs per tick (sv_maxusrcmdprocessticks) and drops packets that claim too many commands [from memory]. Game servers rate-limit each connection before work reaches the simulation [from memory].
- **Fix:** In world-engine, give each player a command budget per second (a few times the tick rate) and set WithReadMaxBytes on the Cardinal handler. Past the budget, drop and count, never log per command. Claiming moves to matchmaking, which assigns the Persona at spawn, as spawn_system.go already plans.
- **Rejected:**
  - *Rate-limit in InputSystem.* By then commands are already decoded and queued, so the memory and decode cost is paid outside the tick.
  - *Rely on the gateway.* The dev edge has no limits [from memory]. The shard should protect itself.
- **Cost:**
  - Size: S
  - Risk: low
  - Purge: no
  - World-engine: yes: a per-player command budget and ReadMaxBytes in Cardinal's service
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Stalling a 60 Hz tick takes about 170k valid POSTs a second, and HTTP gives out first. Claiming is labeled scaffolding, and DEV auth is documented. So low.
- **Evidence:**
  - Verified: The command queue is unbounded (world-engine@v0.22.0-beta/pkg/cardinal/internal/command/queue.go:26-31)
  - Verified: No ReadMaxBytes is set, and connect's default accepts any size (world-engine@v0.22.0-beta/pkg/cardinal/service.go:154-160; connectrpc.com/connect@v1.21.0/option.go:280-282)
  - Verified: Moves and aims that are NaN, Inf or too large become zero (shards/game/internal/player/motion/motion.go:23-29)
  - Verified: Button bits beyond the kit are ignored (shards/game/internal/ability/cast/cast.go:59-61)
  - Verified: A first Input claims a free Body (shards/game/internal/player/input_system.go:129-151)
  - Verified: DEV auth trusts the X-Player-Id header (world-engine@v0.22.0-beta/pkg/cardinal/service.go:999-1005)

<a id="f38"></a>

### F38. seenBy can't judge gone entities or what a shot's end reveals

**not generic**, low. From `netcode-8`.

- **Where:** `shards/game/internal/netsync/items.go:43`, `shards/game/internal/netsync/items.go:81`, `shards/game/internal/projectile/sync_system.go:44`, `shards/game/internal/projectile/projectile_sync_event.go:20`
- **What breaks:** netsync judges an item only by its entity and the viewer's team. A gone item (AddGone) has the zero entity, so seenBy can't judge it. A shot's end carries TargetSerial and Offset, so end minus offset gives the target's center. A shot's first tick carries Caster plus PrevX and PrevY, which is its caster's spot. Once anything is hidden, these leak it, and the one place CLAUDE.md promises can't stop them.
- **Measured:** Read only. seenBy returns true today (items.go:81), so nothing leaks yet. The GDD plans no stealth, and map sightlines are still open.
- **Mature engines:** Valorant's fog of war withholds enemy positions the server judges unseen, while its server still decides hits [from memory]. Unreal judges relevancy per actor and connection, and owner or instigator links make an actor relevant [from memory].
- **Fix:** Make each item list every entity it reveals: Add(e, audience, state, refs...) with the target, caster or owner it points at. seenBy then hides the item unless the viewer's team sees all of them. A gone entity passes its last position and team instead of the zero entity.
- **Rejected:**
  - *Strip TargetSerial and Offset from others' shots.* Breaks 'a shot that ended on a target ends on it on every screen' (CLAUDE.md:525-530).
  - *Wait until stealth exists.* The cost is the API shape: every sync's Add call changes later. Today there are five call sites.
- **Cost:**
  - Size: S
  - Risk: low
  - Purge: no
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Nothing is hidden today, and the GDD plans no stealth. Low, and worth doing only while netsync has five callers.
- **Evidence:**
  - Verified: AddGone items carry the zero entity (shards/game/internal/netsync/items.go:43-45)
  - Verified: seenBy sees only (entity, team) (shards/game/internal/netsync/items.go:71-81)
  - Verified: A shot's end carries TargetSerial and Offset; its first tick carries Caster and Prev (shards/game/internal/projectile/sync_system.go:44-50; shards/game/internal/projectile/projectile_sync_event.go:20-35)
  - Summarized: The GDD plans no stealth, and sightlines are open (docs/GDD.md §8)

<a id="f39"></a>

### F39. The test-file rule puts every world test in one file and blocks package tests

**not standard**, low. From `tests-8`.

- **Where:** `shards/game/main_test.go:1-3111`, `CLAUDE.md:29-31`, `shards/game/internal/player/input_system.go:68`
- **What breaks:** World-level tests need register, which only package main has, so all 69 sit in main_test.go. The rule also allows a new test file only for a new source file, so enqueue, the mover and the cast gate can't get a unit or fuzz test beside their code. With more contributors, every feature PR edits one 3,100-line file and its 58 shared helpers.
- **Measured:** main_test.go: 3,111 lines, 69 tests, 58 helpers. Sections: input 390 lines/9 tests, abilities and shots 1,004/28, Echo 312/9, movement 90/3, effects 388/14, prediction 665/3, shared core 48/1, wire and view 215/2. Grew 247 to 3,111 lines in 18 days; 9 of 10 game-code commits touched it. The Echo section calls helpers from four other sections. Run time 0.6 s at 20 Hz, 2.4 s at 60 Hz, plus the 5.7 s FMA scan. Integration coverage of internal packages: 90-100%, except area 78%, physics 56%, team 36%.
- **Mature engines:** Go's own net/http has 38 test files; 19 mirror no source file, and serve_test.go is 8,181 lines [from memory; counted in local GOROOT 1.27.1]. Go keeps fuzz targets in the package's own _test.go files [sourced: https://go.dev/doc/security/fuzz/].
- **Fix:** Keep the rule's intent (no topical one-off files, one harness) and restate it: a test file mirrors one source file or one feature package. Split main_test.go into package-main files per feature (player, projectile, echo, effect, prediction, wire), with the helpers staying in main_test.go. Allow unit and fuzz tests beside existing code.
- **Rejected:**
  - *Keep one file.* Size is fine for Go, but every PR touches the same file, and package-level fuzz tests stay banned.
  - *Importable register plus a gametest package.* Restructures production wiring for the tests' sake; same-package files already share helpers.
  - *Table-of-contents comments.* Helps navigation only; same conflicts, same ban.
- **Cost:**
  - Size: small: move code, no logic change, one CLAUDE.md line
  - Risk: low
  - Purge: no
  - World-engine: no
- **Skeptic:**
  - Not run: lean run, so the skeptic checked high findings only.
  - Reviewer's check: Size alone isn't the problem: net/http's serve_test.go is 8,181 lines, the suite is fast, and coverage is high. One author means few conflicts today. So low, not medium.
- **Evidence:**
  - Verified: main_test.go has 69 tests and 58 helpers in 8 sections (shards/game/main_test.go:391, 1395, 1707, 1797, 2185, 2850, 2898)
  - Verified: main_test.go runs real ticks through register (CLAUDE.md:29-31)
  - Verified: enqueue has no package test file (shards/game/internal/player/input_system.go:68)
  - Summarized: Rule: tests go in existing files; a new test file only mirrors a new source file (user standing feedback (task text))

## Recent calls re-examined

The calls you asked about, in your order. Then other recent fixes the reviewers checked.

| Call | Verdict | Why | See |
|---|---|---|---|
| Test content moves a dash and a bolt "a unit / 0.75 u a tick at any rate" (main_test.go:884, 2758) | sound | Both tests check tick-space rules: which rows a view uses, and where between them. Rate-scaled content keeps that geometry at 60 Hz. Same-game coverage at 60 Hz comes from seconds-based tests on shipped content. The comments say so. | |
| The own-Echo shot test checks the hit's offset, not touching (main_test.go:1541-1544) | mixed | The offset check is the right property for that test. But touching was dropped because at 60 Hz the bolt ends 0.067 u inside the Echo (Until 0). That is the frozen-pose bug, not a test problem. Yes: shots should sweep against moving targets. | [F2](#f2) |
| 1e-9 slack in the phase-landing test (main_test.go:1874) | duct tape | The test sits exactly on a side tie and a bound tie. The slack hides float drift instead of asserting the exact cell. | [F28](#f28) |
| 1e-9 slack in the bad-network correction bound (main_test.go:2682) | sound | It only absorbs rounding in sums of steps. It is about 8e7 times smaller than the smallest walk step (0.083 u at 60 Hz). Bitwise checks elsewhere still catch real drift. | |
| The client's recording guard, loosened to hold at both rates (predictor.test.ts:52-54) | duct tape | It swapped X5's exact (several frames, on target) pairs for a set. The 60 Hz recording has no multi-frame step that ends a shot, and the guard can't see that. | [F14](#f14) |
| MaxStates = 64, kept as a memory cap (timeline.ts:3) | mixed | A memory bound is right for hidden tabs. A tick count breaks the seconds rule and fails above about 180 Hz. | [F33](#f33) |
| The fixed 0.05 s pacing target (input_stream.ts:8) | mixed | The closed loop is right. The setpoint is a TS constant that never reacts to coasts. Speed-up tops out at 5%, not the 10% the docs say. | [F7](#f7) |
| SimVersion(rate): callers pass the rate (sim.go:24) | mixed | It works and is tested, but it knows only the rate tag. debug.ReadBuildInfo would cover every tag with no parameter. The rate tag can't reach `world build` at all. | [F23](#f23) |
| Per-rate golden recordings, re-made on each sim change | mixed | Growth is small: about 28 KiB packed per re-record, and only predicted-path changes need one. The real cost is binary derived files per rate that churn and conflict. The text digest alone should be the golden. | [F34](#f34) |
| Correction fades: SnapDistance 3 u, FadeSeconds 0.1 s (prediction.ts:9-10) | mixed | A per-key exponential fade is standard and rate-safe. The two numbers are TS constants tied to no champion speed. | [F32](#f32) |
| Latched taps (controls.ts:65-73) | sound | This is Source's latch. A short tap still casts, and only the first frame of a catch-up burst gets it. Tested. | |
| Reach tree rebuilt every tick, O(T·W) (view.go:62-82) | sound | 2.1-3.0 µs at 12 targets, 5.3-7.6 µs at 24. Simpler than keeping a tree in sync. | |
| The view floor (input_system.go:100-103) | sound | It stops a client walking its view back and forth to pick the best past. It does not bound a steady old view. Only the rewind cap does. | [F9](#f9) |
| The render clock (timeline.ts:22-27, 128-134) | sound | Standard snapshot interpolation. Jitter up to 80 ms never freezes it. A 300 ms stall freezes it about 0.2 s, then it catches up, never going back. | |
| scripts/check.sh plus an opt-in hook instead of CI | duct tape | check.sh itself is good: both rates, 31 s cold, 7.4 s warm. As CI it runs nowhere: the hook is off, `--no-verify` skips it, and it tests the working tree, not the pushed commits. | [F4](#f4) |
| Open item 15: timed path for shots | sound to defer | Shots fly straight at constant speed. A timed path adds nothing until the first bounce or curve. | |
| Open item 16: jump bit | mixed | The right fix, and small. Land it with the first blink, teleport or Echo-Swap. It costs about 15 edits today because fields are copied by hand. | [F21](#f21), [F18](#f18) |
| Open item 18: WebSocket input | not blocked | Input{Session, Frames} is transport-agnostic. Only ShardConnection.send changes here. The work is in world-engine. After it lands, drop the 0.4 s resend. | [F24](#f24) |
| Open: walls in the predictor | mixed | The predictor's casts already include map kinds. But the server's wall builder is internal to physics2d, imports Cardinal and uses math.Cos/Sin. The docs disagree on when it lands. | [F8](#f8) |
| Open: CI | see above | | [F4](#f4) |

Other fixes checked:

| Fix | Verdict | Why | See |
|---|---|---|---|
| Fix 7: SimVersion hashes the content with the code | sound | It hashes the bytes both loaders read, and the two decoders agree (probe). | |
| Fix 8: ref.Spawn serials in syncs | mixed | Sound for syncs: clients and the predictor key by serial. Stored IDs stayed raw (Owner, Caster, Source). Its Spawn check voided the ally test. | [F22](#f22), [F5](#f5) |
| Fix 11: one Cardinal-free cast step (cast.Run plus a Sink) | sound | A new request type is a Sink method, so neither side compiles until it handles it. The tick order lives in both main.go and predict.Step, but TestPredictionMatchesServer pins it. | |
| Fix 12: one target record (TargetHistory), one writer | sound | 9-13 µs a tick at 20 Hz, 21-29 µs at 60 Hz. | |
| Fix 17: Reach tree once per tick; max_target_speed removed | sound | O(T·W) build, then one tree query per shot. A global speed knob checked by hook name is gone. | |
| Run every Go test at 20 and 60 Hz | sound | It catches per-tick constants. It found the rounding the phase slack now hides. | |
| Box2D v3 mover port, planes sorted by geometry | sound | Standard algorithm, 0.13-0.78 µs a move. Exact across rates except the 1 cm stop rule. | [F29](#f29) |
| Effects feed the one mover | sound | One movement path for input, dash, push, stun and phase, shared with the predictor. | |
| EntityViews keyed by bigint Serial | sound | Ends blending across reused IDs. 0.58 µs a frame for 32 lookups, noise beside 8.1 µs per frame. | |
| Land: nearest clear spot on a 0.25 m grid | mixed | Deterministic and simple. But its `ok` is dropped, and its preconditions are unchecked. | [F26](#f26) |

## Docs vs code

Where CLAUDE.md, docs/ or a code comment says one thing and the code does another. Duplicates from several reviewers are merged.

### CLAUDE.md

- **:733-734** says the build uses a `replace` in go.mod. go.mod pins world-engine v0.22.0-beta with no replace, and CLAUDE.md:37-39 agrees with go.mod. The same stale claim sits in client/vite.config.ts:4-7, client/tsconfig.json:17-19, .claude/commands/review-changes.md:46 (which also names a vendor/ that doesn't exist) and map-system-findings.md:224 (its trap list is now reversed).
- **:28** says there is no CI because "a runner needs go-ecs beside the repo". Only the client needs go-ecs. Go fetches world-engine from proxy.golang.org, so the Go half could run in CI today. See [F4](#f4).
- **:27-28** says the hook "makes each push run it first". The hook is opt-in and off (core.hooksPath unset), and it tests the working tree, not the pushed commits. See [F4](#f4).
- **:468-499** mandates the data plugin, bans hand-rolled loaders, and says its hash reconcile lets a game resume on its exact config. predict.Load is already a hand-rolled loader. world-engine main deleted the plugin (#1133). The reconcile only logs a warning, since the embed is the only source. See [F3](#f3).
- **:476-479** says names decode strictly, so a typo fails the boot. True for names and hook params. Tuning keys decode loosely, and a typo loads as 0. See [F3](#f3).
- **:626-630** says pacing runs the frame clock faster or slower "by at most 10%" and absorbs a clock "a few percent off". Speed-up tops out at 5%, since the queue is never below 0. See [F7](#f7).
- **:614-616** (and favor-the-shooter.md:188-191) says pacing keeps late frames rare. The target is fixed and never reacts to coasts. Sim: 1% loss gives 9.4 coasts a minute at 20 Hz, 56.6 at 60 Hz. See [F7](#f7).
- **:604-616** says prediction is exact unless the network surprises it. Your dead Echo mispredicts one tick on a perfect network. See [F6](#f6).
- **:304** says first hit wins, so a swept path can't tunnel at any speed. True for still targets. A moving target can cross a shot's path inside a tick. See [F2](#f2).
- **:540** says Caster and Owner are same-tick links, so they stay EntityIDs. Projectile.Cause.Caster and team.Owner are stored for the entity's whole life. See [F22](#f22).
- **:548-550** says hidden info is decided in one place, netsync.seenBy, including indirect reveals. AddGone passes the zero entity, and a shot's TargetSerial plus Offset, or Caster plus Prev, point at entities seenBy never judges. No leak today, since seenBy returns true. See [F38](#f38).
- **:585** (and favor-the-shooter.md:100-102) says shot code names no target kind except to find your own Echo, and a new kind needs no predictor, glue or shot code. targetKinds is a const naming Body and Echo, Row is one circle, and your own Body is special-cased twice. See [F19](#f19).
- **:617** says predictor walls come with the arena. map-system-findings.md:214-216 says they come later, at step 5, through a world-engine export. The two disagree. See [F8](#f8).
- **:251-252** says a build tag picks the rate. `world build` passes no tags, and main.go's TickRate overrides world.toml's tick_rate. A deployed shard is always 20 Hz. See [F23](#f23).
- **:596-599** says SimVersion covers the source "without tests or testdata". True for the hash. The binary still embeds 1.92 MB of tests and recordings. See [F23](#f23).
- **:233** says a 60 Hz build is the same game as 20 Hz. Exact to 1e-12 m on open ground and straight walls. Convex corners and pillars differ 1.6-6.2 cm from sampling. The 1 cm stop rule differs 0.118 m. See [F29](#f29).
- **:254-255** says the client's tests run at 20 and 60 Hz. Only predictor.test.ts reads the build's rate. input_stream.test.ts loops over both rates itself. The rest use 0.05 s, so the rate60 run repeats 90 of 94 client tests unchanged.
- **:253** says tests state times in seconds. main_test.go:2485 uses a lag of 2.3 ticks, and :1642 adds 5 raw ticks. Minor: 2.3 ticks is 38 ms at 60 Hz, under the 0.1 s interpolation delay.
- **:631** says past input_buffer_seconds the oldest frames drop. Correct, but the drop is never logged or counted. See [F25](#f25).
- **:643-646** says a new champion is JSON entries, never code. True for the server. Other screens can't tell champions apart: BodyState has no Champion. See [F20](#f20).
- **:440** (and design lines 29, 591) says every receiver sorts its intents. echo.SpawnSystem, EquipSystem and CastHookSystem walk events in emit order. Harmless today (each touches its own entity, and what they emit is sorted later), but the rule as written is false.
- **:377-379** says friend or foe goes through team.SameTeam. SameTeam (team.go:64) has no callers. Every rule uses interact's Relation through team.SideOf. Delete the helper or fix the doc.
- **:131-133** lists player's imports without echo/component, which player/sync_system.go:9 imports. A leaf import, which the rule allows. The list just omits it.
- **:59 and :205** say collision bits live in physics/categories.go. They live in interact/interact.go:18-27. categories.go:24 holds only Match, which nothing calls.
- **:245-248** says "if Health.HP stays an int, a continuous source must accumulate the fraction". HP is float64. Anti-camp still kills at exactly 4.0 s at 20 and 60 Hz (one tick late at 30 Hz).

### docs/research and docs/GDD.md

- **system-architecture-design.md rule 10 (:40) and :271** ask for Ref{ID, Serial} on stored refs to things that can die. Cause.Caster (Echo casters die) and status Entry.Source are raw IDs. See [F22](#f22).
- **system-architecture-design.md rule 14 (:46)** says a match resumes only on the build that started it. Nothing enforces it. The data plugin warns and resumes on new content.
- **system-architecture-design.md §6 (:399)** puts echo.ReplaySystem in Update. main.go:82 registers it in PreUpdate. §6 also lacks History, TargetSync and Death. CLAUDE.md is right; the design list is stale.
- **system-architecture-design.md §12 (:581)** says areas use OverlapAABB plus a circle test. area_system.go:66-68 uses one OverlapShape. The code is better; update the doc.
- **system-architecture-design.md :159** says a mark-then-burst combo needs no code. Payload is a bool and there is no marked tag, so it needs Go changes.
- **system-architecture-options.md :182** plans a snapshot-restore test. None exists. See [F15](#f15).
- **character-foundation.md :84** says a stun or root ends your own dash. Any cc ends it, a slow included. See [F17](#f17).
- **character-foundation.md :69** shows `physics.MoveBlockers &^ st.Pass`. It is now motion.Blockers (motion.go:18).
- **character-foundation.md :93-97** says Land covers phase, blinks and restore lag, and map tests check wall depth and pockets. No blink, map or map test exists, and depth is only a comment. See [F26](#f26).
- **favor-the-shooter.md :218-225** says every multi-frame replay lands on the server's bits, ends included, at 20 and 60 Hz. True at 20 Hz only. See [F14](#f14).
- **favor-the-shooter.md :234 and system-architecture-design.md :572** say each new rule, broken once, fails a test (mutation checks). The mutants are not in the repo. See [F27](#f27).
- **map-system-findings.md :134** says the static map is baked data, never snapshot state. :188 of the same doc plans one static ECS entity. See [F8](#f8).
- **map-system-findings.md :42-43** says the mover sticks before a two-box seam at 85° or more. The current mover passes that seam at 84-89° at both rates (probe). The reason to reject flush seams no longer holds.
- **GDD.md :163, :209-213** describe an Environment category and a CollisionSystem → DamageResolveSystem flow. The code uses Wall and Bounds kinds, and hits resolve inside flight's Box2D cast. The GDD notes predate the query-based design.

### Code comments

- **ability/component/abilities.go:12-13**: only CastSystem writes Abilities. EquipSystem also sets it on claim (equip_system.go:27).
- **echo/spawn_system.go:51-55**: one Echo per Body for the whole match ("Echo death = elimination"). The GDD says out for the round, with best-of-N rounds. See [F1](#f1).
- **echo/spawn_system.go:53**: DamageResolveSystem destroys the old Echo. DeathSystem does (death_system.go:20).
- **player/spawn_system.go:56-57**: the first Input claims the Body, "see MovementSystem". InputSystem claims it (input_system.go:128-151).
- **client/scripts/predictor.mjs:1-2**: builds with "the Go toolchain the server is built with". It runs the local go. The shard builds in golang:1.27.1-bookworm. They match only while local Go is 1.27.1. See [F23](#f23).
- **shards/game/main_test.go:1871**: she lands "on the side she came from". Land takes the nearest cell, ties in grid order. It has no notion of direction. See [F28](#f28).
- **client/vite.config.ts:4-7, client/tsconfig.json:17-19**: the SDK alias works "like go.mod's replace". There is no replace. The alias points at an unpinned local go-ecs checkout (branch ryan/revert, at 0ec6eb05c, which no remote has).
- **.claude/commands/review-changes.md:46**: gives reviewers wrong build facts (a local replace and a vendor/). The command also never runs check.sh or the 60 Hz suite.

## Solid as is

Keep these. Duplicates from several reviewers are merged.

### ECS and sim

- **Package by feature, with component leaves** (shards/game/internal/*). go list shows no feature importing another feature's behavior package.
- **One constructor per entity kind** (player/spawn_system.go:50-79, echo/spawn_system.go:101, projectile/launch_system.go:51). Four Create sites in server code, each stamping ref.Spawn.
- **One writer per component.** 16 game components. Two justified exceptions: Transform2D is split by entity kind, and Equip sets Abilities once.
- **Deferred intents with one sorted resolver each** (launch_system.go:34-83, damage_system.go:30-91, effect/resolve_system.go:27-53). Emit order, which can change after a restore, never changes a result.
- **No state outside components.** Systems hold only plugin handles. Package vars are read-only tables.
- **Snapshot-safe component shapes.** No maps, pointers or private fields. Every component round-trips through msgpack.
- **Deterministic server sim.** No clock, no rand. Map ranges only in Validate and Bind. RequireDeterministic runs.
- **System order pinned by tests** (main.go:73-132). 13 of 17 order mutations fail a test. Three of the four that pass commute. Only Equip-before-Cast is unpinned ([F31](#f31)).
- **Linear per-tick cost, no O(P²).** Game systems take 124-135 µs a tick at 20 Hz and 173-205 µs at 60 Hz, at 3v3 with about 20 shots.
- **Rate-independent sim** (timestep/rate20.go, rate60.go). Per-second rates scaled by dt. Straight paths match to 1e-12 m between 20 and 60 Hz.
- **Deterministic ID reuse in Cardinal** (world_state.go:128-131). The smallest free ID is reused, and restore rebuilds gaps in order, so IDs match after a restore.

### Input and netcode

- **Input is routed by the authenticated player, never by the payload** (input_system.go:41-46). A client can't drive another Body. Commands from a shard are refused.
- **Client values are sanitized** (motion.go:23-29, cast.go:59-61). NaN, Inf, huge or zero moves and aims become zero. Button bits beyond the kit are ignored.
- **One frame per Body per tick, in Seq order, duplicates dropped** (input_system.go:68-107). Extra frames can't speed a Body up.
- **Oversized Inputs are dropped before they can claim a Body** (input_system.go:47-52). The merge costs 1-6 µs per Input.
- **No Broadcast anywhere; cooldowns go to allies only** (ability/sync_system.go:36). sentTo fails any test that broadcasts.
- **Each sync is complete every tick and keyed by Serial.** Joins and reconnects need no extra path. Keep this; compress the stream instead ([F10](#f10)).
- **The sim-version check comes before any use of a sync** (local_player.ts:110-118). A mismatched client never acks, steps or claims.
- **Wire fixtures pin the Go and TS types both ways** (main_test.go:2906-3021, wire_fixtures.test.ts). They cover bigint past 2^53, unicode, and TS-to-Go input.
- **Latched taps** (controls.ts:65-73), Source-style.
- **The pacing loop works in seconds and takes its cap from the server** (input_stream.ts:76-101). Keep the loop; change only its setpoint ([F7](#f7)).
- **The input timeout stops a silent client** (movement_system.go:56-63). It coasts on the last frame, never on Velocity2D, then stops after 0.5 s.
- **netsync's audience filter** (netsync/items.go:54-76). Small, one list per team. It could move to world-engine if a second game needs it.
- **Streams end instead of dropping** (Cardinal service.go:406-430). A stream 5 s behind is closed and the client reconnects. Events are never silently thinned.

### Prediction and lag compensation

- **The server's Go code is the predictor** (predictor/main.go, predict/predict.go). One sim, no TS copy. Bit-exact with the server at both rates, and on arm64 under qemu.
- **Server rewind with no client hit claims** (view.go:88-116). The server runs its own query. A client only picks a view inside the window.
- **Pair cast in the target's frame** (view.go:121-140). Bit-exact with the world cast, no slack. It is also the base for [F2](#f2)'s fix.
- **View clamp with no cliff, plus ViewFloor** (view.go:88-94, input_system.go:100-103). NaN, Inf and stale views clamp into the window. Backtracking can't pick an older moment.
- **One view rule, pinned on both sides** by testdata/view.json (TestViewFixture, entity_views.test.ts).
- **Row as one target record, keyed by serial**, with a per-tick Reach rebuild of 2-8 µs.
- **Server lag compensation costs under 50 µs a tick** at 3v3, even at 60 Hz.
- **The predictor's world never steps** (predict.go:143-215). Queries only, so no step order or contact state can drift. p50 0.039 ms, p99 0.198 ms a call.
- **Per-key exponential correction fade** (prediction.ts:129-149). Standard and rate-safe. Only its constants should move ([F32](#f32)).
- **cast.Run plus a Sink** (cast.go:82-107). One glue for server and predictor. A new request type fails to compile until both handle it.

### Content and rules

- **Rule vocabulary with first-match rules, and the GDD's rules as default content** (interact.go:67-96, flight/tuning.go:104-112). A shot that passes Bodies or reads a tag is JSON only.
- **Kind, tag and relation bits pinned by the wire fixtures.** Renumbering a bit changes fixture bytes on both sides, so append-only is enforced.
- **Cast gate and cooldowns as absolute server ticks, per ability ID**, shared with the predictor (cast.go:16-71). A client can't speed a cooldown up.
- **Hooks: one registry, strict params at the build's rate, Refs for cross-file IDs** (hooks.go:77-114). launch and dash make common abilities JSON only. A dash covers its exact distance at both rates.
- **Status is folded on read, never cached** (status.go:62-82). 16-185 ns a fold, 0 allocations. A cache would add a second truth for nothing.
- **Combat: Health only on Echoes, Died timed by the lethal hit, DeathSystem after the syncs.** Every screen sees a death once, at its real time. Float HP keeps anti-camp exact at 20 and 60 Hz.
- **Pass with a payload is rejected until a hit list exists** (flight/tuning.go:93-94). No GDD content needs a hit list yet.
- **Anti-camp is the Echo's area on its own hurtbox** (echo/spawn_system.go:117-121, area_system.go:37-83). One Box2D overlap per area per tick.
- **The Echo ring is sized from seconds and prefilled** (echo/spawn_system.go:94-99). It holds exactly the delay at any rate.
- **The shard and predictor decoders agree** on all shipped content and 10 edge cases. Keep that property when [F3](#f3) lands.
- **Content-wide tests scale with content** (TestAnyCasterCastsEveryAbility, TestContentIDsLineUp). They loop over every ability, champion and kit.
- **Tests change content through the real loader** (withContent and patched, main_test.go:80-141).

### Physics and client engine

- **A faithful Box2D v3 mover** (mover.go:26-60): collide, sort, solve, cast. 0.13-0.78 µs a move. Planes sorted by geometry, so shape IDs don't matter.
- **Effects feed the one mover** (motion.go:44-57). Input, dash, push, stun and phase share one Move.
- **A cheap physics2d step.** About 10.5 µs a tick for 6 Bodies, the Box2D step alone 1.3 µs.
- **Land is deterministic** (land.go:14-37). Power-of-two grid offsets, fixed tie order.
- **The render clock** (timeline.ts:22-27, 128-134). Time-based ease, never steps back, capped at the newest tick.
- **EntityViews** (entity_views.ts:43-90). Serial keys, reused meshes, shared materials. 8.1 µs a frame for 32 entities.
- **Engine and game split on the client.** Nothing under client/src/engine imports game code.
- **Controls** (controls.ts:29-33). Physical key codes and a clear on blur.
- **Wasm pipeline** (client/scripts/predictor.mjs). Stamped simVersion, stripped build: 1.41 MB gzip, 10.6 ms compile, under 1 s warm rebuild. One caveat: tight-loop GC pauses of 1.8-6.8 ms.

### Tests and tooling

- **Bit-exact Go vs wasm chain** (main_test.go:2576-2629, predictor.test.ts:50-75). Re-recording reproduces every golden byte.
- **FMA and Cardinal-import scan of compiled code** for arm64, amd64/v3 and wasm (main_test.go:2866-2896). math.Hypot can't sneak in.
- **SimVersion coverage test** (sim_test.go:72-116). go list proves every file the builds use is hashed.
- **Tests stated in seconds** (ticks(seconds), drawLag). All 15 rate mutants are caught.
- **badNetwork scripted network** (main_test.go:2640-2750). Lost, held, bunched and dropped frames, with vacuity counters.
- **A fast suite.** check.sh takes 31 s cold, 7.4 s warm.
- **Earlier mutants still caught.** 127 of 128 that still apply fail a test. The one miss is the known unobservable G6.
- **Core properties hold under fuzzing.** FlyAtView equals Fly over 8.6M execs. enqueue's invariants hold over 2.1M.
- **Client tests mirror source files and rebuild the wasm on each run**, so vitest never runs a stale predictor.

## Appendix A. Mechanisms enumerated

Each reviewer's list, from the code. Overlaps between areas are kept.

### ECS architecture

| Mechanism | Where | What |
|---|---|---|
| Tick hooks and order | shards/game/main.go:73-132 | PreUpdate: Input, Equip, Cast, Movement, Replay, then the physics step. Init: DevSpawn. Update: echo.Spawn, CastHook, Launch, Flight, Area, DamageResolve, effect.Resolve, History, 5 syncs, Death. 20 game systems. |
| System events | world-engine@v0.22.0-beta/pkg/cardinal/internal/ecs/world.go:41-46 | Same-tick only and cleared at tick end, so each emitter registers before its receivers. 8 game system events. |
| Entity IDs | world-engine@v0.22.0-beta/pkg/cardinal/internal/ecs/world_state.go:16,128-131 | uint32 with no generation; the smallest free ID is reused at once. |
| ref.Spawn serials | shards/game/internal/ref/ref.go:31-39 | One world counter. Every constructor stamps a serial, and syncs and the predictor key by it. |
| Snapshots | shards/game/main.go:24; world-engine@v0.22.0-beta/pkg/cardinal/world.go:318-319 | The whole world as msgpack every 10 ticks, and every tick under debug. |
| Restore | world-engine@v0.22.0-beta/pkg/cardinal/world.go:184-190; internal/ecs/world_state.go:367-462 | Init systems run, then restore. IDs are rebuilt in ascending order, and an unknown component fails the restore. |
| Content loading | shards/game/main.go:42-49; shards/game/internal/predict/predict.go:36-49 | The data plugin catalog on the server (6 tunings); predict.Load in wasm (3 tunings). |
| Body claim | shards/game/internal/player/input_system.go:108-151 | A player's first command claims the next free Body, and BodyClaimed fires once. |
| Echo spawn | shards/game/internal/echo/spawn_system.go:72-125 | One Echo per claimed Body, with a PathHistory ring of delay+1 poses and an anti-camp Area from tick+delay. |
| Deferred intents | ability/cast_hook_system.go; projectile/launch_system.go; combat/damage_system.go; effect/resolve_system.go | Hooks emit Launch, Damage, Heal and Effect intents, and one owner resolves each kind, sorted. |
| Rewind targets | shards/game/internal/projectile/history_system.go; projectile/flight/view.go | TargetHistory rows per Body and Echo, plus a Reach tree each tick. Predicted shots meet targets where the shooter's view had them. |
| Physics plugin | world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/system/pipeline.go:74-115 | In PreUpdate: reconcile by entity ID, step, then writeback (Manual and Static bodies skipped). |
| Tick rate | shards/game/internal/physics/timestep/rate20.go, rate60.go | A build tag picks 20 or 60 Hz, shared by server, physics and predictor. |
| Sync and visibility | shards/game/internal/netsync/items.go | Per-team lists at O(teams·N). seenBy returns true today. |
| Death | shards/game/internal/combat/death_system.go:18-22 | Destroys Died targets in sorted order, after the syncs sent them once more. |

### Input and netcode

| Mechanism | Where | What |
|---|---|---|
| Input command | shards/game/internal/player/input_command.go:12 | Input{Session, Frames}: a client's unacked frames, newest last, routed by the authenticated player ID. |
| InputFrame | shards/game/internal/player/component/input.go:14 | Seq, MoveX/MoveY, AimX/AimY, Buttons (bit N casts kit slot N) and ViewTick. Move() and Aim() go through motion.Unit. |
| receive | shards/game/internal/player/input_system.go:38 | Drops commands not sent by a player and Inputs over the cap, claims a free Body on a player's first Input, then merges. |
| enqueue | shards/game/internal/player/input_system.go:68 | Merges frames by Seq, skips applied and duplicate frames, and trims to the cap, oldest first. A new Session starts over but keeps LastTick and ViewFloor. |
| advance | shards/game/internal/player/input_system.go:92 | Applies one frame per Body per tick. A view below ViewFloor, or NaN, is taken at the floor. |
| Applied, View and Ack | shards/game/internal/player/component/input.go:46 | Systems act on the frame applied this tick. View ages the last ViewTick by the ticks since. |
| Coast and timeout | shards/game/internal/player/movement_system.go:56 | With no frame, a Body repeats the last frame's move until input_timeout_seconds (0.5 s), then stops. |
| Claiming | shards/game/internal/player/input_system.go:109 | bodyIndex maps each Persona to its Body. A new player's first Input claims a free Body and emits BodyClaimed. |
| Dev spawn | shards/game/internal/player/spawn_system.go:42 | Spawns two Bodies (a 1v1) at Init: scaffolding until matchmaking. |
| Movement tuning | shards/game/internal/player/tuning.go:13 | input_buffer_seconds (the cap), input_timeout_seconds and echo_path_seconds, converted at the build's tick rate. |
| netsync.Viewers | shards/game/internal/netsync/viewer.go:23 | Players with a claimed Body, with their team: the SendTo recipients. |
| netsync.Items and Lists | shards/game/internal/netsync/items.go:33 | Per-entity state with an audience (Everyone or Allies), filtered once per viewer team. |
| seenBy | shards/game/internal/netsync/items.go:81 | The visibility hook. It returns true today. |
| BodySync | shards/game/internal/player/sync_system.go:51 | Per viewer: Tick, TickSeconds, SimVersion, YouState (ack, queue, cap, kit, effects, own shots, Echo delay and path) and all Bodies. |
| EchoSync | shards/game/internal/echo/sync_system.go:64 | All Echoes with HP and Until. A dead one is sent once more, at HP 0. |
| AbilitySync | shards/game/internal/ability/sync_system.go:26 | Kits and ReadyTicks, sent to the owner's team only (Allies). |
| ProjectileSync | shards/game/internal/projectile/sync_system.go:18 | Shots in flight, plus each shot that ended this tick (AddGone) with its Until, target serial and offset. |
| TargetSync | shards/game/internal/projectile/target_sync_system.go:22 | Each Body's and Echo's row this tick (kind, side, center, radius, tags, Until), for the predictor. |
| Event encoding | world-engine@v0.22.0-beta/pkg/cardinal/internal/schema/schema.go:17 | msgpack.Marshal by reflection: maps keyed by Go field names, float64, 8-byte uint64. |
| Event delivery | world-engine@v0.22.0-beta/pkg/cardinal/service.go:623 | Serialized per recipient and wrapped in StartEventStreamResponse{address, name, payload} on the player's one server stream. A 16 MiB backlog or a 5 s lag ends the stream. |
| SendTo | world-engine@v0.22.0-beta/pkg/cardinal/world.go:692 | Sends a targeted event, silently dropped when the player has no open stream. |
| Command intake | world-engine@v0.22.0-beta/pkg/cardinal/service.go:296 | Unary SendCommand, decoded into an unbounded per-type queue (queue.go:26) and drained at the start of the tick. |
| SDK transport | go-ecs/pkg/sdk-ts/src/shard/create-connect-client.ts:20 | connect-web with binary protobuf. Every request carries X-Player-Id (DEV) or a bearer token. |
| SDK codec | go-ecs/pkg/sdk-ts/src/shard/codec.ts:5 | @msgpack/msgpack with useBigInt64, so 64-bit ints decode as bigint. Payloads reach the game as decoded objects. |
| ShardConnection.send | client/src/engine/net/shard_connection.ts:60 | Fire-and-forget sendCommand that logs one warning per kind of error. |
| InputStream | client/src/game/player/input_stream.ts:30 | Numbers frames by Seq and keeps 6.4 s of unacked frames. Sends one Input per new frame carrying min(0.4 s, cap) of them, caps a burst at 0.15 s, and paces toward a 0.05 s queue (EMA 0.5 s, gain 1/s, ±10%). |
| LocalPlayer | client/src/game/player/local_player.ts:110 | Checks SimVersion first, then acks and reconciles. Sends frames only while playing, and a join frame on each connect. |
| Controls | client/src/engine/input/controls.ts:65 | Buttons = held \| pressed since the last take (latched taps). |
| Frame loop | client/src/main.ts:84 | Each animation frame builds and sends Inputs with the last drawn render tick as ViewTick, then advances the render clock. |
| SimVersion | sim.go:24 | The Go release, the rate and a sha256 of the non-test files in the embedded go.mod, go.sum, pkg and shards. body_sync sends it. |
| Version stamp | client/scripts/predictor.mjs:21 | Runs go run ./shards/game/predictor/version, then stamps the result into predictor.wasm with -X main.simVersion. |
| Wire fixtures | shards/game/main_test.go:2906 | Golden msgpack per payload in testdata/wire, decoded and compared by client/src/game/wire_fixtures.test.ts. |
| Telemetry | world-engine@v0.22.0-beta/pkg/cardinal/metrics.go:19 | OTel metrics for tick duration, overruns, commands and events sent. Spans per tick, per system (when recording) and per event publish. |

### Client prediction and lag compensation

| Mechanism | Where | What |
|---|---|---|
| Reset | shards/game/internal/predict/predict.go:259-264 | Loads the acked state (Body, shots, kit, Echo path) into the predictor. |
| Step | shards/game/internal/predict/predict.go:321-341 | One local tick: move, cast, place targets at the frame's view, fly shots, apply ends. |
| See | shards/game/internal/predict/predict.go:154-183 | Stores each target_sync's rows for KeptRows ticks; keeps bodies only for targets the newest sync shows. |
| place | shards/game/internal/predict/predict.go:220-257 | Places each target at the view with AtView; your own Body and Echo at your predicted pose. Allocates a map per step. |
| fly and affect | shards/game/internal/predict/predict.go:375-416 | Flies predicted shots one tick with FlyAtView and applies their ends to your state. |
| Echo and echoAt | shards/game/internal/predict/predict.go:278-296 | Your Echo from your predicted path and the delay. |
| reconcile | client/src/game/player/prediction.ts:83-120; client/src/game/player/local_player.ts:127 | On every body_sync: reset to the acked state, replay all unacked frames, read shots, Echo and kit. |
| correction smoothing | client/src/game/player/prediction.ts:9-16,129-149 | Per-key offset fades by exp(−tick/0.1 s); an error over 3 u snaps. |
| wasm glue | shards/game/predictor/main.go:95-183; client/src/game/player/predictor.ts:98-121 | Exports reset, peek, step, seeTargets, forget, shots, echo, kit; reads each field through js.Value; uint64 as decimal strings. |
| server view pipeline | shards/game/internal/player/input_system.go:68-103; shards/game/internal/player/component/input.go:57-59; shards/game/internal/projectile/flight/view.go:88-94 | Frame ViewTick, raised to ViewFloor, plus coast ticks, clamped to [tick−R, tick−1]. |
| Reach and FlyAtView | shards/game/internal/projectile/flight/view.go:62-140; shards/game/internal/projectile/flight_system.go:147-158 | Per-tick DynamicTree of each target's kept rows; per shot a map cast, a tree query and a ShapeCastCircle in the target's frame at the view. |
| live Fly | shards/game/internal/projectile/flight/flight.go:102-214 | World cast against Box2D bodies at this tick's end pose; first hit wins. |
| TargetHistory and TargetSync | shards/game/internal/projectile/history_system.go:51-52; shards/game/main.go:125-129 | Each target keeps KeptRows rows, copied whole each tick; sent to clients last in the tick. |
| own Body and Echo at now | shards/game/internal/projectile/flight_system.go:121-143; shards/game/internal/predict/predict.go:236-249 | The caster's own Body and Echo are judged where they are now, not at the view. |
| ends drawn | client/src/game/player/prediction.ts:145; client/src/engine/render/entity_views.ts:27-31 | Predicted ends kept from the last step only; blend draws nothing past Until. |
| SimVersion | sim.go:24-49 | Go release, rate and a hash of non-test source, baked into the wasm. |
| determinism scan | shards/game/main_test.go:2852-2896 | Builds sharedCore for arm64, amd64 v3 and wasm; fails on FMA, inexact math calls or a Cardinal import. |
| Big-O | measured in the copy | Step O(T + S·(log T + C)); See O(T + W); server per shot O(log T + C); Reach O(T·W + T log T); history copies O(T·W) per tick. |
| prediction gaps | shards/game/internal/predict/predict.go; client/src/game/player/prediction.ts | Not predicted: effects others cause (0.75-1.25 u walking, 3-5 u on a cut dash), late frames (up to one walk step each), walls (none yet), a target someone else kills first, others' Bodies (drawn in the past; every contact would mispredict if Bodies block), Echo-Swap and delay changes, a death when body_sync lands before target_sync (up to one tick wrong end). |
| wasm size | P/wasm/predictor.wasm (copy) | 5.1 MB, 1.4 MB with gzip -9. |

### Content and rules

| Mechanism | Where | What |
|---|---|---|
| interact vocabulary | shards/game/internal/interact/interact.go:13-129 | Kind (6 bits), Tags (4 of 64), Relation (4), Filter{Kinds, Relations, Require, Forbid}. First returns the first matching rule and its index. Names decode strictly. |
| team | shards/game/internal/team/team.go:18-71 | Team, Owner and SideOf build an interact.Side. Same owner is own, same team is ally, anything else is enemy. SameTeam is unused. |
| Content files | shards/game/content/*/tuning.json; shards/game/content/embed.go | Six JSON files embedded as content.FS: 1 champion, 2 abilities, 1 shot type, 4 effect kinds, 1 area kind. |
| Data plugin (pinned v0.22.0-beta) | shards/game/main.go:42-49; world-engine@v0.22.0-beta/pkg/plugin/data | Register and Get, a goccy Unmarshal, Resolve (AbilityTuning binds hooks), Validate, and a ConfigManifest reconcile over an embed-only source. |
| predict.Load | shards/game/internal/predict/predict.go:38-60 | Decodes abilities, shots and effects with encoding/json, then runs Bind and Validate, for the wasm predictor. |
| Cross-file checks | shards/game/main_test.go:1351-1396 | TestContentIDsLineUp checks hook Refs (shots, effect kinds) and that each champion has a kit, an Echo and a known area kind. |
| Test content | shards/game/main_test.go:80-141 | withContent and patched deep-merge JSON patches into the shipped files. overlay serves the plugin (Fetch) and predictors (Open). |
| Ability rows | shards/game/internal/ability/cast/tuning.go | Kits by champion (slot N = button bit N), Def{cooldown_seconds, aimed, hook, params}. Bind binds hooks at load. At most 32 slots. |
| Cast gate | shards/game/internal/ability/cast/cast.go:48-71 | Each held, ready, aimed kit slot casts, in slot order. A stunned Body casts nothing. Cooldown = tick + round(seconds × rate). |
| Hook run | shards/game/internal/ability/cast/cast.go:82-107 | cast.Run sets TickRate, runs OnCast, and hands Launches then Effects to a Sink. The server sink emits intents; the predictor's keeps your shots and self effects. |
| Hooks | shards/game/internal/ability/hooks/hooks.go:30-114 | Out{Launches, Effects}. Effect{Target, Kind, Ticks, Vel} has no Value. Registry: launch, dash. Params are strict, with Validate(rate) and Refs. |
| Ability systems | shards/game/internal/ability/equip_system.go, cast_system.go, cast_hook_system.go, sync_system.go | Equip resets cooldowns on claim. CastSystem (PreUpdate) emits AbilityCast. CastHookSystem (Update) emits LaunchIntent and EffectIntent. AbilitySync goes to allies only. |
| Effect entries | shards/game/internal/effect/status/status.go:32-133 | Entry{Kind, Tags, Pass, Stat, Value, VelX, VelY, Source, Start, End, Serial}. Stacking: merge, per_source, instanced. A refresh keeps the later End. cc ends ends_on_cc entries; unstoppable blocks cc. |
| Fold | shards/game/internal/effect/status/status.go:61-91 | ORs tags and passes, sums hastes, keeps the strongest slow, sums forced velocity. Speed is 0 when stunned, else base × (1 + haste) × (1 − slow). |
| Effect kinds | shards/game/internal/effect/status/tuning.go | KindDef{tags, passes (wall only), stat (move_speed), stacking}. Each kind is resolved again on every Apply and every Compare (14 µs to sort 36 intents). |
| Effect ResolveSystem | shards/game/internal/effect/resolve_system.go:27-69 | Sorts intents by target, cc last, source, kind and numbers, applies them from the next tick, then sweeps entries that are over. An unknown kind is logged and dropped. |
| Shot rules | shards/game/internal/projectile/flight/tuning.go:16-129 | Meet{Filter, do pass\|stop, payload bool}. A shot type's own meets come before the defaults. A thing no rule matches is passed. Pass with a payload is rejected. The map is judged neutral, with no tags. |
| Shot executor | shards/game/internal/projectile/flight/flight.go:142-214; shards/game/internal/projectile/flight_system.go:58-104 | One shape cast finds the nearest hit it doesn't pass. Ties go by rule, then map, then ID. A payload becomes a DamageIntent with Until, and every end becomes a ShotEnded. |
| Areas | shards/game/internal/area/area_system.go:37-99; shards/game/internal/area/tuning.go | Area{Kind, Start} acts on the holder's first circle. One OverlapShape per area per tick, then the first matching rule per thing. The only response is to: self, damage_per_second × dt. |
| Combat | shards/game/internal/combat/damage_system.go; shards/game/internal/combat/death_system.go | Float Health on Echoes only. Damage is sorted by (target, Until, credit, cause, amount), and heals are added first. Died carries the first lethal hit's Until. DeathSystem destroys after the syncs. |
| Echo | shards/game/internal/echo/spawn_system.go; replay_system.go; component/echo.go | Spawns once per Body on BodyClaimed. The ring holds delay+1 poses, prefilled. Replay reads the oldest slot. Path() feeds YouState. Anti-camp Start = spawn + delay. |
| Cause and credit | shards/game/internal/ability/component/ability_cast_event.go:8 | Cause{Caster, Ability, Tick, Seq} rides on casts, shots and damage. Credit is the dealer's Owner. |
| Counts | code and content | Hooks 2. Hook request types 2. Shot responses 2 plus a payload bool. Area responses 1. Tags 4 of 64. Kinds 6 of 64. Relations 4. Stats 1. Stacking rules 3. Effect kinds 4 (1 used by an ability). |
| Per-tick cost | effect/status/zz_probe_test.go (a scratch copy) | Fold takes 16, 27, 48, 91 and 185 ns at 2, 4, 8, 16 and 32 entries. 7 call sites make about 40 folds a tick at 3v3, about 1 µs. Areas: one OverlapShape each, 6 a tick at 3v3. |
| Change (a): ability on an existing hook (Echo-Medic's primary fire via launch) | content/ability/tuning.json; content/projectile/tuning.json | 2 JSON files, 0 Go. TestContentIDsLineUp and TestAnyCasterCastsEveryAbility cover it automatically. The client draws every shot type as one ball. |
| Change (b): ability with a new hook (Echo-Medic heals its own Echo) | new ability/hooks/heal_hook.go; ability/hooks/hooks.go (registry, Out.Heals, the caster's Echo via Cast or a Reader); ability/cast/cast.go; ability/cast_hook_system.go; predict/predict.go; main_test.go AnyCaster; content/ability/tuning.json | 8 files (6 Go). Echo-Swap takes about 12: add a teleport (player/motion or effect/status), the Echo ring rewind (echo/replay_system.go, echo/component) and YouState. |
| Change (c): shot type | content/projectile/tuning.json (plus an ability row) | 1-2 JSON files, 0 Go. |
| Change (d): effect kind | known stat or tag: content/effect/tuning.json. New stat (echo_delay, GDD §9 #5): effect/status/status.go, status/tuning.go, echo/replay_system.go, echo/component/echo.go, echo/spawn_system.go, echo/tuning.go, player/sync_system.go, predict/predict.go, content, plus hooks.go and cast.go for Value | 1 JSON file. But the first effect a hook sends with a number also edits hooks.go and cast.go. A new stat is about 11 files plus a re-record. |
| Change (e): target kind (a placed barrier for Tank screening) | interact/interact.go; projectile/flight/tuning.go (targetKinds); area/tuning.go (meetable); new area/place_system.go, register.go, main.go; content/projectile defaults; client view | About 7 files. History, TargetSync and the predictor are generic. A box shape adds Row, AtView and Reach (+3). |
| Change (f): champion (Assassin, Echo-Medic, Tank) | content/player/tuning.json; content/ability/tuning.json; content/echo/tuning.json | 3 JSON files and 0 Go on the server. The first second champion also needs BodyState.Champion, a client decoder, a look per champion and wire fixtures: about 5 files, once. |
| Change (g): tag (the Assassin's invulnerable) | interact/interact.go (constant and name); combat/damage_system.go (the reader, which then imports effect/component); content/effect/tuning.json | 2 Go files and 1 JSON file. A tag that blocks or cancels something also edits status.Add or cast.Gate. |
| Change (h): rule response (deflect, GDD §9 #3; bounce) | flight/tuning.go; flight/flight.go; flight/view.go; projectile/component/projectile.go; flight_system.go; launch_system.go; projectile_sync_event.go; sync_system.go; predict/predict.go; predictor/main.go; client draw; recordings; content | About 13 files. The timed path (fix 15) is still open. |
| Change (i): new hook request type (Heals) | ability/hooks/hooks.go (Out); ability/cast/cast.go (Sink and Run); ability/cast_hook_system.go; predict/predict.go; main_test.go AnyCaster | 5 files, or up to 9 when the receiver is new (intent in its leaf, receiver system, register.go, main.go). The Sink interface makes a missed side a compile error. |
| GAS: activation, costs, cooldowns | shards/game/internal/ability/cast/cast.go:48-71 | Has activation (the gate) and cooldowns as server ticks. No costs: GDD §9 #1 leaves resources open, so they come later. |
| GAS: tags that block or cancel | effect/status/status.go:96-101; ability/cast/cast.go:55 | Has them, but as Go rules. Needs a data form before more CC types arrive (F17). |
| GAS: ability tasks (multi-tick abilities) | shards/game/internal/ability/hooks/hooks.go:51-55 | Missing: hooks run once, on cast. Needed later for Echo-Swap's recast window, charges or channels: per-cast state plus a per-tick runner, not a rewrite. |
| GAS: prediction keys | shards/game/internal/predict/predict.go (Key{Seq, Ability, Launch}) | Has an equivalent: full re-simulation from body_sync, keyed by input Seq, Overwatch style. |
| GAS: effect types | shards/game/internal/effect/status/status.go | Has duration effects, and instant damage and heals as intents. Infinite and periodic (DoT, HoT) effects are missing. They are needed later for passives, Echo-Medic heals over time and a ramping anti-camp (GDD §9 #4). |
| GAS: stacking and modifiers | shards/game/internal/effect/status/status.go:22-30, 61-91 | Has 3 stacking rules and one fixed combine rule per stat, like Dota and League; that part is fine. The refresh ignores strength (F16). |
| GAS: executions and immunity | combat/damage_system.go; effect/status/status.go:96-99 | One execution (DamageResolveSystem) is right. Immunity is only unstoppable against cc; invulnerable is planned for the Assassin. |
| GAS: Gameplay Tags | shards/game/internal/interact/interact.go:29-38 | A flat closed list (4 of 64) with JSON names, Require and Forbid queries, and bits pinned by fixtures. Hierarchy isn't needed at this size. |
| GAS: attribute sets | combat/component/health.go; player/component/move_speed.go | Health and MoveSpeed are per-feature components, and the fold computes speed on read. That is fine: a new stat is 3 edits in status plus its reader. |
| GAS: Gameplay Cues | player/body_sync_event.go; echo/echo_sync_event.go | Missing: other players get folded tags only, and no champion (F20). |
| Rewrite risk at 50+ abilities a year | whole area | Nothing built needs a rewrite. The costly gaps are tag relations in code (F17), refresh rules (F16), multi-tick abilities (a new runner) and the loader (F3). |

### Physics and client engine

| Mechanism | Where | What |
|---|---|---|
| Mover | shards/game/internal/physics/mover/mover.go:26-50 | Box2D v3 geometric mover: CollideMover, sort planes, SolvePlanes, CastMover; up to 5 passes, stops on a pass under 1 cm; ClipVector gives the velocity. |
| Plane sort | shards/game/internal/physics/mover/mover.go:53-60 | Orders planes by normal X, Y, then offset. |
| Land | shards/game/internal/physics/mover/land.go:49-57 | Nearest clear spot on a 0.25 m grid within 6 m (1,793 cells); the ray there must not cross Bounds; ok=false if none. |
| landGrid | shards/game/internal/physics/mover/land.go:17-37 | Precomputed offsets sorted by i²+j², then i, then j. |
| Inside | shards/game/internal/physics/mover/land.go:41-43 | Zero-radius overlap of the center against blockers. |
| motion.Step | shards/game/internal/player/motion/motion.go:44-57 | Mask Blockers minus pass; Land if inside (ok dropped); velocity from input x folded speed or a forced velocity; then Move. |
| Blockers | shards/game/internal/player/motion/motion.go:18 | Wall \| Bounds; a pass effect may drop only Wall (status/tuning.go:101). |
| MovementSystem | shards/game/internal/player/movement_system.go:36-52 | PreUpdate, before the physics plugin; runs motion.Step on physics2d's Engine() world; sole writer of Body Transform2D and Velocity2D. |
| Input coast | shards/game/internal/player/movement_system.go:56-64 | Keeps the last frame's direction until the input timeout. |
| Tick rate | shards/game/internal/physics/timestep/rate20.go, rate60.go, timestep.go | Build tag picks 20 or 60 Hz; SecondsPerTick is shared with the predictor. |
| Physics plugin config | shards/game/internal/physics/physics.go:33-56 | NewPlugin, EntityOf, WorldPoint (math.Cos/Sin, server only, via area_system.go:64). |
| physics.Match | shards/game/internal/physics/categories.go:24 | Contact-event helper; nothing calls it. |
| Interaction kinds | shards/game/internal/interact/interact.go:18-27 | Category bits Wall, Body, Echo, Shot, Bounds, Area; Map = Wall \| Bounds. |
| Manual Bodies and Echoes | shards/game/internal/player/spawn_system.go:68-74; shards/game/internal/echo/spawn_system.go:110-116 | BodyTypeManual circles (kinematic in Box2D); Bodies pass through each other. |
| physics2d pipeline | world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/system/pipeline.go:74-179 | Each tick: gather, reconcile ECS into Box2D, Step with 4 substeps, flush contacts, write back (skips Manual and static). |
| physics2d Init rebuild | world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/system/init.go:55-63; internal/rebuild.go:38-80 | Builds Box2D from ECS on boot and restore; destroys only bodies it tracks, keeps the world. |
| physics2d shape builder | world-engine@v0.22.0-beta/pkg/plugin/physics2d/internal/create.go:123-272 | Manual maps to Kinematic; boxes rotate with MakeRot; polygons with math.Cos/Sin and ComputeHull; imports Cardinal. |
| Contact events | world-engine@v0.22.0-beta/pkg/plugin/physics2d/plugin.go:163-168 | Contact and trigger begin/end system events registered; echo_image reads none. |
| Box2D mover primitives | world-engine@v0.22.0-beta/pkg/box2d/world_query.go:546-600,641,656; pkg/box2d/mover.go | CastMover with encroach (deep overlap unhandled), CollideMover, SolvePlanes (20 iterations), ClipVector. |
| Shot cast | shards/game/internal/projectile/flight/flight.go:142-171 | One Box2D shape cast over map kinds and targets; ties: earlier rule, then map, then lower target ID. |
| FMA scan | shards/game/main_test.go:2853-2896 | sharedCore (box2d included) must import no Cardinal, emit no FMA on arm64, amd64/v3 or wasm, and call only exact math. |
| Predictor | shards/game/internal/predict/predict.go:122-215 | Private box2d.World, never stepped, no map; kinematic target circles; runs motion.Step and flight. |
| Wasm bridge | shards/game/predictor/main.go:26-90 | js.FuncOf exports (reset, peek, step, seeTargets, forget, shots, echo, kit); results as map[string]any. |
| Predictor build and load | client/scripts/predictor.mjs; client/src/game/player/predictor.ts:91; client/src/engine/wasm/go.ts | go build -trimpath, stripped, simVersion stamped, wasm_exec.js copied; fetch, arrayBuffer, instantiate; go.run not awaited. |
| Timelines and render clock | client/src/engine/sync/timeline.ts:3-134 | Per-sync buffers (64 states); clock 0.1 s behind, eases with tau 0.5 s, jumps past 0.25 s drift, never steps back, capped at the min newest tick. |
| EntityViews | client/src/engine/render/entity_views.ts:27-90 | Views keyed by bigint Serial, meshes reused; blend between the tick pair. |
| Stage | client/src/engine/render/stage.ts:17-44 | Fixed orthographic camera at (0,0,20), 20 m tall view, grid helper; no follow yet. |
| Controls | client/src/engine/input/controls.ts:3-33 | WASD and arrows by e.code; cleared on blur. |
| Event stream | client/src/engine/net/shard_connection.ts:40-46; go-ecs/pkg/sdk-ts/src/shard/create-stream-manager.ts:95-158 | Connect server stream; reconnect with 0.5-8 s backoff; terminal codes stop it; no idle timeout. |
| Cardinal stream service | world-engine@v0.22.0-beta/pkg/cardinal/service.go:406-430,484-498 | 16 MiB queue; a stream lagging 5 s is ended, never thinned; keepalive every 30 s. |
| Input stream | client/src/game/player/input_stream.ts:5-90 | One frame per tick, 0.4 s resent per Input, unacked cap 6.4 s, queue pacing toward 0.05 s. |
| Frame loop | client/src/main.ts:84-101 | requestAnimationFrame; renderTick, then each view kind draws. |

### Tests and tooling

| Mechanism | Where | What |
|---|---|---|
| World test harness | shards/game/main_test.go:56-196 | cardinal.NewTestWorld with the real register; input/frame builders; tuningOf reads shipped content. |
| Content patching | shards/game/main_test.go:80-141 | withContent/patched deep-merge JSON into shipped files; overlay serves them to the data plugin and the predictor. |
| Scripted clients | shards/game/main_test.go:396-485 | client type (session, seq, view lag) with next/send/cast/fire/restart; duel and claim set up a 1v1. |
| sentTo | shards/game/main_test.go:143-158 | Returns the one event a player got; fails if any was broadcast. |
| Seconds helpers | shards/game/main_test.go:441-444 | ticks(seconds) and drawLag convert seconds at the build's rate. |
| Test emitter system | shards/game/main_test.go:1801-1837 | Injects system events (effects, casts) into a running world. |
| Hand-built extra entities | shards/game/main_test.go:381, 636, 1000, 1659, 1722 | carol allies, area holders and map boxes created in tests, outside the real constructors. |
| TestPredictionMatchesServer | shards/game/main_test.go:2443-2630 | Server vs predictor per step and replay (requirePredicted, requireEnded); writes or checks the per-rate recording and digest, then replays it natively. |
| badNetwork | shards/game/main_test.go:2640-2750 | Scripted lost, held, bunched and missing frames; replays checked when their frame applies; vacuity counters. |
| TestPredictorPlacesOnlyWhatSyncsShow | shards/game/main_test.go:2752-2848 | Predictor placement table in tick space, with a bolt scaled to 0.75 u a tick. |
| TestSharedCoreIsDeterministic | shards/game/main_test.go:2853-2896 | go list (no Cardinal import) and an assembly scan for FMA and non-exact math on arm64, amd64/v3 and wasm. |
| Wire fixtures | shards/game/main_test.go:2906-3016; client/src/game/wire_fixtures.test.ts | Go-written payload goldens checked in TS; TS-written input_client checked in Go (UPDATE_FIXTURES=1). |
| View fixture | shards/game/main_test.go:3054-3111; shards/game/testdata/view.json | Go writes where views draw targets; entity_views.test.ts must match to the bit. |
| RequireDeterministic | shards/game/main_test.go:2143 | Runs one script twice from fresh worlds and compares digests per step; no restore. |
| -update flag | shards/game/main_test.go:2900 | One flag rewrites wire, prediction and view goldens for the selected tests, at the build's rate. |
| SimVersion tests | sim_test.go:18-116 | versionOf table plus go list coverage of the shard and wasm builds. |
| Package unit tests | shards/game/internal/{interact,effect/status,ability/hooks,projectile/flight}/*_test.go | Rule matching, status folding, hook binding, Fly ends, AtView table, View clamp, FlyAtView vs Fly table plus a 60-point sweep. |
| Vitest global setup | client/scripts/predictor.mjs; client/vite.config.ts:28 | Builds predictor.wasm with the server's toolchain, stamps SimVersion, copies wasm_exec.js, provides simVersion to tests. |
| Wasm recording test | client/src/game/player/predictor.test.ts:26-118 | Picks the recording by the wasm's rate, replays every step and replay, plus an ends guard and a version check. |
| Client unit tests | client/src/**/*.test.ts | TS stand-ins for the wasm in reconcile tests; timeline and views at 0.05 s; input_stream looped over 20 and 60 Hz. |
| scripts/check.sh | scripts/check.sh:7-19 | gofmt; vet, wasm vet and go test at default and rate60; tsc; vitest at 60 then 20 Hz. |
| Pre-push hook | .githooks/pre-push:3 | Execs check.sh on the working tree; opt-in via core.hooksPath, currently unset. |
| Mutation checks | /tmp/claude-1000/-home-doglightning-world-engine/6a855a41-562e-4a86-b097-fde3ba6e9d68/scratchpad/mut* | 14 Python runners and 6 whole-file mutant sets; exact-text anchors applied in place in the real tree, then restored and cmp-checked. |
| Review command | .claude/commands/review-changes.md | Prompt for a multi-agent code review; no step that runs tests. |
| SimVersion stamping | sim.go:18-50; shards/game/predictor/version; client/scripts/predictor.mjs | Embeds and hashes the sim source; the shard sends it, the wasm gets it by -X. |

## Appendix B. Earlier review status

Status of the 2026-10-31 review's items, as each reviewer found them. Notes from several reviewers are merged.

| ID | Status | Notes |
|---|---|---|
| C1 | fixed (fix 8) / fixed | Syncs and the predictor key by serial. Stored refs are still raw: F22. Fix 8 put a Serial on every synced entity, at about 16 B per entity per tick. Shots are keyed by serial, not EntityID. EntityViews keyed by bigint Serial (client/src/engine/render/entity_views.ts:49). |
| C2 | fixed (fix 12) / fixed | Bodies and Echoes share one target record (TargetHistory). One Row record fed by TargetSync. |
| C3 | fixed | Confirmed earlier in this review. |
| C4 | open | Every shot is still resent in full each tick, and own shots are still sent twice (F10, F18). |
| C5 | fixed | Fix 9 drops Inputs over the cap; the merge measures 1–6 µs. The engine-edge limits are F37. |
| C6 | open / open (netcode area) | Still msgpack maps with field names. A shot is now 230 B (F10). Measured here: 4,738 B and 34 µs decode per player per 3v3 tick; client cost is fine, bytes are netcode's call. |
| C7 | still fine / open | History costs 9-13 us/tick at 20 Hz and 21-29 us at 60 Hz, about 5% of bytes allocated at 60 Hz. Still copied whole: 20 µs / 57 KB a tick at W=9, 41 µs / 121 KB at W=25 (T=12). Fine at 3v3. |
| C8 | open | Now F13, with bytes measured at 7.1x. |
| C9 | holds / fine | No O(N²); every overlap is a Box2D query. Still holds: no O(N²) per tick; hits are Box2D queries. No O(N²) per tick: mover, Land, shots and areas are Box2D queries. |
| C10 | holds / fine | Boundaries hold; the doc omits player importing echo/component. Still holds. |
| C11 | partly open / fixed | Client cap fixed (MaxStepSeconds 0.15, client/src/game/player/input_stream.ts:5); the SnapshotRate part is F13. X6 made the burst cap 0.15 s, in seconds. |
| CV1 | open | Fields are still copied by hand (F18). Judged fine before; the six-layer copy is the request cost counted in F18. |
| D1 | fixed | Ends come from the server's own judge; no claim placement. |
| D2 | fixed | Shots carry their own start point. |
| D3 | open | prediction.ts:145 keeps only the last step's ends; at 60 Hz on a 60 fps or slower screen, two steps in one frame drop an end. |
| D4 | open / open (fix 15 not done) | Deferred as fix 15 (timed path); fine until the first bounce or curve. Not my area, but it gates the first bounce or deflect response (change h). |
| D5 | open | Now F2 (medium after the skeptic): one pose per tick judges 18-46% of crossing hits unlike the draw at 20 Hz. |
| D6 | fixed | Shots fade on a correction like the Body. |
| D7 | fixed | The render clock never steps back. Clock never steps back (max of eased and current); the sim found no step back under jitter or stalls. |
| D8 | fixed | Until is read as part of the tick (entity_views.ts:27-30). |
| D10 | fine | Still holds. |
| D11 | fine / holds | Still holds. Your Body and your shots share one time base. |
| DV1 | fixed | One rule, pinned by the shared view fixture (TestViewFixture). |
| G1 | fixed | Claims are gone; the judge's ends fit any response. Fix 1 (server rewind): any response runs the normal rule check, so pierce and bounce no longer need claims. |
| G2 | open / partly fixed (fix 11) | F18. A new request type must now be a Sink method on both sides, which the compiler checks. Predicting a placed area still needs new predictor state and new draws (prediction area). |
| G3 | fixed | Your Echo is predicted (predict.go:278-296). |
| G4 | fixed | Clamp, no cliff (fix 1). |
| G5 | open | The jump bit (item 16) is not built. It isn't blocked, but it takes about 15 edits (F18). Fix 16 not done; F21. |
| G6 | open / partly fixed (fix 12) | Row is still one circle and targetKinds a const; F19. Target kinds are now one constant (flight/tuning.go:52) and rows are generic, but a row is still one circle (flight/view.go:14-22), so a box barrier needs more. |
| G7 | fixed | Confirmed earlier in this review. |
| G8 | fixed | Confirmed earlier in this review. Fix 17 removed max_target_speed and the dash-only check. TestContentIDsLineUp now checks IDs only. |
| G9 | fixed | Content is inside SimVersion's hash (fix 7). SimVersion tells a mismatched build apart. Fix 7: SimVersion hashes the content and the code, and a client with a different version doesn't play. |
| G10 | fine / revised | The plan is right; docs disagree on its order (see doc_vs_code). It judged the plan right; the plan keeps the map as ECS state and needs a world-engine export (F8). Doc order still conflicts: map doc step 5 vs CLAUDE.md:617. |
| GV1 | fixed (fix 8) / fixed | Syncs no longer blend a new shot into an old one across reused IDs. Serial keys; no blend across reused IDs. Same fix: a new shot no longer blends from an old one with a reused ID. |
| S1 | fixed | Fix 7's SimVersion works. Its gaps are F23. SimVersion: source hash, Go release and rate (sim.go:24-49). |
| S2 | fixed | One view rule; Go and TS share the fixture. |
| S3 | open (partly fixed) / open | X6 put the client numbers in seconds, and fix 9 sends the cap. The pacing and resend numbers are still TS-only (F7). Partly: InterpolationDelay 0.1 still lives only in TS (timeline.ts:9); it feeds F9. |
| S4 | open | The glue is unchanged, and the input frame type is written 5 times (F18). Field-by-field copies remain (predictor/main.go:95-183); F18. |
| S5 | open | The physics reviewer owns wall building; same root as F36 (the scan covers a hand list). Carried over as F8; the fix changes to a Cardinal-free arena package in echo_image, so no world-engine export is needed. |
| S6 | fixed | Same fix 17. |
| S7 | fixed (fix 11) / fixed | Server and predictor use one cast step (predict.go:333). Fix 11: cast.Run plus a Sink is one Cardinal-free glue, and it isn't duct tape. The tick order is still in both main.go and predict.Step, pinned by TestPredictionMatchesServer. |
| S8 | fixed (fix 12) / fixed | Owner and Until come from components and Died, not guesses. Confirmed earlier in this review; the glue no longer guesses owner or Until. |
| S9 | holds, at risk / fine / still fine | The two loaders still match, but the plugin was deleted upstream (F3). Still holds. Re-checked: both loaders decode, Bind and Validate. goccy and encoding/json agree on the shipped content and 10 edge cases. Both skip unknown keys (F3). |
| S10 | holds / fine | The determinism test passes, and full-key sorts are intact. Still holds for today's imports; the list gap is F36. FMA scan, order-free casts and full-key sorts; TestSharedCoreIsDeterministic passed in the copy. |
| SV1 | moot / fine | Fix 1 removed client claims. Reference leaks past seenBy remain (F38). Latent: seenBy returns true (shards/game/internal/netsync/items.go:81), so there is no hidden info yet. |
| T1 | fixed | Fix 2 added YouState.InputQueued and frame-clock dilation, and the cap is now 0.5 s. Its fixed setpoint is F7. |
| T2 | fixed | Hit claims are gone; the server judges at the frame's view. |
| T3 | fixed | Fix 1: views clamp into the window, no cliff. |
| T4 | fixed | Shots and kit fade per key like the Body. |
| T5 | fixed | Ease is per second: 1 − exp(−dt/0.5 s) (timeline.ts:22-27). |
| T6 | fixed | Fix 14 latches taps. Sound. |
| T7 | fixed | Clock capped at the newest tick, so screen and claims stop together. |
| T8 | open | Still one POST per frame; item 18 is not built (F24). |
| T9 | fine | Reset-and-replay is right; its 60 Hz cost is F12. |
| T10 | fine / holds | Still holds: main.ts:93-96 sends the drawn tick before renderTick runs. ViewTick is still the drawn tick on both sides. ViewTick is the drawn tick, one float on both sides. |
| TV1 | fixed | Step flies shots on every replayed frame (predict.go:338); no unchecked segment. |
| X1 | fixed | Shared rule and fixture. Fix 6: one view rule; testdata/view.json pins Go and TS (TestViewFixture, entity_views.test.ts). |
| X2 | fixed | A KeptRows/3 mutant fails TestPredictionMatchesServer and TestPredictorPlacesOnlyWhatSyncsShow. Re-verified: a predictor window one row short (KeptRows()-1) fails TestPredictorPlacesOnlyWhatSyncsShow at both rates. |
| X3 | open | Repo part done (check.sh, opt-in hook); hook not enabled, no CI. Carried over as F4. |
| X4 | fixed | Server ends are compared with the predictor's. Fix 1: requireEnded compares each predicted end to ShotEnded bit for bit (main_test.go:2330). |
| X5 | fixed / fixed at 20 Hz only | badNetwork drives late, coasted, bunched and dropped frames. At 60 Hz no deep replay ends a shot on bob, in Go or the recording. See F14. |
| X6 | fixed / fixed, two leftovers / fix is duct tape | Tests run at 20 and 60 Hz; at 60 Hz the own-Echo check was loosened around Until 0 (F2). Tests run at 20 and 60 Hz; its MaxStates choice (F33) and 1e-9 slack (F28) remain. Rates work (15/15 rate mutants still caught), but its TS ends guard was loosened to pass 60 Hz. See F14. |
| X7 | superseded / obsolete | Claims are gone; blocking is the judge's own cast. Claims and Confirm were removed in fix 1. The analog mutant (FlyAtView's target-kind mask, mut12 G11) is still caught. |
| X8 | fixed | Digest plus consistency check (mutX8 D1 caught). The binary twin it kept is F34. |
| X9 | fine / holds / fine, still holds | The wasm replay test still holds. Wasm build checked against the native run on the same inputs. Wasm vs native on the same inputs. The amd64 recordings also replay bit-exact on arm64 under qemu at both rates. |
| XV1 | fixed | The dying-Echo claim pin left with the claims. Fix 1: dying targets aren't placed; predictor.test.ts:88-118 expects no hit on a target that died. |
| Z1 | fixed | No claims; the judge always runs its own query. |
| Z2 | fixed | Fix 10's ViewFloor. A +Inf view pins it, which hurts only that Body. This belongs to the lag-compensation area. ViewFloor (fix 10) stops backtracking; a steady old view is F9. |
| dispute: RequireDeterministic without restore | open | Still no snapshot restore test. See F15. |

## Appendix C. Probes run

Every probe ran in a scratch copy of the repo. The probe code stays in the session scratchpad, which is temporary; the numbers in this report are its record.

- **ECS architecture:** Every probe ran in a scratch copy: /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/ecs/echo_image (rsync without .git, with go-ecs symlinked). Baseline tests pass there at 20 Hz and 60 Hz. Probe files exist only in the copy: shards/game/zz_probe_bench_test.go (tick benchmarks, snapshot sizes, event bytes, ID alias) and shards/game/zz_probe_wrap.go (per-system timer). For timing, main.go was wrapped and then restored. For F5, content/projectile/tuning.json and main_test.go were edited and then restored. diff -q shows all three identical to the real tree. Benchmarks used 6 Bodies, 6 Echoes and about 20 shots on a TestWorld, which encodes a snapshot and round-trips events every step. At 20 Hz: 0.93-1.16 ms/tick, 649-812 KB and 3,373-4,139 allocs per tick. At 60 Hz: 1.45-1.76 ms, 1.12-1.38 MB and 4,837-5,830 allocs. Game systems alone: 124-135 us/tick at 20 Hz (2.5-2.7 ms per game second) and 173-205 us at 60 Hz (10-12 ms per game second). Biggest systems: Flight 31-51 us, player.Sync 17-26 us, projectile.Sync 15-21 us, History 9-29 us. pprof share of bytes: event dispatch 49% and snapshot encode 29% at 20 Hz; 32% and 48% at 60 Hz. Snapshot payload: 67,375 B at 20 Hz, 160,375 B at 60 Hz. Events (netcode area): 56 KB/tick at 20 Hz and 60 KB/tick at 60 Hz, about 9-10 KB per player per tick. Counts: 20 game systems, 16 game components (plus 4 physics and 1 data), 8 system events, 5 events, 1 command. Each component has 1 writer after construction, except Transform2D (3, split by kind) and Abilities (2). Order mutations: 17 run (scratchpad mutate_order.py), 13 caught. Web: 10 searches and 2 fetches; the Valve wiki fetch returned 403, so that claim rests on the search summary. Nothing changed in /home/doglightning/echo_image, world-engine or go-ecs. No git writes, no game or MCP run, no GitHub.

- **Input and netcode:** All runs happened in /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/netcode/echo_image, an rsync copy with go-ecs symlinked beside it. Changes there only: a ProbePlayers knob in shards/game/internal/player/spawn_system.go, and these probes. shards/game/probe_netcode_test.go measures event bytes for 2/4/6 players × 0/10/20 shots at 20 and 60 Hz (go test and go test -tags rate60), plus envelopes, gzip, stream deflate, the unchanged share, serialize cost and SimVersion cost. shards/game/internal/player/probe_enqueue_test.go benchmarks enqueue. shards/game/probe_rpc benchmarks the SendCommand server path with otelconnect, protovalidate, auth and decode. client/probe/pacing_sim.ts runs the real InputStream against a simulated server queue with bun. I built the shard twice with -o into the scratchpad (with and without the embed), then deleted both binaries. The node script /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/probe-js/input_size.mjs encoded Inputs with the SDK's msgpack and proto. /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/flatemem measured deflate memory. Result tables: /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/bandwidth-20hz.txt and /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/bandwidth-60hz.txt. I never ran the game or any Cardinal or echo-world MCP tool. The real tree in /home/doglightning/echo_image is unchanged: git status still shows the same 89 entries, none of the probe files exist there, and the real sim.go is identical to the copy's.
  - Skeptic: Copy: /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/netcode-skeptic/echo_image (the reviewer's probe and ProbePlayers spawn patch copied in; the real tree was not touched). Reran TestProbeBandwidth at 20 and 60 Hz: identical kbit/s in every row. Reran the unchanged-share test (75% at 20 Hz, 72% at 60 Hz) and the serialize cost (190–225 µs a tick). New shards/game/skeptic_probe_test.go: (1) TestSkepticShipped: shipped content, turning aim, fire held, 2 and 6 players. In-flight shots average 1.3 and 4.0, at most 2 and 6. Raw, wire and deflate measured with per-event and per-tick flush. (2) TestSkepticNoShots: the reviewer's no-shot case, per-event vs per-tick flush (937 vs 172 B/tick for 2 players at 20 Hz). (3) TestSkepticFlushSplit: flate levels 1, 6 and 9 with per-event flush. Level 6 misses matches (1466 B/tick), level 9 finds them (383). This explains why the reviewer's no-shot case compressed worse than 20 shots. Code read: all five SendTo sites, the event structs (ProjectileState grew from 9 fields at HEAD to 17), EchoPath (echo.go:36, EchoPathTicks: 11 poses at 20 Hz, 31 at 60 Hz), WE schema.go:17-23, service.go:240-294 and 623-721 (one response per event), the ServiceAddress fields (Project and ServiceId only, so the envelope estimate holds), connect-go Send flush, connect-web validateResponse (connect-transport.js:179 never checks stream encoding, so HTTP Content-Encoding is transparent), and the world CLI edge ReverseProxy with FlushInterval -1 (passes compression through locally; the production gateway was not checked). Content: bolt cooldown 2 s, speed 15, range 20; Echoes never cast (replay_system.go). C4 and C6 are still open in fix-progress.md, so this is not a fixed item coming back. Sources: Gaffer (901 cubes, 17.37 Mbps, 256 kbit/s target, deltas against acked baselines, no final total), fabiensanglard Q3 (32 snapshots, delta against the last acked), the Iris FNetSerializeDeltaArgs page (acked quantized data), CS:GO rate default 196608 and max 786432 B/s (community pages only).

- **Client prediction and lag compensation:** Everything ran in the copy P=/tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/prediction. (1) Benchmarks written only in the copy: P/echo_image/shards/game/internal/predict/probe_bench_test.go (Reconcile, Reach, FlyAtView, See, FlyLive) and P/echo_image/shards/game/internal/projectile/probe_bench_test.go (HistoryCopies). Run with go test -run XXX -bench . -benchtime=2000x, and -benchtime=200000x -count=3 for FlyLive/FlyAtView, at default and -tags rate60. (2) Wasm: GOOS=js GOARCH=wasm go build -tags "$tags" -trimpath -ldflags "-s -w -X main.simVersion=$sim" -o P/wasm/predictor[-rate60].wasm ./shards/game/predictor, as client/scripts/predictor.mjs does. Timing needed a real file, so -o pointed at P/wasm (outside every repo), not /dev/null; nothing landed in a repo. Timed in node 24 with P/bench/wasm_bench.mjs, which drives reset, seeTargets, step, shots, echo and kit like predictor.ts. (3) A t.Logf PROBE line in the copy's TestShotMeetsYourEchoWhereYouDrawIt, run at both rates. (4) Mutation: KeptRows/3 in the copy's predict.go See; go test ./shards/game/ failed TestPredictionMatchesServer and TestPredictorPlacesOnlyWhatSyncsShow; restored from P/predict.go.orig and diffed identical. (5) python3 -I P/bench/crossing.py: frozen vs exact crossing model. (6) go list -deps of the predictor; gzip -9 size of the wasm. The machine was shared (load up to 50), so I quote the quietest runs; loaded runs were 1.5-3× slower. Nothing changed in the real trees: find over /home/doglightning/echo_image, /home/doglightning/world-engine and /home/doglightning/go-ecs for files newer than 2026-11-01 12:40 returned 0. No git writes, no GitHub, no game or MCP calls, no recordings updated.
  - Skeptic: All probes ran in a scratch copy only; nothing in /home/doglightning/echo_image was changed. (1) Real code: I added a t.Logf to TestShotMeetsYourEchoWhereYouDrawIt in /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/prediction-skeptic/echo_image/shards/game/main_test.go. At 20 Hz: Until 0.8067, |Offset| 0.6450. At 60 Hz: Until 0, |Offset| 0.5833. Both match the reviewer. (2) Model /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/prediction-skeptic/bench/crossing.py gives today's rule against exact motion: 18.3%, 6.0%, 46.2%, 15.4%. These match the reviewer at every crossing phase. A constant one-tick lag doubles them. (3) Vitest probe /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/prediction-skeptic/echo_image/client/src/game/player/alignment_probe.test.ts drives the real LocalPlayer, Timelines, InputStream and Prediction the way main.ts does, with a stub predictor and syncs every tick. Drawn tick − ViewTick − alpha: +0.330 (20 Hz, 60 fps), +0.087 (20 Hz, 144 fps), +0.501 (60 Hz, 60 fps), +0.210 (60 Hz, 144 fps). So view v pairs with the shot's tick start. (4) /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/prediction-skeptic/bench/lagged.py uses those offsets to compare today, the proposed fix and D5's fix. The proposed fix is worst on all 8 rows. (5) /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/prediction-skeptic/bench/headon.py gives the Until-0 odds. (6) I read the Box2D source in the v0.22.0-beta module cache and ran one web search on Unreal. I also checked the earlier review: D5 (low) proposed AtView(v)→AtView(v+1), and D3 (medium) covers Until 0 never being drawn.

- **Content and rules:** Copy: /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/content/echo_image (rsync without .git; go-ecs linked beside it). Baseline: scripts/check.sh passed in 30.7 s at both rates (94 client tests each). Probe files, added only in the copy: shards/game/zz_probe_content_test.go (typo keys load as 0; a dead player keeps shooting, 100 → 75; an own payload rule with no relations hits your own Echo, 100 → 75), shards/game/zz_probe_json_test.go (goccy vs encoding/json, 16 cases, all agree), shards/game/zz_probe_hp_test.go (anti-camp kill tick at 20, 30, 60, 64 and 128 Hz; PathHistory and TargetHistory msgpack bytes; effect-kind list and Champion bytes), shards/game/internal/effect/status/zz_probe_test.go (refresh vs strength; a slow ends a dash; Fold and Compare-sort benchmarks). Read only: world-engine main (git show 134dcc5d), the pinned module cache, and rampage's gamedata.go. In the real tree I made no edits, created no files and ran no git write. git status still shows the same 89 entries and no probe files. I did not run the game or call any MCP tool.

- **Physics and client engine:** Everything ran in a scratch copy only: /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/physics-client (echo_image rsynced without .git; go-ecs symlinked and only read). Go probes: shards/game/internal/physics/mover/probe_test.go in the copy (benchmarks for Move, motion.Step, Inside, Land, Box2D step; tests for rate independence at 20/60/120 Hz, convex corner and pillar rounding, a two-box seam at 45-89°, landing slack, Land side ties, Land inside a solid Bounds box, chain-loop edge, and the 1 cm stop rule). The physics2d plugin and game-tick benchmark ran against P/we, a copy of the world-engine v0.22.0-beta module with TestWorld SnapshotRate set to 1<<30, wired by a temporary replace in the copy's go.mod (restored from go.mod.orig). Two temporary mover.go mutations in the copy (passes=6; the new break condition) ran against all Go tests at 20 and 60 Hz, then were restored and diffed clean. TS probes (vitest, in the copy): render clock under jitter and stalls, EntityViews draw cost and bigint vs number keys, 3v3 msgpack decode, predictor.wasm load, per-call time and GC trace; results in scratchpad probe_clock.txt, probe_views.txt, probe_decode.txt, probe_timing.txt. Wasm size from stripped and unstripped builds written only inside the copy. Machine: i9-11900K, Go 1.27.1, node v24.11.1. After a context reset I re-ran the seam, stop-rule, landing, chain-edge, rate and benchmark probes; numbers matched (Land worst re-ran at 0.36-0.42 ms against 0.75 ms earlier). No game run, no MCP tool, no GitHub. Final check: world-engine and go-ecs git status clean; echo_image still shows its 89 pre-existing changed paths; the only newer file there is .git/FETCH_HEAD (a background fetch, not mine).

- **Tests and tooling:** All work ran in scratch copies: /tmp/claude-1000/-home-doglightning-world-engine/7a3955af-30bb-4356-8a77-cabb94ba360b/scratchpad/tests/echo_image (main) and .../scratchpad/mutcopy/echo_image (mutant re-run), each with go-ecs symlinked beside it. Ran: scripts/check.sh twice (31.1 s cold, 7.4 s warm); go test -json at both rates for per-test times; -coverpkg coverage; gzip -9 sizes; scratch git repos (.../scratchpad/gitprobe, gitprobe2) to measure pack deltas of real and re-recorded files; content probes (move_speed 5.5, bolt speed 16) with -update at both rates, then restored; a badNetwork counter probe; X2 window mutants; a dry run plus full re-run of 138 of the earlier session's 140 mutants (runner asserts a scratchpad root); GOARCH=arm64 go test ./... under this machine's qemu binfmt at both rates; cardinal.RunDST over register (8 seeds); Go fuzz probes (FlyAtView vs Fly 90 s, enqueue 45 s); a 1v1 tick benchmark at both rates; a seeded random soak (10 seeds x 4,000 ticks per rate); a re-record idempotence check. All probe files were deleted from the copy afterward; the copy matches the real tree again. Web: 10 searches and 6 fetches (no GitHub fetch). I changed nothing in /home/doglightning/echo_image, /home/doglightning/world-engine or /home/doglightning/go-ecs: git status still shows the same 89 entries, no file there is newer than my first log, and go-ecs shows no changes. Read-only git commands only; no game, no MCP, no GitHub.

## Appendix D. Skeptic coverage and refuted findings

| Area | Findings (high / medium / low) | Skeptic checked | Outcome |
|---|---|---|---|
| ECS architecture | 6 (0 / 5 / 1) | 0 | - |
| Input and netcode | 8 (1 / 5 / 2) | 1 | netcode-1 downgraded to medium |
| Client prediction and lag compensation | 7 (1 / 4 / 2) | 1 | prediction-1 downgraded to medium |
| Content and rules | 7 (0 / 5 / 2) | 0 | - |
| Physics and client engine | 6 (0 / 3 / 3) | 0 | - |
| Tests and tooling | 9 (0 / 6 / 3) | 0 | - |

- Refuted: none. Only the two high findings went to a skeptic, and both survived as medium.
- Merged: content-1 + ecs-2 (F1), ecs-5 + content-2 (F3), ecs-3 + tests-5 (F5), netcode-5 + tests-9 (F23). prediction-4's glue part joined netcode-4 (F18); its target-shape part is F19.
- 43 reviewer findings became 39: 27 medium, 12 low.
