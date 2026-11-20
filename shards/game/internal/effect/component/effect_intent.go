package component

import "echo_image/shards/game/internal/effect/status"

// EffectIntent asks effect.ResolveSystem to add Kind to Target at the end of the tick it drains it, so
// it applies from the next tick for Ticks ticks. It is a status.Intent, which the rules apply as is.
// Any feature sends it to effect's queue with queue.Send.
type EffectIntent status.Intent

// Name names its queue.
func (EffectIntent) Name() string { return "effect_intent" }
