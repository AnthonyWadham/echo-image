// Package bake turns a map drawn in Tiled into the map file pkg/level reads, and writes the Tiled project
// whose classes are a game's kinds and object types. It reads Tiled's JSON itself, so no editor runs in
// CI. Like pkg/level, it names no game's kinds or types: the game passes its Config.
package bake

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
)

// Config is how a game bakes its maps.
type Config struct {
	Schema level.Schema
	// Edges are the kinds drawn around the play area: each of their shapes becomes an edge loop. Every
	// other kind's shape is a solid.
	Edges []string
	// Check is the game's rules for a built map, as its shard runs them at boot. Nil checks nothing.
	Check func(m level.Map, world *box2d.World) error
}

// Bake returns the map file for a Tiled map (.tmj), checked as a shard loads it and by c.Check. name
// names the map in errors. Each object layer's objects bake in order, hidden or not, each by its class
// (Tiled saves it as type): a kind's shape becomes a solid or an edge, an object type's point an object.
func Bake(name string, tmj []byte, c Config) ([]byte, error) {
	if err := c.check(); err != nil {
		return nil, err
	}
	m, err := read(tmj, c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	data, err := level.Encode(m, c.Schema)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	// From here, what a shard does with the file.
	if m, err = level.Decode(data, c.Schema); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	def := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&def)
	if _, err := level.Build(world, m, c.Schema); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if c.Check != nil {
		if err := c.Check(m, world); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return data, nil
}

// check returns an error unless each edge kind is a kind, and no name is both a kind and a type: Tiled
// has one class per name.
func (c Config) check() error {
	var errs []error
	for _, kind := range c.Edges {
		if _, ok := c.Schema.Kinds[kind]; !ok {
			errs = append(errs, fmt.Errorf("bake: edge kind %q isn't one of the schema's kinds", kind))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(c.Schema.Types)) {
		if _, ok := c.Schema.Kinds[name]; ok {
			errs = append(errs, fmt.Errorf("bake: %q is both a kind and an object type", name))
		}
	}
	return errors.Join(errs...)
}
