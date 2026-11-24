// SPDX-License-Identifier: AGPL-3.0-only

// Package sysmetrics declares the system metrics of ADR-0055 §10 on kernel.Meter and measures
// them at the boundary of the adapters: the process (RegisterProcess), the connection pools of
// the database adapters (their RegisterMetrics), the operations of the queue, key-value,
// stream, and blob adapters (the KV, Blobs, Bus, Jobs, and Source decorators), the long-lived
// connections (TrackConn), and the outbound HTTP of internal/adapter/httpclient. The Go
// runtime metrics come from the otel adapter.
//
// Names follow the OpenTelemetry semantic conventions where they define the metric and start
// with "my." otherwise (ADR-0055 §3). Labels never carry a remote host, an account, a URL, or a
// blob key (ADR-0055 §10). The totals that the kernel and the pools keep (CPU time, connection
// waits) are observable counters, which only grow until the process restarts.
//
// The domain and application layers never see these metrics: the platform wraps the adapters
// with the decorators when it assembles them. With metrics_exporter = none, whose Meter is a
// UnitLineMeter, the decorators and RegisterPool put the adapter operations and the pools' use
// on the line that ends each unit of work instead (ADR-0055 §5); every decorator returns the
// adapter itself when the Meter records nothing and is no UnitLineMeter (kernel.NopMeter).
package sysmetrics

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Label keys.
const (
	LabelAdapter   = "my.adapter"
	LabelOperation = "my.operation"
	LabelOutcome   = "my.outcome"
)

// Outcomes of an adapter operation. A lookup that finds nothing (a missing key or blob) is a
// success: it is the answer, not a failure of the adapter.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// The adapter operation metrics of ADR-0055 §10 The failures are the measurements of
// AdapterOperationDuration whose my.outcome is error.
var (
	AdapterOperationDuration = kernel.Instrument{
		Name: "my.adapter.operation.duration", Unit: "s",
		Description: "Duration of the operations of the queue, key-value, stream, and blob adapters.",
		LabelKeys:   []string{LabelAdapter, LabelOperation, LabelOutcome}, Buckets: kernel.DurationBuckets,
	}
	AdapterConnections = kernel.Instrument{
		Name: "my.adapter.connections", Unit: "{connection}",
		Description: "Long-lived connections of the adapters that are open: the Valkey client's and the stream listener's.",
		LabelKeys:   []string{LabelAdapter},
	}
	AdapterReconnects = kernel.Instrument{
		Name: "my.adapter.reconnects", Unit: "{connection}",
		Description: "Long-lived connections of the adapters opened again after one was lost or closed.",
		LabelKeys:   []string{LabelAdapter},
	}
)

// measures reports whether the decorators measure the operations of the adapters on m: m
// records them, or it is a UnitLineMeter, which counts them on the line of each unit of work.
func measures(m kernel.Meter) bool {
	_, lines := m.(*UnitLineMeter)
	return lines || !kernel.IsNop(m)
}

// operations records the duration and outcome of the operations of one adapter, or, on a
// UnitLineMeter, counts them in the unit of work of their context.
type operations struct {
	h       kernel.Histogram
	adapter string
	perUnit bool
}

func newOperations(m kernel.Meter, adapter string) operations {
	_, perUnit := m.(*UnitLineMeter)
	return operations{h: m.Histogram(AdapterOperationDuration), adapter: adapter, perUnit: perUnit}
}

// done records the operation op that started at start and failed with err, unless ok says
// that err is an answer rather than a failure.
func (o operations) done(ctx context.Context, op string, start time.Time, err error, ok func(error) bool) {
	outcome := OutcomeSuccess
	if err != nil && (ok == nil || !ok(err)) {
		outcome = OutcomeError
	}
	if o.perUnit {
		countInUnit(ctx, o.adapter, outcome == OutcomeError)
		return
	}
	o.h.Record(context.WithoutCancel(ctx), time.Since(start).Seconds(),
		kernel.NewLabels(LabelAdapter, o.adapter, LabelOperation, op, LabelOutcome, outcome))
}
