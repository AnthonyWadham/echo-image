package hooks

import "errors"

type launchParams struct {
	Shot string `json:"shot"` // the shot type it fires
}

func (p launchParams) Validate() error {
	if p.Shot == "" {
		return errors.New("launch needs a shot")
	}
	return nil
}

func (p launchParams) Refs() Refs { return Refs{Shots: []string{p.Shot}} }

// launch fires one shot along the aim.
var launch = Def[launchParams]{
	Aimed: true,
	OnCast: func(c Cast, p launchParams, out *Out) {
		out.Launches = append(out.Launches, Launch{Shot: p.Shot, Dir: c.Aim})
	},
}
