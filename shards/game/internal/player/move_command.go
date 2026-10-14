package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/component"
)

// Move carries a client's unacked movement frames, newest last. Clients send one per tick, so a late
// or lost message is covered by the next. Session is random per client run. Routed to the sender's
// Body by the authenticated player ID on the command.
type Move struct {
	Session uint32
	Frames  cardinal.Slice[component.MoveFrame]
}

// Name is the command name clients (and the MCP send_command) use.
func (Move) Name() string { return "move" }
