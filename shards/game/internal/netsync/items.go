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
	entity   cardinal.Entity // unset when gone
	last     Last            // set when gone
	gone     bool
	team     uint8
	hasTeam  bool
	audience Audience
	reveals  []cardinal.Entity
	state    S
}

// Add queues e's state for the viewers a allows. The team comes from e's Team component. reveals are the
// other entities the state gives away, like a shot's caster.
func (it *Items[S]) Add(e cardinal.Entity, a Audience, state S, reveals ...cardinal.Entity) {
	i := item[S]{entity: e, audience: a, reveals: reveals, state: state}
	if e.Has[team.Team]() {
		i.team, i.hasTeam = e.Get[team.Team]().ID, true
	}
	it.list = append(it.list, i)
}

// Last is where an entity destroyed this tick last was, and its team.
type Last struct {
	Team uint8
	X, Y float64
}

// AddGone queues the last state of an entity destroyed this tick, for the viewers a allows. reveals as
// for Add.
func (it *Items[S]) AddGone(last Last, a Audience, state S, reveals ...cardinal.Entity) {
	it.list = append(it.list, item[S]{
		last: last, gone: true, team: last.Team, hasTeam: true, audience: a, reveals: reveals, state: state,
	})
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

// allowed reports whether team t may get the item: Allies by team, Everyone if t sees the entity, or
// where it last was, and all it reveals.
func (i item[S]) allowed(t uint8) bool {
	if i.audience == Allies {
		return i.hasTeam && i.team == t
	}
	if i.gone {
		if !lastSeenBy(i.last, t) {
			return false
		}
	} else if !seenBy(i.entity, t) {
		return false
	}
	for _, e := range i.reveals {
		if !seenBy(e, t) {
			return false
		}
	}
	return true
}

// seenBy reports whether team sees e. Everything is visible today; invisibility and line of sight go
// here. Judge e's team.Owner too: an invisible Body's Echo replays its path.
func seenBy(_ cardinal.Entity, _ uint8) bool { return true }

// lastSeenBy reports whether team sees where a gone entity last was.
func lastSeenBy(_ Last, _ uint8) bool { return true }
