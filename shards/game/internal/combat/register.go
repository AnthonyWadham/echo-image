package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/combat/component"
)

// RegisterTypes registers the combat feature's component and system events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Health]()
	w.RegisterSystemEvent[component.DamageIntent]()
	w.RegisterSystemEvent[component.HealIntent]()
	w.RegisterSystemEvent[component.Died]()
}
