package flight

import (
	"encoding/json"
	"fmt"

	"echo_image/shards/game/internal/interact"
)

// Tuning is the shot types and their rules, loaded from content/projectile/tuning.json. What a shot
// does when it meets a thing is a rule: the shot type's own rules first, then the defaults, and the
// first match wins.
type Tuning struct {
	Defaults []Meet          `json:"defaults"` // the GDD's rules (§7), under every shot type's own
	Shots    map[string]Shot `json:"shots"`    // by shot type ID
}

// Shot is a shot type.
type Shot struct {
	Speed  float64 `json:"speed"`  // world units per second
	Range  float64 `json:"range"`  // world units flown before it expires
	Radius float64 `json:"radius"` // world units
	Damage float64 `json:"damage"` // to a thing a payload rule hits
	Meets  []Meet  `json:"meets"`  // its own rules, checked before the defaults
}

// Meet is what a shot does when it meets a thing its filter matches.
type Meet struct {
	interact.Filter
	Do      Response `json:"do"`
	Payload bool     `json:"payload"` // the thing takes the shot's damage
}

// Response is what a shot does when it meets a thing.
type Response uint8

const (
	_    Response = iota // content must name one
	Pass                 // flies on as if the thing weren't there
	Stop                 // ends at the thing
)

// meetable is the kinds a shot can meet: every kind of thing, but shots and area queries. A thing is a
// target, met by whose it is (its side and tags), because it keeps rows (past.Row), whatever its kind.
const meetable = ^(interact.Shot | interact.Area)

// JSONFile is the path within the content embed.FS.
func (Tuning) JSONFile() string { return "projectile/tuning.json" }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t Tuning) Validate() error {
	if err := validMeets("defaults", t.Defaults); err != nil {
		return err
	}
	for id, s := range t.Shots {
		if s.Speed <= 0 || s.Range <= 0 || s.Radius <= 0 || s.Damage < 0 {
			return fmt.Errorf("projectile_tuning: %q needs speed, range, radius > 0 and damage >= 0, got %+v", id, s)
		}
		if err := validMeets(id, s.Meets); err != nil {
			return err
		}
	}
	return nil
}

func validMeets(of string, meets []Meet) error {
	for i, m := range meets {
		switch {
		case m.Do == 0:
			return fmt.Errorf("projectile_tuning: %s rule %d needs a do", of, i)
		case m.Do == Pass && m.Payload:
			return fmt.Errorf("projectile_tuning: %s rule %d passes with a payload, so it would hit every tick", of, i)
		case m.Payload && m.Relations == 0:
			// A shot type's own rules come before the defaults' own and ally pass, so say who it hits.
			return fmt.Errorf(`projectile_tuning: %s rule %d carries a payload but names no relations, so it `+
				`would hit your own and your allies' too: name them, e.g. ["enemy"]`, of, i)
		case m.Kinds&^meetable != 0:
			return fmt.Errorf("projectile_tuning: %s rule %d names a kind a shot never meets", of, i)
		}
	}
	return nil
}

// meet returns what a shot of type s does on meeting a thing, and the index of the rule that says so,
// which breaks ties. A thing no rule matches is passed.
func (t Tuning) meet(s Shot, k interact.Kind, r interact.Relation, tags interact.Tags) (Meet, int) {
	if m, i, ok := interact.First(s.Meets, k, r, tags); ok {
		return m, i
	}
	if m, i, ok := interact.First(t.Defaults, k, r, tags); ok {
		return m, len(s.Meets) + i
	}
	return Meet{Do: Pass}, -1
}

// rule is the rule meet's index i names.
func (t Tuning) rule(s Shot, i int) Meet {
	if i < len(s.Meets) {
		return s.Meets[i]
	}
	return t.Defaults[i-len(s.Meets)]
}

// UnmarshalJSON reads a response name. An unknown one fails, so bad content fails the boot.
func (r *Response) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	switch name {
	case "pass":
		*r = Pass
	case "stop":
		*r = Stop
	default:
		return fmt.Errorf("unknown response %q", name)
	}
	return nil
}
