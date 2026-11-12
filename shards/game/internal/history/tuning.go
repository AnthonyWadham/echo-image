package history

import "echo_image/shards/game/internal/history/past"

// HistoryTuning is data-driven lag compensation config, loaded once at boot from content/history/tuning.json.
// It is past.Tuning, so the client predictor loads the same window.
type HistoryTuning = past.Tuning
