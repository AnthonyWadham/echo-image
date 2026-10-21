// Package component holds the player feature's ECS component data — the types other features import
// to read player state. It imports only the engine, so it is a dependency leaf: any feature can read
// these without pulling in player's systems, and no import cycle is possible
// (see CLAUDE.md → File layout within a package).
package component

// Player identifies a player-controlled Body.
type Player struct {
	ID       uint32 // stable player id
	Persona  string // authenticated identity; Input commands route to this Body by matching persona
	Nickname string // display / debug name
}

// Name is the ECS component name.
func (Player) Name() string { return "player" }
