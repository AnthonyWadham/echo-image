package track

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/track/component"
)

// RegisterTypes registers the track feature's components and events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.Track]()
	w.RegisterEvent[TargetSync]()
}
