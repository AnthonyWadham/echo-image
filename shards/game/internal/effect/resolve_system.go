package effect

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
)

// holderRow is any entity with an effect list: a Body or an Echo.
type holderRow struct {
	Effects component.Effects
}

// ResolveSystem (Update, after every EffectIntent emitter, before the syncs) is the only writer of
// Effects. It adds this tick's intents in a fixed order, so they apply from the next tick, and drops
// the entries that are over by then. So every system in a tick reads the same status. O(I log I + H·L)
// for I intents and H holders of L entries.
type ResolveSystem struct {
	Data *data.Plugin
}

func (s *ResolveSystem) Run(w *cardinal.World) {
	tuning := data.Get[EffectTuning](s.Data)
	next := w.TickHeight() + 1
	holders := w.Contains[holderRow]()
	intents := slices.Collect(w.SystemEvents[component.EffectIntent]())
	slices.SortFunc(intents, func(a, b component.EffectIntent) int { return compare(tuning, a, b) })
	for i := 0; i < len(intents); {
		j := i + 1
		for j < len(intents) && intents[j].Target == intents[i].Target {
			j++
		}
		if e, err := holders.GetByID(intents[i].Target); err == nil {
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

// compare orders intents by target, then CC last, so an Unstoppable added the same tick blocks it.
// The rest of the key keeps emit order out of it, since that can change after a restore.
func compare(tuning EffectTuning, a, b component.EffectIntent) int {
	return cmp.Or(
		cmp.Compare(a.Target, b.Target),
		cmp.Compare(tuning.ccRank(a.Kind), tuning.ccRank(b.Kind)),
		cmp.Compare(a.Source, b.Source),
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Ticks, b.Ticks),
		cmp.Compare(a.Value, b.Value),
		cmp.Compare(a.VelX, b.VelX),
		cmp.Compare(a.VelY, b.VelY),
	)
}

// ccRank is 1 for a CC kind and 0 for any other.
func (t EffectTuning) ccRank(kind string) int {
	if e, _, _ := t.entry(kind); e.Tags&interact.CC != 0 {
		return 1
	}
	return 0
}

// add adds intents, all for e, to e's effect list from tick on. An intent of an unknown kind is dropped
// with an error, since it's its sender's bug.
func add(w *cardinal.World, e cardinal.Entity, tuning EffectTuning, intents []component.EffectIntent, tick uint64) {
	fx := e.Get[component.Effects]()
	list := slices.Collect(fx.List.Values())
	for _, in := range intents {
		entry, rule, ok := tuning.entry(in.Kind)
		if !ok {
			w.Logger().Error().Str("kind", in.Kind).Uint32("target", uint32(in.Target)).
				Msg("dropped an effect of unknown kind")
			continue
		}
		entry.Value, entry.VelX, entry.VelY, entry.Source = in.Value, in.VelX, in.VelY, uint32(in.Source)
		list, _ = status.Add(list, &fx.Next, entry, rule, tick, in.Ticks)
	}
	fx.List = cardinal.SliceOf(list...)
	e.Set(fx)
}
