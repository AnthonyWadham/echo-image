# 06 Timers, delayed queues and determinism

Researcher notes, 2026-11-19. Tags: (V) read primary this session, (S) secondary/unread primary, (I) my inference.
Status: DONE 2026-11-19.

## Top findings (read this first)
1. Every engine read stores an absolute due time, not a countdown (Doom 3 `gameLocal.time + time`, Source `curtime + fireDelay` / `TIME_TO_TICKS`, Quantum target frame, Varghese & Lauck "not true if we store the absolute time of expiry"). Integer ticks win for determinism (Source think, Quantum, SC2 `fixed`). (V)
2. Tie order: Doom 3 and Source keep FIFO among equal due times (insert after equals) and save/restore the queue in list order, so restore is exact. Linux wheel buckets are LIFO, Go's heap ties are random on purpose (fake time), container/heap is unstable. So: an explicit tie key is required for a heap or wheel; a stable sort on an append-ordered saved slice gets FIFO for free. (V; I for the conclusion)
3. Target gone: shipped engines either check a generation handle at fire time (Source EHANDLE serial, Doom 3 `idEntityPtr` spawnId, Go timer `seq`) or cancel eagerly on destroy (Doom 3 `~idClass`, Unreal "canceled automatically"). Cardinal EntityID is reused immediately with no generation (system.go). A delayed request keyed by bare EntityID can hit the wrong entity. This is the one real gap in the decided design. (V for facts; I for the gap)
4. Cancel: no game engine read gives per-request handles. They cancel by (source) or (target, kind) scans (Doom 3 CancelEvents, Source CancelEvents/CancelEventOn, logic_relay CancelPending), or by overwriting a one-slot due time (Source/Quake think, Linux mod_timer). Flag-cancel needs a purge bound (Go zombies > 25% → clean; Kafka OOM post-mortem; V&L "grow unboundedly"). (V)
5. Runaway/growth: Doom 3 hard-errors at 4,096 events serviced per frame and a 4,096-entry pool; Netty caps pending timeouts and per-tick transfer (100,000). Save/load determinism needs a test: Factorio "heavy mode" saves and loads every tick and diffs. Pause: tick-based due times pause for free; WC3's real-time sleep is the cautionary tale. (V)

## Sources
- [1] DOOM 3 GPL source, id Software 2011: gamesys/Event.h, Event.cpp, Class.cpp, Game_local.cpp, Game_local.h.
  https://github.com/id-Software/DOOM-3/blob/master/neo/game/gamesys/Event.cpp (and Event.h, Class.cpp, ../Game_local.cpp)

- [2] Source SDK 2013 (Valve): shared/baseentity_shared.cpp, shared/physics_main_shared.cpp, server/baseentity.h, server/cbase.cpp, server/gameinterface.cpp, server/logicrelay.cpp, public/basehandle.h.
  https://github.com/ValveSoftware/source-sdk-2013/tree/master/src/game
- [3] Quake (WinQuake/sv_phys.c) and Quake III Arena (code/game/g_main.c), id Software GPL.
  https://github.com/id-Software/Quake/blob/master/WinQuake/sv_phys.c ; https://github.com/id-Software/Quake-III-Arena/blob/master/code/game/g_main.c
- [4] Linux kernel/time/timer.c (master, fetched 2026-11-19). https://github.com/torvalds/linux/blob/master/kernel/time/timer.c
- [5] Netty HashedWheelTimer.java (4.1 branch). https://github.com/netty/netty/blob/4.1/common/src/main/java/io/netty/util/HashedWheelTimer.java
- [6] Apache Kafka TimingWheel.java, SystemTimer.java (trunk, server-common). https://github.com/apache/kafka/blob/trunk/server-common/src/main/java/org/apache/kafka/server/util/timer/TimingWheel.java
- [7] Yasuhiro Matsuda, "Apache Kafka, Purgatory, and Hierarchical Timing Wheels", Confluent blog, 2015-10-28. https://www.confluent.io/blog/apache-kafka-purgatory-hierarchical-timing-wheels/ (read via WebFetch extraction; quotes from that tool)
- [8] Go runtime src/runtime/time.go and src/container/heap/heap.go (master, fetched 2026-11-19). https://github.com/golang/go/blob/master/src/runtime/time.go ; https://github.com/golang/go/blob/master/src/container/heap/heap.go
- [9] Factorio Lua API: LuaBootstrap (on_nth_tick, on_load) and "Data Lifecycle" page. https://lua-api.factorio.com/latest/classes/LuaBootstrap.html ; https://lua-api.factorio.com/latest/auxiliary/data-lifecycle.html (via WebFetch)
- [10] Rseding, Factorio Friday Facts #270 "Save/Load overview", 2018. https://factorio.com/blog/post/fff-270 (via WebFetch)
- [11] Factorio wiki, "Desynchronization" (heavy mode). https://wiki.factorio.com/Desynchronization (community wiki, via WebFetch)
- [12] Factorio forum threads on on_nth_tick desyncs (Cloudflare blocked; search snippets only): https://forums.factorio.com/viewtopic.php?p=512308 ; https://forums.factorio.com/viewtopic.php?p=277765
- [13] jassdoc (community-annotated WC3 common.j / Blizzard.j), lep/jassdoc. https://github.com/lep/jassdoc/blob/master/common.j ; https://github.com/lep/jassdoc/blob/master/Blizzard.j
- [14] StarCraft II Galaxy natives.galaxy (Blizzard file, mirrored by Talv/sc2-data-trigger). https://github.com/Talv/sc2-data-trigger/blob/master/mods/core.sc2mod/base.sc2data/TriggerLibs/natives.galaxy
- [15] Hive Workshop threads on TSA/PolledWait accuracy and desyncs (403 to fetch; search snippets only). https://www.hiveworkshop.com/threads/polled-waits-vs-timers-for-desyncs.159332/ ; https://www.hiveworkshop.com/threads/wait-vs-polledwait-test.34576
- [16] G. Varghese, T. Lauck, "Hashed and Hierarchical Timing Wheels: Data Structures for the Efficient Implementation of a Timer Facility" (SOSP 1987; journal version IEEE/ACM ToN 1997). Read the author's PostScript (ToN version) via crude text extraction: https://cseweb.ucsd.edu/users/varghese/PAPERS/twheel.ps.Z (SOSP scan: http://www.cs.columbia.edu/~nahum/w6998/papers/sosp87-timing-wheels.pdf, image-only, unread)
- [17] Unreal Engine docs, "Gameplay Timers". https://dev.epicgames.com/documentation/en-us/unreal-engine/gameplay-timers-in-unreal-engine (via WebFetch)
- [18] Photon Quantum 3 manual, "Frame Timer". https://doc.photonengine.com/quantum/v3/concepts-and-patterns/frame-timer (via WebFetch)
- [19] Cardinal source in this repo: pkg/cardinal/system.go (EntityID doc), entity.go, internal/ecs/world_state.go, world.go (`TickHeight() uint64`), pkg/immutable/slice.go (`SortStableFunc`, `DeleteFunc`, `Insert`).

## Doom 3 idEvent queue [1]
API shape (V):
- `void idEvent::Schedule( idClass *object, const idTypeInfo *cls, int time )`; callers use `idClass::PostEventMS( const idEventDef *ev, int time, args... )` / `PostEventSec`. (V)
- `static void CancelEvents( const idClass *obj, const idEventDef *evdef = NULL )` — cancel by (target, event kind), not by handle. (V)
- `static void ServiceEvents( void )`, `Save( idSaveGame* )`, `Restore( idRestoreGame* )`. (V)
- Event record: `eventdef`, `data` (packed args), `int time`, `idClass *object`, `typeinfo`, intrusive list node. (V)
Storage and order (V):
- One global sorted intrusive linked list `EventQueue`, fixed pool `static idEvent EventPool[ MAX_EVENTS ]` (MAX_EVENTS 4096). (V)
- Insert is a linear walk: `while( ( event != NULL ) && ( this->time >= event->time ) )` then InsertBefore. `>=` means equal times keep FIFO post order. Stable, but O(n) insert. (V)
- Due time is absolute game ms: `this->time = gameLocal.time + time;` with comment "wraps after 24 days...like I care. ;)". (V)
Drain (V):
- Loop: take head; `if ( event->time > gameLocal.time ) break;` so the drain includes events posted during the drain if due now (time 0 posts fire the same frame). (V)
- Removed from list before the call "so that if then object is deleted, the event won't be freed twice". (V)
- Runaway cap: `if ( num > MAX_EVENTSPERFRAME ) gameLocal.Error( "Event overflow.  Possible infinite loop in script." );` MAX_EVENTSPERFRAME = 4096. Hard error, not a defer. (V)
- Pool exhaustion: `gameLocal.Error( "idEvent::Alloc : No more free events" )`. Bounded growth by a hard fixed pool. (V)
- Service point: in idGameLocal::RunFrame, AFTER `time += msec`, after all `ent->Think()` and deactivation, `// service any pending events  idEvent::ServiceEvents();`. Not at frame start. (V)
Target gone (V):
- `idClass::~idClass() { idEvent::CancelEvents( this ); }` "Cancels any events that depend on this object." Target death cancels its events eagerly by an O(n) list scan. (V)
- Entity args are stored as `idEntityPtr<idEntity>` and resolved at service time via `GetEntity()`. Game_local.h: `if ( ( gameLocal.spawnIds[ entityNum ] == ( spawnId >> GENTITYNUM_BITS ) ) ) return ...; return NULL;` = slot index + generation in one int. A dead or replaced argument resolves to NULL; `D_EVENT_ENTITY_NULL` marks events that accept NULL. (V)
Save/restore (V):
- Save writes count, then per event in queue order: time, `eventdef->GetName()`, `typeinfo->classname`, object ref, arg size, typed args. Restore `AddToEnd` in saved order, so the tie order survives a load. Looks up event and class by NAME and errors on unknown name or arg-size mismatch. (V)
- Pausing: `g_stopTime` branch skips `time += msec` and skips ServiceEvents; queue is keyed to game time, so pause = do not advance the clock. (V)
- Client filter: PostEventArgs returns true without scheduling on MP clients (outside startup, non-thread) "to avoid any bad code filling up the event pool". (V)
- Ignored if class has no handler: `if ( !c->eventMap[ ev->GetEventNum() ] ) return false;` (V)
Lessons (I):
- Doom 3 ties order by insert order (FIFO), which is deterministic only if insert order is. Save keeps list order, so restore is exact. A Cardinal snapshot of a cardinal.Slice keeps order too, but if the queue is rebuilt from many components we need an explicit seq key.
- Eager cancel-on-destroy is O(queue) per destroy; fine at 4096 cap.

## Source engine: think slots and the entity I/O queue [2]
Two different mechanisms. (V)
A) Think functions = one due-tick slot per (entity, context), not a queue. (V)
- `struct thinkfunc_t { BASEPTR m_pfnThink; string_t m_iszContext; int m_nNextThinkTick; int m_nLastThinkTick; }`; entity keeps `CUtlVector< thinkfunc_t > m_aThinkFunctions` plus a base `m_nNextThinkTick`. (V)
- API: `SetNextThink( float nextThinkTime, const char *szContext = NULL )`, `ThinkSet( BASEPTR func, float flNextThinkTime = 0, const char *szContext = NULL )`, `RegisterThinkContext( const char *szContext )`, `GetNextThinkTick(...)`. (V)
- Time given in seconds but stored as ticks: `TIME_TO_TICKS( thinkTime )`. Saved as `FIELD_TICK`. (V)
- Cancel = overwrite: `SetNextThink( TICK_NEVER_THINK )`. Re-scheduling replaces the old due tick. No handles, no growth: max one pending per slot. (V)
- Run: `PhysicsRunSpecificThink`: `if ( thinktick <= 0 || thinktick > gpGlobals->tickcount ) return true;` then clears the slot (`SetNextThink( nContextIndex, TICK_NEVER_THINK )`) BEFORE dispatch, so the think must re-arm itself. Returns `!IsMarkedForDeletion()` and the loop stops for a dead entity. (V)
- Late thinks are clamped: "Don't let things stay in the past." `if ( thinktime < gpGlobals->curtime ) thinktime = gpGlobals->curtime;` (V)
- Order: base think first, then contexts in registration order (`for i < m_aThinkFunctions.Count()`). (V)
- Cost model: polled, every thinking entity checked each tick (EFL_NO_THINK_FUNCTION skips entities with nothing armed via `CheckHasThinkFunction`). (V)
- Debug tool `sv_thinktimecheck` "Check for thinktimes all on same timestamp" reports entities thinking on the same tick (load spikes). (V)
B) Entity I/O event queue `CEventQueue` (map logic outputs with delays). (V)
- Record `EventQueuePrioritizedEvent_t`: `m_flFireTime` (float seconds), `m_iTarget` (name string), `m_iTargetInput`, `m_pActivator`/`m_pCaller`/`m_pEntTarget` (EHANDLE), `m_iOutputID`, `m_VariantValue`. Source and cause travel with the request (activator = cause, caller = source). (V)
- `AddEvent(target, input, value, float fireDelay, activator, caller, outputID)`: `m_flFireTime = gpGlobals->curtime + fireDelay; // priority key in the priority queue`. (V)
- Insert: linear walk, breaks on `pe->m_pNext->m_flFireTime > newEvent->m_flFireTime` so ties go after existing equal-time events (FIFO). (V)
- Drain: `while ( pe != NULL && pe->m_flFireTime <= gpGlobals->curtime )`; comment "remembering that the queue may have been added to" → zero-delay events added during the drain fire in the same pass. No count cap here (contrast Doom 3). (V)
- Target gone: target resolved at fire time by name (`gEntList.FindEntityByName`), else by EHANDLE, else by classname; if nothing found → `DevMsg "unhandled input: ... target entity not found"`, event dropped silently. (V)
- EHANDLE = index + serial number: "The high NUM_SERIAL_NUM_BITS bits are the serial number." A stale handle resolves to NULL. Generation-ID style. (V)
- Cancellation: no per-event handle. `CancelEvents( CBaseEntity *pCaller )` (by source), `CancelEventOn( pTarget, sInputName )` (by target+input), `HasEventPending(...)`. All O(n) list scans. logic_relay exposes input `CancelPending`: "Cancels any I/O events in the queue that were fired by us." (V)
- Entity removal does NOT cancel queued I/O; reliance is on late binding + stale handles. (V for absence in the SDK files I grepped: only callers are the ent_cancelpendingentfires command and logic inputs; (I) that this is the general rule.)
- Service point: GameFrame runs `Physics_RunThinkFunctions( simulating )`, `FrameUpdatePostEntityThinkAllSystems()`, then `ServiceEventQueue()`, then `gEntList.CleanupDeleteList()`. I/O fires after thinks, before deferred deletes. (V)
- Save/restore: Save writes count + each event in list order (`m_flFireTime` as `FIELD_TIME`). Restore calls `AddEvent( ..., tmpEvent.m_flFireTime - gpGlobals->curtime, ...)`, i.e. converts back to a delay and re-inserts in saved order. Ties stay FIFO. (V) Pitfall: float round trip `(t - now) + now` may not equal t exactly (I).
Lessons (I):
- Source keeps two tools: a per-slot due tick (bounded, overwrite = cancel) for "wake me later", and a queue for one-shot delayed messages. Matches our split: a component field `NextTick` vs a delayed queue.
- Integer ticks (think) are safer than float seconds (I/O queue) for determinism.

## Quake 1 / Quake 3 think [3]
- One `nextthink` field per entity; the think is a function pointer field. No queue. (V)
- Quake 1 `SV_RunThink`: `if (thinktime <= 0 || thinktime > sv.time + host_frametime) return true;` clamps past times ("don't let things stay in the past."), sets `ent->v.nextthink = 0;` before `PR_ExecuteProgram (ent->v.think)`, returns `!ent->free`. Float seconds. (V)
- Quake 3 `G_RunThink`: `if (thinktime <= 0) return; if (thinktime > level.time) return; ent->nextthink = 0; ... ent->think (ent);` levelTime is int ms. (V)
- Order = entity array index order in G_RunFrame (S: standard loop, I saw the function header only).
- Lessons (I): the "one due time per entity slot, clear before calling, re-arm inside" pattern is 25+ years old and has no growth, no cancel handle and no restore ordering problem, because order comes from the entity array. Same cost problem: polls every entity every tick.

## Linux kernel timer wheel [4]
- Non-cascading hierarchical wheel since the 2016 rework: "Contrary to the original timer wheel implementation, which aims for 'exact' expiry of the timers, this implementation removes the need for recascading". (V)
- Why: "The vast majority of timeout timers (networking, disk I/O ...) are canceled before expiry." Far timers lose precision: "The farther the expiry time is away the higher the array level and therefore the granularity becomes." (V)
- Shape: LVL_SIZE = 64 buckets per level (`LVL_BITS 6`), each level 8x coarser (`LVL_CLK_SHIFT 3`), 8-9 levels. At HZ 1000: level 0 = 1 ms gran up to 63 ms; level 1 = 8 ms up to 511 ms; ... (V)
- Overflow: "timers with a expiry time above the capacity of the last wheel level are force expired at the maximum timeout value of the last wheel level" (max ~12 days at HZ 1000). (V)
- Bucket insert is `hlist_add_head(&timer->entry, base->vectors + idx);` and expiry pops from head, so timers in one bucket run in reverse insert order, not FIFO; no ordering promise among timers due the same jiffy. (V for the code; (I) for the conclusion.)
- API: timer is embedded in its owner (`struct timer_list`), one pending expiry per timer. `mod_timer(timer, expires)` "is equivalent to: timer_delete(timer); timer->expires = expires; add_timer(timer);". `timer_delete` "only deactivates a pending timer ... It neither prevents rearming". `timer_shutdown` "Deactivate a timer and prevent rearming ... Any attempt to arm @timer after this function returns will be silently ignored." (V)
- Lessons (I): (a) re-arm = overwrite the owner's slot, like Source think; (b) teardown needs a "no re-arm" state, because a callback can re-arm after cancel — the ECS analogue is a request/event for a destroyed entity re-queuing itself; (c) wheel buckets give no tie order, so a deterministic game must sort within the bucket.

## Netty HashedWheelTimer [5]
- "A Timer optimized for approximated I/O timeout scheduling." "this timer does not execute the scheduled TimerTask on time" - checks each tick for tasks "behind the schedule". Default tick 100 ms, 512 buckets. Cites Varghese & Lauck. (V)
- API: `Timeout newTimeout(TimerTask task, long delay, TimeUnit unit)`; `Timeout.cancel()`; `long pendingTimeouts()`. (V)
- Single-level hashed wheel with "rounds": `timeout.remainingRounds = (calculated - tick) / wheel.length;` and `final long ticks = Math.max(calculated, tick); // Ensure we don't schedule for past.` Bucket expiry walks every entry and decrements `remainingRounds` for not-yet-due ones, so far-future timers cost on every lap. (V)
- Cancel = flag + lazy removal: `cancel()` CASes `ST_INIT -> ST_CANCELLED` and pushes onto `cancelledTimeouts`, "it will be removed from HashedWheelBucket on next tick". (V)
- Bounds: optional `maxPendingTimeouts`; over it `newTimeout` throws `RejectedExecutionException("Number of pending timeouts (...) is greater than or equal to maximum allowed pending timeouts ...")`. Per-tick transfer cap: "transfer only max. 100000 timeouts per tick to prevent a thread to stale the workerThread". (V)
- Doc warning: "Do not create many instances." "One of the common mistakes ... is to create a new instance for every connection." (V)
- Lessons (I): rounds-in-bucket is cheap insert but drain cost grows with far timers; caps on pending count and per-tick work are the standard guard.

## Kafka purgatory / hierarchical timing wheel [6][7]
- Class doc: "A timing wheel has O(1) cost for insert/delete (start-timer/stop-timer) whereas priority queue based timers, such as java.util.concurrent.DelayQueue and java.util.Timer, have O(log n) insert/delete cost." (V)
- Overflow wheels: "If the resolution of a wheel at one level is u and the size is n, the resolution of the next level should be n * u." "When a bucket in an overflow bucket expires, all tasks in it are reinserted into the timer recursively." Insert O(m) wheels, delete O(1). (V)
- "The hierarchical timing wheels works especially well when operations are completed before they time out." (V)
- Hybrid: buckets (not tasks) go into a `DelayQueue<TimerTaskList>`; `advanceClock` polls the next non-empty bucket, so empty ticks cost nothing. Blog: "we enqueue task buckets instead of individual tasks" and "The number of items in DelayQueue is capped by the number of buckets". (V)
- Old design failure (post-mortem style): "When a request is completed, the request is not deleted from the timer or watcher lists immediately" ... "the server may exhaust JVM heap and cause OutOfMemoryError"; a "reaper thread" purged completed requests and "the server must pay a significant performance penalty if it scans all lists too frequently." (V via [7])
- Fix: each task keeps its own link cell; "a completed request is removed from the timer queue immediately with O(1) cost." Benchmark: old saturated ~25,000 RPS, new ~105,000 RPS (micro-benchmark). (V via [7])
- Lessons (I): lazy cancel (flags) is fine only if the dead entries are purged on a bound; Kafka's OOM is the cautionary tale for "cancel by flag, sweep later" when most timers are cancelled.

## Go runtime timers and container/heap [8]
- Per-P 4-ary min-heap: `const timerHeapN = 4`; "Each P has a heap of pointers to timers that it manages." (V)
- Tie-break is deliberately random, not FIFO: in `timerWhen.less`, "When timers wake at the same time, use a per-timer random value to order them. We only set the random value for timers using fake time" (synctest). Go treats same-instant order as unspecified and shakes it in tests to expose order bugs. (V)
- Lazy delete with a bound: stopped timers stay as `timerZombie` ("set when the timer has been stopped but is still present in some P's heap"); cleanup forced when `int(zombies) > int(ts.len.Load())/4`; `cleanHead` "speeds up programs that create and delete timers; leaving them in the heap slows down heap operations." (V)
- Stale-fire guard = generation counter: `seq` "is a sequence number to recognize and squelch stale function invocations" and for channel timers "to recognize and squelch sends that correspond to an earlier (stale) timer configuration". (V)
- Re-arm is an in-place modify (`reset(when, period)` → `modify`), with `timerModified` deferring the heap fix until the earliest modified timer could fire ("This speeds up programs that adjust a lot of timers back and forth if the timers rarely expire"). (V)
- container/heap: `Push`, `Pop`, `Remove(h, i)`, `Fix(h, i)` all "O(log n)". Doc says nothing about stability; a binary heap is not stable (I), so equal keys pop in arbitrary order unless Less includes a tie-break (seq). (V for API/complexity; I for stability.)
- Lessons (I): (1) if you use a heap, Less must compare the full key (dueTick, seq/source). (2) Go's random tie-break in tests is a good idea to copy: a test mode that shuffles same-key order proves no reader depends on it. (3) Zombie-ratio compaction (>25% dead → rebuild) is a cheap bound for flag-cancel designs.

## Factorio: on_nth_tick, on_load and save determinism [9]-[12]
- API: `script.on_nth_tick(tick, handler)`; tick may be one value or an array; "When the game is on tick 0 it will trigger all registered handlers." Passing `nil` handler "will unregister it for the provided nth-tick(s)". (V [9])
- Handlers are code, not state. on_load: "Doing anything other than the following three will lead to desyncs, breaking multiplayer and replay functionality." Allowed: re-set metatables, "Re-setup conditional event handlers", local refs to `storage`. `storage` "is safe to read from, but not write to". (V [9])
- The documented consequence: a conditional/one-shot timer registration must be mirrored in saved `storage`, and re-registered in on_load from that saved state; otherwise a joining client or a reloaded save lacks the handler and desyncs. (S [12] search snippets; consistent with (V) on_load rules in [9])
- Save/load is a hard determinism requirement: "saving, exiting, and loading the save shouldn't change any observable behavior." Reasons: MP join, replays ("the replay system wouldn't work if you ever saved, exited, and resumed playing"), testing. (V [10])
- Tooling: "heavy mode ... make Factorio save and load the game every tick" and "highlight if there are any changes in the game state between the save and subsequent load"; dumps exclude mods' global table. (V [11], community wiki)
- Lessons (I):
  - The schedule must be data in the snapshot (Cardinal: components), never "which systems are registered" or closures. on_nth_tick is safe only because its period is static code; a one-shot delay is not.
  - Copy heavy mode: a test that snapshots + restores every tick and compares outputs. It is the direct test for rule 7.

## Warcraft III and StarCraft II trigger waits [13]-[15]
WC3 (V [13], community-tested notes, not Blizzard docs):
- `native TriggerSleepAction takes real timeout returns nothing`: "Makes a trigger execution sleep for a given duration. The thread will yield". A coroutine sleep, not a queued request. (V)
- Wrong clock: "This ticks while the game was paused with the `PauseGame` native, unlike timers." and "This ticks independently from game speed" (Normal speed: a 30 s TSA = 24 s of game time). (V, community-measured)
- Only valid in trigger action context: "If this is called in the wrong context, it crashes the thread." (V)
- Cancel hazard: `TriggerRemoveAction` "@bug ... the removed action was still pending to be called, it will still be called unless there is a `TriggerSleepAction` after". Removal did not cancel already-scheduled work. (V)
- `PolledWait` (Blizzard.j) = a game-time `timer` polled by repeated TSA: sleeps `0.1 * timeRemaining` while above `bj_POLLED_WAIT_SKIP_THRESHOLD` (2.0 s), then `bj_POLLED_WAIT_INTERVAL` (0.10 s) steps. jassdoc flags it as a leak bug ("Leaks handle t"). Accuracy limited by the poll step. (V)
- Community: short TSAs fire late (e.g. "TSA 0.01 seconds ... every ~0.27 seconds"); TSA in player-local branches desyncs; use timers for game logic. (S [15])
SC2 (V [14], Blizzard header comments):
- Three clocks, chosen per call: `const int c_timeGame = 0; c_timeReal = 1; c_timeAI = 2;` `native void Wait (fixed inSecs, int inTimeType);` `native void TimerStart (timer t, fixed duration, bool periodic, int timeType);` `TimerPause (timer t, bool pause)`. (V)
- "Game Time - Takes into account the current game speed". "Real Time ... is still synchronous, and will therefore be affected by lag". "AI Time - Passes at the same rate as game time, but may be paused/unpaused independently of game time via triggers ... to pause the AI while an in-game cinematic sequence is run." (V)
- `fixed` is fixed-point, so durations are deterministic across machines. (V for type; I for intent)
Lessons (I):
- Blizzard fixed WC3's ambiguous waits by making the clock explicit per timer. For us: one clock only (the sim tick). Pause = the tick does not advance, so tick-based due times pause for free.
- Per-group pause (SC2 AI time) needs a separate clock. With due-tick requests, a paused owner can simply not drain; requests keep their absolute due tick and all fire late on resume. If "pause shifts deadlines" is wanted, the delay must be stored as remaining ticks and decremented only while unpaused (I).
- Coroutine waits (TSA, Galaxy Wait) hold a suspended stack; that cannot live in a component snapshot. Our design (data requests) avoids this by construction.

## Varghese & Lauck timing wheels [16]
(V; quotes from crude PS text extraction, spaces restored by me)
- Model, four routines: "StartTimer(Interval, RequestId, ExpiryAction)", "StopTimer(RequestId): This routine uses its knowledge of the client and RequestId to locate the timer and stop it", "PerTickBookkeeping", "ExpiryProcessing". Cancel is by the client's own request id. (V)
- Absolute vs relative: "the elements to be 'sorted' change their value over time if we store the interval. This is not true if we store the absolute time of expiry." (V) → store due tick, not remaining delay.
- Flag-cancel warning: "Simulation Languages assume that canceling event notices is very rare. If this is so, it is sufficient to mark the notice as 'Canceled' and wait until the event is scheduled ... In a timer module, StopTimer may be called frequently; such an approach can cause the memory needs to grow unboundedly beyond the number of timers outstanding". (V)
- Scheme 4 (basic wheel): if all timers < MaxInterval, O(1) Start, Stop and PerTick; bucket (i+j) mod MaxInterval holds timers due at CurrentTime+j; timers go "at the head of a list". (V) For a 20 Hz game with short delays this is the "per-tick bucket" design. (I)
- Scheme 5 (hash, sorted lists per bucket) vs Scheme 6 (unsorted lists, O(1) start, per-tick decrements "the high order bits for every element"). "if each list is doubly linked and StartTimer stores a pointer to each timer element, StopTimer takes O(1) time." (V)
- Scheme 7 (hierarchy, hour/minute/second arrays): StartTimer O(m) levels. The paper already notes the Linux 2016 idea: "Wick Nichols has pointed out that if the timer precision is allowed to decrease with increasing levels in the hierarchy, then we need not migrate timers between levels." (V)
- Origin: "extended the timing wheel mechanism used in logic simulation" — wheels come from discrete-event simulation, the same domain as a fixed-tick game. (V)

## Unreal Engine FTimerManager [17]
- `SetTimer` / `SetTimerForNextTick` fill an `FTimerHandle`; `ClearTimer`: "The timer handle will become invalid at this point, and can be reused". `PauseTimer` / `UnPauseTimer` per handle. (V via WebFetch)
- Target gone: "Timers will be canceled automatically if the Object that they are going to be called on" is destroyed; "the timer handle will become invalid and the function will not be called." (V)
- "Calling SetTimer with a rate less than or equal to zero is identical to calling ClearTimer." `SetTimerForNextTick` "does not fill out a timer handle". (V)
- FTimerHandle internally packs an index plus a serial number (generation ID). (S: from memory of TimerManager.h; engine source not fetched, needs Epic GitHub auth)

## Photon Quantum FrameTimer [18]
- Deterministic rollback engine stores timers as target frame: "The FrameTimer keeps a memory of the target frame", "Stored as 2 x int". `FrameTimer.FromFrames(frame, 60)`, `FrameTimer.FromSeconds(frame, 2)` ("FromSeconds rounds up to whole frames"), `IsRunning(frame)`, `HasStoppedThisFrame(frame)`, `RemainingFrames`. (V via WebFetch)
- Lesson (I): the deterministic-netcode engine settled on integer frame numbers; seconds are converted once, rounding up. Same as our dueTick.

## Synthesis for the Cardinal API (all (I) unless marked)
### Storage choice at 20 Hz
- Delays in a game are short (wind-ups, staggers: 1-100 ticks). A sorted cardinal.Slice per owner queue, drained from the front, is enough. Wheels/heaps pay off only with thousands of long timers mostly cancelled (Linux/Kafka/Netty's case: network timeouts). Not our load.
- If one owner ever holds >~10k pending delayed items, the upgrade path is Varghese Scheme 4 (per-tick buckets, MaxInterval) with an overflow list, like Kafka's bucket-level DelayQueue. Measure first (memory: measure before perf refactors).
- Keep requests sorted by key at insert (binary search + `Insert(i, x)`) or append then `SortStableFunc` once per drain. Append + stable sort is simplest and FIFO-preserving. Note `Insert(Len, x)` appends in place (memory); mid-slice Insert copies.
### Request record (fields seen across engines)
- `Due uint64` (absolute tick; 0 = asap) — Doom 3 / Source / Quantum / V&L.
- `Seq uint64` — per-queue counter, saved in the queue component (not a Go global), so it survives a snapshot. Unique, never reused → doubles as the cancel handle; no generation needed for the request itself.
- `Source`, `Cause` (Source I/O: caller + activator), `Depth`, `Flags`.
- `Target EntityID` + `TargetGen uint32` (see gap below).
- Sort key for drain: `(Due, Seq)`. Optional `(Due, Source, Seq)` adds nothing if Seq is per-queue and producers run in registration order; keep it to two fields.
### The gap: entity ID reuse (evidence-backed flaw)
- Cardinal: "EntityID ... is reused as soon as the entity is destroyed, so an ID kept past that point refers to whichever entity is created next in its place." Free list is a min-heap; "newEntity always reuses the smallest id". (V [19])
- With rule 3 (due ticks, staggered hits), a request outlives the tick it was made in, so its target may be destroyed and the ID re-issued before the drain. Source/Doom 3 solved this with index+serial handles; Unreal with auto-cancel. (V [1][2][17])
- One fix, one path: each targetable entity gets a spawn serial (uint32 from a saved singleton counter) in a component; requests carry it; the owner drops a request whose `(Target, TargetGen)` no longer matches. Same rule as Doom 3 `idEntityPtr::GetEntity`. Eager purge on destroy (Doom 3 style) needs every destroyer to know every queue — breaks rule 1 ownership. Check at drain instead.
- Dropped requests: count them in a debug counter (Source logs "unhandled input ... target entity not found" at DevMsg 2).
### Cancellation
- Prefer "overwrite one slot" where the game has at most one pending thing per (entity, purpose): a `NextTick` field in the owner's own component (Source think contexts, Quake nextthink, Linux mod_timer). Zero growth, no handle, restore-safe.
- For queued requests: cancel by predicate (`DeleteFunc` on source/kind/target) at the owner, like Doom 3 CancelEvents and Source CancelEventOn. Cancel is itself a request to the owner (rule 2), applied before the drain of due items, or it races.
- Avoid flag-cancel unless bounded (Go: purge when zombies > len/4).
- Linux `timer_shutdown` lesson: a handler can re-arm after cancel. A destroyed target must not accept new requests either: the generation check covers this.
### Delayed events (rule 6)
- Firing at tick start differs from Doom 3 / Source, which service after thinks. Start-of-tick is fine and simpler for the reader-after-sender test: a due-at-T event is visible to every T system. Zero-delay "later" = next tick.
- Doom 3 and Source let due-now items posted during the drain fire in the same pass. Do not copy: freeze the due set at drain start (items with Due <= now at entry); new ones wait. Bounds the loop without a 4,096 crash.
### Pause
- Use only the sim tick. Pause = no tick → nothing fires (Doom 3 g_stopTime skips ServiceEvents). Per-feature pause (SC2 AI time) = the owner skips its drain; due items fire late on resume. If shifting deadlines is wanted, store remaining ticks for that feature only. Never wall-clock (WC3 TSA).
### Bounds
- Per-queue length cap and per-drain work cap with a logged error, not a panic in production (Doom 3 Error, Netty RejectedExecutionException). Depth cap from rule 4 covers chains; length cap covers fan-out.
- Max delay cap (Linux force-expires beyond wheel range; Netty clamps past deadlines to now; Source/Quake clamp past think times to now). Due < now → treat as asap.
### Tests to copy
- Factorio heavy mode: snapshot+restore every tick in a test world, compare digests. Directly proves rule 7.
- Go fake-time shuffle: a test build that shuffles equal-key order to prove no one depends on it (only useful if the key is not total; with (Due, Seq) it is total).
- Source `sv_thinktimecheck`: report how many items fire on the same tick to spot load spikes.

## Could not find / not verified
- SOSP '87 scan is image-only; read the 1997 journal PostScript text instead (same schemes). Exact per-scheme cost table not extracted.
- Factorio forum threads (Cloudflare 403): on_nth_tick desync anecdotes are (S). No official doc says "handlers are not saved" in those words; the on_load rules imply it.
- Hive Workshop threads (403): WC3 desync/accuracy claims are (S). No Blizzard-authored WC3 doc; jassdoc is community-tested.
- Unreal TimerManager.h source (needs Epic GitHub access): handle serial-number layout is (S).
- No primary source found for StarCraft II Wait() implementation (coroutine scheduling order, save behavior).
- Did not read Linux `__mod_timer` slack/rounding code beyond the header comment; LWN "Reinventing the timer wheel" (2015) not read.
- No shipped game engine found with per-request cancel handles in its delayed event queue (Doom 3, Source both cancel by scan).
