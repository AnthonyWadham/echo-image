package player

import (
	"github.com/shamaton/msgpack/v3"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// Move is a client command carrying the player's desired movement direction as a vector; the server
// normalizes it. Sent whenever input changes, including a zero vector to stop. Routed to the sender's
// Body by the authenticated persona on the command.
//
// Fields are untagged on purpose: the introspect schema (and thus the payload the MCP/gateway encode)
// keys off the Go field name when there's no msgpack tag, so DirX/DirY here match the wire.
type Move struct {
	DirX float64
	DirY float64
}

// Name is the command name clients (and the MCP send_command) use.
func (Move) Name() string { return "move" }

// moveCodec decodes the command payload as MSGPACK. This is deliberate and verified against the
// running stack: the dev MCP `send_command` encodes payloads as msgpack (wire byte 0x82 = fixmap),
// NOT protobuf. `world sdk generate` emits a *protobuf* codec instead, which fails here with
// "proto: cannot parse invalid wire-format data" — the generator and the MCP disagree on the wire
// format (see the debug-mcp-commands work). Keep msgpack until the MCP sends protobuf or a real
// world-SDK frontend (which would use the generated codec) exists. Do NOT run `world sdk generate`
// for this command — its output collides with this codec and breaks MCP testing.
type moveCodec struct{}

func (moveCodec) Marshal(p cardinal.Command) ([]byte, error) { return msgpack.Marshal(p) }

func (moveCodec) Unmarshal(data []byte) (cardinal.Command, error) {
	var c Move
	if err := msgpack.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return c, nil
}

// init registers the codec before the world boots (required, or WithCommand[Move] panics). Runs
// automatically because main.go imports the player package.
func init() {
	cardinal.RegisterCommandCodec("move", moveCodec{})
}
