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
	Player   cardinal.Ref[component.Player]
	Velocity cardinal.Ref[physics2d.Velocity2D]
}

// MovementSystemState drains Move commands and drives the matching player's Body.
type MovementSystemState struct {
	cardinal.BaseSystemState
	MoveCommands cardinal.WithCommand[Move]
	Movables     cardinal.Contains[movableRow]
	Claimed      cardinal.WithSystemEventEmitter[BodyClaimed]
}

// MovementSystem applies each Move command to the Body owned by the command's authenticated persona,
// setting Velocity2D (kinematic Bodies move by velocity; physics writeback updates Transform2D). The
// velocity persists until the next command, so the client sends a zero vector to stop.
func MovementSystem(state *MovementSystemState) {
	speed := data.Get[MovementTuning]().MoveSpeed // read config once at system entry, not per command
	for cmd := range state.MoveCommands.Iter() {
		if cmd.Persona == "" {
			// An empty persona matches any unclaimed Body in resolveBody and re-enters the claim branch
			// every tick (setting Persona="" is a no-op), re-emitting BodyClaimed and spawning a fresh
			// Echo each time. Authenticated commands always carry a persona; drop the malformed ones.
			state.Logger().Warn().Msg("move: empty persona, ignoring command")
			continue
		}
		eid, ok := state.resolveBody(cmd.Persona)
		if !ok {
			state.Logger().Warn().Str("persona", cmd.Persona).Msg("move: no free player Body to control")
			continue
		}
		row, err := state.Movables.GetByID(eid)
		if err != nil {
			continue
		}
		if p := row.Player.Get(); p.Persona == "" { // first command from this persona claims a free Body
			p.Persona = cmd.Persona
			row.Player.Set(p)
			// Edge-trigger echo.SpawnSystem: this branch runs once per Body (Persona is never cleared), so
			// each Body gets exactly one Echo (see echo.SpawnSystem for why spawn is claim-triggered).
			state.Claimed.Emit(BodyClaimed{Body: eid})
			state.Logger().Info().Str("persona", cmd.Persona).Uint32("id", p.ID).Msg("player Body claimed")
		}
		vx, vy := normalize(cmd.Payload.DirX, cmd.Payload.DirY)
		row.Velocity.Set(physics2d.Velocity2D{Linear: physics2d.Vec2{X: vx * speed, Y: vy * speed}})
	}
}

// resolveBody returns the Body owned by persona, or a free (unclaimed) Body if none is owned yet. The
// claim-on-first-command is a dev-spawn convenience so any MCP email can grab a slot; real
// matchmaking will assign personas at spawn and this falls away.
func (state *MovementSystemState) resolveBody(persona string) (cardinal.EntityID, bool) {
	var free cardinal.EntityID
	haveFree := false
	for eid, m := range state.Movables.Iter() {
		switch p := m.Player.Get(); {
		case p.Persona == persona:
			return eid, true
		case p.Persona == "" && !haveFree:
			free, haveFree = eid, true
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
