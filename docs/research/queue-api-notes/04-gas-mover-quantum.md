# 04 - Unreal GAS, Unreal Mover, Photon Quantum: queue/request/delay API shapes

Researcher notes, 2026-11-19. Tags: (V) read primary this session, (S) secondary/unread primary, (I) my inference.
Status: DONE (2026-11-19)

## Sources
- [Q1] Quantum 3 manual, Frame Timer. https://doc.photonengine.com/quantum/v3/concepts-and-patterns/frame-timer
- [Q2] Quantum 3 manual, Events & Callbacks. https://doc.photonengine.com/quantum/current/manual/quantum-ecs/game-events
- [Q3] Quantum 3 manual, Systems. https://doc.photonengine.com/quantum/current/manual/quantum-ecs/systems
- [Q4] Quantum 3 manual, DSL (collections, signals, events, globals). https://doc.photonengine.com/quantum/current/manual/quantum-ecs/dsl
- [Q5] Quantum 3 manual, Commands. https://doc.photonengine.com/quantum/current/manual/commands
- [Q6] Quantum 3, Events vs Polling. https://doc.photonengine.com/quantum/v3/concepts-and-patterns/events-vs-polling
- [Q7] Quantum 3 API ref, collections namespace. https://doc-api.photonengine.com/en/quantum/current/namespace_quantum_1_1_collections.html
- [M1] Epic, Comparing Mover and Character Movement Component. https://dev.epicgames.com/documentation/en-us/unreal-engine/comparing-mover-and-character-movement-component-in-unreal-engine
- [M2] Epic, Mover Features and Concepts. https://dev.epicgames.com/documentation/en-us/unreal-engine/mover-features-and-concepts-in-unreal-engine
- [M3] Epic API ref, UMoverComponent::QueueInstantMovementEffect / ScheduleInstantMovementEffect / K2_QueueLayeredMove / K2_QueueMovementModifier. https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/UMoverComponent/QueueInstantMovementEffect ; https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/UMoverComponent/ScheduleInstantMovementEffect ; https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/UMoverComponent/K2_QueueLayeredMove ; https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/UMoverComponent/K2_QueueMovementModifier (JS-rendered; I only saw search-engine snippets)
- [M4] Epic API ref, FScheduledInstantMovementEffect / FMoverSchedulingInfo. https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/FScheduledInstantMovementEffect (snippets only)
- [M5] Epic API ref, FLayeredMoveBase / EMoveMixMode / UMovementMixer::MixLayeredMove. https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/FLayeredMoveBase (snippets only)
- [M6] Forum, "mover CancelFeaturesWithTag function, Why Cancel in next tick?" (quotes engine comment). https://forums.unrealengine.com/t/mover-cancelfeatureswithtag-function-why-cancel-in-next-tick/2678946
- [M7] Forum, "Mover Plugin roadmap and layering Issues" (Epic staff reply). https://forums.unrealengine.com/t/mover-plugin-roadmap-and-layering-issues/2712958
- [Q8] Quantum 3 manual, Components (singletons, iteration). https://doc.photonengine.com/quantum/current/manual/quantum-ecs/components
- [Q9] Quantum 3 manual, Frames. https://doc.photonengine.com/quantum/v3/manual/frames
- [Q10] Quantum 3 manual, Physics callbacks. https://doc.photonengine.com/quantum/current/manual/physics/callbacks
- [M9] Forum, "Mover LayeredMoves end one frame in advance" (2026). https://forums.unrealengine.com/t/mover-layeredmoves-end-one-frame-in-advance/2720552
- [G3] Epic API ref, FScopedActiveGameplayEffectLock / FActiveGameplayEffectsContainer (snippets only; pages JS-rendered or 403). https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/GameplayAbilities/FScopedActiveGameplayEffectLock
- [G4] Unreal issue tracker UE-199659 (pending removes after scope lock), UE-201125 (abilities added during scope lock). https://issues.unrealengine.com/issue/UE-199659 ; https://issues.unrealengine.com/issue/UE-201125 (403 to fetcher; snippets only)
- [G1] Epic, Gameplay Effects for GAS. https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-effects-for-the-gameplay-ability-system-in-unreal-engine
- [G2] tranek, GASDocumentation README (community; quotes engine source and GameplayPrediction.h). https://github.com/tranek/GASDocumentation (read in full sections this session; it is secondary, but code it quotes is from GameplayEffectAggregator.cpp / GameplayPrediction.h)
- [M8] Epic API ref, UMoverSimulation (staging queues, lock). https://dev.epicgames.com/documentation/unreal-engine/API/Plugins/Mover/UMoverSimulation (snippets only)

## Photon Quantum

Fetch note: pages read via WebFetch (a summarizer returns quotes). Quotes below are what it returned from the page.

### Signals = typed calls inside the tick (the state-change path)
- Synchronous dispatch: the pages read do not say it in words; generated code is a direct interface call per implementer, so synchronous. (I; prior SURVEY lists it as V [98])
- Declared in DSL: `signal OnDamage(FP damage, entity_ref entity);` (V) [Q4]
- Generated interface: `public interface ISignalOnDamage { public void OnDamage(Frame frame, FP damage, EntityRef entity); }` (V) [Q4]
- Raised: `f.Signals.OnDamage(10, entity)`. "any System can trigger the generated signal, not leading to coupling with a specific implementation". (V) [Q3]
- `signal struct OnDamage(...)` generates `OnDamage(Frame frame, in OnDamageArgs args)`; `ref` params not supported there. (V) [Q4]
- Order of implementers: "the order in which Systems are inserted will be the order in which all callbacks will be executed". So signal listeners run in system registration order. (V) [Q3]
- Signals are not queued. No due tick, no depth, no source. Re-entrancy is the caller's problem. (I) — docs show no queue or depth guard. (V for absence on [Q3][Q4])
- Built-in signals: `ISignalOnComponentAdded<T>` / `ISignalOnComponentRemoved<T>` (Systems page) and `ISignalOnAdd<T>` / `ISignalOnRemove<T>` (DSL page). (V) [Q3][Q4]

### Events = one-way notices to the view, never state changes
- "should never be used to modify/update part of the game state" (V) [Q4]; Signals are the tool for that (V) [Q2].
- Raised `f.Events.MyEvent(...)`; code-generated into `Frame.FrameEvents`. (V) [Q2]
- Dispatched "after all Frames have been simulated right after the `OnUpdateView` callback" -> the target view "may already have been destroyed". (V) [Q2]
- Predicted events are "either cancelled or confirmed once the predicted Frame from which they were fired has been verified". `CallbackEventCanceled`, `CallbackEventConfirmed`. (V) [Q2]
- De-dup: "Quantum identifies duplicates using a hash code function over the Event data members, the Event id and the tick." `nothashed` excludes a field. (V) [Q2]
- `synced event` = dispatched only on verified frames, never cancelled. (V) [Q2]
- Late joiners "will not receive events which were executed before they joined"; build view from state (polling). (V) [Q6]
- Takeaway for us: Quantum keeps events OUT of the sim. Our item 5 (one-tick events read by systems) is closer to a signal-like in-sim notice, so Quantum gives no direct model for in-sim event readers. (I)

### System order and statelessness
- "Be stateless: no mutable fields should be declared in systems." "Quantum only guarantees determinism if all (mutable) game state data is fully contained in the Frame instance." (V) [Q3]
- Registration order = execution order, and that is what makes it deterministic. (V) [Q3]
- `frame.SystemDisable<T>()` / `SystemEnable<T>()`; enabled state is in the frame. (V) [Q3]

### FrameTimer = the delay primitive: a stored target frame, polled
- `FrameTimer.FromFrames(frame, 60)`, `FrameTimer.FromSeconds(frame, 2)` ("rounds up to whole frames"), `default(FrameTimer)` = not set. (V) [Q1]
- Queries: `IsRunning(frame)`, `IsSet`, `HasStoppedThisFrame(frame)` ("Only if the timer ran out this exact frame"), `RemainingFrames`, `RemainingSeconds`, `ElapsedFrames`, `ElapsedSeconds`, `DurationNormalized`. (V) [Q1]
- Storage: "2 x int"; it "keeps a memory of the target frame". (V) [Q1] Probably start + target frame. (I)
- Counts sim frames; ignores `Frame.GameTimeScale`. (V) [Q1]
- No callback on expiry. Code polls. (V for absence) [Q1]
- Pitfall (I): `HasStoppedThisFrame` is edge-triggered. If the polling system is disabled, or the entity lacks the filter that frame, the edge is missed forever. A `due <= now` level check (our design) is safer.
- Takeaway: Quantum stores "when" as an absolute frame number in state. Same as our due tick. It has no central delayed-action queue; each component holds its own timers. (I)

### Commands (outside input into the sim)
- `class CommandSpawnEnemy : DeterministicCommand` with `public override void Serialize(BitStream stream)`. (V) [Q5]
- Send: 3.1 `Game.AddCommand(command)` (3.0 `SendCommand`); "All commands queued during a frame are packed together and sent once per frame." (V) [Q5]
- Read in sim: `frame.GetPlayerCommand(player) as CommandSpawnEnemy`, `frame.TryGetPlayerCommand<T>(player, out var c)`, 3.1 `frame.GetPlayerCommands<T>(player)`. (V) [Q5]
- 3.0: "Only one command per player and tick is included in the input stream" -> extra commands slip to later ticks; `CompoundCommand` workaround; obsolete in 3.1. (V) [Q5]
- "Quantum Commands are fully reliable." Remote clients "cannot predict the tick in which the command is received". (V) [Q5]
- No callback pattern is enforced; the game picks a system that polls `GetPlayerCommand` per player. (S: fetcher summary) [Q5] Looping players by index gives a fixed order. (I)
- Takeaway: commands are a per-player, per-tick input slot, read by polling in a fixed system. Like our "owner drains at one fixed point". (I)

### Frame state rules
- "`Frame.Number` is the tick of the frame." (V) [Q9]
- "Quantum always rolls-back and re-simulates frames." "Before predicting, the predicted frame copies the component data from the verified frame." (V) [Q9]
- `Frame.User.cs` partial = extra rollback state; "Adding an excessive amount of data to the frame will impact performance (de/serialization), as well as affect late joins." (V) [Q9]
- FrameContext (`frame.Context`) "is not part of the game state, it is not serialized for late joiners and not restored on rollbacks." "Writing gameplay state into the frame context will cause desyncs." (V) [Q9]
- Singleton: `singleton component MySingleton { FP Foo; }`; `GetSingleton<T>()` throws if missing; `TryGetSingleton<T>(out T)`. (V) [Q8]
- Iteration: `frame.Filter<A,B>()` + `while (filtered.Next(out e, out a, out b))`; `frame.GetComponentIterator<T>()`. No ordering guarantee stated on the page. (V for absence) [Q8]
- "Adding or removing table components while a filter iterates the same entities is safe and does not need to be deferred." (V) [Q8]
- Physics callbacks are signals (`ISignalOnTriggerEnter3D { void OnTriggerEnter3D(Frame frame, TriggerInfo3D info); }`); "disabled systems do not get any signal executed in them"; same pair with callbacks on both sides fires twice. Example destroys an entity inside the callback. No ordering stated. (V) [Q10]
- Takeaway: Quantum gets determinism from (a) everything in the frame, (b) fixed system order, (c) deterministic containers/ids. It does not sort requests; order = system order x storage order. Our explicit sort is stricter than Quantum. (I)

### Dynamic collections (how queues live in frame state)
- "Quantum's custom allocator exposes blittable collections as part of the rollback-able game state." (V) [Q4]
- DSL: `list<EntityRef>`, `dictionary<K,V>`, `hash_set<T>` inside components. Stored as heap pointers (`QListPtr<T>`). (V) [Q4]
- API: `Frame.AllocateList<T>()`, `Frame.FreeList(ptr)`, `Frame.ResolveList(ptr)`; same for Dictionary, HashSet. (V) [Q4]
- `[AllocateOnComponentAdded]`, `[FreeOnComponentRemoved]` ("Do NOT use ... with cross-referenced collections"). (V) [Q4]
- Pitfalls: must free or leak; double free "puts the heap in an invalid state"; "pointers cannot be cached in-between frames"; after free "HAS TO be nullified by setting it to `default`". (V) [Q4]
- `global { ... }` block = frame globals, part of rollback state. Singleton components are the other option. (V) [Q4]
- Takeaway: a request queue in Quantum would be a `list<Request>` on a component or in globals. Cardinal's `cardinal.Slice[T]` plays the same role and needs no free. (I)

## Unreal Mover

Access note: Epic's C++ API pages are JS-rendered; WebFetch got only "Table of Contents". UE source on GitHub needs an Epic-linked account (gh got 404). So API details below are (S) from search snippets of the official API pages, unless marked.

### Rule: no outside writes
- "Unlike CMC, the state of the Mover actor is not directly modifiable externally at any time." "there is no velocity property to manipulate directly." Use "modes, instant effects, and other mechanisms to affect change". (V) [M1]
- This is our rule 1 + 2 exactly: the movement owner is the only writer; others submit requests. (I)

### Three request kinds (what outsiders can queue)
- Layered moves: "generate a proposed move and rely on the active movement mode to execute it." "Multiple layered moves can be active at a time, and they are mixed with other influences." "Their lifetime can have a duration or be instantaneous, and they survive through movement mode changes." (V) [M2]
- Instant effects: "instantly change the movement simulation state without consuming any time" (teleport, forced velocity); don't need a mode. (V) [M2]
- Movement modifiers: for "crouch or stance changes"; change sim parameters, produce no movement. (V) [M2]
- So Mover splits requests by lifetime: one-shot (instant effect), timed contribution (layered move, has duration), timed parameter change (modifier, has handle). (I)

### When queued things apply
- `UMoverComponent::QueueInstantMovementEffect(...)`: "Queue a Instant Movement Effect to take place at the end of this frame or start of the next subtick - whichever happens first". (S) [M3]
- Overload taking a scheduled effect: "Queue a scheduled instant movement effect. Thread safe, can be used outside the game thread." (S) [M3][M4]
- `K2_QueueLayeredMove`: applies on the next simulation frame; "will clone whatever move you pass in, so you'll need to fully set it up before queuing". (S) [M3]
- `QueueMovementModifier`: starts the modifier "during the next simulation frame"; also clones. (S) [M3]
- Cancel is deferred too. Engine comment quoted on the forum: "// Request cancelation of any matching ACTIVE movement features during the next simulation tick" (on `CancelFeaturesWithTag`). (V for the forum quote; S for the engine source) [M6]
- `UMoverSimulation` keeps "GT staging, write-locked queues"; a lock "Protects the three simulation-level staging queues". (S) [M8]
- Takeaway: Mover = per-actor staging queues, drained at the start of the owner's next sim step. Clone-on-queue = request is a value, not a pointer. Matches our "request after owner ran waits for next tick". (I)

### Scheduled (due-frame) requests
- `ScheduleInstantMovementEffect`: "Queue a scheduled Instant Movement Effect to take place after a delay, tunable in the NetworkPhysicsSettingsComponent"; min delay `EventSchedulingMinDelaySeconds`. (S) [M3]
- Epic's advice: default to `QueueInstantMovementEffect`, "except for advanced use cases where it is important the effect get executed on the same frame on all end points". Con: "a delay in execution, which must be masked with an anticipation animation". (S) [M3]
- `FScheduledInstantMovementEffect` fields: `ExecutionServerFrame` (int32) "Server frame at which this instant movement effect should be applied" (fixed step only); `ExecutionServerTimeSeconds` (double) "Server Time (in seconds) after which..." (variable step only). Methods `ShouldExecuteAtFrame`, `ShouldExecuteAtTime`; static `ScheduleEffect` "Turns a FInstantMovementEffect into a scheduled one". (S) [M4]
- ChaosMover `FChaosNetInstantMovementEffect`: "Version of a FScheduledInstantMovementEffect with networkable instanced struct instead", also carries `ExecutionServerFrame`. (S) [M4]
- Takeaway: Mover's due tick is an absolute server frame on the request itself. Same as our design item 3. Mover uses it for cross-machine sync; we use it for gameplay delays too. (I)

### Mixing order for many active requests
- `FLayeredMoveBase`: `MixMode` (`EMoveMixMode`: AdditiveVelocity, OverrideVelocity, OverrideAll, OverrideAllExceptVerticalVelocity), `Priority` ("higher numbers take precedence when layered moves have conflicting overrides"), `StartSimTimeMs` (< 0 = not started), `DurationMs` (> 0 expires; 0 = "ticked only once, regardless of time step"; < 0 = ended manually). (S) [M5]
- `UMovementMixer::MixLayeredMove`: "in charge of mixing Layered Move proposed moves into a cumulative proposed move based on mix mode and priority". (S) [M5]
- Pitfall: default root-motion layered move is `OverrideAll`; users found layering "always behave as a total override". Epic: "the layered move needs its MixMode set to EMoveMixMode::AdditiveVelocity". (V) [M7]
- Takeaway: when many requests target one field, Mover resolves by (priority, mix mode), not arrival order. Our sort key could be (due, priority, source, seq). (I)

### Pitfall: expiry check before contribution (off-by-one)
- User report: Mover checks `FLayeredMoveBase::IsFinished` and removes expired moves, "then we invoke FLayeredMoveBase::GenerateMove" -> the final frame's sample (curve end, TimeValue=1) is never produced. (V) [M9]
- Reply by PyroJimmy (Epic staff per [M7]): "I believe the handling of time and IsFinished ordering is correct." It is about sampling the step at T0 vs T1. (V) [M9]
- Lesson for due ticks (I): define precisely whether a request with `due == now` runs this tick (yes, `due <= now`), and whether a timed effect's last tick is inclusive. Write it as a test.

### Networking
- Clients run "predictively ahead of the server"; inputs "must be combined into a single input command for the movement simulation tick"; "The server buffers these inputs until their simulation time comes." (V) [M1]

## Unreal GAS

Access note: UE source not reachable (Epic-gated GitHub). [G2] quotes engine source; I mark those (S) since I did not see the engine file. Epic page [G1] is (V).

### Effect = request object (spec) with source context
- `GameplayEffectSpec` = the runtime instance that "is actually what is applied to the Target". Made by `UAbilitySystemComponent::MakeOutgoingSpec()`. "do not have to be immediately applied ... pass a GameplayEffectSpec to a projectile". (S) [G2]
- Spec holds: effect class, level, duration, period, stack count, `GameplayEffectContextHandle` ("who created this"), snapshotted attributes, dynamic tags, `SetByCaller` maps (`TMap<FGameplayTag, float>`). (S) [G2]
- `FGameplayEffectContext` holds instigator and TargetData; teams subclass it to carry more data. (S) [G2]
- All apply helpers end in `UAbilitySystemComponent::ApplyGameplayEffectSpecToSelf()` on the Target. Outside code (projectile) gets the Target's ASC and calls `ApplyGameplayEffectToSelf`. (S) [G2]
- So GAS's request = spec + context (source, causer, level, magnitudes). It is applied SYNCHRONOUSLY into the target's ASC, which is the owner. One write path, not a queue. (I)
- Mapping: our request's `source`/`cause` = GAS's EffectContext (instigator, causer, source object). GAS has no depth field. (I)

### Duration policies (how delays and over-time work)
- "A Gameplay Effect has a Duration that can be set to Instant, Infinite, or Has Duration." Instant: "never makes its way into the Active Gameplay Effects Container" - it is "Executed". (V) [G1]
- "Periodic effects are executed at every period; therefore, it is both "Added" and "Executed"." (V) [G1]
- Instant and periodic change `BaseValue`; Duration/Infinite change `CurrentValue` (removed on expiry). (S) [G2]
- Duration/Infinite effects can switch off and on by "Ongoing Tag Requirements" without being removed. (S) [G2]
- Active effects live in `ActiveGameplayEffects` (`FActiveGameplayEffectsContainer`), returned as `FActiveGameplayEffectHandle`. (S) [G2]
- Takeaway: GAS has no "apply at tick T" request. Delay = a Duration effect, or an ability task timer. A periodic effect = a stored repeating request. (I)

### Modifier order (deterministic aggregation, not arrival order)
- Ops: Add, Multiply, Divide, Override. (S) [G2]
- Engine formula (quoted from `FAggregatorModChannel::EvaluateWithBase`, GameplayEffectAggregator.cpp): `((InlineBaseValue + Additive) * Multiplicitive) / Division`. (S) [G2]
- "Any Override Modifiers will override the final value with the last applied Modifier taking precedence." -> Override is order-dependent. (S) [G2]
- Multiply mods are SUMMED with bias 1: `1 + (Mod1 - 1) + (Mod2 - 1)`; two 1.5x = 2x, not 2.25x. Teams must "change the engine code" to multiply. (S) [G2] Critique: designed "with Paragon in mind"; 0.5, 0.5 gives 0. (S) [G2]
- Qualifier hook `FAggregatorEvaluateMetaData` e.g. `MostNegativeMod_AllPositiveMods` (Paragon slows: only the worst slow applies). (S) [G2]
- Takeaway: GAS gets order-independence by making Add and Multiply commutative sums, then applying in fixed op order. Override is the one order-dependent op. Our sorted drain makes even "last wins" deterministic. (I)

### Execution calculations (the "owner resolves the request" step)
- "Executions use the `UGameplayEffectExecutionCalculation` to define custom behaviors" for "complex equations that aren't adequately covered by Modifiers". (V) [G1]
- Only on Instant and Periodic effects; can change many attributes; "can not be predicted"; C++ only. Only run on the server for predicted abilities. (S) [G2]
- Attribute capture: snapshot at spec creation vs at application. (S) [G2] -> same question as our "read target state at enqueue or at drain" (I). At drain is the default for the owner-queue design.
- `PreGameplayEffectExecute`: "can reject or alter the proposed modification"; `PostGameplayEffectExecute`: "can react to the change". (V) [G1]
- Meta attributes: "we commonly define damage as a Meta Attribute ... a placeholder", processed in PostGameplayEffectExecute (shield first, then health). "no persistence between GameplayEffects and is overriden by every one". (S) [G2]
- Takeaway: Meta attribute = a one-slot inbox owned by the target. Our queue component generalizes it to many slots, drained in order. (I)

### Interception (immunity, requirements)
- `UImmunityGameplayEffectComponent`: "Immunity is blocking the application of other GameplayEffectSpecs." `UTargetTagRequirementsGameplayEffectComponent`, `UCustomCanApplyGameplayEffectComponent`. (V) [G1]
- Delegate `OnImmunityBlockGameplayEffectDelegate` fires on a block. (S) [G2]
- Takeaway: rejection happens in the owner at apply time. Our owner drain is the natural place for immunity/flags checks. (I)

### Gameplay cues = cosmetic notices, unreliable
- "Gameplay Cues are a network-efficient way to manage cosmetic effects"; tags must start with `GameplayCue`; events On Active, While Active, Removed, Executed. (V) [G1]
- "GameplayCues in general should be considered unreliable and thus unsuited for anything that directly affects gameplay." Executed cues use unreliable multicast. (S) [G2]
- Batching: `FScopedGameplayCueSendContext` "suppresses `UGameplayCueManager::FlushPendingCues()` until it falls out of scope" -> cues are queued then flushed. (S) [G2]
- Max "two of the same GameplayCue RPCs per net update". (S) [G2]
- Takeaway: like Quantum events, cues never change state. Same split as our item 5 (events) vs item 2 (requests). (I)

### Prediction keys
- Client makes an Activation Prediction Key, sends it with `CallServerTryActivateAbility()`, tags every effect it applies with it; server tags its effects with the same key and replicates back; client removes all its effects for a now-stale key. (S) [G2]
- Key valid only in an atomic "window" ("You can think of this as being only valid during one frame"). Latent task callbacks lose it unless the task makes a new Scoped Prediction Window. (S) [G2]
- What is NOT predicted (quoted from GameplayPrediction.h): "GameplayEffect removal", "GameplayEffect periodic effects (dots ticking)"; Executions don't predict. (S) [G2]
- Predicted Instant effects are "treated like Infinite GameplayEffects so that they can be rolled back"; can cause a "blip" (double apply). (S) [G2]
- Epic (Dave Ratti): "predict the minimum amount of stuff you can get away with". Paragon and Fortnite "do not predict damage". (S) [G2]
- Takeaway: the prediction key is a "cause id" stamped on each request so a whole chain can be undone together. Our `cause` field could serve the same role later. (I)

### Pending adds/removes while iterating (GAS's own deferral queue)
- `FScopedActiveGameplayEffectLock`: "locking the active gameplay effect list while possibly invoking callbacks into gamecode". (S) [G3]
- `FActiveGameplayEffectsContainer` has `int32 ScopedLockCount`; "any pending remove gameplay effects are deleted at the end of their scope lock"; raw accessor "will return GameplayEffects that are pending removal!"; `FActiveGameplayEffect::PendingNext` "Cached pointer" (pending list link). (S) [G3]
- Bugs: UE-199659 "~FScopedActiveGameplayEffectLock has 1 pending removes after a scope lock removal" (ensure in `DecrementLock`); UE-201125 similar for abilities added during an ability-list scope lock. (S) [G4]
- Ability list: `ABILITYLIST_SCOPE_LOCK();` increments `AbilityScopeLockCount`; "Do not try to remove an ability inside the scope". (S) [G2]
- Prior survey also notes GAS release-note fixes for endless recursion in EndAbility (4.27) and a recursive crash (5.3). (S) prior SURVEY [24]
- Takeaway (I): GAS applies synchronously, so callbacks re-enter the container mid-loop. It needed scope locks + pending lists, and they still had bugs. A drain-at-one-point queue avoids this class of bug: requests added during a drain go to the next drain (or next tick), never mid-loop.

### Time base
- Durations are world-time floats: duration change code resets `StartServerWorldTime`, `CachedStartServerWorldTime`, `StartWorldTime` then calls `CheckDuration()`; uses a `const_cast` ("may not be Epic's intended way"). (S) [G2]
- Contrast (I): GAS uses seconds and timers; Quantum (FrameTimer) and Mover fixed-step (`ExecutionServerFrame`) use integer frames. Integer due ticks are the deterministic choice.

### WaitDelay and latent ability tasks
- `UAbilityTask_WaitDelay` in `AbilityTask_WaitDelay.h`: public `OnFinish` delegate, protected float `Time`; documented as functionally the same as a Delay node. (S) search snippet of Epic API ref. I could not confirm that it uses `FTimerManager::SetTimer` (likely, (I)).
- "Other tasks like the WaitDelay task do not have built-in code to create a new scoped prediction window for its callback." Workaround: `WaitNetSync` with `OnlyServerWait`. (S) [G2]

## Mapping to the decided architecture (I)

All of this section is (I) unless a source tag is given.

| Design item | GAS | Mover | Quantum | Note for Cardinal API |
|---|---|---|---|---|
| 1 one owner writes | Attribute changes go through effects into the target ASC [G1][G2] | "not directly modifiable externally at any time" [M1] | No rule; any system writes any component [Q3] | Mover is the closest match. |
| 2 request queue drained at one point | No queue: `ApplyGameplayEffectSpecToSelf` is synchronous; needs scope locks + pending lists [G2][G3] | Per-actor staging queues; applied next sim frame / next subtick [M3][M8] | Commands polled per player per tick [Q5]; signals are sync calls [Q3] | Mover proves "next tick" latency is acceptable for movement. GAS shows the cost of sync apply. |
| 3 due tick | None on effects; durations in seconds [G2] | `ExecutionServerFrame` on `FScheduledInstantMovementEffect` [M4] | `FrameTimer` stores target frame, polled [Q1] | Store absolute `Due uint64` tick on the request. Check `due <= now`, not `==` (Quantum `HasStoppedThisFrame` is edge-only). |
| 4 source/cause/depth/flags | `FGameplayEffectContext` (instigator, causer, target data), prediction key, dynamic tags [G2] | Requests cloned on queue [M3]; Priority, MixMode [M5] | Commands carry player slot [Q5] | None of the three has a depth cap. Depth is our addition; keep it. Clone-on-queue matches value-type requests. |
| 5 one-tick event | Gameplay cues: cosmetic, unreliable [G1][G2] | (OnPostSimulationTick delegate; snippet only) | Events: never change state; de-dup by hash; dispatched to view [Q2][Q4] | Both engines keep notices OUT of state changes. Our in-sim events read by systems are more like Quantum signals. |
| 6 delayed event queue | None | `ScheduleInstantMovementEffect` (delayed request, not event) [M3] | None; use FrameTimer in a component [Q1] | No precedent found for a delayed EVENT queue. Precedent exists for delayed REQUESTS (Mover). Consider: is item 6 just item 3 with an event payload? |
| 7 sorted order | Aggregation by op order: `((Base+Add)*Mul)/Div`; Override = last applied [G2] | Mixer by Priority + MixMode [M5] | Registration order, frame-contained state [Q3][Q9] | Sort key idea: (due, priority, target id, source id, seq). Prefer commutative merges (sum, max) so order matters less, like GAS Add/Mul. |

### Concrete API ideas taken from the sources (I)
- Request is a value; copy on enqueue (Mover clones [M3]). In Go: `cardinal.Slice[Req]` on the owner's queue component; `Insert(Len, x)`.
- Request header: `Due uint64` (0 = asap), `Source EntityID`, `Cause uint64` (chain id, like a GAS prediction key [G2]), `Depth uint8`, `Flags uint32`, `Seq uint32` (tie-break). Priority optional (Mover [M5]).
- Drain: owner system reads all `due <= now`, sorts by key, applies, keeps `due > now`. Requests added during the drain go to next tick (avoids GAS-style scope-lock bugs [G3][G4]).
- Apply step = GAS "execution" hook: owner may reject (immunity [G1]) or transform (meta attribute pattern [G2]).
- Read target state at drain time (GAS "no snapshot" default [G2]), not at enqueue.
- Timers: store absolute tick (Quantum [Q1], Mover [M4]); never seconds (GAS [G2]).
- Inclusive-boundary test for the last tick of a timed effect (Mover off-by-one thread [M9]).

## Could not find
- Unreal engine source (GameplayEffect.cpp, AbilityTask_WaitDelay.cpp, MovementModeStateMachine.cpp, MoverSimulation): GitHub access is Epic-gated (gh 404). Epic C++ API pages are JS-rendered (fetch returned only "Table of Contents"); issues.unrealengine.com returned 403. So all Mover API signatures and GAS internals are (S).
- Exact Mover sim-tick step order (when staged queues flush vs transitions vs GenerateMove). Only the forum description of IsFinished-before-GenerateMove [M9].
- Whether Mover cancel/modifier queues are sorted or FIFO. Not found.
- Whether `WaitDelay` uses `FTimerManager` (likely) - not confirmed.
- GAS pending-ADD semantics (do effects added during a scope lock apply at lock end?) - only the pending-remove side is documented [G3].
- Quantum: order of multiple signal implementers beyond "order in which Systems are inserted" [Q3]; behavior of a signal raised inside a signal (re-entrancy); iteration order guarantee of `Filter`/`QDictionary`; physics callback order. Not in the pages read.
- Quantum has no documented central delayed-action queue; no depth cap on signals.
- Critics/post-mortems specific to Quantum signals or FrameTimer: none found in the time spent.
