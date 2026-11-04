// SPDX-License-Identifier: AGPL-3.0-only

// Package none holds the kernel.Meter adapter of metrics_exporter = none and the kernel.Tracer
// adapter of traces_exporter = none, the defaults (ADR-0055 §2, ADR-0055 §13): they record
// nothing, so that an instance without metrics or traces costs what it did before them. The
// Tracer still gives the log lines the trace ID of a trusted proxy's traceparent (ADR-0055 §18).
package none

import "github.com/mycroblog/my/internal/app/kernel"

// New returns the Meter that records nothing.
func New() kernel.Meter { return kernel.NopMeter{} }

// NewTracer returns the Tracer that records nothing.
func NewTracer() kernel.Tracer { return kernel.NopTracer{} }
