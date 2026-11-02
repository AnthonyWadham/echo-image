# Character foundation

Status: proposed on 2026-10-24 from research. Build steps 1-8 are built (see Build order, which also lists what
was left for later). Where this file and the code or `CLAUDE.md` disagree, the code and `CLAUDE.md` win.

Since then, `system-architecture-design.md` steps 1-9 (built 2026-10-25) changed some built parts: effect flags are
now `interact` tags, the category bits are `interact.Kind`, `team.Owner` replaced `Echo.Owner` and
`Projectile.Credit`, and the dash is the `dash` hook (no `DashSystem`, no `dashes`). This file is updated to match.

## Summary

- Shipped games split a champion three ways: fixed data, small runtime state, and code picked by ID.
- Echo Image's convention (`ability` emits `AbilityCast`, features react by ability ID) is the right skeleton.
- It lacks six shared pieces that are costly to add late. Build them before the second champion.
- The movement rule: an ability never moves a Body itself. It feeds the one mover a forced move, a speed change, or a
  pass (a kind of shape that stops blocking it).

## What shipped games do

These were checked against primary sources (docs, engine source, data-mined files, GDC talks).

- **No game is pure data.** Unique abilities end up as code. Deadlock gives 250 of 263 signature abilities their own
  class. All 43 newer Heroes of the Storm hero mods ship script.
- **The costliest rewrites came from missing core concepts.**
  - League had no "ability" concept, so Sylas forced a refactor of almost every spell.
  - Mel's reflect needed the owner who gets credit split from the source that deals damage.
  - Mortal Kombat X added rollback after launch and spent about 7-8 engineer-years on it.
- **What scaled is the shared plumbing:** one effect mechanism with a declared stacking rule, a small closed list of
  state flags, one combine rule per stat, one damage resolver, and one movement step.
- **Prediction:** predict your own champion (movement, casts, self effects). Never predict damage or death (Overwatch,
  Unreal GAS).
- **Lasting effects travel as state with a start tick,** not as one-shot events. Valorant lost events under fog of war,
  and rampage leaked statuses five times this way.

## Rules for every champion and ability

1. A champion is an ID. Its numbers are rows keyed by that ID.
2. An ability is a reactor keyed by ability ID. Its numbers sit under that ID in the feature it changes.
3. An ability that touches several features is one reactor that emits each owner's intent. Only the owner writes its
   own state.
4. Never branch on a champion or ability name in shared code. Name mechanics (dash, pass), not champions.
5. Timed things store start and end ticks, like `Cooldown.ReadyTick`. No countdowns.
6. Components hold values and IDs only. Behavior lives in code tables keyed by those IDs. An interface in a component
   loses its type on snapshot restore.
7. An effect takes hold the tick after it is applied, so a dash moves from the tick after its cast. Casts run before
   movement, so input, cast and move run in the order the predictor steps them.
8. Code the client predicts is Cardinal-free and deterministic.

## The six foundation pieces

| # | Piece | What | Why |
|---|---|---|---|
| 1 | Tick order | `ability.Equip` and `ability.Cast` run in PreUpdate, before `player.MovementSystem`. | Input, cast and move run in the order the predictor steps them. A teleport can apply on its cast tick. |
| 2 | Effect list | One list per entity (Body, Echo, later summons), always present. A declared stacking rule per kind. | Every buff, stun, shield and pass uses it. |
| 3 | Fold | A pure function turns the list into flags and stats, with fixed combine rules. | Changing a combine rule later is a balance patch (Overwatch armor, 2023). |
| 4 | One movement executor | `MovementSystem` is the only pose writer. Effects feed it. A landing rule handles ending inside a wall. | Rampage's dash went around its mover, ignored stun, and every teleport had to cancel it. |
| 5 | Credit vs cause | Each cast and shot records who gets credit apart from what deals damage. Damage goes through a staged resolver. | Reflects, turrets and lifesteal all need it (League's Mel). |
| 6 | Shared core | Cardinal-free packages, plus a test that blocks FMA. | The predictor runs the same code. Retrofitting this is what cost Mortal Kombat years. |

## Movement abilities

### What blocks a Body

- Map shapes carry their kind as a category bit: wall, bounds, later pit or low wall. A map shape's own mask is all bits.
- Each tick, for each Body:

  ```go
  st := body.Get[effectcomp.Effects]().Status(tick)
  mask := physics.MoveBlockers &^ st.Pass // MoveBlockers = interact.Wall | interact.Bounds
  filter := box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(mask)}
  ```

- The bounds kind has no content name, so no effect can pass it.
- Never change shape bits for a per-Body rule. Never put team in bits.
- Why kinds and not channels: two passes at once combine only with kinds. With channels, a Body with two pass effects
  gets the strictest rule (tested on the real box2d).
- This replaces map plan §6's "CategoryBits = Environment, MaskBits = channels".

### Forced moves (dash, knockback, pull)

- A forced move is an effect entry with a velocity and an end tick. The mover runs it every tick, so walls, passes and
  bounds still apply.
- If any forced move is active, velocity is their sum. Otherwise it is the input times the folded speed.
- A stun or root stops input and ends your own dash. It does not stop pushes from others.
- Unstoppable blocks new pushes and crowd control.
- Coast on the last input frame, not on `Velocity2D`, or a dash leaks into the coast.

### Teleports (blink, Echo-Swap, round spawns)

- A `TeleportIntent` goes to `player`. `MovementSystem` applies it the same tick, through the landing rule.

### Landing rule

- If a Body's center is inside a blocker for this tick's mask, move it to the nearest clear spot with a fixed,
  deterministic grid search. The straight way there must not cross the map edge, which is a thin chain loop with
  nothing solid behind it. This covers a phase ending, blinks, and restore lag.
- Maps may have no sealed pockets and no wall deeper than the landing reach. The arena loader will check both
  when the first map ships (`map-system-findings.md`, F26); no map ships yet, so no test does.

### Examples

| Ability | Content | Code |
|---|---|---|
| Walk through walls for 2 s | An effect with `"pass": ["wall"]` for 2 s | None, once the foundation exists |
| Dash 6 m | A forced velocity for N ticks | A generic dash reactor |
| Knockback from a shot | The shot's on-hit applies a push entry (instanced) | None |
| Blink 5 m | `TeleportIntent` to the aim point | A generic blink reactor |
| Echo-Swap | One reactor: `TeleportIntent` for the Body and `RewindIntent` for the Echo | Its own reactor |
| Slow zone | A zone kind applies a per-source slow; strongest wins | A zone feature |

## Effects

- Entry (built): `{Kind, Tags, Pass, Stat, Value, VelX, VelY, Source, Start, End, Serial}`. It holds the kind's
  resolved rules and the applier's numbers, so the fold needs no catalog. Serials, because Cardinal reuses entity IDs
  at once with no generation. `Credit` and `Stacks` come with their first users.
- Stacking per kind: `merge` (one per kind), `per_source` (one per kind and source), `instanced` (always new: shields,
  pushes). A refresh keeps whichever entry ends later (built), so a short stun never cuts a long one short.
- One writer: the effect resolve system at the end of Update. It applies in a fixed order: by target, then adds, then
  crowd control (so a same-tick Unstoppable blocks it), then by source, kind and numbers. Cleanses go first once they
  exist. Never by emit order, which can change after a restore.
- An entry is dropped once it is over, so the list after a resolve is what applies from the next tick on.
- The fold gives closed tags (`interact.Tags`) and stats in integer basis points. Built tags: stunned, cc,
  ends_on_cc, unstoppable. Rooted, silenced and invulnerable come with the rules that read them.
- Keep hero-only state in that hero's own component, not in the shared tags. Deadlock's state enum filled up with
  hero-only entries.

## Damage and credit

- Built: `Cause{Caster, Ability, Tick}`, which `AbilityCast` embeds. `DamageIntent{Target, Credit, Cause, Amount}`,
  `HealIntent` (same fields), and `Projectile{Cause}` on a shot that also carries `team.Owner`, the Body credited.
- Built resolver: per target, the tick's damage, then its heals. HP is written once, never above MaxHP, and at 0 or
  below the target dies, so HP is never stored below 0. Overkill counts. Intents are summed in sorted order.
- Later, with the first rule that reads them: immunity and modifier stages before damage, and `Slot` on the cast.

## Prediction

- Built 2026-10-25: every input of your own (movement, casts, cooldowns, the dash and your shots), through the
  server's own code in wasm (`player/motion`, `ability/cast`, `projectile/flight`, `predict`,
  `shards/game/predictor`), from `YouState`. See CLAUDE.md, client sync.
- Shared core, built: `physics/mover`, `physics/timestep`, `effect/status`, `interact`, `ability/hooks`,
  `ability/cast`, `projectile/flight`, `player/motion`, `predict`. Planned: `ability/charges`, `echo/rewind`. None may
  import Cardinal or a component leaf (a leaf pulls in about 737 packages).
- Predicted: your own movement, cast gate, cooldowns, self effects, your shots' flight (each cast's hook through the
  server's own `cast.Run`, 2026-11-01), and where they stop on a Body or Echo your screen drew (rebuilt 2026-10-31:
  the server flies your shot at your view, the same way; see `favor-the-shooter.md`). Screen, predictor and server
  read a view by one rule (2026-11-01), pinned by `testdata/view.json`, and the render clock never passes the newest
  tick. Your own Echo too (2026-10-31): body_sync sends its next spots, which the server's ring already holds. Not
  damage, death or other players.
- `YouState` carries your champion, cooldown ticks, effect entries (with `EffectsNext`), shots, and your Echo's delay
  and path. Send the entries, never the folded mask.
- One build (2026-11-01): body_sync's `SimVersion` hashes the sim's Go code, content and modules, and the
  predictor is stamped with the same. A client whose predictor differs doesn't play, so a server-only deploy
  or a `world reload` can't leave it mispredicting every frame.
- An Input carries at most the server queue's frames (2026-11-01, `YouState.InputCap`). A bigger one is dropped
  whole, so one command can't stall a tick with an O(frames²) merge.
- Determinism: round every product, no `math.Sin`, `Exp`, `Pow` or `Hypot` (it fuses on arm64; use `box2d.Length`),
  integer stats, fixed orders, and an assembly-scan test for FMA and inexact `math` calls over the shared packages
  (built: `TestSharedCoreIsDeterministic`). The mover gave the same bits on amd64 (v1 and v3), js/wasm and wasip1.

## Cardinal facts that shape this

- The values-only component rule is a convention, not checked by code.
- Snapshots store MessagePack keyed by Go field name. A renamed field comes back as zero, silently.
- Entity IDs are reused at once, smallest first, with no generation. Syncs carry each entity's `ref.Spawn` serial
  (2026-11-01), which is never reused.
- Adding or removing a component moves the entity to a new archetype, about 15x the cost of a plain `Set`.
- System events last one tick. A reader registered before the emitter misses them.
- The data plugin ignores unknown JSON keys, and `Validate` sees one kind only. Cross-file checks need a test.
- In a benchmark, one effect list per target needed 3-4x less effect logic than one entity per effect.

## Build order

1. **Built.** Tick order: casts before movement. No test pins it yet, since nothing depends on it; the first
   same-tick teleport's test will.
2. **Built.** `effect/status` pure core with unit tests.
3. **Built**, except the generic on-cast reactor. The effect feature: leaf, `EffectIntent`, kinds tuning with
   `Validate`, the resolve system, and the list on the Body and Echo constructors. The reactor waits for its first
   user (an ability that only applies effects), since where its config lives is an open decision below.
4. **Built.** Movement reads the fold: speed, forced velocity, pass mask, landing, and coasting from the last frame.
   Walls and bounds are kind bits.
5. **Built**, except immunity and modifier stages. Credit and cause on casts, damage and shots. The resolver takes
   damage, then heals, and keeps HP between 0 and MaxHP; at 0 the target dies.
6. **Built.** A `Champion` component. Move speed, radius and kits are keyed by champion ID. Cooldowns are per ability
   ID.
7. **Built.** `TestSharedCoreIsDeterministic`: the shared packages import no Cardinal and compile with no FMA on
   arm64, amd64/v3 and wasm.
8. **Built.** The dash: now the `dash` hook, with its numbers in its ability row. A stunned Body can't cast. Tests
   cover distance, wall stop, stun ending it, a stunned cast, and determinism.

Steps 1-8 added `Effects` and `Champion` and changed `Abilities`, so an old snapshot needs one `world purge`.

Left for later, each with its first user:

- Immunity and modifier stages (step 5). The generic on-cast reactor (step 3) is built as `ability.CastHookSystem`.
- Charges, teleports and the Echo rewind.
- A test that pins the tick order (step 1).
- Walls in the predictor (with the arena). Predicted shot hits are built (`favor-the-shooter.md`).

## Open decisions

- How do slows combine? Proposal: strongest only; hastes add. Built this way.
- When does an applied effect take hold? Proposal: the next tick, with casts before movement. Built this way, so a
  dash starts 1 tick (50 ms) after its cast.
- Where do champion numbers live: per feature (as today) or in one `kit.json`? Proposal: per feature. One file is worth
  it only if non-programmers will write kits. Built per feature.
- Echo-Swap re-sync and Echo delay changes: proposal is to walk the Echo back at a bounded rate, with no jumps.

## Appendix

- Scratch research and the prototype (temporary, may be deleted):
  `/tmp/claude-1000/-home-doglightning-world-engine/e9ca6229-cb21-4a80-803a-a038807cdeca/scratchpad/charresearch/out/`
  and `.../scratchpad/chardesign/design-ecs-native-features/proto/`. The prototype's tests passed on 2026-10-24.
- Key sources: Valve Dota and Deadlock data (SteamTracking/GameTracking-Deadlock), Riot tech blog (Sylas, Mel,
  "taxonomy of tech debt"), Tim Ford and Dan Reed (GDC 2017, Overwatch), Stallone (GDC 2018, Mortal Kombat rollback),
  Photon Quantum docs, Unreal GAS and Mover docs, box2d.org character mover docs.
