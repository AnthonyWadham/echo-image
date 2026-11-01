package player

import (
	"errors"

	"go.opentelemetry.io/otel/metric"
)

// InputMetrics counts input health over every Body for ops: from otel's global meters, the shard's,
// which Cardinal's telemetry exports with its own. Each Body's own counts are its InputQueue.Health.
type InputMetrics struct {
	coasts, dropped, oversized, floored metric.Int64Counter
	queued                              metric.Int64Histogram
}

// NewInputMetrics makes InputMetrics' instruments from meters.
func NewInputMetrics(meters metric.MeterProvider) (InputMetrics, error) {
	meter := meters.Meter("echo_image/shards/game/internal/player")
	var m InputMetrics
	var err, errs error
	m.coasts, err = meter.Int64Counter("echo.input.coasts", metric.WithUnit("{coast}"),
		metric.WithDescription("Times a Body's input queue ran dry while its client played: each is a correction."))
	errs = errors.Join(errs, err)
	m.dropped, err = meter.Int64Counter("echo.input.frames_dropped", metric.WithUnit("{frame}"),
		metric.WithDescription("Input frames dropped past a Body's queue cap."))
	errs = errors.Join(errs, err)
	m.oversized, err = meter.Int64Counter("echo.input.oversized", metric.WithUnit("{input}"),
		metric.WithDescription("Input commands dropped whole for more frames than the queue holds."))
	errs = errors.Join(errs, err)
	m.floored, err = meter.Int64Counter("echo.input.views_floored", metric.WithUnit("{frame}"),
		metric.WithDescription("Input frames whose view went back, taken at the newest so far."))
	errs = errors.Join(errs, err)
	m.queued, err = meter.Int64Histogram("echo.input.queued", metric.WithUnit("{frame}"),
		metric.WithDescription("Frames left in a Body's queue after one applies."),
		metric.WithExplicitBucketBoundaries(0, 1, 2, 3, 4, 6, 8, 12, 16, 24, 32))
	errs = errors.Join(errs, err)
	return m, errs
}
