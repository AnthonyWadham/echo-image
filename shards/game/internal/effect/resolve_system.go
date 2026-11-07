package effect

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
)

// holderRow is any entity with an effect list: a Body or an Echo.
type holderRow struct {
	Effects component.Effects
}

// ResolveSystem (Update, last: after every EffectIntent emitter) is the only writer of Effects. It
// adds this tick's intents in a fixed order, so they apply from the next tick, and drops the entries
// that are over by then. So every system in a tick reads the same status. The rules are
// effect/status's, which the client predictor runs too. O(I log I + H·L) for I intents and H holders
// of L entries.
type ResolveSystem struct {
	Tuning EffectTuning
}

func (s *ResolveSystem) Run(w *cardinal.World) {
	tuning := s.Tuning
	next := w.TickHeight() + 1
	holders := w.Contains[holderRow]()
	var intents []status.Intent
	for in := range w.SystemEvents[component.EffectIntent]() {
		intents = append(intents, status.Intent(in))
	}
	slices.SortFunc(intents, tuning.Compare)
	for i := 0; i < len(intents); {
		j := i + 1
		for j < len(intents) && intents[j].Target == intents[i].Target {
			j++
		}
		if e, err := holders.GetByID(cardinal.EntityID(intents[i].Target)); err == nil {
			add(w, e, tuning, intents[i:j], next)
		}
		i = j
	}
	for e := range holders.Iter() {
		fx := e.Get[component.Effects]()
		if fx.List.ContainsFunc(func(x status.Entry) bool { return x.Over(next) }) {
			fx.List = cardinal.SliceOf(status.Sweep(slices.Collect(fx.List.Values()), next)...)
			e.Set(fx)
		}
	}
}

// add adds intents, all for e, to e's effect list from tick on. An intent of an unknown kind is dropped
// with an error, since it's its sender's bug.
func add(w *cardinal.World, e cardinal.Entity, tuning EffectTuning, intents []status.Intent, tick uint64) {
	fx := e.Get[component.Effects]()
	list := slices.Collect(fx.List.Values())
	for _, in := range intents {
		var ok bool
		if list, ok = tuning.Apply(list, &fx.Next, in, tick); !ok {
			w.Logger().Error().Str("kind", in.Kind).Uint32("target", in.Target).
				Msg("dropped an effect of unknown kind")
		}
	}
	fx.List = cardinal.SliceOf(list...)
	e.Set(fx)
}
