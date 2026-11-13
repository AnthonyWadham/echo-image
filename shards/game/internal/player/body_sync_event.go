package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics2dcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"

	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/player/component"
)

// BodySync is one player's view of the Bodies this tick, plus their own prediction data. Server time
// is Tick * TickSeconds; clients interpolate on it and predict their own Body in steps of TickSeconds.
// Sent every tick and complete, so a client that joins or reconnects needs nothing else.
type BodySync struct {
	Tick        uint64
	TickSeconds float64
	SimVersion  string // the sim this shard runs (echoimage.SimVersion): a client predicts only with the same
	MapID       string // the live map: content/arena/<MapID>.level.json, which SimVersion pins
	You         YouState
	Bodies      cardinal.Slice[BodyState]
}

// Name is the event name.
func (BodySync) Name() string { return "body_sync" }

// YouState is the receiving player's own data, for client-side prediction: everything the predictor
// starts from, in one event. InputAck and InputQueued count only for the client run whose
// Input.Session matches InputSession.
type YouState struct {
	Body         uint32                        // your Body's EntityID
	Team         uint8                         // your Body's row, which the predictor starts from: its team,
	X, Y         float64                       // spot
	Radius       float64                       // and collider radius
	InputSession uint32                        // Input.Session the ack belongs to
	InputAck     uint32                        // Seq of the last applied frame, included in your Body's position
	InputQueued  uint32                        // frames waiting for later ticks: the client paces its frames by it
	InputCap     uint32                        // the most frames the queue holds: a bigger Input is dropped
	InputTarget  uint32                        // frames to keep queued: it rises each time your queue runs dry
	InputHealth  component.InputHealth         // what went wrong with your input stream, for your HUD
	MoveSpeed    float64                       // units per second at full input
	SnapSeconds  float64                       // a correction longer than this much of MoveSpeed jumps
	FadeSeconds  float64                       // a shorter one shrinks to 1/e in this long
	Champion     string                        // the champion your Body plays: its kit
	Cooldowns    cardinal.Slice[cast.Cooldown] // when each ability your Body has cast is ready again
	Effects      cardinal.Slice[status.Entry]  // your Body's timed effects, which the predictor folds itself
	EffectsNext  uint32                        // the last serial your Body's effect list gave out
	Shots        cardinal.Slice[ShotState]     // the shots in flight your predictor flies: your frames cast them
	EchoDelay    uint32                        // ticks your Echo trails your Body: 0 with no Echo
	EchoSerial   uint64                        // your Echo's serial, its row's in target_sync: 0 with no Echo
	// EchoPath is your Echo's spot this tick, then where it will be on each next tick, up to
	// echo_path_seconds ahead: your predictor draws it at your predicted tick. Empty the tick it dies.
	EchoPath cardinal.Slice[physics2dcomp.Vec2]
}

// ShotState is one of your shots in flight: what the predictor flies it on with. Ability, Seq and
// Launch are its key: the ability and input frame that cast it, and its place among that cast's
// launches. It flew this tick from PrevX, PrevY to X, Y.
type ShotState struct {
	Type         string // shot type ID: its rules
	Ability      string
	Seq          uint32
	Launch       uint32
	PrevX, PrevY float64
	X, Y         float64
	VelX, VelY   float64 // units per second
	Radius       float64
	Remaining    float64 // distance left before it expires
}

// BodyState is what a player Body's row in history.TargetSync lacks: who plays it and how it looks.
// Its row has its team, spot, size and tags.
type BodyState struct {
	EntityID uint32
	Serial   uint64 // its ref.Spawn serial, as its row's
	PlayerID uint32
	Nickname string
	Champion string                       // what it looks like: a client keys its visuals by champion ID
	Effects  cardinal.Slice[status.Shown] // its effect kinds from the next tick on, for their visuals
}
