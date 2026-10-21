package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/team"
)

// syncRow is a kit allies see.
type syncRow struct {
	Abilities component.Abilities
	Team      team.Team
}

// SyncSystem (Update, after casts) sends each player their team's kits, with SendTo. O(B·S) to
// collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var abilities netsync.Items[AbilityState]
	for e := range w.Contains[syncRow]().Iter() {
		for i, slot := range e.Get[component.Abilities]().Slots.All() {
			abilities.Add(e, netsync.Allies, AbilityState{
				Body:      uint32(e.ID()),
				Slot:      uint32(i),
				Ability:   slot.Ability,
				ReadyTick: slot.ReadyTick,
			})
		}
	}
	lists := abilities.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, AbilitySync{Tick: w.TickHeight(), Abilities: lists.For(v)})
	}
}
