package test

import (
	"bytes"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/history"
	"echo_image/shards/game/internal/history/past"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/vmihailenco/msgpack/v5"
)

// TestWireFixtures pins each payload's bytes in testdata/wire, which the client checks too
// (client/src/game/wire_fixtures.test.ts). A renamed field fails here instead of silently zeroing on
// the other side.
func TestWireFixtures(t *testing.T) {
	in := player.Input{Session: 305419896, Frames: cardinal.SliceOf(
		playercomp.InputFrame{Seq: 1, MoveX: 1},
		playercomp.InputFrame{
			Seq: 2, MoveX: 0.7071067811865476, MoveY: -0.7071067811865476, AimX: 3.5, AimY: -1.25, Buttons: 1,
			ViewTick: 41.375,
		},
	)}
	samples := []struct {
		name    string
		payload any
	}{
		{"body_sync", player.BodySync{
			Tick:        9007199254740993, // 2^53+1, past float64 precision
			TickSeconds: 0.05,
			SimVersion:  "go1.27.1+0123456789abcdef",
			You: player.YouState{
				Body: 1, Team: 1, X: 1.5, Y: -2, Radius: 0.5,
				InputSession: 4294967295, InputAck: 42, InputQueued: 3, InputCap: 10, InputTarget: 4,
				MoveSpeed: 5, SnapSeconds: 0.6, FadeSeconds: 0.125, Champion: "starter",
				InputHealth: playercomp.InputHealth{Coasts: 7, DroppedFrames: 2, OversizedInputs: 1, FlooredViews: 4294967295},
				Cooldowns: cardinal.SliceOf(
					cast.Cooldown{Ability: "bolt", ReadyTick: 9007199254740993},
					cast.Cooldown{Ability: "dash", ReadyTick: 0},
				),
				Effects: cardinal.SliceOf(
					status.Entry{Kind: "slow", Tags: interact.CC, Stat: status.MoveSpeed, Value: -4000, Source: 2,
						Start: 9007199254740993, End: 18446744073709551615, Serial: 1, Stacking: status.PerSource},
					status.Entry{Kind: "dash", Tags: interact.EndsOnCC, Pass: interact.Wall, VelX: -12.5, VelY: 16.25,
						Source: 1, Start: 7, End: 13, Serial: 2},
				),
				EffectsNext: 4294967295,
				Shots: cardinal.SliceOf(player.ShotState{
					Type: "bolt", Ability: "bolt", Seq: 4294967295, Launch: 1, PrevX: -4.75, PrevY: 2.125, X: -4.25, Y: 1.5,
					VelX: 9, VelY: -12, Radius: 0.15, Remaining: 17.75,
				}),
				EchoDelay:  100,
				EchoSerial: 9007199254740993,
				EchoPath:   cardinal.SliceOf(physics2d.Vec2{X: -3.5, Y: 0.25}, physics2d.Vec2{X: -3.25, Y: 0.5}),
			},
			Bodies: cardinal.SliceOf(
				player.BodyState{EntityID: 1, Serial: 1, PlayerID: 1, Nickname: "Player One", Champion: "starter"},
				player.BodyState{EntityID: 2, Serial: 9007199254740993, PlayerID: 2, Nickname: "Échø ✦", Champion: "rook",
					Effects: cardinal.SliceOf(status.Shown{Kind: "out", End: 18446744073709551615},
						status.Shown{Kind: "stun", End: 13})},
			),
		}},
		{"echo_sync", echo.EchoSync{Tick: 7, Echoes: cardinal.SliceOf(
			echo.EchoState{EntityID: 3, Serial: 3, PlayerID: 1,
				Effects: cardinal.SliceOf(status.Shown{Kind: "phase", End: 9007199254740993}), HP: 62.5, MaxHP: 100},
			echo.EchoState{EntityID: 4, Serial: 4, PlayerID: 2, MaxHP: 100},
		)}},
		{"ability_sync", ability.AbilitySync{Tick: 7, Abilities: cardinal.SliceOf(
			ability.AbilityState{Body: 1, Slot: 0, Ability: "bolt", ReadyTick: 18446744073709551615},
		)}},
		{"projectile_sync", projectile.ProjectileSync{Tick: 8, Projectiles: cardinal.SliceOf(
			projectile.ProjectileState{EntityID: 9, Serial: 18446744073709551615, Type: "bolt", Caster: 4294967295,
				Team: 1, PrevX: -0.5, PrevY: -6.75, X: 0.125, Y: -7.5, Radius: 0.15, Until: 1},
			projectile.ProjectileState{EntityID: 10, Serial: 12, Type: "bolt", Caster: 3, Team: 0, PrevX: 2.25, PrevY: 0.5,
				X: 2.5, Y: 1, Radius: 0.15, Until: 0.375, Ended: true, OnTarget: true, TargetSerial: 4, OffsetX: -0.625,
				OffsetY: 0.125},
		)}},
		{"projectile_sync_empty", projectile.ProjectileSync{Tick: 9}},
		{"target_sync", history.TargetSync{Tick: 9007199254740993, Targets: cardinal.SliceOf(
			history.TargetState{EntityID: 0, Serial: 1, Kind: interact.Body, Owner: 0, Team: 0, X: 1.5, Y: -2, Radius: 0.5,
				Tags: interact.Stunned | interact.CC, Until: 1},
			history.TargetState{EntityID: 4, Serial: 18446744073709551615, Kind: interact.Echo, Owner: 2, Team: 1, X: 3,
				Y: -1.25, Radius: 0.5, Until: 0.625},
		)}},
		{"input", in},
	}
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			got, err := msgpack.Marshal(s.payload) // as Cardinal's schema.Serialize does
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "wire", s.name+".msgpack")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%T no longer encodes as %s. If intended, run "+
					"`go test ./shards/game/test -run TestWireFixtures -update` and update its type in client/src/game",
					s.payload, path)
			}
		})
	}

	// The client's own encoding of the input sample, written by client/src/game/wire_fixtures.test.ts.
	t.Run("input_client", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("testdata", "wire", "input_client.msgpack"))
		if err != nil {
			t.Fatal(err)
		}
		var got player.Input
		if err := msgpack.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		gotFrames, wantFrames := slices.Collect(got.Frames.Values()), slices.Collect(in.Frames.Values())
		if got.Session != in.Session || !reflect.DeepEqual(gotFrames, wantFrames) {
			t.Fatalf("the client's Input decodes as Session %d, Frames %+v; want %d, %+v. Update "+
				"client/src/game/player/input_command.ts, then run "+
				"`UPDATE_FIXTURES=1 bun x vitest run src/game/wire_fixtures.test.ts` in client",
				got.Session, gotFrames, in.Session, wantFrames)
		}
	})
}

// viewFixture is testdata/view.json: targets as a few ticks of syncs show them, and where views across
// those ticks draw each.
type viewFixture struct {
	Syncs []viewSync
	Views []viewDrawn
}

type viewSync struct {
	Tick    uint64
	Targets []viewTarget
}

type viewTarget struct {
	EntityID uint32
	Serial   uint64
	X, Y     float64
	Until    float64
}

type viewDrawn struct {
	View  float64
	Drawn []viewSpot
}

type viewSpot struct {
	Serial uint64
	X, Y   float64
}

// TestViewFixture pins where a view draws each target, which the client's screen draws by too
// (client/src/engine/render/entity_views.test.ts), to the bit: a frame's shots meet them there. A view
// draws the syncs at past.RowTicks with past.AtView, by serial, and nothing without both syncs, as
// the predictor places them.
func TestViewFixture(t *testing.T) {
	at := func(id uint32, serial uint64, x, y, until float64) viewTarget {
		return viewTarget{EntityID: id, Serial: serial, X: x, Y: y, Until: until}
	}
	// 1 walks; 2 is new at tick 12; 3 dies 0.375 into tick 12; 4 dies as tick 11 begins, so it's never
	// drawn, and a new target takes its ID at tick 12, drawn where it first is.
	fixture := viewFixture{Syncs: []viewSync{
		{10, []viewTarget{at(1, 11, 0, 0, 1), at(3, 13, -1, 2, 1), at(4, 14, 7, 7, 1)}},
		{11, []viewTarget{at(1, 11, 0.1, 0.7, 1), at(3, 13, -2.2, 2.5, 1), at(4, 14, 7, 7, 0)}},
		{12, []viewTarget{at(1, 11, 2.5, 1.3, 1), at(2, 12, 5, 5, 1), at(3, 13, -3, 4.1, 0.375), at(4, 15, -6, 1, 1)}},
		{13, []viewTarget{at(1, 11, 3, 3.3, 1), at(2, 12, 5.5, 4, 1), at(4, 15, -5.5, 1.5, 1)}},
	}}
	syncAt := func(tick uint64) (map[uint64]past.Row, []viewTarget, bool) {
		for _, s := range fixture.Syncs {
			if s.Tick == tick {
				rows := map[uint64]past.Row{}
				for _, x := range s.Targets {
					rows[x.Serial] = past.Row{Tick: tick, Center: box2d.Vec2{X: x.X, Y: x.Y}, Until: x.Until}
				}
				return rows, s.Targets, true
			}
		}
		return nil, nil, false
	}
	// Before the first two syncs, through each tick's middle and end, at a death, and past the newest.
	for _, view := range []float64{9.75, 10, 10.3, 11, 11.2, 11.375, 11.5, 12, 12.7, 13, 13.25} {
		drawn := viewDrawn{View: view, Drawn: []viewSpot{}}
		a, b := past.RowTicks(view)
		older, _, hasOlder := syncAt(a)
		newer, targets, _ := syncAt(b)
		if !hasOlder {
			targets = nil
		}
		for _, x := range targets {
			o, ok := older[x.Serial]
			if row, ok := past.AtView(o, ok, newer[x.Serial], view); ok {
				drawn.Drawn = append(drawn.Drawn, viewSpot{Serial: x.Serial, X: row.Center.X, Y: row.Center.Y})
			}
		}
		fixture.Views = append(fixture.Views, drawn)
	}
	got, err := json.MarshalIndent(fixture, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "view.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s no longer matches how a view draws (%v). If intended, run "+
			"`go test ./shards/game/test -run TestViewFixture -update`, then make the client's draw match", path, err)
	}
}
