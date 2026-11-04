// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
)

// Meter creates the instruments of the service and system metrics (ADR-0055 §1, ADR-0055).
// An instrument is declared once, at startup, with its name, unit, description, and the label
// keys it accepts, and then records measurements: a Counter adds to a sum, an UpDownCounter
// adds to a value that also goes down, a Histogram records a distribution, a Gauge is a
// function that the exporter calls when it collects, and an ObservableCounter is such a
// function that reports a total kept elsewhere (the CPU time of the process, the waits of a
// connection pool), which only grows until the process restarts.
//
// Labels take their values from finite sets fixed in code (ADR-0055 §6): never a remote
// domain, an account, status, or job ID, a raw URL or path, a client IP, or a User-Agent.
// A label whose key the instrument did not declare is a programming error: adapters drop it,
// and testkit.Meter reports it so that tests fail.
//
// Instruments are safe for concurrent use. Declaring the same name twice returns instruments
// that record into the same metric when the adapter aggregates (otel), so every declaration of
// a name must agree. The adapters live in internal/adapter/telemetry; NopMeter records nothing.
type Meter interface {
	Counter(i Instrument) Counter
	UpDownCounter(i Instrument) UpDownCounter
	Histogram(i Instrument) Histogram
	// Gauge registers f, which reports the current values of i whenever the exporter collects.
	// Exporters that do not collect (emf) call it only when asked through GaugeWriter.
	Gauge(i Instrument, f GaugeFunc)
	// ObservableCounter registers f, which reports the totals of i whenever the exporter
	// collects: monotonic sums that the exporters show as counters (Prometheus's _total, an
	// OTLP monotonic Sum). A total never goes down until the process restarts. Exporters that do
	// not collect (emf) call it only when asked through GaugeWriter.
	ObservableCounter(i Instrument, f GaugeFunc)
	// Begin starts a unit of work, an HTTP request or a job, and returns its context and the
	// function that ends it. An exporter that writes one record per unit of work (emf) collects
	// the measurements made with that context and writes them when end is called; the others
	// return ctx and a function that does nothing. Call end exactly once.
	Begin(ctx context.Context) (_ context.Context, end func())
}

// GaugeWriter is implemented by a Meter whose exporter does not collect (emf): WriteGauges
// calls the functions of the gauges and the observable counters once and writes what they
// report, so that a periodic job exports the gauges of the metrics snapshot (ADR-0055 §4).
type GaugeWriter interface {
	WriteGauges(ctx context.Context) error
}

// LineMeter is implemented by a Meter whose exporter writes the measurements as log lines
// (emf): the one log line that ends a unit of work (ADR-0055 §5) carries the measurements of
// that unit that the line describes, so that they are not written again on lines of their own.
type LineMeter interface {
	// Line returns attrs, the attributes of the log line of the unit of work that ctx belongs
	// to, with the measurements of instruments made with ctx whose labels are the line's
	// attributes of the same keys and values, and takes those measurements out of the unit, so
	// that its end does not write them. It returns attrs alone outside a unit of work.
	Line(ctx context.Context, attrs []slog.Attr, instruments ...Instrument) []slog.Attr
}

// LogUnit writes the log line that ends a unit of work of m (an HTTP request or a job,
// ADR-0055 §5) with logger: when m is a LineMeter and logger writes the line at level, the line
// also carries the measurements of instruments that it describes (LineMeter.Line).
func LogUnit(ctx context.Context, logger *slog.Logger, level slog.Level, msg string, m Meter, attrs []slog.Attr, instruments ...Instrument) {
	if !logger.Enabled(ctx, level) {
		return // the measurements stay in the unit, which writes them at its end
	}
	if lm, ok := m.(LineMeter); ok {
		attrs = lm.Line(ctx, attrs, instruments...)
	}
	logger.LogAttrs(ctx, level, msg, attrs...)
}

// Instrument declares a metric (ADR-0055 §3).
type Instrument struct {
	// Name follows the OpenTelemetry semantic conventions where they define the metric, and
	// starts with "my." otherwise; Prometheus shows it with "_" for ".".
	Name string
	// Unit is a UCUM unit: "s", "By", or an annotation such as "{request}".
	Unit        string
	Description string
	// LabelKeys are the keys that measurements may carry, at most MaxLabels.
	LabelKeys []string
	// Buckets are the explicit bucket boundaries of a Histogram, in Unit; empty takes the
	// adapter's default.
	Buckets []float64
}

// Accepts reports whether the instrument declared the label key.
func (i Instrument) Accepts(key string) bool { return slices.Contains(i.LabelKeys, key) }

// Check reports the first label of l whose key i did not declare.
func (i Instrument) Check(l Labels) error {
	for k := range l.All() {
		if !i.Accepts(k) {
			return fmt.Errorf("kernel: metric %s has no label %q (declared %v)", i.Name, k, i.LabelKeys)
		}
	}
	return nil
}

// DurationBuckets are the bucket boundaries, in seconds, of the duration histograms: those of
// the OpenTelemetry semantic conventions for http.server.request.duration.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

// Counter is a monotonic sum. n must not be negative.
type Counter interface {
	Add(ctx context.Context, n int64, l Labels)
}

// UpDownCounter is a sum that also goes down, such as the requests in progress.
type UpDownCounter interface {
	Add(ctx context.Context, n int64, l Labels)
}

// Histogram records a distribution of values, such as durations in seconds.
type Histogram interface {
	Record(ctx context.Context, v float64, l Labels)
}

// GaugeFunc reports the current values of a gauge, or the totals of an observable counter, to
// o, one per label set.
type GaugeFunc func(ctx context.Context, o GaugeObserver) error

// GaugeObserver receives the values of a GaugeFunc.
type GaugeObserver interface {
	Observe(v float64, l Labels)
}

// MaxLabels is the most labels one measurement carries.
const MaxLabels = 4

// Labels are the labels of one measurement. It is a value, so that recording a measurement
// does not allocate; build it with NewLabels or With.
type Labels struct {
	n    int
	keys [MaxLabels]string
	vals [MaxLabels]string
}

// NewLabels returns the labels of alternating keys and values. It panics on an odd number of
// arguments or more than MaxLabels labels, which are programming errors.
func NewLabels(kv ...string) Labels {
	if len(kv)%2 != 0 {
		panic("kernel: NewLabels needs key-value pairs")
	}
	var l Labels
	for i := 0; i < len(kv); i += 2 {
		l = l.With(kv[i], kv[i+1])
	}
	return l
}

// With returns l with the label key set to value, replacing a label of the same key. It
// panics beyond MaxLabels labels.
func (l Labels) With(key, value string) Labels {
	for i := range l.n {
		if l.keys[i] == key {
			l.vals[i] = value
			return l
		}
	}
	if l.n == MaxLabels {
		panic(fmt.Sprintf("kernel: more than %d labels", MaxLabels))
	}
	l.keys[l.n], l.vals[l.n] = key, value
	l.n++
	return l
}

// Len returns the number of labels.
func (l Labels) Len() int { return l.n }

// Get returns the value of the label key.
func (l Labels) Get(key string) (string, bool) {
	for i := range l.n {
		if l.keys[i] == key {
			return l.vals[i], true
		}
	}
	return "", false
}

// All yields the labels in the order they were set.
func (l Labels) All() func(yield func(key, value string) bool) {
	return func(yield func(key, value string) bool) {
		for i := range l.n {
			if !yield(l.keys[i], l.vals[i]) {
				return
			}
		}
	}
}

// NopMeter is a Meter that records nothing, the meter of metrics_exporter = none. Its
// instruments cost a method call.
type NopMeter struct{}

var _ Meter = NopMeter{}

type nopInstrument struct{}

func (nopInstrument) Add(context.Context, int64, Labels)      {}
func (nopInstrument) Record(context.Context, float64, Labels) {}

// Counter implements Meter.
func (NopMeter) Counter(Instrument) Counter { return nopInstrument{} }

// UpDownCounter implements Meter.
func (NopMeter) UpDownCounter(Instrument) UpDownCounter { return nopInstrument{} }

// Histogram implements Meter.
func (NopMeter) Histogram(Instrument) Histogram { return nopInstrument{} }

// Gauge implements Meter.
func (NopMeter) Gauge(Instrument, GaugeFunc) {}

// ObservableCounter implements Meter.
func (NopMeter) ObservableCounter(Instrument, GaugeFunc) {}

// Begin implements Meter.
func (NopMeter) Begin(ctx context.Context) (context.Context, func()) { return ctx, nopEnd }

func nopEnd() {}

// MeterOrNop returns m, or NopMeter when m is nil, for the optional Meter of a dependency set.
func MeterOrNop(m Meter) Meter {
	if m == nil {
		return NopMeter{}
	}
	return m
}

// IsNop reports whether m records nothing, so that a caller can skip the work of measuring.
func IsNop(m Meter) bool {
	if m == nil {
		return true
	}
	_, ok := m.(NopMeter)
	return ok
}
