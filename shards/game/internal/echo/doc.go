// Package echo implements the Echo: a delayed replay of a player Body that carries the player's Health
// and is the only damageable target (GDD §3). Each tick the Body's pose is recorded into a per-Echo
// ring buffer (component.PathHistory) and the Echo is driven to the pose from delay_seconds ago, so it
// retraces the Body's exact path — dashes and blinks included, because we record each tick's outcome,
// not its input. The Echo is a BodyTypeManual physics body: gameplay owns its pose, Box2D only detects
// hits against it.
package echo
