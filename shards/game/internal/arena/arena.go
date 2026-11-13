// Package arena is the game's side of its maps: the kinds of their shapes, the type of their objects,
// and which maps ship. pkg/level reads and builds a map, and pkg/levelstate keeps the live one. It
// imports no Cardinal, so the predictor can read maps too.
package arena

import (
	"fmt"
	"io/fs"
	"math"
	"path"
	"reflect"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
	"echo_image/shards/game/internal/interact"
)

// Spawn is where a Body of Team starts: a map object of type spawn.
type Spawn struct {
	Team uint8 `json:"team"`
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
