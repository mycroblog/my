// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/queue/inline"
	"github.com/mycroblog/my/internal/adapter/telemetry/none"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
)

// lineHandler keeps the attributes of the records it handles at level or above, one map per
// line, and drops the others as a logger whose level is disabled does.
type lineHandler struct {
	level slog.Level
	mu    sync.Mutex
	lines []map[string]slog.Value
}

func (h *lineHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *lineHandler) Handle(_ context.Context, r slog.Record) error {
	line := map[string]slog.Value{}
	r.Attrs(func(a slog.Attr) bool {
		line[a.Key] = a.Value.Resolve()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, line)
	return nil
}

func (h *lineHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *lineHandler) WithGroup(string) slog.Handler      { return h }

func (h *lineHandler) all() []map[string]slog.Value {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]map[string]slog.Value(nil), h.lines...)
}

// unitLine writes the line that ends the unit of ctx at INFO and returns its attributes.
func unitLine(ctx context.Context, t *testing.T, m kernel.Meter) map[string]slog.Value {
	t.Helper()
	h := &lineHandler{level: slog.LevelInfo}
	kernel.LogUnit(ctx, slog.New(h), slog.LevelInfo, "unit", m, []slog.Attr{slog.String("a", "1")})
	lines := h.all()
	if len(lines) != 1 {
		t.Fatalf("LogUnit wrote %d lines, want 1", len(lines))
	}
	if lines[0]["a"].String() != "1" {
		t.Errorf("the caller's attribute is missing from the line: %v", lines[0])
	}
	return lines[0]
}

// number returns the numeric value of v.
func number(t *testing.T, key string, v slog.Value) float64 {
	t.Helper()
	switch v.Kind() {
	case slog.KindInt64:
		return float64(v.Int64())
	case slog.KindUint64:
		return float64(v.Uint64())
	case slog.KindFloat64:
		return v.Float64()
	default:
		t.Fatalf("%s = %v (%v), want a number", key, v, v.Kind())
		return 0
	}
}

// wantNumber checks that line has key with value want.
func wantNumber(t *testing.T, line map[string]slog.Value, key string, want float64) {
	t.Helper()
	v, ok := line[key]
	if !ok {
		t.Errorf("no %s on the line %v, want %v", key, line, want)
		return
	}
	if got := number(t, key, v); got != want {
		t.Errorf("%s = %v, want %v", key, got, want)
	}
}

// wantAbsent checks that line has none of keys.
func wantAbsent(t *testing.T, line map[string]slog.Value, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if v, ok := line[k]; ok {
			t.Errorf("%s = %v on the line, want none", k, v)
		}
	}
}

// group returns the numbers of the group attribute key of line.
func group(t *testing.T, line map[string]slog.Value, key string) map[string]float64 {
	t.Helper()
	v, ok := line[key]
	if !ok {
		return nil
	}
	if v.Kind() != slog.KindGroup {
		t.Fatalf("%s = %v (%v), want a group", key, v, v.Kind())
	}
	out := map[string]float64{}
	for _, a := range v.Group() {
		out[a.Key] = number(t, key+"."+a.Key, a.Value.Resolve())
	}
	return out
}

var adapterKeys = []string{sysmetrics.LineAdapterOperations, sysmetrics.LineAdapterErrors, sysmetrics.LineAdapterErrorsByAdapter}

var waitKeys = []string{sysmetrics.DBConnectionWaits.Name, sysmetrics.DBConnectionWaitTime.Name, sysmetrics.DBConnectionTimeouts.Name}

// failingBlobs is a blob store whose Put fails.
type failingBlobs struct{ blob.Store }

func (failingBlobs) Put(context.Context, string, io.Reader, blob.PutOptions) (blob.Info, error) {
	return blob.Info{}, errors.New("down")
}

func newBlobs() *testkit.MemoryBlobStore { return testkit.NewMemoryBlobStore(newClock()) }

func newInline() *inline.Queue {
	clock := newClock()
	return inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
}

// TestUnitLineMeterIsNop checks that the Meter of metrics_exporter = none records nothing for
// kernel.IsNop, is a LineMeter, and is what none.New returns.
func TestUnitLineMeterIsNop(t *testing.T) {
	m := sysmetrics.NewUnitLineMeter()
	if !kernel.IsNop(m) {
		t.Error("kernel.IsNop(NewUnitLineMeter()) = false")
	}
	var _ kernel.LineMeter = m
	if _, ok := none.New().(*sysmetrics.UnitLineMeter); !ok {
		t.Errorf("none.New() = %T, want *sysmetrics.UnitLineMeter", none.New())
	}
	for _, nop := range []kernel.Meter{nil, kernel.NopMeter{}, struct{ kernel.NopMeter }{}} {
		if !kernel.IsNop(nop) {
			t.Errorf("kernel.IsNop(%T) = false", nop)
		}
	}
}

// TestDecoratorsWrapForTheUnitLine checks that every decorator wraps its adapter for a
// UnitLineMeter, which counts the operations although it is nop, and returns the adapter itself
// for NopMeter.
func TestDecoratorsWrapForTheUnitLine(t *testing.T) {
	m := sysmetrics.NewUnitLineMeter()
	kv := testkit.NewKeyValueStore(newClock())
	blobs := newBlobs()
	q := newInline()
	bus := stream.NewMemoryBroker().Bus()
	if got := sysmetrics.KV(kv, kernel.NopMeter{}, "kv.test"); got != kernel.KeyValueStore(kv) {
		t.Error("KV wraps the store with NopMeter")
	}
	if got := sysmetrics.KV(kv, m, "kv.test"); got == kernel.KeyValueStore(kv) {
		t.Error("KV returns the bare store with a UnitLineMeter")
	}
	if got := sysmetrics.Blobs(blobs, kernel.NopMeter{}, "blob.test"); got != blob.Store(blobs) {
		t.Error("Blobs wraps the store with NopMeter")
	}
	if got := sysmetrics.Blobs(blobs, m, "blob.test"); got == blob.Store(blobs) {
		t.Error("Blobs returns the bare store with a UnitLineMeter")
	}
	if got := sysmetrics.Jobs(q, kernel.NopMeter{}, "queue.test"); got != queue.JobQueue(q) {
		t.Error("Jobs wraps the queue with NopMeter")
	}
	if got := sysmetrics.Jobs(q, m, "queue.test"); got == queue.JobQueue(q) {
		t.Error("Jobs returns the bare queue with a UnitLineMeter")
	}
	if got := sysmetrics.Source(q, kernel.NopMeter{}, "queue.test"); got != queue.Source(q) {
		t.Error("Source wraps the queue with NopMeter")
	}
	if got := sysmetrics.Source(q, m, "queue.test"); got == queue.Source(q) {
		t.Error("Source returns the bare queue with a UnitLineMeter")
	}
	if got := sysmetrics.Bus(bus, kernel.NopMeter{}, "stream.test", nil); got != bus {
		t.Error("Bus wraps the bus with NopMeter")
	}
	if got := sysmetrics.Bus(bus, m, "stream.test", nil); got == bus {
		t.Error("Bus returns the bare bus with a UnitLineMeter")
	}
}

// newClock returns a clock for the stores.
func newClock() *testkit.Clock { return testkit.NewClock(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) }

// TestUnitLineAdapterOperations checks the adapter operations and errors on the line of a unit
// (done-when 1).
func TestUnitLineAdapterOperations(t *testing.T) {
	t.Run("every decorated adapter is counted", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		kv := sysmetrics.KV(testkit.NewKeyValueStore(newClock()), m, "kv.test")
		blobs := sysmetrics.Blobs(newBlobs(), m, "blob.test")
		q := newInline()
		jobs := sysmetrics.Jobs(q, m, "queue.test")
		source := sysmetrics.Source(q, m, "queue.test")
		bus := sysmetrics.Bus(stream.NewMemoryBroker().Bus(), m, "stream.test", nil)

		ctx, end := m.Begin(t.Context())
		defer end()
		if err := kv.Set(ctx, "ns", "k", []byte("v"), time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := blobs.Put(ctx, "a/b.txt", bytes.NewReader([]byte("x")), blob.PutOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := jobs.Enqueue(ctx, queue.NewJob{Type: "test.job"}); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := source.Claim(ctx, "w", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Minute); !ok || err != nil {
			t.Fatalf("Claim = %v, %v", ok, err)
		}
		if err := bus.Send(ctx, []byte("x")); err != nil {
			t.Fatal(err)
		}
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.LineAdapterOperations, 5)
		wantNumber(t, line, sysmetrics.LineAdapterErrors, 0)
		wantAbsent(t, line, sysmetrics.LineAdapterErrorsByAdapter)
	})

	t.Run("errors by adapter", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		kv := sysmetrics.KV(failingKV{testkit.NewKeyValueStore(newClock())}, m, "kv.test")
		blobs := sysmetrics.Blobs(failingBlobs{newBlobs()}, m, "blob.test")
		ctx, end := m.Begin(t.Context())
		defer end()
		for range 2 {
			if err := kv.Delete(ctx, "ns", "k"); err == nil {
				t.Fatal("Delete succeeded")
			}
		}
		if err := kv.Set(ctx, "ns", "k", []byte("v"), 0); err != nil {
			t.Fatal(err)
		}
		if _, err := blobs.Put(ctx, "a/b.txt", bytes.NewReader(nil), blob.PutOptions{}); err == nil {
			t.Fatal("Put succeeded")
		}
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.LineAdapterOperations, 4)
		wantNumber(t, line, sysmetrics.LineAdapterErrors, 3)
		got := group(t, line, sysmetrics.LineAdapterErrorsByAdapter)
		if len(got) != 2 || got["kv.test"] != 2 || got["blob.test"] != 1 {
			t.Errorf("%s = %v, want kv.test 2 and blob.test 1", sysmetrics.LineAdapterErrorsByAdapter, got)
		}
	})

	t.Run("a missing key or blob is not an error", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		kv := sysmetrics.KV(testkit.NewKeyValueStore(newClock()), m, "kv.test")
		blobs := sysmetrics.Blobs(newBlobs(), m, "blob.test")
		ctx, end := m.Begin(t.Context())
		defer end()
		if _, err := kv.Get(ctx, "ns", "missing"); !errors.Is(err, kernel.ErrKeyNotFound) {
			t.Fatalf("Get = %v", err)
		}
		if _, _, err := blobs.Get(ctx, "a/missing"); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("blob Get = %v", err)
		}
		if _, err := blobs.Head(ctx, "a/missing"); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("blob Head = %v", err)
		}
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.LineAdapterOperations, 3)
		wantNumber(t, line, sysmetrics.LineAdapterErrors, 0)
		wantAbsent(t, line, sysmetrics.LineAdapterErrorsByAdapter)
	})

	t.Run("no operation, no keys", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		ctx, end := m.Begin(t.Context())
		defer end()
		wantAbsent(t, unitLine(ctx, t, m), adapterKeys...)
	})

	t.Run("operations outside a unit are counted nowhere", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		kv := sysmetrics.KV(failingKV{testkit.NewKeyValueStore(newClock())}, m, "kv.test")
		if err := kv.Set(t.Context(), "ns", "k", []byte("v"), 0); err != nil {
			t.Fatal(err)
		}
		if err := kv.Delete(t.Context(), "ns", "k"); err == nil {
			t.Fatal("Delete succeeded")
		}
		// The line written outside a unit carries nothing of them.
		wantAbsent(t, unitLine(t.Context(), t, m), adapterKeys...)

		ctx, end := m.Begin(t.Context())
		defer end()
		wantAbsent(t, unitLine(ctx, t, m), adapterKeys...)

		ctx2, end2 := m.Begin(t.Context())
		defer end2()
		if err := kv.Set(ctx2, "ns", "k", []byte("v"), 0); err != nil {
			t.Fatal(err)
		}
		line := unitLine(ctx2, t, m)
		wantNumber(t, line, sysmetrics.LineAdapterOperations, 1)
		wantNumber(t, line, sysmetrics.LineAdapterErrors, 0)
	})

	t.Run("a nested unit counts its own", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		kv := sysmetrics.KV(failingKV{testkit.NewKeyValueStore(newClock())}, m, "kv.test")
		outer, endOuter := m.Begin(t.Context())
		defer endOuter()
		if err := kv.Set(outer, "ns", "k", []byte("v"), 0); err != nil {
			t.Fatal(err)
		}
		inner, endInner := m.Begin(outer)
		if err := kv.Set(inner, "ns", "k", []byte("v"), 0); err != nil {
			t.Fatal(err)
		}
		if err := kv.Delete(inner, "ns", "k"); err == nil {
			t.Fatal("Delete succeeded")
		}
		innerLine := unitLine(inner, t, m)
		endInner()
		wantNumber(t, innerLine, sysmetrics.LineAdapterOperations, 2)
		wantNumber(t, innerLine, sysmetrics.LineAdapterErrors, 1)
		if got := group(t, innerLine, sysmetrics.LineAdapterErrorsByAdapter); len(got) != 1 || got["kv.test"] != 1 {
			t.Errorf("inner %s = %v, want kv.test 1", sysmetrics.LineAdapterErrorsByAdapter, got)
		}
		outerLine := unitLine(outer, t, m)
		wantNumber(t, outerLine, sysmetrics.LineAdapterOperations, 1)
		wantNumber(t, outerLine, sysmetrics.LineAdapterErrors, 0)
		wantAbsent(t, outerLine, sysmetrics.LineAdapterErrorsByAdapter)
	})

	// go test -race reports a data race between the operations of one unit.
	t.Run("concurrent operations of a unit are all counted", func(t *testing.T) {
		const n = 64
		m := sysmetrics.NewUnitLineMeter()
		kv := sysmetrics.KV(failingKV{testkit.NewKeyValueStore(newClock())}, m, "kv.test")
		ctx, end := m.Begin(t.Context())
		defer end()
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				if i%4 == 0 {
					_ = kv.Delete(ctx, "ns", "k")
					return
				}
				_ = kv.Set(ctx, "ns", "k", []byte("v"), 0)
			})
		}
		wg.Wait()
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.LineAdapterOperations, n)
		wantNumber(t, line, sysmetrics.LineAdapterErrors, n/4)
		if got := group(t, line, sysmetrics.LineAdapterErrorsByAdapter); got["kv.test"] != n/4 {
			t.Errorf("%s = %v, want kv.test %d", sysmetrics.LineAdapterErrorsByAdapter, got, n/4)
		}
	})
}

// TestUnitLinePoolUsage checks db.client.connection.usage on the line (done-when 2): used over
// max when the line is written, the largest over the pools, and absent when no pool has a max.
func TestUnitLinePoolUsage(t *testing.T) {
	t.Run("at write time, the fullest pool", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		var used atomic.Int64
		used.Store(1)
		sysmetrics.RegisterPool(m, "main", func() sysmetrics.PoolStats {
			return sysmetrics.PoolStats{Used: used.Load(), Idle: 1, Max: 10}
		})
		sysmetrics.RegisterPool(m, "replica", func() sysmetrics.PoolStats {
			return sysmetrics.PoolStats{Used: 2, Max: 5}
		})
		sysmetrics.RegisterPool(m, "unlimited", func() sysmetrics.PoolStats {
			return sysmetrics.PoolStats{Used: 100}
		})
		ctx, end := m.Begin(t.Context())
		defer end()
		wantNumber(t, unitLine(ctx, t, m), sysmetrics.LinePoolUsage, 0.4)
		used.Store(8)
		wantNumber(t, unitLine(ctx, t, m), sysmetrics.LinePoolUsage, 0.8)
	})

	t.Run("no pool with a max", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		sysmetrics.RegisterPool(m, "unlimited", func() sysmetrics.PoolStats { return sysmetrics.PoolStats{Used: 3} })
		ctx, end := m.Begin(t.Context())
		defer end()
		wantAbsent(t, unitLine(ctx, t, m), sysmetrics.LinePoolUsage)
	})

	t.Run("no pool", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		ctx, end := m.Begin(t.Context())
		defer end()
		wantAbsent(t, unitLine(ctx, t, m), sysmetrics.LinePoolUsage)
	})
}

// pool is a pool whose totals the test raises; it is safe for concurrent use.
type pool struct {
	waits, waitTime, timeouts atomic.Int64
}

func (p *pool) stats() sysmetrics.PoolStats {
	timeouts := p.timeouts.Load()
	return sysmetrics.PoolStats{Used: 1, Max: 4, Waits: p.waits.Load(), WaitTime: time.Duration(p.waitTime.Load()), Timeouts: &timeouts}
}

func (p *pool) raise(waits int64, waitTime time.Duration, timeouts int64) {
	p.waits.Add(waits)
	p.waitTime.Add(int64(waitTime))
	p.timeouts.Add(timeouts)
}

// waitTime returns the wait_time of line as a Duration, rounded to the nanosecond, so that the
// sum of the lines compares exactly with the pool's total.
func waitTime(t *testing.T, line map[string]slog.Value) time.Duration {
	t.Helper()
	v, ok := line[sysmetrics.DBConnectionWaitTime.Name]
	if !ok {
		return 0
	}
	return time.Duration(math.Round(number(t, sysmetrics.DBConnectionWaitTime.Name, v) * float64(time.Second)))
}

// TestUnitLinePoolWaits checks the waits, wait time, and timeouts on the lines (done-when 3):
// each line carries the increase since the process's previous line, only when it is not zero.
func TestUnitLinePoolWaits(t *testing.T) {
	t.Run("the increase since the previous line", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		p := &pool{}
		p.raise(2, 1500*time.Millisecond, 1)
		sysmetrics.RegisterPool(m, "main", p.stats)
		ctx, end := m.Begin(t.Context())
		defer end()

		first := unitLine(ctx, t, m)
		wantNumber(t, first, sysmetrics.DBConnectionWaits.Name, 2)
		wantNumber(t, first, sysmetrics.DBConnectionWaitTime.Name, 1.5)
		wantNumber(t, first, sysmetrics.DBConnectionTimeouts.Name, 1)

		wantAbsent(t, unitLine(ctx, t, m), waitKeys...)

		p.raise(3, 250*time.Millisecond, 0)
		third := unitLine(ctx, t, m)
		wantNumber(t, third, sysmetrics.DBConnectionWaits.Name, 3)
		wantNumber(t, third, sysmetrics.DBConnectionWaitTime.Name, 0.25)
		wantAbsent(t, third, sysmetrics.DBConnectionTimeouts.Name)
	})

	t.Run("the pools' increases add up", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		a, b := &pool{}, &pool{}
		sysmetrics.RegisterPool(m, "main", a.stats)
		sysmetrics.RegisterPool(m, "replica", b.stats)
		ctx, end := m.Begin(t.Context())
		defer end()
		a.raise(1, time.Second, 1)
		b.raise(2, time.Second, 0)
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.DBConnectionWaits.Name, 3)
		wantNumber(t, line, sysmetrics.DBConnectionWaitTime.Name, 2)
		wantNumber(t, line, sysmetrics.DBConnectionTimeouts.Name, 1)
	})

	t.Run("a pool that does not count timeouts", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		sysmetrics.RegisterPool(m, "main", func() sysmetrics.PoolStats {
			return sysmetrics.PoolStats{Used: 1, Max: 4, Waits: 1, WaitTime: time.Second}
		})
		ctx, end := m.Begin(t.Context())
		defer end()
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.DBConnectionWaits.Name, 1)
		wantAbsent(t, line, sysmetrics.DBConnectionTimeouts.Name)
	})

	t.Run("a line of a disabled level takes nothing", func(t *testing.T) {
		m := sysmetrics.NewUnitLineMeter()
		p := &pool{}
		sysmetrics.RegisterPool(m, "main", p.stats)
		ctx, end := m.Begin(t.Context())
		defer end()
		p.raise(4, 2*time.Second, 2)
		h := &lineHandler{level: slog.LevelInfo}
		kernel.LogUnit(ctx, slog.New(h), slog.LevelDebug, "unit", m, nil)
		if got := h.all(); len(got) != 0 {
			t.Fatalf("a DEBUG line was written at INFO: %v", got)
		}
		line := unitLine(ctx, t, m)
		wantNumber(t, line, sysmetrics.DBConnectionWaits.Name, 4)
		wantNumber(t, line, sysmetrics.DBConnectionWaitTime.Name, 2)
		wantNumber(t, line, sysmetrics.DBConnectionTimeouts.Name, 2)
	})

	// Every increase goes on exactly one line, also when the lines are written concurrently, so
	// their sum is the pool's total; go test -race reports a data race.
	t.Run("summed over concurrent lines, the totals", func(t *testing.T) {
		const writers, perWriter = 16, 32
		m := sysmetrics.NewUnitLineMeter()
		p := &pool{}
		sysmetrics.RegisterPool(m, "main", p.stats)
		h := &lineHandler{level: slog.LevelInfo}
		logger := slog.New(h)
		var wg sync.WaitGroup
		for range writers {
			wg.Go(func() {
				for i := range perWriter {
					p.raise(1, time.Millisecond+time.Duration(i)*time.Microsecond, int64(i%2))
					ctx, end := m.Begin(t.Context())
					kernel.LogUnit(ctx, logger, slog.LevelInfo, "unit", m, nil)
					end()
				}
			})
		}
		wg.Wait()
		ctx, end := m.Begin(t.Context())
		kernel.LogUnit(ctx, logger, slog.LevelInfo, "unit", m, nil)
		end()

		var waits, timeouts float64
		var wait time.Duration
		for _, line := range h.all() {
			if v, ok := line[sysmetrics.DBConnectionWaits.Name]; ok {
				n := number(t, sysmetrics.DBConnectionWaits.Name, v)
				if n <= 0 {
					t.Errorf("%s = %v on a line, want only increases", sysmetrics.DBConnectionWaits.Name, n)
				}
				waits += n
			}
			if v, ok := line[sysmetrics.DBConnectionTimeouts.Name]; ok {
				timeouts += number(t, sysmetrics.DBConnectionTimeouts.Name, v)
			}
			wait += waitTime(t, line)
		}
		total := p.stats()
		if waits != float64(total.Waits) {
			t.Errorf("sum of %s = %v, want %d", sysmetrics.DBConnectionWaits.Name, waits, total.Waits)
		}
		if wait != total.WaitTime {
			t.Errorf("sum of %s = %v, want %v", sysmetrics.DBConnectionWaitTime.Name, wait, total.WaitTime)
		}
		if timeouts != float64(*total.Timeouts) {
			t.Errorf("sum of %s = %v, want %d", sysmetrics.DBConnectionTimeouts.Name, timeouts, *total.Timeouts)
		}
	})
}
