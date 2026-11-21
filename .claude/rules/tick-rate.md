---
paths:
  - "shards/game/**"
  - "pkg/**"
  - "client/src/**"
---

# Tick-rate independence (fixed-timestep sim)
The simulation must be **tick-rate agnostic**: changing `TickRate` changes only how finely we sample
time — never how fast the game plays or how hard anything hits. A build at 60 Hz must be the *same
game* as one at 20 Hz, just smoother: exact on open ground and along straight walls; a Body rounding a
corner or a pillar differs by its sampling, a few cm. Every system honors this:
- **Rates are per-second, scaled by `dt` at the point of use.** `dt = 1 / physics.ServerTickRateHz`
  (`physics/timestep`, which the client predictor shares).
  Never bake a per-*tick* constant. A damage-over-time of 25/s is `25 * dt` this tick, not a hardcoded
  `1.25`. Movement already complies: `Velocity2D` is world-units-per-*second* and
  `player.MovementSystem` moves a Body by `velocity * dt`, so units/second is constant across tick
  rates — a faster tick rate must never move anything faster.
- **Durations & delays are configured in seconds and converted to ticks** by `timestep.Ticks`
  (`round(seconds * Rate)`), the one place that converts — never stored as a raw tick count, the
  snapshot interval (`SnapshotRate`, 0.5 s) included. The Echo delay, cooldowns, and any ring-buffer
  sizing all derive from seconds so they hold their wall-clock meaning when the rate changes.
- **Discrete vs continuous.** A discrete event (a projectile hit, a spawn) applies a fixed amount and is
  already rate-independent. A continuous effect (DoT, regen, movement) must scale by `dt`.
- **Sub-unit-per-tick damage.** `dt`-scaled continuous damage is fractional, and `Health.HP` is a
  float64, so a continuous source applies its fraction each tick, with no accumulator. Never round it
  per tick, or a DoT rounds toward zero at high tick rates.
- **One source of truth for the rate.** Read `physics.ServerTickRateHz` (or a `physics` `dt` helper);
  never hardcode `20`. `main.go` already feeds it to both the Cardinal `WorldOptions.TickRate` and the
  physics2d step. A build tag picks it: `timestep.Rate` is 20, or 60 built with `-tags rate60`
  (`rate20.go`, `rate60.go`), and the sim version says which. `world build` passes no build tags yet,
  so a deployed shard runs 20 Hz (F23 in `docs/research/foundation-review.md`).
- **Tests run at both rates.** They state durations in seconds (`ticks(seconds)` in `harness_test.go`);
  raw ticks appear only as a few ticks of margin or a fractional view lag. Each rate has its own
  prediction recording. The client converts seconds to frames with body_sync's `TickSeconds`
  (`InputStream`, whose tests loop over both rates); `GOFLAGS=-tags=rate60` reruns the predictor's tests
  on a 60 Hz wasm, and the rest of the client's tests read no rate.
