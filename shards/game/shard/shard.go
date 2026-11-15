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
	// Content: loaded strictly and checked once, so bad content fails the boot.
	c, err := loadCatalog(fsys)
	if err != nil {
		panic(err)
	}

	// Map (Init, then PreUpdate first): the live map is set before any system reads it.
	world.RegisterPlugin(c.levels)

	// Every type a system uses, before StartGame.
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

	// Physics plugin: built here so areas can query it, registered after every mover.
	physicsPlugin := physics.NewPlugin()

	// Input, cast, move (PreUpdate, after the map): the predictor's step order, before the physics plugin.
	inputMetrics, err := player.NewInputMetrics(meters)
	if err != nil {
		panic(err)
	}
	world.RegisterSystem(&player.InputSystem{Tuning: c.movement, Metrics: inputMetrics},
		cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&ability.CastSystem{Tuning: c.abilities}, cardinal.WithHook(cardinal.PreUpdate))
	world.RegisterSystem(&player.MovementSystem{Tuning: c.movement, Levels: c.levels},
		cardinal.WithHook(cardinal.PreUpdate))

	// Echo replay (PreUpdate, after movement, before the physics plugin).
	world.RegisterSystem(&echo.ReplaySystem{}, cardinal.WithHook(cardinal.PreUpdate))

	// Physics plugin (PreUpdate, after every mover).
	world.RegisterPlugin(physicsPlugin)

	// Dev roster (Init): replace with command-driven spawning.
	world.RegisterSystem(&player.DevSpawnSystem{Tuning: c.movement, Levels: c.levels},
		cardinal.WithHook(cardinal.Init))

	// Echo spawn (Update): reads PreUpdate's BodyClaimed.
	world.RegisterSystem(&echo.SpawnSystem{Tuning: c.echoes})

	// Scene (Update, after echo.SpawnSystem, before every judge).
	scene := past.NewScene(c.history.Window)
	world.RegisterSystem(&history.SceneSystem{Scene: scene})

	// Ability hooks (Update, before every receiver of their intents).
	world.RegisterSystem(&ability.CastHookSystem{Tuning: c.abilities})

	// Projectiles (Update): launch, then fly, so a shot moves the tick it launches.
	world.RegisterSystem(&projectile.LaunchSystem{Tuning: c.shots})
	world.RegisterSystem(&projectile.FlightSystem{Tuning: c.shots, Scene: scene, Levels: c.levels})

	// Areas (Update).
	world.RegisterSystem(&area.AreaSystem{Tuning: c.areas, Physics: physicsPlugin})

	// Combat (Update, after every DamageIntent and HealIntent emitter).
	world.RegisterSystem(&combat.DamageResolveSystem{})

	// Elimination (Update, after damage resolves, before effects).
	world.RegisterSystem(&echo.EliminationSystem{Tuning: c.echoes})

	// Effects (Update, after every EffectIntent emitter and status reader).
	world.RegisterSystem(&effect.ResolveSystem{Tuning: c.effects})

	// PostUpdate reads the tick's final state: a new writer goes in Update.
	post := cardinal.WithHook(cardinal.PostUpdate)

	// Histories (PostUpdate, first).
	world.RegisterSystem(&history.RecordSystem{Window: c.history.Window}, post)

	// Client sync (PostUpdate): each feature sends each player this tick's final state it may see.
	world.RegisterSystem(&player.SyncSystem{Tuning: c.movement, SimVersion: echoimage.SimVersion()}, post)
	world.RegisterSystem(&echo.SyncSystem{}, post)
	world.RegisterSystem(&ability.SyncSystem{Tuning: c.abilities}, post)
	world.RegisterSystem(&projectile.SyncSystem{}, post)
	world.RegisterSystem(&history.SyncSystem{}, post)

	// Deaths (PostUpdate, last, after each sync).
	world.RegisterSystem(&combat.DeathSystem{}, post)
}
