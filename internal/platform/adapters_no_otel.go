// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_otel

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/platform/config"
)

// setUpOTelMeter reports that the build tag no_otel left the OpenTelemetry adapter out.
func (a *App) setUpOTelMeter(_ context.Context, cfg *config.Config, _ AppOptions) error {
	return config.ExcludedError("metrics_exporter", cfg.MetricsExporter, "no_otel")
}

// setUpOTelTracer reports that the build tag no_otel left the OpenTelemetry adapter out.
func (a *App) setUpOTelTracer(_ context.Context, cfg *config.Config, _ AppOptions) error {
	return config.ExcludedError("traces_exporter", cfg.TracesExporter, "no_otel")
}

// setUpOTLPLogs reports that the build tag no_otel left the OpenTelemetry adapter out.
func (a *App) setUpOTLPLogs(_ context.Context, cfg *config.Config, _ AppOptions) error {
	return config.ExcludedError("logs_exporter", cfg.LogsExporter, "no_otel")
}
