// Package team holds allegiance — which side an entity fights for. It's a shared primitive because
// player Bodies, Echoes, projectiles, and the win condition all key off it; ownership is genuinely
// cross-cutting, so it lives here rather than inside any one feature.
//
// Team is deliberately NOT encoded in physics category bits. "Enemy" is relational (team A's enemies
// are teams B and C, who aren't allies of each other) and teams are assigned at match start, so it
// can't be a fixed bit — friend/foe is a component read at collision time
// (see CLAUDE.md → Collision filtering & teams).
package team

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Team is an entity's allegiance.
type Team struct {
	ID uint8
}

// Name is the ECS component name.
func (Team) Name() string { return "team" }

// Team ids for the current small-team modes (1v1 / 2v2 / 3v3).
const (
	A uint8 = 0
	B uint8 = 1
)

// RegisterTypes registers Team with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[Team]()
}

// of returns entity e's team id, or ok=false when e has no Team or no longer exists. Unexported until
// a feature needs a bare single-entity team read; SameTeam is the entry point.
func of(w *cardinal.World, e cardinal.EntityID) (id uint8, ok bool) {
	entity := w.Entity(e)
	if !entity.Has[Team]() {
		return 0, false
	}
	return entity.Get[Team]().ID, true
}

// SameTeam reports whether a and b are on the same team. An entity with no Team (environment, neutral
// geometry) is never "same" as anything, so a missing team can't read as allied and suppress a check.
func SameTeam(w *cardinal.World, a, b cardinal.EntityID) bool {
	ta, oka := of(w, a)
	if !oka {
		return false // short-circuit: no second lookup when the first entity is teamless
	}
	tb, okb := of(w, b)
	return okb && ta == tb
}
