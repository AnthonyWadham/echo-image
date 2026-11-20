# Event architecture: our ideas tested against the survey

Researched 2026-11-19. Research only: no code changed then. Inputs: [the survey](gameplay-coupling-survey.md),
our ideas (appendix B), World Engine v0.22.3 code and rampage-backend code. The brief and the follow-up
questions from the chat are at the end.

> **These are findings, not the source of truth.** Code and [`CLAUDE.md`](../../CLAUDE.md) win when
> they disagree with this file.

## Status (2026-11-20)

- **Built in echo:** owner queues for damage, heal, effect and launch intents
  ([`owner-queues.md`](owner-queues.md)). The API research is [`queue-api.md`](queue-api.md).
- **Notices** stay Cardinal system events (`Died`, `ShotEnded`).
- **The startup check** isn't in Cardinal yet. Echo checks the same order with a test that reads each
  system's file (`TestSendersRunBeforeTheirReceivers`).
- **Depth and flags on requests were dropped.** Echo has no loop, and rampage's loops branch, so a depth
  cap stops them too late ([`owner-queues.md`](owner-queues.md) §3).
- **One queue per request type**, held by one entity, not one per target entity.

## In plain words

**The problem**
- Many features change the same data, like health.
- Events vanish if the system that reads them runs too early.
- Nothing warns you when that happens.

**The fix, in 3 rules**
1. **One owner.** Only the health feature changes health.
2. **Ask, don't touch.** Other features leave a note like "deal 30 damage to E" in a list that health
   owns. Notes are never lost.
3. **The owner announces.** Health says "E died." A check makes sure every system that listens for
   "died" runs after health.

**Why it helps**
- No more lost events.
- No more "MUST run after" comments.
- A heal and a hit in the same tick are added up before the death check, so a late heal can't miss.

---

Legend: (V) verified = we read the primary source or the code. (S) secondary = summary or wiki. (I) inferred = our reasoning. S[n] = source n in [the survey](gameplay-coupling-survey.md). C[n] = code source below.

## Verdict

- The proposal is mostly right. Parts 1–4 match what shipped games do. Part 5, the test-time check, is too weak to carry the safety story.
- The real problem is not "events vs calls". It is scattered writes plus order that nobody declares.
  - Cardinal drops an event when its reader runs first, and nothing says so. (V) C[1][2]
  - Rampage keeps its order alive with comments like "MUST run after" in main.go. (V) C[5]
- Recommended design, in one line: **owner inboxes for changes, forward-only notices for happenings, and declared event access that Cardinal checks at startup.**
- It replaces three things:
  - Cross-feature writes and command-style events become requests in the owner's inbox component.
  - Ordering comments become declared Reads/Emits, checked when the game starts.
  - Listener-of-listener chains (lifesteal) become one ordered health drain, and real loops move to the next tick.
- Why: requests in a component survive the tick and the snapshot, so they can't be lost. A startup check finds a bad order on every run, not just in tests that happen to hit it.

## Engine facts (checked in code)

| Fact in our ideas (appendix B) | Result |
|---|---|
| PreUpdate, Update, PostUpdate; systems run in registration order | True. (V) C[1] |
| System events cleared at end of every tick (`defer w.systemEvents.clear()`) | True at v0.22.3. (V) C[1] |
| Systems don't declare reads or emits | True. `System` is just `Run(w)`; `RegisterSystem` takes only a hook option. (V) C[2] |
| Snapshots save components, not events | True. A snapshot encodes `world.ToProto()`; events live in a separate manager. (V) C[2][3] |

Extra facts that matter:
- `SystemEvents[T]` yields events "emitted so far in the current tick". A reader never sees later emits. (V) C[2]
- Cardinal already knows which system is running (`runSystem` has its name). So tracking reads and emits per system is cheap. (V) C[2]
- `TestWorld` already taps every emitted event (`Emitted[T]`). (V) C[4]
- Rampage has 132 `EmitSystemEvent` call sites and 57 `SystemEvents[` read sites, outside tests. (V) C[5]
- `ProjectileSpawnRequest` is a multi-tick request: indices, `NextSpawnTick`, `Cancelled`. (V) C[6]
- No identifier named lifesteal exists in `shards/gameplay/internal`. (V) C[5] So Andrew's lifesteal case is a design case, not current code. (I)

## The team's positions

### Andrew
- The diagnosis is right. Bevy has the same failure: unread messages are dropped silently, and fixed-step readers missed events (#7691). (V) S[3][2]
- Request entities like ProjectileSpawnRequest are a known good pattern:
  - Entitas guides use them. (V) S[17]
  - Unreal Mover queues outside influences for the next tick. (V) S[28]
  - Overwatch queues damage and healing. (S) S[45]
- Lifesteal as a listener of a listener is the risky part. It makes a chain inside one tick. The survey's answer is to put it in the owner's math or defer it (see part 4). (I) from S[23][86]

### Anthony
- "Events and intents are the basis of ECS": half right.
  - **Intents (requests):** widely used. (V) S[28][127][116]
  - **Events:** shipped engines split on them.
    - Quantum lets events only carry data out to the view. (V) S[98]
    - Flecs says to prefer a system. (V) S[8]
    - Bitsquid prefers polling when few objects are watched. (V) S[41]
    - Space Station 14 bans "method events". (V) S[128]
- "Half events, half calls makes a mess": the mess is many writers, not the mix. A heal from one package and damage from another is a two-writer race. One owner applying both together removes it. (I) from S[92][86]
- "All listeners in PostUpdate; listeners never send events": breaks in two places.
  - A listener that must cause an effect (a lifesteal heal) has no legal way to do it. An inbox fixes that: the listener appends a request instead of sending an event. (I)
  - It forbids notice chains, such as death to loot. In the design below only owners emit notices, so this goes away. (I)
- "Linters and tests so no one can break the order": right goal. Tests alone only catch paths a test happens to run. Declarations plus a startup check catch every run. (I) from S[116][5]

### Hartanto
- Feature-first ownership and read-only leaf helpers match the survey.
  - Flecs splits each feature into a components module and a systems module. (V) S[9]
  - Bitsquid moves data through a layer above the systems. (V) S[41]
  - Space Station 14 enforces owner-only writes. (V) S[128]
- The "next hub" warning is real. A health owner that absorbs every modifier becomes a god system. (I)
- The tree breaks in three branches:
  - **"Read and predict, then send an event":** the prediction can be wrong. A shield or immunity applied by the owner changes the result. An on-kill reward then fires for a kill that never happened. GAS and MTG put such modifiers inside one pipeline for this reason. (V) S[23][86] (I) for the failure
  - **"Owner copies events into its own queue component":** the copy system is itself an event reader. If it runs before the sender, the event is still lost. Appending straight to the owner's queue has no order to get wrong. (I) from C[2]
  - **"Fix the order in main.go, or the event is silently lost":** this branch is the bug class. It should be checked by the engine, not left to a person. (I)

## The proposal, part by part

| Part | Verdict | Why |
|---|---|---|
| 1. Owner-drained queue components | Keep | Overwatch's queue (S) S[45]; Mover (V) S[28]; SpatialOS commands to the owner (V) S[127]; Encompass's merging system (V) S[116]. A component survives the tick and the snapshot (V) C[3]. |
| 2. One-tick events only flow forward | Keep, but enforce | Forward-only is right. Today nothing enforces it; order lives in comments. (V) C[5] |
| 3. Loops go to the next tick, with caps | Keep, add cause flags | Next-tick hops are used by Factorio circuits and GameCode4. (V) S[78][35] Caps: Doom 3, Source, TrinityCore. (V) S[34][33][90] |
| 4. Lifesteal and shields in the owner's math | Keep, with two passes | Shields are interception, like GAS and MTG replacement. (V) S[23][86] Lifesteal needs a "can't trigger" flag. (V) S[90][54] |
| 5. Test check: read, then more sent later | Replace | Runtime-only, so it depends on test coverage, and it misfires on types with several readers. (I) |

### How each part breaks
- **Part 1**
  - Appending to another feature's queue is a write. Hartanto's leaf rule forbids writes. The owner must publish an explicit append-only API. (I)
  - Latency depends on where the producer runs. A producer before the drain lands this tick; one after lands next tick. That is deterministic, but it must be a known rule. (I)
- **Part 2:** "forward" is defined by registration order, which nobody declares. A move in main.go silently turns forward into backward. (V) C[1][5]
- **Part 3**
  - A per-chain cap needs the chain's depth carried in each request. A global per-tick cap drops whichever effects come last in order, which is itself an order bug. (I)
  - League carries the cause (cast ID, slot) through every effect for exactly this kind of rule. (V) S[48]
  - Caps must be loud: TrinityCore and Doom 3 log or abort; they don't drop silently. (V) S[90][34]
- **Part 4**
  - Each new modifier (crit, reduction, thorns, execute) wants into the owner's code. Features should contribute numbers, as stat components, not code. Dota's modifier value hooks and GAS modifiers work this way. (V) S[54][23]
  - Thorns is a loop. It needs the reflection flag: reflected damage can't be reflected. Dota has such a flag. (V) S[54] A wiki says it stops two Blade Mails reflecting forever. (S) S[56]
- **Part 5**
  - It only sees ticks where the late send actually happens in a test. (I)
  - Legit multi-reader types (an early reader and a late reader) trigger it. (I)
  - It can't say which order was intended. Encompass gets intent from declared sends and receives, and derives the order. (V) S[116]

## Answers to the questions

1. **Event lifetime.**
   - Keep one tick for notices. They are cleared before the snapshot, so nothing is half saved. (V) C[1][3]
   - Use inbox components for anything that must be handled. They last until drained.
   - Skip a double buffer. Bevy's only turns a loss into a one-tick delay, then drops after two. (V) S[3] It would also need saving in snapshots. (I)
   - Dropped events become impossible by declaration (Q8). (I)
2. **Declaring and enforcing order.**
   - Encompass derives the order from declared sends and reads, and forbids cycles. (V) S[116]
   - Bevy flags unordered conflicting systems and has a shuffle seed. (V) S[5][2]
   - Flecs inserts sync points from declared writes. (V) S[7]
   - None of these catch an explicitly wrong order except Encompass, by construction. (I)
3. **Loops.**
   - Same tick only for effects flagged "can't trigger" (lifesteal, thorns). TrinityCore's triggered spells can't proc; Dota flags reflection. (V) S[90][54]
   - Everything else goes to the next tick, with depth carried in the request. MTG also limits each replacement effect to once per event. (V) S[86]
   - Cap per chain; log or panic when the cap hits. (I)
4. **Order inside one tick.** One health drain:
   - Collect all damage and heals.
   - Apply modifiers and shields.
   - Apply the net change at once. Hearthstone moved to all-at-once healing; MTG applies results together, then checks. (S) S[92] (V) S[86]
   - Second pass for flagged lifesteal and thorns.
   - Death as a state check (`hp <= 0 && !dead`), like Carmack's one-point check and MTG's state-based actions. (V) S[118][86]
   - Emit a Died notice forward.
5. **Events vs request entities vs queue components.**
   - Notice event: something happened; readers later this tick only.
   - Queue (inbox) component: a change to another owner's data.
   - Request entity: work that spans ticks and has its own progress, like ProjectileSpawnRequest. (V) C[6] (I) for the split
6. **Direct calls are still fine:**
   - Inside your own feature.
   - Read-only leaf helpers.
   - Pure libraries.
   - The owner's append-only inbox API.
   - Not fine: a call that writes another feature's components. (V) S[128] (I)
7. **Enforcement in Go.**
   - A startup check from declarations: panic at StartGame. (I)
   - A runtime honesty check: an undeclared read or emit panics in tests. Cardinal knows the running system, and TestWorld already taps emits. (V) C[2][4]
   - go/analysis rules:
     - No feature imports another feature's systems package.
     - Set only on your own feature's components (like Space Station 14's RS2008 analyzer).
     - EmitSystemEvent only of your own event types. (V) S[128] (I)
   - A snapshot round-trip test, like Space Station 14's save-load-save test and Factorio's heavy mode. (V) S[129][79]
8. **Smallest World Engine change.**
   - Loud, with no API change: track reads per system per tick. If an emit of T comes after any system already read T this tick, panic in tests and log an error with a metric in production. (I) from C[2]
   - Impossible, with a small API change: options like `cardinal.Reads[T]()` and `cardinal.Emits[T]()` on `RegisterSystem` (it already takes options). StartGame then checks that every emitter of T runs before every reader of T. (V) C[2] for the option hook; (I) for the design

## Recommended design for our code

**Name:** owner inboxes + forward-only notices, checked at startup.

1. **Ownership.**
   - Each component has one owning feature. Only the owner calls Set on it.
   - Others read it through the owner's leaf helpers. This is Hartanto's leaf rule, kept. (V) S[128] (I)
2. **Inboxes.**
   - To change another feature's data, append a request to the owner's inbox component, through the owner's append-only API.
   - The owner drains its inbox at one fixed point.
   - Requests carry the cause (source entity, cause ID, depth, flags). (V) S[48] (I)
3. **Notices.**
   - Only an owner emits a notice, after it changes its own state ("died", "spawned").
   - Readers run later in the tick and never emit the same type. Every emit and read is declared. (I) from S[116]
4. **Health drain.** The order in answer 4, with modifiers from stat components and a second pass for flagged secondary effects. (I)
5. **Loops.** On-death effects enqueue into inboxes and land next tick, with depth + 1. A cap logs and stops the chain. (V) S[34][90] (I)
6. **Checks.**
   - Startup order check from declarations.
   - Runtime honesty check in tests.
   - go/analysis ownership and import rules.
   - Snapshot round-trip test. (I)

**What it replaces:**
- Direct Set calls into other features.
- Command-style system events.
- "MUST run after" comments.
- The proposed part 5 test.
- Branches of the decision tree: "read and predict", "copy events into a queue" and "fix the order in main.go".

**Why this one:**
- Requests in components can't be lost and are in every snapshot. (V) C[3]
- One writer per state removes heal/damage races. (I) from S[92][128]
- Declared order is checked on every start, not only on tested paths. (I) from S[116]

**Cost:** about 190 call sites to declare (132 emits, 57 reads). The runtime honesty check finds missed ones. (V) C[5] (I)

## Not found or unverified
- **Survey gaps (research stopped at the usage limit):**
  - Overwatch is still (S) only, through one translation. The queue pattern rests on other (V) sources too (Mover, SpatialOS, Encompass, Space Station 14).
  - Not covered: StarCraft II effect trees, actor servers, Insomniac/Halo/Destiny talks, blackboards.
- **Not read:** the dependency-map artifact (claude.ai) and echo_image's `system-architecture-options.md`. No claim here relies on them.
- **No source measured:**
  - the cost of declared event access in a Go ECS;
  - the cost of a per-system read tracker at 20 Hz.
- **Not tested:** none of the proposed checks were built or run.
- **Not reviewed:** this report and the edited SURVEY.md had no independent review pass.
- **Lifesteal:** no identifier with that name was found in rampage. Andrew's example is treated as a design case.

## Sources
### Code (read 2026-11-19)
- C[1] World Engine v0.22.3, `pkg/cardinal/internal/ecs/world.go` (Tick, systemEvents.clear) — the world-engine repo (`git show v0.22.3:...`)
- C[2] World Engine `pkg/cardinal/world.go` (SystemEvents, EmitSystemEvent, RegisterSystem, runSystem) and `pkg/cardinal/system.go`
- C[3] World Engine `pkg/cardinal/world.go` encodeSnapshot (`world.ToProto()`), and `internal/ecs/system_event.go`
- C[4] World Engine `pkg/cardinal/testworld.go` (`Emitted[T]`, systemEventTap)
- C[5] rampage-backend `shards/gameplay/main.go` (order comments) and grep counts over `shards/gameplay` (non-test)
- C[6] rampage-backend `shards/gameplay/internal/projectile/components/projectile_spawn_request.go`

### Survey sources cited (full list in [the survey](gameplay-coupling-survey.md))
- S[2] Bevy issues #4377, #7691, #19569 — https://github.com/bevyengine/bevy/issues/7691
- S[3] Bevy Messages docs — https://docs.rs/bevy/0.20.1/bevy/ecs/message/struct.Messages.html
- S[5] Bevy scheduling source — https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/schedule.rs
- S[7] Flecs Systems manual — https://github.com/SanderMertens/flecs/blob/master/docs/Systems.md
- S[8] Flecs Observers manual — https://github.com/SanderMertens/flecs/blob/master/docs/ObserversManual.md
- S[9] Designing with Flecs — https://github.com/SanderMertens/flecs/blob/master/docs/DesignWithFlecs.md
- S[17] How I build games with Entitas — https://github.com/sschmid/Entitas/wiki/How-I-build-games-with-Entitas-(FNGGames)
- S[23] Unreal GAS docs — https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-attributes-and-attribute-sets-for-the-gameplay-ability-system-in-unreal-engine
- S[28] Unreal Mover vs CMC — https://dev.epicgames.com/documentation/en-us/unreal-engine/comparing-mover-and-character-movement-component-in-unreal-engine
- S[33] Source SDK physics.cpp — https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/physics.cpp
- S[34] DOOM 3 Event.cpp — https://raw.githubusercontent.com/id-Software/DOOM-3/master/neo/game/gamesys/Event.cpp
- S[35] GameCode4 EventManager — https://raw.githubusercontent.com/MikeMcShaffry/gamecode4/master/Source/GCC4/EventManager/EventManagerImpl.cpp
- S[41] Bitsquid, Managing Coupling — https://bitsquid.blogspot.com/2011/01/managing-coupling.html
- S[45] Ford, Overwatch GDC 2017 (Chinese translation) — https://www.sohu.com/a/148848770_466876
- S[48] Riot, Getting Into the Guts of Berserk — https://technology.riotgames.com/news/getting-guts-berserk
- S[54] Dota 2 VScript API dump — https://github.com/ModDota/API/blob/master/dump/script_help2.lua
- S[56] Dota 2 Wiki, Damage types — https://dota2.fandom.com/wiki/Damage_types
- S[78] Factorio FFF #204 — https://factorio.com/blog/post/fff-204
- S[79] Factorio FFF #315 — https://www.factorio.com/blog/post/fff-315
- S[86] MTG Comprehensive Rules — https://media.wizards.com/2026/downloads/MagicCompRules%2020261106.txt
- S[90] TrinityCore Unit.cpp — https://github.com/TrinityCore/TrinityCore/blob/c4192c00729bfb1edf9d68dcb88e0a33b4c53e33/src/server/game/Entities/Unit/Unit.cpp#L5606-L5611
- S[92] Hearthstone Advanced rulebook — https://hearthstone.wiki.gg/wiki/Advanced_rulebook
- S[98] Quantum 3, Events & Callbacks — https://doc.photonengine.com/quantum/current/manual/quantum-ecs/game-events
- S[116] Encompass docs (MECS) — https://gitea.moonside.games/MoonsideGames/encompass-cs-docs/src/branch/main/content/why/architecture/mecs.md
- S[118] Carmack on inlined code — http://number-none.com/blow/john_carmack_on_inlined_code.html
- S[127] SpatialOS authority and commands — https://web.archive.org/web/20210928052112/https://documentation.improbable.io/spatialos-overview/docs/commands
- S[128] Space Station 14 conventions — https://docs.spacestation14.com/en/general-development/codebase-info/conventions.html
- S[129] Space Station 14 SaveLoadSaveTest — https://github.com/space-wizards/space-station-14/blob/master/Content.IntegrationTests/Tests/SaveLoadSaveTest.cs

---

## The design, explained (chat, 2026-11-19)

This is the design as recommended that day. Echo later dropped depth and flags (see Status).

- Every component has one owner feature. Only the owner changes it.
- Other features read it through the owner's read-only helpers.
- To change it, another feature leaves a request in the owner's **inbox**.
- After a change, the owner sends a **notice** saying what happened. Cardinal checks at startup that every
  system reading the notice runs after the owner.

### Two ways to affect another feature

| | Inbox request | Notice |
|---|---|---|
| Means | "Please change this" | "This happened" |
| Example | 30 damage to enemy E | Enemy E died |
| Who sends it | Any feature, through the owner's "add request" function | Only the owner |
| Who reads it | Only the owner | Any system that runs after the owner |
| How long it lasts | Until the owner handles it. It's a component, so it's saved in snapshots. | One tick |
| Sent after the reader already ran | Waits for the next tick | Lost. The startup check makes this order impossible. |

### One tick, step by step

Example: a player P with lifesteal hits enemy E, which explodes when it dies.

**Tick 10**
1. **Combat** runs before Health. It adds "30 damage to E, from P, cause: sword, depth 0" to Health's inbox.
2. **Health** empties its inbox in a fixed order:
   1. Collect all damage and heals for each target.
   2. Apply armor and shields. E's armor blocks 6, so 24 lands. Armor is a number in a stat component, so
      the armor feature never touches Health's code.
   3. Apply the net change once. E goes from 20 HP to −4.
   4. Second pass: P's 25% lifesteal heals P by 6. This heal is marked *secondary*, so it can't trigger
      more lifesteal or thorns.
   5. Death check: E has HP ≤ 0 and isn't dead yet, so E dies.
   6. Send the notice "E died, killed by P".
3. **Loot** runs after Health. It reads "died" and drops loot.
4. **Explosion** runs after Health. It reads "died" and finds nearby units with a Box2D overlap. It adds
   damage requests to Health's inbox with depth 1. Health already ran this tick, so these wait for tick 11.

Suppose another feature had also queued a +10 heal for E this tick. It would be added in step 1, before
the death check, and E would live on 6 HP. That removes the heal-vs-damage race in Anthony's example.

**Tick 11**
- Health applies the explosion damage. If another exploder dies, its blast goes in at depth 2, and so on.
- At the depth cap (say 8), the chain stops and logs an error. It can't loop forever, and it never stops
  silently.

### What a request carries

| Field | Why |
|---|---|
| Source | Who caused it. Lifesteal heals them, and they get the kill. |
| Cause | Which attack or ability. This allows rules like "once per cast". |
| Depth | Steps since the first hit. Caps the chain. |
| Flags | "Secondary" marks lifesteal and thorns, so they can't trigger more effects. |

### How mistakes are caught

Each system declares the events it reads and sends:

```go
// Proposed API: Reads and Emits don't exist in Cardinal yet.
world.RegisterSystem(&health.DrainSystem{}, cardinal.Emits[health.Died]())
world.RegisterSystem(&loot.DropSystem{}, cardinal.Reads[health.Died]())
```

If someone moves the loot line above the health line, the game won't start, and the panic names both
systems.

| Check | What it catches | When it runs |
|---|---|---|
| Startup order check | A reader placed before its sender | Every start |
| Honesty check | A system reads or sends an event it didn't declare, so the declarations stay true | Tests |
| Linter | Set on another feature's component, importing another feature's systems, or sending another feature's event | Lint |
| Snapshot round trip | State kept outside components, which would be lost on restore. The test saves, loads, saves again and compares. | Tests |

### What it replaces

| Today | Instead |
|---|---|
| Calling Set on another feature's component | Add a request to that feature's inbox |
| An event used as an order ("heal X") | A request in the owner's inbox |
| "MUST run after" comments in main.go | Declarations plus the startup check |
| Part 5's test check | The startup check, which catches every bad order, not just ones a test happens to hit |
| Decision tree: "read and predict" | Send the request, then react to the owner's notice. You can't predict the armor and shields the owner applies. |
| Decision tree: "copy events into a queue" | Add to the inbox directly. A copier is just another reader that can run too early. |
| Decision tree: "fix the order in main.go" | The startup check does it for you |

The report left two things open: whether each feature has one inbox or each entity has its own, and the
depth cap value. Echo picked one inbox per request type, and no depth cap.

## Questions from the chat (2026-11-19)

### Is the request list a component?

Yes. The list is a component that the health feature owns.

- Because it's a component, it's saved in snapshots and lasts until health empties it.
- Other features only add notes to it. Health is the only one that reads it and clears it.

### Why a list and not events? Aren't events the common way in ECS?

Events are common in ECS. But the big engines mostly use them for "this happened" news, not for "please
change this" requests.

**Why a list beats an event for requests:**
- **An event lasts one tick.** If health runs before the sender, the damage is lost and nothing warns you.
  A note in the list waits until health reads it.
- **Events aren't saved in snapshots.** The list is a component, so a restore keeps it.
- **One reader.** Only health reads the list, so all hits and heals are added up together.

**What other engines do:**
- **Overwatch:** damage goes into a queue component that the health system empties.
- **Bevy:** its events drop silently if no one reads them in time. That's the same bug Cardinal has.
- **Flecs:** its docs say to use a system rather than an observer when a system can do the job.
- **Quantum:** events only send news out to the game client. They never change game state.

So keep events, but only for news like "E died". Use the list when you want another feature's data to
change.

### Which shipped games use events?

**Event hooks on buffs and items**
- **League of Legends:** buffs react to hooks like "on hit".
- **Dota 2:** modifiers react to attacks and damage.
- **Caves of Qud:** each object is built from parts that talk through events.
- **Dungeon Siege:** components send each other messages.

**Event queues**
- **Doom 3:** events are queued, capped at 4,096 per frame.
- **Half-Life 2 and other Source games:** map objects fire outputs into other objects' inputs.

**Rules engines with a queue of effects**
- **Hearthstone, Slay the Spire, Legends of Runeterra, and MTG Arena (Magic: The Gathering).**

**Events set up in game data**
- **Thief:** designers wire objects together.
- **Crusader Kings 3:** game data hooks onto named events.
- **Factorio:** mods react to game events.

So events are common in shipped games. The difference is how they use them:
- Most use events as hooks on a buff or an object.
- The ones with long chains add hard limits, like Doom 3's cap.
- None of them rely on events surviving a tick when the order is wrong. That's the bug Cardinal has today.

The full list with links is in [the survey](gameplay-coupling-survey.md), in the "Approaches at a glance"
table.

### How do Dota and League do it? Are they ECS?

No. Neither is ECS. They're object-based engines with scripts.
- **Dota 2** runs on Source 2. Heroes are objects with modifiers attached.
- **League** runs on Riot's own engine. Champions are objects with buff scripts.

**How their events work**
- In both, "events" are hooks that run straight away, inside the same call.
- When a hit lands, the damage code calls every hook on the attacker and the target right then: on hit,
  on take damage, lifesteal, thorns.
- Nothing waits for another system, so there's no order to get wrong.

**How they stop loops**
- **Dota:** damage carries a reflection flag, so reflected damage can't be reflected again.
- **League:** each effect carries its cause, such as which cast and which ability slot. Rules check that
  cause.

**Why we can't copy this as-is**
- Cardinal events wait for a later system to read them. Dota and League hooks run instantly.
- So their design never has the lost-event bug: an event lost because the reader ran first.

The closest ECS version of their idea is the single health list. All the hooks run in one place, in one
fixed order.

**How sure:**
- Checked in their sources: the Dota hook names and flags, and League carrying the cause. The sources are
  Dota's script API list and Riot's tech blog.
- Not confirmed: that their damage code calls the hooks directly.

### What about ECS engines like World Engine?

| Engine | How it's like ours | How systems affect each other |
|---|---|---|
| **Photon Quantum** | Fixed tick, server or rollback, snapshots | Events only send news to the game client. To change state, a system calls another system directly in the same tick ("signals"), or edits component data. |
| **Space Station 14** | Server-authoritative ECS game, many contributors | Uses events a lot, but they run instantly. A lint tool blocks writes to another system's data. |
| **Overwatch** | ECS game, server-authoritative | Damage and heals go into a queue component that the health system empties. |
| **Bevy** | General ECS | Has two kinds. Immediate observers. Buffered messages that get dropped if no one reads them in time, which is our bug. |
| **Flecs** | General ECS | Immediate observers, but the docs say to use a system when you can. |
| **Encompass** | Small ECS | One-tick messages like ours. Each system declares what it sends and reads, and the engine works out the order, so nothing is lost. |
| **Ark, Donburi** | Go ECS libraries | Ark has immediate observers. Donburi has event queues that you empty yourself. |

**The pattern.** These engines avoid the lost-event bug in one of three ways:
- Events run instantly, as in Space Station 14 and the Bevy and Flecs observers.
- Changes go through queues or data, as in Overwatch and Quantum.
- The engine checks the order from declarations, as in Encompass.

Cardinal does none of these. It has one-tick events and an order nobody checks, so Bevy's buffered
messages are its closest match. Bevy has the same lost-event bug.

**How sure:**
- Checked in each engine's own docs or code: Quantum, Bevy, Flecs, Encompass and Ark.
- Overwatch rests on one Chinese translation of the GDC talk.
- For Space Station 14, the lint rule was checked. That its events run instantly is from memory.

### Quantum changes state directly. Why can't we?

Calling another feature directly is what causes the import cycles.

Quantum avoids the cycle like this:
- A signal is declared in one shared place, like "OnDamage".
- Any system can listen to it. The sender just calls OnDamage, so it never imports the listener.
- In Go, that's an interface in a shared package, with the owner plugging itself in at startup. That's the
  interfaces option Hartanto calls a last resort.

The list does the same job with no interface:
- The list type and its "add request" function live in the owner's components package. That package
  imports nothing from other features.
- Damage code imports only that package and adds "30 damage to E".
- Health's systems read the list. Nobody imports health's systems, so there's no cycle.

One rule makes this work: the "add request" function must live in the components package, never in the
systems package. If it lives in systems, the cycle comes back. (Echo's `pkg/queue` goes one step further:
`queue.Send` lives in a shared library, so the owner's leaf holds only the request type.)

## Appendix A: the brief

```markdown
# Research: how gameplay systems affect each other in a game server

Research only. Don't edit or build anything in any repo.

## Our situation (facts, not ideas)
- A server-authoritative multiplayer game server in Go, on an ECS with a fixed 20 Hz tick.
- Systems run in a fixed order each tick. Game state is saved in snapshots and restored.
- Many features (combat, abilities, items, pets, enemies) are built fast by several people and AI tools. Content keeps growing.
- The problem: features are tangled. Code in one feature changes another feature's data directly, and import cycles appear.

## Step 1: open survey
Do this before you open any other file. Ignore any saved memories about this topic until step 2.

Map the whole topic: every way shipped games let gameplay systems affect each other, and how each holds up as a game grows.
- Don't assume events are the answer. Include designs with no events at all.
- Stay on gameplay simulation. Skip rendering, UI and network transport.

Answer these:
1. What are all the ways systems affect each other? For each: how it works, who uses it, what it's good for, how it fails.
2. How do games set and enforce the order systems run in, within one tick?
3. How do they handle chains of cause and effect, and loops (damage that causes more damage)?
4. How do they keep features decoupled as the code grows? Who may write which data?
5. How do they keep it safe to change with many contributors: tests, static checks, tools, rules?
6. Where do approaches disagree? Find critics, post-mortems and "we tried X and moved away" stories.
7. What changes for a fixed-tick authoritative server (determinism, snapshots, replay, prediction)?

Sources:
- Find them yourself. Cover ECS engines, shipped games with public talks or blogs, non-ECS engines, deterministic simulation frameworks, and post-mortems.
- Include sources that argue against ECS or against events.
- Prefer primary sources: talks, official docs, engine source code.

Save it as SURVEY.md (now `gameplay-coupling-survey.md`). Start with what most shipped games agree on and where they split. Include one table: approach, used by, good for, fails when. No advice for our code yet.

## Step 2: test our ideas
Only after SURVEY.md is saved, read OUR-IDEAS.md (appendix B). It has our engine's facts, the team's positions, our current proposal and questions about our code.
- Judge each position and each part of the proposal against the survey. Try to break the proposal.
- Research more only where the survey has gaps.
- End with one recommended design for our code: what it replaces, and why.

Save it as REPORT.md (now this file).

## Both files
- Verdict first.
- Mark every claim as verified (you read the primary source), secondary (a summary), or inferred.
- End with a "not found or unverified" list, then sources with links.
- Style: short sentences, bullets, small tables, no long paragraphs.
```

## Appendix B: the ideas we tested

### Our engine: World Engine's Cardinal ECS

Check these facts in the code:

- A tick runs PreUpdate, then Update, then PostUpdate. Inside each, systems run in the order they are registered.
- System events are cleared at the end of every tick: `defer w.systemEvents.clear()` in
  world-engine `pkg/cardinal/internal/ecs/world.go`.
- Systems don't declare what they read or send. They call `SystemEvents[T]` and `EmitSystemEvent` inside their code.
  So Cardinal can't check the order at startup.
- Snapshots save components, not events. Anything that must survive a restore must be a component.

### Our code

- Game server: rampage-backend (World Engine v0.22.3). Don't treat today's code as a good example.
- Example of a request entity: shards/gameplay/internal/projectile/components/projectile_spawn_request.go. It is a
  volley that runs over many ticks.
- Map of how rampage features depend on each other: https://claude.ai/artifact/LAha4CMDwexg5LUWX9JmH2
- Earlier research for a sister project: [`system-architecture-options.md`](system-architecture-options.md).
  Check any claim from it before you rely on it.

### The team's positions

#### Andrew

- Early rampage used events heavily.
- The biggest pain: events didn't survive to the next tick, so system order needed great care. An event was lost if
  its receiver was registered before its sender.
- The team worked around it with data entities, like ProjectileSpawnRequest, which carry what's needed across ticks.
- Lifesteal: DamageProcessingSystem handles the damage event. Lifesteal needs an event from DamageProcessingSystem,
  which is itself a listener.

#### Anthony

- Events and intents are the basis of ECS. Rampage is half events and half direct calls, and that makes a mess.
- Example: one package heals a little, another deals damage that kills you, and a heal from a third package lands
  later. Scattered writes give order bugs.
- Wants a design based on events, protected by linters and tests, so no one (including AI) can break the order by
  moving systems.
- Suggested: put all listeners in PostUpdate, and listeners never send events.

#### Hartanto

- Fix coupling at the feature level first. DI plus interfaces, or leaf design, only as a last resort.
- An owner's read-only helpers live in a leaf package that imports only components and shared libraries. They make
  no Set calls, send no events and send no client messages.
- Open to changing World Engine if needed.
- The decision tree, for each call into another feature:

```text
Is the other side in your own feature?
├─ YES → Out of scope. Stop.
└─ NO  → continue

Is it a general tool (queue, maths, hook helpers), not game logic?
├─ YES → Move it to a shared library below all features. Stop.
└─ NO  → continue

Is it a type or constant?
├─ YES → Move it to the owner's components or events package. Stop.
└─ NO  → continue

Does it change data?
├─ NO (it's a read)
│   Is it used only by your system?
│   ├─ YES → Move it into your system. Stop.
│   └─ NO  → Move it to the owner's read-only helpers. Stop.
│
└─ YES → Who owns the data?
         (Owner = the feature that defines the component.
          For shared components: the feature that owns that kind of entity.)
         │
         Does it have exactly one owner?
         ├─ NO  → Pick one first, then come back.
         └─ YES → continue
         │
         Are you the owner?
         ├─ YES → Change it directly. Stop.
         └─ NO  → continue
         │
         Do you need the result inside your own calculation, right now?
         ├─ YES → Can you work it out by reading,
         │        and nothing else changes that data in the same tick?
         │        ├─ YES → Read and predict, then send an event to make it real.
         │        └─ NO  → Ownership is wrong. Move it to you;
         │                 the old owner sends you events.
         │                 ⚑ Check you're not becoming the next hub.
         │                   If you are, give that data its own small feature.
         └─ NO  → continue
         │
         Is it a current state (position, HP) or a one-time happening (spawned, died, teleported)?
         ├─ STATE     → The owner reads it and reacts. Delete your call; no event. Stop.
         └─ HAPPENING → Send an event.
                        Does the owner already have one?
                        ├─ YES → Use it.
                        └─ NO  → Add one to the owner's events package.
                        │
                        Must it survive past this tick (a backlog, a per-tick budget)?
                        ├─ YES → The owner copies events into its own queue component.
                        └─ NO  → continue
                        │
                        Does the owner's system run after yours in the tick?
                        ├─ YES → Done.
                        └─ NO  → Fix the order in main.go, or the event is silently lost.

At the end of every branch:
• Delete any copy of the owner's logic in your code.
• The owner sends its own client messages.
• Still importing their systems package? Go back to the top.
```

### Current proposal (try to break it)

1. Queue components that the owner drains at a fixed point, like Overwatch's ModifyHealthQueue.
2. One-tick events only flow forward in the tick.
3. Real loops (damage → death → explosion → damage) go to the next tick through a queue, with chain caps.
4. Lifesteal and shields are part of the health owner's own math, not events.
5. A Cardinal check fails a test when a system reads an event type and more of it is sent later in the same tick.

### Questions about our code

1. Event lifetime: one tick, a double buffer, or a queue that lasts until handled? How do we make sure no unread
   event is dropped?
2. How should we declare and enforce system order? Which engines catch "a reader runs before its sender"
   automatically?
3. Loops (lifesteal, thorns, on-kill explosions): same tick with a cap, or next tick? How do we cap chains?
4. In what order should damage, heals, shields and the death check run in one tick?
5. Events vs request entities vs queue components: when is each right?
6. When are direct calls between features still fine?
7. Enforcement in Go: tests, custom go/analysis linters, startup checks. What stops someone moving a system and
   silently breaking the order?
8. What is the smallest World Engine change that makes a lost event impossible, or at least loud?

### Sources to check if the survey missed them

- Overwatch: Tim Ford and Dan Reed, GDC 2017. Ford's talk is members-only on GDC Vault. A Chinese translation is at
  https://www.sohu.com/a/148848770_466876
- Bevy, Unity Entities and Netcode for Entities, Flecs, Unreal Mass and GAS, Photon Quantum, Dota 2, Riot (League,
  VALORANT), Risk of Rain 2, Path of Exile.

### Out of scope

- How effect kinds are sent to their owners (switch vs interface).
- Old-snapshot compatibility.

## Appendix C: a brief for a second opinion

Written 2026-11-19 for another AI that knows the code but not this research. Put the question in the "My
question" slot before sending it. It shows the design as it stood that day, with depth and flags.

```markdown
# Design to check: owner queue components (inboxes)

I want you to check a design against the code you know. Read it, then answer my question at the end.
Try to break it. Don't just agree.

## The problem it fixes

- World Engine's Cardinal ECS runs PreUpdate, Update, PostUpdate. Inside each, systems run in registration order.
- System events (`EmitSystemEvent` / `SystemEvents[T]`) are cleared at the end of every tick.
  A reader that runs before its sender never sees the event, and nothing warns us.
- Snapshots save components only, not events.
- Features calling each other directly cause Go import cycles.
- Many features write the same data (e.g. health), so heals and damage land in a random order.

## The design

1. **One owner.** Each component has one owner feature. Only the owner calls Set on it.
2. **Queue component.** To change another feature's data, add a request to a queue component that the owner
   owns. Example: "30 damage to entity E, from P, cause: sword, depth 0".
3. **Where the code lives.** The queue type and its "add request" function live in the owner's components
   package. That package imports nothing from other features. So the sender imports only that package:
   no import cycle. The function must never live in the systems package.
4. **One reader.** Only the owner's system reads the queue and empties it, at one fixed point in the tick.
5. **Lifetime.** The queue is a component, so it lasts until the owner empties it and is saved in snapshots.
   A request added after the owner ran this tick waits for next tick. It is never lost.
6. **Request fields.** Source (who caused it), cause (which attack or ability), depth (steps since the
   first hit), flags (e.g. "secondary" for lifesteal or thorns: can't trigger more effects).
7. **Fixed order for health.** The health system empties its queue like this:
   1. collect all damage and heals per target;
   2. apply armor and shields, read from stat components;
   3. apply the net change once;
   4. second pass for lifesteal and thorns, marked secondary;
   5. death check (hp <= 0 and not already dead);
   6. emit a "died" event.
8. **Events stay for news only.** Only the owner emits them ("died"). Readers must run after the owner in
   the same tick. Readers never emit the same type.
9. **Loops.** On-death effects (e.g. an explosion) add requests to the queue with depth + 1. They land next
   tick. At a depth cap the chain stops and logs an error.

## Open choices (not decided)

- One queue per feature (singleton component), or one queue per target entity?
- The depth cap value.
- Note: in this Cardinal version, `cardinal.Slice` Append copies the whole slice each time. A busy queue
  may need `Insert(Len, x)` instead.

## Where the idea comes from

- Overwatch reportedly uses queue components like this for health (GDC 2017 talk; only seen via a translation).
- Unreal Mover queues outside forces for the next tick. SpatialOS sends commands to the one owner of a
  component. Space Station 14 enforces owner-only writes with a lint rule.

## My question

<WRITE WHAT YOU WANT CHECKED HERE>

## How to answer

- Verdict first, then reasons.
- Point to real files and lines in our code.
- Say what you checked in the code and what you are guessing.
- Short sentences, bullets.
```
