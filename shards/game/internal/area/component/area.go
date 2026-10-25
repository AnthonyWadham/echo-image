// Package component holds the area feature's ECS data — the leaf a constructor imports to give its
// entity an area without depending on area's systems.
package component

// Area makes its entity's first shape act on the things that overlap it, by the rules of area kind
// Kind, from tick Start on.
type Area struct {
	Kind  string // a key of area.AreaTuning.Kinds
	Start uint64
}

// Name is the ECS component name.
func (Area) Name() string { return "area" }
