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

// Row looks an entity's Team up by id via cardinal.Contains[Row].GetByID.
type Row struct {
	Team cardinal.Ref[Team]
}
