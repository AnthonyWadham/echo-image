package projectile

import "echo_image/shards/game/internal/projectile/flight"

// ProjectileTuning is data-driven shot config, loaded once at boot from content/projectile/tuning.json:
// the shot types and the rules for what a shot does on meeting each thing. It is flight.Tuning, so the
// client predictor loads the same rules. Read it via data.Get[ProjectileTuning]().
type ProjectileTuning = flight.Tuning
