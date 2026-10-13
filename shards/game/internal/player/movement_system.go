package player

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/player/component"
)

// movableRow is a player Body we can drive: identity (to match the command persona) + velocity.
type movableRow struct {
	Player   component.Player
	Velocity physics2d.Velocity2D
}

// MovementSystem applies each Move command to the Body owned by the command's authenticated persona,
// setting Velocity2D (kinematic Bodies move by velocity; physics writeback updates Transform2D). The
// velocity persists until the next command, so the client sends a zero vector to stop. It emits
// BodyClaimed the first tick each Body is claimed.
type MovementSystem struct {
	Data *data.Plugin
}

func (s *MovementSystem) Run(w *cardinal.World) {
	speed := data.Get[MovementTuning](s.Data).MoveSpeed // read config once at system entry, not per command
	movables := w.Contains[movableRow]()
	for cmd := range w.Commands[Move]() {
		persona, ok := cmd.Player()
		if !ok || persona == "" {
			// An empty persona matches any unclaimed Body in resolveBody and re-enters the claim branch
			// every tick (setting Persona="" is a no-op), re-emitting BodyClaimed and spawning a fresh
			// Echo each time. Only players move Bodies; drop shard-sent and malformed commands.
			w.Logger().Warn().Str("sender", cmd.Sender()).Msg("move: not from a player, ignoring command")
			continue
		}
		body, ok := resolveBody(movables, persona)
		if !ok {
			w.Logger().Warn().Str("persona", persona).Msg("move: no free player Body to control")
			continue
		}
		if p := body.Get[component.Player](); p.Persona == "" { // first command from this persona claims a free Body
			p.Persona = persona
			body.Set(p)
			// Edge-trigger echo.SpawnSystem: this branch runs once per Body (Persona is never cleared), so
			// each Body gets exactly one Echo (see echo.SpawnSystem for why spawn is claim-triggered).
			w.EmitSystemEvent(BodyClaimed{Body: body.ID()})
			w.Logger().Info().Str("persona", persona).Uint32("id", p.ID).Msg("player Body claimed")
		}
		vx, vy := normalize(cmd.Payload.DirX, cmd.Payload.DirY)
		body.Set(physics2d.Velocity2D{Linear: physics2d.Vec2{X: vx * speed, Y: vy * speed}})
	}
}

// resolveBody returns the Body owned by persona, or a free (unclaimed) Body if none is owned yet. The
// claim-on-first-command is a dev-spawn convenience so any MCP email can grab a slot; real
// matchmaking will assign personas at spawn and this falls away.
func resolveBody(movables cardinal.Search, persona string) (cardinal.Entity, bool) {
	var free cardinal.Entity
	haveFree := false
	for m := range movables.Iter() {
		switch p := m.Get[component.Player](); {
		case p.Persona == persona:
			return m, true
		case p.Persona == "" && !haveFree:
			free, haveFree = m, true
		}
	}
	return free, haveFree
}

// normalize returns the unit vector of (x, y), or (0, 0) when the input is ~zero (a stop command).
func normalize(x, y float64) (float64, float64) {
	mag := math.Hypot(x, y)
	if mag < 1e-9 {
		return 0, 0
	}
	return x / mag, y / mag
}
