// Package team holds allegiance — which side an entity fights for: its Team, and its Owner, the player
// it acts for. It's a shared primitive because player Bodies, Echoes, projectiles, and the win
// condition all key off it; ownership is genuinely cross-cutting, so it lives here rather than inside
// any one feature.
//
// Team is deliberately NOT encoded in physics category bits. "Enemy" is relational (team A's enemies
// are teams B and C, who aren't allies of each other) and teams are assigned at match start, so it
// can't be a fixed bit — friend/foe is a component read at collision time
// (see CLAUDE.md → Collision filtering & teams).
package team

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/interact"
)

// Team is an entity's allegiance.
type Team struct {
	ID uint8
}

// Name is the ECS component name.
func (Team) Name() string { return "team" }

// Owner is the player a thing acts for, as that player's Body: a Body itself, an Echo's Body, a shot's
// shooter. With Team it decides how two things relate (interact.Relation). A reflect would change it.
type Owner struct {
	Body cardinal.EntityID
}

// Name is the ECS component name.
func (Owner) Name() string { return "owner" }

// SideOf returns who e acts for, for interact rules. e must have an Owner and a Team.
func SideOf(e cardinal.Entity) interact.Side {
	return interact.Side{Owner: uint32(e.Get[Owner]().Body), Team: e.Get[Team]().ID}
}

// RegisterTypes registers Team and Owner with the world.
func RegisterTypes(w *cardinal.World) {
	w.RegisterComponent[Team]()
	w.RegisterComponent[Owner]()
}
