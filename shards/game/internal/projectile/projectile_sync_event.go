package projectile

import "github.com/argus-labs/world-engine/pkg/cardinal"

// ProjectileSync is one player's view of the shots in flight this tick. Sent every tick and complete:
// a shot missing from it hit something or expired.
type ProjectileSync struct {
	Tick        uint64
	Projectiles cardinal.Slice[ProjectileState]
}

// Name is the event name.
func (ProjectileSync) Name() string { return "projectile_sync" }

// ProjectileState is one shot.
type ProjectileState struct {
	EntityID uint32
	Team     uint8
	X, Y     float64
	Radius   float64
}
