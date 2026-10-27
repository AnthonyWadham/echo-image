// Package timestep is the fixed timestep that Cardinal ticks, the physics step and the client
// predictor all run at. It imports nothing, so the predictor builds it for wasm. Server code reads
// it as physics.ServerTickRateHz and physics.SecondsPerTick.
package timestep

// Rate is ticks per second.
const Rate = 20

// Seconds is one tick: 1 / Rate.
const Seconds = 1.0 / float64(Rate)
