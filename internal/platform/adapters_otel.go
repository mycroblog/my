// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpOTelMeter sets the Meter of metrics_exporter = prometheus or otlp.
func (a *App) setUpOTelMeter(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	m, err := otel.New(ctx, otel.Options{
		Exporter: cfg.MetricsExporter, ServiceVersion: opts.Version, Role: string(cfg.Role), Namespace: cfg.Host,
	})
	if err != nil {
		return err
	}
	a.Meter, a.metricsHandler, a.closeMeter = m, m.Handler(), m.Shutdown
	return nil
}

// setUpOTelTracer sets the Tracer of traces_exporter = otlp (ADR-0055 §17).
func (a *App) setUpOTelTracer(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	t, err := otel.NewTracer(ctx, otel.TracerOptions{
		ServiceVersion: opts.Version, Role: string(cfg.Role), Namespace: cfg.Host, SampleRatio: cfg.TracesSampleRatio, Flush: cfg.TracesFlush,
	})
	if err != nil {
		return err
	}
	a.Tracer, a.closeTracer = t, t.Shutdown
	return nil
}

// setUpOTLPLogs makes the logger send its lines over OTLP (logs_exporter = otlp, ADR-0055 §20),
// with the trace and span of a.Tracer. The logger's handler so far, JSON lines on standard error,
// keeps deciding the levels, and writes the records that an export fails to send and the lines
// logged after Close. Call it once a.Tracer is set and before the trace IDs are added to the
// lines.
func (a *App) setUpOTLPLogs(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	l, err := otel.NewLogs(ctx, otel.LogsOptions{
		ServiceVersion: opts.Version, Role: string(cfg.Role), Namespace: cfg.Host,
		Fallback: a.Logger.Handler(), TraceContext: a.Tracer.LogIDs,
	})
	if err != nil {
		return err
	}
	a.Logger, a.closeLogs = slog.New(requestIDHandler{clientGoneHandler{l.Handler()}}), l.Shutdown
	return nil
}
