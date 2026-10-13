package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// bodyRow is a player Body: identity + team + the physics2d trio. Deliberately NO Health — the Body
// is invincible; Health lives on the Echo (added with the echo feature).
type bodyRow struct {
	Player    component.Player
	Team      team.Team
	Transform physics2d.Transform2D
	Velocity  physics2d.Velocity2D
	Body      physics2d.PhysicsBody2D
}

// DevSpawnSystem drops two players on opposing teams (a 1v1) at fixed spawn points at world init.
// This is dev scaffolding — real spawning will be command-driven (matchmaking / client join). Replace
// this system when that lands.
type DevSpawnSystem struct{}

func (s *DevSpawnSystem) Run(w *cardinal.World) {
	spawnBody(w, 1, "Player One", team.A, physics2d.Vec2{X: -5, Y: 0})
	spawnBody(w, 2, "Player Two", team.B, physics2d.Vec2{X: 5, Y: 0})
}

func spawnBody(w *cardinal.World, id uint32, nickname string, teamID uint8, pos physics2d.Vec2) {
	b := w.Create[bodyRow]()
	// Persona left empty: the Body is unclaimed until the first Move command from a persona claims it
	// (see MovementSystem). Real matchmaking will assign the persona at spawn instead.
	b.Set(component.Player{ID: id, Nickname: nickname})
	b.Set(team.Team{ID: teamID})
	b.Set(physics2d.Transform2D{Position: pos})
	b.Set(physics2d.Velocity2D{})
	// Kinematic: the Body is driven by player input later, not by physics forces. No Health component
	// is set, which is what makes it invincible (never a combat damage target).
	b.Set(physics.NewBody2D(physics2d.BodyTypeKinematic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       0.5,
		Density:      1,
		CategoryBits: physics.CategoryBody,
		MaskBits:     physics.CategoryBody | physics.CategoryEnvironment, // solid vs. other bodies and walls
	}))
	w.Logger().Info().
		Uint32("id", id).Str("nickname", nickname).Uint8("team", teamID).
		Float64("x", pos.X).Float64("y", pos.Y).
		Msg("player spawned (dev)")
}
