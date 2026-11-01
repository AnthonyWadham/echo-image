package netsync

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/team"
)

// Audience is who may receive an entity's state.
type Audience uint8

const (
	// Everyone sends to every team that can see the entity.
	Everyone Audience = iota
	// Allies sends only to the entity's own team, e.g. cooldowns.
	Allies
)

// Items collects one kind of per-entity state for this tick.
type Items[S any] struct {
	list []item[S]
}

type item[S any] struct {
	entity   cardinal.Entity
	team     uint8
	hasTeam  bool
	audience Audience
	state    S
}

// Add queues e's state for the viewers a allows. The team comes from e's Team component.
func (it *Items[S]) Add(e cardinal.Entity, a Audience, state S) {
	i := item[S]{entity: e, audience: a, state: state}
	if e.Has[team.Team]() {
		i.team, i.hasTeam = e.Get[team.Team]().ID, true
	}
	it.list = append(it.list, i)
}

// AddGone queues the last state of an entity of team that was destroyed this tick, for the viewers a
// allows.
func (it *Items[S]) AddGone(team uint8, a Audience, state S) {
	it.list = append(it.list, item[S]{team: team, hasTeam: true, audience: a, state: state})
}

// Lists is the states each viewer team may get. Viewers on a team share one list, so don't modify it.
type Lists[S any] map[uint8]cardinal.Slice[S]

// For returns v's list.
func (l Lists[S]) For(v Viewer) cardinal.Slice[S] { return l[v.Team] }

// Lists filters the items once per viewer team: O(T·N).
func (it *Items[S]) Lists(viewers []Viewer) Lists[S] {
	lists := Lists[S]{}
	for _, v := range viewers {
		if _, done := lists[v.Team]; done {
			continue
		}
		var states []S
		for _, i := range it.list {
			if i.allowed(v.Team) {
				states = append(states, i.state)
			}
		}
		lists[v.Team] = cardinal.SliceOf(states...)
	}
	return lists
}

func (i item[S]) allowed(t uint8) bool {
	if i.audience == Allies {
		return i.hasTeam && i.team == t
	}
	return seenBy(i.entity, t)
}

// seenBy reports whether team can see e. Everything is visible today; invisibility and line of
// sight go here. Hide what reveals a hidden entity too (an invisible Body's Echo replays its path). A
// gone entity (AddGone) is the zero Entity: judge it by its last state.
func seenBy(_ cardinal.Entity, _ uint8) bool { return true }
