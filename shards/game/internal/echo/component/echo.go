// Package component holds the echo feature's ECS data — the leaf other features import to read an
// Echo without depending on echo's systems, so no import cycle is possible
// (see CLAUDE.md → File layout within a package).
package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics2dcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// PathHistory is the Echo's ring buffer of the Body's recent positions, driving the delayed replay: the
// tick's outcome, not the input, so dashes and knockback replay too. Poses is
// sized once at spawn to delayTicks+1 (delay_seconds × the server tick rate, +1 so the read slot never
// collides with the write slot) — it holds exactly the configured delay, no hardcoded ceiling. It's an
// ECS component (not a Go map) so snapshots capture the trail and an in-flight match resumes on it
// (GDD §11). Head indexes the newest frame.
type PathHistory struct {
	Poses cardinal.Slice[physics2dcomp.Vec2]
	Head  int
}

// Name is the ECS component name.
func (PathHistory) Name() string { return "path_history" }

// Delay is how many ticks the Echo trails its Body.
func (h PathHistory) Delay() int { return max(h.Poses.Len()-1, 0) }

// Path is the Echo's spot this tick, then where it will be on each of the next n ticks, as far as the
// ring holds: its Body's poses from Delay ticks ago on, so at most Delay of them.
func (h PathHistory) Path(n int) []physics2dcomp.Vec2 {
	size := h.Poses.Len()
	if size == 0 {
		return nil
	}
	path := make([]physics2dcomp.Vec2, min(n, size-1)+1)
	for i := range path {
		path[i] = h.Poses.At((h.Head + 1 + i) % size) // the oldest first
	}
	return path
}

// Echo marks a delayed replay of a Body: the Body in its team.Owner.
type Echo struct {
	PlayerID uint32
}

// Name is the ECS component name.
func (Echo) Name() string { return "echo" }
