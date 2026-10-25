// Package ability decides whether a Body can use an ability, and runs what it does. It holds each Body's
// cooldowns, casts from its champion's kit and the input buttons, and emits AbilityCast. What an
// ability does is its hook (ability/hooks), named in its content row: the hook fills requests, and
// CastHookSystem turns them into intents for the features that own the state (projectile launches a
// shot). So a new ability is a content row, plus a hook only when no shared one fits.
package ability
