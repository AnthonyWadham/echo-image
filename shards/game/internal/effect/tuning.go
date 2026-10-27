package effect

import "echo_image/shards/game/internal/effect/status"

// EffectTuning is data-driven effect config, loaded once at boot from content/effect/tuning.json: what
// each effect kind does. It is status.Tuning, so the client predictor loads the same rules. Read it
// via data.Get[EffectTuning]().
type EffectTuning = status.Tuning
