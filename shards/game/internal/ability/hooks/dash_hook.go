package hooks

import (
	"echo_image/shards/game/internal/physics/timestep"
	"errors"
)

type dashParams struct {
	Distance float64 `json:"distance"` // world units along the aim
	Seconds  float64 `json:"seconds"`
	Effect   string  `json:"effect"` // the effect kind that carries the move
}

func (p dashParams) Validate(tickRate int) error {
	if p.Distance <= 0 || timestep.TicksAt(p.Seconds, tickRate) < 1 || p.Effect == "" {
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
		n := c.Ticks(p.Seconds)
		speed := p.Distance * float64(c.TickRate) / float64(n)
		out.Effects = append(out.Effects, Effect{
			Target: c.Caster, Kind: p.Effect, Ticks: n, Vel: Vec2{X: c.Aim.X * speed, Y: c.Aim.Y * speed},
		})
	},
}
