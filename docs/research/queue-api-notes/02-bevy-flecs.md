# 02 — Bevy and Flecs: queues, deferral, events, delays

Researcher notes, 2026-11-19. Versions read: Bevy v0.20.1 (tag, 2026-10-28), Flecs v4.2.0 (tag, 2026-11-04).
Source clones (read-only): local clones of bevy and flecs (not kept)
Marks: (V) read primary source this session, (S) secondary/unread primary, (I) my inference.

Status: complete. Sections 1-7 Bevy, 8-10 Flecs, 11 synthesis for Cardinal, 12 gaps. [B1-reader] = message_reader.rs (see [B21]).

## Sources

- [B1] Bevy `Messages<M>` source, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/message/messages.rs
- [B2] Bevy message update systems, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/message/update.rs and .../message/message_registry.rs
- [B3] Bevy `SubApp::default` (schedules message update in `First`), v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_app/src/sub_app.rs
- [B4] Bevy `TimePlugin` (fixed-step message signal, delayed-command check), v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/lib.rs
- [B5] Bevy `DelayedCommands`, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/delayed_commands.rs
- [B6] Bevy PR #23090 "Add a DelayedCommands helper" (merged 2026-02-25) + comments. https://github.com/bevyengine/bevy/pull/23090
- [B7] Bevy issue #15129 (timed commands request) + comments; PR #20155 (closed draft); PR #15136 (closed); PR #24179 (cancel handle, closed unmerged). https://github.com/bevyengine/bevy/issues/15129 ; https://github.com/bevyengine/bevy/pull/20155 ; https://github.com/bevyengine/bevy/pull/15136 ; https://github.com/bevyengine/bevy/pull/24179

## 1. Bevy Messages (buffered events) — double buffer and drop

API shape (V) [B1]:
- `pub struct Messages<M: Message> { messages_a, messages_b: MessageSequence<M>, message_count: usize }` — a Resource. One per message type.
- `fn write(&mut self, m: M) -> MessageId<M>`; `write_batch`; `write_default`.
- Readers: `MessageReader<M>` system param = per-system cursor (`last_message_count`). `read()`, `read_with_id()`, `par_read()`. `MessageCursor::missed_messages(&Messages) -> usize` exists.
- `fn update(&mut self)`: "Swaps the message buffers and clears the oldest message buffer. In general, this should be called once per frame/update." (V)
- `update_drain` returns removed messages (for whoever wants them).

Drop behaviour (V) [B1]:
- "Messages will persist across a single frame boundary and so ordering of message producers and consumers is not critical (although poorly-planned ordering may cause accumulating lag). If messages are not handled by the end of the frame after they are updated, they will be dropped silently."
- "MessageReaders that read at least once per update will never drop messages."
- "If no ordering is applied between writing and reading systems, there is a risk of a race condition."
- Buffers "grow indefinitely if update is never called".

Where `update` runs (V) [B2][B3][B4]:
- `SubApp::default` adds `message_update_system` to `First`, gated by `message_update_condition`.
- Registry only updates a type if it changed or was updated last time (`previously_updated || has_changed`) — skips idle types.
- Fixed step fix: `TimePlugin` adds `signal_message_update_system` to `FixedPostUpdate`, and starts the registry in `Waiting`. Comment: "Ensure the messages are not dropped until `FixedMain` systems can observe them." So buffers only swap in a frame after at least one fixed tick ran.
- (I) Consequence: with fixed step, a message may be seen by a fixed system on 2 consecutive fixed ticks if no swap happened between them (the reader cursor prevents double-read per reader, but life spans vary with frame/tick ratio). The lifetime of a message is tied to the variable frame, not the sim tick.

Takeaways for Cardinal (I):
- Cardinal has one fixed tick, no render frame. So a plain "cleared at tick end" one-tick event is simpler and fully deterministic. Bevy's double buffer exists because readers run before writers in frame order; Cardinal's plan instead tests "every reader runs after every sender", which removes the need for a second buffer.
- Silent drop is Bevy's main complaint. Cardinal's test-based ordering check is stronger than Bevy's "persist one extra frame".

## 2. Bevy DelayedCommands (new in 0.19/0.20) — the closest built-in to a "due tick" queue

API shape (V) [B5]:
- `commands.delayed()` -> `DelayedCommands<'w,'s> { queues: HashMap<Duration, CommandQueue>, commands: Commands }`.
- `.duration(d) -> Commands` and `.secs(f32) -> Commands`: a `Commands` rebound to a queue for that delay. Any normal command can be delayed.
- On `Drop`, each queue becomes an entity with `DelayedCommandQueue { submit_at: Duration, queue: CommandQueue }`. `submit_at` = relative delay + `Time::elapsed()` at apply time.
- `check_delayed_command_queues` runs in `PreUpdate`: `if queue.submit_at <= elapsed { commands.append(&mut queue.queue); commands.entity(e).despawn(); }`.
- Doc: "Delayed commands are currently checked against the default clock in the PreUpdate schedule. There's currently no way to specify different clocks ... you'll likely have to implement your own delay system."
- "Entity allocation happens immediately even if the spawn command is delayed."
- `queue` field is `#[reflect(ignore)]` -> not serializable. (V)

Pitfalls (V unless marked):
- Time-based, not tick-based. Checked once per frame in `PreUpdate`, not in `FixedPreUpdate`. Reviewers asked for fixed-time support from the start: ItsDoot: "We'll also want to support delayed commands in both Update and FixedUpdate." [B7 #20155]. Alice/Runi "tried to hash out designs for 'make this generic over the clock' ... but weren't satisfied with any solution" [B6]. Proposal was `DelayedCommands<T = ()>` with "one for Fixed (this one goes in FixedPreUpdate)" — not shipped.
- Due ones fire in query iteration order, not sorted by `submit_at` or by enqueue order. (I, from the loop over `Query<(Entity, &mut DelayedCommandQueue)>` [B5])
- Not cancellable in core: PR #24179 (`DelayedCommandHandle`) closed unmerged. (V, state=closed, merged_at=None) Workaround = despawn the queue entity.
- Opaque closures cannot be saved or inspected -> not snapshot-safe. (I from reflect(ignore))
- App exit with pending queues logged "CommandQueue has un-applied commands" (#24346), fixed by silencing (#24347). (V titles)
- Critic: iiYese on #15129: "I can't think of a context for this where it isn't just a foot gun. It's the equivalent of doing `sleep(duration)` in conventional engines." (V)
- benfrankel on #15129: "A deferred command can't assume that the world is in the same state as when it was issued ... whether it runs later in the frame (Commands) or in a later frame (TimedCommands)." (V) -> delayed requests must re-validate targets.
- ThierryBerger on #15129 preferred plain data: component with a `Timer` + an event when it finishes, because it gives "cancellation (despawn the entity with the timer...)", "reflection", inspector support. (V)

Takeaways (I):
- Bevy's delayed queue is one entity per (system run, delay). Matches Cardinal's "due tick on request" only loosely. Cardinal should store due as an integer tick in plain data (snapshot-safe, inspectable, cancellable by removing the request), and drain at a fixed point. This is exactly the gap Bevy's reviewers named (fixed clock, cancel, reflection).
- Sort due requests by (due tick, then a stable key) before applying; Bevy does not, which is fine for Bevy (no determinism goal) but not for Cardinal.

More sources:
- [B8] Bevy `Commands`/`EntityCommands` docs, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/system/commands/mod.rs
- [B9] Bevy `ApplyDeferred` and executors, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/executor/mod.rs ; .../executor/multi_threaded.rs
- [B10] Bevy `ScheduleBuildSettings`, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/schedule.rs (line ~1839)
- [B11] Bevy CI ambiguity test, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/tests/ecs/ambiguity_detection.rs
- [B12] Bevy observer docs (`Observer`), v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/observer/distributed_storage.rs ; Event docs https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/event/mod.rs
- [B13] Bevy issue #19569 "Observers don't respect the 'stack nature' of commands" (open, 2025-06-10). https://github.com/bevyengine/bevy/issues/19569
- [B14] Bevy issue #14890 "Observer ordering/scheduling" (open, 2024-08-23). https://github.com/bevyengine/bevy/issues/14890
- [B15] Bevy issue #7691 "Events can be missed or double-counted by fixed time step systems" (closed 2024-06-19). https://github.com/bevyengine/bevy/issues/7691
- [B16] bevy_ggrs "Rollback Pitfalls" (main). https://github.com/gschup/bevy_ggrs/blob/main/docs/pitfalls.md

## 3. Bevy Commands — deferred application and ordering

API shape (V) [B8][B9]:
- `Commands` = system param wrapping a per-system `CommandQueue`. Methods: `spawn`, `entity(e).insert/remove/despawn`, `queue(impl Command)`, `trigger(event)`, `append(&mut CommandQueue)`.
- `Command` trait: `fn apply(self, world: &mut World)` (may return `Result`). Closures `|w: &mut World| {..}` are commands.
- Applied "when the `ApplyDeferred` system runs". "Commands are executed one-by-one so that each command can have exclusive access to the World."
- EntityCommands are fallible: "an entity you're trying to change ... can be despawned by the time the command is executed. All deferred entity commands will check whether the entity exists at the time of execution and will return an error." Default error handler panics; configurable (`FallbackErrorHandler`, `queue_handled`).

Sync points (V) [B9][B10]:
- `ApplyDeferred` auto-inserted "between systems with dependencies if the dependency has deferred buffers", plus at schedule end ("if ... scheduled in Update, all the changes will be visible in PostUpdate").
- `auto_insert_apply_deferred: bool` (default true) can be turned off to "only sync deferred params at the end of the schedule, or ... manually insert all your sync points".
- Order of applying queues from many systems: executor loops `for system_index in unapplied_systems.ones()` -> topological index order, not finish order. Doc: "Modifying a Schedule may change the order buffers are applied." (V)
- (I) So Bevy command order across systems is deterministic for a fixed schedule build but changes when systems are added. Within one queue it is FIFO. Nothing sorts by target or key.

Takeaways (I):
- Bevy's commands are opaque closures applied in schedule order. Cardinal's plan (typed requests with data, drained by the owner, sorted) is stricter: order is a property of the data, not of which system happened to be indexed first. Good.
- Bevy's "check entity exists at apply time, else error" is the right default for requests that target entities. Cardinal should drop (and count/log) requests whose target died, not panic.

## 4. Bevy observers / triggers

API shape (V) [B12]:
- `#[derive(Event)] struct Speak{..}`; `world.add_observer(|e: On<Speak>| ..)`; `world.trigger(Speak{..})`; `commands.trigger(..)`.
- `EntityEvent` targets one entity; opt-in bubbling ("propagation").
- Timing: "Observers triggered via World::trigger are evaluated immediately, as are all commands they queue up. Observers triggered via Commands::trigger are evaluated at the next sync point."
- Nested: "When the commands are flushed (including these 'nested triggers') they will be recursively evaluated until there are no commands left, meaning nested triggers all evaluate at the same time!"
- Order: "Bevy does not provide a way to specify the relative ordering of observers watching for the same event. Their ordering is considered to be arbitrary." (issue #14890 open since 2024-08) [B14]
- "Commands sent by observers are currently not immediately applied. Instead, all queued observers will run, and then all of the commands from those observers will be applied." [B13 still open]
- Hooks vs observers: add -> hooks first then observers; remove -> observers first then hooks.
- No depth or recursion limit found in observer/event/command_queue source (grep for depth/recursion/MAX_: none). (V, absence by grep)

Complaints (V):
- #19569: "I don't want my projectiles to hit more than one target." Observers ran before earlier observers' commands -> "When unexpected ordering caused me to trigger an event on a despawned entity, the game crashed." Workaround: each observer only queues one command, so all logic runs "in command-land".
- #14890: "Sometimes an observer can rely on things added by other observers. But it is currently not possible to explicitly order the observers."

Takeaways (I):
- Immediate observers give the "chain resolves this tick" behaviour but no order, no depth cap, and surprise interleaving. Cardinal's design (requests wait for the owner's drain point; same-tick only if owner runs later) avoids all three. The cost is latency of up to one tick per hop for cross-feature chains where the owner already ran. That latency is the intended trade, and it is what makes depth caps rarely fire.
- Depth cap: Bevy has none, so a runaway observer loop is a stack overflow / hang. Cardinal's `depth` field is a real addition, not a copy.

## 5. Bevy ambiguity detection and shuffle (the "reader after sender" check)

(V) [B10][B11]:
- `ScheduleBuildSettings.ambiguity_detection: LogLevel` — "systems with conflicting access but indeterminate order". Default `Ignore`.
- Fine-grained escapes: `.ambiguous_with(system)` (used by TimePlugin for `time_system` vs `message_update_system` [B4]).
- Bevy CI test asserts zero ambiguities for DefaultPlugins in main and render apps: `assert_eq!(main_app_ambiguities.total(), 0, ...)`. It disables `auto_insert_apply_deferred` for the check because sync points "can cause two ambiguous systems to become accidentally ordered".
- `shuffle_seed: Option<u64>` (debug feature): "systems will be shuffled according to the given seed ... if you spot erroneous behavior when shuffling, that is an indication that the 'default' ordering is correct by chance".
- Message readers take `Res<Messages<T>>`, writers `ResMut` -> unordered writer+reader shows up as an ambiguity. (I, from MessageReader concurrency doc: readers can't run "concurrently with MessageWriter<T>" [B1-reader])

Takeaways (I):
- Cardinal's planned test ("every reader runs after every sender") is a narrower, stronger version of Bevy's ambiguity check: Bevy only says "unordered", Cardinal requires a specific direction. Since Cardinal is single-threaded in registration order, the check can be a static assertion over registration indices.
- Copy Bevy's lesson: the check must run in CI over the full real app, with an explicit allow-list (like `ambiguous_with`) rather than a global off switch.
- A shuffle mode is cheap for Cardinal too: shuffle systems within a phase in tests to find order that holds by chance. (I)

## 6. Bevy + rollback (bevy_ggrs) — what must live in snapshots

(V) [B16]:
- "Bevy does not guarantee a stable iteration order for queries ... Always sort queries that mutate state." Provides `RollbackOrdered` for a stable key.
- "Bevy's `Messages<T>` resource is not snapshotted. Messages written during a frame that later gets rolled back are not retracted, and resimulating that frame writes them again ... Do not use MessageWriter / MessageReader inside GgrsSchedule. Instead: Use a component or resource to communicate state changes between rollback systems."
- "`Local<T>` is per-system state that is not snapshotted."
- "Entity IDs can change when entities are despawned and re-created during rollback."

Takeaways (I):
- Strong support for Cardinal's design: request queues and delayed-event queues are components, so they are in the snapshot. Only one-tick events live outside, and they are cleared each tick, so a restore at a tick boundary loses nothing.
- Requirement: queues must be empty-or-saved at the snapshot point. Pending delayed events and due-later requests must be in components (they are). Any system-local state (Go closures, maps in system structs) must not hold pending work.
- Sort key must not rely on raw entity IDs if IDs can differ after restore. Cardinal restores IDs from the snapshot (I — verify in Cardinal), so (due, target entity, source entity, seq) is OK there.

More sources:
- [B17] Bevy `Timer`, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/timer.rs
- [B18] Bevy time run conditions (`on_timer`), v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/common_conditions.rs
- [B19] Bevy `Time<Fixed>`, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_time/src/fixed.rs
- [B20] Bevy schedule run conditions, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/schedule/condition.rs

## 7. Bevy Timer, run conditions, fixed step

Timer (V) [B17]:
- `Timer::new(Duration, TimerMode::{Once,Repeating})`; `tick(delta) -> &Self`; `just_finished()`, `is_finished()`, `times_finished_this_tick() -> u32`, `pause`, `reset`, `remaining()`.
- "in order to advance the timer `tick` MUST be called." Timer is plain data in a component; each user ticks it in a system.
- "Repeating timers might elapse multiple times per frame if the time is advanced by more than the timer duration." -> `times_finished_this_tick`.

Run conditions (V) [B18][B20]:
- `system.run_if(on_timer(Duration::from_secs(1)))` — "does not guarantee that systems will run at exactly the specified interval. If delta time is larger than the specified duration then the system will only run once even though the timer may have completed multiple times."
- `on_timer` keeps its Timer inside the closure (`impl FnMut`) -> hidden, not in the world. (I) Not snapshot-safe.
- `on_message::<M>` consumes messages from its own reader so it isn't re-true; `run_once(Local<bool>)`; `resource_changed`.
- Pitfall doc: short-circuit `and` + conditions with `Local<T>` or `MessageReader<T>` "may not be updated every time the combined condition is evaluated."
- `PopulatedMessageReader<T>` "to skip the system if there are no messages." [B1-reader]

Fixed step (V) [B19]:
- `FixedUpdate` "may run 0, 1 or more times during a single update". Default 64 Hz, chosen to avoid a 60 Hz "pathological interaction with the monitor refresh rate".
- `Time` inside FixedUpdate reports `delta == timestep`, elapsed grows by one timestep per run.

Takeaways (I):
- Bevy has no integer tick counter API for due times; everything is `Duration`. For Cardinal at fixed 20 Hz, a `uint64` tick is the natural due field. Avoid durations in requests; convert at the edge (`ticksFor(d)`), round once.
- "Skip the drain when the queue is empty" is a standard Bevy pattern (`run_if`, `PopulatedMessageReader`). For Cardinal, the drain system can early-return on `Len()==0`; cheap if the queue is one singleton per owner, not one per entity (matches the user's +130us per-entity-queue measurement).
- Timers that must survive restore belong in components (Bevy keeps them in components too); run-condition-local timers are a known hidden-state trap.

## Flecs sources

- [F1] Flecs Systems manual (v4.2.0): pipelines, staging, sync points, immediate systems, timers. https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/Systems.md
- [F2] Flecs Manual, "Deferred operations" (v4.2.0). https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/Manual.md#deferred-operations
- [F3] Flecs v4.2.0 release notes (2026-11-04), incl. breaking changes. https://github.com/SanderMertens/flecs/releases/tag/v4.2.0
- [F4] Flecs command queue source (v4.2.0). https://github.com/SanderMertens/flecs/blob/v4.2.0/src/commands.c ; .../src/commands.h ; .../src/stage.c ; .../src/stage.h
- [F5] `FLECS_DAG_DEPTH_MAX` (v4.2.0). https://github.com/SanderMertens/flecs/blob/v4.2.0/include/flecs.h (line ~362) ; https://github.com/SanderMertens/flecs/blob/v4.2.0/src/observable.c (line ~720)

## 8. Flecs deferred mode, stages and command merging

API shape (V) [F2][F3]:
- v4.2.0 breaking change: "Implicit defer mode is removed from the core and the C++ API." Also "Rename ecs_is_deferred to ecs_is_stage", C++ `entity::mut()` -> `entity::stage()`.
- Now: "Pass an explicit stage to enqueue ECS operations. Pass the world to execute operations immediately. The world has no deferred mode, and stages always enqueue operations."
- `ecs_world_t *stage = ecs_get_stage(world, 0); ecs_add(stage, e, Position); ecs_merge(stage);`
- In systems/observers: `it->stage` queues, `it->world` acts on the real world. "Each application thread must use its own stage."
- "Creating a staged entity allocates its identifier immediately. Commands targeting an entity deleted before their execution are discarded. Commands that parent an entity to a deleted parent delete the child during the merge." (V)
- (I) The move from implicit to explicit staging is a reversal of the old design (Systems.md still says "When calling progress() the world enters a readonly state in which all ECS operations ... are enqueued"). The builder chose explicit "where does this write go" over "same call means different things inside a system". That matches the survey's complaint [7] that the same call acted differently inside and outside a system.

Command record (V) [F4]:
- `typedef struct ecs_cmd_t { ecs_cmd_kind_t kind; int32_t next_for_entity; ecs_id_t id; ecs_cmd_entry_t *entry; ecs_entity_t entity; union{...} is; ecs_entity_t system; /* System that enqueued the command */ } ecs_cmd_t;`
- Note the `system` field: each command records who enqueued it (for debugging). Analog of Cardinal's `source`.
- `ecs_on_commands_action_t` callback "used to capture commands of a frame" -> hook to observe/record the queue (debug, replay).

Merging / batching (V) [F4]:
- `flecs_cmd_new_batched` links all commands for the same entity into a per-entity chain (`next_for_entity`; first one marked by a negative sign). Caches last/first/second entity for speed; sparse map otherwise.
- At flush, `flecs_cmd_batch_for_entity` applies the whole chain for that entity at once: one table move for many add/remove, collecting a table diff for hooks/observers.
- `flecs_defer_find_cmd_value`: a later `set`/`ensure` on the same (entity, component) in the same queue finds the earlier queued value; `remove`/`delete`/`clear` reset it. Release note: "Repeated calls to ensure in deferred mode now return the same storage."
- Dead targets: flush checks `flecs_entities_is_alive(world, e)`; skip/discard counters (`world->info.cmd.discard_count`, `batched_entity_count`).
- Multi-stage merge: `for (i = 0; i < count; i ++) flecs_commands_flush(world, ecs_get_stage(world, i))` -> stages merged in stage index order. (V) Deterministic given the same split of work to stages. (I)

Nested queues (V) [F2][F4]:
- "Observer commands use nested command queues. When a command invokes an observer, commands queued by that observer, including commands from further nested observers, finish before the next command in the parent queue."
- Source: during flush, if the stage's current queue gets new commands, it saves `cursor = i + 1`, enters the nested queue, then resumes the parent (`goto enter` / `goto resume`). Queues are a linked list grown on demand (`flecs_commands_grow`) with no cap.
- This is depth-first — the exact behaviour Bevy #19569 asks for and Bevy lacks. (I, comparing [B13])
- No depth limit on nested command/observer chains. Only `FLECS_DAG_DEPTH_MAX` (128), which panics `CYCLE_DETECTED` on cyclic relationship graphs, not on event chains. (V)

Takeaways (I):
- Per-target batching is a Flecs strength: all requests for one entity applied together. For Cardinal: sorting requests by target entity first gives the same locality and lets an owner fold several requests into one write (e.g., sum damage) — "merge before apply".
- Store `source` on every request (Flecs stores `system`) — both for debugging and as a sort tie-breaker.
- Depth-first vs breadth-first: Flecs resolves consequences depth-first inside one flush; Cardinal's design resolves breadth-first by tick (a request waits for the owner's slot). Breadth-first is easier to make deterministic and to cap; depth-first gives "one projectile, one hit" for free. Cardinal gets "one hit" by having the owner consume and mark the projectile in its own drain (owner-only writes), not by stack order.

More Flecs sources:
- [F6] Flecs Observers manual (v4.2.0): usage limits, enqueue vs emit, execution, ordering. https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/ObserversManual.md
- [F7] Flecs timer addon header (v4.2.0). https://github.com/SanderMertens/flecs/blob/v4.2.0/include/flecs/addons/timer.h
- [F8] Designing with Flecs (v4.2.0): phases, declaration order, module ordering. https://github.com/SanderMertens/flecs/blob/v4.2.0/docs/DesignWithFlecs.md
- [F9] Flecs issue #768 "Enable a system to listen to an event such like a defered observer until its phase" (2022, closed) + author reply. https://github.com/SanderMertens/flecs/issues/768

## 9. Flecs observers — when not to use them

(V) [F6]:
- "for simple use cases, a basic event queue is always going to outperform observers."
- Limitations listed by the author: observers can't (easily) "Peek ahead in the event queue", "Have different event queues per entity (possible, but inefficient/difficult)", "Sort events before processing them", "Process events on multiple threads", "Bubble up events", "Stop event propagation".
- Bad use: "If you find yourself adding or removing components just to trigger observer events, that's a bad application ... it would also be unreliable because features like command batching impact how and when events are emitted."
- "if you can solve something with a system, it should probably be solved with a system ... systems are much more efficient to run, and have more predictable performance."
- Hooks vs observers: one hook per event/component; "Hooks can mutate a component"; "Observers should never mutate a component".
- Enqueue vs emit: `ecs_emit` invokes observers now; `ecs_enqueue(stage, desc)` copies the event into the command queue (copy hook / memcpy); on the real world, enqueue == emit.
- Execution: observers run when the operation runs; with deferral "most observers will also be invoked during sync points".
- Ordering rules: "Observer order is undefined ... Applications should never rely on observer order, not even if the observed order is apparently 'correct'". "Event order is undefined between entities" (so the queue can batch per entity). "OnAdd & OnRemove order is undefined" even within one entity. "OnSet order is maintained". "Custom event order is maintained."

Author on event entities (V) [F9]:
- User wanted "full control of execution order between observers" and proposed a "transient" entity per delayed event, swept by a system and deleted at end of frame. Sander Mertens: "Yep, that is a common pattern that other people have used as well. The performance should be good".

Takeaways (I):
- The Flecs author's own list of what observers can't do (sort, peek, per-entity queues, threads) is the feature list of Cardinal's request queue. Strong primary-source support for "queue component drained by a system" over observers for gameplay writes.
- Flecs keeps custom-event order (FIFO) but not cross-entity order. Cardinal needs a full sort for restore-determinism; Flecs only guarantees determinism by being single-threaded + FIFO. FIFO is enough only if every producer runs in a fixed order — which Cardinal has (registration order). So the sort may be cheap insurance rather than strictly required; keep it but make the key cheap (see §11).

## 10. Flecs timers, rate filters, tick sources, pipeline

API shape (V) [F7][F1]:
- Components on entities (plain data, so serializable): `EcsTimer { ecs_ftime_t timeout, time, overshoot; int32_t fired_count; bool active, single_shot; }`, `EcsRateFilter { ecs_entity_t src; int32_t rate, tick_count; ecs_ftime_t time_elapsed; }`, `EcsTickSource` (`tick` bool).
- `ecs_set_timeout(world, tick_source, timeout)` (single shot), `ecs_set_interval`, `ecs_start_timer/stop/reset`, `ecs_set_rate(world, e, rate, source)`, `ecs_set_tick_source(world, system, tick_source)`, `ecs_randomize_timers(world)`.
- Per-system shorthand: `.interval = 1.0` or `.rate = 2`.
- Determinism quote: "Rate filters enable deterministic system execution which cannot be achieved with interval timers alone. For example, if timer A has interval 2.0 and timer B has interval 4.0, it is not guaranteed that B will tick at exactly twice the multiple of A. This is partly due to the nondeterministic nature of timers, and partly due to floating-point rounding errors."
- "If no tick source is provided, the rate filter will use the frame tick as source, which corresponds with the number of times ecs_progress() is called."
- Shared tick source: "When two systems share the same tick source, it is guaranteed that they tick in the same frame" — a disabled system with its own interval would drift out of sync.
- `ecs_randomize_timers`: "systems/timers for the same interval don't all happen on the same tick" (load spreading).

Pipeline / order (V) [F1][F8]:
- "A pipeline by default orders systems by their entity id, to ensure deterministic order." Recycled ids can break declaration order -> "prevent entity deletion while registering systems".
- Phases (OnLoad, PostLoad, PreUpdate, OnUpdate, OnValidate, PostUpdate, PreStore, OnStore) then declaration order. "It prevents you from defining dependencies between systems, which can make it difficult to reuse systems across projects." Module import order sets order "at the module level, never at the individual system level".
- Auto sync points: a sync point is inserted before a system that reads a component for which an earlier system declared a write (`.write<T>()` / `[out] T()`). Default assumption: "systems insert no commands / that it is OK for commands to be merged at the end of the frame."

Takeaways (I):
- Cardinal's model (phases + registration order) is Flecs's model. Flecs's author defends it for module reuse. Good fit.
- Due times: Flecs explicitly prefers integer tick counts (rate filters) over float intervals for determinism. Cardinal's `due tick` as uint64 is the right choice; never float seconds.
- Flecs's declared-write sync points are the analogue of Cardinal's "reader after sender" test: the declaration (`write<T>`) drives the check. Cardinal could make the queue registration itself the declaration: `RegisterQueue[T](owner system)` + `Sends[T](system)` -> test asserts registration order.

More sources:
- [B21] Bevy `Message` trait docs, v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/message/mod.rs (lines ~36-50); `MessageWriter`/`MessageReader` fields: .../message/message_writer.rs, .../message/message_reader.rs
- [B22] Bevy `CommandQueue` apply (nested world flush), v0.20.1. https://github.com/bevyengine/bevy/blob/v0.20.1/crates/bevy_ecs/src/world/command_queue.rs (line ~127)
- [B23] Bevy 0.17 release notes, "Events vs Messages". https://bevy.org/news/bevy-0-17/
- [B24] Bevy issue #7691 comments (cheatbook link, jabuwu FixedUpdate pitfalls list, paholg determinism + RemovedComponents, "now resolved: Events now integrate with the fixed update" 2024-06-19). https://github.com/bevyengine/bevy/issues/7691
- [B25] Unofficial Bevy Cheatbook, "Manual Event Clearing" (targets Bevy 0.9, marked outdated). https://bevy-cheatbook.github.io/patterns/manual-event-clear.html

Extra verified points:
- Message rationale (V) [B21]: "messages are useful for efficiently batch processing a large number of messages at once ... can be more efficient than Events". "Unlike Events triggered for observers, messages are evaluated at fixed points in the schedule rather than immediately when they are sent. This allows for more predictable scheduling, and deferring message processing to a later point in time."
- Reader cursor is `Local<MessageCursor<M>>`, writer is `ResMut<Messages<M>>` (V) [B21]. So: (a) reader progress is per-system hidden state (not snapshotted, cf. bevy_ggrs [B16]); (b) an unordered writer/reader pair is a Res/ResMut conflict that ambiguity detection reports. (I)
- 0.17 split (V via WebFetch summary, so (S)) [B23]: "Event is now exclusively the name/trait for the concept of something that is 'triggered' and 'observed'. Message is the name / trait for something that is 'buffered'".
- Bevy commands that queue world commands are flushed right after that command: "The command may have queued up world commands, which we flush here" (V) [B22]. So plain commands are depth-first; only observer-issued commands break that (#19569).
- paholg on #7691 (V) [B24]: wanted "a fully deterministic game with bevy and bevy_rapier", but `RemovedComponents` cleanup ran in the main schedule, not `FixedUpdate`; workaround = custom runner calling `World::clear_trackers` in FixedUpdate. Lesson: any per-tick clearing must be tied to the sim tick, not the outer loop.
- Cheatbook (S) [B25]: "if you have systems that do not read events every frame, they might miss some events" — fixed timestep and custom run criteria named as causes; workaround = register `Events<T>` as a plain resource and clear it yourself.

## 11. Synthesis for Cardinal's queue API (all (I) unless cited)

What the two engines confirm about the decided design:
1. Requests as data in a component drained by the owner system — Flecs's author lists exactly the things observers can't do (sort, peek, per-entity queues) [F6], endorses "transient event entities swept by a system" [F9]. Bevy calls buffered messages "more predictable" [B21]. Neither engine has an owner-only rule; that rule is Cardinal's own.
2. Due tick as an integer — Flecs: rate filters (integer tick counts) are deterministic, float intervals are not [F7]. Bevy's new `DelayedCommands` is Duration-based, checked once per frame in PreUpdate, and its reviewers could not make it work for the fixed clock [B5][B6][B7]. Integer due tick avoids that whole class.
3. Delays as data, not closures — Bevy's delayed queue is opaque (`reflect(ignore)`), not cancellable in core (#24179 unmerged), not snapshot-safe. Critics asked for "component + Timer + event" for cancel/reflect/inspect [B7]. Cardinal's request struct with a due field gets all three.
4. Snapshot — bevy_ggrs: `Messages` and `Local` are not snapshotted, so they must not be used inside the rollback schedule; use components [B16]. Cardinal: request queues and delayed-event queues are components (in the snapshot); one-tick events cleared each tick (nothing to save at the tick boundary). Keep any sequence counter used for ordering inside a component too.
5. Order checks — Bevy's CI asserts zero ambiguities across all default plugins and has a debug shuffle seed [B10][B11]. Flecs inserts sync points from declared writes [F1]. Cardinal's "every reader after every sender" test is the same idea, made directional. Drive it from registration (`Sends[E]`, `Reads[E]`) so the declaration and the test can't drift.
6. Depth cap — neither Bevy nor Flecs has one for event/command chains (Bevy: none found in observer/event/command_queue; Flecs: nested queues grow without cap; only a 128 relationship-cycle panic) [B12][F4][F5]. Cardinal's depth field is new; no prior art to copy.

Concrete API shape suggested by the evidence (Go, sketch):
```go
type Meta struct {
    Due    uint64        // 0 = asap; else absolute tick (never seconds)
    Source cardinal.EntityID // who asked (Flecs stores enqueuing system: ecs_cmd_t.system)
    Cause  uint32        // feature/system id that started the chain
    Depth  uint8         // hops since first trigger
    Flags  uint8
    Seq    uint32        // per-queue insertion counter, saved in the component (Bevy MessageId analogue)
}
type Queue[R any] struct { Items cardinal.Slice[Req[R]]; NextSeq uint32 } // one singleton per owner
type Req[R any] struct { Meta; Target cardinal.EntityID; Body R }

func Send[R any](w, q *Queue[R], m Meta, target EntityID, body R)  // Insert(Len, x): in-place append
func Drain[R any](w, q *Queue[R], now uint64, apply func(Req[R]))  // stable sort (Due, Target, Seq); apply due; keep rest
```
- One singleton queue per owner, not one per entity (user's +130us/500 entities; Flecs "different event queues per entity ... inefficient/difficult" [F6]).
- Sort key: (Due, Target, Seq). Grouping by target gives Flecs-style per-entity batching [F4] and lets the owner fold several requests (sum damage) before one write. Seq keeps FIFO inside a target; it is deterministic because producers run in fixed registration order and it is saved.
- Dead target at apply time: drop + count, never panic (Bevy entity commands error on despawned targets [B8]; Flecs discards commands for deleted entities [F2]).
- Early return when `Len()==0` (Bevy `PopulatedMessageReader`, run conditions [B1][B20]).
- Delayed events: store as data with integer due tick; fire at tick start (beats Bevy's PreUpdate time check, which isn't tied to the fixed clock).

Possible flaws / open questions in the decided design (evidence-backed where marked):
- Depth on self-repeating effects: a DoT that re-enqueues itself each tick will hit the depth cap. Flecs handles periodic work with tick sources/rate filters [F7], not chains. Suggest: repeats are a flag (or a separate timer component) that does not inherit depth. (I)
- Breadth-first vs depth-first: Bevy users want "fully solve one collision ... before moving to the next" (#19569) [B13]; Flecs gives that via nested queues [F2]. Cardinal's tick-delayed model can't. Mitigation: owner-only writes + sorted drain mean the owner itself decides one-hit-per-projectile in its drain; document this pattern. (I)
- One-tick events with no double buffer: safe only if the reader-after-sender test covers every event type, including events sent by drains. Bevy's whole double-buffer exists to tolerate missing order [B1]. Keep the test mandatory in CI, Bevy-style zero-violation assert [B11]. (I)
- Any hidden per-system state (Go struct fields, closures) holding pending work breaks restore — same as Bevy `Local<T>` [B16]. Lint/test for it if possible. (I)

## 12. Could not find / not verified
- No Bevy or Flecs doc on request queues with an owner-only rule; the "owner feature" idea has no analogue in either.
- No depth or cycle limit for observer/command chains in Bevy or Flecs (searched source).
- No sorted application of commands in either engine (Bevy: topological system index order [B9]; Flecs: per-stage FIFO + per-entity batching [F4]).
- No Flecs issue/complaint thread found about lost events or command ordering beyond small bugs; Flecs issue search was title-only.
- Did not verify how Bevy's `bevy_platform` `HashMap` hashes `Duration` keys (affects order DelayedCommands spawns its queue entities in). (I) likely irrelevant since each delay gets its own entity.
- Did not read Bevy 0.17 release notes directly (WebFetch summary only) — marked (S).
- Did not check how Flecs's JSON/snapshot handles pending command queues (likely: queues are not serialized; snapshots taken outside progress()).
