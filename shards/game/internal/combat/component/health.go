// Package component holds the combat feature's ECS component data — the leaf other features import to
// read or attach damageable state without depending on combat's systems. It imports only the engine
// and ability's leaf (for Cause), so no feature that reads Health can close an import cycle back into
// combat (see CLAUDE.md → File layout within a package).
package component

// Health is current/max hit points for any entity that can take damage. Float, not int: continuous
// effects (the Echo's anti-camp DoT, future heals) apply per-second × dt each tick — a fractional
// amount that would truncate to zero at high tick rates if HP were integer
// (see CLAUDE.md → Tick-rate independence).
type Health struct {
	HP    float64
	MaxHP float64
}

// Name is the ECS component name.
func (Health) Name() string { return "health" }
