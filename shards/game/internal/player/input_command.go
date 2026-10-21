package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/component"
)

// Input carries a client's unacked input frames, newest last. Clients send one per tick, so a late
// or lost message is covered by the next. Session is random per client run. Routed to the sender's
// Body by the authenticated player ID on the command.
type Input struct {
	Session uint32
	Frames  cardinal.Slice[component.InputFrame]
}

// Name is the command name clients (and the MCP send_command) use.
func (Input) Name() string { return "input" }
