package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/team"
)

// bodyRow is a player Body: identity + team + the physics2d trio. Deliberately NO Health — the Body
// is invincible; Health lives on the Echo (added with the echo feature).
type bodyRow struct {
	Player    cardinal.Ref[Player]
	Team      cardinal.Ref[team.Team]
	Transform cardinal.Ref[physics2d.Transform2D]
	Velocity  cardinal.Ref[physics2d.Velocity2D]
	Body      cardinal.Ref[physics2d.PhysicsBody2D]
}

// DevSpawnSystemState spawns players directly at world init. This is dev scaffolding — real spawning
// will be command-driven (matchmaking / client join). Replace this system when that lands.
type DevSpawnSystemState struct {
	cardinal.BaseSystemState
	Bodies cardinal.Contains[bodyRow]
}

// DevSpawnSystem drops two players on opposing teams (a 1v1) at fixed spawn points.
func DevSpawnSystem(state *DevSpawnSystemState) {
	spawnBody(state, 1, "Player One", team.A, physics2d.Vec2{X: -5, Y: 0})
	spawnBody(state, 2, "Player Two", team.B, physics2d.Vec2{X: 5, Y: 0})
}

func spawnBody(state *DevSpawnSystemState, id uint32, nickname string, teamID uint8, pos physics2d.Vec2) {
	_, b := state.Bodies.Create()
	b.Player.Set(Player{ID: id, Nickname: nickname})
	b.Team.Set(team.Team{ID: teamID})
	b.Transform.Set(physics2d.Transform2D{Position: pos})
	b.Velocity.Set(physics2d.Velocity2D{})
	// Kinematic: the Body is driven by player input later, not by physics forces. No Health component
	// is set, which is what makes it invincible (never a combat damage target).
	b.Body.Set(physics.NewBody2D(physics2d.BodyTypeKinematic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       0.5,
		Density:      1,
		CategoryBits: physics.CategoryBody,
		MaskBits:     physics.CategoryBody | physics.CategoryEnvironment, // solid vs. other bodies and walls
	}))
	state.Logger().Info().
		Uint32("id", id).Str("nickname", nickname).Uint8("team", teamID).
		Float64("x", pos.X).Float64("y", pos.Y).
		Msg("player spawned (dev)")
}
