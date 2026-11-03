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

// AttachAbilitiesSystem (PreUpdate, after player.InputSystem) attaches Abilities to a Body the tick it
// is claimed, so CastSystem can cast for it.
type AttachAbilitiesSystem struct{}

func (s *AttachAbilitiesSystem) Run(w *cardinal.World) {
	bodies := w.Contains[claimedRow]()
	for claim := range w.SystemEvents[playercomp.BodyClaimed]() {
		body, err := bodies.GetByID(claim.Body)
		if err != nil {
			continue // claimed Body vanished the same tick
		}
		body.Set(component.Abilities{})
	}
}
