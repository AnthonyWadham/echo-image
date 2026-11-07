package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/netsync"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// syncRow is a kit allies see.
type syncRow struct {
	Champion  playercomp.Champion
	Abilities component.Abilities
	Team      team.Team
}

// SyncSystem (PostUpdate) sends each player their team's kits, with SendTo. O(B·S) to
// collect, then one send per player.
type SyncSystem struct {
	Tuning AbilityTuning
}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	kits := s.Tuning.Kits
	var abilities netsync.Items[AbilityState]
	for e := range w.Contains[syncRow]().Iter() {
		state := e.Get[component.Abilities]()
		for i, id := range kits[e.Get[playercomp.Champion]().ID] {
			abilities.Add(e, netsync.Allies, AbilityState{
				Body:      uint32(e.ID()),
				Slot:      uint32(i),
				Ability:   id,
				ReadyTick: state.ReadyTick(id),
			})
		}
	}
	lists := abilities.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, AbilitySync{Tick: w.TickHeight(), Abilities: lists.For(v)})
	}
}
