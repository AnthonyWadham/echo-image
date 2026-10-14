package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/component"
)

// RegisterTypes registers the player feature's components, command, and system event with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Player]()
	w.RegisterComponent[component.Input]()
	w.RegisterComponent[component.MoveSpeed]()
	w.RegisterCommand[Move]()
	w.RegisterSystemEvent[BodyClaimed]()
}
