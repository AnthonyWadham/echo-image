package level

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// Build adds m's shapes to world on one new static body at the origin, and returns the body. Shapes go
// in file order (solids, outlines, edges), with the filter the schema gives their kind, and each one's
// user data is its place in that order, from 1; each segment of a loop takes its own place, in point
// order. If m fails Decode's checks, it adds nothing. Call it outside a Box2D step.
func Build(world *box2d.World, m Map, schema Schema) (box2d.BodyID, error) {
	if err := m.check(schema); err != nil {
		return box2d.BodyID{}, err
	}
	bodyDef := box2d.DefaultBodyDef()
	body := world.CreateBody(&bodyDef)
	var place uint64
	for _, s := range m.Solids {
		place++
		def := box2d.DefaultShapeDef()
		def.Filter = schema.Kinds[s.Kind]
		def.UserData = place
		switch {
		case s.Circle != nil:
			circle := box2d.Circle{Center: s.Circle.Center.Vec2(), Radius: meters(s.Circle.Radius)}
			world.CreateCircleShape(body, &def, &circle)
		case s.Capsule != nil:
			c := s.Capsule
			capsule := box2d.Capsule{Center1: c.Center1.Vec2(), Center2: c.Center2.Vec2(), Radius: meters(c.Radius)}
			world.CreateCapsuleShape(body, &def, &capsule)
		default:
			hull := box2d.ComputeHull(vecs(s.Polygon))
			polygon := box2d.MakePolygon(&hull, 0)
			world.CreatePolygonShape(body, &def, &polygon)
		}
	}
	for _, l := range slices.Concat(m.Outlines, m.Edges) {
		def := box2d.DefaultChainDef()
		def.Points = vecs(l.Points)
		def.IsLoop = true
		def.Filter = schema.Kinds[l.Kind]
		segments := make([]box2d.ShapeID, len(l.Points))
		world.ChainSegments(world.CreateChain(body, &def), segments)
		for _, segment := range segments {
			place++
			world.SetShapeUserData(segment, place)
		}
	}
	return body, nil
}
