// Package hooks holds what abilities do: one hook per name, each with its own params. Every ability
// row names its hook. A hook reads its cast and params and fills requests in Out, and the ability
// feature turns them into the intents of the features that own the state, so a hook never writes
// state. It imports only the standard library, timestep and status, so a client predictor can run it.
package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"

	"echo_image/shards/game/internal/effect/status"
)

// Vec2 is a point or a direction.
type Vec2 struct{ X, Y float64 }

// Cast is one cast of an ability. Never assume the caster: it may not be a Body.
type Cast struct {
	Caster, Owner uint32 // the caster, and the player it acts for
	Team          uint8
	Pos, Aim      Vec2 // Aim is a unit vector, or zero for an ability that isn't aimed
}

// Out collects a hook's requests. A request type is added with its first user.
type Out struct {
	Launches []Launch
	Effects  []status.Intent // the ability feature sets Source to the caster
}

// Launch asks for a shot of type Shot from the caster along Dir, a unit vector.
type Launch struct {
	Shot string
	Dir  Vec2
}

// Def is one hook's code. P is its params, decoded from the ability's JSON at boot.
type Def[P Params] struct {
	Aimed  bool // it needs an aimed ability
	OnCast func(c Cast, p P, out *Out)
}

// Params are a hook's numbers and IDs. They check themselves, and list the IDs they name in other
// content files so a test can check those exist.
type Params interface {
	Validate() error
	Refs() Refs
}

// Refs are content IDs in other files.
type Refs struct {
	Shots   []string // keys of projectile.ProjectileTuning.Shots
	Effects []string // keys of effect.EffectTuning.Kinds
}

// Hook is a hook bound to one ability's params.
type Hook struct {
	Aimed  bool
	OnCast func(c Cast, out *Out) // nil if it does nothing on cast
	Refs   Refs
}

// Bind returns hook name bound to params. An unknown name, an unknown field or a bad value is an error,
// so bad content fails the boot.
func Bind(name string, params json.RawMessage) (Hook, error) {
	b, ok := registry[name]
	if !ok {
		return Hook{}, fmt.Errorf("unknown hook %q", name)
	}
	return b(params)
}

type binder func(json.RawMessage) (Hook, error)

func bind[P Params](d Def[P]) binder {
	return func(raw json.RawMessage) (Hook, error) {
		var p P
		if len(raw) > 0 {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				return Hook{}, fmt.Errorf("params: %w", err)
			}
		}
		if err := p.Validate(); err != nil {
			return Hook{}, fmt.Errorf("params: %w", err)
		}
		h := Hook{Aimed: d.Aimed, Refs: p.Refs()}
		if d.OnCast != nil {
			h.OnCast = func(c Cast, out *Out) { d.OnCast(c, p, out) }
		}
		return h, nil
	}
}

// registry is every hook, by name. A unique hook is named after its ability.
var registry = map[string]binder{
	"launch": bind(launch),
	"dash":   bind(dash),
}
