package component

// MoveSpeed is how fast a Body moves at full input, in world units per second.
type MoveSpeed struct {
	Value float64
}

// Name is the ECS component name.
func (MoveSpeed) Name() string { return "move_speed" }
