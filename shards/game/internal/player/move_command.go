package player

// Move is a client command carrying the player's desired movement direction as a vector; the server
// normalizes it. Sent whenever input changes, including a zero vector to stop. Routed to the sender's
// Body by the authenticated persona on the command.
type Move struct {
	DirX float64
	DirY float64
}

// Name is the command name clients (and the MCP send_command) use.
func (Move) Name() string { return "move" }
