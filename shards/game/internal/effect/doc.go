// Package effect owns timed effects on any entity: one list per entity (in the component leaf), and
// one resolver that applies EffectIntent at the end of each tick. Other features read the folded
// status from the leaf and never write the list. The rules themselves are in effect/status, which a
// client predictor can run.
package effect
