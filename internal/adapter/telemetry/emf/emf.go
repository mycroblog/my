// SPDX-License-Identifier: AGPL-3.0-only

// Package emf is the kernel.Meter adapter of metrics_exporter = emf (ADR-0055 §2): it writes
// the measurements as JSON lines in the CloudWatch Embedded Metric Format, which CloudWatch
// Logs turns into metrics, so that the process aggregates nothing and a frozen Lambda execution
// environment loses nothing.
//
// The measurements made with the context of a unit of work (kernel.Meter.Begin: an HTTP request
// or a job) are written when it ends: one line per distinct label set, holding every metric
// recorded with those labels (a counter's sum, a histogram's values). The unit's own log line
// (the access log, the job log, ADR-0055 §5) takes the measurements that it describes first
// (kernel.LineMeter): it gets the _aws directive and the values, so that one line is both the
// log and the metrics, and the end of the unit writes only the others. A measurement outside a
// unit of work is written at once. UpDownCounters record nothing, since they are values of one
// process that the platform's own metrics stand in for (Lambda's concurrency). Gauges and
// observable counters are not collected: WriteGauges (kernel.GaugeWriter) writes them when the
// periodic job of the metrics snapshot asks (ADR-0055 §4). EMF has no counter of totals, so an
// observable counter is written as its increase since the previous write, which the Sum
// statistic of CloudWatch adds up as it does the counters.
package emf

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/mycroblog/my/internal/app/kernel"
)

// maxValues is the most values of one metric in one EMF document.
const maxValues = 100

// Options configure New.
type Options struct {
	// Writer receives the lines; the process passes its standard output.
	Writer io.Writer
	// Namespace is the CloudWatch namespace of the metrics (metrics_emf_namespace).
	Namespace string
	// Dimensions are the label keys that become CloudWatch dimensions when a line carries
	// them (metrics_emf_dimensions); the other labels stay properties of the line, which
	// CloudWatch Logs Insights can still query. Each combination of dimension values is a
	// metric that CloudWatch bills.
	Dimensions []string
	// Properties are written on every line: the resource (service.name, service.version,
	// my.role, ADR-0055 §6).
	Properties map[string]string
	Clock      kernel.Clock
}

// Meter is a kernel.Meter that writes EMF lines.
type Meter struct {
	opts Options
	mu   sync.Mutex // serializes the writes, one line each

	gaugesMu sync.Mutex
	gauges   []gauge
	// totals are the totals of the observable counters at the previous WriteGauges, by name
	// and label set.
	totals map[string]float64
	// pending are the totals observed by the running WriteGauges, kept once its lines are written.
	pending map[string]float64
}

type gauge struct {
	i kernel.Instrument
	f kernel.GaugeFunc
	// counter reports totals, written as their increase.
	counter bool
}

var (
	_ kernel.Meter       = (*Meter)(nil)
	_ kernel.GaugeWriter = (*Meter)(nil)
	_ kernel.LineMeter   = (*Meter)(nil)
)

// New returns the Meter on opts. Writer and Clock are required.
func New(opts Options) *Meter {
	if opts.Writer == nil || opts.Clock == nil {
		panic("emf: Options.Writer and Options.Clock are required")
	}
	if opts.Namespace == "" {
		opts.Namespace = "my"
	}
	return &Meter{opts: opts}
}

type kind int

const (
	kindSum kind = iota + 1
	kindValues
)

// Counter implements kernel.Meter.
func (m *Meter) Counter(i kernel.Instrument) kernel.Counter {
	return instrument{m: m, i: i, k: kindSum}
}

// UpDownCounter implements kernel.Meter; it records nothing.
func (m *Meter) UpDownCounter(i kernel.Instrument) kernel.UpDownCounter {
	return kernel.NopMeter{}.UpDownCounter(i)
}

// Histogram implements kernel.Meter.
func (m *Meter) Histogram(i kernel.Instrument) kernel.Histogram {
	return instrument{m: m, i: i, k: kindValues}
}

// Gauge implements kernel.Meter. EMF has no collection: WriteGauges calls f.
func (m *Meter) Gauge(i kernel.Instrument, f kernel.GaugeFunc) {
	m.gaugesMu.Lock()
	defer m.gaugesMu.Unlock()
	m.gauges = append(m.gauges, gauge{i: i, f: f})
}

// ObservableCounter implements kernel.Meter. EMF has no collection: WriteGauges calls f and
// writes the increase of each total since the previous call.
func (m *Meter) ObservableCounter(i kernel.Instrument, f kernel.GaugeFunc) {
	m.gaugesMu.Lock()
	defer m.gaugesMu.Unlock()
	m.gauges = append(m.gauges, gauge{i: i, f: f, counter: true})
}

// WriteGauges implements kernel.GaugeWriter: it calls the functions of the gauges and the
// observable counters and writes their values, one line per distinct label set, and returns
// the errors of the functions. Calls are serialized, so that each increase is written once.
func (m *Meter) WriteGauges(ctx context.Context) error {
	m.gaugesMu.Lock()
	defer m.gaugesMu.Unlock()
	if m.totals == nil {
		m.totals = map[string]float64{}
	}
	b := &batch{}
	bctx := context.WithValue(ctx, batchKey{}, b)
	m.pending = map[string]float64{}
	var errs []error
	for _, g := range m.gauges {
		errs = append(errs, g.f(ctx, gaugeObserver{m: m, ctx: bctx, g: g}))
	}
	if err := m.flush(b); err != nil {
		// The increases were not written: keep the previous totals, so that the next call
		// writes them again.
		errs = append(errs, err)
	} else {
		maps.Copy(m.totals, m.pending)
	}
	m.pending = nil
	return errors.Join(errs...)
}

type gaugeObserver struct {
	m   *Meter
	ctx context.Context
	g   gauge
}

// Observe records v; for an observable counter, the increase of v over the previous total of
// the label set (all of v the first time, or after the total went back to 0 when its source
// restarted). The caller holds gaugesMu.
func (o gaugeObserver) Observe(v float64, l kernel.Labels) {
	if o.g.counter {
		key := seriesKey(o.g.i, l)
		prev, seen := o.m.totals[key]
		if p, ok := o.m.pending[key]; ok {
			prev, seen = p, true
		}
		o.m.pending[key] = v
		if seen && v >= prev {
			v -= prev
		}
	}
	o.m.record(o.ctx, o.g.i, kindValues, v, 0, l)
}

// seriesKey identifies the series of i with the declared labels of l.
func seriesKey(i kernel.Instrument, l kernel.Labels) string {
	var sb strings.Builder
	sb.WriteString(i.Name)
	for _, key := range i.LabelKeys {
		if value, ok := l.Get(key); ok {
			sb.WriteString("\x00" + key + "=" + value)
		}
	}
	return sb.String()
}

type batchKey struct{}

// Begin implements kernel.Meter: the measurements made with the returned context are written
// when end is called.
func (m *Meter) Begin(ctx context.Context) (context.Context, func()) {
	b := &batch{}
	return context.WithValue(ctx, batchKey{}, b), func() { _ = m.flush(b) }
}

// batch is the measurements of one unit of work.
type batch struct {
	mu     sync.Mutex
	groups []*group
	ended  bool
}

// group is the metrics recorded with one label set.
type group struct {
	labels  []label
	metrics []*measure
}

type label struct{ key, value string }

type measure struct {
	name, unit string
	sum        int64
	values     []float64
}

type instrument struct {
	m *Meter
	i kernel.Instrument
	k kind
}

func (in instrument) Add(ctx context.Context, n int64, l kernel.Labels) {
	if n < 0 {
		return
	}
	in.m.record(ctx, in.i, in.k, float64(n), n, l)
}

func (in instrument) Record(ctx context.Context, v float64, l kernel.Labels) {
	in.m.record(ctx, in.i, in.k, v, 0, l)
}

func (m *Meter) record(ctx context.Context, i kernel.Instrument, k kind, v float64, n int64, l kernel.Labels) {
	labels := make([]label, 0, l.Len())
	for key, value := range l.All() {
		if i.Accepts(key) {
			labels = append(labels, label{key, value})
		}
	}
	slices.SortFunc(labels, func(a, b label) int { return strings.Compare(a.key, b.key) })

	b, _ := ctx.Value(batchKey{}).(*batch)
	if b == nil {
		_ = m.write(&group{labels: labels, metrics: []*measure{newMeasure(i, k, v, n)}})
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ended {
		_ = m.write(&group{labels: labels, metrics: []*measure{newMeasure(i, k, v, n)}})
		return
	}
	g := b.group(labels)
	for _, ms := range g.metrics {
		if ms.name != i.Name {
			continue
		}
		if k == kindSum {
			ms.sum += n
			return
		}
		if len(ms.values) == maxValues {
			// A document holds at most maxValues values of a metric: write those so far.
			_ = m.write(&group{labels: g.labels, metrics: []*measure{ms}})
			ms.values = nil
		}
		ms.values = append(ms.values, v)
		return
	}
	g.metrics = append(g.metrics, newMeasure(i, k, v, n))
}

func newMeasure(i kernel.Instrument, k kind, v float64, n int64) *measure {
	ms := &measure{name: i.Name, unit: i.Unit}
	if k == kindSum {
		ms.sum = n
	} else {
		ms.values = []float64{v}
	}
	return ms
}

func (b *batch) group(labels []label) *group {
	for _, g := range b.groups {
		if slices.Equal(g.labels, labels) {
			return g
		}
	}
	g := &group{labels: labels}
	b.groups = append(b.groups, g)
	return g
}

func (m *Meter) flush(b *batch) error {
	b.mu.Lock()
	groups := b.groups
	b.groups, b.ended = nil, true
	b.mu.Unlock()
	var errs []error
	for _, g := range groups {
		errs = append(errs, m.write(g))
	}
	return errors.Join(errs...)
}

// Line implements kernel.LineMeter: it adds to attrs the measurements of instruments in the
// unit of work of ctx whose labels, for every label key the instrument declares, are those of
// the line (a key absent from both, or the same value), with one _aws directive per label set
// and the properties of the meter that the line lacks. The values of the attributes that
// become dimensions are written as strings, which EMF requires.
func (m *Meter) Line(ctx context.Context, attrs []slog.Attr, instruments ...kernel.Instrument) []slog.Attr {
	b, _ := ctx.Value(batchKey{}).(*batch)
	if b == nil || len(instruments) == 0 {
		return attrs
	}
	fields := make(map[string]string, len(attrs))
	for _, a := range attrs {
		fields[a.Key] = a.Value.Resolve().String()
	}
	taken := b.take(fields, instruments)
	if len(taken) == 0 {
		return attrs
	}

	out := slices.Clone(attrs)
	for _, k := range slices.Sorted(maps.Keys(m.opts.Properties)) {
		if _, ok := fields[k]; !ok {
			out = append(out, slog.String(k, m.opts.Properties[k]))
		}
	}
	directives := make([]directive, 0, len(taken))
	dimensions := map[string]bool{}
	for _, g := range taken {
		dims := m.dimensions(g.labels)
		for _, k := range dims {
			dimensions[k] = true
		}
		d := directive{Namespace: m.opts.Namespace, Dimensions: [][]string{dims}}
		for _, ms := range g.metrics {
			d.Metrics = append(d.Metrics, metricDef{Name: ms.name, Unit: Unit(ms.unit)})
			out = append(out, slog.Any(ms.name, ms.value()))
		}
		directives = append(directives, d)
	}
	for i, a := range out {
		if dimensions[a.Key] && a.Value.Kind() != slog.KindString {
			out[i] = slog.String(a.Key, a.Value.Resolve().String())
		}
	}
	return append(out, slog.Any("_aws", document{Timestamp: m.opts.Clock.Now().UnixMilli(), CloudWatchMetrics: directives}))
}

// take takes out of b the measurements of instruments that the line of fields describes, by
// label set.
func (b *batch) take(fields map[string]string, instruments []kernel.Instrument) []*group {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ended {
		return nil
	}
	var taken []*group
	for _, g := range b.groups {
		var ms []*measure
		g.metrics = slices.DeleteFunc(g.metrics, func(x *measure) bool {
			i := slices.IndexFunc(instruments, func(i kernel.Instrument) bool { return i.Name == x.name })
			if i < 0 || !describes(fields, instruments[i], g.labels) {
				return false
			}
			ms = append(ms, x)
			return true
		})
		if len(ms) > 0 {
			taken = append(taken, &group{labels: g.labels, metrics: ms})
		}
	}
	return taken
}

// describes reports whether the line of fields describes the measurements of i with labels:
// for every label key of i, the line has the label's value, or neither has the key.
func describes(fields map[string]string, i kernel.Instrument, labels []label) bool {
	for _, key := range i.LabelKeys {
		v, onLine := fields[key]
		j := slices.IndexFunc(labels, func(l label) bool { return l.key == key })
		if onLine != (j >= 0) || onLine && labels[j].value != v {
			return false
		}
	}
	return true
}

// dimensions returns the dimensions of the metrics with labels: those of Options.Dimensions
// that are one of the labels or a property, as on a line of their own (write), whatever else
// the log line carries.
func (m *Meter) dimensions(labels []label) []string {
	dims := []string{}
	for _, d := range m.opts.Dimensions {
		_, property := m.opts.Properties[d]
		if property || slices.ContainsFunc(labels, func(l label) bool { return l.key == d }) {
			dims = append(dims, d)
		}
	}
	return dims
}

// value is the value of ms on a line: a counter's sum, a histogram's value or values.
func (ms *measure) value() any {
	switch len(ms.values) {
	case 0:
		return ms.sum
	case 1:
		return ms.values[0]
	default:
		return ms.values
	}
}

type document struct {
	Timestamp         int64       `json:"Timestamp"`
	CloudWatchMetrics []directive `json:"CloudWatchMetrics"`
}

type directive struct {
	Namespace  string      `json:"Namespace"`
	Dimensions [][]string  `json:"Dimensions"`
	Metrics    []metricDef `json:"Metrics"`
}

type metricDef struct {
	Name string `json:"Name"`
	Unit string `json:"Unit"`
}

// write writes g as one EMF line.
func (m *Meter) write(g *group) error {
	if len(g.metrics) == 0 {
		return nil
	}
	line := make(map[string]any, len(m.opts.Properties)+len(g.labels)+len(g.metrics)+1)
	for k, v := range m.opts.Properties {
		line[k] = v
	}
	for _, l := range g.labels {
		line[l.key] = l.value
	}
	dims := []string{}
	for _, d := range m.opts.Dimensions {
		if _, ok := line[d]; ok {
			dims = append(dims, d)
		}
	}
	defs := make([]metricDef, 0, len(g.metrics))
	for _, ms := range g.metrics {
		defs = append(defs, metricDef{Name: ms.name, Unit: Unit(ms.unit)})
		line[ms.name] = ms.value()
	}
	line["_aws"] = document{
		Timestamp: m.opts.Clock.Now().UnixMilli(),
		CloudWatchMetrics: []directive{{
			Namespace: m.opts.Namespace, Dimensions: [][]string{dims}, Metrics: defs,
		}},
	}
	out, err := json.Marshal(line)
	if err != nil {
		return nil // only a NaN or an infinity fails, which no instrument records
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err = m.opts.Writer.Write(append(out, '\n'))
	return err
}

// Unit returns the CloudWatch unit of a UCUM unit: Seconds, Milliseconds, Bytes, Count for the
// annotations such as {request} and 1, and None otherwise.
func Unit(ucum string) string {
	switch {
	case ucum == "s":
		return "Seconds"
	case ucum == "ms":
		return "Milliseconds"
	case ucum == "By":
		return "Bytes"
	case ucum == "1", strings.HasPrefix(ucum, "{") && strings.HasSuffix(ucum, "}"):
		return "Count"
	default:
		return "None"
	}
}
