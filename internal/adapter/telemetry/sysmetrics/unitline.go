// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The attributes that UnitLineMeter adds to the line that ends a unit of work. The pool's
// usage and waits take the names of the pool metrics (DBConnectionWaits and the others).
const (
	// LineAdapterOperations is the number of adapter operations that the unit ran.
	LineAdapterOperations = "my.adapter.operations.total"
	// LineAdapterErrors is the number of those whose my.outcome is error.
	LineAdapterErrors = "my.adapter.errors.total"
	// LineAdapterErrorsByAdapter is the group of the errors by my.adapter value.
	LineAdapterErrorsByAdapter = "my.adapter.errors"
	// LinePoolUsage is the used connections over the most the pool allows, of the fullest pool.
	LinePoolUsage = "db.client.connection.usage"
)

// UnitLineMeter is the Meter of metrics_exporter = none (ADR-0055 §2): its instruments record
// nothing (kernel.IsNop reports true), and the line that ends each unit of work
// (kernel.LogUnit) carries what the alerts on the adapters and the database pool need when
// no exporter collects and the process gets no CPU between requests to write on a timer:
//
//   - the operations of the decorated adapters (KV, Blobs, Jobs, Source, Bus) that ran in the
//     unit, LineAdapterOperations and LineAdapterErrors, and the errors by adapter in the group
//     LineAdapterErrorsByAdapter, only when the unit ran any; an operation outside a unit is
//     not counted;
//   - LinePoolUsage of the pools that RegisterPool reported, when the line is written;
//   - the increase of the pools' DBConnectionWaits, DBConnectionWaitTime (seconds), and
//     DBConnectionTimeouts since the process's previous line, each only when it is not zero.
//     Each increase goes on exactly one line of the process, so their sum over the lines is
//     the pools' total.
//
// It is safe for concurrent use.
type UnitLineMeter struct {
	kernel.NopMeter

	pools atomic.Pointer[[]func() PoolStats]
	// The pools' totals that the lines have carried so far.
	waits, waitTime, timeouts atomic.Int64
}

var (
	_ kernel.Meter     = (*UnitLineMeter)(nil)
	_ kernel.LineMeter = (*UnitLineMeter)(nil)
)

// NewUnitLineMeter returns the Meter of metrics_exporter = none.
func NewUnitLineMeter() *UnitLineMeter { return &UnitLineMeter{} }

// unitOpsKey is the context key of a unit's *unitOps.
type unitOpsKey struct{}

// unitOps counts the adapter operations of one unit of work.
type unitOps struct {
	total atomic.Int64

	mu     sync.Mutex
	errors map[string]int64 // by my.adapter
}

func (u *unitOps) add(adapter string, failed bool) {
	u.total.Add(1)
	if !failed {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.errors == nil {
		u.errors = make(map[string]int64, 1)
	}
	u.errors[adapter]++
}

// countInUnit counts an operation of adapter in the unit of work that ctx belongs to, if any.
func countInUnit(ctx context.Context, adapter string, failed bool) {
	if u, _ := ctx.Value(unitOpsKey{}).(*unitOps); u != nil {
		u.add(adapter, failed)
	}
}

// Begin implements kernel.Meter: the returned context counts the adapter operations of the unit.
func (*UnitLineMeter) Begin(ctx context.Context) (context.Context, func()) {
	return context.WithValue(ctx, unitOpsKey{}, &unitOps{}), func() {}
}

// Line implements kernel.LineMeter: it adds to attrs the attributes of UnitLineMeter. It
// ignores instruments, which record nothing.
func (m *UnitLineMeter) Line(ctx context.Context, attrs []slog.Attr, _ ...kernel.Instrument) []slog.Attr {
	if u, _ := ctx.Value(unitOpsKey{}).(*unitOps); u != nil {
		attrs = u.appendAttrs(attrs)
	}
	return m.appendPools(attrs)
}

func (u *unitOps) appendAttrs(attrs []slog.Attr) []slog.Attr {
	total := u.total.Load()
	if total == 0 {
		return attrs
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	var failed int64
	adapters := make([]string, 0, len(u.errors))
	for a, n := range u.errors {
		failed += n
		adapters = append(adapters, a)
	}
	attrs = append(attrs, slog.Int64(LineAdapterOperations, total), slog.Int64(LineAdapterErrors, failed))
	if failed == 0 {
		return attrs
	}
	slices.Sort(adapters)
	group := make([]any, 0, len(adapters))
	for _, a := range adapters {
		group = append(group, slog.Int64(a, u.errors[a]))
	}
	return append(attrs, slog.Group(LineAdapterErrorsByAdapter, group...))
}

func (m *UnitLineMeter) addPool(stats func() PoolStats) {
	for {
		old := m.pools.Load()
		var next []func() PoolStats
		if old != nil {
			next = slices.Clone(*old)
		}
		next = append(next, stats)
		if m.pools.CompareAndSwap(old, &next) {
			return
		}
	}
}

func (m *UnitLineMeter) appendPools(attrs []slog.Attr) []slog.Attr {
	pools := m.pools.Load()
	if pools == nil {
		return attrs
	}
	usage, hasUsage := 0.0, false
	var waits, waitTime, timeouts int64
	hasTimeouts := false
	for _, stats := range *pools {
		s := stats()
		if s.Max > 0 {
			usage, hasUsage = max(usage, float64(s.Used)/float64(s.Max)), true
		}
		waits += s.Waits
		waitTime += int64(s.WaitTime)
		if s.Timeouts != nil {
			timeouts += *s.Timeouts
			hasTimeouts = true
		}
	}
	if hasUsage {
		attrs = append(attrs, slog.Float64(LinePoolUsage, usage))
	}
	if d := advance(&m.waits, waits); d > 0 {
		attrs = append(attrs, slog.Int64(DBConnectionWaits.Name, d))
	}
	if d := advance(&m.waitTime, waitTime); d > 0 {
		attrs = append(attrs, slog.Float64(DBConnectionWaitTime.Name, time.Duration(d).Seconds()))
	}
	if hasTimeouts {
		if d := advance(&m.timeouts, timeouts); d > 0 {
			attrs = append(attrs, slog.Int64(DBConnectionTimeouts.Name, d))
		}
	}
	return attrs
}

// advance moves seen forward to total and returns by how much, or 0 when another line has
// already carried total, so that each increase of a total goes on one line.
func advance(seen *atomic.Int64, total int64) int64 {
	for {
		old := seen.Load()
		if total <= old {
			return 0
		}
		if seen.CompareAndSwap(old, total) {
			return total - old
		}
	}
}
