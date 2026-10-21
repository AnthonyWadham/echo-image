package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/team"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     physics.ServerTickRateHz,
		SnapshotRate: 10,
	})
	if err != nil {
		panic(err.Error())
	}
	register(world)
	world.StartGame()
}

// register installs the plugins, game types and systems.
func register(world *cardinal.World) {
	// Data plugin: loads content/<feature>/*.json into a typed catalog at init; systems read it
	// via data.Get[T](dataPlugin). Register every kind BEFORE RegisterPlugin so it's in the catalog
	// before the tick loop starts.
	dataPlugin := data.NewPlugin(data.Config{EmbeddedFS: content.FS})
	data.Register[combat.CombatTuning](dataPlugin)
	data.Register[player.MovementTuning](dataPlugin)
	data.Register[echo.EchoTuning](dataPlugin)
	data.Register[ability.AbilityTuning](dataPlugin)
	data.Register[projectile.ProjectileTuning](dataPlugin)
	world.RegisterPlugin(dataPlugin)

	// Every component, command, event and system event a system uses must be registered before
	// StartGame; an unregistered one panics on first use.
	team.RegisterTypes(world)
	player.RegisterTypes(world)
	echo.RegisterTypes(world)
	ability.RegisterTypes(world)
	projectile.RegisterTypes(world)
	combat.RegisterTypes(world)

	// Input (PreUpdate, first): queues Input frames and applies one per Body per tick, and emits
	// BodyClaimed the first tick each Body is claimed. Movement then sets velocity from that frame.
	// Both run BEFORE the physics plugin, so a frame moves the Body the same tick its ack is sent.
	world.RegisterSystem(&player.InputSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&player.MovementSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.PreUpdate))

	// Physics plugin: PreUpdate step produces contact/trigger events each tick. Kept for queries.
	physicsPlugin := physics.Register(world)

	// Players: dev scaffolding spawns two players (a 1v1) at init. Replace with command-driven
	// spawning when matchmaking/clients land.
	world.RegisterSystem(&player.DevSpawnSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.Init))

	// Echo spawn (Update): give a newly-claimed Body its one Echo carrying its Health. BodyClaimed from
	// PreUpdate is still visible (system events clear at tick end). Claim-triggered, not Init — see
	// echo.SpawnSystem.
	world.RegisterSystem(&echo.SpawnSystem{Data: dataPlugin})

	// Echo replay (Update): record the Body's post-step pose and drive the Echo to the pose from
	// delay_seconds ago, ticking down each Echo's anti-camp warmup. Registered before projectile and
	// AntiCampSystem so the Echo's transform and warmup are current when they read them.
	world.RegisterSystem(&echo.ReplaySystem{})

	// Abilities (Update): equip a newly-claimed Body's kit, then cast from this tick's input frame.
	// Each cast emits AbilityCast, which the features it changes read the same tick.
	world.RegisterSystem(&ability.EquipSystem{Data: dataPlugin})
	world.RegisterSystem(&ability.CastSystem{Data: dataPlugin})

	// Projectiles (Update): launch a shot for each cast that has a launcher, then move every shot and
	// resolve its first hit. A shot launched this tick moves this tick, so its path starts at the shooter.
	world.RegisterSystem(&projectile.LaunchSystem{Data: dataPlugin})
	world.RegisterSystem(&projectile.FlightSystem{Physics: physicsPlugin})

	// Combat (Update). Order matters: every DamageIntent emitter (FlightSystem, AntiCampSystem)
	// runs before DamageResolveSystem drains them the same tick — system events are cleared at tick end.
	world.RegisterSystem(&combat.AntiCampSystem{Data: dataPlugin})
	world.RegisterSystem(&combat.DamageResolveSystem{})

	// Client sync (Update, last): each feature sends each player this tick's final state it may see.
	world.RegisterSystem(&player.SyncSystem{})
	world.RegisterSystem(&echo.SyncSystem{})
	world.RegisterSystem(&ability.SyncSystem{})
	world.RegisterSystem(&projectile.SyncSystem{})
}
