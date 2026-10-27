// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// recorder is an ErrorReporter that keeps the events; stopper adds a ProcessStopper.
type recorder struct {
	mu     sync.Mutex
	events []kernel.ErrorEvent
}

func (r *recorder) Report(_ context.Context, ev kernel.ErrorEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recorder) Recover(ctx context.Context, where string) {
	kernel.RecoverPanic(ctx, r, recover(), where)
}

func (r *recorder) got() []kernel.ErrorEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]kernel.ErrorEvent(nil), r.events...)
}

type stopper struct {
	recorder
	stopped chan error
}

func (s *stopper) StopProcess(cause error) bool {
	s.stopped <- cause
	return true
}

func panicking(r kernel.ErrorReporter, v any) {
	defer r.Recover(context.Background(), "test.where")
	panic(v)
}

func TestRecoverValue(t *testing.T) {
	r := &recorder{}
	panicking(r, "boom")
	evs := r.got()
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	ev := evs[0]
	if ev.Kind != kernel.ErrorKindPanic || ev.Source != kernel.ErrorSourceServer || ev.Where != "test.where" {
		t.Errorf("event = %+v", ev)
	}
	if ev.Type != "string" || ev.Message != "boom" {
		t.Errorf("Type, Message = %q, %q; want string, boom", ev.Type, ev.Message)
	}
	if !strings.Contains(ev.Stack, "kernel_test.panicking") {
		t.Errorf("stack misses the panicking function:\n%s", ev.Stack)
	}
}

func TestRecoverError(t *testing.T) {
	r := &recorder{}
	panicking(r, fmt.Errorf("open: %w", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}))
	evs := r.got()
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	if want := "*fmt.wrapError > *fs.PathError > *errors.errorString"; evs[0].Type != want {
		t.Errorf("Type = %q, want %q", evs[0].Type, want)
	}
	if evs[0].Message != "open: open x: file does not exist" {
		t.Errorf("Message = %q", evs[0].Message)
	}
}

func TestRecoverPassesAbortHandler(t *testing.T) {
	for _, r := range []kernel.ErrorReporter{&recorder{}, kernel.NopErrorReporter{}} {
		func() {
			defer func() {
				if err, _ := recover().(error); !errors.Is(err, http.ErrAbortHandler) {
					t.Errorf("%T: recovered %v, want http.ErrAbortHandler", r, err)
				}
			}()
			panicking(r, http.ErrAbortHandler)
		}()
		if rec, ok := r.(*recorder); ok && len(rec.got()) != 0 {
			t.Errorf("http.ErrAbortHandler was reported: %+v", rec.got())
		}
	}
}

func TestRecoverWithoutPanic(t *testing.T) {
	r := &recorder{}
	func() { defer r.Recover(context.Background(), "x") }()
	if len(r.got()) != 0 {
		t.Errorf("reported without a panic: %+v", r.got())
	}
}

func TestNopErrorReporterRecovers(_ *testing.T) {
	panicking(kernel.NopErrorReporter{}, "boom") // does not panic again
	kernel.NopErrorReporter{}.Report(context.Background(), kernel.ErrorEvent{})
}

//go:noinline
func makeStackErr() error { return kernel.WithStack(errSentinel) }

var errSentinel = errors.New("sentinel")

func TestWithStack(t *testing.T) {
	if kernel.WithStack(nil) != nil {
		t.Error("WithStack(nil) != nil")
	}
	err := fmt.Errorf("wrap: %w", makeStackErr())
	if !errors.Is(err, errSentinel) || err.Error() != "wrap: sentinel" {
		t.Errorf("WithStack changed the error: %v", err)
	}
	if again := kernel.WithStack(err); again != err { //nolint:errorlint // the identity: WithStack returns err itself
		t.Error("WithStack wrapped an error that already has a stack")
	}
	ev := kernel.NewErrorEvent(err, "job.type")
	if ev.Kind != kernel.ErrorKindError || ev.Where != "job.type" || ev.Message != "wrap: sentinel" {
		t.Errorf("event = %+v", ev)
	}
	if want := "*fmt.wrapError > *errors.errorString"; ev.Type != want {
		t.Errorf("Type = %q, want %q (without the stack wrapper)", ev.Type, want)
	}
	if !strings.Contains(ev.Stack, "kernel_test.makeStackErr") {
		t.Errorf("stack is not the one of WithStack:\n%s", ev.Stack)
	}
}

func TestNewErrorEventStackOfCaller(t *testing.T) {
	ev := kernel.NewErrorEvent(errSentinel, "x")
	if !strings.Contains(ev.Stack, "kernel_test.TestNewErrorEventStackOfCaller") {
		t.Errorf("stack is not the caller's:\n%s", ev.Stack)
	}
	if ev := kernel.NewErrorEvent(nil, "x"); ev.Type != "" || ev.Message != "" {
		t.Errorf("NewErrorEvent(nil) = %+v", ev)
	}
}

func TestErrorTypesJoin(t *testing.T) {
	if got, want := kernel.ErrorTypes(fmt.Errorf("a: %w", errors.Join(errSentinel))), "*fmt.wrapError > *errors.joinError"; got != want {
		t.Errorf("ErrorTypes = %q, want %q", got, want)
	}
}

func TestGoReportsAndStops(t *testing.T) {
	r := &stopper{stopped: make(chan error, 1)}
	kernel.Go(context.Background(), "test.loop", r, func(context.Context) { panic("loop broke") })
	select {
	case cause := <-r.stopped:
		if !strings.Contains(cause.Error(), "test.loop") {
			t.Errorf("cause = %v", cause)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StopProcess was not called")
	}
	evs := r.got()
	if len(evs) != 1 || evs[0].Where != "test.loop" || !evs[0].Fatal || evs[0].Kind != kernel.ErrorKindPanic || evs[0].Message != "loop broke" {
		t.Errorf("events = %+v", evs)
	}
}

func TestGoRunsFn(t *testing.T) {
	r := &recorder{}
	done := make(chan struct{})
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	kernel.Go(ctx, "test.ok", r, func(ctx context.Context) {
		if ctx.Value(ctxKey{}) != "v" {
			t.Error("fn did not get ctx")
		}
		close(done)
	})
	<-done
	if len(r.got()) != 0 {
		t.Errorf("reported without a panic: %+v", r.got())
	}
}

type ctxKey struct{}

// TestGoPanicsAgain runs the test binary again to see a goroutine's panic crash the process when
// the reporter cannot stop it.
func TestGoPanicsAgain(t *testing.T) {
	if os.Getenv("KERNEL_GO_CRASH") == "1" {
		kernel.Go(context.Background(), "test.crash", &recorder{}, func(context.Context) { panic("crash me") })
		time.Sleep(10 * time.Second)
		return
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestGoPanicsAgain$") //nolint:gosec // G204: the test binary itself
	cmd.Env = append(os.Environ(), "KERNEL_GO_CRASH=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("the process did not crash:\n%s", out)
	}
	if !strings.Contains(string(out), "panic: crash me") {
		t.Errorf("output misses the panic:\n%s", out)
	}
}
