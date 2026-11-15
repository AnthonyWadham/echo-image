// Package cast is the Cardinal-free core of abilities: the ability rows, the gate that decides what a
// frame casts and the cooldowns it starts, and Run, which runs each cast's hook. ability.CastSystem and
// ability.CastHookSystem run them on the server, and the client predictor (internal/predict) runs the
// same code. It imports only the standard library and other pure packages, so it builds for wasm.
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
// is held for kit slot N, and aim is a unit vector or zero. A caster with a tag an ability is blocked by
// (stunned) doesn't cast it, and an aimed ability needs an aim.
func (t Tuning) Gate(champion string, cooldowns Cooldowns, tags interact.Tags, buttons uint32, aim hooks.Vec2,
	tick uint64,
) (Cooldowns, []string) {
	if buttons == 0 {
		return cooldowns, nil
	}
	var casts []string
	for slot, id := range t.Kits[champion] {
		def, known := t.Abilities[id]
		if !known || buttons&(1<<slot) == 0 || tick < cooldowns.ReadyTick(id) || tags&t.blockedBy(def) != 0 {
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

// blockedBy is def's own blocked tags, or the file's when it has none.
func (t Tuning) blockedBy(def Def) interact.Tags {
	if def.BlockedBy != nil {
		return *def.BlockedBy
	}
	return t.BlockedBy
}

// Launch is a shot a cast asks for: the cast's Launch-th, of type Shot, from From along Dir, a unit
// vector. With the cast's input frame, Ability and Launch are the shot's key.
type Launch struct {
	Ability   string
	Launch    uint32
	Shot      string
	From, Dir hooks.Vec2
}

// Sink acts on what casts ask for: ability.CastHookSystem's emits each request as its feature's intent,
// and the predictor's predicts the ones its client draws. A new request type is a new method, so neither
// side can miss it.
type Sink interface {
	Launch(l Launch)
	Effect(in status.Intent)
}

// Run runs ability id's hook, if it has one, for c, and hands sink what it
// asks for in order: each launch from c.Pos, numbered within the cast, then each effect, from c.Caster.
func (t Tuning) Run(id string, c hooks.Cast, sink Sink) {
	h, ok := t.bound[id]
	if !ok || h.OnCast == nil {
		return
	}
	var out hooks.Out
	h.OnCast(c, &out)
	for i, l := range out.Launches {
		sink.Launch(Launch{Ability: id, Launch: uint32(i), Shot: l.Shot, From: c.Pos, Dir: l.Dir})
	}
	for _, e := range out.Effects {
		e.Source = c.Caster
		sink.Effect(e)
	}
}
