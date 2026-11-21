# Owner queues: intents go to the feature that owns the state

Built 2026-11-20 on echo_image `46bd18e`. It follows the 2026-11-19 research:
[`event-architecture.md`](event-architecture.md) (with its [survey](gameplay-coupling-survey.md)) and
[`queue-api.md`](queue-api.md).

> **These are findings, not the source of truth.** Code and [`CLAUDE.md`](../../CLAUDE.md) win when
> they disagree with this file.

## Summary

- **What changed.** Damage, heal, effect and launch intents are no longer system events. Each one
  goes to a queue that its owner holds, and the owner drains it once a tick. The library is
  `pkg/queue`.
- **Why.** Each intent has one reader: its owner. A system event goes to every reader, is cleared
  at the end of the tick, and is lost if it is sent after its reader ran. A queue is state. A
  snapshot keeps it, and a late send waits one tick.
- **Delayed events** (fire a system event on a later tick) are built and tested, with no user yet.
- **No loop guard.** A depth cap on every intent was built and taken out. Echo has no loop, and a cap
  is the wrong guard for loops that branch. See section 3.
- **Another game can copy it.** Steps are in section 5.

## 1. How it works

- The owner calls `queue.Declare[T](w)` in its `RegisterTypes`. That registers the queue's
  component and an Init system that makes the one entity holding it.
- Any system sends with `queue.Send(w, T{…})`, or `queue.SendAt(w, tick, T{…})` for a later tick.
- The owner's one system calls `queue.Drain[T](w, compare)` once a tick. It gets everything due,
  sorted by the tick each was due, then by the owner's `compare`.
- Send order never counts, because it can change after a restore.
- A second drain of one queue in a tick panics.
- In echo:

  | Intent | Owner (drains) | Senders |
  | --- | --- | --- |
  | `LaunchIntent` | `projectile.LaunchSystem` | `ability.CastHookSystem` |
  | `DamageIntent` | `combat.DamageResolveSystem` | `projectile.FlightSystem`, `area.AreaSystem` |
  | `HealIntent` | `combat.DamageResolveSystem` | none yet |
  | `EffectIntent` | `effect.ResolveSystem` | `ability.CastHookSystem`, `echo.EliminationSystem` |

- `Died` stays a system event: many systems react to a death.
- Delayed events: `queue.DeclareDelayed[E](w, compare)` registers E, its queue, and a PreUpdate
  system that fires E at the start of its tick. `queue.Schedule` returns a ticket, and
  `queue.Cancel` or `queue.CancelFunc` stop waiting events.

## 2. Choices made while building

- **Order is by content:** due tick, then the owner's compare. There is no send counter, because
  send order can change after a restore.
- **No shared header.** There is no generic Target, Source or Cause field. Each intent keeps its own
  fields (`Credit`, `Cause`).
- **`Drain` returns a plain slice.**
- **No ID versions on requests yet.** Every request in echo is sent and drained in the same tick, and
  the order test keeps it so. Only shots are destroyed before PostUpdate, and no request names a
  shot. A request that waits (`SendAt`, or a send after the drain) needs `ref.Spawn` serials when it
  gets its first user.
- **A past tick is not moved up.** A request or event for a tick that has passed lands at the next
  drain or fire, before that tick's own.
- **No length cap or schedule limit yet.** Nothing needs one.
- **Lost check:** no world test reads `Credit` or `Cause` on damage now, since nothing acts on them.
  Before, tests read the intent itself.

## 3. Loops of triggers: left out

- **What was built and removed:** a `Chain` on every intent and event. Each step a trigger caused was
  one deeper, and a step past 16 was dropped.
- **Echo has no loop.** A death triggers "out", and "out" triggers nothing.
- **Rampage has real loops, and four hand-made guards** (checked on origin/main, 2026-11-20):

  | Where | Guard |
  | --- | --- |
  | `abilities/systems/damage_processing.go` | An on-crit effect loses its own on-crit list |
  | `abilities/systems/arc_request_system.go` | On-kill and on-crit lightning runs on a copy that can't crit |
  | `abilities/systems/spawn_projectile_hook_system.go` | Split children drop on-destroy, if the ability's JSON sets `inheritDestroyHooks: false` |
  | `abilities/components/aoe.go` | AoEs spawned by a chain skip their expire hooks |

- **Why a depth cap doesn't fit them:**
  - The guards are design rules. "An on-crit strike can't crit again" stops at step 1, not 16.
  - Rampage's loops branch. With 2 children per step, step 16 alone has 65,536 copies.
- **What games do** (from memory, not checked): a rule per mechanic, carried on the child. Dota flags
  reflected damage so it can't be reflected again. Risk of Rain 2 records which items already fired in
  a chain.
- **Later:** one shared "already fired" flag set, designed from rampage's four guards
  (`docs/backlog.md`).
- **A risk in rampage today:** the split guard is opt-in. A new split ability without
  `inheritDestroyHooks: false` recurses exponentially.

## 4. What World Engine lacks

| Need | Stand-in in echo | What World Engine could add |
| --- | --- | --- |
| IDs with versions | `ref.Spawn` serials | a version in each entity ID |
| A startup check of system order | `TestSendersRunBeforeTheirReceivers` reads each system's file | systems declare what they send, drain, emit and read, and `StartGame` checks the order |
| A phase before PreUpdate | the fire system runs in PreUpdate, where `DeclareDelayed` is called | a hook before PreUpdate for timers |

- **World Engine main's `Slice` API changed (#1177).** Its methods now change the slice in place
  instead of returning a new one. So three calls in `pkg/queue` change when echo moves to main:
  `Insert` changes the slice itself, `Filter` becomes `DeleteFunc`, and `Without` becomes `Delete`.
  Cost stays low: on main, `Insert` at the end is an `Append`, which copies at most one 32-item
  chunk.

## 5. Copying it to another game

1. Copy `pkg/queue`, until World Engine has it.
2. For each intent, in the owner's `RegisterTypes`, replace `RegisterSystemEvent[T]` with
   `queue.Declare[T]`.
3. Senders: `w.EmitSystemEvent(T{…})` becomes `queue.Send(w, T{…})`.
4. The owner: reading `w.SystemEvents[T]()` and sorting becomes `queue.Drain[T](w, compare)`.
5. Tests that read an intent with `Emitted` check what the owner does instead.
6. Add the order test (`TestSendersRunBeforeTheirReceivers` and `sourceUses`).
7. Old snapshots have no queue entity: purge the world.

## 6. Keeping the pattern: lint or a skill

Asked 2026-11-20: can lint rules stop features drifting back to events or hand-made queues, as in
rampage? Short answer: lint for the mechanical rules, a skill for the judgment calls. A skill only guides.

- **Already blocked without lint:** sending to a queue nobody declared, and a second drain in a tick.
  Both panic.
- **A lint rule can catch:**
  - a package that drains or declares a queue it doesn't own;
  - an intent sent or read as a system event;
  - a drain that doesn't name its type.
- **Lint can't catch:**
  - System order (senders before the drain). It's only known at runtime. The order test checks it now;
    World Engine should check it at startup.
  - A new hand-made queue. It looks like any other list in a component, so it needs a skill rule and
    review.
  - "Should this be a queue at all?" That's a design choice.
- **Checked 2026-11-20:** World Engine and rampage both run gocritic, forbidigo and depguard. Neither has a
  custom analyzer. golangci-lint 2.15 can build in your own rules (`golangci-lint custom`).
- **Engines ship such checks** (from memory, not checked): Unity's Entities package has analyzers, and
  Bevy has `bevy_lint`. Space Station 14 enforces owner-only writes with an analyzer ([the
  survey](gameplay-coupling-survey.md)).

| Option | Good | Bad |
| --- | --- | --- |
| forbidigo name patterns | Already on, nothing new to install | Matches text only, so renames break it and it flags the wrong things |
| gocritic's `ruleguard` | Built into golangci-lint, knows types | Probably can't check "only the owner drains" |
| A small analyzer inside `pkg/queue` | Checks every rule exactly, and moves to World Engine with the package | One more tool to keep matched to the Go version |
| A test that reads the source (echo has one) | No lint tools, already runs in CI | Each repo needs its own copy, and nothing shows in the editor |

- **The pick:** the analyzer inside `pkg/queue`, run with `go vet -vettool`. When the package moves to
  World Engine, every game gets the same rules.
- **A start exists:** the queue research built a prototype owner-write analyzer
  ([`queue-api-notes/ownercheck.md`](queue-api-notes/ownercheck.md)).
- **Not written yet:** the rule spec.

## Appendix: files

- Library: `pkg/queue/doc.go`, `queue.go`, `delayed_event.go`, and their tests.
- Intents: `combat/component/damage_intent.go`, `heal_intent.go`;
  `effect/component/effect_intent.go`; `projectile/component/launch_intent.go`.
- Drains: `combat/damage_system.go`, `effect/resolve_system.go`, `projectile/launch_system.go`.
- Senders: `projectile/flight_system.go`, `area/area_system.go`, `ability/cast_hook_system.go`,
  `echo/elimination_system.go`.
- Order test: `queuetest.CheckOrder` in `pkg/queue/queuetest`, called from
  `shards/game/test/harness_test.go`. Mutants: `QU-*` in `scripts/mutate/mutants.json`.
- `sim.go` hashes `pkg/queue` into `SimVersion`.
