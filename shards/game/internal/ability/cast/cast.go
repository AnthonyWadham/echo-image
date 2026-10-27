// Package cast is the Cardinal-free core of abilities: the ability rows, the gate that decides what a
// frame casts, and the cooldowns it starts. ability.CastSystem runs the gate on the server, and the
// client predictor (internal/predict) runs the same code. It imports only the standard library and
// other pure packages, so it builds for wasm.
package cast

import (
	"slices"

	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
)

// Cooldown is when an ability can be cast again.
type Cooldown struct {
	Ability   string // ability ID, a key of Tuning.Abilities
	ReadyTick uint64
}

// Cooldowns is a caster's cooldowns, one per ability ID it has cast, so a cooldown follows its ability
// into whichever slot holds it.
type Cooldowns []Cooldown

// ReadyTick is when ability can be cast again: 0 if it never was cast.
func (c Cooldowns) ReadyTick(ability string) uint64 {
	if i := c.index(ability); i >= 0 {
		return c[i].ReadyTick
	}
	return 0
}

// With returns a copy of c with ability castable again from tick.
func (c Cooldowns) With(ability string, tick uint64) Cooldowns {
	out := slices.Clone(c)
	if i := c.index(ability); i >= 0 {
		out[i].ReadyTick = tick
		return out
	}
	return append(out, Cooldown{Ability: ability, ReadyTick: tick})
}

func (c Cooldowns) index(ability string) int {
	return slices.IndexFunc(c, func(x Cooldown) bool { return x.Ability == ability })
}

// Gate returns what a frame casts at tick for a caster playing champion: each ready ability in the
// champion's kit whose button is held, in slot order, and the cooldowns after them. Bit N of buttons
// is held for kit slot N, and aim is a unit vector or zero. A stunned caster casts nothing, and an
// aimed ability needs an aim.
func (t Tuning) Gate(champion string, cooldowns Cooldowns, tags interact.Tags, buttons uint32, aim hooks.Vec2,
	tick uint64,
) (Cooldowns, []string) {
	if buttons == 0 || tags&interact.Stunned != 0 {
		return cooldowns, nil
	}
	var casts []string
	for slot, id := range t.Kits[champion] {
		def, known := t.Abilities[id]
		if !known || buttons&(1<<slot) == 0 || tick < cooldowns.ReadyTick(id) {
			continue
		}
		if def.Aimed && aim == (hooks.Vec2{}) {
			continue // no aim: no cast, and no cooldown
		}
		cooldowns = cooldowns.With(id, tick+def.CooldownTicks())
		casts = append(casts, id)
	}
	return cooldowns, casts
}

// EffectIntent is the intent for an effect a hook asks for, from source.
func EffectIntent(e hooks.Effect, source uint32) status.Intent {
	return status.Intent{Target: e.Target, Kind: e.Kind, Ticks: e.Ticks, VelX: e.Vel.X, VelY: e.Vel.Y, Source: source}
}
