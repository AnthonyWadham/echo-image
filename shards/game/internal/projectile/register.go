package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/projectile/component"
)

// RegisterTypes registers the projectile feature's component and events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Projectile]()
	w.RegisterSystemEvent[component.LaunchIntent]()
	w.RegisterEvent[ProjectileSync]()
}
