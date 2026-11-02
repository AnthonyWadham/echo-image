# System architecture design: one rule layer, generic executors, ability hooks

Status: proposed on 2026-10-25 and revised the same day after Anthony's review ("some shots can go through, some
can't"). Migration steps 1-9 ("now") are built; section 12 lists where the build differs from this plan. It builds
on `character-foundation.md` (built) and `system-architecture-options.md` (the research). Where this file and the
code or `CLAUDE.md` disagree, the code and `CLAUDE.md` win.

## 1. Verdict

- **The foundation is one shared rule layer for "what happens when X meets Y".** Shots, areas (anti-camp, pickups,
  zones), movement and damage all use it. A rule is a filter (which things) plus a response (what happens). The first
  rule that matches wins.
- **The GDD's rules are default data, not code.** "Shots pass through your team", "a Body blocks a shot" and "an Echo
  is hurt by its own Body" become default rules. Any shot, champion or pickup can change them in JSON.
- **Each feature that owns state runs one generic executor.** It follows its rules and a few fixed options, and never
  checks a content name.
- **Unique logic has one home: a hook per name, in the pure `ability/hooks` package.** Shared hooks make common
  abilities JSON-only. Passives and items are abilities with no button.
- **Hooks never write state.** They fill a request list, and each owning feature does the change.
- **Timing stays as today.** A cast's shot moves on its cast tick. Effects, teleports and placed things act from the
  next tick.

**Assumptions** (not answered yet; the design works either way): programmers write behavior in Go and numbers live
in JSON; plan for 50+ abilities and 20+ shot types a year; items that change shots come later.

## 2. Rules every system follows

1. **Every "X meets Y" is data:** a filter plus a response, first match wins. The GDD's rules are the default rules.
2. **One owner writes each state. Others send intents.** Every receiver sorts its intents by a full key before it
   acts, because emit order can change after a restore.
3. **Shared executors switch on rules and options, never on content IDs.**
4. **Unique logic gets one hook, with its numbers in JSON under its ability.** Make it a shared hook at the third
   copy (League's shared cone script fed 293 files).
5. **Hooks read and request. They never write.**
6. **Never assume the caster.** Caster, owner and cause are inputs, so an Echo or a stolen ability can cast too.
7. **Emitters register before receivers.** Effects, teleports and placed things act from the next tick.
8. **What a hit uses up is spent the same tick, in sorted order, by the executor that walks the hits.** Two hits in
   one tick never both use the same shield or charge.
9. **Copy at creation.** A shot copies its counts, payload and mods at launch.
10. **Components hold IDs, numbers and append-only enums.** A stored reference to a thing that can die carries a
    generation, `Ref{ID, Serial}`, because Cardinal reuses IDs at once.
11. **Content references are checked.** Names inside a file when it loads. IDs across files in
    `TestContentIDsLineUp`.
12. **Areas test who is inside when they act.** They never trust Begin/End events.
13. **Predicted code is Cardinal-free and FMA-free:** `interact`, `mover`, `status`, `hooks`, the cast gate.
14. **A match resumes only on the build that started it.** A content ID from state that is missing logs an error and
    is skipped, so a tick never panics.

## 3. The rule layer: what happens when X meets Y

A new pure package, `interact`. It is a leaf like `team`, so every feature and the client predictor can use it.

### The words

| Word | What it is | Where it comes from |
|---|---|---|
| **Kind** | What a thing is: `body`, `echo`, `shot`, `wall`, `bounds`, `barrier`, `area` | The Box2D category bits, moved here from `physics/categories.go` with the same values |
| **Tags** | What state a thing is in: `stunned`, `unstoppable`, `invulnerable`, `marked`, `collector`… | Effects grant them, and the effect fold combines them. A closed list in Go, append-only, each with a JSON name |
| **Relation** | How two things relate: `own` (same player), `ally`, `enemy`, `neutral` (the map) | Each side's `Owner` and `Team` |
| **Filter** | Which things a rule applies to: kinds, relations, tags it must have, tags it must not have. An empty field matches anything | Content |
| **Rule** | A filter plus a response. A list is checked top to bottom, and the first match wins | Content |

```go
package interact // pure: no Cardinal

type Kind uint64    // category bits, same values as today
type Tags uint64    // a closed list; some have core meaning
type Relation uint8 // bitmask: Own | Ally | Enemy | Neutral

const (
    Stunned Tags = 1 << iota // the cast gate and the mover read it
    CC                       // Unstoppable blocks it
    EndsOnCC
    Unstoppable
    Invulnerable // the damage resolver skips it
    Marked       // read only by content filters
    // append only: snapshots store the bits
)

type Filter struct {
    Kinds           Kind     // 0 = any
    Relations       Relation // 0 = any
    Require, Forbid Tags
}

func (f Filter) Match(k Kind, r Relation, t Tags) bool

type Rule[R any] struct {
    Filter
    Do R
}

// First returns the first rule a thing matches, and its index, which breaks ties.
func First[R any](rules []Rule[R], k Kind, r Relation, t Tags) (R, int, bool)

// Owner is the player a thing acts for: a Body itself, an Echo's Body, a shot's shooter or reflector.
type Owner struct{ Body uint32 }
```

- **Names become bits when content loads.** `Kind`, `Tags` and `Relation` decode from JSON lists of names, so an
  unknown name fails the boot.
- **One `Owner` for everything.** It replaces `Echo.Owner` and `Projectile.Credit`, so relation is one function for
  every kind. A reflect changes a shot's Owner, so "pass through your own team" follows the reflector.
- **Who can take damage is who has `Health`.** Bodies have none, so the GDD's "only Echoes take damage" is how
  things are built, not a check in code.
- **Tags are for things several features read.** State only one champion uses stays in that champion's own
  component (Deadlock's shared state list filled up with hero-only entries).
- **Cost.** A match is a few integer tests. An executor gathers only the kinds its rules mention, so the Box2D mask
  filters at the source. Shot-vs-shot checks cost only the shots whose rules mention shots.

### Who uses it

| System | Its rules | Default rules |
|---|---|---|
| Shots | `meets`: stop, pass, pierce, bounce, chain, reflect; payload yes or no | Wall or bounds → stop. Own or ally → pass. Enemy Body or barrier → stop. Enemy Echo → payload and stop (GDD §7) |
| Shot targets (homing, chain) | `target`: a filter plus a pick (`nearest`, `lowest_hp`, `owner`) | Homing: nearest enemy Echo |
| Areas (anti-camp, pickups, zones) | `meets`: apply to `self`, `other` or `other_echo`; consume and respawn | Anti-camp: own Body → damage this Echo per second (GDD §3) |
| Movement | `blocked_by`: per champion, with a default in player tuning | Walls and bounds only (GDD §9 #3 is open) |
| Damage | Immunity: the resolver skips `invulnerable` targets, for every source | None |
| Effects | Unstoppable blocks CC (built). A receiver filter per kind, with its first user | — |
| Targeted abilities | A target filter on the ability row, with its first user | — |

- **Defaults sit under each content's own rules.** A shot type's rules are checked first, then the shot defaults, so
  50 shot types don't each copy "walls stop".
- **Ties.** When a shot meets several things at the same point, the earlier rule goes first, then the lower entity
  ID. Today's "a Body beats an Echo" is just the default order.
- **Rules about the actor's own state**, like "while I'm phased, enemy Bodies don't block me", need `SelfRequire` and
  `SelfForbid`. These are added with their first user.

### Examples, all JSON

```json
"shots": {
  "defaults": [
    {"kinds": ["wall", "bounds"], "do": "stop"},
    {"relations": ["own", "ally"], "do": "pass"},
    {"kinds": ["body", "barrier"], "do": "stop"},
    {"kinds": ["echo"], "relations": ["enemy"], "do": "stop", "payload": true}
  ],
  "bolt": {"speed": 18, "damage": 25},
  "lance": {"speed": 24, "damage": 15, "pierces": 2, "meets": [
    {"kinds": ["body"], "do": "pass"},
    {"kinds": ["echo"], "relations": ["enemy"], "do": "pierce", "payload": true}
  ]},
  "ricochet": {"speed": 16, "damage": 20, "bounces": 2, "meets": [
    {"kinds": ["body"], "relations": ["enemy"], "do": "bounce"}
  ]}
}
```

| Case | How it works |
|---|---|
| Some shots pass through Bodies, some don't | `lance` passes Bodies. `bolt` uses the default stop |
| A shot that bounces off enemy Bodies | `ricochet`'s own rule |
| A pickup your Body collects (the default) | The orb's rule: `{"kinds": ["body"], "apply": {"heal": 30, "to": "other_echo"}, "consume": 20}` |
| A champion whose Echo collects | Its passive gives its Echo the tag `collector`. The orb adds a rule for Echoes that have it |
| The Assassin's Echo can't be hurt | Its ability gives its Echo `invulnerable`. The resolver skips the damage from every source |
| A barrier your own team shoots through | A placed `barrier` has an Owner, so the default "own or ally → pass" lets your shots through and stops enemy shots |
| A mark-then-burst combo, with no code | Ability A applies `marked`. Ability B's shot has a rule for `marked` targets with a bigger payload |

## 4. The building blocks

"Built" means it exists today. Everything else is new and comes with its first user, unless step 9 says "now".

### Ability
- **Owns:** `Abilities` (cooldowns built; recast stage new), `Kit{ID}` (what a thing can cast).
- **Executor:** `CastSystem` (the gate, built). It calls a pure `Gate` func, so the predictor runs the same gate.
- **Hook runners:** `CastHookSystem` (OnCast, before LaunchSystem) and `HitHookSystem` (OnHit and procs, after
  FlightSystem). OnCast runs through a pure `cast.Run`, so the predictor runs the same glue (built 2026-11-01).
- **Ability row:** `cooldown_seconds`, `aimed` (built), `hook`, `params`, `recast_seconds`, `button` (false for a
  passive or item), `target` (a filter, for targeted abilities).
- **Passives and items:** an ability with `button: false`. Holding it applies its forever effect (stats, tags, shot
  mods). Its hooks react to facts (OnHit for hits its holder deals; OnKill later).
- **Kit:** each constructor sets `Kit{ID}`. A Body's kit is its champion's. An Echo can get a kit too, for kits built
  around the Echo.

### Projectile
- **Owns:** shots. **Executor:** `FlightSystem` (built), in fixed stages per shot: steer, sweep, what it met, end.
- **Rules:** `meets` (the shot type's, then the defaults), `target` (for homing and chain), `on_end` (none, split,
  return), counts (`bounces`, `pierces`, `chains`), and an on-hit payload (damage plus effects).
- **`on_end` fires whenever a shot ends:** on something it met, or at max range. Its fact is built:
  `ShotEnded` in projectile's leaf, with `flight.End` (where, when in the tick, on what).
- **In:** `LaunchIntent{Shot, Cause, Owner, Dir, Target}`. **Out:** `DamageIntent` (built), `EffectIntent`,
  `ShotHit` (a fact for hooks).

```go
type Projectile struct {
    Type      string // shot type ID: its rules and the client's visuals
    Cause     abilitycomp.Cause
    Velocity  physics2dcomp.Vec2
    Radius, Damage, Remaining float64
    Bounces, Pierces, Chains  uint8   // left
    Mods      [4]string               // shot mods active at launch, sorted
    Target    ref.Ref                 // homing
    Hits      cardinal.Slice[ref.Ref] // pierced or chained, never hit twice
}
// The entity also carries interact.Owner and team.Team.
```

- **The rules are looked up by `Type`, with `Mods` folded in.** Each mod adds counts, payloads or rules in front of
  the type's rules. Rules are content, so a shot stores only what can change per shot.
- **Split children** are built by FlightSystem with the one shot constructor. They inherit `Hits`, so they skip what
  the parent hit, and they fly from the next tick. A split child can't split (`TestContentIDsLineUp` checks it) until
  something needs it; then add an inherited budget, like `bounces`.
- **Chain** is the same shot. It re-aims at the next thing its `target` filter matches that isn't in `Hits`.
- **Return** (boomerang): at max range it homes back on its Owner.

### Movement (player)
- **Owns:** Body pose and velocity, `InputQueue` (built).
- **Executor:** `MovementSystem`. From the effect fold it takes forced velocity, speed, passes and landing (built),
  plus a teleport (`line` or `point`).
- **`blocked_by` rules** decide which other things block a Body. The default is walls and bounds only, which is
  today's behavior. A Body-blocks-Body rule, or a `solid` tag, is one rule away (GDD §9 #3).
- **New:** `InputQueue.Prev` (last frame's buttons), so a recast needs a fresh press.

### Effect
- **Owns:** `Effects`. **Executor:** `ResolveSystem` (both built).
- **Per kind:** tags (today's flags, renamed), passes, stat, stacking (built); `teleport: line | point`; `shot_mod`;
  `forever`.
- **Stats:** `move_speed` (built) and `echo_delay` (new).
- **Teleport and rewind kinds are one-shot.** The resolver sets Ticks = 1, and `Validate` rejects other tags, stats
  or stacking on them. `compare` adds `ToX` and `ToY`, so same-tick teleports resolve in a fixed order.
- **Proc stacks:** a stacking rule "at N stacks, apply X and reset", in the resolver's sorted order.
- **An unknown kind logs an error** instead of vanishing.

### Combat
- **Owns:** `Health`. **Executor:** `DamageResolveSystem`. **In:** `DamageIntent`, `HealIntent` (all built).
- **Immunity stage:** targets tagged `invulnerable` take no damage, from any source.
- **Shields:** a `Shield` next to Health, spent in the resolver in sorted order.
- **`Died{Target, Team, Owner, Cause}`** comes with its first reader (a kill feed, OnKill).

### Echo
- **Owns:** `Echo`, `PathHistory`. **Executor:** `ReplaySystem` (all built).
- **Delay is per champion, and abilities can change it.** The base comes from echo tuning, keyed by champion ID. The
  `echo_delay` stat changes it. When the target delay changes, the Echo moves toward it along its recorded path at a
  bounded rate, so it never jumps. The rate is data, and the history buffer covers the longest delay content allows
  (GDD §9 #5 stays open: all three answers work).
- **Rewind** is a one-shot effect that ReplaySystem reads, used by Echo-Swap. Your client predicts your Echo from
  the ring's next poses (`PathHistory.Path`, sent in body_sync), so a rewind or a delay change must change that path
  the same way: build its rule as shared code, with its first user.
- **`SpawnEchoIntent{Body, X, Y}`** means replace, so there is still one Echo per Body (GDD §3).

### Area (new; replaces `combat.AntiCampSystem`)
- **Owns:** `Area{Rules, Radius, Every, Start, End, ReadyTick}` on any entity: an Echo (anti-camp), a pickup, a zone,
  a barrier.
- **Executor:** `AreaSystem`. Each area whose pulse is due tests who is inside (one Box2D overlap query), finds the
  first matching rule for each thing, and emits `DamageIntent`, `HealIntent`, `EffectIntent` and an `AreaMet` fact.
- **Apply to:** `self` (the area's holder), `other` (the thing inside), or `other_echo` (that player's Echo, since
  Bodies carry no Health).
- **Consume:** a rule can use the area up for N seconds. The area itself sets `ReadyTick`, so a pickup needs no
  feature of its own.
- **Placed things:** `area.PlaceSystem` is the one constructor for zones, barriers and pickups. Abilities place
  them with an `AreaIntent`; the map places pickups. A barrier shots meet carries `TargetHistory`: shots judge
  whatever keeps rows by its rows, whatever its kind, and `TargetSync` sends them. That target record is built
  (2026-11-01); barriers aren't. A row is one circle: a box barrier first gives `flight.Row` a shape, cast by
  that shape (F19 in foundation-review.md; a purge, as `TargetHistory` changes shape).
- **Anti-camp is its first user.** The Echo's constructor gives it an area with the `anticamp` rule. The warmup is
  the area's `Start`. A champion could carry a different rule.

### Game mode (with round play)
- **Owns:** one `Match{Mode, Phase, PhaseEnds, Round, Wins}`. **Executor:** `RoundSystem`, first in Update.
- **Per mode:** `win` (last team with a living Echo, GDD §8), `rounds_to_win` (best-of-N), `countdown_seconds`.
- **Reads:** living Echoes per team (state, not events). **Out:** `SpawnEchoIntent`, spawn teleports, a `frozen`
  effect.

### Ref (new primitive, with its first user)
- A component-only leaf: `Spawn{Serial}` on every entity, stamped by its constructor from one world counter, and
  `Ref{ID, Serial}`.
- A lookup through a Ref treats a serial mismatch as "gone".
- Built 2026-11-01 for syncs, its first user: `ref.Spawn` and `ref.Next` (the counter is saved with the world).
  Every synced state carries `Serial`, and clients and the predictor key what they keep across ticks by it. The
  `Ref{ID, Serial}` struct comes with the first component that stores a reference to a thing that can die.

### Sync
- `netsync` is unchanged. A new feature adds its own `SyncSystem` (areas: `AreaSync`).
- `ProjectileState` gains `Type`. `AbilityState` gains the recast window.

## 5. Ability hooks

```go
// ability/hooks/hooks.go: imports no Cardinal, so a predictor can run hooks.
package hooks

type Vec2 struct{ X, Y float64 }

type Cast struct {
    Caster, Owner uint32
    Stage, Team   uint8 // Stage 1 = recast
    Pos, Aim      Vec2
}

type Hit struct {
    Caster, Owner uint32 // Owner: who holds the shot now (a reflect changes it)
    Target        uint32
    Echo          bool
    Pos, Dir      Vec2
}

// Reader is all a hook may read. Add a method only with its first user.
type Reader interface {
    Pose(id uint32) (Vec2, bool)
    EchoOf(body uint32) (uint32, bool)
    Slot(body uint32, i int) (string, bool) // never the catalog's slice
}

// Out collects requests. The runner adds Cause, Owner, Team and start points. A request type is added only with its
// first user.
type Out struct {
    Launches []Launch // {Shot, Dir, Target}
    Effects  []Effect // {Target, Kind, Seconds, Value, Vel, To}
    Heals    []Heal   // {Target, Amount}
    Areas    []Area   // {Kind, At, Dir}
}

// Def is one hook's code. P is its params type, decoded from JSON at boot.
type Def[P Params] struct {
    OnCast func(c Cast, p P, r Reader, out *Out)
    OnHit  func(h Hit, p P, r Reader, out *Out)
}

// Params validate themselves and list the IDs they name in other files, for TestContentIDsLineUp.
type Params interface {
    Validate() error
    Refs() Refs // {Shots, Effects, Areas}
}

// registry.go: one map. Every ability row names its hook.
var Hooks = map[string]Binder{
    "launch": bind(launch), "dash": bind(dash), "blink": bind(blink), "place": bind(place), "heal": bind(heal),
    "echo_swap": bind(echoSwap), // unique hooks are named after their ability
}
```

- `bind` decodes `params` into `P` (unknown fields rejected) and calls `Validate`. Ability tuning binds every row at
  boot, so a bad hook name or param fails the boot.
- The server `Reader` lives in `ability/reader.go`. Tests pass a fake one.

```go
// ability/hooks/echo_swap_hook.go
type echoSwapParams struct {
    Teleport string `json:"teleport"` // a point teleport kind
    Rewind   string `json:"rewind"`   // a rewind kind
}

var echoSwap = Def[echoSwapParams]{
    OnCast: func(c Cast, p echoSwapParams, r Reader, out *Out) {
        echo, ok := r.EchoOf(c.Caster)
        if !ok {
            return // a caster with no Echo
        }
        to, _ := r.Pose(echo)
        out.Effect(Effect{Target: c.Caster, Kind: p.Teleport, To: to})
        out.Effect(Effect{Target: echo, Kind: p.Rewind})
    },
}
```

| A hook may | A hook may not |
|---|---|
| Read its `Cast` or `Hit`, its params and the `Reader` | Import Cardinal or a component leaf |
| Append to `Out` | Write state or keep package state |
| Call `hooks` helpers (`aimPoint`, `launchAlong`) | Branch on a champion ID or another ability's ID |
| Use FMA-free math (`float64(a*b)`) | Build output by ranging over a map |

**Where hooks run**
- `ability.CastHookSystem` (Update, where `player.DashSystem` sits today): each `AbilityCast` runs its ability's
  `OnCast`.
- `ability.HitHookSystem` (Update, after `projectile.FlightSystem`): each `ShotHit` runs `OnHit` of
  `hit.Cause.Ability`, then the OnHit of passives the involved things hold (attacker, then target), sorted.
- `OnHit` may not launch shots yet. No case needs it. When one does, add a per-hit cap and an inherited budget.
- The runner turns `Out` into each owner's intents. Seconds become ticks here. For OnCast, `cast.Run` hands each
  request to a `cast.Sink`: the server's emits intents, and the predictor's keeps what your client draws.
- A hit reaches its hook through `ShotHit`, which comes from projectile's own leaf and carries `Cause`. Projectile
  never learns that abilities exist.

**Shared hooks, so common abilities are JSON-only**

| Hook | Params | Does |
|---|---|---|
| `launch` | `shot` | one shot along the aim |
| `dash` | `distance`, `seconds`, `effect` | a forced velocity on self |
| `blink` | `distance`, `effect` | a line teleport to the aim point |
| `place` | `area`, `distance` | places a zone, barrier or other area at the aim point |
| `heal` | `amount`, `target` | heals what the ability's target filter picks (for example your own Echo) |

## 6. Tick order

```go
// PreUpdate
player.InputSystem           // frames; BodyClaimed
ability.EquipSystem          // cooldowns on claim
ability.CastSystem           // the gate; AbilityCast
player.MovementSystem        // teleport, then the move, with blocked_by
physicsPlugin                // step
// Init
player.DevSpawnSystem
// Update
mode.RoundSystem             // with round play
echo.SpawnSystem             // on BodyClaimed or SpawnEchoIntent; gives the Echo its areas
echo.ReplaySystem            // record, replay at the current delay, rewind
ability.CastHookSystem       // NOW, in DashSystem's place: OnCast → intents
projectile.LaunchSystem      // every LaunchIntent, sorted; new shots move this tick
projectile.FlightSystem      // meets rules → DamageIntent, EffectIntent, ShotHit; split children fly next tick
ability.HitHookSystem        // with its first user
area.PlaceSystem             // with its first user: AreaIntent → placed things, active next tick
area.AreaSystem              // NOW (anti-camp): pulses → DamageIntent, HealIntent, EffectIntent, AreaMet
combat.DamageResolveSystem   // immunity, damage, heals, deaths
effect.ResolveSystem         // every EffectIntent; applies from next tick
// Sync, last: player, echo, ability, projectile, area, mode
```

- `player.DashSystem` and `combat.AntiCampSystem` are gone. Every receiver sits below all its emitters.
- **A cast's shot** is built at tick T at the caster's position after its move, and moves at T, as today
  (`TestPointBlankShotHitsEcho` keeps passing). **A split child** is built at T and first moves at T+1.
- **Spawn chains are fixed by content.** A split child can't split, chain and return don't spawn, and `OnHit` can't
  launch. Fan-out is at most 8 per cast and 8 per split.

## 7. Test cases

**From the GDD**

| Case | How it works |
|---|---|
| Primary fire `bolt` (§7) | `launch` hook, default shot rules |
| Shots pass through your team, enemy Bodies block them (§7) | The default rules, by relation |
| Ally body-block in 2v2 (§8) | Relation `ally`: your shots pass allies, enemy shots stop on them |
| Anti-camp (§3) | The Echo's area rule: own Body → damage this Echo per second |
| Echo-Swap (§4) | `echo_swap` hook: point teleport for the Body, rewind for the Echo; a recast window |
| Echo-Medic heals its own Echo (§4) | `heal` hook with target `{"kinds": ["echo"], "relations": ["own"]}`, or a healing area |
| Assassin's Echo can't be hurt for a while (§4) | An effect gives its Echo `invulnerable`; the resolver skips it |
| Tank screening (§4) | A bigger radius (champion data), or a placed `barrier` |
| Echo delay per champion or changed by abilities (§9 #5) | Base per champion, the `echo_delay` stat, a bounded re-sync |
| Bodies block movement, or not (§9 #3) | A `blocked_by` rule, per champion or by default |
| Bodies absorb or deflect shots later (§9 #3) | Shot rules on Bodies: `stop`, `bounce` or `reflect` |
| Rounds, best-of-N (§8) | `mode`: last team with a living Echo wins the round |
| Dash | `dash` hook → forced-velocity effect (built today as `DashSystem`) |

**Shot variety (mechanics, not champions)**

| Case | Rules and options |
|---|---|
| Passes through Bodies, hits Echoes | `{"kinds": ["body"], "do": "pass"}` |
| Pierces 2 Echoes | `"pierces": 2`, `"do": "pierce"` on Echoes |
| Bounces off walls, or off enemy Bodies | `"bounces": 3`, `"do": "bounce"` on the kinds you pick |
| Homing | `"motion": "homing"`, `"target"` filter, turn rate |
| Chain to 3 targets | `"chains": 3`, `"do": "chain"`, `"target"` filter |
| Splits into 3 when it ends | `"on_end": "split"`, child shot type, count, spread |
| Boomerang | `"on_end": "return"`: homes back on its Owner |
| Slows on hit | the payload's effects: `[{"kind": "slow", "seconds": 2, "value": -3000}]` |
| Bigger hit on marked targets | a rule for `marked` with its own payload |

**Stress tests** (not planned content; they check that the design stays open)

| Case | How it fits |
|---|---|
| Steal another champion's ability | A unique hook plus a `Grants` state in ability. Never-assume-the-caster makes stolen casts work |
| A barrier that reflects enemy shots | A `reflect` rule on barriers tagged `reflector`: the shot reverses and takes the barrier's Owner |
| Item: your shots bounce once | An item ability whose forever effect names a shot mod (`bounces: +1`), folded in at launch |
| Passive: every 3rd hit stuns | A proc stack, spent in the effect resolver in sorted order |

## 8. Adding a new X

**Shot ability:** add a shot type in `content/projectile` (its own `meets` rules only where it differs from the
defaults); add `{"hook": "launch", "params": {"shot": "<type>"}}`; put its ID in a kit; run `go test ./...`.

**Unique ability:** write `ability/hooks/<id>_hook.go` (params with `Validate` and `Refs`, then `OnCast` and/or
`OnHit`); register it in `hooks.Hooks` under the ID; add its JSON row with `"hook": "<id>"`; unit-test it with a fake
`Reader`, including a cast from a caster it doesn't expect.

**Passive or item:** add an ability row with `"button": false`; give it a forever effect (stats, tags or a shot mod)
and, if it reacts to hits, a hook with `OnHit`; put it in a kit.

**Champion:** add `champions.<id>` in `content/player` (speed, radius, `blocked_by` if not the default); add
`kits.<id>` in `content/ability`; add its Echo delay in `content/echo`; add any new abilities.

**Tag:** append it to the list in `interact` (never reorder) with its JSON name. Grant it with an effect kind, and use
it in filters.

**Shot behavior (a new response or option):** append the enum value and its JSON name; write one stage function in
`flight_system.go`; test it paired with every other response, a reflect and a team change.

**Area (zone, pickup, barrier):** add an area kind in `content/area` with its rules; place it with the `place` hook or
the map; test who it affects and when it rearms.

**Effect kind:** add a row in `content/effect`; add a tag or stat in code only if a central rule reads it.

**Game mode:** add a row in `content/mode`; if no option fits, write `mode/rule/<id>_rule.go`; test a full match.

## 9. Migration from today's code

Each step is small and ships with its test.

**Now: the rule layer, and today's hardcoded rules become data** (all built 2026-10-25; see section 12)
1. Add `interact`: kinds (moved from `physics/categories.go`, same bits), tags (today's effect flags renamed, plus
   `invulnerable`), relation, filter, first match. Effects use `Tags`. Test: filter unit tests; `interact` joins
   `TestSharedCoreIsDeterministic`. Needs a `world purge`.
2. `interact.Owner` replaces `Echo.Owner` and `Projectile.Credit`. Test: today's tests unchanged; relation tests.
3. Shot rules: rename `launchers` to `shots`, add `meets` with the GDD defaults, and add `Type` to `Projectile` and
   `ProjectileState`. FlightSystem asks the rules instead of its hardcoded team, Body and Echo checks
   (`flight_system.go:79`, `:103`, `:109`). Test: today's shot tests pass unchanged, plus a lance and a ricochet; wire
   fixtures on both sides.
4. The `area` feature and `AreaSystem`. Anti-camp becomes the Echo's area rule. Delete `combat.AntiCampSystem`. Test:
   today's anti-camp tests unchanged, plus a champion whose Echo has no anti-camp.
5. Echo delay per champion, in echo tuning (`echo/tuning.go:14`). Test: two champions with different delays.
6. `ability/hooks` with one registry, `CastHookSystem` in `DashSystem`'s place, the `launch` hook and `LaunchIntent`
   (sorted in LaunchSystem). Test: shot tests unchanged; `hooks` joins the determinism test.
7. The `dash` hook. Delete `DashSystem` and `dashes`. Test: dash tests unchanged.
8. Content checks: names fail at load; IDs across files (each hook's `Refs`) in `TestContentIDsLineUp`; an unknown
   effect kind logs an error.
9. An any-caster test: cast every ability from a bare champion.

**With the first user**
10. `Ref` and `Spawn`, with the first stored reference to a thing that can die (a homing target).
11. Bounce, pierce, chain, homing, split and return, one at a time. Test: each paired with the others.
12. The payload's effects, with the first slowing shot.
13. One-shot teleport kinds, with blink. Test: edge clip and landing.
14. The pure `Gate`, then recast, `Prev` and rewind, with Echo-Swap.
15. The `echo_delay` stat and the bounded re-sync, with the first ability that changes the delay.
16. The immunity stage, with the Assassin. The `heal` hook, with the Echo-Medic.
17. `area.PlaceSystem` and the `barrier` kind, with the first zone or barrier.
18. `blocked_by` rules in the mover, with the first rule beyond walls and bounds.
19. `HitHookSystem`, `ShotHit` and `OnHit`, with the first on-hit passive.
20. Passives and items (`button: false`, forever effects, shot mods, proc stacks), with the first item.
21. Pickups (an area that consumes), with the first map that has them.
22. `mode`, with round play.

## 10. Not building now, weak points, choices to check

**Not now, and why**
- `Died`, `OnKill`, and Cause on effects: nothing reads them yet.
- Launches from `OnHit`, and a spawn budget: no case needs them.
- Typed immunity (immune to some sources only), effect receiver filters, and `SelfRequire`/`SelfForbid`: no case
  needs them yet.
- Knockback on shots, charges, channels, ability steals (`Grants`), and bots: no case needs them yet. Bots would only
  write input frames, like a client, so nothing else would change.

**Weak points**
- `Reader` grows one method per need.
- Hook names are strings, so a typo fails at boot, not at compile time.
- Response and option lists will grow (Brawl Stars' ChargeType went from 7 to 29). The rule of three keeps one-offs
  in hooks.
- Rule lists are checked in order, so a rule placed too high can hide the ones below it. Keep the shared defaults
  short, and test each shot type against each kind it can meet.
- Tags are a 64-bit set. If they ever run out, widening it needs a `world purge`.

**Choices I made that you may want to change**
1. **Tags are a closed list in Go.** A new tag is a one-line code change, which keeps tags few and checked when
   content loads. The alternative, tags defined in content, lets designers add tags, but then only a test can catch
   a typo.
2. **First match wins, and a content item's own rules come before the defaults.** Order is the only priority rule.
3. **A changed Echo delay walks to its new value at a bounded rate**, so the Echo never jumps. The rate is data.

## 11. What the reviews changed

Two critics tested the first draft (future cases; simplicity and Cardinal fit). Then Anthony reviewed it. Claims
were checked against the code.

| Change | From |
|---|---|
| Every "X meets Y" rule is data (tags, filters, first match), and the GDD's rules became defaults. The draft fixed one hit rule per shot for every target and decided who is affected inside each system | Anthony |
| Turret, summon and AI test cases removed. Test cases come from the GDD; stress tests are labeled | Anthony |
| Pickups became areas, with no pickup feature. Anti-camp became the first area rule | Anthony's review, follow-up |
| Keep today's shot timing. The draft's +1 tick broke `TestPointBlankShotHitsEcho` (`main_test.go:645`). FlightSystem collects its shots before flying them (`flight_system.go:47-58`), so split children fly next tick | simplicity critic |
| Teleport and rewind kinds are one-shot | simplicity critic |
| Cross-file checks are a test, through each hook's `Refs`. `Validate` and `Resolve` see one file only | simplicity critic |
| Every intent receiver sorts. `Reader` returns one slot. `SpawnEchoIntent` replaces. One hook registry. Flight builds children. The rewind package was deferred | simplicity critic |
| Passives and items are abilities with no button. Shots carry an on-hit payload, and items fold shot mods in at launch | future critic |
| Children inherit `Hits`; chain and return are options on the same shot; `on_end` fires at any end | future critic |
| A pure cast gate. Consumables spent by the executor that walks the hits. Areas test who is inside. `Ref{ID, Serial}`, since Cardinal reuses the smallest free ID at once (`world_state.go:76,128`). Matches resume only on their own build | future critic |

## 12. What building steps 1-9 changed

Built 2026-10-25. Each step ran the full tests plus mutation checks (a deliberate bug per rule, each
caught). The list now lives in `scripts/mutate/mutants.json` (`go run ./scripts/mutate`).

| Plan | Built | Why |
|---|---|---|
| `interact.Owner` | `team.Owner{Body cardinal.EntityID}`, registered by `team`. The pure `interact.Side` and `Relation` stay in `interact` | `interact` can't import Cardinal to register a component, and `team` already holds allegiance |
| Add the `invulnerable` tag in step 1 | Not added | Nothing reads it yet, so it would be a trap. Tags are append only, so step 16 adds it for free |
| Hooks read through `Reader` | No `Reader` yet | No hook reads state. It comes with the first that does (`echo_swap`) |
| The runner turns seconds into ticks | A hook gets `Cast.TickRate` and converts once (`Cast.Ticks`). `Bind` also takes the rate, so params check durations at load | A dash must cover its exact distance in whole ticks |
| `Area{Rules, Radius, Every, Start, End, ReadyTick}` | `Area{Kind, Start}`. The area is its holder's first shape. `Every`, `End` and `ReadyTick` come with their first users | No second radius to keep in sync with the Echo's. Anti-camp now means "the shapes overlap", as the GDD says; for the starter champion that is the old 1.0 m |
| An area finds things with one overlap query | `OverlapAABB` plus an exact circle test. A new `area` kind bit, which Bodies and Echoes mask | The plugin has no circle overlap, and Box2D's query filter is two-way |
| Step 3 tests a lance and a ricochet | A shot that passes Bodies, and a rule that reads a tag | A ricochet needs `bounce`, which is step 11. Responses today: `stop` and `pass`. `pass` with a payload fails the load until there is a hit list |
| `shots.defaults` | Top-level `defaults` next to `shots` | A rule list can't sit in the map of shot types |
| `barrier` in the shot defaults | Left out | The kind doesn't exist yet |
| Step 4: "today's anti-camp tests unchanged" | There were none. Three Echo tests were added before the change (replay, own Body hurts, enemy Body doesn't) and pass unchanged after it | So the refactor had a safety net |
| — | Echo tuning is per champion: `champions.<id>{delay_seconds, area}`. `max_hp` and `hit_radius` stay global | Steps 4 and 5 both key it by champion |
| — | `combat.CombatTuning` and `content/combat` are deleted; anti-camp's rate is in `content/area` | Its only numbers moved to the area |
| — | `main.go` has `registerWith(world, src)`, and tests use `withContent` to patch content | Steps 3-5 need test-only shot types and champions |
| — | `TestAnyCasterCastsEveryAbility` casts each ability from a Body and from an Echo | Step 9 |

Still true from the plan: shot timing is unchanged (`TestPointBlankShotHitsEcho`), and every receiver sorts its
intents. Not done: decoding every tuning file strictly. Field names in tuning JSON (not hook params) are still
unchecked, so a typo like `"cooldown_secnds"` loads as 0. It is a small change per tuning type, worth doing next.

The client change is the `ProjectileState.Type` mirror only. An old snapshot needs one `world purge` (renamed and
removed fields, new masks).

## Appendix

- Today's files the "now" steps touch: `shards/game/main.go`; `internal/physics/categories.go`;
  `internal/effect/status/status.go`, `internal/effect/tuning.go`; `internal/projectile/flight_system.go`,
  `launch_system.go`, `tuning.go`, `component/projectile.go`; `internal/combat/anticamp_system.go`;
  `internal/echo/tuning.go`, `spawn_system.go`, `component/echo.go`; `internal/ability/cast_system.go`;
  `internal/player/dash_system.go`, `tuning.go`; `content/*/tuning.json`.
- Sources: `system-architecture-options.md` and its research (League, Dota, Deadlock, Overwatch, Unreal GAS, Mover and
  collision responses, Photon Quantum, Risk of Rain 2, Enter the Gungeon, Brawl Stars, Source EHANDLE).
- Workflow runs: research `wf_0bd35091-ef2` (about 430k tokens), design and review `wf_114ec4de-cc8` (about 745k
  tokens).
