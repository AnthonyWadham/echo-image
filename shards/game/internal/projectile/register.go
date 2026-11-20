package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/queue"
	"echo_image/shards/game/internal/projectile/component"
)

// RegisterTypes registers the projectile feature's component, the queue it drains and its events with
// the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Projectile]()
	queue.Declare[component.LaunchIntent](w)
	w.RegisterSystemEvent[component.ShotEnded]()
	w.RegisterEvent[ProjectileSync]()
}
