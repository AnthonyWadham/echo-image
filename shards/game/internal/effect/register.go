package effect

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/effect/component"
)

// RegisterTypes registers the effect feature's component and system event with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Effects]()
	w.RegisterSystemEvent[component.EffectIntent]()
}
