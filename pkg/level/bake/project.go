package bake

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/goccy/go-json"
)

// colors are the classes' colors in Tiled, in turn: #AARRGGBB.
const colors = "#ff8a95a5 #ffd9a441 #ff5fb3a1 #ffc76a8c #ff7d8fd6 #ffb5c35b"

// The Tiled property types a props field may be.
const (
	tiledBool   = "bool"
	tiledInt    = "int"
	tiledFloat  = "float"
	tiledString = "string"
)

// Project returns the Tiled project (.tiled-project) for c: a class for each kind but the fills, to draw
// shapes with, and one for each object type, with a member for each of its props. A designer picks a class from a list
// and sets typed props, so a map names only what the game has. Tiled doesn't reload the file while it's
// open.
func Project(c Config) ([]byte, error) {
	if err := c.check(); err != nil {
		return nil, err
	}
	var classes []any
	palette := strings.Fields(colors)
	class := func(name string, members []any) {
		classes = append(classes, map[string]any{
			"color": palette[len(classes)%len(palette)], "drawFill": true, "id": len(classes) + 1,
			"members": members, "name": name, "type": "class", "useAs": []any{"object"},
		})
	}
	for _, kind := range slices.Sorted(maps.Keys(c.Schema.Kinds)) {
		if !c.fills(kind) {
			class(kind, []any{})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(c.Schema.Types)) {
		fs, err := fields(c.Schema.Types[name])
		if err != nil {
			return nil, err
		}
		members := []any{}
		for _, f := range fs {
			members = append(members, map[string]any{"name": f.name, "type": f.tiled, "value": zero(f.tiled)})
		}
		class(name, members)
	}
	data, err := json.MarshalIndent(map[string]any{
		"automappingRulesFile": "",
		"commands":             []any{},
		"extensionsPath":       "extensions",
		"folders":              []any{"."},
		"properties":           []any{},
		"propertyTypes":        classes,
	}, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// field is one of a props struct's fields, as a Tiled member: its JSON name, and its Tiled type.
type field struct {
	name  string
	index int
	tiled string
}

// tiledType is the Tiled property type of a field of kind k, or "" for one Tiled can't hold.
func tiledType(k reflect.Kind) string {
	switch {
	case k == reflect.Bool:
		return tiledBool
	case k == reflect.String:
		return tiledString
	case k == reflect.Float32 || k == reflect.Float64:
		return tiledFloat
	case k >= reflect.Int && k <= reflect.Uint64: // every int and uint, not uintptr
		return tiledInt
	}
	return ""
}

// zero is a Tiled type's default: a Go field's zero value.
func zero(tiled string) any {
	switch tiled {
	case tiledBool:
		return false
	case tiledString:
		return ""
	}
	return 0
}

// fields returns t's fields that JSON writes, in order, or an error for one Tiled can't hold.
func fields(t reflect.Type) ([]field, error) {
	var fs []field
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		tiled := tiledType(f.Type.Kind())
		if tiled == "" {
			return nil, fmt.Errorf("bake: %v's field %s is a %v: Tiled holds bool, int, float and string", t,
				f.Name, f.Type)
		}
		fs = append(fs, field{name: name, index: i, tiled: tiled})
	}
	return fs, nil
}
