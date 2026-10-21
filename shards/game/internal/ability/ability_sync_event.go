package ability

import "github.com/argus-labs/world-engine/pkg/cardinal"

// AbilitySync is one player's view of their team's kits this tick. Enemy kits are never sent, so an
// enemy can't time your next cast. Sent every tick and complete.
type AbilitySync struct {
	Tick      uint64
	Abilities cardinal.Slice[AbilityState]
}

// Name is the event name.
func (AbilitySync) Name() string { return "ability_sync" }

// AbilityState is one ability in an allied Body's kit. It can be cast from ReadyTick on.
type AbilityState struct {
	Body      uint32 // the Body's EntityID
	Slot      uint32 // input button Slot casts it
	Ability   string // ability ID
	ReadyTick uint64
}
