// Package component holds the effect feature's ECS data: the effect list and EffectIntent. Any feature
// reads an entity's status here, and asks for a change by emitting EffectIntent. It imports only the
// engine, the pure effect/status core and interact.
package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
)

// TagsOf is the tags e's effects give it at tick, or none if it has no Effects.
func TagsOf(e cardinal.Entity, tick uint64) interact.Tags {
	if !e.Has[Effects]() {
		return 0
	}
	return e.Get[Effects]().Status(tick).Tags
}

// Effects is an entity's timed effects, in the order they were added. Only effect.ResolveSystem
// writes it. Next numbers the entries.
type Effects struct {
	Next uint32
	List cardinal.Slice[status.Entry]
}

// Name is the ECS component name.
func (Effects) Name() string { return "effects" }

// Status folds the entries active at tick.
func (e Effects) Status(tick uint64) status.Status { return status.Fold(e.List.Values(), tick) }

// Shown is the kinds active at tick, as other screens see them.
func (e Effects) Shown(tick uint64) cardinal.Slice[status.Shown] {
	return cardinal.SliceOf(status.ShownAt(e.List.Values(), tick)...)
}
