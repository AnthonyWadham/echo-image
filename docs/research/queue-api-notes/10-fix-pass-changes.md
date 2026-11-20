=== CHANGES ===

What I checked: I read REPORT.md, 08 and 09 in full, plus 01. I checked the Go signatures against WE `main` f7d4a52b, read-only.

API bugs fixed:
- `cardinal.SystemEvent` does not exist. The constraint is in `internal/ecs`, so other packages cannot name it. The `Event` interface now uses `Name() string`.
- The system options are renamed to `WithDrains`, `WithEmits`, `WithReads` and `WithSends`, to match the real `WithPhase`.
- `Drained` is now stored as tick+1. Tick height starts at 0, so the old field would have panicked on the first drain at tick 0.
- Gen starts at 1. Entity ID 0 is a valid entity, so a zero Handle would otherwise look like a real one.

Restore holes closed:
- Limits (`MaxLen`, `MaxDepth`) were stored in the component. A restore would bring back old caps, and that contradicted the report's own "config is not stored" line. They now come from the payload's `Limits()` method. `MaxDepth` is one constant, because a chain crosses queue types.
- Generations of free IDs are now saved. Without that, a restore resets them and a stale Handle can match again.

How each 09 issue was handled:
- 1: void (REPORT.md now exists).
- 2: the key was already right. I dropped 09's `Kind` field: each queue holds one type, so Kind never varies. Added: Seq only breaks program-order ties; who is hit by restore divergence; the queue cannot fix order inside a sender.
- 3: `Queue` was already handled. Added `Delayed[E].Name()`, the component name rule for `E.Name()`, and a test that registers two instantiations in one world.
- 4: already handled (`uint64` Seq). Added the uint32 wrap to the alternatives.
- 5: already handled, because `Send` owns its Get/Append/Set. I corrected the signatures as listed above.
- 6: already handled.
- 7: already handled. One number set (48 B) is quoted.
- 8: partly handled. Added "no Go buffer outlives the sending system". The alternatives row now gives the real reason: no hook runs before the snapshot, so a buffered request is lost on restore.
- 9: partly handled. Added:
  - a Header-literal analyzer and a check for `Root` called inside a reader loop;
  - a TestWorld panic when a reader system calls `Root`;
  - `Schedule` checks depth;
  - repeats are owner state;
  - DST panics too.
  - Dropped the exhaustruct row (an explicit `Depth: 0` passes it).
  - Partly rejected: 09 wanted `Root` to take a client command. Contacts and owner timers are also true starts, so the checks above are used instead.
- 10: partly taken. Due input stays absolute. `Send` and `Schedule` now range-check it: panic in TestWorld, clamp + log + count in prod. Rejected 09's `delay Ticks` input (row in alternatives): it adds a second time model, and the range check catches the same underflow and past-tick bugs.
- 11: rejected 09's plugin spawn-serial fix (row in alternatives). Every target would need the component, and it only fixes queues. Kept the engine generation. `Source` is now a Handle too, which closes the cancel-by-source reuse hole. Ticket is typed: `Ticket[E]`.
- 12: added the drain steps and the cancel timing (a cancel on the due tick is too late, and a test checks it). Cancel returns false for fired, cancelled or unknown items. Tickets are never reused.
- 13: (b) was already handled; added the `world purge` error text. (a) was missing; added the Init-events fact and an engine fix (panic on `EmitSystemEvent` during Init).
- 14: mostly handled. Added "First is reserved for fire systems". Fixed the verdict overclaim that every rule has a precedent.
- 15: added `WithSends` and the line that same-tick apply is incidental. Put the "always next tick" option under Open.
- 16: added the per-handle cancel precedents. Unreal is now marked (S) in the G1 row and the Q4 table.
- 17: Unity 6.6 marked (S). Added the analyzer's limits and its red test. Mover, GAS and the Go spec were already (S). The user measurements are now marked (S) instead of (V).
- 18: Go links pinned to go1.25.5. Mass `master` and Quantum `GameTimer` were already right. flecs.h is not cited, so nothing to fix.
- 19: the alternatives row now says the key has no room for Target and Source.
- 20: already marked (I). Added the singleton lookup to the cost estimate.
- 21: added the "who is hit" line.
- 22: already consistent. Copy-out plus `SortFunc` is fine because the drain copies out anyway.
- 23: alternatives and the modifier and spawn gaps were already there. Added the open points: how a sender learns the outcome, and the same-tick vs next-tick option.

Cuts: removed the duplicate RegisterSystem snippet from §1.3 and the exhaustruct test row. The report is about the same length as before.
