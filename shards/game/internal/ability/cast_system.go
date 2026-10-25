package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/ability/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	playercomp "echo_image/shards/game/internal/player/component"
)

// casterRow is a Body with input, a champion, cooldowns and effects.
type casterRow struct {
	Input     playercomp.InputQueue
	Champion  playercomp.Champion
	Abilities component.Abilities
	Effects   effectcomp.Effects
}

// CastSystem (PreUpdate, after EquipSystem and before player.MovementSystem) casts each ready ability
// in the Body's champion's kit whose button is held in this tick's input frame, unless the Body is
// stunned: it starts that ability's cooldown and emits AbilityCast, and does nothing else. It is the
// only writer of cooldowns. O(B·S) for B Bodies with S slots.
type CastSystem struct {
	Data *data.Plugin
}

func (s *CastSystem) Run(w *cardinal.World) {
	tuning := data.Get[AbilityTuning](s.Data)
	tick := w.TickHeight()
	for body := range w.Contains[casterRow]().Iter() {
		f, ok := body.Get[playercomp.InputQueue]().Applied(tick)
		if !ok || f.Buttons == 0 || body.Get[effectcomp.Effects]().Status(tick).Tags&interact.Stunned != 0 {
			continue
		}
		aimX, aimY := f.Aim()
		state := body.Get[component.Abilities]()
		cast := false
		for i, id := range tuning.Kits[body.Get[playercomp.Champion]().ID] {
			def, known := tuning.Abilities[id]
			if !known || !f.Pressed(i) || tick < state.ReadyTick(id) {
				continue
			}
			if def.Aimed && aimX == 0 && aimY == 0 {
				continue // no aim: no cast, and no cooldown
			}
			state = state.WithReadyTick(id, tick+def.CooldownTicks())
			cast = true
			cause := component.Cause{Caster: body.ID(), Ability: id, Tick: tick}
			w.EmitSystemEvent(component.AbilityCast{Cause: cause, AimX: aimX, AimY: aimY})
		}
		if cast {
			body.Set(state)
		}
	}
}
