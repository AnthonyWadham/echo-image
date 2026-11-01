package component

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"

	abilitycomp "echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/projectile/flight"
)

// ShotEnded is emitted the tick a shot ends, for every end: its range, the map, or a Body or Echo. Its
// last tick's flight began at Prev, and End is where, when in the tick and on what. It lives in this leaf
// so any feature can react to a shot's end without importing projectile's systems.
type ShotEnded struct {
	Shot      cardinal.EntityID
	Serial    uint64 // the shot's ref.Spawn serial: it is gone before the syncs read this
	Type      string // shot type ID
	Cause     abilitycomp.Cause
	Predicted bool
	Team      uint8
	Radius    float64
	Prev      box2d.Vec2
	End       flight.End
}

// Name is the system-event name.
func (ShotEnded) Name() string { return "shot_ended" }
