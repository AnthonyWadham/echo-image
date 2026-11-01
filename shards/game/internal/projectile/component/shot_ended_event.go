package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	abilitycomp "echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/projectile/flight"
)

// ShotEnded is emitted the tick a shot ends, for every end: its range, the map, or a Body or Echo, by
// a claim or not. End is where, when in the tick and on what. It lives in this leaf so any feature can
// react to a shot's end without importing projectile's systems.
type ShotEnded struct {
	Shot   cardinal.EntityID
	Type   string // shot type ID
	Cause  abilitycomp.Cause
	Team   uint8
	Radius float64
	End    flight.End
}

// Name is the system-event name.
func (ShotEnded) Name() string { return "shot_ended" }
