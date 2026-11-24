// SPDX-License-Identifier: AGPL-3.0-only

// Package none holds the kernel.Meter adapter of metrics_exporter = none and the kernel.Tracer
// adapter of traces_exporter = none, the defaults (ADR-0055 §2, ADR-0055 §13): they record
// nothing, so that an instance without metrics or traces costs what it did before them. The
// Meter still puts the adapter operations and the database pool's use of each unit of work on
// the line that ends it (sysmetrics.UnitLineMeter), and the Tracer still gives the log lines the
// trace ID of a trusted proxy's traceparent (ADR-0055 §18).
package none

import (
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
)

// New returns the Meter whose instruments record nothing and whose unit lines carry the
// adapter operations and the pool's use (sysmetrics.UnitLineMeter).
func New() kernel.Meter { return sysmetrics.NewUnitLineMeter() }

// NewTracer returns the Tracer that records nothing.
func NewTracer() kernel.Tracer { return kernel.NopTracer{} }
