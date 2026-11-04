// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpOTelMeter sets the Meter of metrics_exporter = prometheus or otlp.
func (a *App) setUpOTelMeter(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	m, err := otel.New(ctx, otel.Options{Exporter: cfg.MetricsExporter, ServiceVersion: opts.Version, Role: string(cfg.Role)})
	if err != nil {
		return err
	}
	a.Meter, a.metricsHandler, a.closeMeter = m, m.Handler(), m.Shutdown
	return nil
}

// setUpOTelTracer sets the Tracer of traces_exporter = otlp (ADR-0055 §17).
func (a *App) setUpOTelTracer(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	t, err := otel.NewTracer(ctx, otel.TracerOptions{
		ServiceVersion: opts.Version, Role: string(cfg.Role), SampleRatio: cfg.TracesSampleRatio, Flush: cfg.TracesFlush,
	})
	if err != nil {
		return err
	}
	a.Tracer, a.closeTracer = t, t.Shutdown
	return nil
}
