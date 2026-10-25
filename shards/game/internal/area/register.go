package area

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/area/component"
)

// RegisterTypes registers the area feature's component with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Area]()
}
