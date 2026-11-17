// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

// The UX measurements of the web client (ADR-0167 §3): the samples that clients send are
// counted by hour, metric, name, and bucket in ux_measurement_counts, which the admin view sums,
// and recorded in the metrics my.ux.duration and my.ux.call.packet_loss, which OpenTelemetry
// exports. No measurement carries an account, a session, an address, or a URL.

// JobPruneUXCounts deletes the hourly counts of the UX measurements older than
// diagnostics.UXRetention, on the schedule of PruneInterval.
const JobPruneUXCounts = "diagnostics.purge_ux_counts"

// Label keys of the UX metrics.
const (
	LabelUXMetric = "my.ux.metric"
	LabelUXName   = "my.ux.name"
)

// The UX metrics: the durations in seconds by metric and name, and the packet loss of the calls
// in percent by transport.
var (
	UXDuration = kernel.Instrument{
		Name: "my.ux.duration", Unit: "s", Description: "Durations that the web client measured, by metric and by route, interaction kind, action, or call transport.",
		LabelKeys: []string{LabelUXMetric, LabelUXName}, Buckets: secondsOf(diagnostics.UXBounds(diagnostics.UXINP)),
	}
	UXCallPacketLoss = kernel.Instrument{
		Name: "my.ux.call.packet_loss", Unit: "%", Description: "Packet loss of the voice calls that the web client measured, by call transport.",
		LabelKeys: []string{LabelUXName}, Buckets: diagnostics.UXBounds(diagnostics.UXCallPacketLoss),
	}
)

func secondsOf(ms []float64) []float64 {
	out := make([]float64, len(ms))
	for i, v := range ms {
		out[i] = v / 1000
	}
	return out
}

// UXMeasurement is one sample that a client sent: a value of Metric, in milliseconds or percent,
// named by Name.
type UXMeasurement struct {
	Metric diagnostics.UXMetric
	Name   string
	Value  float64
}

// UXCount is a number of samples of one hour, metric, name, and bucket (ux_measurement_counts).
type UXCount struct {
	Hour    time.Time
	Metric  diagnostics.UXMetric
	Name    string
	Bucket  int
	Samples int64
}

// UXRepository is the persistence port of the UX measurements.
type UXRepository interface {
	// AddUXCount adds c to its row in one statement outside any transaction, so that concurrent
	// writers each add theirs.
	AddUXCount(ctx context.Context, c UXCount) error
	// DeleteUXCountsBefore deletes the counts of the hours that start before before, in one
	// statement outside any transaction, and returns how many it deleted.
	DeleteUXCountsBefore(ctx context.Context, before time.Time) (int64, error)
	// UXCountsSince returns the counts of the hours that start at or after since, summed over the
	// hours, in no particular order.
	UXCountsSince(ctx context.Context, since time.Time) ([]UXCount, error)
}

// UXSummary is the samples of one metric and name over a window: Buckets holds the samples of
// each bucket (diagnostics.UXBounds), and Samples their sum.
type UXSummary struct {
	Metric  diagnostics.UXMetric
	Name    string
	Samples int64
	Buckets []int64
}

// UXOptions configure a UX.
type UXOptions struct {
	Repository UXRepository
	Clock      kernel.Clock
	// Meter records the UX metrics; nil records none.
	Meter kernel.Meter
	// Reporter reports a panic of the goroutine of the writes (kernel.Go).
	Reporter kernel.ErrorReporter
	// Interval is the period of the writes; zero means FlushInterval.
	Interval time.Duration
	// Manual starts no goroutine: only Flush and Close write. For tests.
	Manual bool
}

// UX counts the UX measurements in memory and writes the counts every Interval and at Close,
// each in one statement outside any transaction, from a goroutine that starts with the first
// measurement, so that a request never waits for the database. A write that fails is dropped:
// the counts are a rough picture, and the metrics hold the same samples.
type UX struct {
	repo     UXRepository
	clock    kernel.Clock
	reporter kernel.ErrorReporter
	interval time.Duration
	manual   bool
	duration kernel.Histogram
	loss     kernel.Histogram
	writing  sync.Mutex

	mu      sync.Mutex
	pending map[uxKey]int64
	// names are the names kept of each metric, at most diagnostics.MaxUXNames.
	names   map[diagnostics.UXMetric]map[string]struct{}
	started bool
	closed  bool
	stop    chan struct{}
	done    chan struct{}
	// abort ends when Close gives up waiting: it cancels the writes in flight, so that no write
	// goes on after Close returns.
	abort       context.Context //nolint:containedctx // cancels the writes of the goroutine
	cancelAbort context.CancelFunc
}

type uxKey struct {
	hour   time.Time
	metric diagnostics.UXMetric
	name   string
	bucket int
}

// NewUX returns a UX on o.
func NewUX(o UXOptions) *UX {
	if o.Interval <= 0 {
		o.Interval = FlushInterval
	}
	abort, cancelAbort := context.WithCancel(context.Background())
	u := &UX{
		repo: o.Repository, clock: o.Clock, reporter: o.Reporter, interval: o.Interval, manual: o.Manual,
		pending: map[uxKey]int64{}, names: map[diagnostics.UXMetric]map[string]struct{}{},
		stop: make(chan struct{}), done: make(chan struct{}), abort: abort, cancelAbort: cancelAbort,
	}
	if o.Meter != nil {
		u.duration, u.loss = o.Meter.Histogram(UXDuration), o.Meter.Histogram(UXCallPacketLoss)
	}
	return u
}

// Record counts the measurements whose metric, name, and value are valid and drops the others.
// A name past the first diagnostics.MaxUXNames of its metric in the process counts as
// diagnostics.UXOtherName. Measurements after Close are dropped.
func (u *UX) Record(ctx context.Context, ms []UXMeasurement) {
	now := u.clock.Now()
	hour := diagnostics.Hour(now)
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return
	}
	for _, m := range ms {
		if !diagnostics.ValidUXName(m.Metric, m.Name) || !diagnostics.ValidUXValue(m.Metric, m.Value) {
			continue
		}
		name := u.nameLocked(m.Metric, m.Name)
		k := uxKey{hour: hour, metric: m.Metric, name: name, bucket: diagnostics.UXBucket(m.Metric, m.Value)}
		if _, ok := u.pending[k]; !ok && len(u.pending) >= maxPending {
			continue
		}
		u.pending[k]++
		u.measure(ctx, m.Metric, name, m.Value)
	}
	if len(u.pending) > 0 {
		u.startLocked()
	}
}

// nameLocked returns name, or diagnostics.UXOtherName once the metric has its most names.
func (u *UX) nameLocked(m diagnostics.UXMetric, name string) string {
	set := u.names[m]
	if set == nil {
		set = map[string]struct{}{}
		u.names[m] = set
	}
	if _, ok := set[name]; ok {
		return name
	}
	if len(set) >= diagnostics.MaxUXNames {
		return diagnostics.UXOtherName
	}
	set[name] = struct{}{}
	return name
}

func (u *UX) measure(ctx context.Context, m diagnostics.UXMetric, name string, v float64) {
	if u.duration == nil {
		return
	}
	if m == diagnostics.UXCallPacketLoss {
		u.loss.Record(ctx, v, kernel.NewLabels(LabelUXName, name))
		return
	}
	u.duration.Record(ctx, v/1000, kernel.NewLabels(LabelUXMetric, string(m), LabelUXName, name))
}

func (u *UX) startLocked() {
	if u.started || u.manual {
		return
	}
	u.started = true
	kernel.Go(context.Background(), "diagnostics.ux", u.reporter, u.run)
}

func (u *UX) run(ctx context.Context) {
	defer close(u.done)
	t := time.NewTicker(u.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = u.Flush(ctx) // dropped: the metrics hold the same samples
		case <-u.stop:
			_ = u.Flush(ctx)
			return
		}
	}
}

// Flush writes the counts of the measurements recorded since the last flush, each with its own
// timeout and without the cancellation of ctx, and returns the failures joined. Every count is
// written by one flush only.
func (u *UX) Flush(ctx context.Context) error {
	u.writing.Lock()
	defer u.writing.Unlock()
	u.mu.Lock()
	pending := u.pending
	u.pending = map[uxKey]int64{}
	u.mu.Unlock()
	var errs []error
	for k, n := range pending {
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
		stop := context.AfterFunc(u.abort, cancel)
		err := u.repo.AddUXCount(wctx, UXCount{Hour: k.hour, Metric: k.metric, Name: k.name, Bucket: k.bucket, Samples: n})
		stop()
		cancel()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close writes what the UX holds and stops its goroutine, waiting until ctx ends at most. When
// ctx ends first, the writes in flight are canceled and awaited for a short grace, so that
// nothing is written after Close returns. The measurements after it are dropped.
func (u *UX) Close(ctx context.Context) error {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return nil
	}
	u.closed = true
	started := u.started
	u.mu.Unlock()
	if !started {
		return u.Flush(ctx)
	}
	close(u.stop)
	select {
	case <-u.done:
		return nil
	case <-ctx.Done():
	}
	u.cancelAbort()
	t := time.NewTimer(abortGrace)
	defer t.Stop()
	select {
	case <-u.done:
	case <-t.C:
	}
	return ctx.Err()
}

// LastHours returns the start of the window of the last hours hours, the current one included,
// and the summaries of that window.
func (u *UX) LastHours(ctx context.Context, hours int) (time.Time, []UXSummary, error) {
	since := diagnostics.Hour(u.clock.Now()).Add(-time.Duration(hours-1) * time.Hour)
	out, err := u.Summaries(ctx, since)
	return since, out, err
}

// Summaries returns the samples of every metric and name over the hours that start at or after
// since, by metric in the order of diagnostics.UXMetrics and then by most samples.
func (u *UX) Summaries(ctx context.Context, since time.Time) ([]UXSummary, error) {
	counts, err := u.repo.UXCountsSince(ctx, diagnostics.Hour(since))
	if err != nil {
		return nil, fmt.Errorf("diagnostics: list the UX measurements: %w", err)
	}
	type key struct {
		metric diagnostics.UXMetric
		name   string
	}
	byKey := map[key]*UXSummary{}
	for _, c := range counts {
		if c.Bucket < 0 || c.Bucket >= diagnostics.UXBuckets(c.Metric) {
			continue
		}
		k := key{c.Metric, c.Name}
		s := byKey[k]
		if s == nil {
			s = &UXSummary{Metric: c.Metric, Name: c.Name, Buckets: make([]int64, diagnostics.UXBuckets(c.Metric))}
			byKey[k] = s
		}
		s.Buckets[c.Bucket] += c.Samples
		s.Samples += c.Samples
	}
	out := make([]UXSummary, 0, len(byKey))
	for _, s := range byKey {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b UXSummary) int {
		return cmp.Or(
			cmp.Compare(slices.Index(diagnostics.UXMetrics, a.Metric), slices.Index(diagnostics.UXMetrics, b.Metric)),
			cmp.Compare(b.Samples, a.Samples),
			strings.Compare(a.Name, b.Name),
		)
	})
	return out, nil
}

// PruneCounts deletes the hourly counts older than diagnostics.UXRetention and returns how many it
// deleted. It is safe to repeat.
func (u *UX) PruneCounts(ctx context.Context) (int64, error) {
	n, err := u.repo.DeleteUXCountsBefore(ctx, diagnostics.Hour(u.clock.Now().Add(-diagnostics.UXRetention)))
	if err != nil {
		return n, fmt.Errorf("diagnostics: prune the UX counts: %w", err)
	}
	return n, nil
}

// RegisterJobs registers the JobPruneUXCounts handler and its daily schedule.
func (u *UX) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPruneUXCounts, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := u.PruneCounts(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobPruneUXCounts, Every: PruneInterval, Job: queue.NewJob{Type: JobPruneUXCounts}})
}
