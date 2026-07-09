package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     physics.ServerTickRateHz,
		SnapshotRate: 10,
	})
	if err != nil {
		panic(err.Error())
	}

	// Data plugin: loads content/<feature>/*.json into a typed catalog at init; systems read it
	// via data.Get[T](). Register every kind BEFORE RegisterPlugin so it's in the catalog before
	// the tick loop starts.
	dataPlugin := data.NewPlugin(data.Config{EmbeddedFS: content.FS})
	data.Register[combat.CombatTuning](dataPlugin)
	data.Register[player.MovementTuning](dataPlugin)
	data.Register[echo.EchoTuning](dataPlugin)
	cardinal.RegisterPlugin(world, dataPlugin)

	// Physics plugin: PreUpdate step produces contact/trigger events each tick.
	physics.Register(world)

	// Players: dev scaffolding spawns two players (a 1v1) at init. Replace with command-driven
	// spawning when matchmaking/clients land.
	cardinal.RegisterSystem(world, player.DevSpawnSystem, cardinal.WithHook(cardinal.Init))
	// Movement (Update): drains Move commands, drives the sender's Body velocity, and emits BodyClaimed
	// the first tick each Body is claimed.
	cardinal.RegisterSystem(world, player.MovementSystem)

	// Echo spawn (Update, right after Movement so this tick's BodyClaimed is still visible): give a
	// newly-claimed Body its one Echo carrying its Health. Claim-triggered, not Init — see echo.SpawnSystem.
	cardinal.RegisterSystem(world, echo.SpawnSystem)

	// Echo replay (Update): record the Body's post-step pose and drive the Echo to the pose from
	// delay_seconds ago, ticking down each Echo's anti-camp warmup. Registered before AntiCampSystem so
	// the Echo's transform and warmup are current when anti-camp reads them.
	cardinal.RegisterSystem(world, echo.ReplaySystem)

	// Combat pipeline (Update). Order matters: the emitters (CollisionSystem, AntiCampSystem) run before
	// DamageResolveSystem drains their intents the same tick — system events are cleared at tick end.
	cardinal.RegisterSystem(world, combat.CollisionSystem)
	cardinal.RegisterSystem(world, combat.AntiCampSystem)
	cardinal.RegisterSystem(world, combat.DamageResolveSystem)

	world.StartGame()
}
