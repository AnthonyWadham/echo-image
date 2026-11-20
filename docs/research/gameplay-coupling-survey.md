# Survey: how gameplay systems affect each other

Researched 2026-11-19. Research only. It is step 1 of the study in
[`event-architecture.md`](event-architecture.md), which tests our ideas against it.

> **These are findings, not the source of truth.** Code and [`CLAUDE.md`](../../CLAUDE.md) win when
> they disagree with this file.

Scope: gameplay simulation only (no rendering, UI or network transport). No advice for any codebase.

Legend: (V) verified = we read the primary source. (S) secondary = summary, wiki, snippet or unread primary. (I) inferred = our reasoning. [n] = source number.

## Verdict
### What most engines and shipped games surveyed agree on
- Each tick runs in fixed phases, in a set order. Unreal tick groups, Source's server frame, Doom 3's sorted entity list, Naughty Dog's update buckets and Flecs phases all work this way. (V) [21][32][34][36][7]
- Code must not depend on the order of listeners (callbacks that run when something happens). Bevy, Flecs, Ark, EnTT, Unreal multicast delegates and Lyra's message router all call that order undefined. (V) [4][8][18][15][21][22]
- Structural changes made while iterating (adding or removing components, despawns) usually wait for a sync point (a set place where deferred changes are applied). Bevy, Flecs, Unity, Unreal Mass, Ark and Source all do this. (V) [5][7][10][13][18][32] Overwatch reportedly made entity creation immediate after launch. (S) [45]
- Engine docs keep core state changes out of loose notices. Flecs says to use a system where one can do the job. Entitas recommends reactive systems over its event listeners for game logic. Bevy calls buffered messages more predictable. Godot says a signal should respond to behavior, not start it. (V) [8][16][3][31]
  - Quantum forbids events from changing state, but routes state changes through signals, which are synchronous calls inside the tick. So the line is "notice vs command", not "call vs no call". (V) [98]
- Rollback, replay and late join need all mutable state inside the snapshot, and re-run ticks must not repeat one-off effects. Quantum, GGPO, bevy_ggrs, Photon Fusion, Unity Netcode and Factorio all document this. (V) [98][101][102][103][100][79]
- Content outgrows the first design. (I) Dungeon Siege's template system was built for under 100 templates and shipped over 7,300 object types (its paper says 4,000). CK2's event polling "tanked performance". A bug might have doubled values in any of League's 400,000 script lines, and the game was tuned around them. (V) [66][81][51]

### Where they split
- Chain limits. Games with long effect chains add explicit limits: Doom 3 stops at 4,096 events per frame, Source caps a physics queue flush at 20 passes, TrinityCore (a WoW server emulator) stops proc chains at 10, Minecraft caps chained block updates, and an MTG replacement effect changes an event only once. (V) [34][33][90][75][86] General-purpose ECS libraries document no such limit. (I) from [4][19]
- One writer or open writes. Engines built for many contributors give key state one writer: Unreal GAS stores attributes as const and prefers Gameplay Effects for changes; SpatialOS let one worker write each component; Space Station 14 enforces owner-only writes with an analyzer. (V) [23][127][128] Bevy, Flecs and Unity Entities let any system with access write any component; Encompass allows a declared write priority. (I) from [5][7][10][116]
- Push or pull. Godot runs most interaction through signal callbacks. Caves of Qud builds objects from parts that talk through events. Unity's own guide promotes global event channels. Bitsquid prefers polling when few objects are watched. Flecs and Quantum keep gameplay in systems and state. (V) [112][67][30][41][8][98]
- How order is set. Bevy, Unity and Unreal use before/after rules. Flecs uses phases plus declaration order and refuses per-system dependencies. Entitas and Quantum run systems in the order added. Encompass derives the order from declared reads and writes. (V) [5][10][21][9][16][98][116]
- When chained effects land. Bevy runs nested triggers until no commands remain. Source fires zero-delay outputs in the same frame. Factorio circuit loops and Yampa push the next step to the next update. MTG puts effects on a stack that players respond to. (V) [4][32][78][123][86]
- ECS (entity component system: data in components, logic in systems) or not. Overwatch, V Rising (about 1,600 systems) and large Flecs projects use it. Godot, Jonathan Blow, Dennis Gustafsson, LogLog Games and the Mach engine reject or doubt it. (V) [44][62][9][112][113][114][115][135]
- Defer or apply now. Bevy, Unity and Flecs defer structural changes by design. LogLog says deferral splits one piece of logic into two places. (V) [5][10][7][115]
- How much determinism (same inputs, bit-identical results). Quantum, GGPO, Factorio and For Honor require it. Unity Netcode and VALORANT let the server correct drift. League kept a variable frame time and records it for replay. (V) [98][101][80][60][100][53][52]
- Where rules live. MTG Arena compiles card text into rules for a generic engine. Noita runs simple local pixel rules. Riot found spells tied to their champion's scripts blocked new mechanics. (V) [87][85][50]

## Approaches at a glance
Each row's sources and statuses are in section 1.

| Approach | Used by | Good for | Fails when |
|---|---|---|---|
| Direct calls | Source damage [33]; Quake [124]; Braid, The Witness [113]; Neversoft [42] | Flow you can read and step through | Callee re-enters the caller; coupling grows with content |
| Shared component data | Overwatch [45]; V Rising [62]; Bevy [5]; Unity [10]; Flecs [9] | Many entities; state as plain data | Two writers in the wrong order; order becomes hidden coupling |
| Immediate observers, signals, delegates | Bevy [4]; Flecs [8]; Ark [18]; EnTT [15]; Unreal [21]; Godot [112]; Quantum signals [98] | Rare events; links between unrelated parts | Undefined listener order; flow visible only at runtime |
| Global message bus | Lyra [22]; Unity event channels [30]; Source game events [32] | Notices across features | It is a global; payload types checked at runtime |
| Lifecycle hooks | Bevy [4]; Flecs [8]; EnTT [15] | Indexes and cleanup owned by the component | In-place edits skip hooks; one hook per component |
| Parts and buffs with event hooks | League [48]; Dota 2 [54]; Caves of Qud [67]; Dungeon Siege [66]; Overwatch Statescript [46] | Content without engine changes | An action skips the hook path; events fan out to all listeners |
| Buffered and timed event queues | Bevy Messages [3]; EnTT [15]; Donburi [19]; GameCode4 [35]; Source I/O [32]; Doom 3 [34] | Batches; known service point; delays | Unread events dropped; stale world; same-frame loops |
| Request components, owner-resolved queues | Overwatch [45]; Entitas [17]; Unreal Mover [28]; Yampa [123]; SpatialOS [127] | One writer; pending work is visible data | Resolve point after the reader (a tick of lag, as in Mover); pending requests are state a snapshot must hold (I) |
| Command buffers and sync points | Bevy [5]; Flecs [7]; Unity [10]; Unreal Mass [13]; Ark [18] | Safe iteration; parallel jobs | Effects invisible until the sync point |
| Polling and change detection | Bitsquid [41]; Factorio [78]; Bevy [6]; Unity [11]; Entitas [16] | Explicit timing; reactions stay in order | Coarse change flags; stale collected entities |
| Pull-based modifiers, derived stats | Dota 2 [54]; MTG layers [86] | Stacking that ignores apply order | Many value hooks to query on each read (I); effects that depend on each other need ordering rules (MTG) |
| State checks at fixed points | MTG [86]; Hearthstone [92]; Carmack [118] | Simultaneous deaths; no death-event chains | Doomed entities linger until the check |
| Effect pipeline with interception | Unreal GAS [23]; MTG [86]; MTG Arena [87]; Dota 2 filters [54]; Minecraft Bedrock [77] | Shields, immunity, one write path | Several interceptors need an order; re-entrancy bugs |
| Rules engine with a stack or queue | MTG [86]; Hearthstone [92]; Slay the Spire [97]; Legends of Runeterra [94] | Ordered, explainable chains | Edge cases need written rules; caps shape content |
| Rule tables and stimuli | Breath of the Wild [84]; Noita [85]; Thief [64]; The Dark Mod [65]; Dwarf Fortress [73] | Many interactions from few rules | Untested combinations; stimuli pile up |
| Designer-wired I/O and data hooks | Source Hammer I/O [32]; Thief [43]; CK3 on_actions [81]; Factorio mods [80] | Designers build behavior without code | Logic hidden in data; needs separate checkers |
| Smart objects | The Sims [70]; Ultima Online [71]; Oblivion [63] | New objects without changing agents | One object dominates; emergent acts break quests |
| Double-buffered state | Nystrom [40]; Gregory [36]; WoW batching [57]; Carmack [118] | Results that ignore update order | Every reaction lands a tick later |
| Transactions | Sweeney [120]; QuakeTM [121]; Verse [122] | All-or-nothing updates | Overhead and aborts; little shipped evidence |

## 1. Ways systems affect each other
### Direct calls
- How: the caller holds a handle and calls the target. Source's TakeDamage checks filters and game rules, then calls OnTakeDamage, which may call Event_Killed. (V) [33]
- Who: Braid and The Witness use one Entity base struct with subclasses. Neversoft dropped a component manager for direct pointers when lookups cost over 5% of CPU. (V) [113][42]
- Good for: flow you can read top to bottom. Gustafsson proposes calling each entity type's update by hand to keep control of order. (V) [114]
- Fails: calls re-enter a system mid-update. Source queues damage raised inside physics callbacks until after the physics step. (V) [33] Carmack's answer is a state check at one point (see State checks). (V) [118]
- Guard: Quake's exploding barrel turns off its own damage before its blast, so a chain cannot hit it twice. (V) [124]

### Shared component data
- How: systems read and write shared components in place. Overwatch's rules: components have no functions, systems have no state, and systems never call each other. (S) [45]
- Who: Overwatch showed about 46 systems and 103 component types, about 40% of them singletons. (S) [45]
- Who: V Rising runs about 1,600 ECS systems. Large Flecs projects report up to 800 components. (V) [62][9]
- Good for: West says state kept as plain components is easy to clone or save. She also calls it "still morally global". (V) [126]
- Fails: two writers in the wrong order. Encompass's example is a teleport overwritten by a later motion system. Bevy found 54 ambiguous system pairs in its own engine, some of them real bugs. (V) [116][2]

### Immediate observers, signals and delegates
- How: a trigger runs every matching callback inside the call. Bevy's World::trigger runs observers at once; Commands::trigger waits for the next sync point. (V) [4]
- Who: Bevy, Flecs, Ark (a Go ECS), EnTT, Unreal multicast delegates and Godot signals. Quantum's signals are typed calls that any system can implement, inside the tick. (V) [4][8][18][15][21][112][98]
- Good for: linking mostly unrelated parts, such as physics and achievements. (V) [39]
- Fails: listener order is undefined, and who gets called is known only at runtime. (V) [8][21][39]
- Fails: in Bevy issue #19569, commands from observers ran only after all observers. That broke one-hit-per-projectile logic and crashed on an already despawned target. (V) [2]

### Global message bus
- How: a router keyed by tag or name. Lyra's router calls listeners synchronously, and senders and listeners never reference each other. (V) [22]
- Who: Lyra's GameplayMessageSubsystem, Unity's ScriptableObject event channels, and Source's networked game events (at most 512 types). (V) [22][30][32]
- Good for: Unity says channels "can connect anything with anything" and can replace singletons. (V) [30]
- Fails: Lyra warns that listener order can change over time. A payload type mismatch is only logged at runtime. (V) [22]
- Fails: Nystrom calls a central event queue "still a global". Bitsquid rejects a "magic global switchboard". (V) [39][41]

### Lifecycle hooks
- How: on_add, on_insert and on_remove hooks run inside the change itself, like a constructor or destructor. (V) [4][8][15]
- Bevy allows one hook per event per component; a second one panics. Hooks are for structural side effects, not general logic, and do not run on in-place edits. (V) [4]
- Flecs allows one hook per component, which may change it; observers must not. EnTT fires on_update only through replace or patch, and listeners must not remove the component they watch. (V) [8][15]
- Good for: keeping indexes and cleanup with the component's owner. (I) from [4][8]

### Parts and buffs with event hooks
- How: effects live in buffs, modifiers or parts owned by a unit. The engine fires named hooks on them, such as OnDealHit. (V) [48][54]
- Who: League buffs, Dota 2 modifiers, Caves of Qud parts, Dungeon Siege components, Uncharted 2 state scripts and Overwatch Statescript. (V) [48][54][67][66][38][46]
- Good for: Statescript let new heroes ship with little or no new code. Bucklew claims composition keeps Qud's maintenance growth linear (his claim, not measured). (V) [46][67]
- Fails: Riot's Berserk prototype faked an attack, so on-hit hooks never fired. Dungeon Siege components became intertwined and order-dependent; Qud added part priorities in 2024 that also set save order. (V) [48][66][69]
- Fails: a Dota 2 OnAttack hook runs for every attack in the game, so each script must filter. (S) [55]

### Buffered and timed event queues
- How: writers append, readers poll at set points in the schedule, and buffers are cleared on a schedule. (V) [3][15]
- Who: Bevy Messages, EnTT enqueue/update, Donburi (Go) and GameCode4. Source's entity I/O and Doom 3's events also carry a fire time. (V) [3][15][19][35][32][34]
- Good for: batches and predictable timing, per Bevy's docs. Source designers can add delays. (V) [3][32]
- Fails: Bevy drops unread messages silently after one frame boundary. Fixed-step systems missed or double-counted events (#7691). (V) [3][2]
- Fails: the world changes before handling, so events must carry their data. (V) [39] Source re-services zero-delay outputs in the same frame (V) [32], so a zero-delay output loop never lets that frame's I/O pass end (I).

### Request components and owner-resolved queues
- How: a feature writes a request as data, and the owning system resolves all requests at one point. (I) from [45][17][28]
- Overwatch appends impacts to a Contact singleton and all damage and healing to a ModifyHealthQueue. One system resolves each later in the frame. (S) [45]
- An Entitas guide replaces direct logger calls with a LogMessageComponent and a reactive system. Unreal Mover forbids outside writes; other code queues moves and effects for the next tick. (V) [17][28]
- Yampa objects output kill and spawn requests that their collection applies. SpatialOS workers that do not own a component send commands to the one that does. (V) [123][127]
- Fails: Overwatch's deferred creation left new entities unusable until the next frame, so creation became immediate after launch. (S) [45]

### Command buffers and sync points
- How: structural changes are recorded while iterating and applied at sync points. (V) [5][7][10][13][18]
- Flecs makes the world read-only inside systems ("staging"), so the same call acts differently inside and outside a system. Undeclared writes merge at the end of the frame. (V) [7]
- Unity plays command buffers back in creation order. Commands recorded in parallel need sort keys to play back in a fixed order. (V) [10]
- Fails: LogLog says moving work across an event barrier splits one piece of logic into two places. (V) [115]

### Polling and change detection
- How: a scheduled system reads state or asks what changed, so reactions stay in the system order. (I) from [41][6]
- Bitsquid prefers polling when few objects are watched, with one consumer per event stream. (V) [41]
- Factorio walks a list of active entities each tick. Idle roboports leave the list and wake on demand, which cut their cost from 1 ms to 0.025 ms. (V) [78]
- Unity DOTS community veterans advise polling with change filters instead of events. (S) [20]
- Fails: Bevy counts any mutable access as a change. Unity flags whole chunks when a writer ran. Entitas groups check every component change, and one team removed position matchers for speed. (V) [6][11][16]
- Rust's specs flagged every entity fetched by a mutable join as modified, even when nothing changed. (V) [137]

### Pull-based modifiers and derived stats
- How: when a value is needed, the engine asks every active modifier or recomputes it from base values. Dota 2 splits modifier hooks into MODIFIER_PROPERTY value functions and MODIFIER_EVENT callbacks. (V) [54][86]
- MTG recomputes characteristics from printed values through seven layers, by timestamp within a layer unless one effect depends on another. (V) [86]
- Riot called League's old crowd control "quite a mess": designers set the buff type, called apply logic and kept can-attack and can-move flags in sync by hand. Now the buff type alone drives the effect. (V) [48]
- Good for: the result does not depend on the order effects were added. (I) from [86]

### State checks at fixed points
- How: consequences are found by scanning state at a checkpoint and applied together, until nothing changes. MTG does this before each priority, then puts waiting triggers on the stack. (V) [86]
- Carmack preferred one "health <= 0 && !killed" check over KillPlayer() called from 20 places. (V) [118]
- Hearthstone checks deaths only when the outer phase ends, then removes all doomed entities at once. (S) [92]
- Fails: entities already at lethal damage stay active until the check. (S) [92]

### Effect pipeline with interception
- How: every change passes one pipeline, where registered effects may replace, reduce or block it. (V) [86][23][87]
- GAS: code holds Attribute Sets as const, and Epic prefers Gameplay Effects for every attribute and tag change, because many effects roll back when the server rejects a prediction. An Immunity component blocks effects. (V) [23]
- GAS abilities block, cancel and gate each other through tag lists; Gameplay Events can also trigger them directly. (V) [23]
- Eric Lippert's rules-as-objects design evaluates a command against rule objects and the game state to produce effects, instead of putting rules in domain-class methods. (V) [136]
- MTG: a replaced event never happens. A replacement applies at most once per event, and the affected player orders competing ones. (V) [86]
- MTG Arena writes pending actions to a "whiteboard", and card rules erase or rewrite entries. Dota 2 filters can rewrite or cancel damage and orders. Minecraft Bedrock's before-event handlers may not change world state. (V) [87][54][77]
- Fails: GAS release notes list fixes for endless recursion in EndAbility (UE 4.27) and a recursive crash (UE 5.3). (S) [24]

### Rules engine with a stack or queue
- How: effects become stack items or queued actions, resolved one at a time under written rules. MTG uses a last-in-first-out stack; triggers wait for the next priority, then go on in APNAP order (active player first). (V) [86]
- Hearthstone resolves depth-first, with responder queues frozen in play order. Slay the Spire pushes reactions to the front of its action queue. (S) [92][97]
- Fails: before Naxxramas (2014) Hearthstone had no order-of-play rule, so simultaneous deaths were hard to predict. (S) [92]
- Fails: Legends of Runeterra caps its stack at 9 spells plus 1, and Riot cut a card partly over that cap. (S) [94][95]

### Rule tables and stimuli
- How: interactions are rules over categories (elements, materials, stimuli), so new content joins by tag. (I) from [84][85][64]
- Breath of the Wild: elements change materials and each other; materials cannot change each other. (S) [84]
- Noita: each pixel follows simple local rules; fire meeting water becomes steam. (V) [85]
- Thief's receptrons map a stimulus type and intensity to effects, including stimulating another object. The Dark Mod never applies a stim to its owner. (S) [64][65]
- Fails: Dwarf Fortress cats died of alcohol poisoning after grooming spilled drink from their fur. (V) [73]

### Designer-wired I/O and data hooks
- How: content links one object's outputs to another's inputs, or attaches events to named hooks. Source posts every output connection to one global queue, with fire time = now + delay. (V) [32][81]
- Thief's programmers defined properties and relations; designers built every object type in tools. CK3 fires all events through on_actions hooks, and script can add new hooks. (V) [43][81]
- Fails: CK3 "fallback" chains can loop and stop game time. (S) [82]
- Fails: logic in data needs its own checks. Tiger statically checks CK3 mod files for undefined references. (V) [83]

### Smart objects
- How: objects advertise what they offer, and agents pick by score. In The Sims, behavior code belongs to the object but runs on the Sim's thread. (V) [70]
- Ultima Online's planned ecology used resource labels with amount, max and regrowth. (V) [71]
- Fails: an early Joy Booth was so attractive that Sims used it until they collapsed. (V) [70]
- Fails: UO's beta ecology failed and was not tried again. BioShock shipped without its planned ecology. (V) [71][72]
- Fails: during testing, Oblivion NPCs killed a quest NPC; the team toned their behavior down. (S) [63]

### Double-buffered state (read old, write new)
- How: everyone reads last tick's state and writes the next one, so results do not depend on update order. Gregory notes this costs twice the memory. (V) [40][36]
- Original WoW checked every action in a batch against start-of-batch state, so two conflicting actions could both succeed. (V) [57]
- Carmack (2012) floated a read-only world, copied over at frame end. (V) [118]
- His Haskell Wolfenstein 3D reportedly queued effects such as damage for the next frame. (S) [119]
- Fails: every reaction lands one tick later, by design. (V) [40]

### Transactions
- Sweeney (2006) called transactions the only plausible fix for concurrent mutable state in gameplay code. (V) [120]
- QuakeTM ported a Quake server to software transactional memory. It scaled, but overhead and abort rates were too high. (V) [121]
- Verse rolls back every change in a failing <transacts> function. (V) [122]
- Fails: little shipped gameplay evidence beyond Verse. (I) from [120][121][122]

## 2. Order within one tick
- Phases: Unreal tick groups each finish before the next starts. Source runs pre-think hooks, entity think and physics, post-think hooks, the I/O queue, then deferred deletes. Doom 3 sorts pushers first, then actors. (V) [21][32][34]
- Source servers tick at a fixed rate (15 ms by default). Each tick processes user commands, runs a physics step, checks game rules, then updates object states. (V) [138]
- Buckets: in Uncharted 2, an object may read only objects in earlier buckets, never its own. (V) [37]
- Explicit graphs: Bevy orders systems only through before/after, sets or chains; otherwise they run in parallel, in no fixed order. (V) [5]
- Unity orders systems inside groups, and UpdateBefore/After work only among siblings. Unreal tick prerequisites make one actor wait for another. (V) [10][21]
- Declaration order: Flecs orders by phase, then entity id, and refuses per-system dependencies so modules stay reusable. Entitas and Quantum run systems in the order they were added, for determinism. (V) [7][9][16][98]
- Derived order: Encompass builds the order from each system's declared messages and component reads and writes. (V) [116]
- Ties: Unity scripts with equal order run in an order that can change across builds and machines. Godot breaks priority ties by scene-tree order. (V) [29][31]
- Sync points: Bevy adds them between ordered systems. Flecs adds one only when a later system reads what an earlier one declared it writes. (V) [5][7]
- Checks on order: see section 5 (Bevy's ambiguity test and shuffle seed).
- Main bug class: reading a peer that has not updated yet ("one frame off"). Gregory, Epic's tick docs and Nystrom all describe it. (V) [36][21][40]
  - Uncharted 2's ray casts saw one-frame-old collision until broadphase bounds were updated between buckets. (V) [37]
- Churn: Bevy 0.11 removed the Base Sets that 0.10's "stageless" rework had added. LogLog ended up declaring many order constraints in Bevy anyway. (V) [1][115]
- Order across features: Epic says Lyra 5.0 had race conditions before its init-state system. (V) [25]
- A studio reported Mass processors racing on their first parallel run after upgrading to UE 5.6; an Epic engineer acknowledged it. (S) [14]

## 3. Chains and loops
### How chains resolve
- At once, to the end: Bevy evaluates nested triggers until no commands remain. Donburi (Go) loops until its queue is empty. (V) [4][19]
- Depth-first: Hearthstone fully resolves each new event first; once resolution starts, its responder queue is frozen in play order. (S) [92]
- Stack: MTG resolves the last-added item only after all players pass. (V) [86]
- Queue with push-to-front: Slay the Spire puts reactions inside an action at the front of the queue. (S) [97]
- Next update: GameCode4 swaps two queues, so events raised during processing wait. Factorio circuit loops count up one step per update. (V) [35][78]

### How loops are stopped
- Hard caps: Doom 3 aborts after 4,096 events in a frame. Source's physics flush runs at most 20 extra passes. TrinityCore stops proc chains at 10. Minecraft caps chained neighbor updates. (V) [34][33][90][75]
- Content caps: Legends of Runeterra's stack holds 9 spells plus 1. Hearthstone ends the game as a draw at turn 90. (S) [94][92]
- Cause flags: TrinityCore's triggered spells cannot proc by default. Dota 2's damage API has a REFLECTION flag. (V) [90][54]
  - A wiki says the flag exists to stop two Blade Mails reflecting forever. (S) [56]
- Once rules: an MTG replacement effect touches an event only once. "Do this only once each turn" limits a trigger. (V) [86]
- Re-checks: MTG's "intervening if" checks a trigger's condition when it fires and again when it resolves. (V) [86]
- Immunity and delays: Source props ignore damage on the tick they spawn. Blast-resistant props burn for a while instead of exploding at once. (V) [33]
- Structural bans: Encompass forbids message cycles between systems. (V) [116]
- Per-trigger limits: Path of Exile allows one trigger gem per skill. Path of Exile 2 stopped Meta-gem skills gaining energy, and a triggered Greater Lightning Bolt triggering another. (V) [91]
- Adjudication: MTG declares a loop of only mandatory actions a draw. XMage asks players to accept a draw after a loop counter passes 15. (V) [86][89]
  - MTG Arena players have filed a bug report about its "take a different action or the game will end in a draw" loop warning. (S) [88]
- No ECS engine studied documents a depth or cycle limit for observers. (I) from [4][8][15][18][19]

### Carrying the cause through a chain
- League's Spell Origination carries cast ID, time and slot through every extended effect. Riot used it for kill credit and once-per-cast limits. (V) [48]
- League tracks a spell's original dimension through buffs, summons and missiles. (V) [49]
- MTG checks most triggers just after an event, but leave-play triggers "look back in time". (V) [86]

### Recorded failures
- Nystrom: synchronous cycles overflow the stack fast; queued cycles keep running unnoticed. His rule: do not send events from inside a handler. (V) [39]
- Stale data: an MTG Arena rule stored a spell's object ID and later read its changed mana value. The fix stored the value. (V) [87]
- Leaks across systems: WoW's Corrupted Blood stayed on dismissed pets and escaped its raid. A Dwarf Fortress loyalty-cascade bug filed in 2010 was still open in 2025. (V) [58][73]
- Copy races: Mel's reflect hit a "Black Hole Glitch" race between missile duplication and reflection. (V) [50]

## 4. Decoupling and who may write what
### One writer per piece of state
- SpatialOS let one worker write a component at a time. Others read it and send commands to the owner. (V) [127]
- Space Station 14 lets only the owning system modify a component. An analyzer reports outside writes as error RS2008. (V) [128]
- GAS keeps the Attribute Set const, and Epic says to make changes through Gameplay Effects. (V) [23]
- Unreal Mover allows no outside writes; other code queues influences for the next tick. (V) [28]
- Encompass allows two writers only through one merging system or a declared write priority. (V) [116]
- Overwatch made CommandSystem the only place with side effects on player commands. (S) [45]
- Fails: SpatialOS's optional command short circuiting could run a command on a worker that had just lost authority. (V) [127]

### Rules on how features talk
- Overwatch: systems never call other systems. A helper called from many places should read few components and have few side effects. (S) [45]
- Space Station 14 prohibits "method events" (events raised to make another system act). Code calls the owning system's method instead. (V) [128]
- Bitsquid: a high-level layer moves data between low-level systems that never know each other. (V) [41]
- Quantum: effects inside the tick go through signals; events only carry data out of the simulation. (V) [98]

### Module boundaries
- Flecs splits each feature into a components module and a systems module, so other code imports only the data. (V) [9]
- Bevy documents system sets as a stable public API that other plugins order against. (V) [5]
- Unreal Game Features put each feature in a plugin that adds its own components to actors that opt in. Fortnite blocks cycles between feature plugins with an asset-reference checker. (V) [25][26]
- Unreal Build Tool checks module cycles against an allow list. Unity assemblies kept Backyard Baseball's input and gameplay layers apart. (V) [25][131]
- Unity write groups let a system take over another system's output without editing it. (V) [11]

### Content coupling
- Before Sylas, League spells lived inside each champion's scripts, so nearly every spell needed a refactor. Mel's reflect found spells that assumed their own champion was present. (V) [50]
- Per-pair relationship contracts did not scale for Mordekaiser's realm, so Riot added a dimension value to every object. (V) [49]
- For Berserk, Riot changed shared targeting code instead of every spell. A dormant NeverSelf flag then had to be set in almost every check. (V) [48]

### How boundaries decay
- Bitsquid calls it "coupling creep": deadline pressure opens back doors between systems. (V) [41]
- Riot rates tech debt by "contagion": how far it spreads through other systems and copied data. (V) [51]

## 5. Staying safe with many contributors
### Tests
- Sea of Thieves had 10,000 tests, 90% of them unit tests, with a core set run before every check-in. (V) [61]
  - Listed drawbacks: time to write, tests on a changing game, flaky integration tests. Unit tests miss interactions between big systems. (V) [61]
- Riot's League test system ran about 5,500 cases per build, about 100,000 a day (2016). New tests sat in staging for a week. Raw sleeps were banned. (V) [51]
- Factorio's map tests compare CRCs (checksums) with saved values. A test server runs all day and emails the latest committers. (V) [79]
- Space Station 14 spawns and deletes every entity prototype, and checks that save-load-save gives identical output. (V) [129]
- MTG Arena compiles about 80% of new cards automatically and checks card work with scripted regression games. (V) [87]
- Quantum uses recorded replays with checksums as regression tests; an intended gameplay change forces a re-record. (V) [99]

### Static and runtime checks
- Bevy panics at runtime (error B0001) on conflicting queries in one system. (V) [6]
- Unity's job safety system throws when two unordered jobs could write one container. Data passed by hand in NativeArrays is not tracked. (V) [11]
- Bevy's ambiguity detection is off by default. Bevy's repo has a test asserting zero order ambiguities in its default plugins, and a debug shuffle_seed exposes order that holds by chance. (V) [2][5]
- Space Station 14 routes review of gameplay folders to named owners through CODEOWNERS. (V) [129]
- Tiger checks CK3 mod files for undefined references and scope misuse. go-arch-lint enforces Go layer rules (a general tool, not game-specific). (V) [83][132]
- For Honor put validation macros in every state accessor and generated gameplay state code. (V) [60]

### Tools for cause and effect
- Unity Journaling records write access, not actual changes, so it guesses which system set a value. (V) [12]
- Unreal's Visual Logger records per-actor state on a timeline to scrub after a session. (V) [27]
- Riot logs state as key-value pairs and diffs the logs for the first divergence. (V) [52]
- Gregory suggests time-stamping state and asserting on it, to catch half-updated reads. (V) [36]

### Rules and process
- For Honor: define clear rules, remove all exceptions, and enforce them in the architecture early. (V) [60]
- Our Machinery wants rules clear enough for a machine to check, for example in a pre-commit hook. (V) [130]
- Riot: a bug might have doubled numeric values in any of League's 400,000 script lines, and the game was tuned around them. A 4-line fix needed a live toggle, regex searches and QA sweeps; in the end only a handful of champions needed script changes. (V) [51]
- Thief's designers could use only a pre-chosen set of AI triggers, whose timing and cost programmers controlled. (V) [43]
- AI-written code: Godot (2026) tightened its policy because AI raised PR counts while reviewers stayed the same. Bevy found a ban unenforceable; it now requires disclosure and that authors understand every line. (V) [133][134]

## 6. Where approaches disagree
### ECS: critics and supporters
- Godot: ECS speeds up logic only in rare games with tens of thousands of objects; most games have hundreds at most. (V) [112]
- Jonathan Blow never shipped a game with components; component codebases made simple things complicated. (V) [113]
- LogLog Games left Rust and Bevy for Unity in 2024 and call ECS "a very specific tool". (V) [115]
- Mach's docs say its object system is "NOT an entity component system". (V) [135]
- Supporters: Overwatch credits ECS with handling unusual hero mechanics; V Rising runs about 1,600 systems. (V) [44][62]

### Events and observers: critics and supporters
- Nystrom: observers turn a static call graph into runtime state. (V) [39]
- Maier et al. cite Adobe: event code was 1/3 of desktop-app code and held 1/2 of bugs (UI apps, not games). Observers can also show half-updated state ("glitches"). (S) [117]
- Bitsquid prefers polling and single-consumer streams. Space Station 14 bans method events. (V) [41][128]
- Supporters: Unity's guide promotes global event channels, Qud's parts talk through events, and V Rising gives designers event components. (V) [30][67][62]
- Engines narrowed the idea: Bevy 0.17 split "events" into buffered Messages and observer Events because one name confused users. (V) [1]

### "We tried X and moved away"
- CK2 polled events by probability; it "tanked performance". CK3 fires all events through on_actions hooks. (V) [81]
- Neversoft dropped manager lookups for direct pointers when they passed 5% of CPU. (V) [42]
- Planetary Annihilation left lockstep, because lockstep runs at the slowest player's speed. (V) [105]
- Caves of Qud added typed, pooled "MinEvents" next to its older string-named events. (S) [68]
- Overwatch's kill cam (a second ECS world) broke global system access, so system state moved into singleton components. (S) [45]
- League moved from champion-bound spells to shared abilities, and from per-pair contracts to a per-object dimension. (V) [50][49]
- Hearthstone changed area healing from one-at-a-time to all-at-once when a card needed it. (S) [92]
  - In 2021 it re-aligned all cards to one priority order for conflicting effects. (V) [93]
- Minecraft's opt-in Redstone Experiments (snapshot 24w33a, 2024) order wire updates by context instead of position, and pick randomly only where context is not enough. (V) [75]
  - Mojang kept the quasi-connectivity quirk (MC-108) as intended. (S) [76]
- Factorio dropped multithreaded electric networks: shared entities made results order-dependent, and the work was memory-bound. (V) [78]
- WoW Classic cut its spell batch window from 400 ms to 10 ms and expected bugs. (V) [57]
- Ultima Online's ecology was removed. Koster says the alpha version worked and the beta failed; Sylvester quotes Garriott that players killed everything, and argues dense simulations hide cause and effect. (V) [71][72]
- Tarn Adams calls Dwarf Fortress's polymorphic item hierarchy "a mistake". (V) [74]
- Amethyst halted in 2021 over governance and vision, not over ECS. (V) [125]

## 7. What changes on a fixed-tick authoritative server
### Fixed ticks
- VALORANT steps movement and physics at a fixed 128 Hz, because two 5 ms steps differ from one 10 ms step. Apex simulates at 20 Hz and saves a full world snapshot every tick. (V) [53][59]
- Overwatch uses fixed command frames (16 ms), and anything predicted runs in UpdateFixed. (S) [45]
- Fiedler: the renderer produces time and the simulation consumes it in fixed steps. (V) [106]
- League keeps a variable frame time and records each frame's timing for replay. (V) [52]
- Bevy's fixed schedule runs zero or more times per frame. Events read there were missed or double-counted (#7691); Bevy now swaps message buffers only after a fixed cycle. (V) [5][2][3]
- Age of Empires ran each command two communication turns after it was issued, so all machines ran it on the same turn. (V) [104]
- Fiedler's lockstep sends the full input state every frame, not press and release events, so the same input applies on the same frame. (V) [106]

### Snapshots must hold all state
- Quantum: systems must be stateless, and all mutable state lives in the frame. Timers are stored as target frame numbers. Late joiners start from a serialized frame, so static fields are lost. (V) [98][99]
- bevy_ggrs restores only registered components; per-system Local<T> is not restored. Fusion resets networked properties but not plain fields. (V) [102][103]
- Factorio: a speed cache rebuilt on load, not saved, desynced once new content could change speed. (V) [79]
- Source saves its pending entity I/O queue in save games, so delayed outputs survive save and load. (V) [32]
- EnTT snapshots must load in exactly the order they were written. (V) [15]

### Events under rollback and replay
- Quantum events only carry data out to the view and never change state. Late joiners never get earlier events. (V) [98]
- bevy_ggrs bans Bevy Messages inside the rollback schedule: rolled-back messages are not retracted and get written again. (V) [102]
- Factorio limits what mods may do when a save loads; event registrations must match after loading. (V) [80]
- One-off effects: Unity flags the first full prediction of a tick. Fusion exposes IsForward. Quantum de-duplicates events by hash. GGPO defers effects until rollback ends. (V) [100][103][98][101]

### Prediction scope
- Overwatch's Statescript keeps rollback history only for the local entity; data such as health has none. (V) [46]
  - Overwatch reportedly never predicts damage. (S) [45]
- GAS cannot predict effect removal, periodic ticks or executions. Paragon and Fortnite do not predict damage. (S) [24]
- Unity Netcode does not need full determinism. Partial snapshots mispredict effects between entities; its docs suggest the acting entity write the target. (V) [100]
- VALORANT: when input is late, the server guesses, commits the guess as truth, and corrects that client. (V) [53]
- Destiny: a raid bug came from boss logic built only in client systems, which the host never secured. (V) [47]
- Destiny's activity scripts declare up front which objects and facts they care about. Halo: Reach host migrations had left script state inconsistent (duplicate CTF flags), so Destiny updates every object a script cares about atomically. (V) [47]
- Legends of Runeterra previews how the stack will resolve when the outcome is predictable. (V) [96]

### Determinism hazards
- The Go spec leaves map iteration order unspecified, and lets compilers fuse float operations (for example FMA) unless an explicit conversion forces rounding. (V) [108]
- Riot found pointer-keyed maps iterate in address order, which varies between runs, and six separate clock APIs in one server. (V) [52]
- Box2D found that threads appending with atomics changed order run to run. It reached cross-platform float results by avoiding fast-math and FMA, but does not support rollback. (V) [107]
- Factorio changed its Lua so pairs() follows insertion order and random uses the map seed. (V) [80]
- Age of Empires saw tiny divergences compound over minutes: a misaligned deer led to a villager missing a spear throw. Out-of-sync bugs stayed hard to trace even with world checksums. (V) [104]
- Supreme Commander hashed all game state once a second and ended the game on any mismatch. (V) [105]

### Testing determinism
- GGPO's SyncTest forces a one-frame rollback every frame and compares. Factorio's heavy mode saves and reloads every tick. (V) [101][79]
- Riot replays 2-3 thousand real games a day. Box2D runs a hash test on every pull request. (V) [52][107]
- FoundationDB and TigerBeetle simulate whole systems from a seed, so any failure repeats exactly. (V) [109][110]
- For Honor: 80% of desyncs came from cosmetic code, threading violations and random mistakes. (V) [60]
- Costs: Fiedler limits lockstep to 2-4 players. (V) [106]
  - NetherRealm reportedly spent about 2 man-years on serialization when adding rollback to Mortal Kombat X. (S) [111]

## Not found or unverified
### Gap research not done (stopped at usage limit)
- A primary source for Ford's Overwatch talk. All Overwatch ECS claims are still (S) via one translation.
- StarCraft II effect trees and validators; Risk of Rain 2 proc masks; official Diablo III proc rules.
- Actor and message-passing game servers (EVE, Orleans, SpatialOS shutdown).
- Insomniac's component architecture (GDC 2010); Halo: Reach networking; Destiny engine multithreading.
- Blackboards and shared fact stores (F.E.A.R., Unreal blackboards, Valve's dynamic dialog rules).
- A final critic pass on this edited version.

### Not found
- Left out for lack of a checked claim: Unity C# events, Entitas destroy flags, Unity enableable components, Flecs table-level change detection, singletons in Bevy, Flecs and Ark, Mass's "64 signals per entity" limit (it can be worked around), the date of Space Station 14's method-event ban, when or why Mach left ECS.
- No official guidance on events between systems in Unity's Entities manual. No Bevy or Flecs page documents event entities or request components; that evidence is Entitas guides and Unity forum threads.
- No documented depth or cycle limit for observers in Bevy, Flecs, EnTT, Ark or Donburi.
- No primary source says how pending event queues should be stored in snapshots.
- No commercial post-mortem says "we removed our event bus". Closest: Neversoft's direct pointers, Bitsquid's polling, Space Station 14's ban.
- No game studio source on AI-written gameplay code (only engine policies), and no studio documents using ArchUnit, import-linter, depguard or go-arch-lint.
- No Epic doc says the core game must not depend on feature plugins. Epic's init-state manager records states but does not enforce them.
- Unconfirmed: Path of Exile trigger cooldowns, Diablo III's official proc rules ("procs can not proc procs" is community-only), Risk of Rain 2's ProcChainMask, Factorio inserter sleep and wake, Dwarf Fortress tick order, RimWorld comp hooks, Divinity: Original Sin 2 surface rules, StarCraft II effect recursion limits, Hearthstone loop rules within a turn.
- Not researched: Yu-Gi-Oh! chain rules; Rainbow Six Siege, Diablo IV, Guild Wars 2, Hitman and Supercell architecture.

### Unread or blocked primary material
- Talks known only through summaries: Ford's Overwatch talk (Chinese translation), Breath of the Wild GDC 2017 (press), Stallone's Mortal Kombat X talk (Infil), Carmack's QuakeCon 2013 keynote (fan transcript).
- Talks not read at all: Hipple's Unite 2017 talk, Bucklew's IRDC 2015 talk, Noita GDC 2019, Nystrom's Roguelike Celebration 2018, Halo: Reach networking (2011), Rocket League (2018), Overwatch replay (2017), Insomniac components (2010), the Sea of Thieves video, and Realms of Ruin determinism (2024; image-only slides).
- Books not read beyond excerpts: Game Engine Architecture section 14.7 (events) and Game Coding Complete's prose.
- Blocked pages: Valve Developer Community (bot check), Paradox wikis and forum, Fandom pages for Thief and Dota, Liquipedia, Unreal API reference (script-rendered), Unreal GameplayPrediction.h (gated repo), Epic's "Damage in UE4" blog, Hive Workshop, poewiki (anti-scraper check respected), MTG Arena feedback pages (sign-in), the SpatialOS events page.
- Unreal Mass ordering, observer and access rules rest only on community docs (MassSample, X157); they are not used above.
- Seed corrections: Amethyst halted in 2021, not 2022, over governance; MTG's shortcut rule is now 733; Sander Mertens's ECS FAQ has no section on events. Conflicting numbers: Dungeon Siege's paper says 4,000 object types, its slides over 7,300. The Mortal Kombat X talk is dated 2017 by one site and 2018 by GDC Vault.

## Sources
#### ECS engines and libraries
- [1] Bevy release notes 0.11 and 0.17, Bevy project, 2023 and 2025, https://bevy.org/news/bevy-0-11/ ; https://bevy.org/news/bevy-0-17/, release notes
- [2] Bevy PR #20731 "Event Rearchitecture" and issues #4377, #7691, #19569, Bevy project, 2022-2025, https://github.com/bevyengine/bevy/pull/20731 ; https://github.com/bevyengine/bevy/issues/4377 ; https://github.com/bevyengine/bevy/issues/7691 ; https://github.com/bevyengine/bevy/issues/19569, issue tracker
- [3] Bevy message docs (Messages, Message trait, message update), Bevy 0.20.1, n.d., https://docs.rs/bevy/0.20.1/bevy/ecs/message/struct.Messages.html ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/message/mod.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/message/update.rs, official docs and engine source
- [4] Bevy observer, hook and observer-example source, Bevy 0.20.1, n.d., https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/observer/distributed_storage.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/lifecycle.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/examples/ecs/observers.rs, engine source
- [5] Bevy scheduling source (system order, ScheduleBuildSettings, ApplyDeferred, SystemSet, FixedUpdate), Bevy 0.20.1, n.d., https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/system/mod.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/schedule.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/executor/mod.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/set.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_app/src/main_schedule.rs, engine source
- [6] Bevy Changed<T> filter docs and error B0001, Bevy 0.20.1, n.d., https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/query/filter.rs ; https://github.com/bevyengine/bevy/blob/v0.20.1/errors/B0001.md, engine source and official docs
- [7] Flecs Systems manual (v4), Sander Mertens, n.d., https://github.com/SanderMertens/flecs/blob/master/docs/Systems.md, official docs
- [8] Flecs Observers manual (v4), Sander Mertens, n.d., https://github.com/SanderMertens/flecs/blob/master/docs/ObserversManual.md, official docs
- [9] Designing with Flecs, Sander Mertens, n.d., https://github.com/SanderMertens/flecs/blob/master/docs/DesignWithFlecs.md, official docs
- [10] Unity Entities 6.5 manual: system groups; command buffer automatic playback; command buffer playback, Unity, n.d., https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-update-order.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entity-command-buffer-automatic-playback.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entity-command-buffer-playback.html, official docs
- [11] Unity Entities 6.5 manual: query filters; write groups; job dependencies; Unity Manual: job safety system, Unity, n.d., https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entityquery-filters.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-write-groups.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/scheduling-jobs-dependencies.html ; https://docs.unity3d.com/Manual/job-system-native-container.html, official docs
- [12] Unity Entities manual: cleanup components (6.5); singleton components (6.5); Journaling (1.3), Unity, n.d., https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-cleanup-introducing.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-singleton.html ; https://docs.unity3d.com/Packages/com.unity.entities@1.3/manual/entities-journaling.html, official docs
- [13] Overview of Mass Entity; Overview of Mass Gameplay (UE 5.9), Epic Games, n.d., https://dev.epicgames.com/documentation/en-us/unreal-engine/overview-of-mass-entity-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/overview-of-mass-gameplay-in-unreal-engine, official docs
- [14] Race condition with Mass Processors writing to UWorld::SubsystemCollection::SubsystemArrayMap, Unreal Engine forum, 2025, https://forums.unrealengine.com/t/race-condition-with-mass-processors-writing-to-uworld-subsystemarraymap/2667479, forum
- [15] EnTT Crash Course: events and signals; entity-component system, skypjack, n.d., https://github.com/skypjack/entt/blob/master/docs/md/signal.md ; https://github.com/skypjack/entt/blob/master/docs/md/entity.md, official docs
- [16] Entitas wiki: Systems; Attributes; FAQ, Entitas project, n.d., https://github.com/sschmid/Entitas/wiki/Systems ; https://github.com/sschmid/Entitas/wiki/Attributes ; https://github.com/sschmid/Entitas/wiki/FAQ, official docs
- [17] How I build games with Entitas (FNGGames), Entitas wiki, n.d., https://github.com/sschmid/Entitas/wiki/How-I-build-games-with-Entitas-(FNGGames), community wiki
- [18] Ark documentation: Event system; Queries (v0.8.x), Ark project, n.d., https://ark.mlange.dev/events/ ; https://ark.mlange.dev/queries/, official docs
- [19] Donburi features/events/events.go (v1.15.8), yohamta, n.d., https://github.com/yohamta/donburi/blob/main/features/events/events.go, engine source
- [20] Event component data (thread), Unity Discussions, 2020-2022, https://discussions.unity.com/t/event-component-data/816934, forum
#### Non-ECS engines and books
- [21] Actor Ticking; Multicast Delegates (UE5 docs), Epic Games, n.d., https://dev.epicgames.com/documentation/en-us/unreal-engine/actor-ticking-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/multicast-delegates-in-unreal-engine, official docs
- [22] GameplayMessageSubsystem.h and .cpp (Lyra plugin, community mirror), Epic Games, n.d., https://raw.githubusercontent.com/imnazake/gameplay-message-router/master/Source/GameplayMessageRuntime/Public/GameFramework/GameplayMessageSubsystem.h ; https://raw.githubusercontent.com/imnazake/gameplay-message-router/master/Source/GameplayMessageRuntime/Private/GameFramework/GameplayMessageSubsystem.cpp, engine source
- [23] Gameplay Ability System docs: Attributes and Attribute Sets; Understanding GAS; Using Gameplay Abilities; Gameplay Effects, Epic Games, n.d., https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-attributes-and-attribute-sets-for-the-gameplay-ability-system-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/understanding-the-unreal-engine-gameplay-ability-system ; https://dev.epicgames.com/documentation/en-us/unreal-engine/using-gameplay-abilities-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-effects-for-the-gameplay-ability-system-in-unreal-engine, official docs
- [24] GASDocumentation, tranek, n.d., https://github.com/tranek/GASDocumentation, community wiki
- [25] Game Features and Modular Gameplay; Game Framework Component Manager; Module Properties, Epic Games, n.d., https://dev.epicgames.com/documentation/en-us/unreal-engine/game-features-and-modular-gameplay-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/game-framework-component-manager-in-unreal-engine ; https://dev.epicgames.com/documentation/en-us/unreal-engine/module-properties-in-unreal-engine, official docs
- [26] Real world implementation detail of build-time asset and plugin exclusion (reply by Epic staff), Epic Developer Community forum, 2026, https://forums.unrealengine.com/t/real-world-implementation-detail-of-build-time-asset-and-plugin-exclusion/2720546, forum
- [27] Visual Logger in Unreal Engine, Epic Games, n.d., https://dev.epicgames.com/documentation/en-us/unreal-engine/visual-logger-in-unreal-engine, official docs
- [28] Comparing Mover and Character Movement Component (UE 5.9), Epic Games, n.d., https://dev.epicgames.com/documentation/en-us/unreal-engine/comparing-mover-and-character-movement-component-in-unreal-engine, official docs
- [29] Script execution order (Unity 6.6 manual), Unity, n.d., https://docs.unity.com/en-us/engine/6000.6/manual/scripting/managing-update-order/script-execution-order, official docs
- [30] Architect your code with ScriptableObjects (after Ryan Hipple, Unite 2017); Use ScriptableObjects as event channels, Unity, n.d., https://unity.com/how-to/architect-game-code-scriptable-objects ; https://unity.com/how-to/scriptableobjects-event-channels-game-code, article
- [31] Godot class reference (Node.xml, Object.xml); Scene organization, Godot Engine, n.d., https://raw.githubusercontent.com/godotengine/godot/master/doc/classes/Node.xml ; https://raw.githubusercontent.com/godotengine/godot/master/doc/classes/Object.xml ; https://docs.godotengine.org/en/stable/tutorials/best_practices/scene_organization.html, official docs
- [32] Source SDK 2013 server code: cbase.cpp; gameinterface.cpp; igameevents.h, Valve, n.d., https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/cbase.cpp ; https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/gameinterface.cpp ; https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/public/igameevents.h, engine source
- [33] Source SDK 2013 server code: baseentity.cpp; physics.cpp; props.cpp, Valve, n.d., https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/baseentity.cpp ; https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/physics.cpp ; https://raw.githubusercontent.com/ValveSoftware/source-sdk-2013/master/src/game/server/props.cpp, engine source
- [34] DOOM 3 GPL source: Game_local.cpp; Event.cpp, id Software, 2011, https://raw.githubusercontent.com/id-Software/DOOM-3/master/neo/game/Game_local.cpp ; https://raw.githubusercontent.com/id-Software/DOOM-3/master/neo/game/gamesys/Event.cpp, engine source
- [35] GameCode4 EventManager.h and EventManagerImpl.cpp (Game Coding Complete, 4th ed.), Mike McShaffry and David Graham, 2012, https://raw.githubusercontent.com/MikeMcShaffry/gamecode4/master/Source/GCC4/EventManager/EventManager.h ; https://raw.githubusercontent.com/MikeMcShaffry/gamecode4/master/Source/GCC4/EventManager/EventManagerImpl.cpp, engine source
- [36] Book Excerpt: Game Engine Architecture, Jason Gregory, 2009, https://www.gamedeveloper.com/programming/book-excerpt-game-engine-architecture, book
- [37] Multiprocessor Game Loops: Lessons from Uncharted 2, Jason Gregory (Naughty Dog), 2010, https://www.gameenginebook.com/resources/gfg2010-final.pdf, slides
- [38] State-Based Scripting in Uncharted 2, Jason Gregory (Naughty Dog), 2009, https://www.gameenginebook.com/resources/gdc09-statescripting-uncharted2.pdf, slides
- [39] Game Programming Patterns: Observer; Event Queue, Robert Nystrom, 2014, https://gameprogrammingpatterns.com/observer.html ; https://gameprogrammingpatterns.com/event-queue.html, book
- [40] Game Programming Patterns: Update Method; Double Buffer; Component, Robert Nystrom, 2014, https://gameprogrammingpatterns.com/update-method.html ; https://gameprogrammingpatterns.com/double-buffer.html ; https://gameprogrammingpatterns.com/component.html, book
- [41] Managing Coupling; Managing Coupling Part 2 (polling, callbacks and events), Niklas Frykholm (Bitsquid), 2011, https://bitsquid.blogspot.com/2011/01/managing-coupling.html ; https://bitsquid.blogspot.com/2011/02/managing-decoupling-part-2-polling.html, dev blog
- [42] Evolve Your Hierarchy, Mick West, 2007, https://cowboyprogramming.com/2007/01/05/evolve-your-heirachy/, dev blog
- [43] Postmortem: Thief: The Dark Project, Tom Leonard, 1999, https://www.gamedeveloper.com/design/postmortem-i-thief-the-dark-project-i-, postmortem
#### Shipped games: talks and blogs
- [44] 'Overwatch' Gameplay Architecture and Netcode (session page), Timothy Ford (Blizzard), 2017, https://gdcvault.com/play/1024001, talk
- [45] Chinese translation of Ford's GDC 2017 talk (kevinan for Tencent GAD), Sohu, 2017, https://www.sohu.com/a/148848770_466876, article
- [46] Networking Scripted Weapons and Abilities in Overwatch (session page; slides), Dan Reed (Blizzard), 2017, https://gdcvault.com/play/1024653/Networking-Scripted-Weapons-and-Abilities ; https://media.gdcvault.com/gdc2017/Presentations/Reed_Dan_NetworkingScriptedWeapons.pdf, talk and slides
- [47] Shared World Shooter: Destiny's Networked Mission Architecture, Justin Truman (Bungie), 2015, https://media.gdcvault.com/gdc2015/presentations/Truman_Justin_Shared_World_Shooter.pdf, slides
- [48] Getting Into the Guts of Berserk, Jeffrey Doering (Riot Games), 2022, https://technology.riotgames.com/news/getting-guts-berserk, dev blog
- [49] Welcome to the Death Dimension, Joshua Parker (Riot Games), 2020, https://technology.riotgames.com/news/welcome-death-dimension, dev blog
- [50] ORIGINS: Sylas (2022); Reflecting on Reflection: a Mel Medarda tech blog (Cliff Zhu, 2025), Riot Games, https://www.leagueoflegends.com/en-us/news/dev/origins-sylas/ ; https://www.riotgames.com/en/news/mel-reflect-tech-blog, dev blog
- [51] Automated Testing for League of Legends (Jim Merrill, 2016); A Taxonomy of Tech Debt (Bill Clark, 2018), Riot Games, https://technology.riotgames.com/news/automated-testing-league-legends ; https://technology.riotgames.com/news/taxonomy-tech-debt, dev blog
- [52] Determinism in League of Legends: Introduction; Implementation; Fixing Divergences (Rick Hoskinson), Riot Games, 2017-2018, https://www.riotgames.com/en/news/determinism-league-legends-introduction ; https://www.riotgames.com/en/news/determinism-league-legends-implementation ; https://www.riotgames.com/en/news/determinism-league-legends-fixing-divergences, dev blog
- [53] Peeking into VALORANT's Netcode, Riot Games, 2020, https://technology.riotgames.com/news/peeking-valorants-netcode ; https://www.riotgames.com/en/news/peeking-valorants-netcode, dev blog
- [54] script_help2.lua (dump of Dota 2's VScript API help), ModDota, n.d., https://github.com/ModDota/API/blob/master/dump/script_help2.lua, engine source
- [55] Ability Lua Tutorial 4: Troll Warlord's Fervor, Elfansoer (DOTAFire), 2018, https://www.dotafire.com/dota-2/blog/elfansoer/ability-lua-tutorial-4-troll-warlords-fervor, article
- [56] Damage types, Dota 2 Wiki (Fandom), n.d., https://dota2.fandom.com/wiki/Damage_types, community wiki
- [57] Spell Batching in Classic (Kaivax, 2019); WoW Classic Version 1.13.7 PTR, Blizzard WoW forums, https://us.forums.blizzard.com/en/wow/t/spell-batching-in-classic/137118 ; https://us.forums.blizzard.com/en/wow/t/wow-classic-version-1137-ptr-is-now-available/838945, forum and release notes
- [58] How Blizzard coped with World of Warcraft's blood plague, Steven Messner (PC Gamer), 2019, https://www.pcgamer.com/how-blizzard-coped-with-world-of-warcrafts-blood-plague-and-other-early-disasters/, article
- [59] What Makes Apex Tick, Respawn/EA, 2021, https://www.ea.com/games/apex-legends/news/servers-netcode-developer-deep-dive, dev blog
- [60] Back to the Future! Working with Deterministic Simulation in For Honor (slides; session page), Jennifer Henry (Ubisoft Montreal), 2019, https://media.gdcvault.com/gdc2019/presentations/Henry_Jennifer_BackToTheFuture.pdf ; https://gdcvault.com/play/1026077/Back-to-the-Future-Working, slides and talk
- [61] Automated Testing of Gameplay Features in Sea of Thieves (session page; slide-text mirror of a Masella deck), Robert Masella (Rare), 2019, https://gdcvault.com/play/1026366/Automated-Testing-of-Gameplay-Features ; https://www.sambuz.com/doc/automated-testing-for-multiplayer-game-ai-automated-ppt-presentation-901544, talk and slides
- [62] V Rising: Behind the vampire realm built on 1,600 ECS systems, Unity, 2024, https://unity.com/case-study/v-rising, article
- [63] Dimming the Radiant AI in Oblivion, GamesBeat, 2010, https://gamesbeat.com/dimming-the-radiant-ai-in-oblivion/, article
#### Emergent and data-driven games
- [64] DromEd Act/React Receptrons, Thief Wiki (Fandom; Wayback copy 2024), n.d., http://web.archive.org/web/20240607011944/https://thief.fandom.com/wiki/DromEd/Properties/Act_React/Receptrons, community wiki
- [65] Stim/Response, The Dark Mod Wiki (by the system's programmer), n.d., https://wiki.thedarkmod.com/index.php?title=S/R, community wiki
- [66] A Data-Driven Game Object System (slides; paper), Scott Bilas (Gas Powered Games), 2002, http://web.archive.org/web/20170921215751/http://scottbilas.com:80/files/2002/gdc_san_jose/game_objects_slides.pdf ; http://web.archive.org/web/20171215081637/http://scottbilas.com:80/files/2002/gdc_san_jose/game_objects_paper.pdf ; https://www.gamedevs.org/uploads/data-driven-game-object-system.pdf, slides and paper
- [67] Caves of Qud with Brian Bucklew (transcript), Software Engineering Daily, 2025, https://softwareengineeringdaily.com/wp-content/uploads/2025/01/SED1801-Caves-of-Qud.txt, talk
- [68] Modding:Events, Caves of Qud Wiki, n.d., https://wiki.cavesofqud.com/wiki/Modding:Events, community wiki
- [69] The Spring Molting update is now live, Freehold Games, 2024, https://freeholdgames.itch.io/cavesofqud/devlog/744713/the-spring-molting-update-is-now-live, release notes
- [70] Some notes on programming objects in The Sims, Kenneth D. Forbus and Will Wright, 2001, https://qrg.northwestern.edu/papers/Files/Programming_Objects_in_The_Sims.pdf, paper
- [71] UO's resource system, Raph Koster, 2006, https://raphkoster.com/?p=517, dev blog
- [72] The Simulation Dream, Tynan Sylvester, 2013, https://www.gamedeveloper.com/design/the-simulation-dream, dev blog
- [73] Dwarf Fortress bug tracker: issues 9195 and 3259, Dwarf Fortress bug tracker, 2010-2025, https://dwarffortressbugtracker.com/view.php?id=9195 ; https://dwarffortressbugtracker.com/view.php?id=3259, issue tracker
- [74] 700,000 lines of code, 20 years, and one developer: How Dwarf Fortress is built (Q&A with Tarn Adams), Stack Overflow Blog, 2021, https://stackoverflow.blog/2021/07/28/700000-lines-of-code-20-years-and-one-developer-how-dwarf-fortress-is-built/, article
- [75] Minecraft Snapshot 22w11a; Minecraft Snapshot 24w33a, Mojang, 2022 and 2024, https://www.minecraft.net/en-us/article/minecraft-snapshot-22w11a ; https://www.minecraft.net/en-us/article/minecraft-snapshot-24w33a, release notes
- [76] MC-11193 (mojira.dev mirror of Mojang's tracker), Mojang, 2013-2024, https://mojira.dev/MC-11193, issue tracker
- [77] Scripting Execution Privilege, Minecraft Creator docs (Microsoft Learn), n.d., https://learn.microsoft.com/en-us/minecraft/creator/documents/scripting/execution-privilege, official docs
- [78] Friday Facts #88, #204 and #421, Wube Software, 2015-2024, https://factorio.com/blog/post/fff-88 ; https://factorio.com/blog/post/fff-204 ; https://factorio.com/blog/post/fff-421, dev blog
- [79] Friday Facts #60, #186, #315 and #340, Wube Software, 2014-2020, https://factorio.com/blog/post/fff-60 ; https://factorio.com/blog/post/fff-186 ; https://www.factorio.com/blog/post/fff-315 ; https://www.factorio.com/blog/post/fff-340, dev blog
- [80] Factorio Lua API: Data Lifecycle; Libraries and functions, Wube Software, n.d., https://lua-api.factorio.com/latest/auxiliary/data-lifecycle.html ; https://lua-api.factorio.com/latest/auxiliary/libraries.html, official docs
- [81] Crusader Kings 3 Dev Diary #30: Event Scripting (Wayback copy), Wokeg (Paradox), 2020, http://web.archive.org/web/20210725130228/https://forum.paradoxplaza.com/forum/developer-diary/crusader-kings-3-dev-diary-30-event-scripting.1397140/, dev blog
- [82] CK3 modding notes 7 (ck3mod制作笔记 7), kunlii, 2020, https://kunlii.github.io/blog/2020/12/24/ck3mod%E5%88%B6%E4%BD%9C%E7%AC%94%E8%AE%B0-7, article
- [83] Tiger README, amtep, n.d., https://github.com/amtep/tiger, other
- [84] Breath of the Wild, science, and clever little lies, Daniel New (Thumbsticks), 2017, https://www.thumbsticks.com/gdc-17-breath-of-the-wild-science-lies, article
- [85] Noita: a Game Based on Falling Sand Simulation (Q&A with Petri Purho), 80.lv, 2019, https://80.lv/articles/noita-a-game-based-on-falling-sand-simulation/, article
#### Rules engines and card games
- [86] Magic: The Gathering Comprehensive Rules (effective Nov 6, 2026), Wizards of the Coast, 2026, https://media.wizards.com/2026/downloads/MagicCompRules%2020261106.txt, rules text
- [87] On Whiteboards, Naps, and Living Breakthrough, Alex Werner (Wizards of the Coast), 2023, https://magic.wizards.com/en/news/mtg-arena/on-whiteboards-naps-and-living-breakthrough, dev blog
- [88] MTG Arena Bugs & Product Suggestions forum (player reports), Wizards of the Coast, n.d., https://feedback.wizards.com/forums/918667-mtg-arena-bugs-product-suggestions/suggestions/49598225-bug-please-take-a-different-action-or-the-game, issue tracker
- [89] XMage GameImpl.checkInfiniteLoop (commit 5769a6c), magefree, n.d., https://github.com/magefree/mage/blob/5769a6cbf0dedfe33fae414af98c4950bdcf283f/Mage/src/main/java/mage/game/GameImpl.java#L2035-L2083, engine source
- [90] TrinityCore Unit.cpp; SpellDefines.h; SpellMgr.h (commit c4192c0), TrinityCore, n.d., https://github.com/TrinityCore/TrinityCore/blob/c4192c00729bfb1edf9d68dcb88e0a33b4c53e33/src/server/game/Entities/Unit/Unit.cpp#L5606-L5611 ; https://github.com/TrinityCore/TrinityCore/blob/c4192c00729bfb1edf9d68dcb88e0a33b4c53e33/src/server/game/Spells/SpellDefines.h#L286 ; https://github.com/TrinityCore/TrinityCore/blob/c4192c00729bfb1edf9d68dcb88e0a33b4c53e33/src/server/game/Spells/SpellMgr.h#L255-L292, engine source
- [91] Patch Notes 1.2.2; Path of Exile 2 Patch 0.1.1 Patch Note Preview, Grinding Gear Games, 2014 and 2025, https://www.pathofexile.com/forum/view-thread/1049640 ; https://www.pathofexile.com/forum/view-thread/3695606, release notes
- [92] Hearthstone Wiki: Advanced rulebook; Game mechanics updates; Turn, wiki.gg (community), n.d., https://hearthstone.wiki.gg/wiki/Advanced_rulebook ; https://hearthstone.wiki.gg/wiki/Game_mechanics_updates ; https://hearthstone.wiki.gg/wiki/Turn, community wiki
- [93] Patch 22.0 Card Effect Priority Update, Blizzard Hearthstone forums, 2021, https://us.forums.blizzard.com/en/hearthstone/t/patch-220-card-effect-priority-update/76118, release notes
- [94] Spell (Legends of Runeterra), League of Legends Wiki, n.d., https://wiki.leagueoflegends.com/en-us/LoR:Spell, community wiki
- [95] Runeterra Patch 1.14 notes (Riot text reprinted by Out of Games), n.d., https://outof.games/news/2174-runeterra-patch-114-7-champion-updates-friend-spectating-gauntlets-goes-best-of-3-for-tournament-preparations/, release notes
- [96] Oracle's Eye, Riot Games Player Support, n.d., https://support.riotgames.com/en-us/legends-of-runeterra/gameplay/oracle-s-eye, official docs
- [97] Action Queue (BasicMod modding wiki for Slay the Spire), Alchyr, n.d., https://github.com/Alchyr/BasicMod/wiki/Action-Queue, community wiki
#### Determinism, rollback and replay
- [98] Quantum 3 manual: Systems; Events & Callbacks; Events vs Polling, Photon Engine, n.d., https://doc.photonengine.com/quantum/current/manual/quantum-ecs/systems ; https://doc.photonengine.com/quantum/current/manual/quantum-ecs/game-events ; https://doc.photonengine.com/quantum/v3/concepts-and-patterns/events-vs-polling, official docs
- [99] Quantum 3 manual: Frame Timer; Fixed Point; Testing, Photon Engine, n.d., https://doc.photonengine.com/quantum/v3/concepts-and-patterns/frame-timer ; https://doc.photonengine.com/quantum/current/manual/quantum-ecs/fixed-point ; https://doc.photonengine.com/quantum/v3/manual/testing, official docs
- [100] Netcode for Entities 1.6: Introduction to prediction; Managing latency with prediction; Prediction edge cases, Unity, n.d., https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/intro-to-prediction.html ; https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/prediction-n4e.html ; https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/prediction-details.html, official docs
- [101] GGPO Developer Guide, pond3r/ggpo, n.d., https://github.com/pond3r/ggpo/blob/master/doc/DeveloperGuide.md, official docs
- [102] bevy_ggrs docs: Rollback Pitfalls, gschup/bevy_ggrs, n.d., https://github.com/gschup/bevy_ggrs/blob/main/docs/pitfalls.md, official docs
- [103] Fusion 1 manual: FixedUpdateNetwork, Photon Engine, n.d., https://doc.photonengine.com/fusion/v1/manual/network-object/fixed-update-network, official docs
- [104] 1500 Archers on a 28.8: Network Programming in Age of Empires and Beyond, Paul Bettner and Mark Terrano, 2001, https://www.gamedeveloper.com/programming/1500-archers-on-a-28-8-network-programming-in-age-of-empires-and-beyond, paper
- [105] Synchronous RTS Engines and a Tale of Desyncs (2011); The Tech of Planetary Annihilation: ChronoCam (2013), Forrest Smith, https://www.forrestthewoods.com/blog/synchronous_rts_engines_and_a_tale_of_desyncs/ ; https://www.forrestthewoods.com/blog/tech_of_planetary_annihilation_chrono_cam/, dev blog
- [106] Fix Your Timestep!; Snapshot Interpolation; Deterministic Lockstep, Glenn Fiedler, 2014 (last two), https://gafferongames.com/post/fix_your_timestep/ ; https://gafferongames.com/post/snapshot_interpolation/ ; https://gafferongames.com/post/deterministic_lockstep/, dev blog
- [107] Determinism, Erin Catto (box2d.org), 2024, https://box2d.org/posts/2024/08/determinism/, dev blog
- [108] The Go Programming Language Specification, go.dev, n.d., https://go.dev/ref/spec, official docs
- [109] FoundationDB documentation: Testing, Apple, n.d., https://apple.github.io/foundationdb/testing.html, official docs
- [110] Deterministic Simulation Testing (VOPR), TigerBeetle, n.d., https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/internals/vopr.md, official docs
- [111] Netcode explainer part 5 (summary of Michael Stallone's talk), Infil, 2019, https://words.infil.net/w02-netcode-p5.html, article
#### Critics and alternatives
- [112] Why isn't Godot an ECS-based game engine?, Juan Linietsky, 2021, https://godotengine.org/article/why-isnt-godot-ecs-based-game-engine/, dev blog
- [113] Approaching entity systems (post #6924), Jonathan Blow, Handmade Network forum, 2016, https://handmade.network/forums/t/1210/p/6900, forum
- [114] Thoughts on ECS, Dennis Gustafsson (Voxagon blog), 2025, https://blog.voxagon.se/2025/03/28/thoughts-on-ecs.html, dev blog
- [115] Leaving Rust gamedev after 3 years, LogLog Games, 2024, https://loglog.games/blog/leaving-rust-gamedev/, dev blog
- [116] Encompass docs: Why / Architecture / ECS; MECS, Moonside Games, 2019, https://gitea.moonside.games/MoonsideGames/encompass-cs-docs/src/branch/main/content/why/architecture/ecs.md ; https://gitea.moonside.games/MoonsideGames/encompass-cs-docs/src/branch/main/content/why/architecture/mecs.md, official docs
- [117] Deprecating the Observer Pattern, Ingo Maier, Tiark Rompf and Martin Odersky (EPFL), 2010, https://infoscience.epfl.ch/record/148043/files/DeprecatingObserversTR2010.pdf, paper
- [118] In-depth: Functional programming in C++ (2012); John Carmack on Inlined Code (2007 email), John Carmack, https://www.gamedeveloper.com/programming/in-depth-functional-programming-in-c- ; http://number-none.com/blow/john_carmack_on_inlined_code.html, dev blog and other
- [119] John Carmack Programming Haskell (fan transcription of the QuakeCon 2013 keynote), TeamLiquid, 2013, https://www.teamliquid.net/blogs/427090-john-carmack-programming-haskell?view=all, forum
- [120] The Next Mainstream Programming Language (POPL 2006 slides), Tim Sweeney (Epic Games), 2006, https://www.cs.princeton.edu/~dpw/popl/06/Tim-POPL.ppt, slides
- [121] QuakeTM: Parallelizing a Complex Sequential Application Using Transactional Memory, Gajinov et al., 2009, https://timharris.uk/papers/2009-ics.pdf, paper
- [122] The Verse Programming Language: Overview, Verse book, n.d., https://verselang.github.io/book/00_overview, official docs
- [123] The Yampa Arcade, Antony Courtney, Henrik Nilsson and John Peterson, 2003, https://www.antonycourtney.com/pubs/hw03.pdf, paper
- [124] Quake QuakeWorld source qw-qc/misc.qc, id Software, n.d., https://github.com/id-Software/Quake/blob/master/qw-qc/misc.qc, engine source
- [125] Amethyst: Starting fresh (Wayback copy), Erlend Sogge Heggen, 2021, http://web.archive.org/web/20211004104732/https://amethyst.rs/posts/amethyst--starting-fresh/, dev blog
- [126] Using Rust for Game Development (RustConf 2018 keynote, written version), Catherine West, 2018, https://kyren.github.io/2018/09/14/rustconf-talk.html, talk
#### Scale and contributor safety
- [127] SpatialOS docs: Authority and interest; Commands (archived 2021), Improbable, n.d., https://web.archive.org/web/20210928045444/https://documentation.improbable.io/spatialos-overview/docs/authority-and-interest ; https://web.archive.org/web/20210928052112/https://documentation.improbable.io/spatialos-overview/docs/commands, official docs
- [128] Space Station 14 Codebase Conventions; RobustToolbox AccessAttribute.cs, space-wizards, n.d., https://docs.spacestation14.com/en/general-development/codebase-info/conventions.html ; https://github.com/space-wizards/RobustToolbox/blob/master/Robust.Shared/Analyzers/AccessAttribute.cs, official docs and engine source
- [129] space-station-14 repository: CODEOWNERS; EntityTest.cs; SaveLoadSaveTest.cs, space-wizards, n.d., https://github.com/space-wizards/space-station-14/blob/master/.github/CODEOWNERS ; https://github.com/space-wizards/space-station-14/blob/master/Content.IntegrationTests/Tests/EntityTest.cs ; https://github.com/space-wizards/space-station-14/blob/master/Content.IntegrationTests/Tests/SaveLoadSaveTest.cs, other and engine source
- [130] Physical Design of The Machinery, Our Machinery, 2017, https://ruby0x1.github.io/machinery_blog_archive/post/physical-design/index.html, dev blog
- [131] Scaling Unity workflows: Lessons from medium to large projects, Matthew Wojtechko (Mega Cat Studios), 2026, https://unity.com/blog/scaling-workflows-lessons-from-medium-to-large-projects, dev blog
- [132] go-arch-lint README, fe3dback, n.d., https://github.com/fe3dback/go-arch-lint, other
- [133] Changes to our Contribution Policies, Godot Foundation, 2026, https://godotengine.org/article/contribution-policy-2026/, dev blog
- [134] AI Policy, Bevy contributor docs, n.d., https://bevy.org/learn/contribute/policies/ai/, official docs
- [135] Object system: objects, Mach engine docs, n.d., https://machengine.org/docs/object/objects/, official docs
#### Added after review
- [136] Wizards and warriors, part five, Eric Lippert, 2015, https://ericlippert.com/2015/05/11/wizards-and-warriors-part-five/, dev blog
- [137] The Specs Book: FlaggedStorage and modification events, Amethyst/specs project, n.d., https://github.com/amethyst/specs/blob/master/docs/tutorials/src/12_tracked.md, official docs
- [138] Source Multiplayer Networking (Valve Developer Community; archived 2024-12-25), Valve, n.d., https://web.archive.org/web/20241225081104/https://developer.valvesoftware.com/wiki/Source_Multiplayer_Networking, official docs
