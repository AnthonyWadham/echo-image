# Character foundation (proposal)

Status: a proposal from research done on 2026-10-24. Nothing here is built yet. Where this file and the code or
`CLAUDE.md` disagree, the code and `CLAUDE.md` win.

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
5. Timed things store start and end ticks, like `Slot.ReadyTick`. No countdowns.
6. Components hold values and IDs only. Behavior lives in code tables keyed by those IDs. An interface in a component
   loses its type on snapshot restore.
7. An effect takes hold the tick after it is applied. Casts run before movement, so your own dash still moves on the
   cast tick.
8. Code the client predicts is Cardinal-free and deterministic.

## The six foundation pieces

| # | Piece | What | Why |
|---|---|---|---|
| 1 | Tick order | `ability.Equip` and `ability.Cast` move into PreUpdate, before `player.MovementSystem`. A test pins it. | A dash moves on the cast tick. |
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
  st := status.Fold(effects, tick)
  mask := physics.MoveBlockers &^ st.Pass // MoveBlockers = KindWall | KindBounds
  filter := box2d.QueryFilter{CategoryBits: physics.CategoryBody, MaskBits: mask}
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
  deterministic grid search. This covers a phase ending, blinks, and restore lag.
- Maps may have no sealed pockets and no wall deeper than the landing reach. Map tests check both.

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

- Entry: `{Kind, Serial, Credit, Source, Start, End, Stacks}` plus resolved numbers. Serials, because Cardinal reuses
  entity IDs at once with no generation.
- Stacking per kind: `merge` (one per kind), `per_source` (one per kind and source), `instanced` (always new: shields,
  pushes).
- One writer: the effect resolve system at the end of Update. It applies in a fixed order: by target, then cleanses,
  then adds, then crowd control, then by source and kind. Never by emit order, which can change after a restore.
- The fold gives closed flags (stunned, rooted, silenced, unstoppable, invulnerable) and stats in integer basis points.
- Keep hero-only state in that hero's own component, not in the shared flags. Deadlock's state enum filled up with
  hero-only entries.

## Damage and credit

- `AbilityCast{Slot, Tick}`, `DamageIntent{Credit, Cause}`, `Projectile{Credit, Cause}`.
- Resolver stages: immunity, then modifiers, then apply, then deaths last. A `HealIntent` with a MaxHP clamp.

## Prediction

- Shared core: `physics/mover`, `effect/status`, `ability/charges`, `echo/rewind`. None may import Cardinal or a
  component leaf (a leaf pulls in about 737 packages).
- Predicted: your own movement, cast gate, cooldowns and self effects. Not damage, death or other players.
- `YouState` gains your own effect entries and cooldown ticks. Send the entries, never the folded mask.
- Determinism: round every product, no `math.Sin`, `Exp` or `Pow`, integer stats, fixed orders, and an assembly-scan
  test for FMA over the shared packages. The mover gave the same bits on amd64 (v1 and v3), js/wasm and wasip1.

## Cardinal facts that shape this

- The values-only component rule is a convention, not checked by code.
- Snapshots store MessagePack keyed by Go field name. A renamed field comes back as zero, silently.
- Entity IDs are reused at once, smallest first, with no generation.
- Adding or removing a component moves the entity to a new archetype, about 15x the cost of a plain `Set`.
- System events last one tick. A reader registered before the emitter misses them.
- The data plugin ignores unknown JSON keys, and `Validate` sees one kind only. Cross-file checks need a test.
- In a benchmark, one effect list per target needed 3-4x less effect logic than one entity per effect.

## Build order

1. Tick order: casts before movement, pinned by a test. About 15 lines.
2. `effect/status` pure core with unit tests. About 160 lines plus 100 test lines.
3. The effect feature: leaf, `EffectIntent`, kinds tuning with `Validate`, the resolve system, and a generic on-cast
   reactor. Add the list to the Body and Echo constructors. About 300 lines.
4. Movement reads the fold: speed, forced velocity, pass mask, landing, and coasting from the last frame. Walls and
   bounds become kind bits. About 120 lines.
5. Credit and cause on casts, damage and shots. Resolver stages, `HealIntent`, MaxHP clamp, deaths last. About 80
   lines.
6. Champion ID: a `Champion` component, numbers keyed by champion ID, and cooldowns per ability ID. About 100 lines.
7. The FMA scan over the shared packages. About 60 lines.
8. First real user: ship the dash end to end, with tests for distance, wall stop, stun ending it, and determinism.

About 900 lines with tests. Adding a new component needs one `world purge`.

## Open decisions

- How do slows combine? Proposal: strongest only; hastes add.
- When does an applied effect take hold? Proposal: the next tick, with casts before movement.
- Where do champion numbers live: per feature (as today) or in one `kit.json`? Proposal: per feature. One file is worth
  it only if non-programmers will write kits.
- Echo-Swap re-sync and Echo delay changes: proposal is to walk the Echo back at a bounded rate, with no jumps.

## Appendix

- Scratch research and the prototype (temporary, may be deleted):
  `/tmp/claude-1000/-home-doglightning-world-engine/e9ca6229-cb21-4a80-803a-a038807cdeca/scratchpad/charresearch/out/`
  and `.../scratchpad/chardesign/design-ecs-native-features/proto/`. The prototype's tests passed on 2026-10-24.
- Key sources: Valve Dota and Deadlock data (SteamTracking/GameTracking-Deadlock), Riot tech blog (Sylas, Mel,
  "taxonomy of tech debt"), Tim Ford and Dan Reed (GDC 2017, Overwatch), Stallone (GDC 2018, Mortal Kombat rollback),
  Photon Quantum docs, Unreal GAS and Mover docs, box2d.org character mover docs.
