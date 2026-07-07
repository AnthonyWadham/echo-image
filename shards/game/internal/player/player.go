// Package player owns the player Body: the invincible, directly-controlled avatar. The Body carries
// identity (Player) and allegiance (team.Team) but NOT Health — "invincible" is modeled simply by the
// Body having no Health component, so it can never be in combat's Damageable set. The damageable Echo
// (which does carry Health) is a separate entity, added with the echo feature.
package player

// Player identifies a player-controlled Body.
type Player struct {
	ID       uint32 // stable player id
	Persona  string // authenticated identity; Move commands route to this Body by matching persona
	Nickname string // display / debug name
}

// Name is the ECS component name.
func (Player) Name() string { return "player" }
