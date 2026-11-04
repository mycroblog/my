// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Label keys of the connection pool metrics, from the OpenTelemetry semantic conventions.
const (
	LabelPoolName  = "db.client.connection.pool.name"
	LabelConnState = "db.client.connection.state"
)

// The connection pool metrics of ADR-0055 §10, named after the OpenTelemetry semantic
// conventions. The pools keep their waits as totals since they opened, so WaitTime, Waits, and
// Timeouts are observable counters of those totals, not histograms.
var (
	DBConnectionCount = kernel.Instrument{
		Name: "db.client.connection.count", Unit: "{connection}",
		Description: "Connections of the database pool, by state (idle or used).",
		LabelKeys:   []string{LabelPoolName, LabelConnState},
	}
	DBConnectionMax = kernel.Instrument{
		Name: "db.client.connection.max", Unit: "{connection}",
		Description: "Most open connections the database pool allows.",
		LabelKeys:   []string{LabelPoolName},
	}
	DBConnectionWaitTime = kernel.Instrument{
		Name: "db.client.connection.wait_time", Unit: "s",
		Description: "Total time spent waiting for a connection of the database pool since it opened.",
		LabelKeys:   []string{LabelPoolName},
	}
	DBConnectionWaits = kernel.Instrument{
		Name: "my.db.client.connection.waits", Unit: "{request}",
		Description: "Acquisitions of a connection of the database pool that had to wait, since it opened.",
		LabelKeys:   []string{LabelPoolName},
	}
	DBConnectionTimeouts = kernel.Instrument{
		Name: "db.client.connection.timeouts", Unit: "{timeout}",
		Description: "Acquisitions of a connection of the database pool that ended before getting one, since it opened.",
		LabelKeys:   []string{LabelPoolName},
	}
)

// PoolStats is what a database pool reports about its connections.
type PoolStats struct {
	Idle, Used int64
	// Max is the most open connections; 0 is no limit, which is not reported.
	Max int64
	// Waits and WaitTime are the acquisitions that waited and the time they waited.
	Waits    int64
	WaitTime time.Duration
	// Timeouts are the acquisitions that gave up; nil when the pool does not count them.
	Timeouts *int64
}

// RegisterPool reports the connection pool named pool (such as main) from stats, which the
// gauges and the observable counters call when the exporter collects, so that queries are not measured one by one (ADR-0055 §10).
func RegisterPool(m kernel.Meter, pool string, stats func() PoolStats) {
	if kernel.IsNop(m) {
		return
	}
	l := kernel.NewLabels(LabelPoolName, pool)
	// Reading the stats of a pool is cheap: each gauge reads them.
	read := stats
	m.Gauge(DBConnectionCount, func(_ context.Context, o kernel.GaugeObserver) error {
		s := read()
		o.Observe(float64(s.Idle), l.With(LabelConnState, "idle"))
		o.Observe(float64(s.Used), l.With(LabelConnState, "used"))
		return nil
	})
	m.Gauge(DBConnectionMax, func(_ context.Context, o kernel.GaugeObserver) error {
		if s := read(); s.Max > 0 {
			o.Observe(float64(s.Max), l)
		}
		return nil
	})
	m.ObservableCounter(DBConnectionWaitTime, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(read().WaitTime.Seconds(), l)
		return nil
	})
	m.ObservableCounter(DBConnectionWaits, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(float64(read().Waits), l)
		return nil
	})
	m.ObservableCounter(DBConnectionTimeouts, func(_ context.Context, o kernel.GaugeObserver) error {
		if s := read(); s.Timeouts != nil {
			o.Observe(float64(*s.Timeouts), l)
		}
		return nil
	})
}
