package player

import (
	"fmt"
	"slices"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/timestep"
)

// MovementTuning is data-driven movement config, loaded once at boot from content/player/tuning.json
// and handed to each system that reads it — never hardcode the speed.
type MovementTuning struct {
	InputBufferSeconds  float64 `json:"input_buffer_seconds"`  // abuse cap on queued input; older drops
	InputTimeoutSeconds float64 `json:"input_timeout_seconds"` // a Body with no input this long stops
	EchoPathSeconds     float64 `json:"echo_path_seconds"`     // how much Echo path YouState sends
	// A client keeps a target of frames queued: a tick at first, and QueueStepSeconds more each time its
	// queue runs dry, less that much for each QueueCalmSeconds it doesn't, up to input_buffer_seconds.
	QueueStepSeconds float64                `json:"queue_step_seconds"`
	QueueCalmSeconds float64                `json:"queue_calm_seconds"`
	Champions        map[string]ChampionDef `json:"champions"`    // by champion ID
	DevChampion      string                 `json:"dev_champion"` // the champion DevSpawnSystem's Bodies play
	DevSpawns        []DevSpawn             `json:"dev_spawns"`   // the Bodies DevSpawnSystem makes
}

// DevSpawn is a Body DevSpawnSystem makes at world init. Where it starts is the map's.
type DevSpawn struct {
	Nickname string `json:"nickname"`
	Team     uint8  `json:"team"`
}

// DevPlayers is how many Bodies DevSpawnSystem makes on each team.
func (t MovementTuning) DevPlayers() map[uint8]int {
	players := map[uint8]int{}
	for _, d := range t.DevSpawns {
		players[d.Team]++
	}
	return players
}

// Radii is each champion's Body radius, once each, smallest first: the sizes a map must fit.
func (t MovementTuning) Radii() []float64 {
	var radii []float64
	for _, c := range t.Champions {
		radii = append(radii, c.Radius)
	}
	slices.Sort(radii)
	return slices.Compact(radii)
}

// ChampionDef is a champion's Body.
type ChampionDef struct {
	MoveSpeed float64 `json:"move_speed"` // world units per second at full input
	Radius    float64 `json:"radius"`     // the Body's collider circle
	// How its client draws a correction to its prediction: one longer than SnapSeconds of its move speed
	// jumps, and a shorter one shrinks to 1/e in FadeSeconds. 0 draws every correction at once.
	SnapSeconds float64 `json:"snap_seconds"`
	FadeSeconds float64 `json:"fade_seconds"`
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

// QueueStepFrames converts QueueStepSeconds to frames.
func (t MovementTuning) QueueStepFrames() int { return timestep.Ticks(t.QueueStepSeconds) }

// QueueCalmTicks converts QueueCalmSeconds to ticks.
func (t MovementTuning) QueueCalmTicks() uint64 { return uint64(timestep.Ticks(t.QueueCalmSeconds)) }

// TimeoutTicks converts InputTimeoutSeconds to ticks.
func (t MovementTuning) TimeoutTicks() uint64 {
	return uint64(timestep.Ticks(t.InputTimeoutSeconds))
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t MovementTuning) Validate() error {
	if t.BufferFrames() < 1 || t.TimeoutTicks() < 1 || t.EchoPathTicks() < 0 || t.QueueStepFrames() < 1 ||
		t.QueueCalmTicks() < 1 {
		return fmt.Errorf("movement_tuning: input_buffer_seconds, input_timeout_seconds, queue_step_seconds and "+
			"queue_calm_seconds must be >= 1 tick at %d Hz, and echo_path_seconds >= 0, got %+v",
			physics.ServerTickRateHz, t)
	}
	for id, c := range t.Champions {
		if c.MoveSpeed < 0 || c.Radius <= 0 || c.SnapSeconds < 0 || c.FadeSeconds < 0 {
			return fmt.Errorf("movement_tuning: champion %q needs radius > 0, and move_speed, snap_seconds and "+
				"fade_seconds >= 0, got %+v", id, c)
		}
	}
	if _, ok := t.Champions[t.DevChampion]; !ok {
		return fmt.Errorf("movement_tuning: dev_champion %q is not a champion", t.DevChampion)
	}
	if len(t.DevSpawns) == 0 {
		return fmt.Errorf("movement_tuning: dev_spawns lists no Body, so no player can join")
	}
	return nil
}
