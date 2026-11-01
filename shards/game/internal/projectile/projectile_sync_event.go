package projectile

import "github.com/argus-labs/world-engine/pkg/cardinal"

// ProjectileSync is one player's view of the shots this tick. Sent every tick and complete: the shots
// in flight, and once more each shot that ended in the tick, where it ended.
type ProjectileSync struct {
	Tick        uint64
	Projectiles cardinal.Slice[ProjectileState]
}

// Name is the event name.
func (ProjectileSync) Name() string { return "projectile_sync" }

// ProjectileState is one shot. It flew this tick from PrevX, PrevY (its launch point on its first tick)
// to X, Y, and Until is the part of the tick that took: 1 in flight, and for an Ended shot when it got
// there. An Ended shot that met a Body or Echo (OnTarget) ends on the one with serial TargetSerial,
// Offset from its center, where each screen draws it. Your own predicted shots aren't sent to you: your
// predictor draws them.
type ProjectileState struct {
	EntityID         uint32
	Serial           uint64 // its ref.Spawn serial: a client keys it by this, as a newer shot may take its ID
	Type             string // shot type ID, for its look
	Caster           uint32 // the Body or Echo that cast it
	Team             uint8
	PrevX, PrevY     float64
	X, Y             float64
	Radius           float64
	Until            float64
	Ended            bool
	OnTarget         bool
	TargetSerial     uint64
	OffsetX, OffsetY float64
}
