// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

const (
	// FlushInterval is how often a Recorder writes the occurrences it counted (ADR-0103 §4 (2)).
	FlushInterval = 60 * time.Second
	// writeTimeout bounds each write of a Recorder, so that a database that does not answer
	// holds nothing for long.
	writeTimeout = 5 * time.Second
	// firstQueue is how many first occurrences wait for their write; more are dropped.
	firstQueue = 256
	// maxSeen and maxPending bound the memory of a Recorder: past maxSeen pairs of fingerprint
	// and version it forgets them (the next occurrence of each is written at once again), and
	// past maxPending counts it drops the occurrences of new fingerprints and hours until the
	// next flush.
	maxSeen    = 10000
	maxPending = 10000
	// abortGrace is how long Close waits for the goroutine after it cancelled the write in
	// flight, because the deadline of its caller ended first.
	abortGrace = time.Second
)

// RecorderOptions configure a Recorder.
type RecorderOptions struct {
	Service *Service
	Clock   kernel.Clock
	// Reporter reports a panic of the Recorder's goroutine (kernel.Go).
	Reporter kernel.ErrorReporter
	// Interval is the period of the flushes; zero means FlushInterval.
	Interval time.Duration
	// Manual starts no goroutine: the first occurrences wait in the queue too, and only Flush
	// and Close write. For tests.
	Manual bool
}

// Recorder is the output of the ErrorReporter that groups the reports in the database
// (ADR-0103 §4 (2)). The first occurrence of a fingerprint and version in the process is written
// at once (Service.RecordFirst: the group, a regression or a confirmation, the count, and the
// sample); the later
// ones are counted in memory by hour and written every Interval and at Close, each count and
// sample in one statement outside any transaction. The writes run in a goroutine of the
// Recorder, which starts with the first report, so that ReceiveError never waits for the
// database. A write that fails is dropped: the logs and the metrics keep the exact numbers.
type Recorder struct {
	svc      *Service
	clock    kernel.Clock
	reporter kernel.ErrorReporter
	interval time.Duration
	manual   bool
	first    chan firstReport
	// writing serializes the writes, so that the counts of a fingerprint follow the write of
	// its group.
	writing sync.Mutex

	mu      sync.Mutex
	seen    map[seenKey]struct{}
	pending map[countKey]*pendingCount
	started bool
	closed  bool
	stop    chan struct{}
	done    chan struct{}
	// abort ends when Close gives up waiting: it cancels the write in flight, so that no write
	// goes on after Close returns.
	abort       context.Context //nolint:containedctx // cancels the writes of the goroutine
	cancelAbort context.CancelFunc
	// finished is set when no write is left to run: Flush writes nothing after it.
	finished atomic.Bool
}

// seenKey is what makes an occurrence a first one in the process: a new fingerprint, version,
// or presence of a session (the first report with a session may confirm a group).
type seenKey struct {
	fingerprint, version string
	unauthenticated      bool
}

// firstReport is a first occurrence waiting for its write, with the context of its report
// without its cancellation: the values it carries stay, and a report made with a context that
// has ended (a closing stream, a response already sent) is still written.
type firstReport struct {
	ctx context.Context //nolint:containedctx // carried to the goroutine of the writes
	ev  kernel.ErrorEvent
}

type countKey struct {
	fingerprint string
	hour        time.Time
}

// pendingCount is what a Recorder counted of one fingerprint and hour since its last flush.
type pendingCount struct {
	n    int64
	last kernel.ErrorEvent
}

// NewRecorder returns a Recorder on o.
func NewRecorder(o RecorderOptions) *Recorder {
	if o.Interval <= 0 {
		o.Interval = FlushInterval
	}
	abort, cancelAbort := context.WithCancel(context.Background())
	return &Recorder{
		abort: abort, cancelAbort: cancelAbort,
		svc: o.Service, clock: o.Clock, reporter: o.Reporter, interval: o.Interval, manual: o.Manual,
		first: make(chan firstReport, firstQueue),
		seen:  map[seenKey]struct{}{}, pending: map[countKey]*pendingCount{},
		stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// ReceiveError counts ev in the group of its fingerprint (platform.ErrorSink). An event without
// a fingerprint, or one that arrives after Close, is dropped.
func (r *Recorder) ReceiveError(ctx context.Context, ev kernel.ErrorEvent) {
	if ev.Fingerprint == "" {
		return
	}
	if ev.Time.IsZero() {
		ev.Time = r.clock.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.startLocked()
	k := seenKey{ev.Fingerprint, ev.Version, ev.Unauthenticated}
	if _, ok := r.seen[k]; !ok {
		if len(r.seen) >= maxSeen {
			clear(r.seen)
		}
		select {
		case r.first <- firstReport{ctx: context.WithoutCancel(ctx), ev: ev}:
			r.seen[k] = struct{}{}
		default: // the queue is full: dropped, and the next occurrence tries again
		}
		return
	}
	ck := countKey{ev.Fingerprint, diagnostics.Hour(ev.Time)}
	p := r.pending[ck]
	if p == nil {
		if len(r.pending) >= maxPending {
			return
		}
		p = &pendingCount{}
		r.pending[ck] = p
	}
	p.n++
	if !ev.Time.Before(p.last.Time) {
		p.last = ev
	}
}

// startLocked starts the goroutine of the writes once.
func (r *Recorder) startLocked() {
	if r.started || r.manual {
		return
	}
	r.started = true
	kernel.Go(context.Background(), "diagnostics.recorder", r.reporter, r.run)
}

func (r *Recorder) run(ctx context.Context) {
	defer close(r.done)
	defer r.finished.Store(true)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case f := <-r.first:
			r.writing.Lock()
			r.writeFirst(f)
			r.writing.Unlock()
		case <-t.C:
			r.flush(ctx)
		case <-r.stop:
			r.flush(ctx)
			return
		}
	}
}

// Flush writes the first occurrences that wait, then the counts and the latest samples counted
// since the last flush, each write with its own timeout and without the cancellation of ctx. Every occurrence is written by one flush only, so that concurrent
// flushes count each once. It writes nothing once Close has returned.
func (r *Recorder) Flush(ctx context.Context) {
	if r.finished.Load() {
		return
	}
	r.flush(ctx)
}

func (r *Recorder) flush(ctx context.Context) {
	r.writing.Lock()
	defer r.writing.Unlock()
	for drained := false; !drained; {
		select {
		case f := <-r.first:
			r.writeFirst(f)
		default:
			drained = true
		}
	}
	r.mu.Lock()
	pending := r.pending
	r.pending = map[countKey]*pendingCount{}
	r.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	counts := make([]Count, 0, len(pending))
	latest := map[string]kernel.ErrorEvent{}
	for k, p := range pending {
		c := countOf(p.last, p.n)
		c.Hour = k.hour
		counts = append(counts, c)
		if l, ok := latest[k.fingerprint]; !ok || p.last.Time.After(l.Time) {
			latest[k.fingerprint] = p.last
		}
	}
	samples := make([]Sample, 0, len(latest))
	for _, ev := range latest {
		samples = append(samples, sampleOf(ev))
	}
	wctx, cancel := r.writeContext(context.WithoutCancel(ctx))
	defer cancel()
	_ = r.svc.AddOccurrences(wctx, counts, samples) // dropped: the reporter never reports itself
}

// writeContext bounds a write by writeTimeout and by the abort of Close.
func (r *Recorder) writeContext(base context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(base, writeTimeout)
	stop := context.AfterFunc(r.abort, cancel)
	return ctx, func() { stop(); cancel() }
}

// writeFirst writes a first occurrence; when it fails, the next occurrence of the same
// fingerprint and version is written at once again.
func (r *Recorder) writeFirst(f firstReport) {
	wctx, cancel := r.writeContext(f.ctx)
	defer cancel()
	if err := r.svc.RecordFirst(wctx, f.ev); err != nil {
		r.mu.Lock()
		delete(r.seen, seenKey{f.ev.Fingerprint, f.ev.Version, f.ev.Unauthenticated})
		r.mu.Unlock()
	}
}

// Close writes what the Recorder holds and stops its goroutine, waiting until ctx ends at most
// (platform.ErrorSinkCloser). When ctx ends first, the write in flight is cancelled and awaited
// for a short grace, so that nothing is written after Close returns. The reports after it are
// dropped.
func (r *Recorder) Close(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	started := r.started
	r.mu.Unlock()
	if !started {
		r.flush(ctx)
		r.finished.Store(true)
		return nil
	}
	close(r.stop)
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
	}
	r.cancelAbort()
	t := time.NewTimer(abortGrace)
	defer t.Stop()
	select {
	case <-r.done:
	case <-t.C:
	}
	return ctx.Err()
}
