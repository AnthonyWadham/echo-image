package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/projectile/component"
)

// RegisterTypes registers the projectile feature's component and event with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Projectile]()
	w.RegisterEvent[ProjectileSync]()
}
