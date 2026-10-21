package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// BodyClaimed is emitted the one tick a player first claims a Body (see player.InputSystem). Features
// set the Body up from it: echo spawns its Echo, ability equips its kit. It lives in this leaf so they
// don't import player's systems.
type BodyClaimed struct {
	Body cardinal.EntityID
}

// Name is the system-event name.
func (BodyClaimed) Name() string { return "body_claimed" }
