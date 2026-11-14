package levelstate_test

import (
	"crypto/sha256"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/level"
	"echo_image/pkg/levelstate"
)

type spawn struct {
	Team int `json:"team"`
}

var schema = level.Schema{
	Kinds: level.Kinds{"wall": {CategoryBits: 1, MaskBits: math.MaxUint64}},
	Types: level.Types{"spawn": reflect.TypeFor[spawn]()},
}

func pt(x, y float64) level.Point { return level.Point{int32(x * level.Unit), int32(y * level.Unit)} }

// file returns a map's file: a 20 x 12 m edge of 4 segments, a spawn, and solids.
func file(t *testing.T, solids ...level.Solid) []byte {
	t.Helper()
	edge := []level.Point{pt(-10, -6), pt(-10, 6), pt(10, 6), pt(10, -6)}
	data, err := level.Encode(level.Map{
		Solids:  solids,
		Edges:   []level.Loop{{Src: 1, Kind: "wall", Points: edge}},
		Objects: []level.Object{{ID: 2, Type: "spawn", At: pt(-8, 0), Props: spawn{Team: 1}}},
	}, schema)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// config has two maps: open, the default, is the edge alone (4 shapes), and pillar adds one (5).
func config(t *testing.T) levelstate.Config {
	t.Helper()
	pillar := level.Solid{Src: 3, Kind: "wall", Circle: &level.Circle{Center: pt(0, 0), Radius: level.Unit}}
	return levelstate.Config{
		Maps:    map[string][]byte{"open": file(t), "pillar": file(t, pillar)},
		Schema:  schema,
		Default: "open",
	}
}

// probe is a PreUpdate system after the plugin. It keeps what it saw last tick.
type probe struct {
	plugin  *levelstate.Plugin
	shapes  int
	changed []levelstate.Changed
}

func (s *probe) Run(w *cardinal.World) {
	s.shapes = s.plugin.Engine().Counters().ShapeCount
	s.changed = slices.Collect(w.SystemEvents[levelstate.Changed]())
}

// newWorld registers the plugin first, then the probe, and runs Init.
func newWorld(t *testing.T, cfg levelstate.Config) (*cardinal.TestWorld, *levelstate.Plugin, *probe) {
	t.Helper()
	p, err := levelstate.NewPlugin(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := &probe{plugin: p}
	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		w.RegisterPlugin(p)
		w.RegisterSystem(s, cardinal.WithHook(cardinal.PreUpdate))
	})
	return w, p, s
}

type row struct {
	Current levelstate.Current
}

// holder returns the entity that holds Current.
func holder(t *testing.T, w *cardinal.TestWorld) cardinal.Entity {
	t.Helper()
	e, err := w.Contains[row]().Iter().Single()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func state(t *testing.T, w *cardinal.TestWorld) levelstate.Current {
	t.Helper()
	return holder(t, w).Get[levelstate.Current]()
}

// NewPlugin refuses a map with no ID, a map that doesn't decode, and a default that isn't a map.
func TestNewPluginRefuses(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*levelstate.Config)
		want string
	}{
		{"a map with no ID", func(c *levelstate.Config) { c.Maps[""] = c.Maps["open"] }, "a map has no ID"},
		{"a map that doesn't decode", func(c *levelstate.Config) { c.Maps["broken"] = []byte("{}") }, "map broken: "},
		{"a default that isn't a map", func(c *levelstate.Config) { c.Default = "cave" }, `default map "cave"`},
		{"a map the game's rules refuse", func(c *levelstate.Config) {
			c.Check = func(m level.Map, world *box2d.World) error {
				if len(m.Solids) > 0 {
					return fmt.Errorf("%d shapes, and a solid", world.Counters().ShapeCount)
				}
				return nil
			}
		}, "map pillar: 5 shapes, and a solid"},
	} {
		cfg := config(t)
		c.edit(&cfg)
		if _, err := levelstate.NewPlugin(cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
}

// A plugin keeps one world's live map, so a second world can't register it.
func TestRegisterServesOneWorld(t *testing.T) {
	p, err := levelstate.NewPlugin(config(t))
	if err != nil {
		t.Fatal(err)
	}
	cardinal.NewTestWorld(t, p.Register)
	var got any
	func() {
		defer func() { got = recover() }()
		cardinal.NewTestWorld(t, p.Register)
	}()
	if !strings.Contains(fmt.Sprint(got), "serves one world") {
		t.Errorf("a second Register: got %v, want a panic", got)
	}
}

// Change keeps the live map for the rest of the tick. The next tick's first system swaps it in, raises the
// epoch and emits Changed, so every later system sees the new map and the event.
func TestChangeSwapsFirstInTheNextTick(t *testing.T) {
	cfg := config(t)
	w, p, s := newWorld(t, cfg)
	w.Tick()
	if err := p.Change(w.World, "pillar"); err != nil {
		t.Fatal(err)
	}
	if got := state(t, w); got.ID != "open" || got.Next != "pillar" || p.Engine().Counters().ShapeCount != 4 {
		t.Fatalf("before the next tick: state %+v, want open live and pillar next", got)
	}
	w.Tick()
	want := levelstate.Current{ID: "pillar", Hash: sha256.Sum256(cfg.Maps["pillar"]), Epoch: 2}
	if got := state(t, w); got != want {
		t.Errorf("state %+v, want %+v", got, want)
	}
	if s.shapes != 5 || len(p.Map().Solids) != 1 {
		t.Errorf("a later system saw %d shapes and Map %d solids, want pillar's 5 and 1", s.shapes, len(p.Map().Solids))
	}
	if want := []levelstate.Changed{{Previous: "open", ID: "pillar", Epoch: 2}}; !slices.Equal(s.changed, want) {
		t.Errorf("a later system saw %+v, want %+v", s.changed, want)
	}
	w.Tick()
	if len(s.changed) != 0 {
		t.Errorf("the tick after saw %+v, want nothing", s.changed)
	}
}

// Asking for the live map changes nothing.
func TestChangeToTheLiveMapChangesNothing(t *testing.T) {
	cfg := config(t)
	w, p, s := newWorld(t, cfg)
	if err := p.Change(w.World, "open"); err != nil {
		t.Fatal(err)
	}
	w.Tick()
	want := levelstate.Current{ID: "open", Hash: sha256.Sum256(cfg.Maps["open"]), Epoch: 1}
	if got := state(t, w); got != want || len(s.changed) != 0 {
		t.Errorf("state %+v and %+v, want %+v and nothing", got, s.changed, want)
	}
}

// Change refuses a map the build lacks, and leaves state as it was.
func TestChangeRefusesAMapTheBuildLacks(t *testing.T) {
	cfg := config(t)
	w, p, _ := newWorld(t, cfg)
	if err := p.Change(w.World, "cave"); err == nil || !strings.Contains(err.Error(), `no map "cave"`) {
		t.Errorf("got %v, want an error", err)
	}
	if got, want := state(t, w), (levelstate.Current{
		ID: "open", Hash: sha256.Sum256(cfg.Maps["open"]), Epoch: 1,
	}); got != want {
		t.Errorf("state %+v, want %+v", got, want)
	}
}
