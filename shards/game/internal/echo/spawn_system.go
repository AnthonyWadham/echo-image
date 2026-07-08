package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// bodyRow is a player Body an Echo shadows: identity (Player marks Body-only archetypes and gives the
// PlayerID), team (copied onto the Echo for friendly-fire), and pose (the Echo's spawn point + first
// recorded frame).
type bodyRow struct {
	Player    cardinal.Ref[playercomp.Player]
	Team      cardinal.Ref[team.Team]
	Transform cardinal.Ref[physics2d.Transform2D]
}

// echoRow is the Echo entity: identity + trail, the Health it carries, team, and the physics2d trio.
// BodyTypeManual means the replay owns the pose and Box2D is used only to detect hits against it.
type echoRow struct {
	Echo      cardinal.Ref[component.Echo]
	History   cardinal.Ref[component.PathHistory]
	Health    cardinal.Ref[combatcomp.Health]
	Team      cardinal.Ref[team.Team]
	Transform cardinal.Ref[physics2d.Transform2D]
	Velocity  cardinal.Ref[physics2d.Velocity2D]
	Body      cardinal.Ref[physics2d.PhysicsBody2D]
}

// SpawnSystemState gives every player Body one Echo at world init.
type SpawnSystemState struct {
	cardinal.BaseSystemState
	Bodies cardinal.Contains[bodyRow]
	Echoes cardinal.Contains[echoRow]
}

// SpawnSystem creates one Echo per Body, once, on Init (right after DevSpawnSystem). It is deliberately
// NOT reactive-on-Update: a presence check ("Body has no Echo → spawn one") would RESURRECT a dead Echo
// the tick after DamageResolveSystem destroys it — the invincible Body survives, so it would look
// unpaired and get a fresh full-HP Echo, defeating "Echo death = elimination" (GDD §8). Init runs once
// on a fresh world and NOT on snapshot restore (restore reconstructs the Echoes from state), so there's
// no resurrection and no re-spawn.
//
// TODO(matchmaking): when Bodies can join mid-match, Echo spawn + elimination need a dedicated lifecycle
// (spawn on join; a dead Echo must stay dead) — presence-reactivity is the wrong trigger for it.
func SpawnSystem(state *SpawnSystemState) {
	// Collect Body ids first, then create — don't mutate archetype storage while iterating the search.
	var bodies []cardinal.EntityID
	for bodyEID := range state.Bodies.Iter() {
		bodies = append(bodies, bodyEID)
	}
	if len(bodies) == 0 {
		return
	}

	tuning := data.Get[EchoTuning]() // catalog loads eagerly at RegisterPlugin, so this is valid on Init
	for _, bodyEID := range bodies {
		b, err := state.Bodies.GetByID(bodyEID)
		if err != nil {
			continue
		}
		state.spawnEcho(tuning, bodyEID, b)
	}
}

func (state *SpawnSystemState) spawnEcho(tuning EchoTuning, bodyEID cardinal.EntityID, b bodyRow) {
	pose := b.Transform.Get()
	playerID := b.Player.Get().ID
	allegiance := b.Team.Get()
	delay := tuning.DelayTicks()

	// Size the trail to exactly the configured delay (delay+1 so the read slot never collides with the
	// write slot) and prefill it with the spawn pose: for the first delay window the Echo sits at spawn
	// ("you were at spawn N seconds ago") instead of snapping in from the origin.
	frame := component.EchoFrame{Pose: pose}
	hist := component.PathHistory{Poses: make([]component.EchoFrame, delay+1)}
	for i := range hist.Poses {
		hist.Poses[i] = frame
	}

	_, e := state.Echoes.Create()
	// Warmup = trail length: a full buffer of real frames must arrive before the read slot stops
	// yielding the prefilled spawn pose, so anti-camp (M3) stays off until then (see component.Echo).
	e.Echo.Set(component.Echo{Owner: bodyEID, PlayerID: playerID, Warmup: len(hist.Poses)})
	e.History.Set(hist)
	e.Health.Set(combatcomp.Health{HP: tuning.MaxHP, MaxHP: tuning.MaxHP})
	e.Team.Set(allegiance)
	e.Transform.Set(pose)
	e.Velocity.Set(physics2d.Velocity2D{})
	e.Body.Set(physics.NewBody2D(physics2d.BodyTypeManual, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       tuning.HitRadius,
		Density:      1,
		CategoryBits: physics.CategoryEcho,
		MaskBits:     physics.CategoryProjectile, // takes projectile hits; anti-camp is a gameplay distance check
	}))

	state.Logger().Info().
		Uint32("owner", uint32(bodyEID)).Uint32("player", playerID).
		Float64("x", pose.Position.X).Float64("y", pose.Position.Y).
		Msg("echo spawned")
}
