package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// syncRow is what players see of a Body. Movement components are read only for the owner's
// YouState, so a Body without them is still shown.
type syncRow struct {
	Player    component.Player
	Team      team.Team
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D // for its radius
}

// SyncSystem (Update, after game logic) sends each player the Bodies they may see, and their own
// prediction data, with SendTo. O(B) to collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var bodies netsync.Items[BodyState]
	for e := range w.Contains[syncRow]().Iter() {
		p := e.Get[component.Player]()
		pos := e.Get[physics2d.Transform2D]().Position
		bodies.Add(e, netsync.Everyone, BodyState{
			EntityID: uint32(e.ID()),
			PlayerID: p.ID,
			Nickname: p.Nickname,
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   radius(e),
		})
	}
	lists := bodies.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, BodySync{
			Tick:        w.TickHeight(),
			TickSeconds: physics.SecondsPerTick,
			You:         you(v.Body),
			Bodies:      lists.For(v),
		})
	}
}

// you returns a viewer's own data. A Body without movement components gets zero ack and speed.
func you(e cardinal.Entity) YouState {
	y := YouState{Body: uint32(e.ID())}
	if e.Has[component.InputQueue]() {
		q := e.Get[component.InputQueue]()
		y.InputSession, y.InputAck = q.Session, q.Ack()
	}
	if e.Has[component.MoveSpeed]() {
		y.MoveSpeed = e.Get[component.MoveSpeed]().Value
	}
	return y
}
