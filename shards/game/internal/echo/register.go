package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/echo/component"
)

// RegisterTypes registers the echo feature's components with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Echo]()
	w.RegisterComponent[component.PathHistory]()
}
