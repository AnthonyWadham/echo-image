package main

import (
	"fmt"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/echo"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
	"echo_image/shards/game/internal/track"
	trackcomp "echo_image/shards/game/internal/track/component"
	"echo_image/shards/game/internal/track/past"
)

// Each tracked thing is in each tick's target_sync, and only those. Its track keeps the rows every view
// the window allows is drawn from, one per tick in a ring of KeptRows, and its newest row is what the
// sync shows of it: what screens draw, and what predictors place it by, as the server's shots meet it.
// Each Body and Echo is predicted by its client; body_sync and echo_sync send only what rows lack.
func TestTrackIsWhatSyncsShow(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	fx.effect(yourBody(t, bodiesFor(t, w, "bob")), effectcomp.EffectIntent{Kind: "stun", Ticks: 3})
	keep := int(tuningOf[track.TrackTuning](t).Window.KeptRows())
	stunned := false
	check := func(what string, tr trackcomp.Track, want past.Row, full bool) {
		t.Helper()
		rows := tr.AppendRows(nil, want.Tick+1)
		if tr.Rows.Len() != keep || len(rows) == 0 || full && len(rows) != keep || rows[len(rows)-1] != want {
			t.Fatalf("%s: %d slots, %d rows ending %v, want %d of each, ending %+v", what, tr.Rows.Len(), len(rows),
				rows[max(len(rows)-1, 0):], keep, want)
		}
		for i, r := range rows {
			if r.Tick != want.Tick-uint64(len(rows)-1-i) {
				t.Fatalf("%s: row %d is tick %d, want one row per tick up to %d", what, i, r.Tick, want.Tick)
			}
		}
	}
	predicted := func(what string, serial uint64) {
		t.Helper()
		if r := rowOf(t, w, "alice", serial); !w.Entity(cardinal.EntityID(r.EntityID)).Get[trackcomp.Track]().Predicted {
			t.Fatalf("%s %d isn't predicted", what, r.EntityID)
		}
	}
	type tracked struct{ Track trackcomp.Track }
	for i := range keep + 4 {
		bob.move(0, 1) // stunned at first, then he walks
		w.Tick()
		bodies, echoes := bodiesFor(t, w, "alice"), sentTo[echo.EchoSync](t, w, "alice")
		targets := sentTo[track.TargetSync](t, w, "alice")
		sent := map[uint32]track.TargetState{}
		for s := range targets.Targets.Values() {
			sent[s.EntityID] = s
		}
		held := 0
		for e := range w.Contains[tracked]().Iter() {
			held++
			s, ok := sent[uint32(e.ID())]
			if !ok {
				t.Fatalf("tick %d: target_sync lacks tracked thing %d", targets.Tick, e.ID())
			}
			stunned = stunned || s.Kind == interact.Body && s.Tags&interact.Stunned != 0
			check(fmt.Sprintf("target %d", s.EntityID), e.Get[trackcomp.Track](), past.Row{Tick: targets.Tick,
				Kind: s.Kind, Side: interact.Side{Owner: s.Owner, Team: s.Team}, Center: box2d.Vec2{X: s.X, Y: s.Y},
				Radius: s.Radius, Tags: s.Tags, Until: s.Until}, i >= keep)
			if s.Serial != e.Get[ref.Spawn]().Serial || s.Owner != uint32(e.Get[team.Owner]().Body) {
				t.Fatalf("target_sync sent %+v, want its serial and owner", s)
			}
		}
		if targets.Tick != bodies.Tick || targets.Targets.Len() != held {
			t.Fatalf("target_sync %+v, want tick %d's %d tracked things", targets, bodies.Tick, held)
		}
		for b := range bodies.Bodies.Values() { // what the rows lack, of each
			predicted("Body", b.Serial)
		}
		for e := range echoes.Echoes.Values() {
			predicted("Echo", e.Serial)
		}
	}
	if !stunned {
		t.Fatal("no sync showed bob stunned")
	}
}
