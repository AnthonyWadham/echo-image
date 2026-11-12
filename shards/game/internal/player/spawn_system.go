package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	effectcomp "echo_image/shards/game/internal/effect/component"
	historycomp "echo_image/shards/game/internal/history/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// bodyRow is a player Body: identity, champion, team, owner (itself), input, speed, the physics2d trio,
// its effect list, and its history (the rows screens draw it from and shots meet it by). Deliberately NO
// Health — the Body is invincible; Health lives on the Echo.
type bodyRow struct {
	Spawn     ref.Spawn
	Player    component.Player
	Champion  component.Champion
	Team      team.Team
	Owner     team.Owner
	Input     component.InputQueue
	Speed     component.MoveSpeed
	Transform physics2d.Transform2D
	Velocity  physics2d.Velocity2D
	Body      physics2d.PhysicsBody2D
	Effects   effectcomp.Effects
	History   historycomp.History
}

// DevSpawnSystem makes the dev roster's Bodies (content: dev_spawns) at world init, numbered from 1 in
// order. Players claim them in that order. This is dev scaffolding — real spawning will be
// command-driven (matchmaking / client join). Replace this system when that lands.
type DevSpawnSystem struct {
	Tuning MovementTuning
}

func (s *DevSpawnSystem) Run(w *cardinal.World) {
	for i, d := range s.Tuning.DevSpawns {
		spawnBody(w, s.Tuning, s.Tuning.DevChampion, uint32(i+1), d.Nickname, d.Team, physics2d.Vec2{X: d.X, Y: d.Y})
	}
}

// spawnBody is the one constructor for a player Body; every spawn path uses it, so no Body is
// missing a component another system needs. Its champion's numbers set its speed and size.
func spawnBody(w *cardinal.World, tuning MovementTuning, champion string, id uint32, nickname string,
	teamID uint8, pos physics2d.Vec2,
) {
	def := tuning.Champions[champion]
	b := w.Create[bodyRow]()
	b.Set(ref.Spawn{Serial: ref.Next(w)})
	// Persona left empty: the Body is unclaimed until the first Input command from a persona claims it
	// (see InputSystem). Real matchmaking will assign the persona at spawn instead.
	b.Set(component.Player{ID: id, Nickname: nickname})
	b.Set(component.Champion{ID: champion})
	b.Set(team.Team{ID: teamID})
	b.Set(team.Owner{Body: b.ID()})
	b.Set(component.MoveSpeed{Value: def.MoveSpeed})
	b.Set(physics2d.Transform2D{Position: pos})
	b.Set(physics2d.Velocity2D{})
	b.Set(historycomp.History{Predicted: true})
	// Manual: MovementSystem owns the pose and stops it at walls with Box2D's mover. Box2D itself never
	// moves or blocks it. No Health component is set, which is what makes it invincible (never a combat
	// damage target).
	b.Set(physics.NewBody2D(physics2d.BodyTypeManual, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       def.Radius,
		Density:      1,
		CategoryBits: uint64(interact.Body),
		MaskBits:     uint64(interact.Wall | interact.Area | interact.Shot), // Bodies pass through each other, for now
	}))
	w.Logger().Info().
		Uint32("id", id).Str("nickname", nickname).Str("champion", champion).Uint8("team", teamID).
		Float64("x", pos.X).Float64("y", pos.Y).
		Msg("player spawned (dev)")
}

// radius is the radius of a Body's collider: spawnBody's one circle.
func radius(body cardinal.Entity) float64 {
	return body.Get[physics2d.PhysicsBody2D]().Shapes.At(0).Radius
}
