package component

import "echo_image/shards/game/internal/effect/status"

// EffectIntent asks effect.ResolveSystem to add Kind to Target at the end of this tick, so it applies
// from the next tick for Ticks ticks. It is a status.Intent, which the rules apply as is.
type EffectIntent status.Intent

// Name is the system-event name.
func (EffectIntent) Name() string { return "effect_intent" }
