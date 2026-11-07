package ability

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/ability/hooks"
	effectcomp "echo_image/shards/game/internal/effect/component"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/ref"
)

// casterRow is a Body with input, a champion, cooldowns and effects.
type casterRow struct {
	Spawn     ref.Spawn
	Input     playercomp.InputQueue
	Champion  playercomp.Champion
	Abilities component.Abilities
	Effects   effectcomp.Effects
}

// CastSystem (PreUpdate, after AttachAbilitiesSystem and before player.MovementSystem) casts what each Body's
// frame this tick asks for, by cast.Gate, the gate the client predictor runs too: it starts each
// cast's cooldown and emits AbilityCast, and does nothing else. It is the only writer of cooldowns.
// O(B·S) for B Bodies with S slots.
type CastSystem struct {
	Tuning AbilityTuning
}

func (s *CastSystem) Run(w *cardinal.World) {
	tuning := s.Tuning
	tick := w.TickHeight()
	for body := range w.Contains[casterRow]().Iter() {
		f, ok := body.Get[playercomp.InputQueue]().Applied(tick)
		if !ok || f.Buttons == 0 {
			continue
		}
		aimX, aimY := f.Aim()
		cooldowns, casts := tuning.Gate(body.Get[playercomp.Champion]().ID,
			slices.Collect(body.Get[component.Abilities]().Cooldowns.Values()),
			body.Get[effectcomp.Effects]().Status(tick).Tags, f.Buttons, hooks.Vec2{X: aimX, Y: aimY}, tick)
		if len(casts) == 0 {
			continue
		}
		body.Set(component.Abilities{Cooldowns: cardinal.SliceOf(cooldowns...)})
		for _, id := range casts {
			cause := component.Cause{Caster: body.ID(), CasterSerial: body.Get[ref.Spawn]().Serial, Ability: id,
				Tick: tick, Seq: f.Seq}
			w.EmitSystemEvent(component.AbilityCast{Cause: cause, AimX: aimX, AimY: aimY})
		}
	}
}
