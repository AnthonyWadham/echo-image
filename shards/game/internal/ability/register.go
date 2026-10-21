package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/ability/component"
)

// RegisterTypes registers the ability feature's component and events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Abilities]()
	w.RegisterSystemEvent[component.AbilityCast]()
	w.RegisterEvent[AbilitySync]()
}
