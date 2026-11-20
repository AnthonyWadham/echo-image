package area

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/pkg/queue"
	"echo_image/shards/game/internal/area/component"
	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/team"
)

// areaRow is an entity with an area: its pose and shape, and the side it acts for. A neutral holder (a
// map pickup) comes with its first user.
type areaRow struct {
	Area      component.Area
	Team      team.Team
	Owner     team.Owner
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
}

// AreaSystem (Update, after echo.ReplaySystem so each area is where its holder is this tick, and before
// combat.DamageResolveSystem) runs every area that has started. One Box2D overlap finds the things
// touching its holder's first shape, and for each the first rule that matches decides what happens. It
// tests who is inside every tick and never trusts contact events. O(A·(log N + k)) for A areas with k
// things inside.
type AreaSystem struct {
	Tuning  AreaTuning
	Physics *physics2d.Plugin
}

func (s *AreaSystem) Run(w *cardinal.World) {
	tuning := s.Tuning
	tick := w.TickHeight()
	world := s.Physics.Engine()
	var unknown []cardinal.Entity
	for e := range w.Contains[areaRow]().Iter() {
		a := e.Get[component.Area]()
		if tick < a.Start {
			continue
		}
		def, ok := tuning.Kinds[a.Kind]
		if !ok {
			unknown = append(unknown, e)
			continue
		}
		s.act(w, world, e, def, tick)
	}
	// Removed after the search, since it changes the entity's archetype.
	for _, e := range unknown {
		w.Logger().Error().Str("kind", e.Get[component.Area]().Kind).Msg("removed an area of unknown kind")
		e.Remove[component.Area]()
	}
}

// act runs area e's rules on each thing that overlaps its first shape, a circle.
func (s *AreaSystem) act(w *cardinal.World, world *box2d.World, e cardinal.Entity, def KindDef, tick uint64) {
	shape := e.Get[physics2d.PhysicsBody2D]().Shapes.At(0)
	center := physics.WorldPoint(e.Get[physics2d.Transform2D](), shape.LocalOffset)
	side := team.SideOf(e)
	proxy := box2d.MakeProxy([]box2d.Vec2{box2d.Vec2(center)}, 1, shape.Radius)
	filter := box2d.QueryFilter{CategoryBits: uint64(interact.Area), MaskBits: uint64(def.meets())}
	world.OverlapShape(&proxy, filter, func(id box2d.ShapeID, _ any) bool {
		thing := w.Entity(physics.EntityOf(world, id))
		if thing.ID() == e.ID() {
			return true
		}
		kind := interact.Kind(world.ShapeFilter(id).CategoryBits)
		if rule, _, ok := interact.First(def.Rules, kind, relation(side, thing), effectcomp.TagsOf(thing, tick)); ok {
			// Self is the only To so far.
			queue.Send(w, combatcomp.DamageIntent{
				Target: e.ID(), Credit: cardinal.EntityID(side.Owner), Amount: rule.DamagePerSecond * physics.SecondsPerTick,
				Until: 1,
			})
		}
		return true
	}, nil)
}

// relation returns how thing relates to side. A thing that acts for no player is neutral.
func relation(side interact.Side, thing cardinal.Entity) interact.Relation {
	if !thing.Has[team.Owner]() || !thing.Has[team.Team]() {
		return interact.Neutral
	}
	return side.Relation(team.SideOf(thing))
}
