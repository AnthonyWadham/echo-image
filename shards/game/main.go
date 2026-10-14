package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
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
	world.RegisterPlugin(dataPlugin)

	// Every component, command and system event a system uses must be registered before StartGame;
	// an unregistered one panics on first use.
	team.RegisterTypes(world)
	player.RegisterTypes(world)
	echo.RegisterTypes(world)
	combat.RegisterTypes(world)
	netsync.RegisterTypes(world)

	// Movement (PreUpdate): queues Move frames and applies one per Body per tick, and emits BodyClaimed
	// the first tick each Body is claimed. Registered BEFORE the physics plugin so it runs before the
	// step, and a frame moves the Body the same tick its ack is sent.
	world.RegisterSystem(&player.MovementSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.PreUpdate))

	// Physics plugin: PreUpdate step produces contact/trigger events each tick.
	physics.Register(world)

	// Players: dev scaffolding spawns two players (a 1v1) at init. Replace with command-driven
	// spawning when matchmaking/clients land.
	world.RegisterSystem(&player.DevSpawnSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.Init))

	// Echo spawn (Update): give a newly-claimed Body its one Echo carrying its Health. BodyClaimed from
	// PreUpdate is still visible (system events clear at tick end). Claim-triggered, not Init — see
	// echo.SpawnSystem.
	world.RegisterSystem(&echo.SpawnSystem{Data: dataPlugin})

	// Echo replay (Update): record the Body's post-step pose and drive the Echo to the pose from
	// delay_seconds ago, ticking down each Echo's anti-camp warmup. Registered before AntiCampSystem so
	// the Echo's transform and warmup are current when anti-camp reads them.
	world.RegisterSystem(&echo.ReplaySystem{})

	// Combat pipeline (Update). Order matters: the emitters (CollisionSystem, AntiCampSystem) run before
	// DamageResolveSystem drains their intents the same tick — system events are cleared at tick end.
	world.RegisterSystem(&combat.CollisionSystem{Data: dataPlugin})
	world.RegisterSystem(&combat.AntiCampSystem{Data: dataPlugin})
	world.RegisterSystem(&combat.DamageResolveSystem{})

	// Net sync (Update, last): send each player this tick's final state.
	world.RegisterSystem(&netsync.WorldStateSystem{})
}
