//go:build js && wasm

// Command predictor is internal/predict for the browser, with the server's own content embedded.
// client/scripts/predictor.mjs builds it with the Go toolchain the server uses, and
// client/src/game/player/predictor.ts runs it.
package main

import (
	"strconv"
	"syscall/js"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/predict"
	"echo_image/shards/game/internal/projectile/flight"
)

// simVersion is the echoimage.SimVersion of the tree this was built from: predictor.mjs stamps it with
// -ldflags -X, from ./version.
var simVersion string

func main() {
	c, err := predict.Load(content.FS)
	if err != nil {
		js.Global().Set("echoPredictorError", err.Error())
		return
	}
	p := predict.New(c)
	js.Global().Set("echoPredictor", js.ValueOf(map[string]any{
		"version": simVersion,
		"reset": js.FuncOf(func(_ js.Value, args []js.Value) any {
			p.Reset(state(args[0]))
			return nil
		}),
		"peek": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return point(p.Peek(args[0].Float(), args[1].Float()))
		}),
		"step": js.FuncOf(func(_ js.Value, args []js.Value) any {
			at, ended := p.Step(frame(args[0]))
			ends := []any{}
			for _, e := range ended { // drawn from where the tick found it to where it ended
				ends = append(ends, map[string]any{
					"Type": e.Type, "Seq": e.Seq, "Ability": e.Ability, "Launch": e.Launch,
					"X": e.End.At.X, "Y": e.End.At.Y, "PrevX": e.Pos.X, "PrevY": e.Pos.Y,
					"Radius": e.Radius, "Until": e.End.Until, "OnTarget": e.End.Hit.OnTarget(),
					"TargetSerial": strconv.FormatUint(e.End.Hit.Serial, 10), "OffsetX": e.End.Hit.Offset.X,
					"OffsetY": e.End.Hit.Offset.Y,
				})
			}
			return map[string]any{"x": at.X, "y": at.Y, "ended": ends}
		}),
		"seeTargets": js.FuncOf(func(_ js.Value, args []js.Value) any {
			p.See(uint64Of(args[0].Get("Tick")), sights(args[0].Get("Targets")))
			return nil
		}),
		"forget": js.FuncOf(func(js.Value, []js.Value) any {
			p.Forget()
			return nil
		}),
		"shots": js.FuncOf(func(js.Value, []js.Value) any {
			var shots []any
			for _, s := range p.State().Shots {
				shots = append(shots, map[string]any{
					"Type": s.Type, "Seq": s.Seq, "Ability": s.Ability, "Launch": s.Launch,
					"X": s.Pos.X, "Y": s.Pos.Y, "PrevX": s.Prev.X, "PrevY": s.Prev.Y, "Radius": s.Radius,
				})
			}
			return shots
		}),
		"echo": js.FuncOf(func(js.Value, []js.Value) any {
			at, prev, ok := p.Echo()
			if !ok {
				return nil
			}
			return map[string]any{"X": at.X, "Y": at.Y, "PrevX": prev.X, "PrevY": prev.Y}
		}),
		"kit": js.FuncOf(func(js.Value, []js.Value) any {
			var slots []any
			for _, c := range p.Kit() {
				slots = append(slots, map[string]any{"Ability": c.Ability, "ReadyTick": float64(c.ReadyTick)})
			}
			return map[string]any{"Tick": float64(p.State().Tick), "Slots": slots}
		}),
	}))
	select {} // serves calls until the page closes
}

func point(v box2d.Vec2) map[string]any { return map[string]any{"x": v.X, "y": v.Y} }

// frame reads an InputFrame.
func frame(f js.Value) predict.Frame {
	return predict.Frame{
		Seq:   uint32(f.Get("Seq").Int()),
		MoveX: f.Get("MoveX").Float(), MoveY: f.Get("MoveY").Float(),
		AimX: f.Get("AimX").Float(), AimY: f.Get("AimY").Float(),
		Buttons:  uint32(f.Get("Buttons").Int()),
		ViewTick: f.Get("ViewTick").Float(),
	}
}

// sights reads a target_sync's Targets, as predictor.ts sends them.
func sights(list js.Value) []predict.Sight {
	var out []predict.Sight
	for i := range list.Length() {
		v := list.Index(i)
		out = append(out, predict.Sight{
			ID:     uint32(v.Get("EntityID").Int()),
			Serial: uint64Of(v.Get("Serial")),
			Row: flight.Row{
				Kind:   interact.Kind(uint64Of(v.Get("Kind"))),
				Side:   interact.Side{Owner: uint32(v.Get("Owner").Int()), Team: uint8(v.Get("Team").Int())},
				Center: box2d.Vec2{X: v.Get("X").Float(), Y: v.Get("Y").Float()},
				Radius: v.Get("Radius").Float(),
				Tags:   interact.Tags(uint64Of(v.Get("Tags"))),
				Until:  v.Get("Until").Float(),
			},
		})
	}
	return out
}

// state reads the predict.State that predictor.ts built from a body_sync. A uint64 comes as a decimal
// string, since a JS number can't hold every one.
func state(v js.Value) predict.State {
	s := predict.State{
		Tick:        uint64Of(v.Get("Tick")),
		Body:        uint32(v.Get("Body").Int()),
		Team:        uint8(v.Get("Team").Int()),
		Center:      box2d.Vec2{X: v.Get("X").Float(), Y: v.Get("Y").Float()},
		Radius:      v.Get("Radius").Float(),
		MoveSpeed:   v.Get("MoveSpeed").Float(),
		Champion:    v.Get("Champion").String(),
		EffectsNext: uint32(v.Get("EffectsNext").Int()),
		EchoDelay:   uint32(v.Get("EchoDelay").Int()),
	}
	path := v.Get("EchoPath")
	for i := range path.Length() {
		at := path.Index(i)
		s.EchoPath = append(s.EchoPath, box2d.Vec2{X: at.Get("X").Float(), Y: at.Get("Y").Float()})
	}
	cooldowns := v.Get("Cooldowns")
	for i := range cooldowns.Length() {
		c := cooldowns.Index(i)
		s.Cooldowns = append(s.Cooldowns, cast.Cooldown{
			Ability: c.Get("Ability").String(), ReadyTick: uint64Of(c.Get("ReadyTick")),
		})
	}
	effects := v.Get("Effects")
	for i := range effects.Length() {
		e := effects.Index(i)
		s.Effects = append(s.Effects, status.Entry{
			Kind:   e.Get("Kind").String(),
			Tags:   interact.Tags(uint64Of(e.Get("Tags"))),
			Pass:   interact.Kind(uint64Of(e.Get("Pass"))),
			Stat:   status.Stat(e.Get("Stat").Int()),
			Value:  int32(e.Get("Value").Int()),
			VelX:   e.Get("VelX").Float(),
			VelY:   e.Get("VelY").Float(),
			Source: uint32(e.Get("Source").Int()),
			Start:  uint64Of(e.Get("Start")),
			End:    uint64Of(e.Get("End")),
			Serial: uint32(e.Get("Serial").Int()),
		})
	}
	shots := v.Get("Shots")
	for i := range shots.Length() {
		h := shots.Index(i)
		key := predict.Key{
			Seq: uint32(h.Get("Seq").Int()), Ability: h.Get("Ability").String(), Launch: uint32(h.Get("Launch").Int()),
		}
		s.Shots = append(s.Shots, predict.Shot{Type: h.Get("Type").String(), Key: key, Flight: flight.Flight{
			Pos:       box2d.Vec2{X: h.Get("X").Float(), Y: h.Get("Y").Float()},
			Velocity:  box2d.Vec2{X: h.Get("VelX").Float(), Y: h.Get("VelY").Float()},
			Radius:    h.Get("Radius").Float(),
			Remaining: h.Get("Remaining").Float(),
		}, Prev: box2d.Vec2{X: h.Get("PrevX").Float(), Y: h.Get("PrevY").Float()}})
	}
	return s
}

func uint64Of(v js.Value) uint64 {
	n, err := strconv.ParseUint(v.String(), 10, 64)
	if err != nil {
		panic(err) // predictor.ts always sends a decimal string
	}
	return n
}
