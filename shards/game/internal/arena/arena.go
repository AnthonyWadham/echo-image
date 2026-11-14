// Package arena is the game's side of its maps: the kinds of their shapes, the type of their objects,
// and which maps ship. pkg/level reads and builds a map, and pkg/levelstate keeps the live one. It
// imports no Cardinal, so the predictor can read maps too.
package arena

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"path"
	"reflect"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
	"echo_image/shards/game/internal/interact"
)

// Spawn is where a Body of Team starts: a map object of type spawn. A map holds one for each player.
type Spawn struct {
	Team uint8 `json:"team"`
}

// Spawns returns m's spawn points by team, each team's in file order: its players take them in turn.
func Spawns(m level.Map) map[uint8][]box2d.Vec2 {
	spawns := map[uint8][]box2d.Vec2{}
	for _, s := range level.Objects[Spawn](m) {
		spawns[s.Props.Team] = append(spawns[s.Props.Team], s.At.Vec2())
	}
	return spawns
}

// CheckSpawns returns an error unless m has a spawn for each player, where players is how many start on
// each team.
func CheckSpawns(m level.Map, players map[uint8]int) error {
	spawns := Spawns(m)
	var errs []error
	for _, team := range slices.Sorted(maps.Keys(players)) {
		if have, want := len(spawns[team]), players[team]; have < want {
			errs = append(errs, fmt.Errorf("arena: team %d has spawns for %d of its %d players", team, have, want))
		}
	}
	return errors.Join(errs...)
}

// Schema is what the game's maps may hold: walls and the edge, by their interact kinds, and spawns. Like
// every map shape, each masks all bits, so each query's mask decides whether it stops there.
func Schema() level.Schema {
	return level.Schema{
		Kinds: level.Kinds{"wall": mapKind(interact.Wall), "bounds": mapKind(interact.Bounds)},
		Types: level.Types{"spawn": reflect.TypeFor[Spawn]()},
	}
}

func mapKind(k interact.Kind) box2d.Filter {
	return box2d.Filter{CategoryBits: uint64(k), MaskBits: math.MaxUint64}
}

// ArenaTuning is content/arena/tuning.json: the maps that ship, and the one live at boot.
type ArenaTuning struct {
	Maps       []string `json:"maps"`        // each is content/arena/<id>.level.json
	DefaultMap string   `json:"default_map"` // the map live at boot
}

// JSONFile is the path within the content embed.FS.
func (ArenaTuning) JSONFile() string { return "arena/tuning.json" }

// Files reads each map's file from fsys, by ID. Only pkg/level decodes them.
func (t ArenaTuning) Files(fsys fs.FS) (map[string][]byte, error) {
	files := make(map[string][]byte, len(t.Maps))
	for _, id := range t.Maps {
		data, err := fs.ReadFile(fsys, path.Join("arena", id+".level.json"))
		if err != nil {
			return nil, fmt.Errorf("arena_tuning: map %q: %w", id, err)
		}
		files[id] = data
	}
	return files, nil
}
