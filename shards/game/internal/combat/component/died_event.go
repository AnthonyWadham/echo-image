package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Died is emitted the tick a target's HP reaches 0. Until is when in the tick it died (0 to 1): when
// the hit that took it there landed. combat.DeathSystem removes it after the syncs, so each sync sends
// it once more. It lives in this leaf so any feature can react to a death.
type Died struct {
	Target cardinal.EntityID
	Until  float64
}

// Name is the system-event name.
func (Died) Name() string { return "died" }
