// Package component holds the combat feature's ECS component data — the leaf other features import to
// read or attach damageable state without depending on combat's systems. It imports only the engine,
// so no feature that reads Health can close an import cycle back into combat
// (see CLAUDE.md → File layout within a package).
package component

// Health is current/max hit points for any entity that can take damage.
type Health struct {
	HP    int
	MaxHP int
}

// Name is the ECS component name.
func (Health) Name() string { return "health" }
