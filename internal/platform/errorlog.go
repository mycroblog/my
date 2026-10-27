// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Labels of my.errors.reported (ADR-0103 §8).
const (
	LabelErrorSource = "my.error.source"
	LabelErrorKind   = "my.error.kind"
)

// ErrorsReported counts the error reports (ADR-0103 §8, ADR-0055 §3), every one of them,
// whatever their sinks do with it.
var ErrorsReported = kernel.Instrument{
	Name: "my.errors.reported", Unit: "{error}", Description: "Errors reported to the operator: panics, 5xx, dead jobs, startup failures.",
	LabelKeys: []string{LabelErrorSource, LabelErrorKind},
}

// reportedErrorEventType is the @type that makes Cloud Error Reporting take a log line as an
// error event whatever its fields are.
const reportedErrorEventType = "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"

// errorLogMsg is the msg of the line of an error report.
const errorLogMsg = "error report"

// ErrorLogSink writes each error report as one JSON line of the process logger (ADR-0103 §4
// (1)), in the form Cloud Error Reporting takes: @type, serviceContext, the short fields first,
// and message, the header line "<Type>: <Message>" and the stack, last, because the anomaly
// detection of CloudWatch Logs reads only the first 1,500 characters of a line. The level is
// ERROR, and CRITICAL for an error that stops the process. It is the only line with a stack.
type ErrorLogSink struct{ handler slog.Handler }

var _ ErrorSink = ErrorLogSink{}

// NewErrorLogSink returns the sink that writes the error reports through logger.
func NewErrorLogSink(logger *slog.Logger) ErrorLogSink {
	return ErrorLogSink{handler: logger.Handler()}
}

// ReceiveError writes ev's line. The request and trace IDs are those of ev: the context of the
// line is left empty, so that the logger's handlers do not add them a second time after message.
func (s ErrorLogSink) ReceiveError(_ context.Context, ev kernel.ErrorEvent) {
	ctx := context.Background()
	level := slog.LevelError
	if ev.Fatal {
		level = slog.LevelError + 4
	}
	if !s.handler.Enabled(ctx, level) {
		return
	}
	at := ev.Time
	if at.IsZero() {
		at = time.Now()
	}
	r := slog.NewRecord(at, level, errorLogMsg, 0)
	r.AddAttrs(
		slog.String("@type", reportedErrorEventType),
		slog.Group("serviceContext", slog.String("service", "my"), slog.String("version", ev.Version)),
		slog.String("my.error.fingerprint", ev.Fingerprint),
		slog.String(LabelErrorSource, string(ev.Source)),
		slog.String(LabelErrorKind, string(ev.Kind)),
		slog.String("my.error.where", ev.Where),
		slog.String("my.role", ev.Role),
		slog.String("deployment.environment.name", ev.Environment),
	)
	for _, a := range []struct{ key, value string }{
		{"request_id", ev.RequestID}, {"job_id", ev.JobID}, {"trace_id", ev.TraceID}, {"span_id", ev.SpanID},
	} {
		if a.value != "" {
			r.AddAttrs(slog.String(a.key, a.value))
		}
	}
	if ev.Attempt > 0 {
		r.AddAttrs(slog.Int("attempt", ev.Attempt))
	}
	r.AddAttrs(slog.String("message", errorLogMessage(ev)))
	_ = s.handler.Handle(ctx, r) // a line that cannot be written is not reported
}

// errorLogMessage joins the header of ev and its stack with one newline, as the Go library of
// Cloud Error Reporting does.
func errorLogMessage(ev kernel.ErrorEvent) string {
	header := ev.Message
	if ev.Type != "" {
		header = ev.Type + ": " + ev.Message
	}
	if ev.Stack == "" {
		return header
	}
	return header + "\n" + ev.Stack
}

// The limits of the exports (ADR-0103 §3): one export of a fingerprint a minute in a process,
// and 60 exports a minute in all. The reports over them still reach the log line and the error
// groups of the database.
const (
	exportFingerprintInterval = time.Minute
	exportsPerMinute          = 60
)

// exportLimiter passes the reports to the sink of errors_exporter within the limits of the
// exports and drops the others, which it counts through the sink's CountDropped when the sink
// has one (ExportQueue counts them as dropped in my.errors.exported).
type exportLimiter struct {
	sink  ErrorSink
	clock kernel.Clock

	mu          sync.Mutex
	last        map[string]time.Time
	windowStart time.Time
	inWindow    int
}

// newExportLimiter returns sink behind the limits of the exports.
func newExportLimiter(sink ErrorSink, clock kernel.Clock) *exportLimiter {
	return &exportLimiter{sink: sink, clock: clock, last: map[string]time.Time{}}
}

// ReceiveError passes ev to the sink when the limits allow it.
func (l *exportLimiter) ReceiveError(ctx context.Context, ev kernel.ErrorEvent) {
	if l.allow(ev.Fingerprint) {
		l.sink.ReceiveError(ctx, ev)
		return
	}
	if c, ok := l.sink.(interface{ CountDropped(context.Context) }); ok {
		c.CountDropped(ctx)
	}
}

// Close flushes the sink when it holds something.
func (l *exportLimiter) Close(ctx context.Context) error {
	if c, ok := l.sink.(ErrorSinkCloser); ok {
		return c.Close(ctx)
	}
	return nil
}

func (l *exportLimiter) allow(fingerprint string) bool {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.windowStart) >= time.Minute {
		l.windowStart, l.inWindow = now, 0
		for fp, at := range l.last { // forget what can no longer hold an export back
			if now.Sub(at) >= exportFingerprintInterval {
				delete(l.last, fp)
			}
		}
	}
	if at, ok := l.last[fingerprint]; ok && now.Sub(at) < exportFingerprintInterval {
		return false
	}
	if l.inWindow >= exportsPerMinute {
		return false
	}
	l.last[fingerprint] = now
	l.inWindow++
	return true
}

// ReportStartupFailure writes the error report of err, which stopped the server before it
// could start (the configuration, the database, the adapters), as the one line of
// ErrorLogSink through logger: no App and so no other sink exists yet.
func ReportStartupFailure(ctx context.Context, logger *slog.Logger, version, role, environment string, err error) {
	r := NewErrorReporter(version, role, environment, nil, nil)
	r.AddSink(NewErrorLogSink(logger))
	ev := kernel.NewErrorEvent(err, "startup")
	ev.Fatal = true
	r.Report(ctx, ev)
}
