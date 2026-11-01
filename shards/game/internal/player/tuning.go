package player

import (
	"fmt"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/timestep"
)

// MovementTuning is data-driven movement config, loaded once at boot from content/player/tuning.json
// and handed to each system that reads it — never hardcode the speed.
type MovementTuning struct {
	InputBufferSeconds  float64                `json:"input_buffer_seconds"`  // abuse cap on queued input; older drops
	InputTimeoutSeconds float64                `json:"input_timeout_seconds"` // a Body with no input this long stops
	EchoPathSeconds     float64                `json:"echo_path_seconds"`     // how much Echo path YouState sends
	Champions           map[string]ChampionDef `json:"champions"`             // by champion ID
	DevChampion         string                 `json:"dev_champion"`          // the champion DevSpawnSystem's Bodies play
}

// ChampionDef is a champion's Body.
type ChampionDef struct {
	MoveSpeed float64 `json:"move_speed"` // world units per second at full input
	Radius    float64 `json:"radius"`     // the Body's collider circle
}

// JSONFile is the path within the content embed.FS.
func (MovementTuning) JSONFile() string { return "player/tuning.json" }

// BufferFrames converts InputBufferSeconds to frames (one frame per tick).
func (t MovementTuning) BufferFrames() int {
	return timestep.Ticks(t.InputBufferSeconds)
}

// EchoPathTicks converts EchoPathSeconds to ticks: how much of your Echo's coming path YouState
// carries.
func (t MovementTuning) EchoPathTicks() int {
	return timestep.Ticks(t.EchoPathSeconds)
}

// TimeoutTicks converts InputTimeoutSeconds to ticks.
func (t MovementTuning) TimeoutTicks() uint64 {
	return uint64(timestep.Ticks(t.InputTimeoutSeconds))
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t MovementTuning) Validate() error {
	if t.BufferFrames() < 1 || t.TimeoutTicks() < 1 || t.EchoPathTicks() < 0 {
		return fmt.Errorf("movement_tuning: input_buffer_seconds and input_timeout_seconds must be >= 1 tick at %d Hz, "+
			"and echo_path_seconds >= 0, got %+v", physics.ServerTickRateHz, t)
	}
	for id, c := range t.Champions {
		if c.MoveSpeed < 0 || c.Radius <= 0 {
			return fmt.Errorf("movement_tuning: champion %q needs move_speed >= 0 and radius > 0, got %+v", id, c)
		}
	}
	if _, ok := t.Champions[t.DevChampion]; !ok {
		return fmt.Errorf("movement_tuning: dev_champion %q is not a champion", t.DevChampion)
	}
	return nil
}
