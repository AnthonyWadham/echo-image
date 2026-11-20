# 03 - Unity Entities + Unreal Mass: queues, command buffers, signals

Researcher notes, 2026-11-19. Tags: (V) read primary this session, (S) secondary/unread primary, (I) my inference.
Note: web pages were read through a fetch tool that summarises; quotes are the lines it returned verbatim.

## Sources
- [U1] Unity Entities 6.5, ECB playback: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entity-command-buffer-playback.html
- [U2] Unity Entities 6.5, ECB automatic playback: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/systems-entity-command-buffer-automatic-playback.html
- [U3] Unity Entities 6.5, dynamic buffers intro: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-buffer-introducing.html
- [U4] Unity Entities 6.5, modify buffers with an ECB: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-buffer-command-buffer.html
- [U6] Unity Entities 1.5.0 package source (downloaded from packages.unity.com), files Unity.Entities/EntityCommandBuffer.cs, EntityCommandBufferSystem.cs: https://download.packages.unity.com/com.unity.entities/-/com.unity.entities-1.5.0.tgz
- [N1] Netcode for Entities 1.6, command stream: https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/command-stream.html
- [N2] Netcode for Entities 1.6, prediction edge cases: https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/prediction-details.html
- [N3] Netcode for Entities 1.6, intro to prediction: https://docs.unity3d.com/Packages/com.unity.netcode@1.6/manual/intro-to-prediction.html
- [N4] Netcode for Entities 7.2.0 package source (packages.unity.com), Runtime/Command/ICommandData.cs, IInputComponentData.cs, PredictionTicking/NetworkTime.cs: https://download.packages.unity.com/com.unity.netcode/-/com.unity.netcode-7.2.0.tgz
- [F1] "Entities-as-events: Yes or No?", Unity Discussions, Nov 2019: https://discussions.unity.com/t/entities-as-events-yes-or-no/764755
- [F2] "Event component data", Unity Discussions, 2020-2022: https://discussions.unity.com/t/event-component-data/816934
- [F3] tertle, BovineLabs Event System repo (README is a stub; not used): https://github.com/tertle/com.bovinelabs.event
- [M1] UE 5.6.1 MassSignals source (public GitHub copy of Epic's code; Epic's own repo is gated): MassSignalSubsystem.h/.cpp, MassSignalProcessorBase.h/.cpp at https://github.com/Viktor4221/ue5.6.1/tree/main/UnrealEngine-5.6.1-release/Engine/Plugins/Runtime/MassGameplay/Source/MassSignals
- [M2] X157, "Mass Signals" (community notes): https://x157.github.io/UE5/Mass/Signals
- [M3] UE 5.6.1 MassEntity source (same public copy): Public/MassCommandBuffer.h, Private/MassCommandBuffer.cpp, Private/MassEntityManager.cpp, Public/MassProcessor.h, Public/MassProcessingTypes.h, Private/MassProcessorDependencySolver.cpp, Private/MassProcessingPhaseManager.cpp, Private/MassProcessor.cpp, Private/MassExecutor.cpp under https://github.com/Viktor4221/ue5.6.1/tree/main/UnrealEngine-5.6.1-release/Engine/Source/Runtime/MassEntity
- [M4] "MassCommandAddFragmentInstances adding fragments to incorrect entities", UE forum, Dec 2022: https://forums.unrealengine.com/t/masscommandaddfragmentinstances-adding-fragments-to-incorrect-entities/731341
- [M5] "Question about FMassBatchedCommand best practices", UE forum, Oct-Nov 2025 (Epic staff James Keeling replies): https://forums.unrealengine.com/t/question-about-fmassbatchedcommand-best-practices/2684678
- [M6] "Race condition with Mass Processors writing to UWorld::SubsystemCollection::SubsystemArrayMap", UE forum, 2025 (from prior survey [14], not re-read): https://forums.unrealengine.com/t/race-condition-with-mass-processors-writing-to-uworld-subsystemarraymap/2667479
- [U7] "ECS Development Status for Unity 6.6", Unity Discussions, 2026-06-29 (joepl, Unity): https://discussions.unity.com/t/ecs-development-status-for-unity-6-6/1724480
- [U8] Unity Entities 0.50 manual, Entity Command Buffers (old, for wording): https://docs.unity3d.com/Packages/com.unity.entities@0.50/manual/entity_command_buffer.html
- [U9] Unity Entities 1.5.0 source: Unity.Entities/ComponentSystemSorter.cs, ComponentSystemGroup.cs (same tarball as [U6])
- [N5] Netcode 7.2.0 source: Runtime/Rpc/RpcCommandRequest.cs, Runtime/Connection/WarnAboutStaleRpcSystem.cs, Runtime/Debug/NetDebug.cs (same tarball as [N4])
- [U10] Unity Physics 1.3 design page: https://docs.unity3d.com/Packages/com.unity.physics@1.3/manual/design.html
- [U5] Unity Entities 6.5, enableable components intro + use: https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-enableable-intro.html ; https://docs.unity3d.com/Packages/com.unity.entities@6.5/manual/components-enableable-use.html

## 1. Unity EntityCommandBuffer (ECB)

### Playback order and sort keys
- (V) [U1] Parallel recording order is not deterministic: "the order of the commands is non-deterministic because they depend on job scheduling."
- (V) [U1] Fix = an int sort key as first arg of every command: "If the recorded sort keys are independent from the scheduling, then the sorting makes the playback order deterministic." "Unity always plays back commands with larger sort keys after commands with smaller sort keys."
- (V) [U1] Recommended key in jobs: `[ChunkIndexInQuery] int sortKey`; "unique for each chunk in the query and will be consistent regardless of scheduling."
- (V) [U1] API shape: `void Execute(Entity e, [ChunkIndexInQuery] int sortKey, in FooComp foo)`; `ecbParallel.AddComponent<BarComp>(sortKey, e);` with `EntityCommandBuffer.ParallelWriter` from `ecb.AsParallelWriter()`.
- (V) [U1] Unity admits the cost: determinism can hurt performance; some projects may accept non-determinism.
- (V) [U1] Playback is single-shot by default: "If you call the `Playback` method more than once, it throws an exception." `PlaybackPolicy.MultiPlayback` allows repeat.

### Automatic playback (ECB systems = fixed drain points)
- (V) [U2] Nine built-in ECB systems at start/end of groups (BeginInitialization ... EndSimulation, BeginPresentation). No EndPresentation: "the end of one frame is the beginning of the next frame."
- (V) [U2] API: `var singleton = SystemAPI.GetSingleton<FooECBSystem.Singleton>(); EntityCommandBuffer ecb = singleton.CreateCommandBuffer(state.WorldUnmanaged);`
- (V) [U2] On update the ECB system completes registered jobs, then "Plays back all ECBs created via the system in the same order they were created," then disposes them.
- (V) [U2] Placeholder entities from `ecb.CreateEntity()` mean nothing outside the ECB: "Exception! The placeholderEntity has no meaning outside the ECB which created it, even after playback."
- (I) Late recording: a buffer made from a system that already ran this frame waits for that system's next update (next frame). Same as our rule 2. Doc does not say it outright.

### How the sort really works (engine source)
- (V) [U6] Each thread records into its own chain (`m_ThreadedChains[threadIndex]`). `Reserve(chain, sortKey, size)` starts a new linked chain when the key goes down: `if (Hint.Unlikely(newSortKey < chain->m_LastSortKey)) { // copy current chain to new next and reset current chain`. So every chain is non-decreasing by key.
- (V) [U6] Playback = k-way merge of all chains with a min-heap `ECBChainPriorityQueue` keyed only on `SortKey`: `if (val.SortKey < m_Heap[child].SortKey) break;`. No tie-break field.
- (V) [U6] A chain plays until its next command has a bigger key than the next chain: `if (nextChain != -1 && header->SortKey > nextChainSortKey) { // early out because a different chain needs to playback`.
- (I) Consequence: equal sort keys recorded on different threads interleave by heap position = by thread index. Not deterministic. Determinism holds only if each key is written by one job iteration (hence ChunkIndexInQuery, unique per chunk). Within one key, record order holds.
- (I) Cost model: no full sort. Recording is append-only per thread; ordering is a cheap merge of mostly-sorted runs. For Cardinal (single-threaded systems) the analogue is: append in system order, then a stable sort by (dueTick, key) at drain.
- (V) [U6] EntityCommandBufferSystem docs: "This buffer system executes each of these separate command buffers in the order that you created them." and "To avoid sync points, you should use as few entity command buffer systems as possible." and "unused command buffers systems do not create sync points".
- (I) Unity's sort key is a scheduling key (who recorded it), not a semantic key (target entity, request type). Our design rule 7 ("sorted, deterministic order") should pick a key that is independent of system scheduling too. In single-threaded Cardinal, append order is already deterministic if system order is fixed; a sort is only needed if senders run in a nondeterministic order (e.g. map iteration) or if the owner wants a semantic order (e.g. by due tick, then by target).

## 2. DynamicBuffer as a per-entity queue
- (V) [U3] "A dynamic buffer component is a component that acts as a resizable array of unmanaged structs." Has `Length` and `Capacity`. Default inline capacity fits 128 bytes; `[InternalBufferCapacity(n)]` overrides. Elements live in the chunk until they outgrow it, then move to heap.
- (V) [U3] Structural changes "destroy or move the array"; stale handles throw: "A safety check will throw an exception on any read or write actions on the buffer."
- (V) [U3] Guidance: dynamic buffer when many entities need a list; a singleton holding a native container otherwise.
- (V) [U4] ECB has three buffer commands. Call forms: `ecb.AddBuffer<T>(e)`, `ecb.SetBuffer<T>(e)`, `ecb.AppendToBuffer(e, new T{...})`.
  - AddBuffer / SetBuffer replace contents at playback. With several threads SetBuffer-ing one entity, only the last by sortKey survives.
  - AppendToBuffer keeps contents, adds one element. Many threads may append to one buffer; "sortKey determines element order."
  - Pitfall: AppendToBuffer "fails at playback if the target entity doesn't contain a buffer component of type T." Advice: "it's best practice to precede every AppendToBuffer command with AddComponent<T>."
- (I) AppendToBuffer(sortKey, target, req) is the closest Unity shape to our "add a request to the owner's queue". Order = sortKey, then record order inside one key (single-threaded record order). Unity does NOT sort by request content; the sender's key decides.
- (I) The "must add before append" pitfall maps to our design: if the queue is a singleton component, it always exists; no such failure.
- (I) Cost note matches the user's measurement: a buffer on every entity costs chunk space (128 B default inline) even when empty. Unity's own answer is to size it with InternalBufferCapacity(0) or use a singleton.

## 3. Enableable components
- (V) [U5] "inherit them from IEnableableComponent"; works for IComponentData and IBufferElementData. Goal: "avoid structural changes in some situations." Adding/removing is better for "infrequent" changes that persist.
- (V) [U5] "An entity with component T disabled matches queries as if it doesn't have component T at all."
- (V) [U5] API: `IsComponentEnabled<T>(e)`, `SetComponentEnabled<T>(e, bool)`, `ComponentLookup<T>.SetComponentEnabled` (safe "from worker threads, because no structural change is needed"), `EnabledRefRW<T>` = "the most efficient way".
- (V) [U5] Pitfalls: "Avoid enabling or disabling a component on an entity that another thread might process in a job"; job write access to enabled bits can block main thread.
- (I) Relevance: an enable bit is a cheap "has pending requests" flag per entity. A Cardinal analogue would be a per-owner bool or a dirty list on the singleton queue. Not needed if queues are singletons (the user's measure says per-entity queues cost +130us/tick empty).

## 4. Netcode for Entities: tick-stamped input buffers and prediction

### Input buffer = a per-entity ring buffer keyed by tick
- (V) [N4] `public interface ICommandData : IBufferElementData { NetworkTick Tick { get; set; } }`. Doc: "It is mandatory to set the tick before adding the command".
- (V) [N4] `CommandDataUtility.k_CommandDataMaxSize = 64` (ring size).
- (V) [N4] `AddCommandData<T>(this DynamicBuffer<T> buf, T cmd) -> bool`: "If a command with the same tick already exists in the buffer, it will be overwritten." When full, overwrites the oldest slot. Returns true if it replaced a same-tick entry.
- (V) [N4] `GetDataAtTick<T>(this DynamicBuffer<T> buf, NetworkTick target, out T cmd) -> bool`: "latest without going over". Linear scan of 64 slots; no removal. Reading does not consume.
- (V) [N1] Server side: `CommandReceiveSystem` dispatches received commands to the owning entity. Server "should never overwrite or change the input received by the client."
- (V) [N1] Redundancy: each packet also carries the previous n ticks of input (default 4), delta-compressed. Payload cap: "A size limit of 1024 bytes is enforced on the command payload."
- (V) [N4] One-shot inputs use a counter, not a bool: `public struct InputEvent { public uint Count; public bool IsSet => Count > 0; }` "it's ensured that single input events like jumping or triggers will be properly detected exactly once by the server."
- (V) [N1] Processing can run several times per tick during rollback, so input-processing systems must be safe to re-run.
- (I) Lessons for our due-tick queue: (a) keyed by tick, read by "latest at or before tick" = a natural way to hold future requests; (b) the input ring is NOT drained/consumed, it is indexed - that is what makes re-simulation safe; (c) a one-shot is counted so a dropped packet does not lose it. Our request queue is drained (consumed). That is fine for a server-authoritative tick, but a rollback client must snapshot the queue with the state (Cardinal snapshots components, so a queue component is saved - good).

### Prediction, one-off effects, cross-entity writes
- (V) [N4] `IsFirstTimeFullyPredictingTick`: "This is frequently used to make sure effects which cannot easily be rolled back, such as spawning objects / particles / vfx or playing sounds, only happens once and are not repeated."
- (V) [N3] "Only ghosts that received an update in the snapshot are rolled back, not the whole simulation."
- (V) [N2] Rollback depth capped: "Netcode will clamp the rollback to the input queue size (currently a const set to 64)."
- (V) [N2] Cross-entity problem: "if B collides with C and changes C's velocity, C will still only execute one tick using that velocity, while A and B will interact with a 'frozen' C".
- (V) [N2] Unity's advice: "Change other ghosts' states instead of waiting for them to change themselves." "ghost A picks up ghost C. Instead of having ghost C update its own position according to ghost A, have ghost A update ghost C's position."
- (V) [N2] "Netcode is not a deterministic package." Advice: "quantize values yourself at the end of your tick".
- (I) TENSION with our rule 1-2 (owner-only writes via queues). Unity's advice is the opposite (actor writes target) - but only because Netcode does PARTIAL rollback (only some entities resimulate). In a full-state rollback (whole world restored, as Cardinal snapshots/echo rewind do), the owner-drains-queue model is fine. Flag: if a client ever predicts only some entities, a queued request to a non-resimulated owner is lost or applied once. Not a flaw for a full-world tick; worth one line in the design doc.

## 5. Request / event entities and queues in Unity DOTS (community practice)
No official Unity page on events between systems (prior survey also found none). Evidence is forum threads; all (V) below = read the thread, (S) for the poster's authority.
- (V) [F1] Entity events (an entity per event, made via ECB): DreamingImLatios lists cons: "Requires non-Burst step (playback or EET)." "Generates sync point (or uses existing one delaying when event handlers can run)."
- (V) [F1] tertle: "Events created using ECB don't scale that well", problems from "100-1000s per/frame".
- (V) [F1] Queue tradeoffs (DreamingImLatios): NativeQueue "Faster than NativeStream in my personal testing" but "Single-threaded reading"; NativeStream: "Parallel writing", "Parallel reading", "Read multiple times". PhilSA: a queue "you can only read from it once."
- (V) [F1] Ordering dispute: NativeQueue order "uses threadIndex, which is nondeterministic" (DreamingImLatios); tertle: "What happens if you have 2 machines with different thread counts?" ECB is the deterministic option because it takes an explicit index.
- (V) [F1] Joachim_Ante_1 (Unity co-founder/CTO at the time (S)): "There is no reason for introducing any sync point." - schedule the reader job as a dependency instead.
- (V) [F2] Entity events OK at low frequency (brunocoimbra: "For low-frequency event: yes, totally good."). Event entity create/destroy "looked comparatively slow" in a benchmark (lclemens).
- (V) [F2] Polling is fine in ECS (tertle): "in entities it's actually really not that bad"; "for rarely tweaked components you can use change filtering". DreamingImLatios warns change filters in queries "can force jobs to complete".
- (V) [F2] PhilSA damage-queue example: a NativeQueue many systems write; warns parallel apply is risky because "different events can attempt to modify the health" - i.e. one consumer must apply them. Same shape as our owner-drained queue.
- (V) [F2] DreamingImLatios: "I sometimes recoil at the mentioning of 'event system' now because it is generally the wrong mindset."
- (I) Takeaway: the Unity community converged on (a) a single-consumer queue for high-volume requests like damage, (b) entity events only when rare, (c) polling/change filters for state reactions. That matches rules 2 and 5. Entity-per-request is the costly variant; Cardinal analog = spawning an entity per request (avoid).

### Official request-entity example: Netcode RPCs (V) [N5]
- (V) An incoming RPC is an ENTITY carrying the RPC struct plus `public struct ReceiveRpcCommandRequest : IComponentData { public Entity SourceConnection; ... }`. Outgoing: `SendRpcCommandRequest { Entity TargetConnection; ... }`.
- (V) Handler must consume: `public void Consume()` (debug: sets `Age = ushort.MaxValue`), or remove the component, or destroy the entity.
- (V) Debug watchdog `WarnAboutStaleRpcSystem`, `[UpdateInGroup(typeof(SimulationSystemGroup), OrderLast = true)]`, ages unconsumed requests each frame and warns: "has not been consumed or destroyed for '{command.Age}' (MaxRpcAgeFrames) frames! Assumed unhandled." Default `MaxRpcAgeFrames = 4`.
- (I) Steal this: a debug check that a queue was drained (or a request type has a registered owner) catches "request sent to nobody". In our design, the owner always drains, so the check is "every request type has exactly one drain system registered" - a startup test, not a per-tick cost.
- (V) [U10] Unity Physics calls itself "a completely deterministic rigid body dynamics and spatial query system" and is "Stateless" (no cache). (I) Unity's own determinism story relies on stateless + sorted inputs, same as rule 7.

## 6. Unreal Mass signals (immediate, deferred, delayed)

### API (V) [M1] MassSignalSubsystem.h, UE 5.6.1
```
DECLARE_MULTICAST_DELEGATE_TwoParams(FSignalDelegate, FName /*SignalName*/, TConstArrayView<FMassEntityHandle> /*Entities*/);
FSignalDelegate& GetSignalDelegateByName(FName SignalName);
void SignalEntity(FName SignalName, const FMassEntityHandle Entity);
void SignalEntities(FName SignalName, TConstArrayView<FMassEntityHandle> Entities);
void DelaySignalEntity(FName SignalName, const FMassEntityHandle Entity, const float DelayInSeconds);
void DelaySignalEntities(FName, TConstArrayView<FMassEntityHandle>, const float DelayInSeconds);
void SignalEntityDeferred(FMassExecutionContext& Context, FName, const FMassEntityHandle);      // via command buffer
void DelaySignalEntityDeferred(FMassExecutionContext& Context, FName, const FMassEntityHandle, const float DelayInSeconds);
struct FDelayedSignal { FName SignalName; TArray<FMassEntityHandle> Entities; double TargetTimestamp; };
TArray<FDelayedSignal> DelayedSignals;
```
- (V) [M1] Signals carry NO payload, only an FName + entity list. [M2] also: "Signals do not contain any Event or other context data."
- (V) [M1] Immediate `SignalEntities` = multicast delegate Broadcast, synchronous, to every subscriber.
- (V) [M1] Deferred = wraps the call in a lambda pushed on the command buffer: `Context.Defer().PushCommand<FMassDeferredSetCommand>([SignalName, InEntities ...](const FMassEntityManager& ...) { ... SignalSubsystem->SignalEntities(SignalName, InEntities); });`
- (V) [M1] Delay is in float SECONDS of world time, not ticks: `DelayedSignal.TargetTimestamp = CachedWorld->GetTimeSeconds() + DelayInSeconds;`
- (V) [M1] Delayed signals fire in the subsystem's own `Tick(float DeltaTime)`: loop over the array, `if (DelayedSignal.TargetTimestamp <= CurrentTime) { SignalEntities(...); DelayedSignals.RemoveAtSwap(i, ...); }`
- (I) Pitfalls visible in that code: (a) linear scan of all pending delayed signals every tick; (b) `RemoveAtSwap` reorders the array, so fire order of same-time signals depends on removal history - not a stable FIFO; (c) float seconds + time compare = delays snap to whichever frame first passes the time, varies with frame rate; (d) the subsystem tick is a tickable object, not a fixed point inside the processor graph. Our design (due tick int, fire at the very start of the tick, sorted) avoids all four.
- (V) [M1] Thread safety: subsystem traits say `ThreadSafeWrite = false` with "@todo this subsystem not being thread-safe when writing is an obstacle in parallelizing multiple processors". Delayed-signal comment: "With the game thread executing UMassSignalSubsystem::Tick then you need to reorganize your tasks to prevent senders from executing at the same time as the subsystem tick."

### Receiver = per-processor double-buffered queue drained in Execute (V) [M1] MassSignalProcessorBase
- (V) Subscribe: `SubscribeToSignal(UMassSignalSubsystem&, const FName SignalName)` binds `OnSignalReceived` on the delegate. Derived classes implement `virtual void SignalEntities(FMassEntityManager&, FMassExecutionContext&, FMassSignalNameLookup& EntitySignals)`.
- (V) Storage: "Received signals are double buffered as we can receive new one while processing them": `TStaticArray<FFrameReceivedSignals, 2>`; each holds `TArray<FEntitySignalRange>` (name + begin/end) and `TArray<FMassEntityHandle> SignaledEntities` ("can contain duplicates").
- (V) Execute flips the buffer index under a lock, then processes the old buffer: "Once that's done the incoming signals will end up in the other buffer". So a signal that arrives during/after the processor ran waits for the next frame. Same as our rule 2.
- (V) Drain groups entities per archetype and dedups: `FMassArchetypeEntityCollection(Set.Archetype, Set.Entities, FMassArchetypeEntityCollection::FoldDuplicates)`. Per-entity signal set is a bitmask (`SignalNameLookup.GetOrAddSignalName` returns a `uint64` flag); when > 64 names arrive "we'll do passes until all signals are processed".
- (I) So Mass processing order is NOT receipt order (x157's "in order of receipt" [M2] is only about which buffer); it is by archetype, then entity collection order; the same signal twice to one entity collapses to one call. Fine for payload-less "wake up" signals; wrong for requests with payload (two damage requests must not fold).
- (V) "We don't want signaling processors to be pruned by default, since that can mess up signal processing just after processor's creation (might miss some signals initially)." -> late-subscriber pitfall: a receiver that does not exist yet misses signals.
- (I) Mapping: Mass signal = our one-tick event (rule 5) without payload. Mass has no built-in "request with payload to an owner" type; payload goes in fragments or a command buffer lambda.

## 7. Unreal Mass FMassCommandBuffer (deferred commands)

### API (V) [M3] MassCommandBuffer.h
```
template<template<typename...> typename TCommand, typename... TArgs> void PushCommand(const FMassEntityHandle Entity, TArgs&&... InArgs);
template<typename TCommand, typename... TArgs> void PushCommand(TArgs&&... InArgs);
AddFragment<T>(Entity) / RemoveFragment<T>(Entity) / AddTag<T> / RemoveTag<T> / SwapTags<TOld,TNew> / DestroyEntity(Entity)
void MoveAppend(FMassCommandBuffer& InOutOther);
bool HasPendingCommands() const;
// user code: Context.Defer().PushCommand<FMassDeferredSetCommand>([...](FMassEntityManager& M){ ... });
```
- (V) Commands are batched BY TYPE, not by record order: "All commands in the array are unique (by type) and reusable with subsequent PushCommand calls" (`CreateOrAddCommand<T>()` indexes by `FMassBatchedCommand::GetCommandIndex<T>()`).
- (V) Flush order is a stable sort by operation group, not by time: `CommandTypeOrder = { None: MAX-1, Create: 0, Add: 2, Remove: 6, ChangeComposition: 3, Set: 4, Destroy: 6 }` then `CommandsOrder.StableSort();`.
- (V) Single-thread rule: "Commands can be pushed only in the same thread where the command buffer was created." and "Trying to push commands is not supported while the given buffer is being flushed".
- (V) Parallel processors each get a fresh buffer, then `MainSharedPtr->MoveAppend(ExecutionContext.Defer());` [MassProcessor.cpp].
- (I) Consequence: inside one op group, order = MoveAppend order = which processor task finished first, in parallel mode. Mass does not promise a deterministic cross-processor order. No sort key exists (unlike Unity).
- (I) Consequence: a Set lambda recorded BEFORE a Destroy still runs before it (Set=4 < Destroy=6), but an Add recorded AFTER a Set runs BEFORE it (Add=2). Record order is lost across types. Epic chose batching speed over causal order.

### Where it drains: end of every phase + cascade cap (V) [M3]
- (V) `FMassProcessingPhaseManager::OnPhaseEnd`: `if (GetEntityManagerRef().Defer().HasPendingCommands()) { GetEntityManagerRef().FlushCommands(); }` - one drain point per phase (6 phases).
- (V) `FMassEntityManager::FlushCommands()`: double-buffered; commands pushed during the flush (e.g. by observers) go to the other buffer and are flushed in another loop pass: `// repeat if there were commands submitted while commands were being flushed (by observers for example)`.
- (V) Hard cascade cap: `constexpr int32 MaxIterations = 5;` then error "Reached loop count limit while flushing commands. Limiting the number of commands pushed during commands flushing could help." Leftover commands stay in the buffer (I: run at the next flush).
- (V) Must not flush during processing: "Calling %hs is not supported while Mass Processing is active. Call FMassEntityManager::AppendCommands instead."
- (I) Mass's cascade cap is per FLUSH (global loop count), not per chain (depth on each request). Our per-request depth (rule 4) is finer: one runaway chain is cut without stopping unrelated work. Mass's design also shows the shape of a "drain loop until quiet, max N" which our design avoids by pushing late requests to next tick.

## 8. Unreal Mass processor order (V) [M3]
- (V) Phases: `enum class EMassProcessingPhase { PrePhysics, StartPhysics, DuringPhysics, EndPhysics, PostPhysics, FrameEnd }`; default `ProcessingPhase = EMassProcessingPhase::PrePhysics`.
- (V) Per processor: `struct FMassProcessorExecutionOrder { FName ExecuteInGroup; TArray<FName> ExecuteBefore; TArray<FName> ExecuteAfter; }` - config-editable (`UPROPERTY(..., config)`). Groups are dotted names: "A.B.C" -> ["A", "A.B", "A.B.C"].
- (V) `int16 ExecutionPriority` "used when sorting nodes that otherwise seem similar"; high priority propagates to dependencies.
- (V) Solver adds implicit edges from declared fragment access: "For every read-only requirement we make InOutNode depend on the currently stored Writer of this resource" / "For every read-write requirement we make InOutNode depend on all the readers and writers currently stored." (I) i.e. registration order + declared R/W decide order between writers - like a hidden version of our "every reader after every sender" check.
- (V) Missing names still order: "if Processor A and B declare dependency, respectively, "Before C" and "After C" we still expect A to come before B regardless of whether C exists".
- (V) Cycles do not fail the build: "Encountered processing dependency cycle - cutting the chain at an arbitrary location." (logged as Error, keeps running).
- (V) Signal processors opt out of pruning ("might miss some signals initially").
- (I) Lesson for rule 5 test: Mass logs and silently breaks cycles; our test should FAIL instead. Mass derives order from declared access; Cardinal uses registration order, so a static test is the right tool.

## 9. Pitfalls and complaints (collected)

### Unity
- (V) [U8] "_playback_ is always just single-threaded on the main thread." Only recording is parallel.
- (V) [U8] Sort key = "a number that has a fixed and unique association with that entity in the job's query"; reusing one parallel ECB across jobs "can lead to unexpected (and potentially undesirable) sort orders."
- (V) [U6] (from source, sec. 1) equal sort keys from different threads have no tie-break.
- (V) [U7] Unity 6.6 (2026-06-29): "EntityCommandBuffer multi-playback has been deprecated." "This feature could cause bugs that could occur when an ECB was played back multiple times." -> replaying a command list is a bug source; our queue should be consumed once per drain.
- (V) [U7] Also: "EntityCommandBuffer operations now properly return created Entities that can be used immediately." (fix for the placeholder-entity pain in [U2]).
- (V) [U7] Determinism note from Unity: "Common causes of non-determinism in bakers include iterating over unordered collections" (I: same trap as Go map iteration in Cardinal senders).
- (V) [U3]/[U4] Stale DynamicBuffer handle after structural change throws; AppendToBuffer to an entity without the buffer fails at playback.
- (V) [F1] Entity-per-event cost at 100-1000s/frame; queue ordering by threadIndex is nondeterministic.
- (V) [N2] Partial rollback breaks "owner updates itself" across entities.

### Unreal Mass
- (V) [M1] Signals have no payload; Epic staff [M5] (quoted by a later poster): "We would like to add the ability to have payloads to MassSignals." - no timeline.
- (V) [M5] Epic staff on command-buffer use: "BatchedMassCommands were intended for specific, single, batched action on a set of entities." "It may currently support larger operations such as queries, but it is not something we will support." "any issues found would not be considered bugs by us." Also on timing: "It is supposed to happen between processing phases," but they found inconsistencies in how IsProcessing runs.
- (V) [M4] User report (no Epic reply in page): batched AddFragmentInstances wrote payload to the wrong entities because the archetype move reversed entity order while payload kept the original order: "The order of entities is _not_ necessarily the same". (I) lesson: if requests are reordered (sorted), payload must travel WITH the target id, never by position.
- (V) [M3] Flush cascade cap 5; dependency cycles cut "at an arbitrary location" with only a log.
- (V) [M1] Delayed signals: float seconds, linear scan, RemoveAtSwap reorder, ThreadSafeWrite=false "@todo".
- (S) [M6] Processors raced on first parallel run after UE 5.6 upgrade; Epic acknowledged (prior survey; not re-read).

## 10. Unity system order (for comparison with Mass and Cardinal)
- (V) [U9] Each group sorts in three buckets: `public int OrderingBucket; // 0 = OrderFirst, 1 = none, 2 = OrderLast`, then a topological sort on [UpdateBefore]/[UpdateAfter].
- (V) [U9] Ties broken by a stable type hash, not registration order: `var cmp = systemTypeHash.CompareTo(other.systemTypeHash); return cmp != 0 ? cmp : unsortedIndex.CompareTo(other.unsortedIndex);`
- (V) [U9] Cycles THROW: "The following systems form a circular dependency cycle (check their [*Before]/[*After] attributes):" (contrast Mass: logs and cuts arbitrarily).
- (I) Unity makes unconstrained order depend on type-name hash: deterministic but arbitrary, so adding a system can reorder unrelated ones. Cardinal's registration order is more predictable; the rule-5 test (readers after senders) guards it.

## 11. What this means for the Cardinal queue API (my synthesis, all (I))
- Drain point + late = next tick: confirmed by BOTH engines. Mass signal processors double-buffer and flip at Execute [M1]; Unity ECB systems play back what was created before they ran [U2]. Rule 2 is standard.
- Order key: Unity sorts by a caller-supplied int that must not depend on scheduling, ties unbroken [U1][U6]; Mass sorts only by op type and loses record order [M3]. Neither sorts by request content. For Cardinal (single-threaded, fixed system order) append order is already deterministic; sort (stable) by dueTick only, keep append order inside a tick. Add an explicit key only if a sender iterates a Go map.
- Payload by id, never by position [M4]. Each request stores its target id.
- Delays: use integer due ticks, not seconds [M1]; keep the delayed list sorted (or a min-heap) by (dueTick, seq), never RemoveAtSwap [M1]. Fire at the very start of the tick - Mass fires in an unrelated subsystem tick, which is a known source of order surprises.
- Cascade control: Mass caps flush passes at 5 globally [M3]; our per-request depth is finer. Log + drop when over the cap (Mass logs an error).
- Payload-less signals fold duplicates [M1]; requests with payload must not fold. Keep events and requests as separate types.
- Queue storage: Unity's advice = dynamic buffer only when many entities need a list, singleton otherwise [U3]; user's measurement agrees (per-entity queue +130us/tick empty). Use one singleton queue per owner feature.
- Ordering test: Unity throws on cycles [U9], Mass logs and cuts [M3]. Make the reader-after-sender test fail hard.
- Rollback: Netcode inputs are a tick-indexed ring, read not consumed [N4]; one-off effects gated by IsFirstTimeFullyPredictingTick [N4]. A drained queue is fine if the queue component is in the snapshot (Cardinal saves components). Partial rollback breaks owner-applies-request [N2]; full-world restore does not.
- Watchdog: Netcode warns on requests unconsumed after 4 frames [N5]; cheap equivalent = startup check that each request type has one drain system.

## 12. Could not find / not verified
- Official Unreal docs pages for Mass signals and command buffer (API reference is script-rendered; fetch returned an empty TOC). Used a public GitHub copy of UE 5.6.1 source instead - code is Epic's, host is not.
- Any Epic statement on Mass determinism. None found; Mass makes no determinism claim.
- Unity official guidance on gameplay events/requests between systems (none; forums only).
- A Unity benchmark of ECB playback cost per command; Unity docs only say determinism "can" cost performance.
- Enableable components: no official perf numbers for queries over many disabled entities.
- [M6] race-condition thread not re-read this session.
- Netcode docs version gap: docs read at 1.6, source at 7.2.0; signatures checked in source.
