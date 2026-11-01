// Package ref tells an entity from a newer one that took its ID. Cardinal gives a freed ID out again at
// once, so one ID can name two entities a tick apart. Every constructor stamps a Spawn serial, which the
// world never gives out twice: syncs carry it, and clients and the predictor key what they keep across
// ticks by it. A component-only leaf.
package ref

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Spawn is an entity's serial, from Next when its constructor made it.
type Spawn struct {
	Serial uint64
}

// Name is the component name.
func (Spawn) Name() string { return "spawn" }

// serials is the world's one count of serials given out. It's saved with the world, so a restore never
// gives one out twice.
type serials struct {
	Last uint64
}

// Name is the component name.
func (serials) Name() string { return "serials" }

type counter struct {
	Serials serials
}

// Next gives out the world's next serial, from 1.
func Next(w *cardinal.World) uint64 {
	for e := range w.Contains[counter]().Iter() {
		n := e.Get[serials]().Last + 1
		e.Set(serials{Last: n})
		return n
	}
	w.Create[counter]().Set(serials{Last: 1})
	return 1
}

// RegisterTypes registers Spawn and the count with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[Spawn]()
	w.RegisterComponent[serials]()
}
