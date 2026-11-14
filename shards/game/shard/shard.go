package shard

import (
	"io/fs"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	echoimage "echo_image"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/area"
	"echo_image/shards/game/internal/combat"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect"
	"echo_image/shards/game/internal/history"
	"echo_image/shards/game/internal/history/past"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// Register installs the plugins, game types and systems.
func Register(world *cardinal.World) { RegisterWith(world, content.FS, otel.GetMeterProvider()) }

// RegisterWith is Register with the content in fsys and the game's metrics in meters, so a test can
// change content or read the metrics.
func RegisterWith(world *cardinal.World, fsys fs.FS, meters metric.MeterProvider) {
	// Content: each file loaded strictly and checked once, here, so bad content fails the boot. Each
	// system that reads a file holds it.
	c, err := loadCatalog(fsys)
	if err != nil {
		panic(err)
	}

	// Map (Init, and PreUpdate first): makes the map state names live before any system reads it.
	world.RegisterPlugin(c.levels)

	// Every component, command, event and system event a system uses must be registered before
	// StartGame; an unregistered one panics on first use.
	ref.RegisterTypes(world)
	team.RegisterTypes(world)
	player.RegisterTypes(world)
	echo.RegisterTypes(world)
	ability.RegisterTypes(world)
	projectile.RegisterTypes(world)
	history.RegisterTypes(world)
	combat.RegisterTypes(world)
	effect.RegisterTypes(world)
	area.RegisterTypes(world)

	// Physics plugin: built here so areas can query it, registered after movement. It holds Bodies and
	// Echoes; the map's walls are the map plugin's.
	physicsPlugin := physics.NewPlugin()

	// Input (PreUpdate, first): queues Input frames and applies one per Body per tick, and emits
	// BodyClaimed the first tick each Body is claimed. Abilities then give a newly-claimed Body its
	// cooldowns and cast from its champion's kit; each cast emits AbilityCast, whose hook runs the same
	// tick. Movement then moves each Body by the frame and its effects, stopping at walls, so a frame
	// moves the Body the same tick its ack is sent. Input, cast, move is the order a client predictor
	// steps; effects a cast starts apply from the next tick. All run BEFORE the physics plugin, so its
	// step sees each Body where it moved to this tick.
	inputMetrics, err := player.NewInputMetrics(meters)
	if err != nil {
		panic(err)
	}
	world.RegisterSystem(&player.InputSystem{Tuning: c.movement, Metrics: inputMetrics},
		cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.AttachAbilitiesSystem{}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.CastSystem{Tuning: c.abilities}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&player.MovementSystem{Tuning: c.movement, Levels: c.levels},
		cardinal.WithHook(cardinal.PreUpdate))

	// Echo replay (PreUpdate, after Movement): record the Body's pose and drive the Echo to the pose
	// from delay_seconds ago. Before the physics plugin, so Box2D holds this tick's Echo poses when
	// shots and areas query it.
	world.RegisterSystem(&echo.ReplaySystem{}, cardinal.WithHook(cardinal.PreUpdate))

	// Physics plugin: PreUpdate step produces contact/trigger events each tick. Kept for queries.
	world.RegisterPlugin(physicsPlugin)

	// Players: dev scaffolding spawns the dev roster (a 1v1) at init, on the live map's spawns. Replace
	// with command-driven spawning when matchmaking/clients land.
	world.RegisterSystem(&player.DevSpawnSystem{Tuning: c.movement, Levels: c.levels},
		cardinal.WithHook(cardinal.Init))

	// Echo spawn (Update): give a newly-claimed Body its one Echo carrying its Health. BodyClaimed from
	// PreUpdate is still visible (system events clear at tick end). Claim-triggered, not Init — see
	// echo.SpawnSystem.
	world.RegisterSystem(&echo.SpawnSystem{Tuning: c.echoes})

	// Scene (Update, after echo.SpawnSystem, before every judge): every recorded thing over the window, and
	// each client's view, once a tick. Shots, and every later judge, query it.
	scene := past.NewScene(c.history.Window)
	world.RegisterSystem(&history.SceneSystem{Scene: scene})

	// Ability hooks (Update): each cast runs its ability's hook, which asks the features that own the
	// state for changes with intents (a shot to launch, a dash's forced move). Before every receiver of
	// those intents.
	world.RegisterSystem(&ability.CastHookSystem{Tuning: c.abilities})

	// Projectiles (Update): launch a shot for each LaunchIntent, then move every shot and resolve what
	// it meets by its rules. A shot launched this tick moves this tick, so its path starts at the
	// shooter.
	world.RegisterSystem(&projectile.LaunchSystem{Tuning: c.shots})
	world.RegisterSystem(&projectile.FlightSystem{Tuning: c.shots, Scene: scene, Levels: c.levels})

	// Areas (Update): each area acts on the things overlapping its holder, by its rules. An Echo's
	// anti-camp is one.
	world.RegisterSystem(&area.AreaSystem{Tuning: c.areas, Physics: physicsPlugin})

	// Combat (Update). Order matters: every DamageIntent and HealIntent emitter (FlightSystem,
	// AreaSystem) runs before DamageResolveSystem drains them the same tick — system events are
	// cleared at tick end.
	world.RegisterSystem(&combat.DamageResolveSystem{})

	// Elimination (Update, after damage resolves, before effects): a player whose Echo died is out for
	// the round. It reads Died, which DamageResolveSystem emits.
	world.RegisterSystem(&echo.EliminationSystem{Tuning: c.echoes})

	// Effects (Update, after every EffectIntent emitter and every system that reads status): the one
	// writer of Effects. What it adds applies from the next tick, so a whole tick reads one status.
	world.RegisterSystem(&effect.ResolveSystem{Tuning: c.effects})

	// PostUpdate reads the tick's final state: every Update writer has run, so no Update order can put
	// a reader before one. A new writer goes in Update; a new recorder or sync goes here, before
	// DeathSystem, the one system here that changes the world.
	post := cardinal.WithHook(cardinal.PostUpdate)

	// Histories (PostUpdate, first): each records what this tick's syncs show of it, so a predicted shot
	// meets it at its client's view.
	world.RegisterSystem(&history.RecordSystem{Window: c.history.Window}, post)

	// Client sync (PostUpdate): each feature sends each player this tick's final state it may see.
	world.RegisterSystem(&player.SyncSystem{Tuning: c.movement, SimVersion: echoimage.SimVersion()}, post)
	world.RegisterSystem(&echo.SyncSystem{}, post)
	world.RegisterSystem(&ability.SyncSystem{Tuning: c.abilities}, post)
	world.RegisterSystem(&projectile.SyncSystem{}, post)
	world.RegisterSystem(&history.SyncSystem{}, post)

	// Deaths (PostUpdate, last): what died this tick goes, after each sync sent it once more.
	world.RegisterSystem(&combat.DeathSystem{}, post)
}
