---
paths:
  - "pkg/queue/**"
  - "shards/game/internal/**"
---

# Owner queues (`pkg/queue`)
- **An intent goes to its owner's queue.** A request to change state another feature owns (damage, a
  heal, an effect, a launch) is sent to that owner's queue: a component on one entity. It is not a
  system event: it has one reader, a snapshot keeps it, and one sent after the drain waits for the
  next tick instead of being lost. The owner's one system drains it once a tick, sorted by the tick
  each was due, then the owner's compare, never by send order. A second drain in a tick panics.
- **No loop guard yet.** Nothing in echo that triggers on a hit or a death can trigger again. Add a
  guard with the first mechanic that can loop, as a rule on that mechanic (`docs/backlog.md`).
- **Delayed events** (`queue.DeclareDelayed`, `Schedule`, `Cancel`) fire a system event at the start
  of a later tick. Built and tested, with no user yet.
- **What Cardinal lacks, and the stand-ins** (ID versions, the order check, a phase before PreUpdate):
  `pkg/queue/doc.go` and [`docs/research/owner-queues.md`](docs/research/owner-queues.md).
