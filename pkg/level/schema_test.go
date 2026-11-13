package level_test

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"echo_image/pkg/level"
)

// Objects returns the objects of one struct, typed and in file order, and none of another.
func TestObjectsReturnsOneStructInFileOrder(t *testing.T) {
	data, err := level.Encode(arena(), schema)
	if err != nil {
		t.Fatal(err)
	}
	m, err := level.Decode(data, schema)
	if err != nil {
		t.Fatal(err)
	}
	want := []level.Placed[spawn]{
		{ID: 6, At: pt(-8, 0), Props: spawn{Team: 1}},
		{ID: 7, At: pt(8, 0), Props: spawn{Team: 2}},
	}
	if got := level.Objects[spawn](m); !slices.Equal(got, want) {
		t.Errorf("spawns %+v, want %+v", got, want)
	}
	type chest struct{}
	if got := level.Objects[chest](m); len(got) != 0 {
		t.Errorf("chests %+v, want none", got)
	}
}

// A schema's names follow the rule for names, its props are structs, and no two types share one.
func TestSchemaRefuses(t *testing.T) {
	type door struct{}
	for _, c := range []struct {
		name string
		edit func(*level.Schema)
		want string
	}{
		{"a kind in capitals", func(s *level.Schema) { s.Kinds["Glass"] = s.Kinds["wall"] }, `kind "Glass"`},
		{"a type with a space", func(s *level.Schema) {
			s.Types["spawn point"] = reflect.TypeFor[door]()
		}, `type "spawn point"`},
		{"props that aren't a struct", func(s *level.Schema) { s.Types["door"] = reflect.TypeFor[int]() }, "want a struct"},
		{"two types with one struct", func(s *level.Schema) { s.Types["respawn"] = s.Types["spawn"] }, "share the struct"},
	} {
		s := level.Schema{Kinds: maps.Clone(schema.Kinds), Types: maps.Clone(schema.Types)}
		c.edit(&s)
		if _, err := level.Encode(arena(), s); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
}
