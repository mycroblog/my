// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_otel

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/platform/config"
)

func init() { errorExporters["otlp"] = newOTLPErrorExporter }

// newOTLPErrorExporter reports that the build tag no_otel left the OpenTelemetry adapter out.
func newOTLPErrorExporter(_ context.Context, _ *App, cfg *config.Config, _ AppOptions) (ErrorSink, error) {
	return nil, config.ExcludedError("errors_exporter", cfg.ErrorsExporter, "no_otel")
}
