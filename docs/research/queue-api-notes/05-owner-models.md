# 05 — Owner-authority and request models

Researcher notes, 2026-11-19. Marks: (V) read primary this session, (S) secondary, (I) inferred.
Status: DONE. Raw downloads (pinned) were kept locally, not here.

## Verdict up front
- No engine I read has our full set (owner queue + due tick + source/cause/depth + sorted drain). Each has pieces. (V across [1]-[16])
- Closest matches: Overwatch's per-entity `ModifyHealthQueue` + `Contact` singleton (deferred, drained later in the same frame) [1], and SpatialOS GDK's flat per-command-type request list drained by the owner system [10].
- Strongest counter-example: SS14 (huge open codebase) enforces owner-only writes with a compile error but uses **synchronous owner methods**, not queues [2][3][6].
- Every same-tick drain loop I read (RobustToolbox `ProcessEventQueue`, Donburi `ProcessEvents`, GoWorld `post.Tick`) loops until empty with **no cap**. Our "late request waits for next tick" + depth cap is stricter than all of them. (V)[5][14][16]
- Every engine that saves delayed work stores it as **data with an absolute due time**, never a closure (SS14, GoWorld). SS14 explicitly discourages its closure `Timer`. (V)[5][16]

## Notes

### A. Overwatch (Tim Ford, GDC 2017), via Chinese translation [1]
Read the full Sohu text myself this session. It is a translation, not Ford's words, so marks are (V) for "the translation says", and (S) relative to the talk itself.

- Rules list (V)[1]: "组件没有函数；System没有状态；共享代码要放到Utils里；组件里复杂的副作用要通过队列的方式推迟处理，尤其是单例组件；System不能调用其他System的函数"
  - = components have no functions; systems have no state; shared code goes in Utils; complex side effects in components are deferred through queues, especially singleton components; a System may not call another System's functions.
  - Note the queue rule is phrased as "deferred via queues", not "owner-only writes". (I)
- Deferment definition (V)[1]: "“推迟”存储了行为所需状态，然后把副作用延后到当前帧里更好的时机再执行。"
  - = deferment stores the state the behaviour needs and runs the side effect at a better point **later in the current frame**. Same-frame, not next-frame. (V)
  - Guiding question (V)[1]: "这些代码，是必须现在就执行吗？" = does this code have to run now?
- Contact singleton (V)[1]: "它包含了一个未决的碰撞记录的数组，每个记录都有足够的信息，来在本帧的晚些时候创建那个特效。" Producers "只需要添加一条新记录并填充数据" (just append a record). Late in the frame "ResolveContactSystem 会遍历数组" and spawns effects with LOD rules.
  - API shape (I): `Contact { pending []ContactRecord }` singleton; many writers append; one system drains. Record must be self-contained ("足够的信息").
  - Benefits listed (V)[1]: one call site for the heavy side effect; data locality; budgeting (spread effects across frames to smooth spikes); room for complex resolve logic (dedupe scorch marks over bullet holes).
  - Budgeting = a drain may intentionally not empty the queue every frame. (V for "其他特效可以在后面的运算过程中分摊开来") — relevant: owner may carry requests over ticks.
- ModifyHealthQueue (V)[1]: "如果你的实体是敌对的...而且你有一个ModifyHealthQueue组件，你就可以被别的玩家击中" and "ModifyHealthQueue是服务器记录的你身上的全部伤害和治疗。与单例Contact类似，也是延迟计算的，而且有多个调用点".
  - Key fact: ModifyHealthQueue is a **per-entity component**, not a singleton. Having it is what makes an entity damageable (component presence = capability). (V)
  - The text does not name the system that drains it, nor when in the frame. (V: absent)
  - "伤害，也完全不会在客户端预测" = damage is never predicted on the client. (V)
  - Being rewound (utility functions on MovementState) and taking damage (deferred queue) are "two independent behaviours on separate component slices". (V)
- Deferred create/delete (V)[1]: early on both were deferred to end of frame. "推迟销毁一点问题都没有，而推迟创建却有一大堆副作用" — create in System A, use in System B = one frame late. Changed after launch (patch 1.2/1.3) to mid-frame creation.
  - Lesson for us (I): deferring *destroy* and *mutations* is fine; deferring *create* hurts when a later system in the same tick needs the new entity.
- Singletons (V)[1]: ~40% of components are singletons ("我们整个游戏里的40%组件都是单例的"). Killcam needed a second ECS world, which broke global system access, so system state moved into singleton components.
- Utility functions (V)[1]: a utility called from many places should depend on few components and have few/no side effects; if it touches many components, limit call sites. "如果只有一个调用点，那么行为的复杂性就会很低".
- CommandSystem (V)[1]: became "唯一一处能够产生与玩家输入命令相关副作用的地方" (the only place with side effects from player commands).
- Counts (V)[1]: client ~46 systems, 103 components. System update order is a fixed list ("以轮询顺序排列").
- Prediction (V)[1]: predicted systems run in UpdateFixed on fixed command frames; movement and abilities roll back and replay; damage/health not predicted.
- Not in the text (V: absent): request metadata (source/cause/depth), any depth cap, any due-tick/delay on the queue, ordering inside the queue, what happens to requests targeting a dead entity.

### B. Space Station 14 / RobustToolbox [2][3][4][5][6]
RobustToolbox commit 0fb119a9 and SS14 content commit in raw/ss14/sha.txt, read 2026-11-19.

**Owner-only writes, enforced at compile time**
- `[Access(typeof(OwnerSystem))]` on a component (or field). Defaults (V)[3]: `SelfDefaultPermissions = ReadWriteExecute; FriendDefaultPermissions = ReadWriteExecute; OtherDefaultPermissions = Read`.
- Outside write = compile error (V)[3]: "Error RS2008: Tried to perform write access to member 'Counter' in type 'MyComponent', despite read access." Analyzer is `DiagnosticSeverity.Error` (V)[3: AccessAnalyzer.cs].
- Real use (V)[6]: `DamageableComponent` is `[Access(typeof(DamageableSystem), Other = AccessPermissions.ReadExecute)]`, and its `Damage` field is `Other = AccessPermissions.None` (others cannot even read the raw field; they call getters).
- Docs (V)[2]: "Components should specify their access restrictions whenever possible, usually only allowing the entity systems that wrap them to modify their data."
- Go analogue (I): a `go/analysis` linter keyed on a per-component owner declaration (e.g. owner package path) can flag writes from other packages. Package-private fields alone are not enough because Cardinal components are plain exported structs.

**How others ask the owner: a synchronous method, not a queued request**
- "Method Events are *prohibited*, always use Entity System Methods instead." (V)[2] Example given: `RaiseLocalEvent(uid, new ChangeDamageEvent(10))` is banned; `DamageableSystem.ChangeDamage(uid, 10)` is required.
- Exception (V)[2]: "You may use Method Events as long as they're wrapped by an Entity System Method."
- History (V)[4]: stack splitting stopped using a method event "since the anti-method event revolt of 2021".
- Owner API shape (V)[6]: `ChangeDamage(Entity<DamageableComponent?> ent, DamageSpecifier damage, bool ignoreResistances = false, bool interruptsDoAfters = true, EntityUid? origin = null, bool ignoreGlobalModifiers = false)` returns actual damage dealt.
  - Request = target + payload + **origin** (source) + **bool flags**. Matches our "source + flags" fields. No cause chain, no depth. (V)
  - Pipeline inside the owner (V)[6]: `BeforeDamageChangedEvent` (directed, cancellable: "if (before.Cancelled) return") → resistances → `DamageModifyEvent` (others edit `ev.Damage`) → global modifiers → `DamageDealtEvent`.
  - So other features influence damage by subscribing to directed events *the owner raises*, never by writing the component. (V)
- Difference from our design (I): SS14 applies immediately, inside the caller's update. No fixed drain point, no sorted order. Determinism comes from single-threaded fixed system order, not from sorting requests.

**Event bus mechanics**
- Directed events: raised on one entity, delivered to subscriptions keyed (component type, event type) on that entity. Duplicate subscription for the same pair throws "Duplicate Subscriptions for comp=..., event=..." (V)[5: EntityEventBus.Directed.cs].
- "Directed events are always raised before broadcast events." (V)[4]
- Ordering: optional `before`/`after` type arrays per subscription, topologically sorted (`TopologicalSort.FromBeforeAfter`) (V)[5: Ordering.cs]. Docs: "Sorted event subscriptions use a slower path, so only use them when you really need them." (V)[4]
- Events must be structs raised by ref (V)[2]: "Events should always be structs, not classes, and should always be raised by ref."
- Queued events (V)[5]: `QueueEvent(EventSource source, EntityEventArgs toRaise)` "Queues an event to be raised at a later time." Drained by `ProcessEventQueue()` with `while (_eventQueue.Count != 0)` — events queued by handlers during the drain run in the same drain. No depth or count cap. (V)
- Tick order in `EntityManager.TickUpdate` (V)[5]: systems update → `EventBusInternal.ProcessEventQueue()` → `ProcessQueueudDeletions()` → `CullRemovedComponents()`. So queued events and deletions are end-of-tick sync points.

**Deferred deletion**
- `QueueDeleteEntity(uid)` adds to a `Queue<EntityUid>` plus a `HashSet` for dedupe (V)[5]: "if (!QueuedDeletionsSet.Add(uid.Value)) return".
- Cancellable before flush (V)[5]: "// The deletion may have been canceled since it was queued. if (!QueuedDeletionsSet.Remove(uid)) continue;"
- API shape lesson (I): a request store with set-membership gives dedupe + cancel cheaply.

**Future work: data on the entity, compared to CurTime — not callbacks**
- Engine `Timer` (callback after N ms) is explicitly discouraged (V)[5: Timer.cs]: "Non-serializable, but async friendly timers. ... Using these in Space Station 14 is discouraged, it has its own idioms that are all serialization friendly."
- Docs (V)[2]: "Update loops should compare against `CurTime` instead of accumulating `frametime`." and "Async for any game simulation code should be avoided at all costs, as it's generally virulent, cannot be serialized".
- Absolute due times are `TimeSpan` fields on components; `[AutoGenerateComponentPause]` + `[AutoPausedField]` generates a system that adds `args.PausedTime` on unpause (V)[5][2]. `TimeOffsetSerializer` saves them relative to current time (V)[2]: "should always be used for runtime-modified `TimeSpan` fields that represent absolute times rather than durations."
  - For us (I): store **absolute due tick** in the request. Snapshots restore the tick counter, so no offset trick needed unless Cardinal ever pauses/ rebases ticks.
- `TimedDespawnComponent { float Lifetime = 5f }` (V)[5]: a counting-down float, the old style the docs now discourage. Its system: "if (!_timing.IsFirstTimePredicted) return;" with comment "// AAAAAAAAAAAAAAAAAAAAAAAAAAA // Client both needs to predict this, but also can't properly handle prediction resetting." Not networked: "we don't want clients predicting networked entity deletions." Collects due uids into a set first, then raises `TimedDespawnEvent` and `QueueDel` (two-phase: collect then act). (V)
- DoAfter = the delayed-action mechanism (wind-ups, channelled actions) (V)[6]:
  - Per-entity `DoAfterComponent { ushort NextId; Dictionary<ushort, DoAfter> DoAfters }`, `[Access(typeof(SharedDoAfterSystem))]`.
  - `DoAfterArgs`: `User` (required), `Delay TimeSpan` (required), `Target?`, `Used?`, `Event DoAfterEvent` (required — the event to raise when done), `EventTarget?`, `Broadcast`, cancel flags `BreakOnMove`, `BreakOnDamage`, `BreakOnHandChange`, `BreakOnDropItem`, `DistanceThreshold`, `RequireCanInteract`, `BlockDuplicate`, `AttemptFrequency`.
  - Update compares `time - doAfter.StartTime >= doAfter.Args.Delay`; checks `ShouldCancel` each tick; can raise an attempt event that others cancel. (V)
  - API lesson (I): a delayed request needs **cancel conditions** and **dedupe** ("BlockDuplicate"), not only a due time. A wind-up is a request that the owner re-validates at fire time.
- Prediction (V)[4: prediction-guide]: "`IsFirstTimePredicted` is currently misused all across the codebase to hide mispredicts". Predicted code runs "10+ times on the client" as it re-simulates.
  - For us (I): any request queue that a predicted client re-runs must be part of rolled-back state, or it double-applies. Cardinal saves components, so a queue component is rolled back with the snapshot — good.

### C. SpatialOS (Improbable) — commands to the authoritative worker [7][8][9][10]
Docs archived 2021-09-28 (product shut down). GDK for Unity repo is archived (pushed 2022-01-17).

**Authority model**
- "SpatialOS only allows one worker instance to have write access authority to an entity's component at any one time. Only the worker instance with write access authority can make changes to an entity component's value." (V)[8]
- Reason given (V)[8]: "you don't want more than one worker instance at a time to make changes to this component because this could lead to conflicts or deadlock."
- Authority moves between workers as entities cross load-balancing areas (V)[8]. In one Cardinal shard the owner never moves (I) — so most of SpatialOS's failure modes do not apply.

**Commands**
- "Commands are similar to remote procedure calls (RPCs)." (V)[7] "they allow any worker to send a request to the worker with write access authority over a specific component on a specific entity. The receiving worker can take action and should respond to the request." (V)[9]
- Declared in schema on the component: "Every command defines a request and response message" (V)[7]. Example schema line (V)[9]: `command Response open_door(DoorActionRequest);`
- Two handlers (V)[7]: a request handler on the owner, a response handler on the sender.
- Deadline (V)[7]: "The maximum (and default) deadline is five seconds." And: "the sending worker instance can't use deadlines to decide whether a component command was processed on the target worker instance." (= timeout does not mean "not applied"; at-most-once with unknown outcome.)
- Failure codes and advice (V)[7]:
  - Timeout → "Automatically retry with a backoff."
  - Not found (entity/component gone) → "Don't automatically retry."
  - Authority lost → retry if you still see the entity.
  - Application error → "We recommend you define your own failure status codes in schema. Then, indicate failures by returning successful command response messages that include these codes".
  - Internal error → don't retry.
- Short circuiting (V)[7]: handling locally when the sender believes it owns the component "can lead to situations where a worker instance executes a command despite just having lost write access authority over its component."
- Commands vs events (V)[9]: events are component updates, "good for fire-and-forget interactions"; "Request-response interactions could also be implemented by sending multiple events but this is not recommended." "Game logic which relies on multiple events to emulate a command can easily become messy and prone to programmer errors."
- Race resolution by the owner (V)[9]: two players pick up one item → "breaking the race condition by handling the request which arrives first with a success response type and the second one with a failure." And "a response containing a failure is different from a command failing itself."
- Owner validates (V)[9]: "The worker that has write access authority checks the player position to ensure the player is close enough ... Malicious clients may be able to forge a command".

**ECS mapping in GDK for Unity (the closest thing to our design)** (V)[10]
- Owner system drains a per-command-type list: `commandSystem.GetRequests<Launcher.LaunchEntity.ReceivedRequest>()` returns `MessagesSpan<T>`; each request has `EntityId`, `RequestId`, `Payload`. Owner replies with `commandSystem.SendResponse(new ...Response(request.RequestId, ...))`.
- Send: `CommandRequestId SendCommand<T>(T request, Entity sendingEntity = default)`; ids from `nextRequestId++` (monotonic per sender).
- Storage `CommandDiffStorageBase`: **one flat `MessageList<TRequest>` per command type for all entities** — not a list component per entity.
  - `GetRequests(EntityId target)` lazily sorts by `x.EntityId.Id.CompareTo(y.EntityId.Id)` then returns the contiguous range (binary search `GetEntityRange`).
  - `Clear()` empties every frame (requests live one frame in the diff). `RemoveRequests(long entityId)` drops requests for a removed entity.
  - Pitfall (S, .NET docs say List.Sort is unstable): the comparer keys on entity id only, so order of several requests to the same entity is not guaranteed after sort. For us (I): sort key must be total, e.g. (target, due tick, insertion seq).
- This matches the user's measurement (I): a shared per-type queue avoids the per-entity empty-queue cost (+130 us at 500 entities) and still gives per-target lookup by sort + range.
- Chains in the sample (V)[10]: handler of LaunchEntity sends LaunchMe; handler of LaunchMe sends IncreaseScore — request → request chains, no depth tracking.

**Not found**: any SpatialOS doc on command ordering guarantees between two senders, or on idempotency keys. No cause/depth fields.

### D. Go ECS libraries

**Ark (mlange-42/ark v0.8.3, 2026-06-01)** [11][12]
- Observers run immediately (V)[11]: "The observer callbacks are executed immediately by any emitted event."
- Order undefined (V)[11]: "Note that observer order is undefined. Observers are not necessarily triggered in the same order as they were registered."
- Remove events fire before the removal with the world **locked**; create/add/set fire after, unlocked for single ops, locked for batch ops (V)[11].
- Custom event API (V)[12]: `var registry ecs.EventRegistry; OnTeleport := registry.NewEventType()`; `world.Event(OnTeleport).For(ecs.C[Position]()).Emit(entity)`; observer `ecs.Observe1[Position](OnTeleport).Do(func(e ecs.Entity, p *Position){...}).Register(world)`.
  - `type Event struct { world *World; eventType EventType; mask bitMask }` — **no payload field**. Data travels as components on the target entity (or the zero entity). (V)[12: ecs/events.go]
  - `type EventType uint8`; "The maximum number of event types is limited to 256, with 7 predefined and 249 potential custom types." (V)[12]
- No systems or loop (V)[13]: "Ark does not include built-in systems or an update loop." Determinism claim is about iteration only (V)[13]: "Ark guarantees deterministic and reproducible iteration order."
- No queue, no delay, no depth cap in Ark itself. (V: absent from events doc and events.go)
- Lesson (I): Ark is the "immediate observer" model our design rejects for cross-feature writes. Its undefined order is exactly why we sort requests.

**Donburi (yohamta/donburi, features/events)** [14]
- API (V)[14]: `NewEventType[T any]() *EventType[T]`; `(e *EventType[T]) Subscribe(w donburi.World, s Subscriber[T])`; `Publish(w, event T)` appends to a queue; `ProcessEvents(w)`; global `ProcessAllEvents(w)`.
- Storage (V)[14]: the queue and subscriber list live in a component (`eventBusData[T]{ subscribers []Subscriber[T]; queue []T }`) on one singleton entity per event type, created lazily by `mustFindEventBus`. Queue-in-a-singleton-component, like our design. (V)
  - But the component holds funcs, so it cannot be snapshotted as plain data. (I)
- Drain (V)[14]: "// The outer loop is needed, because events can trigger more events." `for len(eventBus.queue) > 0 { queue := eventBus.queue; eventBus.queue = nil; ... }` — swaps the slice, drains, repeats until empty. **No depth or iteration cap**: a cycle hangs the tick. (V)
- Pitfall (V, from code): `Unsubscribe` swap-removes (`subscribers[index] = subscribers[len-1]`), so subscriber order changes after any unsubscribe.
- Every subscriber sees every event (broadcast). No target, no source/cause, no due time. (V)
- `ProcessAllEvents` order = query order over event-type entities = lazy creation order (I from code), which depends on which type was first published/subscribed.

**GoWorld (xiaonanln/goworld, Go MMO server; 2.7k stars)** [15][16]
- Delayed work is stored as **data, not closures** (V)[16]: `AddCallback(d time.Duration, method string, args ...interface{}) EntityTimerID` stores `entityTimerInfo{FireTime, Method, Args, Repeat}` in `e.timers[tid]`. Doc: "The callback will be cancelled if entity is destroyed".
  - Why data: `dumpTimers()` packs them with MessagePack for entity migration/save; `restoreTimers` re-arms each with `timer.FireTime.Sub(now)`. (V)[16]
  - Lesson (I): a delayed request must be plain data (target, kind, args, due) so it survives a snapshot. Closures cannot be saved — the same reason SS14 bans `Timer` for sim code.
  - Pitfalls (V from code): `dumpTimers` ranges over a Go map, so saved order is random; restore regenerates timer IDs with `genTimerId()`, so saved `EntityTimerID`s held elsewhere go stale. Uses wall clock (`time.Now()`), not ticks.
- `CancelTimer(tid)` by handle (V)[16]. Repeat timers clamp to 10 ms minimum (V).
- Cross-entity calls: `e.Call(id, method, args...)` — RPC by method name, routed to the owning game process (V)[16]. Same-space entities "can visit each other directly by calling methods or access attributes" (V)[15 README].
- Same-tick deferral: `post.Post(f)`; `post.Tick()` "loop until there is no callbacks posted anymore" (V)[16 post.go] — again **no cap**; a post-loop hangs the tick.

## E. What this means for the decided design (point by point)

1. **One owner per component.**
   - Precedent: SpatialOS (runtime-enforced) [8]; SS14 (compile error RS2008) [3]; Overwatch (convention only) [1].
   - Overwatch's warning (V)[1]: "仍然有大量代码不符合这个规范，所以它们是复杂度和维护工作的主要来源" = code that still breaks the rules is the main source of complexity and maintenance work. Convention alone leaks.
   - Suggestion (I): a `go/analysis` check like RS2008. Owner declared once per component (e.g. registering package). Writes = `Set`/field assign through a mutable getter from another package → error. Reads allowed by default, like SS14's `OtherDefaultPermissions = Read`.

2. **Requests to the owner's queue, drained at one fixed point; late = next tick.**
   - Overwatch drains later **in the same frame** (V)[1]. GDK drains requests received this frame and `Clear()`s each frame (V)[10]. Both rely on producers running before the drain.
   - Shape choice — per-entity vs shared:
     - Overwatch: per-entity `ModifyHealthQueue` component; presence = "can be damaged" (V)[1].
     - GDK: one flat list per request type; per-target view by sort + binary range (V)[10].
     - User measured per-entity queue ≈ +130 us/tick empty at 500 entities. GDK's shape avoids that. (I) Recommend: one singleton queue per owner/request type, `cardinal.Slice[Req]` with `Insert(Len, x)`; owner sorts once per drain and walks ranges per target.
   - Deferred **create** is the known trap (V)[1]: Overwatch shipped deferred create, then made it immediate post-launch because "System A creates, System B uses" cost a frame. If spawns become requests, a later system in the same tick cannot see the new entity. (I) Keep entity creation immediate inside the owner; queue only the spawn *request* when the requester is a different feature, and accept one-tick lag there.
   - Deferred **destroy** was fine (V)[1]. SS14 also defers destroy to end of tick and lets it be cancelled (V)[5].

3. **Due tick (0 = asap).**
   - SS14: absolute `TimeSpan` due on the component, compare to `CurTime`; "compare against CurTime instead of accumulating frametime" (V)[2]. Pause handled by generated offset system (V)[5].
   - GoWorld: `FireTime` absolute + method name + args (V)[16].
   - Recommend (I): store **absolute due tick** (uint64), not a countdown. Countdowns (SS14 `TimedDespawnComponent.Lifetime -= frameTime`) are the old style.
   - Wind-ups need more than a due tick: SS14 DoAfter carries cancel rules (`BreakOnMove`, `BreakOnDamage`, `DistanceThreshold`, `RequireCanInteract`) and `BlockDuplicate` (V)[6]. (I) Owner should re-validate at fire time; requester may need a cancel handle or dedupe key.

4. **source, cause, depth, flags + depth cap.**
   - SS14 `ChangeDamage(..., EntityUid? origin, bool ignoreResistances, bool interruptsDoAfters, bool ignoreGlobalModifiers)` = source + flags (V)[6]. No cause chain, no depth.
   - SpatialOS: `RequestId` (per sender, monotonic) + payload; chains of commands with no depth tracking (V)[10].
   - No engine I read has a depth cap. (V: absent in [1][5][7][10][11][14][16])
   - Caps that do exist are hard-coded ceilings: SpatialOS max deadline 5 s (V)[7]; Ark 256 event types (V)[12].

5. **One-tick events + test that readers run after senders.**
   - SS14 orders handlers with `before`/`after` and orders systems with `UpdatesAfter`/`UpdatesBefore` (V)[5]. That is declared order, checked by topological sort; ours is registration order + a test. (I)
   - Ark: observer order undefined (V)[11]. Donburi: subscriber order changes on unsubscribe (V)[14].
   - SpatialOS (V)[9]: emulating request/response with events "can easily become messy and prone to programmer errors". Supports keeping requests and events as separate tools.

6. **Delayed-event queue fired at tick start.**
   - No direct analogue found. Closest: SS14 TimedDespawn collects due entities into a set, then raises `TimedDespawnEvent` + `QueueDel` (two-phase) (V)[5]; DoAfter raises its stored `Event` when `time - StartTime >= Delay` (V)[6].
   - Prediction pitfall (V)[5]: TimedDespawn needed `if (!_timing.IsFirstTimePredicted) return;` with comment "Client both needs to predict this, but also can't properly handle prediction resetting." (I) Delayed events must live in snapshotted components so a restore/re-sim fires them exactly once.

7. **Sorted deterministic order.**
   - GDK sorts by `EntityId` only (V)[10]; .NET `List.Sort` is unstable (S), so same-target order is not fixed. (I) Use a total key, e.g. (due tick, target, request kind, source, seq).
   - SS14 and Overwatch get determinism from single-threaded fixed system order, not sorting (I from [1][5]).
   - GoWorld saves timers by ranging a Go map (random order) (V)[16] — a reminder that Go map iteration must never feed the queue. (I)

**Request outcome (gap in the decided design)** (I, from [6][7][9])
- SS14's owner method returns the actual damage dealt; SpatialOS returns a response or a failure code.
- SpatialOS separates "the owner said no" (a normal response with your own failure code) from "the request never ran" (timeout / not found / authority lost) (V)[7][9].
- In one Cardinal shard only two cases matter: target gone (SpatialOS "Not found": "Don't automatically retry" (V)[7]) and owner rejected. Owner can report both with a one-tick event carrying the request's source/cause. No retries needed: the owner never moves.

**How other features modify an in-flight request (gap)** (I, from [6])
- SS14: owner raises `BeforeDamageChangedEvent` (cancellable) and `DamageModifyEvent` (others edit the amount) synchronously, mid-apply (V)[6].
- In our model, one-tick events fire after the fact. So modifiers (armor, shields) must be component leaves the owner reads during its drain. Worth stating in the API doc.

## Critics, failures, pitfalls collected
- Overwatch: deferred creation broke same-frame use; changed after launch in 1.2/1.3 (V)[1].
- Overwatch: rule-breaking legacy code is the main source of bugs and churn (V)[1].
- SpatialOS: short circuiting lets a worker run a command after losing authority (V)[7]; timeout does not tell you if it ran (V)[7].
- SS14: method events banned in the "anti-method event revolt of 2021" (V)[4]; `IsFirstTimePredicted` "misused all across the codebase to hide mispredicts" (V)[4]; engine `Timer` discouraged as non-serializable (V)[5].
- RobustToolbox / Donburi / GoWorld: same-tick drain loops have no cap (V)[5][14][16].
- Donburi: unsubscribe reorders subscribers (V)[14]. Ark: observer order undefined (V)[11].

## Could not find
- Overwatch: which system drains `ModifyHealthQueue`, when in the frame, and in what order; whether queue entries carry source/cause. The talk video/slides were not readable; only the translation and a repost [17].
- SpatialOS: ordering guarantees between commands from different senders; idempotency guidance. Events page was blocked in the prior survey; not retried.
- Any engine with a request depth/chain cap.
- Any engine with a "delayed event fired at tick start" queue as a named mechanism.
- Pitaya (Go) and other Go servers: skipped; they are cross-process RPC, not in-tick queues.
- SS14: a written rationale for owner-only writes beyond the convention text.

## Sources
- [1] Tim Ford GDC 2017, Chinese translation by kevinan (Tencent GAD), Sohu, 2017-06-14. https://www.sohu.com/a/148848770_466876 (read in full; local copy raw/sohu/page.txt)
- [2] SS14 Codebase Conventions. https://docs.spacestation14.com/en/general-development/codebase-info/conventions.html ; source https://github.com/space-wizards/docs/blob/master/src/en/general-development/codebase-info/conventions.md
- [3] RobustToolbox `AccessAttribute.cs`, `AccessAnalyzer.cs` @0fb119a9. https://github.com/space-wizards/RobustToolbox/blob/0fb119a9258ed22a4f0d9bcef9f4521feff989ed/Robust.Shared/Analyzers/AccessAttribute.cs ; .../Robust.Analyzers/AccessAnalyzer.cs
- [4] SS14 docs: ECS and prediction guide. https://github.com/space-wizards/docs/blob/master/src/en/robust-toolbox/ecs.md ; https://github.com/space-wizards/docs/blob/master/src/en/ss14-by-example/prediction-guide.md
- [5] RobustToolbox source @0fb119a9: `EntityEventBus.Broadcast.cs`, `.Directed.cs`, `.Ordering.cs`, `EntityManager.cs`, `EntitySystem.cs`, `Timing/Timer.cs`, `Timing/TimerManager.cs`, `Spawners/SharedTimedDespawnSystem.cs`, `Spawners/TimedDespawnComponent.cs`, `Analyzers/ComponentPauseGeneratorAttributes.cs`. https://github.com/space-wizards/RobustToolbox/tree/0fb119a9258ed22a4f0d9bcef9f4521feff989ed
- [6] SS14 content @75a85474: `Content.Shared/Damage/Components/DamageableComponent.cs`, `Damage/Systems/DamageableSystem.API.cs`, `DoAfter/DoAfterComponent.cs`, `DoAfter/DoAfterArgs.cs`, `DoAfter/SharedDoAfterSystem.Update.cs`. https://github.com/space-wizards/space-station-14/tree/75a8547472043e8d258f0e577563954f19f1b4f7
- [7] SpatialOS docs: Commands (archived 2021-09-28). https://web.archive.org/web/20210928052112/https://documentation.improbable.io/spatialos-overview/docs/commands
- [8] SpatialOS docs: Authority and interest (archived). https://web.archive.org/web/20210928045444/https://documentation.improbable.io/spatialos-overview/docs/authority-and-interest
- [9] SpatialOS docs: Object interaction design guide (archived). https://web.archive.org/web/20210928042813/https://documentation.improbable.io/spatialos-overview/docs/object-interaction
- [10] SpatialOS GDK for Unity (archived repo, master): `Packages/io.improbable.gdk.core/Systems/CommandSystem.cs`, `Commands/CommandDiffStorageBase.cs`, `Assets/Playground/Scripts/Player/ProcessLaunchCommandSystem.cs`. https://github.com/spatialos/gdk-for-unity/tree/master/workers/unity
- [11] Ark docs: Event system (v0.8.3). https://github.com/mlange-42/ark/blob/v0.8.3/docs/content/events/index.md (site: https://ark.mlange.dev/events/)
- [12] Ark source v0.8.3: `ecs/events.go`, `docs/content/events/events_test.go`. https://github.com/mlange-42/ark/blob/v0.8.3/ecs/events.go
- [13] Ark docs: Design (v0.8.3). https://github.com/mlange-42/ark/blob/v0.8.3/docs/content/design/index.md
- [14] Donburi v1.15.8 `features/events/events.go`. https://github.com/yohamta/donburi/blob/v1.15.8/features/events/events.go
- [15] GoWorld README. https://github.com/xiaonanln/goworld
- [16] GoWorld `engine/entity/Entity.go`, `engine/post/post.go` (master). https://github.com/xiaonanln/goworld/blob/master/engine/entity/Entity.go ; https://github.com/xiaonanln/goworld/blob/master/engine/post/post.go
- [17] Repost of [1] on Juejin (secondary; same text). https://juejin.cn/post/6844903635864780813
- [18] GDC Vault session page, 'Overwatch' Gameplay Architecture and Netcode. https://gdcvault.com/play/1024001 (not watched)
