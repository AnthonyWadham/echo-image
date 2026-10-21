package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/ability/component"
	playercomp "echo_image/shards/game/internal/player/component"
)

// claimedRow is a player Body, looked up by the ID in BodyClaimed.
type claimedRow struct {
	Player playercomp.Player
}

// EquipSystem (Update) gives a Body the default kit the tick it is claimed, as echo.SpawnSystem gives
// it an Echo. So player never knows about abilities.
type EquipSystem struct {
	Data *data.Plugin
}

func (s *EquipSystem) Run(w *cardinal.World) {
	bodies := w.Contains[claimedRow]()
	for claim := range w.SystemEvents[playercomp.BodyClaimed]() {
		body, err := bodies.GetByID(claim.Body)
		if err != nil {
			continue // claimed Body vanished the same tick
		}
		body.Set(kit(data.Get[AbilityTuning](s.Data).DefaultKit))
	}
}

// kit returns the abilities in slot order, all ready to cast.
func kit(ids []string) component.Abilities {
	slots := make([]component.Slot, len(ids))
	for i, id := range ids {
		slots[i] = component.Slot{Ability: id}
	}
	return component.Abilities{Slots: cardinal.SliceOf(slots...)}
}
