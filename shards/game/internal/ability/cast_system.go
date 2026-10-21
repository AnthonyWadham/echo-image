package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/ability/component"
	playercomp "echo_image/shards/game/internal/player/component"
)

// casterRow is a Body with input and a kit.
type casterRow struct {
	Input     playercomp.InputQueue
	Abilities component.Abilities
}

// CastSystem (Update, after EquipSystem) casts each ready ability whose button is held in this tick's
// input frame: it starts the cooldown and emits AbilityCast, and does nothing else. It is the only
// writer of cooldowns. O(B·S) for B Bodies with S slots.
type CastSystem struct {
	Data *data.Plugin
}

func (s *CastSystem) Run(w *cardinal.World) {
	defs := data.Get[AbilityTuning](s.Data).Abilities
	tick := w.TickHeight()
	for body := range w.Contains[casterRow]().Iter() {
		f, ok := body.Get[playercomp.InputQueue]().Applied(tick)
		if !ok || f.Buttons == 0 {
			continue
		}
		aimX, aimY := f.Aim()
		kit := body.Get[component.Abilities]()
		cast := false
		for i, slot := range kit.Slots.All() {
			def, known := defs[slot.Ability]
			if !known || !f.Pressed(i) || tick < slot.ReadyTick {
				continue
			}
			if def.Aimed && aimX == 0 && aimY == 0 {
				continue // no aim: no cast, and no cooldown
			}
			slot.ReadyTick = tick + def.CooldownTicks()
			kit.Slots = kit.Slots.With(i, slot)
			cast = true
			w.EmitSystemEvent(component.AbilityCast{Caster: body.ID(), Ability: slot.Ability, AimX: aimX, AimY: aimY})
		}
		if cast {
			body.Set(kit)
		}
	}
}
