package netsync

import "github.com/argus-labs/world-engine/pkg/cardinal"

// RegisterTypes registers the netsync feature's event with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterEvent[WorldState]()
}
