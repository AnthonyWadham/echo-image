// Package demo holds a throwaway spawn scene that exercises the physics → collision → damage
// pipeline on world init, with no client connected. Delete this package once real spawners exist.
package demo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/physics"
)

// enemyRow is a damageable physics body: the physics2d trio plus Health.
type enemyRow struct {
	Transform cardinal.Ref[physics2d.Transform2D]
	Velocity  cardinal.Ref[physics2d.Velocity2D]
	Body      cardinal.Ref[physics2d.PhysicsBody2D]
	Health    cardinal.Ref[combat.Health]
}

// projectileRow is a plain physics body (no Health — projectiles deal damage, don't take it).
type projectileRow struct {
	Transform cardinal.Ref[physics2d.Transform2D]
	Velocity  cardinal.Ref[physics2d.Velocity2D]
	Body      cardinal.Ref[physics2d.PhysicsBody2D]
}

// SpawnSystemState spawns a self-contained scene on world init so the physics → collision →
// damage pipeline runs end to end without a client connected.
type SpawnSystemState struct {
	cardinal.BaseSystemState
	Enemies     cardinal.Contains[enemyRow]
	Projectiles cardinal.Contains[projectileRow]
}

// SpawnSystem places a stationary enemy at the origin and fires a sensor projectile at it from
// the left. Within a few ticks the projectile overlaps the enemy, which drives:
//
//	physics2d TriggerBegin -> combat.CollisionSystem (DamageIntent) -> combat.DamageResolveSystem
//
// Enemy HP is set low so the single hit kills it and you see the death log too.
func SpawnSystem(state *SpawnSystemState) {
	_, enemy := state.Enemies.Create()
	enemy.Transform.Set(physics2d.Transform2D{Position: physics2d.Vec2{X: 0, Y: 0}})
	enemy.Velocity.Set(physics2d.Velocity2D{})
	enemy.Body.Set(physics.NewBody2D(physics2d.BodyTypeKinematic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       0.5,
		Density:      1,
		CategoryBits: physics.CategoryEnemy,
		MaskBits:     physics.CategoryProjectile | physics.CategoryPlayer,
	}))
	enemy.Health.Set(combat.Health{HP: 20, MaxHP: 20})

	_, proj := state.Projectiles.Create()
	proj.Transform.Set(physics2d.Transform2D{Position: physics2d.Vec2{X: -5, Y: 0}})
	proj.Velocity.Set(physics2d.Velocity2D{Linear: physics2d.Vec2{X: 6, Y: 0}})
	proj.Body.Set(physics.NewBody2D(physics2d.BodyTypeDynamic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       0.2,
		Density:      1,
		IsSensor:     true, // pass through and report an overlap instead of physically blocking
		CategoryBits: physics.CategoryProjectile,
		MaskBits:     physics.CategoryEnemy,
	}))
	// No Bullet/CCD here: Box2D CCD only sweeps solid bodies, while sensor overlaps are sampled at
	// discrete post-step positions — so CCD can't stop a fast sensor from tunnelling anyway. This
	// works because movement per step (6*(1/20)=0.3) is well under the combined radius (0.7). For a
	// genuinely fast projectile use physics2d.CircleSweep for a swept overlap, or a solid bullet body.
}
