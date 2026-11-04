package past

import (
	"fmt"

	"echo_image/shards/game/internal/physics/timestep"
)

// Tuning is lag compensation's content, loaded from content/track/tuning.json.
type Tuning struct {
	Window Window `json:"window"`
}

// Window is how far back the view an action is judged at may be: what an honest client's view lags,
// added up. One window drives the client's drawing, the rows kept and the clamp. An older view is
// judged at the oldest, so a slower link leads its shots a little.
type Window struct {
	InterpolationSeconds float64 `json:"interpolation_seconds"` // how far behind the newest tick a client draws
	QueueSeconds         float64 `json:"queue_seconds"`         // how long its frames wait in the server's queue
	PingSeconds          float64 `json:"ping_seconds"`          // the round trip it favors
}

// Seconds is the whole window.
func (w Window) Seconds() float64 { return w.InterpolationSeconds + w.QueueSeconds + w.PingSeconds }

// Ticks converts the window to ticks.
func (w Window) Ticks() uint64 { return uint64(timestep.Ticks(w.Seconds())) }

// KeptRows is how many ticks of rows a target keeps: the oldest view Clamp allows, Ticks back, is
// judged (Across) from the row Ticks back and newer.
func (w Window) KeptRows() uint64 { return w.Ticks() }

// Clamp is the view an action a client predicts is judged at on tick: the one its client drew, held to
// those whose tick's views (Across) are drawn from the rows kept by then, Ticks before tick up to
// tick - 2, whose views run to the newest row (and 1 at least, as rows start at tick 0). An older view
// is judged at the oldest, so a high ping must lead its shots but still hits. A newer one, or one that
// isn't a number, is judged at the newest.
func (w Window) Clamp(view float64, tick uint64) float64 {
	newest := max(float64(tick)-2, 1)
	if !(view <= newest) {
		return newest
	}
	return max(view, float64(tick)-float64(w.Ticks()), 1)
}

// JSONFile is the path within the content embed.FS.
func (Tuning) JSONFile() string { return "track/tuning.json" }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t Tuning) Validate() error {
	w := t.Window
	if !(w.InterpolationSeconds >= 0 && w.QueueSeconds >= 0) ||
		w.Ticks() < uint64(timestep.Ticks(w.InterpolationSeconds+w.QueueSeconds))+1 {
		return fmt.Errorf("track_tuning: window needs interpolation_seconds and queue_seconds >= 0, and "+
			"ping_seconds of at least a tick at %d Hz, got %+v", timestep.Rate, w)
	}
	return nil
}
