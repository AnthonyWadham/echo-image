# Echo Image — Game Design Document

> **Living document.** Every section is tagged with its status so we always know what's real:
> **[Locked]** decided · **[Draft]** proposed / likely · **[Open]** undecided, needs a call.
>
> Working title: **Echo Image**. Revision **0.5** · Last updated **2026-10-14**.
> This doc is the *what/why* (design). The *how* (engine, systems, conventions) lives in
> [`CLAUDE.md`](../CLAUDE.md) and the code.

---

## 1. High concept [Locked]

**Echo Image is a top-down hero-shooter PvP arena where your body can't die — but your *Echo* can.**

Your Echo is a ghost that retraces your exact movements about **five seconds behind you**, and it's
the only thing on the field that takes damage. Your body is invincible and useless as a target, so
its job is to **bodyguard your Echo** — physically interposing to block incoming attacks — while you
hunt down the enemy's Echo.

Every step you take is a promise about where your vulnerable self will be five seconds from now.
Pure PvP, no PvE: it's a duel of Echoes. Win by killing theirs before they kill yours.

---

## 2. Design pillars [Draft]

- **Your past is your weakness.** The Echo is *you-five-seconds-ago*. Defending it means owning the
  ground you just walked through.
- **The body is a shield, not a threat.** You can't be killed, so your body's role is to interpose,
  screen, and zone — never to soak HP.
- **You can never stop.** Movement isn't just optimal — it's *mandatory*. Let your Echo catch up to
  you and it damages itself (§3), so you're perpetually leading your own vulnerable ghost through
  space while you fight.
- **One resource, two jobs.** Every move simultaneously *places your Echo* (defense) and *repositions
  your body* (offense/screening). You can't perfectly optimize both — that tension is the game.
- **Your kit is your playstyle.** Whether you're an aggressor or a protector isn't a global rule —
  it's what your champion's kit is built for (§4). The Echo mechanic is the shared language; kits are
  the dialects.

---

## 3. The Echo — core mechanic [Locked]

- Each player controls an **invincible Body**: fully responsive, collides with the world, **cannot
  take damage**.
- Trailing the Body is the **Echo**: a delayed replay of the Body's movement path, **~5s behind**.
- The **Echo carries the player's health.** All damage lands on the Echo; the Body is never a target.
- **Kill the enemy Echo** to score / eliminate. If **your** Echo dies, you're down.
- You defend your Echo by putting your invincible Body between it and incoming fire.

### The Echo can never touch you — the anti-camp rule [Locked]

The Echo **takes damage whenever it overlaps its own Body.** Stand still and your Echo catches up to
where you're standing; the moment it reaches you it starts **damaging itself**. So you can never park
your Echo safely on top of your invincible Body — you must keep leading it through space.

This enforces perpetual motion *organically*: the pressure to keep moving comes from the Echo
threatening itself, not from an artificial leash or rubber-band. It also sets up the core control
problem — you want your body **near** the Echo to defend it, but never **on** it. That narrow gap
between "close enough to guard" and "so close I'm killing myself" is the moment-to-moment skill.

### Why the delay is the whole game (the deep layer) [Draft — design insight]

- **Your Echo's next 5 seconds are already locked.** At any instant, the Echo's path for the coming
  five seconds is fully determined by what you did in the *last* five seconds — it's already
  "recorded." You cannot change where your Echo is about to go; you can only **defend the path you've
  already committed** while **writing a new path** with your movement right now.
- So you're always managing **two time horizons at once**: guarding the Echo you locked in moments
  ago, and authoring the Echo you'll have to guard soon.
- **The same logic is your offense.** An enemy's Echo is equally predetermined by what they already
  did. Read their recent movement and you can predict exactly where their Echo will travel — and set
  up the kill before they can reposition to defend it.

---

## 4. Champions & kits [Draft]

The game is a **hero-shooter**: a roster of **champions**, each with a distinct **kit** — a primary
fire plus a few abilities. The kit (not a global rule) determines whether a champion plays as an
aggressor, protector, zoner, duelist, etc. The Echo mechanic is universal; kits are how different
playstyles express themselves on top of it. Expect familiar role skeletons — **tank / DPS / support**
— re-expressed through Echoes rather than a health bar you defend directly.

The rich design space is how a kit **interacts with the Echo**: shortening/lengthening your delay,
repositioning or briefly protecting your Echo, threatening enemy Echoes at range, or punishing an
opponent whose Echo is committed to a bad path.

### Early champion concepts [Draft — ideation, not designed kits]

Rough seeds to build on later; none are balanced or final:

- **Echo-Swap (trickster / mobility).** Swap places with your Echo. *Flagged complication:* after a
  swap the Echo has to re-sync to the new position history, and it may not be able to "catch up"
  cleanly within the ~5s window — the re-sync rule needs real thought before this works. Parked as an
  idea, not a commitment.
- **Echo-Medic (support).** Sustains its **own** Echo — healing and protecting it while chipping enemy
  Echoes down. (Implies Echoes have an **HP pool**, not one-shot — nudges §9 #2.)
- **Assassin (dive).** Can make its **own Echo temporarily invulnerable**, buying a window to dive in
  aggressively without babysitting the Echo — but must be back in position to protect it the instant
  the window closes. A clean expression of the *one resource, two jobs* pillar: borrow safety now, pay
  for it after.
- **Tank / DPS.** Classic frontline (big screening presence) and damage-dealer roles — detail TBD.

---

## 5. Perspective & genre [Locked]

- **Top-down** camera.
- **MOBA-arena** structure — reference point: *League of Legends* **Arena** (small teams, multiple
  teams, round-based, tight maps).
- **Tactical-shooter / Overwatch** feel — aimed attacks, distinct champion kits, positioning-driven.

---

## 6. Core loop [Draft]

**Moment-to-moment**
1. **Keep moving** — lead your Echo through safe space (never let it catch you), toward a defensible spot.
2. **Screen** — place your invincible body between your Echo and threats.
3. **Read** the enemy: their Echo's path is locked by what they just did — predict where it's headed.
4. **Attack** the enemy Echo (aimed shots / abilities) while defending your own.

**Round-to-round:** [Open] — see §8.

---

## 7. Combat model [Draft]

Moment-to-moment combat is a **top-down shooter with abilities**:

- **Primary fire** — aimed, shooter-style; the main tool for damaging the enemy Echo. [Locked]
  First pass [Draft]: a projectile on a **2 s cooldown**. **Your team sees your cooldown; enemies
  never receive it** (they can still guess it from seeing you shoot). It is the `bolt` ability:
  its cooldown lives in `content/ability/tuning.json` and its shot in `content/projectile/tuning.json`.
- **The shot-vs-block duel is the core exchange.** Both players actively interpose their invincible
  bodies to block incoming shots aimed at their own (or an ally's) Echo, while trying to land theirs.
  Bodies **physically block projectiles** — that interplay is fundamental to the loop. [Locked]
  - A shot is **destroyed by the first enemy Body or wall** it touches. [Draft]
  - Shots **pass through your own team**: your Body, your teammates' Bodies, and allied Echoes, so you
    can shoot past the Echo you're guarding. [Draft]
- **Abilities** — maneuver, utility, and kit identity layered on top of primary fire (§4). [Locked — structure]
- **Echoes** — carry `Health`; the **only valid damage target**, including **self-damage** when
  overlapping their own Body (§3). [Locked]
- **Tuning** — data-driven; balance numbers live in JSON, not code (e.g. `ProjectileTuning`). [Locked — infra]

Still open (§9): whether Echoes are one-shot or an HP pool (the Echo-Medic concept leans toward a
pool), and whether a body also blocks *movement*.

---

## 8. Match structure [Draft]

- **No PvE.** Pure PvP, Echo-vs-Echo. There is no creep/objective layer. [Locked]
- **Modes: small-team.** **1v1** and **2v2** confirmed; **3v3** possible — the exact cap isn't
  critical, the format scales. [Draft]
- **Win condition: eliminate the enemy team.** A player is out for the round when their Echo dies (the
  invincible body has no further purpose once there's nothing to protect); the last team with a living
  Echo wins the round. [Locked]
- **Team play consequence** — in 2v2/3v3 you can body-block for an **ally's** Echo, not just your own.
  Screening for a teammate while leading your own Echo to safety is a natural source of team-mode depth. [Draft]
- **Round format** — likely **best-of-N** (a few rounds per match); the count is undecided. [Draft/Open]
- **Map design** — arena size, cover/obstacles (`Environment` collision category), sightlines,
  symmetry. **Open.**
- **Progression / ranked / meta** — explicitly **deferred**. Out of scope until core gameplay is proven.

---

## 9. Open questions — decision queue [Open]

Ranked by how much they shape the rest. Answer top-down.

1. **Champion kit model.** The shooter-plus-abilities shape is set (§4/§7); still open is the ability
   schema (resources, cooldowns) and turning the §4 seed concepts into real, balanced kits — including
   solving the Echo-Swap re-sync problem.
2. **Echo HP & death.** One-shot-and-down, or an HP pool over several hits? Leaning **pool** (the
   Echo-Medic archetype needs something to heal). Death already means out-for-the-round per §8 — this
   is about how much punishment an Echo absorbs first.
3. **How bodies block.** Bodies blocking projectiles is now core (§7). First pass: a blocked shot dies
   on the body [Draft]; absorb-up-to-a-cap or deflect stay possible later. Still open: whether a body
   also blocks *movement* (can you wall someone off?).
4. **Self-damage tuning.** When the Echo overlaps you, how punishing — a flat tick, or a ramping DoT
   that worsens the longer you overlap? (Balances how hard the anti-camp rule bites.)
5. **Echo delay.** Fixed ~5s for everyone, per-champion, or ability-modifiable?
6. **Round count.** Best-of how many? (Low stakes; easy to tune later.)

---

## 10. Glossary

- **Body** — the player's directly-controlled, invincible avatar. Not a damage target.
- **Echo** — the delayed replay of the Body's path (~5s behind) that carries the player's health.
- **Echo delay** — the lag between Body and Echo (~5s, tuning TBD).
- **Interpose / body-block** — using the invincible Body to physically block attacks aimed at an Echo.
- **Anti-camp rule** — the Echo self-damages while overlapping its own Body, so you can't stand still.
- **Echo path** — the recorded trail the Echo will follow; for the next ~5s it is already locked.
- **Champion / kit** — a playable hero and its ability set; the kit defines the playstyle.

---

## 11. Design → implementation notes [reference]

Light mapping only; detailed tech design stays in code + `CLAUDE.md`.

- **Echo** ≈ a recorded buffer of the Body's `Transform2D` over the last ~5s, replayed onto a separate
  Echo entity that carries `Health` and the damageable collision category.
- **Body** ≈ an entity on its own invincible collision category (never a damage target; may still
  block projectiles via physics).
- **Anti-camp self-damage** maps cleanly onto the existing pipeline: an Echo↔own-Body overlap is just
  another contact that emits a `DamageIntent` targeting the Echo (source = the owner) — no special
  case, same `CollisionSystem → DamageResolveSystem` flow.
- **Collision categories** will likely grow to `Body`, `Echo`, `Projectile`, `Environment` (see
  `internal/physics`).
- Echo state is authoritative server-side and must live in ECS components (snapshot-safe), including
  the position-history buffer that drives the replay.

---

## Changelog

- **0.5** — 2026-10-14 — First pass of primary fire [Draft]: projectiles on a 2 s cooldown; a shot dies
  on the first enemy Body or wall; shots pass through your own team; cooldowns are visible to your
  team only.
- **0.4** — 2026-07-06 — Locked combat as a **top-down hero-shooter** (primary fire + abilities; the
  shot-vs-block duel is the core exchange; bodies physically block projectiles). Added first champion
  seeds — **Echo-Swap** (with its re-sync complication flagged), **Echo-Medic** support, **Assassin**
  echo-invuln dive — plus tank/DPS. Echo-HP now leaning toward a pool.
- **0.3** — 2026-07-06 — Locked match structure: small-team modes (**1v1 / 2v2**, maybe 3v3) and
  **win = eliminate the enemy team** (out for the round when your Echo dies). Noted ally-Echo
  body-blocking as team-mode depth; round count left Draft. Champion kits are now the top open item.
- **0.2** — 2026-07-06 — Locked: pure PvP (no PvE); the **anti-camp rule** (Echo self-damages while
  overlapping its Body); hero-based **champions & kits** as the source of playstyle. Progression/ranked
  explicitly deferred. Reranked the decision queue (team size + win condition now first).
- **0.1** — 2026-07-06 — Initial draft: high concept, pillars, the Echo mechanic and its
  time-horizon depth, genre, core loop.
