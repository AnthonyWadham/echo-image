package interact

import (
	"encoding/json"
	"testing"
)

func TestFilterMatch(t *testing.T) {
	for _, c := range []struct {
		name string
		f    Filter
		k    Kind
		r    Relation
		t    Tags
		want bool
	}{
		{"empty matches anything", Filter{}, Echo, Enemy, Stunned, true},
		{"kind listed", Filter{Kinds: Wall | Bounds}, Bounds, Neutral, 0, true},
		{"kind not listed", Filter{Kinds: Wall | Bounds}, Body, Neutral, 0, false},
		{"relation listed", Filter{Relations: Own | Ally}, Body, Ally, 0, true},
		{"relation not listed", Filter{Relations: Own | Ally}, Body, Enemy, 0, false},
		{"has every required tag", Filter{Require: Stunned | CC}, Echo, Enemy, Stunned | CC | Unstoppable, true},
		{"misses a required tag", Filter{Require: Stunned | CC}, Echo, Enemy, Stunned, false},
		{"has a forbidden tag", Filter{Forbid: Unstoppable}, Echo, Enemy, CC | Unstoppable, false},
		{"every field", Filter{Kinds: Echo, Relations: Enemy, Require: CC, Forbid: Unstoppable}, Echo, Enemy, CC, true},
	} {
		if got := c.f.Match(c.k, c.r, c.t); got != c.want {
			t.Errorf("%s: Match = %v, want %v", c.name, got, c.want)
		}
	}
}

// Owner decides own; team decides ally or enemy.
func TestRelation(t *testing.T) {
	me := Side{Owner: 7, Team: 1}
	for _, c := range []struct {
		name  string
		other Side
		want  Relation
	}{
		{"my Echo or shot", Side{Owner: 7, Team: 1}, Own},
		{"a teammate", Side{Owner: 8, Team: 1}, Ally},
		{"an enemy", Side{Owner: 9, Team: 0}, Enemy},
		{"my shot after a team change", Side{Owner: 7, Team: 0}, Own},
	} {
		if got := me.Relation(c.other); got != c.want {
			t.Errorf("%s: %b, want %b", c.name, got, c.want)
		}
	}
}

// Order is the only priority: the first rule that matches wins.
func TestFirstMatchWins(t *testing.T) {
	rules := []Filter{{Kinds: Wall}, {Relations: Own}, {Kinds: Body}}
	for _, c := range []struct {
		k    Kind
		r    Relation
		want int // -1 for none
	}{
		{Wall, Neutral, 0},
		{Body, Own, 1}, // rules 1 and 2 both match
		{Body, Enemy, 2},
		{Echo, Enemy, -1},
	} {
		if _, i, ok := First(rules, c.k, c.r, 0); i != c.want || ok != (c.want >= 0) {
			t.Errorf("kind %b, relation %b: rule %d (ok %v), want %d", c.k, c.r, i, ok, c.want)
		}
	}
}

// Content names kinds, relations and tags. They load as bits, and an unknown name fails the load.
func TestNamesLoadAsBits(t *testing.T) {
	var f Filter
	raw := `{"kinds": ["wall", "bounds"], "relations": ["own", "ally"], "require": ["stunned"], "forbid": ["unstoppable"]}`
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if want := (Filter{Kinds: Wall | Bounds, Relations: Own | Ally, Require: Stunned, Forbid: Unstoppable}); f != want {
		t.Fatalf("got %+v, want %+v", f, want)
	}
	for _, bad := range []string{`{"kinds": ["wal"]}`, `{"relations": ["foe"]}`, `{"forbid": ["stun"]}`, `{"kinds": "wall"}`} {
		if err := json.Unmarshal([]byte(bad), &f); err == nil {
			t.Errorf("%s loaded; want an error", bad)
		}
	}
}
