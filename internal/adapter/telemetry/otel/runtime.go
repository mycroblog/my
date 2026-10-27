// SPDX-License-Identifier: AGPL-3.0-only

package otel

import (
	"context"
	"errors"
	"math"
	"runtime/metrics"
	"slices"
	"sync"
	"time"

	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The Go runtime metrics of ADR-0056 §1 that the contrib runtime instrumentation does not
// report: the GC cycles and the GC pauses.
const (
	runtimeScope       = "github.com/mycroblog/my/internal/adapter/telemetry/otel/runtime"
	gcCyclesName       = "go.memory.gc.cycles"
	gcPauseName        = "go.memory.gc.pause.duration"
	runtimeGCCycles    = "/gc/cycles/total:gc-cycles"
	runtimeGCPauses    = "/sched/pauses/total/gc:seconds"
	runtimeMinInterval = time.Second
)

// runtimeProducers are the producers of the Go runtime histograms that a reader exports with
// the instruments of the MeterProvider: go.schedule.duration from the contrib runtime
// instrumentation, and go.memory.gc.pause.duration.
func runtimeProducers() []sdkmetric.Producer {
	return []sdkmetric.Producer{otelruntime.NewProducer(), newGCPauseProducer()}
}

// startRuntime registers the Go runtime metrics on the provider (ADR-0056 §1): the go.*
// metrics of the OpenTelemetry semantic conventions that the contrib runtime instrumentation
// reports, and go.memory.gc.cycles.
func startRuntime(p metric.MeterProvider) error {
	if err := otelruntime.Start(otelruntime.WithMeterProvider(p)); err != nil {
		return err
	}
	m := p.Meter(runtimeScope)
	sample := []metrics.Sample{{Name: runtimeGCCycles}}
	var mu sync.Mutex
	_, err := m.Int64ObservableCounter(gcCyclesName, metric.WithUnit("{gc_cycle}"),
		metric.WithDescription("Completed GC cycles."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			mu.Lock()
			defer mu.Unlock()
			metrics.Read(sample)
			if sample[0].Value.Kind() == metrics.KindUint64 {
				o.Observe(int64(min(sample[0].Value.Uint64(), math.MaxInt64))) // #nosec G115 -- clamped
			}
			return nil
		}))
	return err
}

// gcPauseProducer reports the distribution of the stop-the-world pauses of the GC, which the
// runtime keeps as a cumulative histogram, as an OpenTelemetry histogram.
type gcPauseProducer struct {
	start time.Time

	mu     sync.Mutex
	sample []metrics.Sample
	last   time.Time
}

func newGCPauseProducer() *gcPauseProducer {
	return &gcPauseProducer{start: time.Now(), sample: []metrics.Sample{{Name: runtimeGCPauses}}}
}

// Produce implements sdkmetric.Producer.
func (p *gcPauseProducer) Produce(context.Context) ([]metricdata.ScopeMetrics, error) {
	p.mu.Lock()
	now := time.Now()
	if now.Sub(p.last) >= runtimeMinInterval {
		metrics.Read(p.sample)
		p.last = now
	}
	ts := p.last
	var dp []metricdata.HistogramDataPoint[float64]
	if p.sample[0].Value.Kind() == metrics.KindFloat64Histogram {
		dp = histogramPoints(p.sample[0].Value.Float64Histogram(), p.start, ts)
	}
	p.mu.Unlock()
	if len(dp) == 0 {
		return nil, errors.New("otel: the runtime reports no GC pauses")
	}
	return []metricdata.ScopeMetrics{{
		Scope: instrumentation.Scope{Name: runtimeScope},
		Metrics: []metricdata.Metrics{{
			Name: gcPauseName, Unit: "s", Description: "Duration of the stop-the-world pauses of the GC.",
			Data: metricdata.Histogram[float64]{Temporality: metricdata.CumulativeTemporality, DataPoints: dp},
		}},
	}}, nil
}

// histogramPoints converts a runtime histogram, whose Buckets are the boundaries of its Counts
// (len(Counts)+1 of them, the first a lower bound), to the explicit bucket boundaries of an
// OpenTelemetry histogram, whose last bucket is unbounded. The sum takes each value at the
// lower bound of its bucket, so it underestimates.
func histogramPoints(h *metrics.Float64Histogram, start, ts time.Time) []metricdata.HistogramDataPoint[float64] {
	if h == nil || len(h.Buckets) < 2 || len(h.Buckets) != len(h.Counts)+1 {
		return nil
	}
	// The runtime may reuse the slices of the histogram on the next read, so the data point owns copies.
	bounds := slices.Clone(h.Buckets[1:])
	counts := slices.Clone(h.Counts)
	if math.IsInf(bounds[len(bounds)-1], 1) {
		bounds = bounds[:len(bounds)-1]
	} else {
		counts = append(counts, 0)
	}
	var count uint64
	var sum float64
	for i, c := range counts {
		count += c
		if c > 0 {
			lower := h.Buckets[i]
			if !math.IsInf(lower, 0) {
				sum += lower * float64(c)
			}
		}
	}
	return []metricdata.HistogramDataPoint[float64]{{
		Attributes: *attribute.EmptySet(), StartTime: start, Time: ts,
		Count: count, Sum: sum, Bounds: bounds, BucketCounts: counts,
	}}
}
