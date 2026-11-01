package ability

import (
	"fmt"

	"echo_image/shards/game/internal/ability/cast"
	playercomp "echo_image/shards/game/internal/player/component"
)

// AbilityTuning is data-driven ability config, loaded once at boot from content/ability/tuning.json:
// the kits, how each ability is cast, and the hook that does it with its params. It is cast.Tuning,
// which the client predictor loads too (its Bind binds the hooks at load), plus the check that needs
// the input frame.
type AbilityTuning struct {
	cast.Tuning
}

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
