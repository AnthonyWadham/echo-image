// Package level reads a baked map file and builds its shapes into a Box2D world.
//
// A map is content, not state. The same bytes build the same shapes on every platform, so a server and
// its clients agree without sending geometry. The package imports only Box2D and the standard library,
// and names no game's kinds or types: the game passes a [Schema].
//
// # What a map holds
//
//   - Solids: convex pieces. Each is one polygon, circle or capsule.
//   - Outlines: closed chains around solid shapes, counter-clockwise. They keep things out.
//   - Edges: closed chains around the play area, clockwise. They keep things in.
//   - Objects: typed points with props, such as start points.
//
// Every point is a whole number of units, [Unit] to a meter, so it is an exact float64.
//
// A shape Box2D takes as no one polygon (an L, say) is an outline plus solids that fill it. The game
// gives those pieces a kind of their own, so a walk meets only the outline.
//
// # How it is used
//
//  1. [Decode] reads a file into a [Map]. It refuses anything [Encode] would not write.
//  2. [Build] adds the map's shapes to a Box2D world, on one static body.
//  3. The game asks that world with Box2D queries. The world is never stepped.
//
// [Encode] writes a Map as a file. The baker (package bake) calls it, and so can a server that hands
// the file on to a client.
//
// # What Decode refuses
//
//   - A file that is not byte for byte what Encode writes: one map has one file.
//   - A kind or object type the schema does not name, or props that do not fit their struct.
//   - A polygon that is not its own convex hull, in the hull's order.
//   - A loop that winds the wrong way, crosses itself, or brings two segments within 5 mm.
//   - A segment or capsule of 5 mm or less (Box2D's LinearSlop), or a point outside Box2D's world.
//
// # Files
//
//   - level.go: the Map types, Decode, Encode and every check.
//   - schema.go: Schema, and [Objects] to read the objects of one type.
//   - build.go: Build.
//   - grid.go: [Grid], which samples a built map in cells so a game's rules can flood it, and
//     [Map.Bounds], the box around a map.
package level
