package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/component"
)

// RegisterTypes registers the player feature's component, command, and system event with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Player]()
	w.RegisterCommand[Move]()
	w.RegisterSystemEvent[BodyClaimed]()
}
