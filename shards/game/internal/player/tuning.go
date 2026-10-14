package player

import (
	"fmt"
	"math"

	"echo_image/shards/game/internal/physics"
)

// MovementTuning is data-driven movement config, loaded once at boot from content/player/tuning.json.
// Read it via data.Get[MovementTuning]() at system entry — never hardcode the speed. It's a Singleton
// (one JSON object). TODO: when champions land, move speed becomes per-champion — this either grows
// into a broader player tuning or is superseded by champion definitions.
type MovementTuning struct {
	MoveSpeed           float64 `json:"move_speed"`            // Body speed in world units per second
	InputBufferSeconds  float64 `json:"input_buffer_seconds"`  // most input a Body queues; older frames drop
	InputTimeoutSeconds float64 `json:"input_timeout_seconds"` // a Body with no input this long stops
}

// Name is the data-catalog key used by data.Get[MovementTuning]().
func (MovementTuning) Name() string { return "movement_tuning" }

// JSONFile is the path within the content embed.FS.
func (MovementTuning) JSONFile() string { return "player/tuning.json" }

// BufferFrames converts InputBufferSeconds to frames (one frame per tick).
func (t MovementTuning) BufferFrames() int {
	return int(math.Round(t.InputBufferSeconds * float64(physics.ServerTickRateHz)))
}

// TimeoutTicks converts InputTimeoutSeconds to ticks.
func (t MovementTuning) TimeoutTicks() uint64 {
	return uint64(math.Round(t.InputTimeoutSeconds * float64(physics.ServerTickRateHz)))
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t MovementTuning) Validate() error {
	if t.MoveSpeed < 0 {
		return fmt.Errorf("movement_tuning: move_speed must be >= 0, got %v", t.MoveSpeed)
	}
	if t.BufferFrames() < 1 || t.TimeoutTicks() < 1 {
		return fmt.Errorf("movement_tuning: input_buffer_seconds and input_timeout_seconds must be >= 1 tick at %d Hz, got %+v",
			physics.ServerTickRateHz, t)
	}
	return nil
}
