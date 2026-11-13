package level

import (
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// A Schema is what a game's maps may hold: a Box2D filter for each kind of shape, and a props struct
// for each type of object. The library never names either.
type Schema struct {
	Kinds Kinds
	Types Types
}

// Kinds maps each kind name a map may use to the Box2D filter its shapes get. The game owns the bits.
type Kinds map[string]box2d.Filter

// Types maps each object type name a map may use to the struct its props decode into, strictly. Get a
// struct's type with [reflect.TypeFor]. Each type has a struct of its own.
type Types map[string]reflect.Type

// Placed is an object whose props are a T.
type Placed[T any] struct {
	ID    uint32
	At    Point
	Props T
}

// Objects returns m's objects whose props are a T, in file order.
func Objects[T any](m Map) []Placed[T] {
	var placed []Placed[T]
	for _, o := range m.Objects {
		if props, ok := o.Props.(T); ok {
			placed = append(placed, Placed[T]{ID: o.ID, At: o.At, Props: props})
		}
	}
	return placed
}

// check returns the first name that breaks the rule for names, props type that isn't a struct, or
// struct two types share.
func (s Schema) check() error {
	for _, kind := range slices.Sorted(maps.Keys(s.Kinds)) {
		if err := checkName("kind", kind); err != nil {
			return fmt.Errorf("level: schema: %w", err)
		}
	}
	owner := map[reflect.Type]string{}
	for _, name := range slices.Sorted(maps.Keys(s.Types)) {
		if err := checkName("type", name); err != nil {
			return fmt.Errorf("level: schema: %w", err)
		}
		t := s.Types[name]
		if t == nil || t.Kind() != reflect.Struct {
			return fmt.Errorf("level: schema: type %s: props are a %v, want a struct", name, t)
		}
		if other, ok := owner[t]; ok {
			return fmt.Errorf("level: schema: types %s and %s share the struct %v", other, name, t)
		}
		owner[t] = name
	}
	return nil
}

// checkName takes 1 to 32 lowercase letters, digits and underscores, starting with a letter.
func checkName(what, name string) error {
	ok := len(name) >= 1 && len(name) <= 32 && name[0] >= 'a' && name[0] <= 'z'
	for _, r := range name {
		ok = ok && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	}
	if !ok {
		return fmt.Errorf("%s %q: want a-z, 0-9 and _, starting with a letter, up to 32", what, name)
	}
	return nil
}
