// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// ErrorSink is one output of the ErrorReporter (ADR-0103 §4): the log line, the error groups of
// the database, or the exporter of errors_exporter. ReceiveError gets every event after the
// reporter has filled it; it must not block for long (an exporter queues and sends in the
// background), must not report its own failures, and is called concurrently. A sink that holds
// something to send or write at the end also implements ErrorSinkCloser.
type ErrorSink interface {
	ReceiveError(ctx context.Context, ev kernel.ErrorEvent)
}

// ErrorSinkCloser is an ErrorSink that flushes what it holds when the App closes, within ctx.
type ErrorSinkCloser interface {
	ErrorSink
	Close(ctx context.Context) error
}

// ErrorExporter builds the ErrorSink of a value of errors_exporter other than none. The file of
// each exporter registers its builder in errorExporters from its init function.
type ErrorExporter func(ctx context.Context, a *App, cfg *config.Config, opts AppOptions) (ErrorSink, error)

// errorExporters holds the builder of each errors_exporter value, keyed by the value.
var errorExporters = map[string]ErrorExporter{}

// errorReporterCloseTimeout bounds the flush of the sinks when the App closes: the exporters
// wait up to 2 seconds for their queue (ADR-0103 §4), and the error groups write their counts.
const errorReporterCloseTimeout = 3 * time.Second

// ErrorReporter is the kernel.ErrorReporter of the App (ADR-0103 §1): it fills each event with
// the time, the version, the role, the environment, the place and the request and trace IDs of
// ctx, scrubs it, sets its fingerprint (ADR-0103 §1, §3), counts it in my.errors.reported, and
// hands it to every sink. Every sink receives the scrubbed event only. The modules register their sinks with AddSink while the App is set
// up; setUpErrorReporter adds the exporter of errors_exporter.
//
// While Serve runs, it is also the kernel.ProcessStopper of kernel.Go: the panic of a goroutine
// stops the server as a failure.
type ErrorReporter struct {
	version, role, environment string
	clock                      kernel.Clock
	tracer                     kernel.Tracer
	reported                   kernel.Counter

	mu    sync.RWMutex
	sinks []ErrorSink
	stop  func(cause error)
}

var (
	_ kernel.ErrorReporter  = (*ErrorReporter)(nil)
	_ kernel.ProcessStopper = (*ErrorReporter)(nil)
)

// NewErrorReporter returns an ErrorReporter without sinks for the process of version and role,
// with the environment name of errors_environment.
func NewErrorReporter(version, role, environment string, clock kernel.Clock, tracer kernel.Tracer) *ErrorReporter {
	if tracer == nil {
		tracer = kernel.NopTracer{}
	}
	return &ErrorReporter{
		version: version, role: role, environment: environment, clock: clock, tracer: tracer,
		reported: kernel.NopMeter{}.Counter(ErrorsReported),
	}
}

// AddSink adds s to the outputs of every later report.
func (r *ErrorReporter) AddSink(s ErrorSink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sinks = append(r.sinks, s)
}

// Report fills, scrubs, fingerprints, and counts ev, and hands it to every sink. A sink that
// panics is skipped: the reporter never reports its own failures.
func (r *ErrorReporter) Report(ctx context.Context, ev kernel.ErrorEvent) {
	if r == nil || clientGone(ctx, ev) {
		return
	}
	r.fill(ctx, &ev)
	scrubEvent(&ev)
	if ev.Fingerprint == "" {
		ev.Fingerprint = fingerprint(ev)
	}
	r.reported.Add(ctx, 1, kernel.NewLabels(LabelErrorSource, string(ev.Source), LabelErrorKind, string(ev.Kind)))
	r.mu.RLock()
	sinks := r.sinks
	r.mu.RUnlock()
	for _, s := range sinks {
		receiveError(ctx, s, ev)
	}
}

// receiveError hands ev to s and drops a panic of s.
func receiveError(ctx context.Context, s ErrorSink, ev kernel.ErrorEvent) {
	defer func() { _ = recover() }()
	s.ReceiveError(ctx, ev)
}

// fill sets what the caller of Report leaves to the reporter.
func (r *ErrorReporter) fill(ctx context.Context, ev *kernel.ErrorEvent) {
	if ev.Time.IsZero() && r.clock != nil {
		ev.Time = r.clock.Now()
	}
	if ev.Source == "" {
		ev.Source = kernel.ErrorSourceServer
	}
	if ev.Version == "" {
		ev.Version = r.version
	}
	if ev.Role == "" {
		ev.Role = r.role
	}
	if ev.Environment == "" {
		ev.Environment = r.environment
	}
	if ev.Where == "" {
		ev.Where = requestWhere(ctx)
	}
	if ev.RequestID == "" {
		ev.RequestID = RequestID(ctx)
	}
	if ev.TraceID == "" {
		ev.TraceID, ev.SpanID = r.tracer.LogIDs(ctx)
	}
}

// clientGone reports whether ev is the error of an HTTP request whose client has gone away: the
// request's context ended, so its error is most likely only that, and nothing here is broken.
// A panic is reported whatever the context.
func clientGone(ctx context.Context, ev kernel.ErrorEvent) bool {
	if ev.Kind != kernel.ErrorKindError || ctx.Err() == nil {
		return false
	}
	_, ok := ctx.Value(requestRouteKey{}).(*requestRoute)
	return ok
}

// requestWhere returns the place of an error of the HTTP request of ctx: "<my.surface> <method>
// <http.route>" once the request has reached a route, "http" before, and "" outside a request.
func requestWhere(ctx context.Context) string {
	rr, ok := ctx.Value(requestRouteKey{}).(*requestRoute)
	switch {
	case !ok:
		return ""
	case rr.route == "":
		return "http"
	}
	return rr.surface + " " + rr.method + " " + rr.route
}

// Recover reports a panic as a kernel.ErrorKindPanic event at where and does not panic again;
// http.ErrAbortHandler passes through. Call it with defer.
func (r *ErrorReporter) Recover(ctx context.Context, where string) {
	kernel.RecoverPanic(ctx, r, recover(), where)
}

// StopProcess starts the shutdown of the running Serve with cause and reports whether one runs.
func (r *ErrorReporter) StopProcess(cause error) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	stop := r.stop
	r.mu.RUnlock()
	if stop == nil {
		return false
	}
	stop(cause)
	return true
}

// setStop makes StopProcess call stop until the returned function restores the previous one.
func (r *ErrorReporter) setStop(stop func(cause error)) (restore func()) {
	if r == nil {
		return func() {}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	prev := r.stop
	r.stop = stop
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.stop = prev
	}
}

// close flushes the sinks that hold something, the last added first, and removes every sink.
func (r *ErrorReporter) close(ctx context.Context) {
	r.mu.Lock()
	sinks := r.sinks
	r.sinks = nil
	r.mu.Unlock()
	for _, s := range slices.Backward(sinks) {
		if c, ok := s.(ErrorSinkCloser); ok {
			_ = c.Close(ctx) // what a sink cannot flush in time is lost, as the ADR accepts
		}
	}
}

// setUpErrorReporter builds the App's ErrorReporter and adds the log line (ADR-0103 §4 (1)) and
// the exporter of errors_exporter (§4 (3)) behind the limits of the exports (exportLimiter). The other sinks are added by the setup of their modules.
func (a *App) setUpErrorReporter(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	a.ErrorReporter = NewErrorReporter(opts.Version, string(cfg.Role), cfg.ErrorsEnvironment, a.Clock, a.Tracer)
	a.ErrorReporter.reported = kernel.MeterOrNop(a.Meter).Counter(ErrorsReported)
	a.ErrorReporter.AddSink(NewErrorLogSink(a.Logger))
	if cfg.ErrorsExporter == "" || cfg.ErrorsExporter == "none" {
		return nil
	}
	build, ok := errorExporters[cfg.ErrorsExporter]
	if !ok {
		return fmt.Errorf("errors_exporter %q is not implemented yet", cfg.ErrorsExporter)
	}
	sink, err := build(ctx, a, cfg, opts)
	if err != nil {
		return fmt.Errorf("errors_exporter %s: %w", cfg.ErrorsExporter, err)
	}
	a.ErrorReporter.AddSink(newExportLimiter(sink, a.Clock))
	return nil
}

// closeErrorReporter flushes the sinks of the ErrorReporter while the database is still open.
func (a *App) closeErrorReporter() {
	if a.ErrorReporter == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), errorReporterCloseTimeout)
	defer cancel()
	a.ErrorReporter.close(ctx)
}
