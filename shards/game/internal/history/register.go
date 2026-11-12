package history

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/history/component"
)

// RegisterTypes registers the history feature's components and events with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[component.History]()
	w.RegisterEvent[TargetSync]()
}
