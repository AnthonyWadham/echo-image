package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	echoimage "echo_image"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/area"
	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/ref"
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
func register(world *cardinal.World) { registerWith(world, nil) }

// registerWith is register with content from src, or from the data plugin's own pick when src is nil,
// so a test can change content.
func registerWith(world *cardinal.World, src data.Source) {
	// Data plugin: loads content/<feature>/*.json into a typed catalog at init; systems read it
	// via data.Get[T](dataPlugin). Register every kind BEFORE RegisterPlugin so it's in the catalog
	// before the tick loop starts.
	dataPlugin := data.NewPlugin(data.Config{EmbeddedFS: content.FS, Source: src})
	data.Register[player.MovementTuning](dataPlugin)
	data.Register[echo.EchoTuning](dataPlugin)
	data.Register[ability.AbilityTuning](dataPlugin)
	data.Register[projectile.ProjectileTuning](dataPlugin)
	data.Register[effect.EffectTuning](dataPlugin)
	data.Register[area.AreaTuning](dataPlugin)
	world.RegisterPlugin(dataPlugin)

	// Every component, command, event and system event a system uses must be registered before
	// StartGame; an unregistered one panics on first use.
	ref.RegisterTypes(world)
	team.RegisterTypes(world)
	player.RegisterTypes(world)
	echo.RegisterTypes(world)
	ability.RegisterTypes(world)
	projectile.RegisterTypes(world)
	combat.RegisterTypes(world)
	effect.RegisterTypes(world)
	area.RegisterTypes(world)

	// Physics plugin: built here so movement can query it, registered after movement.
	physicsPlugin := physics.NewPlugin()

	// Input (PreUpdate, first): queues Input frames and applies one per Body per tick, and emits
	// BodyClaimed the first tick each Body is claimed. Abilities then give a newly-claimed Body its
	// cooldowns and cast from its champion's kit; each cast emits AbilityCast, whose hook runs the same
	// tick. Movement then moves each Body by the frame and its effects, stopping at walls, so a frame
	// moves the Body the same tick its ack is sent. Input, cast, move is the order a client predictor
	// steps; effects a cast starts apply from the next tick. All run BEFORE the physics plugin, so its
	// step sees each Body where it moved to this tick.
	world.RegisterSystem(&player.InputSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.EquipSystem{}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.CastSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&player.MovementSystem{Data: dataPlugin, Physics: physicsPlugin},
		cardinal.WithHook(cardinal.PreUpdate))

	// Echo replay (PreUpdate, after Movement): record the Body's pose and drive the Echo to the pose
	// from delay_seconds ago. Before the physics plugin, so Box2D holds this tick's Echo poses when
	// shots and areas query it.
	world.RegisterSystem(&echo.ReplaySystem{}, cardinal.WithHook(cardinal.PreUpdate))

	// Physics plugin: PreUpdate step produces contact/trigger events each tick. Kept for queries.
	world.RegisterPlugin(physicsPlugin)

	// Players: dev scaffolding spawns two players (a 1v1) at init. Replace with command-driven
	// spawning when matchmaking/clients land.
	world.RegisterSystem(&player.DevSpawnSystem{Data: dataPlugin}, cardinal.WithHook(cardinal.Init))

	// Echo spawn (Update): give a newly-claimed Body its one Echo carrying its Health. BodyClaimed from
	// PreUpdate is still visible (system events clear at tick end). Claim-triggered, not Init — see
	// echo.SpawnSystem.
	world.RegisterSystem(&echo.SpawnSystem{Data: dataPlugin})

	// Ability hooks (Update): each cast runs its ability's hook, which asks the features that own the
	// state for changes with intents (a shot to launch, a dash's forced move). Before every receiver of
	// those intents.
	world.RegisterSystem(&ability.CastHookSystem{Data: dataPlugin})

	// Projectiles (Update): launch a shot for each LaunchIntent, then move every shot and resolve what
	// it meets by its rules. A shot launched this tick moves this tick, so its path starts at the
	// shooter.
	world.RegisterSystem(&projectile.LaunchSystem{Data: dataPlugin})
	world.RegisterSystem(&projectile.FlightSystem{Data: dataPlugin, Physics: physicsPlugin})

	// Areas (Update): each area acts on the things overlapping its holder, by its rules. An Echo's
	// anti-camp is one.
	world.RegisterSystem(&area.AreaSystem{Data: dataPlugin, Physics: physicsPlugin})

	// Combat (Update). Order matters: every DamageIntent and HealIntent emitter (FlightSystem,
	// AreaSystem) runs before DamageResolveSystem drains them the same tick — system events are
	// cleared at tick end.
	world.RegisterSystem(&combat.DamageResolveSystem{})

	// Effects (Update, after every EffectIntent emitter and every system that reads status): the one
	// writer of Effects. What it adds applies from the next tick, so a whole tick reads one status.
	world.RegisterSystem(&effect.ResolveSystem{Data: dataPlugin})

	// Target history (Update, after everything that moves, kills or tags a Body or Echo, before the
	// syncs): each saves what this tick's syncs show of it, so a predicted shot meets it at its client's view.
	world.RegisterSystem(&projectile.HistorySystem{Data: dataPlugin})

	// Client sync (Update): each feature sends each player this tick's final state it may see.
	world.RegisterSystem(&player.SyncSystem{Data: dataPlugin, SimVersion: echoimage.SimVersion(physics.ServerTickRateHz)})
	world.RegisterSystem(&echo.SyncSystem{})
	world.RegisterSystem(&ability.SyncSystem{Data: dataPlugin})
	world.RegisterSystem(&projectile.SyncSystem{})
	world.RegisterSystem(&projectile.TargetSyncSystem{})

	// Deaths (Update, last): what died this tick goes, after each sync sent it once more.
	world.RegisterSystem(&combat.DeathSystem{})
}
