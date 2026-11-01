package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/projectile/component"
)

// RegisterTypes registers the projectile feature's components and events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Projectile]()
	w.RegisterComponent[component.TargetHistory]()
	w.RegisterSystemEvent[component.LaunchIntent]()
	w.RegisterSystemEvent[component.ShotEnded]()
	w.RegisterEvent[ProjectileSync]()
	w.RegisterEvent[TargetSync]()
}
