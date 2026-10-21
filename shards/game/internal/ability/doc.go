// Package ability decides whether a Body can use an ability. It holds each Body's kit and cooldowns,
// casts from the input buttons, and emits AbilityCast. What an ability does belongs to the feature it
// changes, which reacts to AbilityCast by ability ID with its own numbers (projectile launches a
// shot). So a new effect never changes ability, and ability never imports an effect.
package ability
