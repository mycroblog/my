// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
)

// ErrorReporter receives the errors worth an operator's attention (ADR-0103 §1, §2): the panics
// of the HTTP handlers, the jobs, and the goroutines, the errors behind a 5xx or a dead job, the
// startup failures, and the uncaught errors of the web client. Each is one ErrorEvent with its
// stack and the place it happened.
//
// The implementation fills the version, role, environment, and correlation IDs that the caller
// leaves empty, scrubs the event, and hands it to its outputs (the log line, the error groups of
// the database, and the configured exporter). Report never fails and never reports its own
// failures.
//
// Recover is called with defer at a boundary (an HTTP handler, a job, a goroutine): it recovers
// a panic, reports it as a KindPanic event at where, and does not panic again, except that
// http.ErrAbortHandler passes through unreported, as net/http expects. An implementation's
// Recover must call recover itself (Go recovers only in the deferred function); RecoverPanic
// does the rest.
//
// The implementation lives in internal/platform; NopErrorReporter discards everything.
type ErrorReporter interface {
	Report(ctx context.Context, ev ErrorEvent)
	Recover(ctx context.Context, where string)
}

// ErrorSource is where an ErrorEvent comes from.
type ErrorSource string

// Error sources.
const (
	ErrorSourceServer ErrorSource = "server"
	ErrorSourceWeb    ErrorSource = "web"
)

// ErrorKind tells a panic from an error.
type ErrorKind string

// Error kinds.
const (
	ErrorKindPanic ErrorKind = "panic"
	ErrorKindError ErrorKind = "error"
)

// ErrorEvent is one reported error (ADR-0103 §1). It holds standard-library types only, so that
// every module and adapter can build one.
type ErrorEvent struct {
	// Time is when it happened; the reporter sets it when zero.
	Time   time.Time
	Source ErrorSource
	Kind   ErrorKind
	// Type is the chain of Go type names that errors.Unwrap walks, joined by " > ", such as
	// "*fmt.wrapError > *pgconn.PgError", or the type of the value of a panic.
	Type string
	// Message is the error's text. The reporter scrubs it before any output (ADR-0103 §3).
	Message string
	// Stack is the debug.Stack() form of the stack: of the panic, of the WithStack that wrapped
	// the error, or of the place it was reported.
	Stack string
	// Where is the finite name of the place: "<my.surface> <http.route>" for HTTP, the job type
	// for a job, the name given to Go for a goroutine.
	Where string
	// Fatal marks an error that stops the process, such as the panic of a goroutine.
	Fatal bool
	// Unauthenticated marks a web event that came without an access token (ADR-0103 §6): it
	// is counted and logged, but never notifies the administrators of a new group.
	Unauthenticated bool
	// Fingerprint groups the events of the same error. The reporter computes it when it is
	// empty; the client API sets the one of a web event, which its stack cannot give.
	Fingerprint string

	// Correlation with the logs and the traces (ADR-0063 §6); the reporter fills RequestID,
	// TraceID, and SpanID from ctx when they are empty.
	RequestID string
	JobID     string
	Attempt   int
	TraceID   string
	SpanID    string

	// Version (service.version), Role (my.role), and Environment (errors_environment); the
	// reporter sets them.
	Version     string
	Role        string
	Environment string
}

// NewErrorEvent returns the KindError event of err at where. Its stack is the one that WithStack
// recorded in err's chain, or the caller's.
func NewErrorEvent(err error, where string) ErrorEvent {
	ev := ErrorEvent{Source: ErrorSourceServer, Kind: ErrorKindError, Where: where}
	if err == nil {
		return ev
	}
	ev.Type, ev.Message = ErrorTypes(err), err.Error()
	if s, ok := errors.AsType[*stackError](err); ok {
		ev.Stack = string(s.stack)
	} else {
		ev.Stack = string(debug.Stack())
	}
	return ev
}

// PanicEvent returns the KindPanic event of the panic value p, with stack (debug.Stack() taken
// in the deferred function), at where.
func PanicEvent(p any, stack []byte, where string) ErrorEvent {
	ev := ErrorEvent{Source: ErrorSourceServer, Kind: ErrorKindPanic, Where: where, Stack: string(stack)}
	if err, ok := p.(error); ok {
		ev.Type, ev.Message = ErrorTypes(err), err.Error()
	} else {
		ev.Type, ev.Message = typeName(p), fmt.Sprint(p)
	}
	return ev
}

// RecoverPanic reports the recovered panic value p at where through r. A nil p does nothing, and
// http.ErrAbortHandler panics again, unreported. Call it from a deferred function with the value
// of recover:
//
//	defer func() {
//		if p := recover(); p != nil {
//			kernel.RecoverPanic(ctx, r, p, where)
//		}
//	}()
func RecoverPanic(ctx context.Context, r ErrorReporter, p any, where string) {
	if p == nil {
		return
	}
	if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		panic(p)
	}
	if r != nil {
		r.Report(ctx, PanicEvent(p, debug.Stack(), where))
	}
}

// ErrorTypes returns the Go type names of err's chain, outermost first, joined by " > ". The
// chain follows Unwrap() error; an error that wraps several errors (errors.Join) ends it. The
// wrapper of WithStack is left out.
func ErrorTypes(err error) string {
	var names []string
	for err != nil {
		if _, ok := err.(*stackError); !ok { //nolint:errorlint // one link of the chain at a time
			names = append(names, typeName(err))
		}
		err = errors.Unwrap(err)
	}
	return strings.Join(names, " > ")
}

// typeName returns the Go name of v's type, such as *fmt.wrapError.
func typeName(v any) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}

// WithStack returns err with the stack of the call, which NewErrorEvent then reports instead of
// the stack of the place that reports it. Use it at a boundary where the error loses its origin,
// such as the return of an adapter. It returns nil for nil and err itself when err's chain
// already has a stack; the error's text and errors.Is / errors.As are unchanged.
func WithStack(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*stackError](err); ok {
		return err
	}
	return &stackError{err: err, stack: debug.Stack()}
}

// stackError is the wrapper of WithStack.
type stackError struct {
	err   error
	stack []byte
}

func (e *stackError) Error() string { return e.err.Error() }
func (e *stackError) Unwrap() error { return e.err }

// ProcessStopper is implemented by an ErrorReporter that can stop the process gracefully, as the
// one of internal/platform does while the server runs. StopProcess starts the shutdown with
// cause and reports whether one was started; false means nothing is running that could stop.
type ProcessStopper interface {
	StopProcess(cause error) bool
}

// Go runs fn in a new goroutine named name, such as "stream.valkey.subscribe" (ADR-0103 §5). A
// panic of fn is reported through reporter as a fatal KindPanic event at name, and then stops
// the process: through reporter's StopProcess when it is a ProcessStopper that has a shutdown to
// start, and otherwise by panicking again, so that the process never runs on in a broken state.
// A nil reporter only panics again.
func Go(ctx context.Context, name string, reporter ErrorReporter, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if reporter != nil {
				ev := PanicEvent(p, debug.Stack(), name)
				ev.Fatal = true
				reporter.Report(ctx, ev)
				if s, ok := reporter.(ProcessStopper); ok && s.StopProcess(fmt.Errorf("goroutine %s panicked: %v", name, p)) {
					return
				}
			}
			panic(p)
		}()
		fn(ctx)
	}()
}

// NopErrorReporter discards every report; its Recover still recovers the panics (and lets
// http.ErrAbortHandler through). For tests and the tools that report nothing.
type NopErrorReporter struct{}

// Report does nothing.
func (NopErrorReporter) Report(context.Context, ErrorEvent) {}

// Recover recovers a panic and discards it.
func (n NopErrorReporter) Recover(ctx context.Context, where string) {
	RecoverPanic(ctx, n, recover(), where)
}
