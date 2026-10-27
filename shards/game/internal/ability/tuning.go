package ability

import (
	"context"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/ability/cast"
	playercomp "echo_image/shards/game/internal/player/component"
)

// AbilityTuning is data-driven ability config, loaded once at boot from content/ability/tuning.json:
// the kits, how each ability is cast, and the hook that does it with its params. It is cast.Tuning,
// which the client predictor loads too, plus the check that needs the input frame. Read it via
// data.Get[AbilityTuning]().
type AbilityTuning struct {
	cast.Tuning
}

// Resolve binds each ability's hook to its params once, at load, so bad content fails the boot.
func (t *AbilityTuning) Resolve(context.Context, data.Source) error { return t.Bind() }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t AbilityTuning) Validate() error {
	if err := t.Tuning.Validate(); err != nil {
		return err
	}
	for champion, kit := range t.Kits {
		if len(kit) > playercomp.ButtonCount {
			return fmt.Errorf("ability_tuning: %q's kit has %d abilities, more than the %d input buttons",
				champion, len(kit), playercomp.ButtonCount)
		}
	}
	return nil
}
