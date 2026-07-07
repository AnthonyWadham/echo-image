package player

import "fmt"

// MovementTuning is data-driven movement config, loaded once at boot from content/player/tuning.json.
// Read it via data.Get[MovementTuning]() at system entry — never hardcode the speed. It's a Singleton
// (one JSON object). TODO: when champions land, move speed becomes per-champion — this either grows
// into a broader player tuning or is superseded by champion definitions.
type MovementTuning struct {
	MoveSpeed float64 `json:"move_speed"` // Body speed in world units per second
}

// Name is the data-catalog key used by data.Get[MovementTuning]().
func (MovementTuning) Name() string { return "movement_tuning" }

// JSONFile is the path within the content embed.FS.
func (MovementTuning) JSONFile() string { return "player/tuning.json" }

// SingleObject marks this kind as one JSON object rather than an array of records.
func (MovementTuning) SingleObject() {}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t MovementTuning) Validate() error {
	if t.MoveSpeed < 0 {
		return fmt.Errorf("movement_tuning: move_speed must be >= 0, got %v", t.MoveSpeed)
	}
	return nil
}
