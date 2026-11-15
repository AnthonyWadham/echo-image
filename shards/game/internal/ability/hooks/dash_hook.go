package hooks

import (
	"errors"

	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/physics/timestep"
)

type dashParams struct {
	Distance float64 `json:"distance"` // world units along the aim
	Seconds  float64 `json:"seconds"`
	Effect   string  `json:"effect"` // the effect kind that carries the move
}

func (p dashParams) Validate() error {
	if p.Distance <= 0 || timestep.Ticks(p.Seconds) < 1 || p.Effect == "" {
		return errors.New("dash needs distance > 0, seconds of at least one tick and an effect")
	}
	return nil
}

func (p dashParams) Refs() Refs { return Refs{Effects: []string{p.Effect}} }

// dash moves the caster its distance along the aim in its seconds, as a forced-velocity effect, so the
// mover runs it into walls and CC can end it. It moves from the next tick.
var dash = Def[dashParams]{
	Aimed: true,
	OnCast: func(c Cast, p dashParams, out *Out) {
		n := uint64(timestep.Ticks(p.Seconds))
		speed := p.Distance * float64(timestep.Rate) / float64(n)
		out.Effects = append(out.Effects, status.Intent{
			Target: c.Caster, Kind: p.Effect, Ticks: n, VelX: c.Aim.X * speed, VelY: c.Aim.Y * speed,
		})
	},
}
