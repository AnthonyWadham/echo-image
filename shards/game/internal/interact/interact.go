// Package interact is the shared vocabulary for what happens when one thing meets another: what a
// thing is (Kind), what state it is in (Tags), and how two things relate (Relation). A feature decides
// each meeting with a list of rules from its content, and the first rule that matches wins, so a new
// shot or area is data, not code. It imports only the standard library, so a client predictor can run
// it.
package interact

import (
	"encoding/json"
	"fmt"
)

// Kind is what a thing is. The kinds are the Box2D category bits, and snapshots keep each shape's bits,
// so never renumber one.
type Kind uint64

const (
	Wall   Kind = 1 << 0 // a map kind: static walls and obstacles
	Body   Kind = 1 << 1 // a player Body
	Echo   Kind = 1 << 2 // a player Echo
	Shot   Kind = 1 << 3 // a shot: no Box2D body, only a query filter
	Bounds Kind = 1 << 4 // a map kind: the map edge, which no effect passes
	Area   Kind = 1 << 5 // an area's query: a Body or Echo masks it so areas can find it
)

// Tags are states that several features read. Effects grant them. Snapshots keep the bits, so never
// renumber one. State only one champion reads stays in that champion's own component.
type Tags uint64

const (
	Stunned     Tags = 1 << 0 // no input movement, no casts
	CC          Tags = 1 << 1 // crowd control: Unstoppable blocks it
	EndsOnCC    Tags = 1 << 2 // removed when CC lands (dashes, channels)
	Unstoppable Tags = 1 << 3 // CC can't land
)

// Relation is how a thing relates to the one it meets. Each is a bit, so a filter can list several.
type Relation uint8

const (
	Own     Relation = 1 << 0 // the same player
	Ally    Relation = 1 << 1 // another player on the same team
	Enemy   Relation = 1 << 2 // a player on another team
	Neutral Relation = 1 << 3 // no player's: the map
)

// Side is who a thing acts for: its owner's Body and team. The map has no side, so it is Neutral.
type Side struct {
	Owner uint32
	Team  uint8
}

// Relation returns how b relates to a: the same owner is Own, the same team Ally, and any other Enemy.
func (a Side) Relation(b Side) Relation {
	switch {
	case a.Owner == b.Owner:
		return Own
	case a.Team == b.Team:
		return Ally
	}
	return Enemy
}

// Filter picks the things a rule applies to. An empty field matches anything.
type Filter struct {
	Kinds     Kind     `json:"kinds"`
	Relations Relation `json:"relations"`
	Require   Tags     `json:"require"` // has all of these
	Forbid    Tags     `json:"forbid"`  // has none of these
}

// Match reports whether a thing of kind k, relation r and tags t passes f.
func (f Filter) Match(k Kind, r Relation, t Tags) bool {
	return (f.Kinds == 0 || f.Kinds&k != 0) && (f.Relations == 0 || f.Relations&r != 0) &&
		t&f.Require == f.Require && t&f.Forbid == 0
}

// Rule is any type with a Filter in it.
type Rule interface {
	Match(Kind, Relation, Tags) bool
}

// First returns the first rule a thing matches and its index, which breaks ties between things met at
// once, or ok=false when none does.
func First[R Rule](rules []R, k Kind, r Relation, t Tags) (R, int, bool) {
	for i, rule := range rules {
		if rule.Match(k, r, t) {
			return rule, i, true
		}
	}
	var none R
	return none, -1, false
}

var (
	kindNames     = map[string]Kind{"wall": Wall, "body": Body, "echo": Echo, "shot": Shot, "bounds": Bounds, "area": Area}
	tagNames      = map[string]Tags{"stunned": Stunned, "cc": CC, "ends_on_cc": EndsOnCC, "unstoppable": Unstoppable}
	relationNames = map[string]Relation{"own": Own, "ally": Ally, "enemy": Enemy, "neutral": Neutral}
)

// UnmarshalJSON reads a list of kind names. An unknown name fails, so bad content fails the boot.
func (k *Kind) UnmarshalJSON(b []byte) error { return decode(b, "kind", kindNames, k) }

// UnmarshalJSON reads a list of tag names. An unknown name fails, so bad content fails the boot.
func (t *Tags) UnmarshalJSON(b []byte) error { return decode(b, "tag", tagNames, t) }

// UnmarshalJSON reads a list of relation names. An unknown name fails, so bad content fails the boot.
func (r *Relation) UnmarshalJSON(b []byte) error { return decode(b, "relation", relationNames, r) }

// decode sets out to the bits of a JSON list of names.
func decode[T ~uint8 | ~uint64](b []byte, what string, names map[string]T, out *T) error {
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("want a list of %s names: %w", what, err)
	}
	var bits T
	for _, name := range list {
		bit, ok := names[name]
		if !ok {
			return fmt.Errorf("unknown %s %q", what, name)
		}
		bits |= bit
	}
	*out = bits
	return nil
}
