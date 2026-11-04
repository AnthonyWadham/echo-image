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

// Each Body and Echo is tracked, as its client predicts it. Its track keeps the rows every view the
// window allows is drawn from, one per tick in a ring of KeptRows, and its newest row is what this
// tick's target_sync shows of it: what screens draw, and what predictors place it by, as the server's
// shots meet it. The sync shows every Body and Echo; body_sync and echo_sync send only what rows lack.
func TestTrackIsWhatSyncsShow(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	fx.effect(yourBody(t, bodiesFor(t, w, "bob")), effectcomp.EffectIntent{Kind: "stun", Ticks: 3})
	keep := int(tuningOf[track.TrackTuning](t).Window.KeptRows())
	stunned := false
	check := func(what string, tr trackcomp.Track, want past.Row, full bool) {
		t.Helper()
		rows := tr.AppendRows(nil, want.Tick+1)
		if !tr.Predicted || tr.Rows.Len() != keep || len(rows) == 0 || full && len(rows) != keep ||
			rows[len(rows)-1] != want {
			t.Fatalf("%s: predicted %v, %d slots, %d rows ending %v, want predicted, %d of each, ending %+v", what,
				tr.Predicted, tr.Rows.Len(), len(rows), rows[max(len(rows)-1, 0):], keep, want)
		}
		for i, r := range rows {
			if r.Tick != want.Tick-uint64(len(rows)-1-i) {
				t.Fatalf("%s: row %d is tick %d, want one row per tick up to %d", what, i, r.Tick, want.Tick)
			}
		}
	}
	for i := range keep + 4 {
		bob.move(0, 1) // stunned at first, then he walks
		w.Tick()
		bodies, echoes := bodiesFor(t, w, "alice"), sentTo[echo.EchoSync](t, w, "alice")
		targets := sentTo[track.TargetSync](t, w, "alice")
		if targets.Tick != bodies.Tick || targets.Targets.Len() != bodies.Bodies.Len()+echoes.Echoes.Len() {
			t.Fatalf("target_sync %+v, want tick %d's %d Bodies and Echoes", targets, bodies.Tick,
				bodies.Bodies.Len()+echoes.Echoes.Len())
		}
		for s := range targets.Targets.Values() {
			stunned = stunned || s.Kind == interact.Body && s.Tags&interact.Stunned != 0
			e := w.Entity(cardinal.EntityID(s.EntityID))
			check(fmt.Sprintf("target %d", s.EntityID), e.Get[trackcomp.Track](), past.Row{Tick: targets.Tick,
				Kind: s.Kind, Side: interact.Side{Owner: s.Owner, Team: s.Team}, Center: box2d.Vec2{X: s.X, Y: s.Y},
				Radius: s.Radius, Tags: s.Tags, Until: s.Until}, i >= keep)
			if s.Serial != e.Get[ref.Spawn]().Serial || s.Owner != uint32(e.Get[team.Owner]().Body) {
				t.Fatalf("target_sync sent %+v, want its serial and owner", s)
			}
		}
		for b := range bodies.Bodies.Values() { // what the rows lack, of each
			rowOf(t, w, "alice", b.Serial)
		}
		for e := range echoes.Echoes.Values() {
			rowOf(t, w, "alice", e.Serial)
		}
	}
	if !stunned {
		t.Fatal("no sync showed bob stunned")
	}
}
