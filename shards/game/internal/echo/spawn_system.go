package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	areacomp "echo_image/shards/game/internal/area/component"
	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	historycomp "echo_image/shards/game/internal/history/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// bodyRow is a player Body an Echo shadows: identity (Player marks Body-only archetypes and gives the
// PlayerID), champion (whose Echo it gets), team (copied onto the Echo for friendly-fire), and pose
// (the Echo's spawn point + first recorded frame).
type bodyRow struct {
	Player    playercomp.Player
	Champion  playercomp.Champion
	Team      team.Team
	Transform physics2d.Transform2D
}

// echoRow is the Echo entity: identity + trail, the Health it carries, team and owner, the physics2d
// trio, its effect list, and its history (the rows screens draw it from and shots meet it by).
// BodyTypeManual means the replay owns the pose; the collider is the hurtbox projectile tests shots
// against.
type echoRow struct {
	Spawn     ref.Spawn
	Echo      component.Echo
	Path      component.PathHistory
	Health    combatcomp.Health
	Team      team.Team
	Owner     team.Owner
	Transform physics2d.Transform2D
	Velocity  physics2d.Velocity2D
	Body      physics2d.PhysicsBody2D
	Effects   effectcomp.Effects
	History   historycomp.History
}

// SpawnSystem gives a Body its one Echo the tick it is first claimed — when player.InputSystem takes
// the Body's first Input command and emits BodyClaimed. Registered on Update; InputSystem emits in
// PreUpdate, and system events last until the tick ends, so the claim is visible here.
//
// Claim-triggered (not Init, not presence-reactive) for two reasons:
//   - No resurrection. A presence check ("Body has no Echo → spawn one") would hand a fresh full-HP Echo
//     to the invincible Body the tick after combat.DeathSystem removes the old one, defeating "Echo
//     death = out for the round" (GDD §8). BodyClaimed fires exactly once per Body (Persona is set once
//     and never cleared), so each Body gets exactly one Echo, dead or alive: a match is one round until
//     rounds exist.
//   - No pre-death. Spawning at Init gave every dev Body an Echo that then warmed up and self-destructed
//     to anti-camp DoT while the Body sat unclaimed at spawn — so a client connecting seconds later saw
//     no Echoes. Tying spawn to the claim means an Echo exists only once a live player drives its Body,
//     appearing fresh (its area not started yet) the moment they first move.
//
// On snapshot restore no BodyClaimed events replay (Bodies are already claimed), so Echoes are rebuilt
// from saved state and none re-spawn — the no-resurrection guarantee holds across restore too.
//
// TODO(matchmaking): when Bodies join mid-match this generalizes cleanly — spawn stays edge-triggered on
// the join/claim, and a dead Echo still stays dead.
type SpawnSystem struct {
	Tuning EchoTuning
}

func (s *SpawnSystem) Run(w *cardinal.World) {
	bodies := w.Contains[bodyRow]()
	for claim := range w.SystemEvents[playercomp.BodyClaimed]() {
		body, err := bodies.GetByID(claim.Body)
		if err != nil {
			continue // claimed Body vanished the same tick — nothing to shadow
		}
		// Read tuning only when a Body is actually claimed (rare) — this Update system runs every tick,
		// but the common no-claim tick does no work. Eagerly loaded at RegisterPlugin, so always valid.
		tuning := s.Tuning
		spawnEcho(w, tuning, body)
	}
}

func spawnEcho(w *cardinal.World, tuning EchoTuning, body cardinal.Entity) {
	champion := body.Get[playercomp.Champion]().ID
	def, ok := tuning.Champions[champion]
	if !ok {
		w.Logger().Error().Str("champion", champion).Msg("no Echo: the champion isn't in echo tuning")
		return
	}
	pose := body.Get[physics2d.Transform2D]()
	playerID := body.Get[playercomp.Player]().ID
	allegiance := body.Get[team.Team]()
	delay := def.DelayTicks()

	// Size the trail to exactly the champion's delay (delay+1 so the read slot never collides with the
	// write slot) and prefill it with the spawn pose: for the first delay window the Echo sits at spawn
	// ("you were at spawn N seconds ago") instead of snapping in from the origin.
	hist := component.PathHistory{Poses: cardinal.SliceOf(component.EchoFrame{Pose: pose}).Repeat(delay + 1)}

	e := w.Create[echoRow]()
	e.Set(ref.Spawn{Serial: ref.Next(w)})
	e.Set(component.Echo{PlayerID: playerID})
	e.Set(hist)
	e.Set(combatcomp.Health{HP: tuning.MaxHP, MaxHP: tuning.MaxHP})
	e.Set(allegiance)
	e.Set(team.Owner{Body: body.ID()})
	e.Set(pose)
	e.Set(physics2d.Velocity2D{})
	e.Set(historycomp.History{Predicted: true})
	e.Set(physics.NewBody2D(physics2d.BodyTypeManual, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       tuning.HitRadius,
		Density:      1,
		CategoryBits: uint64(interact.Echo),
		MaskBits:     uint64(interact.Shot | interact.Area), // shots read this circle, and areas query it
	}))
	// Its area acts once the trail holds real history. Until then the Echo replays the spawn pose, on
	// top of its Body.
	if def.Area != "" {
		e.Set(areacomp.Area{Kind: def.Area, Start: w.TickHeight() + uint64(delay)})
	}

	w.Logger().Info().
		Uint32("owner", uint32(body.ID())).Uint32("player", playerID).
		Float64("x", pose.Position.X).Float64("y", pose.Position.Y).
		Msg("echo spawned")
}
