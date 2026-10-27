// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/errorreport/otlp"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { errorExporters[otlp.Name] = newOTLPErrorExporter }

// newOTLPErrorExporter returns the exporter of errors_exporter = otlp (ADR-0103 §4 (3)): the
// shared send queue in front of the OpenTelemetry log client, whose SimpleProcessor exports one
// record per report. The client stops once the queue has drained.
func newOTLPErrorExporter(ctx context.Context, a *App, cfg *config.Config, opts AppOptions) (ErrorSink, error) {
	c, err := otlp.New(ctx, otlp.Options{ServiceVersion: opts.Version, Role: string(cfg.Role)})
	if err != nil {
		return nil, err
	}
	return &closingExportQueue{ExportQueue: NewExportQueue(c, a.Meter), shutdown: c.Shutdown}, nil
}
