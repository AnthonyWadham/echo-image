package effect

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/queue"
	"echo_image/shards/game/internal/effect/component"
)

// RegisterTypes registers the effect feature's component and the queue it drains with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Effects]()
	queue.Declare[component.EffectIntent](w)
}
