// SPDX-License-Identifier: AGPL-3.0-only

package metrics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/metrics"
	"github.com/mycroblog/my/internal/app/queue"
)

// clock is a kernel.Clock that tests move.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// store is a kernel.KeyValueStore in memory that counts the reads and writes, the store that
// the processes share.
type store struct {
	clock  *clock
	mu     sync.Mutex
	values map[string]entry
	reads  atomic.Int64
	writes atomic.Int64
}

type entry struct {
	value   []byte
	expires time.Time
}

func newStore(c *clock) *store { return &store{clock: c, values: map[string]entry{}} }

func (s *store) live(key string) (entry, bool) {
	e, ok := s.values[key]
	if !ok || (!e.expires.IsZero() && !s.clock.Now().Before(e.expires)) {
		return entry{}, false
	}
	return e, true
}

func expiry(c *clock, ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return c.Now().Add(ttl)
}

func (s *store) Get(_ context.Context, ns, key string) ([]byte, error) {
	s.reads.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(ns + "/" + key)
	if !ok {
		return nil, kernel.ErrKeyNotFound
	}
	return e.value, nil
}

func (s *store) Set(_ context.Context, ns, key string, value []byte, ttl time.Duration) error {
	s.writes.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ns+"/"+key] = entry{value: value, expires: expiry(s.clock, ttl)}
	return nil
}

func (s *store) Delete(_ context.Context, ns, key string) error {
	s.writes.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, ns+"/"+key)
	return nil
}

func (s *store) IncrBy(_ context.Context, ns, key string, n int64, ttl time.Duration) (int64, error) {
	s.writes.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	k := ns + "/" + key
	e, ok := s.live(k)
	var v int64
	if ok {
		v, _ = strconv.ParseInt(string(e.value), 10, 64)
	} else {
		e.expires = expiry(s.clock, ttl)
	}
	v += n
	e.value = []byte(strconv.FormatInt(v, 10))
	s.values[k] = e
	return v, nil
}

func (s *store) DeleteExpired(context.Context, int) (int64, error) { return 0, nil }

// database counts the computations and answers them with fixed values. When block is set, a
// computation of the totals waits for it.
type database struct {
	backlogs atomic.Int64
	totals   atomic.Int64
	block    chan struct{}
	started  chan struct{}
}

func (d *database) Backlog(_ context.Context, now time.Time) (queue.Backlog, error) {
	d.backlogs.Add(1)
	return queue.Backlog{
		Ready:         map[string]int64{"delivery.deliver": 4, "media.process": 1, "retired.type": 2},
		OldestReadyAt: now.Add(-90 * time.Second),
		Dead:          3,
	}, nil
}

func (d *database) Totals(context.Context) (metrics.Totals, error) {
	d.totals.Add(1)
	if d.block != nil {
		d.started <- struct{}{}
		<-d.block
	}
	dbBytes := int64(50000)
	return metrics.Totals{
		Users: 10, Statuses: 200, MediaLocalBytes: 3000, MediaRemoteCacheBytes: 400,
		DatabaseBytes: &dbBytes, InstancesFailing: 5, InstancesUnavailable: 2,
		BillingImportFailing: map[string]bool{"aws": true, "gcp": false},
	}, nil
}

// process is one server process: a Service with the gauges registered on a meter that
// collects them.
type process struct {
	svc   *metrics.Service
	meter *collector
}

func newProcess(kv kernel.KeyValueStore, c *clock, db *database, logs *bytes.Buffer) *process {
	if logs == nil {
		logs = &bytes.Buffer{}
	}
	svc := metrics.NewService(metrics.Deps{
		KV: kv, Clock: c, Backlog: db, Totals: db.Totals,
		KnownJobType: func(t string) bool { return t != "retired.type" },
		Logger:       slog.New(slog.NewJSONHandler(logs, nil)),
	})
	m := &collector{}
	svc.RegisterGauges(m)
	return &process{svc: svc, meter: m}
}

// collector is a kernel.Meter that keeps the gauge functions and calls them on collect.
type collector struct {
	kernel.NopMeter
	mu     sync.Mutex
	gauges []gauge
}

type gauge struct {
	i kernel.Instrument
	f kernel.GaugeFunc
}

func (c *collector) Gauge(i kernel.Instrument, f kernel.GaugeFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gauges = append(c.gauges, gauge{i, f})
}

type observer struct {
	t      *testing.T
	i      kernel.Instrument
	values map[string]float64
}

func (o observer) Observe(v float64, l kernel.Labels) {
	if err := o.i.Check(l); err != nil {
		o.t.Error(err)
	}
	key := o.i.Name
	for k, val := range l.All() {
		key += fmt.Sprintf("{%s=%s}", k, val)
	}
	o.values[key] = v
}

// collect calls every gauge function, as an exporter does, and returns the values by name and
// labels.
func (c *collector) collect(t *testing.T) map[string]float64 {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	values := map[string]float64{}
	for _, g := range c.gauges {
		if err := g.f(t.Context(), observer{t: t, i: g.i, values: values}); err != nil {
			t.Fatalf("gauge %s: %v", g.i.Name, err)
		}
	}
	return values
}

// gaugeWriter is a collector that is a kernel.GaugeWriter, as the emf Meter is.
type gaugeWriter struct {
	*collector
	t       *testing.T
	written []map[string]float64
}

func (w *gaugeWriter) WriteGauges(context.Context) error {
	w.written = append(w.written, w.collect(w.t))
	return nil
}

var start = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

var want = map[string]float64{
	"my.queue.ready{my.job.type=delivery.deliver}":        4,
	"my.queue.ready{my.job.type=media.process}":           1,
	"my.queue.ready{my.job.type=other}":                   2,
	"my.queue.oldest_ready_age":                           90,
	"my.queue.dead":                                       3,
	"my.instance.users":                                   10,
	"my.instance.statuses":                                200,
	"my.media.stored_bytes{my.media.origin=local}":        3000,
	"my.media.stored_bytes{my.media.origin=remote_cache}": 400,
	"my.db.size": 50000,
	"my.billing.import.failing{my.billing.source=aws}":                  1,
	"my.billing.import.failing{my.billing.source=gcp}":                  0,
	"my.federation.instances{my.federation.instance.state=available}":   5,
	"my.federation.instances{my.federation.instance.state=unavailable}": 2,
	"my.metrics.snapshot.age{my.metrics.snapshot.part=queue}":           0,
	"my.metrics.snapshot.age{my.metrics.snapshot.part=totals}":          0,
}

// withAges returns want with the ages of the parts.
func withAges(queueAge, totalsAge time.Duration) map[string]float64 {
	w := maps.Clone(want)
	w["my.metrics.snapshot.age{my.metrics.snapshot.part=queue}"] = queueAge.Seconds()
	w["my.metrics.snapshot.age{my.metrics.snapshot.part=totals}"] = totalsAge.Seconds()
	return w
}

func TestProcessesShareOneComputation(t *testing.T) {
	c := &clock{now: start}
	kv := newStore(c)
	db := &database{}
	a, b := newProcess(kv, c, db, nil), newProcess(kv, c, db, nil)

	// The first collection computes the snapshot; the other process reads it.
	if got := a.meter.collect(t); !maps.Equal(got, want) {
		t.Fatalf("process a = %v, want %v", got, want)
	}
	if got := b.meter.collect(t); !maps.Equal(got, want) {
		t.Fatalf("process b = %v, want %v", got, want)
	}
	if db.backlogs.Load() != 1 || db.totals.Load() != 1 {
		t.Fatalf("computed the queue %d times and the totals %d times, want once each", db.backlogs.Load(), db.totals.Load())
	}

	// Within the limits, the processes read the store and compute nothing.
	c.advance(20 * time.Second)
	for _, p := range []*process{a, b} {
		if got := p.meter.collect(t); !maps.Equal(got, withAges(20*time.Second, 20*time.Second)) {
			t.Fatalf("after 20 s = %v", got)
		}
	}
	if db.backlogs.Load() != 1 || db.totals.Load() != 1 {
		t.Fatalf("computed again within the limits: %d, %d", db.backlogs.Load(), db.totals.Load())
	}

	// Once the queue part is older than its limit, one process computes it again, and only it.
	c.advance(metrics.QueueMaxAge)
	for _, p := range []*process{b, a} {
		if got := p.meter.collect(t); !maps.Equal(got, withAges(0, 20*time.Second+metrics.QueueMaxAge)) {
			t.Fatalf("after the queue limit = %v", got)
		}
	}
	if db.backlogs.Load() != 2 || db.totals.Load() != 1 {
		t.Fatalf("after the queue limit: computed the queue %d times and the totals %d times, want 2 and 1", db.backlogs.Load(), db.totals.Load())
	}
}

func TestOnlyTheLockHolderComputes(t *testing.T) {
	c := &clock{now: start}
	kv := newStore(c)
	db := &database{block: make(chan struct{}), started: make(chan struct{})}
	a, b := newProcess(kv, c, db, nil), newProcess(kv, c, db, nil)

	done := make(chan map[string]float64)
	go func() { done <- a.meter.collect(t) }()
	<-db.started // a holds the lock and computes

	// b finds no snapshot and no lock to take: it reports nothing rather than zeros.
	if got := b.meter.collect(t); len(got) != 0 {
		t.Errorf("process b without a snapshot = %v, want nothing", got)
	}
	close(db.block)
	if got := <-done; !maps.Equal(got, want) {
		t.Errorf("process a = %v, want %v", got, want)
	}
	if db.totals.Load() != 1 {
		t.Errorf("computed the totals %d times, want once", db.totals.Load())
	}
	// Once b's cache expires, it reads what a computed.
	c.advance(metrics.CacheTTL)
	if got := b.meter.collect(t); !maps.Equal(got, withAges(metrics.CacheTTL, metrics.CacheTTL)) {
		t.Errorf("process b after its cache = %v", got)
	}
	if db.totals.Load() != 1 || db.backlogs.Load() != 1 {
		t.Errorf("computed %d, %d times, want once", db.totals.Load(), db.backlogs.Load())
	}
}

func TestCollectionsDoNotWriteTheStore(t *testing.T) {
	c := &clock{now: start}
	kv := newStore(c)
	db := &database{}
	p := newProcess(kv, c, db, nil)
	p.meter.collect(t)
	// The first collection takes the lock, writes the snapshot, and releases the lock.
	if n := kv.writes.Load(); n != 3 {
		t.Fatalf("the first collection wrote %d times, want 3", n)
	}

	// Collections every second within the queue limit read the store once per CacheTTL and
	// never write it, as many requests as there are.
	for range 29 {
		c.advance(time.Second)
		p.meter.collect(t)
	}
	if n := kv.writes.Load(); n != 3 {
		t.Errorf("collections within the limits wrote %d times", n-3)
	}
	if n := kv.reads.Load(); n != 2 {
		t.Errorf("collections within 30 s read the store %d times, want 2", n)
	}
}

func TestTheJobLogsTheSnapshot(t *testing.T) {
	c := &clock{now: start}
	kv := newStore(c)
	db := &database{}
	var logs bytes.Buffer
	p := newProcess(kv, c, db, &logs)
	reg, sched := &queue.JobHandlerRegistry{}, queue.NewScheduler(nopQueue{}, c, slog.New(slog.DiscardHandler))
	p.svc.RegisterJobs(reg, sched, p.meter)
	h, _, ok := reg.Lookup(metrics.JobSnapshot)
	if !ok {
		t.Fatal("no handler for metrics.snapshot")
	}
	if res := h(t.Context(), queue.Job{Type: metrics.JobSnapshot}); !res.IsDone() {
		t.Fatalf("job = %+v, want Done", res)
	}
	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log %q: %v", logs.String(), err)
	}
	ready, _ := line["my.queue.ready"].(map[string]any)
	stored, _ := line["my.media.stored_bytes"].(map[string]any)
	instances, _ := line["my.federation.instances"].(map[string]any)
	failing, _ := line["my.billing.import.failing"].(map[string]any)
	if line["msg"] != "metrics snapshot" || ready["delivery.deliver"] != 4.0 || ready["other"] != 2.0 ||
		line["my.queue.dead"] != 3.0 || line["my.instance.users"] != 10.0 || stored["remote_cache"] != 400.0 ||
		line["my.db.size"] != 50000.0 || instances["unavailable"] != 2.0 || failing["aws"] != true {
		t.Errorf("log line = %v", line)
	}

	// The processes report what the job computed, without computing it again.
	other := newProcess(kv, c, db, nil)
	if got := other.meter.collect(t); !maps.Equal(got, want) {
		t.Errorf("another process = %v, want %v", got, want)
	}
	if db.totals.Load() != 1 {
		t.Errorf("computed the totals %d times, want once", db.totals.Load())
	}
}

func TestTheJobWritesTheGaugesOfEMF(t *testing.T) {
	c := &clock{now: start}
	kv := newStore(c)
	db := &database{}
	var logs bytes.Buffer
	svc := metrics.NewService(metrics.Deps{
		KV: kv, Clock: c, Backlog: db, Totals: db.Totals, Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	w := &gaugeWriter{collector: &collector{}, t: t}
	svc.RegisterGauges(w)
	reg, sched := &queue.JobHandlerRegistry{}, queue.NewScheduler(nopQueue{}, c, slog.New(slog.DiscardHandler))
	svc.RegisterJobs(reg, sched, w)
	h, _, _ := reg.Lookup(metrics.JobSnapshot)
	if res := h(t.Context(), queue.Job{Type: metrics.JobSnapshot}); !res.IsDone() {
		t.Fatalf("job = %+v, want Done", res)
	}
	if len(w.written) != 1 || w.written[0]["my.instance.statuses"] != 200 || w.written[0]["my.queue.ready{my.job.type=retired.type}"] != 2 {
		t.Errorf("written = %v", w.written)
	}
	if logs.Len() != 0 {
		t.Errorf("logged %q besides the EMF lines", logs.String())
	}
	if db.totals.Load() != 1 {
		t.Errorf("computed the totals %d times, want once", db.totals.Load())
	}
}

func TestWithoutABacklogReader(t *testing.T) {
	c := &clock{now: start}
	db := &database{}
	svc := metrics.NewService(metrics.Deps{KV: newStore(c), Clock: c, Totals: db.Totals})
	m := &collector{}
	svc.RegisterGauges(m)
	got := m.collect(t)
	if _, ok := got["my.queue.dead"]; ok || got["my.instance.users"] != 10 {
		t.Errorf("gauges = %v, want the totals only", got)
	}
}

func TestAFailedComputationKeepsNothing(t *testing.T) {
	c := &clock{now: start}
	kv := newStore(c)
	svc := metrics.NewService(metrics.Deps{
		KV: kv, Clock: c, Totals: func(context.Context) (metrics.Totals, error) { return metrics.Totals{}, errors.New("db down") },
		Logger: slog.New(slog.DiscardHandler),
	})
	m := &collector{}
	svc.RegisterGauges(m)
	if got := m.collect(t); len(got) != 0 {
		t.Errorf("gauges = %v, want nothing", got)
	}
	if _, err := kv.Get(t.Context(), metrics.Namespace, "snapshot"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("stored a snapshot after a failure: %v", err)
	}
	if _, err := kv.Get(t.Context(), metrics.Namespace, "lock"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("kept the lock after the computation: %v", err)
	}
}

type nopQueue struct{}

func (nopQueue) Enqueue(context.Context, ...queue.NewJob) error { return nil }
