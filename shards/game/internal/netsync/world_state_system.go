package netsync

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	echocomp "echo_image/shards/game/internal/echo/component"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// bodyRow is what netsync shows of a Body. Movement components are read only for the owner's
// YouState, so a Body without them is still shown.
type bodyRow struct {
	Player    playercomp.Player
	Team      team.Team
	Transform physics2d.Transform2D
}

type echoRow struct {
	Echo      echocomp.Echo
	Health    combatcomp.Health
	Team      team.Team
	Transform physics2d.Transform2D
}

// viewer is a player who controls a Body and so receives WorldState.
type viewer struct {
	player string // player ID, the SendTo recipient
	team   uint8
	you    YouState
}

// view is what one team sees this tick.
type view struct {
	bodies []BodyState
	echoes []EchoState
}

// WorldStateSystem (Update, registered last) sends each player this tick's final state. Views are
// built per team (allies share vision): O(T·(B+E)) per tick, then one send per player.
type WorldStateSystem struct{}

func (s *WorldStateSystem) Run(w *cardinal.World) {
	var viewers []viewer
	views := map[uint8]*view{}
	for e := range w.Contains[bodyRow]().Iter() {
		if p := e.Get[playercomp.Player](); p.Persona != "" {
			t := e.Get[team.Team]().ID
			viewers = append(viewers, viewer{player: p.Persona, team: t, you: you(e)})
			if views[t] == nil {
				views[t] = &view{}
			}
		}
	}
	if len(viewers) == 0 {
		return
	}

	for e := range w.Contains[bodyRow]().Iter() {
		p := e.Get[playercomp.Player]()
		pos := e.Get[physics2d.Transform2D]().Position
		state := BodyState{
			EntityID: uint32(e.ID()),
			PlayerID: p.ID,
			Nickname: p.Nickname,
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
		}
		for t, v := range views {
			if seenBy(e, t) {
				v.bodies = append(v.bodies, state)
			}
		}
	}
	for e := range w.Contains[echoRow]().Iter() {
		h := e.Get[combatcomp.Health]()
		pos := e.Get[physics2d.Transform2D]().Position
		state := EchoState{
			EntityID: uint32(e.ID()),
			PlayerID: e.Get[echocomp.Echo]().PlayerID,
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			HP:       h.HP,
			MaxHP:    h.MaxHP,
		}
		for t, v := range views {
			if seenBy(e, t) {
				v.echoes = append(v.echoes, state)
			}
		}
	}

	shared := map[uint8]WorldState{}
	for t, v := range views {
		shared[t] = WorldState{
			Tick:        w.TickHeight(),
			TickSeconds: physics.SecondsPerTick,
			Bodies:      cardinal.SliceOf(v.bodies...),
			Echoes:      cardinal.SliceOf(v.echoes...),
		}
	}
	// SendTo, never Broadcast: any client can subscribe to any event name.
	for _, v := range viewers {
		state := shared[v.team]
		state.You = v.you
		w.SendTo(v.player, state)
	}
}

// seenBy reports whether team can see e. Everything is visible today; invisibility and line of
// sight go here. Hide what reveals a hidden entity too (an invisible Body's Echo replays its path).
func seenBy(_ cardinal.Entity, _ uint8) bool { return true }

// you returns a viewer's own data. A Body without movement components gets zero ack and speed.
func you(e cardinal.Entity) YouState {
	y := YouState{Body: uint32(e.ID())}
	if e.Has[playercomp.Input]() {
		in := e.Get[playercomp.Input]()
		y.InputSession, y.InputAck = in.Session, in.Ack
	}
	if e.Has[playercomp.MoveSpeed]() {
		y.MoveSpeed = e.Get[playercomp.MoveSpeed]().Value
	}
	return y
}
