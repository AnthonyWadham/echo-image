package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// bodyRow is a player Body an Echo shadows: identity (Player marks Body-only archetypes and gives the
// PlayerID), team (copied onto the Echo for friendly-fire), and pose (the Echo's spawn point + first
// recorded frame).
type bodyRow struct {
	Player    playercomp.Player
	Team      team.Team
	Transform physics2d.Transform2D
}

// echoRow is the Echo entity: identity + trail, the Health it carries, team, and the physics2d trio.
// BodyTypeManual means the replay owns the pose and Box2D is used only to detect hits against it.
type echoRow struct {
	Echo      component.Echo
	History   component.PathHistory
	Health    combatcomp.Health
	Team      team.Team
	Transform physics2d.Transform2D
	Velocity  physics2d.Velocity2D
	Body      physics2d.PhysicsBody2D
}

// SpawnSystem gives a Body its one Echo the tick it is first claimed — when player.MovementSystem
// applies the Body's first Move command and emits BodyClaimed. Registered on Update; MovementSystem
// emits in PreUpdate, and system events last until the tick ends, so the claim is visible here.
//
// Claim-triggered (not Init, not presence-reactive) for two reasons:
//   - No resurrection. A presence check ("Body has no Echo → spawn one") would hand a fresh full-HP Echo
//     to the invincible Body the tick after DamageResolveSystem destroys the old one, defeating "Echo
//     death = elimination" (GDD §8). BodyClaimed fires exactly once per Body (Persona is set once and
//     never cleared), so each Body gets exactly one Echo for the whole match, dead or alive.
//   - No pre-death. Spawning at Init gave every dev Body an Echo that then warmed up and self-destructed
//     to anti-camp DoT while the Body sat unclaimed at spawn — so a client connecting seconds later saw
//     no Echoes. Tying spawn to the claim means an Echo exists only once a live player drives its Body,
//     appearing fresh (full warmup) the moment they first move.
//
// On snapshot restore no BodyClaimed events replay (Bodies are already claimed), so Echoes are rebuilt
// from saved state and none re-spawn — the no-resurrection guarantee holds across restore too.
//
// TODO(matchmaking): when Bodies join mid-match this generalizes cleanly — spawn stays edge-triggered on
// the join/claim, and a dead Echo still stays dead.
type SpawnSystem struct {
	Data *data.Plugin
}

func (s *SpawnSystem) Run(w *cardinal.World) {
	bodies := w.Contains[bodyRow]()
	for claim := range w.SystemEvents[player.BodyClaimed]() {
		body, err := bodies.GetByID(claim.Body)
		if err != nil {
			continue // claimed Body vanished the same tick — nothing to shadow
		}
		// Read tuning only when a Body is actually claimed (rare) — this Update system runs every tick,
		// but the common no-claim tick does no work. Eagerly loaded at RegisterPlugin, so always valid.
		tuning := data.Get[EchoTuning](s.Data)
		spawnEcho(w, tuning, body)
	}
}

func spawnEcho(w *cardinal.World, tuning EchoTuning, body cardinal.Entity) {
	pose := body.Get[physics2d.Transform2D]()
	playerID := body.Get[playercomp.Player]().ID
	allegiance := body.Get[team.Team]()
	delay := tuning.DelayTicks()

	// Size the trail to exactly the configured delay (delay+1 so the read slot never collides with the
	// write slot) and prefill it with the spawn pose: for the first delay window the Echo sits at spawn
	// ("you were at spawn N seconds ago") instead of snapping in from the origin.
	hist := component.PathHistory{Poses: cardinal.SliceOf(component.EchoFrame{Pose: pose}).Repeat(delay + 1)}

	e := w.Create[echoRow]()
	// Warmup = trail length: a full buffer of real frames must arrive before the read slot stops
	// yielding the prefilled spawn pose, so anti-camp (M3) stays off until then (see component.Echo).
	e.Set(component.Echo{Owner: body.ID(), PlayerID: playerID, Warmup: hist.Poses.Len()})
	e.Set(hist)
	e.Set(combatcomp.Health{HP: tuning.MaxHP, MaxHP: tuning.MaxHP})
	e.Set(allegiance)
	e.Set(pose)
	e.Set(physics2d.Velocity2D{})
	e.Set(physics.NewBody2D(physics2d.BodyTypeManual, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       tuning.HitRadius,
		Density:      1,
		CategoryBits: physics.CategoryEcho,
		MaskBits:     physics.CategoryProjectile, // takes projectile hits; anti-camp is a gameplay distance check
	}))

	w.Logger().Info().
		Uint32("owner", uint32(body.ID())).Uint32("player", playerID).
		Float64("x", pose.Position.X).Float64("y", pose.Position.Y).
		Msg("echo spawned")
}
