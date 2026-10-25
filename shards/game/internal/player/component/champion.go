package component

// Champion is the champion a Body plays. Each feature keeps that champion's numbers under its ID.
type Champion struct {
	ID string // a key of player.MovementTuning.Champions
}

// Name is the ECS component name.
func (Champion) Name() string { return "champion" }
