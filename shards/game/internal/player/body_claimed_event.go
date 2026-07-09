package player

import "github.com/argus-labs/world-engine/pkg/cardinal"

// BodyClaimed is emitted the single tick a Body's Persona goes from empty to set — when the first Move
// command claims a free Body (see MovementSystem). It's a player-owned system event; echo.SpawnSystem
// consumes it to give the Body its one Echo. For why the spawn is edge-triggered on the claim rather
// than presence-reactive, see echo.SpawnSystem.
type BodyClaimed struct {
	Body cardinal.EntityID
}

// Name is the system-event name.
func (BodyClaimed) Name() string { return "body_claimed" }
