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

func main() {
	c, err := predict.Load(content.FS)
	if err != nil {
		js.Global().Set("echoPredictorError", err.Error())
		return
	}
	p := predict.New(c)
	js.Global().Set("echoPredictor", js.ValueOf(map[string]any{
		"reset": js.FuncOf(func(_ js.Value, args []js.Value) any {
			p.Reset(state(args[0]))
			return nil
		}),
		"peek": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return point(p.Peek(args[0].Float(), args[1].Float()))
		}),
		"step": js.FuncOf(func(_ js.Value, args []js.Value) any {
			at, claims, ended := p.Step(frame(args[0]))
			out := []any{}
			for _, c := range claims {
				out = append(out, map[string]any{"Seq": c.Seq, "Ability": c.Ability, "Launch": c.Launch, "Target": c.Target})
			}
			ends := []any{}
			for _, e := range ended { // drawn from where the tick found it to where it ended
				ends = append(ends, map[string]any{
					"Type": e.Type, "X": e.End.At.X, "Y": e.End.At.Y, "PrevX": e.Pos.X, "PrevY": e.Pos.Y,
					"Radius": e.Radius, "Until": e.End.Until,
				})
			}
			return map[string]any{"x": at.X, "y": at.Y, "claims": out, "ended": ends}
		}),
		"replay": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return point(p.Replay(frame(args[0])))
		}),
		"seeBodies": js.FuncOf(func(_ js.Value, args []js.Value) any {
			p.See(uint64Of(args[0].Get("Tick")), interact.Body, sights(args[0].Get("Bodies"), false))
			return nil
		}),
		"seeEchoes": js.FuncOf(func(_ js.Value, args []js.Value) any {
			p.See(uint64Of(args[0].Get("Tick")), interact.Echo, sights(args[0].Get("Echoes"), true))
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
					"Type": s.Type, "X": s.Pos.X, "Y": s.Pos.Y, "PrevX": s.Prev.X, "PrevY": s.Prev.Y, "Radius": s.Radius,
				})
			}
			return shots
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
	out := predict.Frame{
		Seq:   uint32(f.Get("Seq").Int()),
		MoveX: f.Get("MoveX").Float(), MoveY: f.Get("MoveY").Float(),
		AimX: f.Get("AimX").Float(), AimY: f.Get("AimY").Float(),
		Buttons:  uint32(f.Get("Buttons").Int()),
		ViewTick: f.Get("ViewTick").Float(),
	}
	claims := f.Get("Claims")
	for i := range claims.Length() {
		c := claims.Index(i)
		key := predict.Key{
			Seq: uint32(c.Get("Seq").Int()), Ability: c.Get("Ability").String(), Launch: uint32(c.Get("Launch").Int()),
		}
		out.Claims = append(out.Claims, predict.Claim{Key: key, Target: uint32(c.Get("Target").Int())})
	}
	return out
}

// sights reads a body_sync's Bodies or an echo_sync's Echoes, as predictor.ts sends them. A Body owns
// itself and lives the whole tick; an Echo names its owner and how much of the tick it lived.
func sights(list js.Value, echoes bool) []predict.Sight {
	var out []predict.Sight
	for i := range list.Length() {
		v := list.Index(i)
		id := uint32(v.Get("EntityID").Int())
		owner, until := id, 1.0
		if echoes {
			owner, until = uint32(v.Get("Owner").Int()), v.Get("Until").Float()
		}
		out = append(out, predict.Sight{
			ID:     id,
			Side:   interact.Side{Owner: owner, Team: uint8(v.Get("Team").Int())},
			Center: box2d.Vec2{X: v.Get("X").Float(), Y: v.Get("Y").Float()},
			Radius: v.Get("Radius").Float(),
			Tags:   interact.Tags(uint64Of(v.Get("Tags"))),
			Until:  until,
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
		}})
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
