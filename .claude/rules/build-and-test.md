---
paths:
  - "shards/game/test/**"
  - "scripts/**"
  - "**/*_test.go"
---

# Build and test details

- **Random play:** `TestRandomPlayIsPredicted` plays 2,000 random but honest ticks from a new seed
  each run, and checks prediction every tick. A failure names its seed; `-seed` reruns it.
  `FuzzFlyMeetsAMovingTargetWhenTheyTouch` (`go test -fuzz`) fuzzes when a shot meets a moving target.
- **Tick cost:** `BenchmarkTick` times a 3v3 tick (`go test -run '^$' -bench Tick ./shards/game/test`, with
  `-tags rate60` for 60 Hz). `TestTickAllocations` fails when that tick allocates past its ceiling for
  the rate. Allocation counts don't depend on the machine; raise a ceiling only for a known reason.
