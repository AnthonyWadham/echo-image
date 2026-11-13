// Package levelstate keeps one live map in a Cardinal world. State names it. Memory holds every map the
// build ships, each decoded, checked and built into a Box2D world of its own at boot, so a change is a
// swap.
//
// One system runs at Init and first in every tick. It takes a requested change, refuses a map the build
// lacks, takes the build's file for the live map, and points memory at it. When a tick changes the live
// map, it emits Changed.
package levelstate

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/level"
)

// Current is the live map, in state. One entity holds it.
type Current struct {
	// ID names the live map.
	ID string
	// Hash is the SHA-256 of the file it was built from.
	Hash [sha256.Size]byte
	// Epoch counts changes of map or file, from 1.
	Epoch uint32
	// Next is the map Change asked for, live from the next tick, or "".
	Next string
}

// Name is the component name.
func (Current) Name() string { return "levelstate_current" }

// Changed is emitted by the first system of a tick that changes the live map: after Change, or after a
// restore whose map file differs in this build.
type Changed struct {
	// Previous names the map live before, or is "" if none was.
	Previous string
	// ID names the live map.
	ID string
	// Epoch is the live map's.
	Epoch uint32
}

// Name is the system event name.
func (Changed) Name() string { return "levelstate_changed" }

// Config is a game's maps: each file by map ID, the schema they're read with, and the map live at boot.
type Config struct {
	Maps    map[string][]byte
	Schema  level.Schema
	Default string
}

// Plugin keeps the live map of one world.
type Plugin struct {
	maps       map[string]*built
	defaultID  string
	live       *built
	registered bool
}

// built is one map, decoded and built.
type built struct {
	m     level.Map
	world *box2d.World
	hash  [sha256.Size]byte
}

// NewPlugin decodes each map and builds it into a Box2D world of its own. It returns the first error, in
// ID order: a map with no ID, a map that doesn't decode, or a default that isn't one of the maps.
func NewPlugin(config Config) (*Plugin, error) {
	p := &Plugin{maps: make(map[string]*built, len(config.Maps)), defaultID: config.Default}
	for _, id := range slices.Sorted(maps.Keys(config.Maps)) {
		if id == "" {
			return nil, errors.New("levelstate: a map has no ID")
		}
		data := config.Maps[id]
		m, err := level.Decode(data, config.Schema)
		if err != nil {
			return nil, fmt.Errorf("levelstate: map %s: %w", id, err)
		}
		worldDef := box2d.DefaultWorldDef()
		world := box2d.NewWorld(&worldDef)
		if _, err := level.Build(world, m, config.Schema); err != nil {
			return nil, fmt.Errorf("levelstate: map %s: %w", id, err)
		}
		p.maps[id] = &built{m: m, world: world, hash: sha256.Sum256(data)}
	}
	live, ok := p.maps[config.Default]
	if !ok {
		return nil, fmt.Errorf("levelstate: default map %q isn't one of the maps", config.Default)
	}
	p.live = live
	return p, nil
}

// Register registers Current, Changed and the system that keeps them, at Init and in PreUpdate. Register
// the plugin before any other PreUpdate system, so the map is live before anything reads it. A plugin
// serves one world: a second call panics.
func (p *Plugin) Register(w *cardinal.World) {
	if p.registered {
		panic(errors.New("levelstate: the plugin serves one world; make another with NewPlugin"))
	}
	w.RegisterComponent[Current]()
	w.RegisterSystemEvent[Changed]()
	w.RegisterSystem(&reconcileSystem{plugin: p}, cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(&reconcileSystem{plugin: p, emit: true}, cardinal.WithHook(cardinal.PreUpdate))
	p.registered = true
}

// Change makes id the live map from the next tick. Asking for the live map changes nothing. It returns an
// error for a map the build lacks.
func (p *Plugin) Change(w *cardinal.World, id string) error {
	if _, ok := p.maps[id]; !ok {
		return fmt.Errorf("levelstate: no map %q", id)
	}
	e := current(w)
	c := e.Get[Current]()
	c.Next = id
	e.Set(c)
	return nil
}

// Engine returns the Box2D world built from the live map, for queries only: never step or change it. The
// first system of a tick may swap it, so get it each tick.
func (p *Plugin) Engine() *box2d.World { return p.live.world }

// Map returns the live map as decoded. Read it only. It lives as long as Engine's world.
func (p *Plugin) Map() level.Map { return p.live.m }
