// Package area runs areas: rules on an entity's shape that act, every tick, on the things overlapping
// it. An Echo's anti-camp is an area (GDD §3). The first rule that matches a thing decides what
// happens, so a new area is content, not code. A holder gets its area from its own constructor, so area
// never imports the features that hold areas.
package area
