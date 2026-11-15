// Package bake turns a map drawn in Tiled into the map file package level reads.
//
// It reads Tiled's JSON (.tmj) itself, so no editor runs in CI. Like package level, it names no game's
// kinds or types: the game passes a [Config].
//
// # What it makes
//
//   - [Bake] returns the map file for one Tiled map.
//   - [Project] returns the Tiled project, whose classes are the game's kinds and object types. A
//     designer picks a class from a list, so a map names only what the game has.
//
// # How a map is drawn
//
//   - A tile is a meter. The canvas's centre is (0, 0), and y points up in the file.
//   - Each object has a class. A kind's shape becomes a solid or an edge. An object type's point
//     becomes an object, and its properties the type's props.
//   - A rectangle, polygon, circle or capsule is a solid. A kind listed in Config.Edges is the edge
//     around the play area.
//   - An object's rotation turns it clockwise about its origin, as Tiled draws it.
//
// # The steps of a bake
//
//  1. Read the Tiled map, and refuse what a map file can't hold: tiles, templates, text, an older
//     Tiled.
//  2. Turn each object's pixels into whole units, and wind and start each loop one fixed way, so a
//     shape bakes the same however it was drawn.
//  3. Cut each shape Box2D takes as no one polygon (see below).
//  4. Encode the file with package level.
//  5. Decode it, build it and run the game's rules (Config.Check), as a shard does at boot.
//
// An error names the map, the object, its ID and where it is in meters.
//
// # Shapes Box2D takes as no one polygon
//
// Box2D's polygon is convex, with at most 8 corners. A shape that is concave, has more corners, or has
// a corner on a line bakes as two things:
//
//   - its outline, a chain, which stops movement with no seams;
//   - triangles of the kind Config.Fills names for the shape's kind, which cover its inside.
//
// The cut uses only the shape's own corners. It tries every way to cut and takes the first whose
// triangles Box2D all takes. A shape too thin for any is refused: add a corner across from the thin
// part, or draw two shapes. A kind with no fill kind must be drawn convex.
//
// # Files
//
//   - bake.go: Config and Bake.
//   - tiled.go: the Tiled JSON types, and each object's turn into map shapes.
//   - split.go: the cut of a shape into triangles.
//   - project.go: Project.
package bake
