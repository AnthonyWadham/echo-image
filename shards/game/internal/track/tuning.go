package track

import "echo_image/shards/game/internal/track/past"

// TrackTuning is data-driven lag compensation config, loaded once at boot from content/track/tuning.json.
// It is past.Tuning, so the client predictor loads the same window.
type TrackTuning = past.Tuning
