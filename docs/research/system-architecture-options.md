# Behavior Dispatch for Every Echo System: Options Report

## 1. Summary

- **The verdict so far:** today's scatter is a problem of ownership and naming. It is not a snapshot-safety problem. Echo already stores only IDs in state, and the code lives in tables (`projectile/launch_system.go:35`, `player/dash_system.go:22`).
- **The real defect:** shared systems react to content names. Projectile reacts to "bolt" and player reacts to "dash". One ability is spread over 3–4 JSON tables, and nothing checks the keys against each other.
- **Every shipped game studied lands on the same base:** generic executors (mover, effects, damage, projectile), plus a small place for unique code, keyed by ID.
- **The open choice is how variety is organised:** by ability (vertical), by mechanic slot (horizontal), or by a per-kind code table. Section 3 lays out all three.
- **Some seams are missing whichever option wins:** a teleport intent, a Died fact, Cause on EffectIntent, one shared overlap query, and a depth cap on spawned shots.

## 2. Principles for every Echo system

| # | Principle | Shipped evidence |
|---|---|---|
| 1 | **State holds IDs and plain numbers. Code lives in read-only tables built at init.** | Quantum asset_ref: switching on the asset type is "very bad design", and editing an asset at runtime is "NOT DETERMINISTIC". |
| 2 | **Shared executors never branch on a content name.** Projectile, mover, effect and damage code stay generic. | Dota OnProjectileHit sits on the ability. GAS projectiles only carry an effect spec. |
| 3 | **Variety picks from a small, closed set of slots.** A new variant adds one value to one slot. Add a slot value only with its first user. | UE Mover's 4 movement kinds. UE projectile flags (bIsHomingProjectile, bShouldBounce). Brawl Stars has 14 TravelTypes. |
| 4 | **Unique code is normal, so give it one cheap, local home.** When 3 handlers do the same thing, promote it to a slot value. | Deadlock: 564 of 655 modifiers are used once. Hades II has about 295 named functions. |
| 5 | **Snapshot values when the thing is made**, not when it hits. Shots in flight are plain numbers. | Overwatch fixed team switches and damage boost by taking them at fire time (2019). |
| 6 | **One writer per state, through sorted intents.** Systems never call systems. | Overwatch deferment queues (Ford, GDC 2017). Echo's DamageResolveSystem. |
| 7 | **Lasting facts are state with a start tick.** One-tick events carry only facts about that tick. Cues come from state. | VALORANT Effect Containers. Symmetra shots showing at size 0. GAS async cue order. |
| 8 | **Spawn chains are capped from day one**, and predicted spawns get a stable key. | RoR2 ProcChainMask. PoE trigger cooldowns after server crashes. Unity Netcode spawn mismatches. |

## 3. Candidate architectures

All three keep principles 1, 6 and 7. They differ in **where variety lives**.

### A. Ability bundle + generic executors (vertical)

**How it works**
- Each ability ID has one Go file in `ability/`, with three hooks: `OnCast`, `OnHit` and `OnExpire`.
- `OnCast` emits only intents owned by other features: `LaunchIntent`, `EffectIntent`, `TeleportIntent`.
- Shots carry `Cause.Ability`. The projectile executor calls the owning ability's `OnHit`, if it has one.
- This is how Dota, Deadlock, Overwatch, GAS and post-Sylas League do it.

```go
// ability/bolt.go
var bolt = Def{
    OnCast: func(c Cast, out *Out) { out.Launch(Shot{Motion: Linear, Speed: c.T.Speed}) },
}
// ability/registry.go
var defs = map[string]Def{"bolt": bolt, "dash": dash, "echo_swap": swap}
```

| Case | Fit |
|---|---|
| Bolt | `OnCast` emits one linear `LaunchIntent`. |
| Homing | `OnCast` emits `Motion: Tracking`. The projectile owns the tracking step. |
| Bounce | Needs a bounce rule in the projectile. Otherwise `OnHit` re-launches the shot (re-entry). |
| Dash | `OnCast` emits `EffectIntent{forced velocity}`. Player loses `dash_system.go`. |
| Echo-Swap | `OnCast` emits 2 `TeleportIntent`s (Body and Echo). The recast window is state in Abilities. |

**Other systems**
- AI writes InputFrames.
- Pickups and modes get their own "bundle" per kind (pickup ID, mode ID) with the same hook shape.

**Files touched for a new ability:** 1 Go file, 1 JSON row, 1 registry line.

**Risks**
- Some abilities share mechanics, so copy-paste drifts between them.
- One helper reused too widely couples abilities (LoL's cone script fed 293 files).
- Shot rules like bounce still need a home in projectile. Without one, `OnHit` turns into hand-made physics.

### B. Owner-owned mechanic slots (horizontal)

**How it works**
- Each feature owns a few fixed stages. Each stage has an enum and a handler table:
  - **Projectile:** `Motion`, `OnWall`, `OnHit`, `OnEnd`.
  - **Movement:** `Mode`, `Layered`, `Modifier`, `Instant`.
  - **Effect:** kind plus stacking rule.
- An ability is **only a JSON row**. It lists which slot values and params it emits.
- This is how RoR2 projectile parts, Brawl Stars TravelType, UE Mover and Noita do it.

```go
// projectile/component/projectile.go
type Projectile struct {
    Motion, OnWall, OnHit, OnEnd uint8
    BouncesLeft, PierceLeft, Depth uint8
    Target cardinal.EntityID; HitIDs [4]cardinal.EntityID
}
// projectile/flight_system.go
motions[p.Motion](&p, dt); sweep(); onWall[p.OnWall](&p, hit); onHit[p.OnHit](&p, hit, out)
```

| Case | Fit |
|---|---|
| Bolt | `{Linear, Stop, Stop, None}`. Pure data. |
| Homing | `Motion: Tracking` plus a turn-rate param. One handler. |
| Bounce | `OnWall: Bounce`, `BouncesLeft: 3`. One handler. |
| Dash | A `Layered` move, which is today's forced effect entry. Pure data. |
| Echo-Swap | No slot fits. It needs an escape hatch: a named `Instant` handler, or a small reactor. |

**Other systems**
- AI is an InputFrame writer, with behavior slots (target pick, move pick) later.
- Pickups are an overlap trigger plus an `OnPickup` slot that emits intents.
- Modes use slots on a match entity (score rule, end rule).

**Files touched**
- A new ability: JSON only.
- A new slot value: 1 enum value, 1 handler, and an update to Validate.

**Risks**
- Slots interact (homing after a bounce, pierce plus chain). Stage order must be fixed and tested in pairs.
- Enums fill with values used once (Brawl Stars ChargeType went from 7 to 29).
- Unique abilities have no natural home, so this needs a second mechanism anyway.
- Renamed fields restore as zero.

### C. Per-kind code table in each owner (Quantum asset methods)

**How it works**
- Every content kind maps to one pure func set, in the feature that owns the state.
- Examples: `projectile.Kinds["homing_bolt"]`, `movement.Modes["zipline"]`, `ability.Casts["echo_swap"]`.
- One generic system per feature calls the func for the kind stored in the component.
- Funcs import no Cardinal, so wasm can predict them.

```go
// projectile/kinds.go
type Kind struct{ Step func(*Projectile, float64); Hit func(*Projectile, Hit, *Out) bool }
var Kinds = map[string]Kind{"bolt": {Step: linear, Hit: stop}, "bouncer": {Step: linear, Hit: bounce}}
```

| Case | Fit |
|---|---|
| Bolt | `Kinds["bolt"]` = linear + stop. |
| Homing | A new kind entry that reuses the shared `track` and `stop` helpers. |
| Bounce | A new kind entry with a `bounce` hit func. |
| Dash | `ability.Casts["dash"]` emits `EffectIntent`. |
| Echo-Swap | `ability.Casts["echo_swap"]` emits 2 `TeleportIntent`s. |

**Other systems**
- The same shape everywhere: AI `Brains[id]`, pickups `Pickups[id]`, modes `Modes[id]`.

**Files touched for a new kind:** 1 func (or reused helpers), 1 table line, 1 JSON row.

**Risks**
- Combinations are not free. Homing plus bounce is a third kind, unless the helpers compose.
- The helper API can grow into a "code stew".
- The table must match on server and wasm. A boot test must check that every content ID resolves.
- Kind names can drift toward champion names (Quantum's `KnightBasicSkillRD`).

### Side by side

| | A: bundle | B: slots | C: kind table |
|---|---|---|---|
| Unit of variety | Ability | Mechanic value | Content kind |
| Free combinations | No | Yes | Via helpers |
| Unique behavior | Natural | Needs escape hatch | Natural |
| New number-only variant | JSON | JSON | JSON |
| Predictable core | Executors only | All slot handlers | All kind funcs |
| Seen at scale | Dota, Deadlock, OW, GAS | RoR2, Brawl Stars, UE Mover, Noita | Quantum samples |

**A hybrid is possible.** Use B for projectile and movement variety, plus A or C as the one escape hatch. Most shipped games end up there. The next phase should test whether one escape-hatch shape can serve every feature.

## 4. Keep and change

### Keep

| What | Where |
|---|---|
| Intents + one sorted resolver | `combat/damage_system.go`, `effect/resolve_system.go` |
| Effect list + pure fold feeding the one mover | `effect/status/`, `player/movement_system.go:63` |
| MovementSystem as the only pose writer | `player/movement_system.go:32` |
| BodyClaimed fan-out, one constructor per entity | `player/spawn_system.go`, `projectile/launch_system.go` |
| InputFrame as the only control seam, so AI writes frames | `player/component/input.go:10`, `player/input_system.go:22` |
| Boot-time Validate on tuning | `*/tuning.go` |
| Component leaves as the only cross-feature imports | `*/component/` |

### Change

| Change | Where |
|---|---|
| Stop keying reactors by ability ID. Key by shot kind or move kind instead. | `projectile/launch_system.go:34-36`, `player/dash_system.go:21-23` |
| Move dash out of player. Remove the duplicate aim checks. | `player/dash_system.go`, `ability/cast_system.go:45`, `projectile/launch_system.go:37` |
| Turn flight into fixed stages (motion, sweep, wall, hit, end). Today every hit destroys the shot. | `projectile/flight_system.go:61-85` |
| Add kind, counters, Depth, a stable spawn key, and an effect payload to the shot. | `projectile/component/projectile.go:18-20`, `projectile/tuning.go:14-19` |
| Add a kind to ProjectileState so the client can draw shot types. | `projectile/projectile_sync_event.go:16-21` |
| Add a TeleportIntent into the mover. | `player/movement_system.go` |
| Add a Died fact before Destroy. | `combat/damage_system.go:61` |
| Add Cause to EffectIntent. Stop dropping unknown kinds silently. | `effect/component/effect_intent_event.go:14`, `effect/resolve_system.go:82` |
| Check every cross-table key at boot (ability ↔ launcher ↔ dash ↔ effect ↔ champion). | `ability/tuning.go:48` |
| Build one shared overlap query for shots, AoE, pickups and anti-camp. | `projectile/flight_system.go:117`, `combat/anticamp_system.go:61` |
| No ranging over Go maps inside systems. Add a snapshot→restore→step synctest (not built: it needs a restore in Cardinal's TestWorld, F15 in `foundation-review.md`). | `ability/tuning.go:38`, `effect/tuning.go:33`, `main_test.go` |
| Add cast state for charges, cast time and recast windows (needed by Echo-Swap). | `ability/component/abilities.go:16`, `ability/cast_system.go:40-51` |

## 5. Open questions for you

1. **Who writes new content: you in Go, or a designer in JSON?** If it's mostly JSON, that favours B. If it's mostly Go, A or C.
2. **How many abilities and shot types do you expect in a year?** About 10, or 50+?
3. **Must shots combine freely?** For example homing + bounce + split from items or talents. If yes, that needs B's slots, or composing helpers in C.
4. **Will items or talents change abilities?** That decides whether we need "fold modifiers at launch" now.
5. **Which systems does the client predict?** Only the local Body's movement and casts, or shots too? That decides how much code must be Cardinal-free.
6. **Is one escape-hatch shape for all features OK?** For example one `map[ID]Hooks` per owner.
7. **Should we build the missing seams now, or with their first user?** These are the teleport intent, the Died fact, the overlap query and the Mode slot.
