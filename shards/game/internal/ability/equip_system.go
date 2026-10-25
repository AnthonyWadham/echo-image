package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/ability/component"
	playercomp "echo_image/shards/game/internal/player/component"
)

// claimedRow is a player Body, looked up by the ID in BodyClaimed.
type claimedRow struct {
	Player playercomp.Player
}

// EquipSystem (PreUpdate, after player.InputSystem) gives a Body its cooldowns, all ready, the tick it
// is claimed, as echo.SpawnSystem gives it an Echo. So player never knows about abilities. The kit is
// the Body's champion's, in tuning.
type EquipSystem struct{}

func (s *EquipSystem) Run(w *cardinal.World) {
	bodies := w.Contains[claimedRow]()
	for claim := range w.SystemEvents[playercomp.BodyClaimed]() {
		body, err := bodies.GetByID(claim.Body)
		if err != nil {
			continue // claimed Body vanished the same tick
		}
		body.Set(component.Abilities{})
	}
}
