package main

import (
	"io/fs"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	echoimage "echo_image"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/area"
	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/timestep"
	"echo_image/shards/game/internal/player"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     physics.ServerTickRateHz,
		SnapshotRate: uint32(timestep.Ticks(0.5)), // ticks between snapshots: half a second at any rate
	})
	if err != nil {
		panic(err.Error())
	}
	register(world)
	world.StartGame()
}

// register installs the plugins, game types and systems.
func register(world *cardinal.World) { registerWith(world, content.FS) }

// registerWith is register with the content in fsys, so a test can change content.
func registerWith(world *cardinal.World, fsys fs.FS) {
	// Content: each file loaded strictly and checked once, here, so bad content fails the boot. Each
	// system that reads a file holds it.
	c, err := loadCatalog(fsys)
	if err != nil {
		panic(err)
	}

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
	world.RegisterSystem(&player.InputSystem{Tuning: c.movement}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.EquipSystem{}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.CastSystem{Tuning: c.abilities}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&player.MovementSystem{Tuning: c.movement, Physics: physicsPlugin},
		cardinal.WithHook(cardinal.PreUpdate))

	// Echo replay (PreUpdate, after Movement): record the Body's pose and drive the Echo to the pose
	// from delay_seconds ago. Before the physics plugin, so Box2D holds this tick's Echo poses when
	// shots and areas query it.
	world.RegisterSystem(&echo.ReplaySystem{}, cardinal.WithHook(cardinal.PreUpdate))

	// Physics plugin: PreUpdate step produces contact/trigger events each tick. Kept for queries.
	world.RegisterPlugin(physicsPlugin)

	// Players: dev scaffolding spawns two players (a 1v1) at init. Replace with command-driven
	// spawning when matchmaking/clients land.
	world.RegisterSystem(&player.DevSpawnSystem{Tuning: c.movement}, cardinal.WithHook(cardinal.Init))

	// Echo spawn (Update): give a newly-claimed Body its one Echo carrying its Health. BodyClaimed from
	// PreUpdate is still visible (system events clear at tick end). Claim-triggered, not Init — see
	// echo.SpawnSystem.
	world.RegisterSystem(&echo.SpawnSystem{Tuning: c.echoes})

	// Ability hooks (Update): each cast runs its ability's hook, which asks the features that own the
	// state for changes with intents (a shot to launch, a dash's forced move). Before every receiver of
	// those intents.
	world.RegisterSystem(&ability.CastHookSystem{Tuning: c.abilities})

	// Projectiles (Update): launch a shot for each LaunchIntent, then move every shot and resolve what
	// it meets by its rules. A shot launched this tick moves this tick, so its path starts at the
	// shooter.
	world.RegisterSystem(&projectile.LaunchSystem{Tuning: c.shots})
	world.RegisterSystem(&projectile.FlightSystem{Tuning: c.shots, Physics: physicsPlugin})

	// Areas (Update): each area acts on the things overlapping its holder, by its rules. An Echo's
	// anti-camp is one.
	world.RegisterSystem(&area.AreaSystem{Tuning: c.areas, Physics: physicsPlugin})

	// Combat (Update). Order matters: every DamageIntent and HealIntent emitter (FlightSystem,
	// AreaSystem) runs before DamageResolveSystem drains them the same tick — system events are
	// cleared at tick end.
	world.RegisterSystem(&combat.DamageResolveSystem{})

	// Effects (Update, after every EffectIntent emitter and every system that reads status): the one
	// writer of Effects. What it adds applies from the next tick, so a whole tick reads one status.
	world.RegisterSystem(&effect.ResolveSystem{Tuning: c.effects})

	// Target history (Update, after everything that moves, kills or tags a Body or Echo, before the
	// syncs): each saves what this tick's syncs show of it, so a predicted shot meets it at its client's view.
	world.RegisterSystem(&projectile.HistorySystem{Tuning: c.shots})

	// Client sync (Update): each feature sends each player this tick's final state it may see.
	world.RegisterSystem(&player.SyncSystem{Tuning: c.movement, SimVersion: echoimage.SimVersion(physics.ServerTickRateHz)})
	world.RegisterSystem(&echo.SyncSystem{})
	world.RegisterSystem(&ability.SyncSystem{Tuning: c.abilities})
	world.RegisterSystem(&projectile.SyncSystem{})
	world.RegisterSystem(&projectile.TargetSyncSystem{})

	// Deaths (Update, last): what died this tick goes, after each sync sent it once more.
	world.RegisterSystem(&combat.DeathSystem{})
}
