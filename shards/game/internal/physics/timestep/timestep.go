// Package timestep is the fixed timestep that Cardinal ticks, the physics step and the client
// predictor all run at. It imports only math, so the predictor builds it for wasm. Server code reads
// the rate as physics.ServerTickRateHz and physics.SecondsPerTick. Every duration becomes ticks
// through Ticks, and nowhere else.
package timestep

import "math"

// Seconds is one tick: 1 / Rate.
const Seconds = 1.0 / float64(Rate)

// Ticks is seconds as whole ticks at Rate.
func Ticks(seconds float64) int { return int(math.Round(seconds * float64(Rate))) }
