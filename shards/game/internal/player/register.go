package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/component"
)

// RegisterTypes registers the player feature's components, command, and events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Player]()
	w.RegisterComponent[component.Champion]()
	w.RegisterComponent[component.InputQueue]()
	w.RegisterComponent[component.MoveSpeed]()
	w.RegisterCommand[Input]()
	w.RegisterSystemEvent[component.BodyClaimed]()
	w.RegisterEvent[BodySync]()
}
