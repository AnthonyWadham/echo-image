package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/queue"
	"echo_image/shards/game/internal/combat/component"
)

// RegisterTypes registers the combat feature's component, the queues it drains and Died with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Health]()
	queue.Declare[component.DamageIntent](w)
	queue.Declare[component.HealIntent](w)
	w.RegisterSystemEvent[component.Died]()
}
