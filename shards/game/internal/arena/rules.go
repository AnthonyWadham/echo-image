package arena

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/mover"
	"echo_image/shards/game/internal/player/motion"
)

// cell is the side of the grid the rules sample, in meters: the landing grid's step, so each spot a
// landing tries from a cell is a cell too.
const cell = mover.LandStep

var (
	// blocked is where a Body can't rest: what stops it, and the walls' fill.
	blocked = box2d.QueryFilter{CategoryBits: uint64(interact.Body),
		MaskBits: uint64(interact.Solid(motion.Blockers))}
	// walls is what a pass lets a Body into: all but the edge.
	walls = box2d.QueryFilter{CategoryBits: uint64(interact.Body),
		MaskBits: uint64(interact.Solid(motion.Blockers) &^ interact.Bounds)}
	// passing is what stops a Body that a pass lets through walls: the edge.
	passing = box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(interact.Bounds)}
)

// Rules is the game's rules for a built map, as a shard runs them at boot and the baker before it writes
// a file: a spawn for each of players, how many start on each team, and CheckMap for each radius.
func Rules(players map[uint8]int, radii []float64) func(level.Map, *box2d.World) error {
	return func(m level.Map, world *box2d.World) error {
		errs := []error{CheckSpawns(m, players)}
		for _, r := range radii {
			errs = append(errs, CheckMap(m, world, r))
		}
		return errors.Join(errs...)
	}
}

// CheckMap returns an error unless a Body of radius can play all of m, built into world: it has room at
// each spawn, walks from the first spawn to every other and never off the map, finds no open space it
// can't walk to, and lands where it walks from anywhere in a wall a pass lets it into. The rules sample
// a grid, so a gap narrower than a cell can slip through.
func CheckMap(m level.Map, world *box2d.World, radius float64) error {
	spawns := level.Objects[Spawn](m)
	if len(spawns) == 0 {
		return errors.New("arena: the map has no spawn")
	}
	walk, err := level.NewGrid(world, m, cell, radius, blocked)
	if err != nil {
		return err
	}
	first, _ := walk.Nearest(spawns[0].At.Vec2())
	if !walk.Open(first) { // nothing to walk from
		return fmt.Errorf("arena: the first spawn, at %s, has no room for a Body", at(spawns[0].At.Vec2()))
	}
	in := walk.Flood([]int{first})
	var errs []error
	for _, s := range spawns {
		p := s.At.Vec2()
		k, _ := walk.Nearest(p)
		switch {
		case mover.Overlaps(world, p, radius, blocked):
			errs = append(errs, fmt.Errorf("arena: the spawn at %s has no room for a Body", at(p)))
		case !in[k]:
			errs = append(errs, fmt.Errorf("arena: a Body can't walk from the first spawn to the one at %s", at(p)))
		}
	}
	for _, k := range walk.Border() {
		if in[k] {
			errs = append(errs, errors.New("arena: a Body walks off the map: its edge is missing or has a gap"))
			break
		}
	}
	out := walk.Flood(walk.Border())
	cut, cutAt := 0, box2d.Vec2{}
	for k := range walk.Len() {
		if walk.Open(k) && !in[k] && !out[k] {
			if cut++; cut == 1 {
				cutAt = walk.Center(k)
			}
		}
	}
	if cut > 0 {
		errs = append(errs, fmt.Errorf("arena: no Body walks to the open space %s: a room walled off, or a "+
			"wall outline with no fill", spots(cut, cutAt)))
	}
	return errors.Join(append(errs, landings(m, world, radius, walk, in, spawns[0].At.Vec2()))...)
}

// landings returns an error unless a Body of radius, inside a wall wherever a pass takes it, lands on a
// cell of walk that in says it walks to.
func landings(m level.Map, world *box2d.World, radius float64, walk *level.Grid, in []bool, spawn box2d.Vec2) error {
	cells := inWalls(world, walk)
	if len(cells) == 0 {
		return nil
	}
	free, err := level.NewGrid(world, m, cell, radius, passing)
	if err != nil {
		return err
	}
	first, _ := free.Nearest(spawn)
	reach := free.Flood([]int{first})
	stuck, stuckAt := 0, box2d.Vec2{}
	for _, k := range cells {
		c := walk.Center(k)
		if f, _ := free.Nearest(c); !reach[f] {
			continue
		}
		// walk's cells say where the Body fits: the same query Land makes there.
		to, ok := motion.LandWhere(world, c, func(p box2d.Vec2) bool {
			at, on := walk.Nearest(p)
			return on && walk.Open(at)
		})
		if l, _ := walk.Nearest(to); !ok || !in[l] {
			if stuck++; stuck == 1 {
				stuckAt = c
			}
		}
	}
	if stuck > 0 {
		return fmt.Errorf("arena: a Body in the wall %s can't land where Bodies walk: no room within %v m, or "+
			"only a walled-off room", spots(stuck, stuckAt), mover.LandReach)
	}
	return nil
}

// inWalls returns walk's cells whose centres are inside a wall, in order: a Box2D query for the walls,
// then each cell in a wall's box.
func inWalls(world *box2d.World, walk *level.Grid) []int {
	seen := map[int]bool{}
	all := box2d.AABB{LowerBound: walk.Center(0), UpperBound: walk.Center(walk.Len() - 1)}
	world.OverlapAABB(all, walls, func(id box2d.ShapeID, _ any) bool {
		box := world.ShapeAABB(id)
		for j := math.Ceil(box.LowerBound.Y / cell); j <= math.Floor(box.UpperBound.Y/cell); j++ {
			for i := math.Ceil(box.LowerBound.X / cell); i <= math.Floor(box.UpperBound.X/cell); i++ {
				c := box2d.Vec2{X: i * cell, Y: j * cell}
				k, ok := walk.Nearest(c)
				if _, done := seen[k]; ok && !done {
					seen[k] = !walk.Open(k) && mover.Inside(world, c, walls)
				}
			}
		}
		return true
	}, nil)
	var cells []int
	for k, in := range seen {
		if in {
			cells = append(cells, k)
		}
	}
	slices.Sort(cells)
	return cells
}

// at is p for an error, in meters.
func at(p box2d.Vec2) string { return fmt.Sprintf("(%g, %g)", p.X, p.Y) }

// spots is n spots from first, for an error.
func spots(n int, first box2d.Vec2) string {
	if n == 1 {
		return "at " + at(first)
	}
	return fmt.Sprintf("at %s and %d more spots", at(first), n-1)
}
