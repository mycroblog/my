// SPDX-License-Identifier: AGPL-3.0-only

// Package otlp sends the error reports of errors_exporter = otlp (ADR-0103 §4 (3)) as
// OpenTelemetry log records of the exception semantic conventions over OTLP/HTTP (protobuf,
// gzip), to the receiver that the standard OTEL_EXPORTER_OTLP_LOGS_ENDPOINT or
// OTEL_EXPORTER_OTLP_ENDPOINT and OTEL_EXPORTER_OTLP_* environment variables name (default
// http://localhost:4318/v1/logs).
//
// Each record has the event name "exception", the severity ERROR (FATAL for an error that stops
// the process), the attributes exception.type, exception.message, exception.stacktrace,
// my.error.fingerprint, and my.error.where, the trace and span IDs of the event, and the
// resource of the otel telemetry adapter. The record never goes through Record.SetErr, which
// would fill exception.message from an error's unscrubbed text: only the fields of the
// ErrorEvent, which the reporter has scrubbed, leave the process.
//
// The SDK's SimpleProcessor exports each record while SendError runs, so the send queue of the
// reporter decides when to send and counts the outcome; there is no batch processor and no
// retry. The destination is the receiver that the operator configured, so the HTTP client is the
// exporter's own, not the SSRF-guarded kernel.HTTPClient.
package otlp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Name is the value of errors_exporter that selects this adapter.
const Name = "otlp"

// eventName is the event name of the exception records (OpenTelemetry semantic conventions).
const eventName = "exception"

// Options configure New.
type Options struct {
	// ServiceName is the service.name of the resource; the default is "my".
	ServiceName    string
	ServiceVersion string
	// Role is the my.role of the resource: web, worker, or all.
	Role string
	// Exporter replaces the OTLP/HTTP exporter. Tests only.
	Exporter sdklog.Exporter
}

// Client sends ErrorEvents as OpenTelemetry log records. It is safe for concurrent use.
type Client struct {
	provider *sdklog.LoggerProvider
	logger   log.Logger
}

// New returns the Client that exports through OTLP/HTTP, or opts.Exporter. Shut it down to stop
// the exporter.
func New(ctx context.Context, opts Options) (*Client, error) {
	res, err := otel.NewResource(ctx, opts.ServiceName, opts.ServiceVersion, opts.Role)
	if err != nil {
		return nil, fmt.Errorf("logs resource: %w", err)
	}
	exp := opts.Exporter
	if exp == nil {
		exp, err = otlploghttp.New(ctx,
			otlploghttp.WithCompression(otlploghttp.GzipCompression),
			// The send queue gives each report one attempt; a failure counts as failed.
			otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}),
		)
		if err != nil {
			return nil, fmt.Errorf("otlp log exporter: %w", err)
		}
	}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(resultExporter{exp})),
	)
	return &Client{provider: provider, logger: provider.Logger("github.com/mycroblog/my")}, nil
}

// Shutdown stops the exporter.
func (c *Client) Shutdown(ctx context.Context) error { return c.provider.Shutdown(ctx) }

// SendError exports ev as one log record and returns the error of the export, nil when the
// receiver took it.
func (c *Client) SendError(ctx context.Context, ev kernel.ErrorEvent) error {
	if sc, ok := spanContext(ev.TraceID, ev.SpanID); ok {
		// The queue's context is not the request's: the IDs come from the event.
		ctx = trace.ContextWithSpanContext(ctx, sc)
	}
	var result exportResult
	ctx = context.WithValue(ctx, resultKey{}, &result)
	c.logger.Emit(ctx, record(ev))
	if !result.done {
		return errors.New("otlp: the record was not exported")
	}
	return result.err
}

// record returns the log record of ev.
func record(ev kernel.ErrorEvent) log.Record {
	var r log.Record
	r.SetEventName(eventName)
	r.SetTimestamp(ev.Time)
	if ev.Fatal {
		r.SetSeverity(log.SeverityFatal)
		r.SetSeverityText("FATAL")
	} else {
		r.SetSeverity(log.SeverityError)
		r.SetSeverityText("ERROR")
	}
	typ := lastType(ev.Type)
	r.SetBody(attribute.StringValue(typ + ": " + ev.Message))
	r.AddAttributes(
		attribute.String("exception.type", typ),
		attribute.String("exception.message", ev.Message),
		attribute.String("exception.stacktrace", ev.Stack),
		attribute.String("my.error.fingerprint", ev.Fingerprint),
		attribute.String("my.error.where", ev.Where),
		attribute.String("my.error.source", string(ev.Source)),
		attribute.String("my.error.kind", string(ev.Kind)),
	)
	if ev.Environment != "" {
		r.AddAttributes(attribute.String("deployment.environment.name", ev.Environment))
	}
	return r
}

// spanContext returns the span context of the hexadecimal IDs of an event.
func spanContext(traceID, spanID string) (trace.SpanContext, bool) {
	tid, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		return trace.SpanContext{}, false
	}
	sid, _ := trace.SpanIDFromHex(spanID) // a trace without a span still links the record
	return trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid}), true
}

// lastType returns the innermost type of the chain t, "*fmt.wrapError > *pgconn.PgError".
func lastType(t string) string {
	if i := strings.LastIndex(t, " > "); i >= 0 {
		t = t[i+len(" > "):]
	}
	if t == "" {
		return "error"
	}
	return t
}

// resultKey is the context key of the *exportResult of one SendError.
type resultKey struct{}

// exportResult is what resultExporter reports back to SendError.
type exportResult struct {
	done bool
	err  error
}

// resultExporter hands the error of each export to the SendError whose context carries it,
// instead of to the SDK, which would pass it to the global error handler: the send queue counts
// the outcome, and a report's failure is never reported or logged again.
type resultExporter struct{ sdklog.Exporter }

func (e resultExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.Exporter.Export(ctx, records)
	if r, ok := ctx.Value(resultKey{}).(*exportResult); ok {
		r.done, r.err = true, err
		return nil
	}
	return err
}
