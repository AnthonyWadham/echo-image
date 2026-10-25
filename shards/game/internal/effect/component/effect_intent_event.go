package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// EffectIntent asks effect.ResolveSystem to add Kind to Target at the end of this tick, so it applies
// from the next tick for Ticks ticks. The kind's tags, passes, stat and stacking come from effect
// tuning; the numbers come from here.
type EffectIntent struct {
	Target     cardinal.EntityID
	Kind       string // a key of effect.EffectTuning.Kinds
	Ticks      uint64
	Value      int32   // the kind's stat, in basis points
	VelX, VelY float64 // forced velocity, units per second
	Source     cardinal.EntityID
}

// Name is the system-event name.
func (EffectIntent) Name() string { return "effect_intent" }
