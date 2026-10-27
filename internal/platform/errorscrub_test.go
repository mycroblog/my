// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// stackSample is the debug.Stack() of a panic recovered by the ErrorReporter, as Go 1.26 prints
// it: the frames of the reporter, of debug.Stack, and of panic, the arguments of the
// function lines, an inlined frame, and a created by line.
const stackSample = `goroutine 37 [running]:
runtime/debug.Stack()
	/usr/local/go/src/runtime/debug/stack.go:26 +0x5e
github.com/mycroblog/my/internal/app/kernel.RecoverPanic({0x1b4e6a0, 0xc0003a2a50}, {0x1b3f2c0, 0xc000118000}, {0x15a0e40, 0x1b2f7d0}, {0x0, 0x0})
	internal/app/kernel/errorreport.go:139 +0xa5
github.com/mycroblog/my/internal/platform.(*ErrorReporter).Recover(0xc000118000, {0x1b4e6a0, 0xc0003a2a50}, {0x0, 0x0})
	internal/platform/errorreport.go:155 +0x4a
panic({0x15a0e40?, 0x1b2f7d0?})
	/usr/local/go/src/runtime/panic.go:787 +0x132
github.com/mycroblog/my/internal/app/status.(*Service).Get(...)
	internal/app/status/get.go:42
github.com/mycroblog/my/internal/interface/clientapi.(*Server).GetStatus(0xc0001c6000, {0x1b4e6a0, 0xc0003a2a50}, {{0xc0002b4f60, 0x24}})
	internal/interface/clientapi/statuses.go:88 +0x2c5
net/http.HandlerFunc.ServeHTTP(0xc0002c8040?, {0x1b4d0f0?, 0xc0003be000?}, 0x0?)
	/usr/local/go/src/net/http/server.go:2294 +0x29
github.com/mycroblog/my/internal/platform.(*App).Handler.func1()
	internal/platform/http.go:70 +0x1c
created by net/http.(*Server).Serve in goroutine 1
	/usr/local/go/src/net/http/server.go:3454 +0x485
`

const stackScrubbed = `goroutine 37 [running]:
github.com/mycroblog/my/internal/app/status.(*Service).Get(...)
	internal/app/status/get.go:42
github.com/mycroblog/my/internal/interface/clientapi.(*Server).GetStatus(...)
	internal/interface/clientapi/statuses.go:88 +0x2c5
net/http.HandlerFunc.ServeHTTP(...)
	/usr/local/go/src/net/http/server.go:2294 +0x29
github.com/mycroblog/my/internal/platform.(*App).Handler.func1()
	internal/platform/http.go:70 +0x1c
created by net/http.(*Server).Serve in goroutine 1
	/usr/local/go/src/net/http/server.go:3454 +0x485
`

func TestScrubGoStack(t *testing.T) {
	t.Parallel()
	if got := scrubGoStack(stackSample); got != stackScrubbed {
		t.Errorf("scrubGoStack =\n%s\nwant\n%s", got, stackScrubbed)
	}
	if got := scrubGoStack(""); got != "" {
		t.Errorf("scrubGoStack(\"\") = %q", got)
	}
}

func TestScrubGoStackCut(t *testing.T) {
	t.Parallel()
	frame := "github.com/mycroblog/my/internal/app/x.f(0x1)\n\tinternal/app/x/x.go:1 +0x1\n"
	got := scrubGoStack("goroutine 1 [running]:\n" + strings.Repeat(frame, 1000))
	if len(got) > maxErrorStackBytes || !strings.HasSuffix(got, "\n") {
		t.Errorf("cut to %d bytes ending %q, want at most 16 KiB on a line boundary", len(got), got[len(got)-5:])
	}
}

// argumentList finds the hexadecimal arguments that a function line of a stack must not keep.
var argumentList = regexp.MustCompile(`(?m)^[^\t].*\((?:0x|\{0x)`)

// panicStack returns the stack that the ErrorReporter receives for a real panic.
func panicStack(t *testing.T) kernel.ErrorEvent {
	t.Helper()
	r := NewErrorReporter("dev", "all", "production", nil, nil)
	var got kernel.ErrorEvent
	r.AddSink(sinkFunc(func(_ context.Context, ev kernel.ErrorEvent) { got = ev }))
	func() {
		defer r.Recover(context.Background(), "test")
		panicWith(42, "user@example.com")
	}()
	return got
}

//go:noinline
func panicWith(n int, s string) {
	panic(errors.New(s + " " + strings.Repeat("x", n%5)))
}

type sinkFunc func(context.Context, kernel.ErrorEvent)

func (f sinkFunc) ReceiveError(ctx context.Context, ev kernel.ErrorEvent) { f(ctx, ev) }

func TestScrubRealStack(t *testing.T) {
	t.Parallel()
	ev := panicStack(t)
	if ev.Message != "<email> xx" {
		t.Errorf("message = %q", ev.Message)
	}
	if !strings.HasPrefix(ev.Stack, "goroutine ") {
		t.Errorf("the stack lost its goroutine header:\n%s", ev.Stack)
	}
	for _, gone := range []string{"runtime/debug.Stack", "\npanic(", "kernel.RecoverPanic", "(*ErrorReporter).Recover"} {
		if strings.Contains(ev.Stack, gone) {
			t.Errorf("the stack keeps %q:\n%s", gone, ev.Stack)
		}
	}
	if m := argumentList.FindString(ev.Stack); m != "" {
		t.Errorf("the stack keeps the arguments of %q:\n%s", m, ev.Stack)
	}
	if !strings.Contains(ev.Stack, "internal/platform.panicWith(...)") || !strings.Contains(ev.Stack, "errorscrub_test.go:") {
		t.Errorf("the stack lost the panicking frame or its position:\n%s", ev.Stack)
	}
	if !strings.HasPrefix(moduleFrames(ev.Stack, 1)[0], "github.com/mycroblog/my/internal/platform.panicStack") &&
		!strings.HasPrefix(moduleFrames(ev.Stack, 1)[0], "github.com/mycroblog/my/internal/platform.panicWith") {
		t.Errorf("first frame of this repository = %v", moduleFrames(ev.Stack, 5))
	}
	if ev.Fingerprint == "" || ev.Fingerprint != panicStack(t).Fingerprint {
		t.Errorf("fingerprint %q is not stable across runs", ev.Fingerprint)
	}
}

func TestScrubWebStack(t *testing.T) {
	t.Parallel()
	ev := kernel.ErrorEvent{Source: kernel.ErrorSourceWeb, Stack: "TypeError: x\n    at f (https://my.example/assets/index-abc.js:1:2)\n"}
	scrubEvent(&ev)
	if !strings.Contains(ev.Stack, "at f (https://my.example/assets/index-abc.js:1:2)") {
		t.Errorf("the web stack was rewritten as a Go stack: %q", ev.Stack)
	}
}

func TestFingerprint(t *testing.T) {
	t.Parallel()
	base := kernel.ErrorEvent{
		Source: kernel.ErrorSourceServer, Where: "client_api GET /api/v1/statuses/{id}",
		Type: "*fmt.wrapError > *pgconn.PgError", Message: "boom", Stack: stackScrubbed,
	}
	fp := fingerprint(base)
	if len(fp) != 32 || strings.Trim(fp, "0123456789abcdef") != "" {
		t.Fatalf("fingerprint %q is not 32 hex digits", fp)
	}
	same := base
	same.Type = "*errors.errorString > *pgconn.PgError" // only the last type counts
	same.Message = "other"                              // the stack has frames of this repository
	same.Stack = strings.ReplaceAll(stackScrubbed, "get.go:42", "get.go:97")
	if fingerprint(same) != fp {
		t.Error("the outer types, the message, or a line number changed the fingerprint")
	}
	for name, change := range map[string]func(*kernel.ErrorEvent){
		"where":  func(ev *kernel.ErrorEvent) { ev.Where = "admin GET /api/v1/admin/x" },
		"type":   func(ev *kernel.ErrorEvent) { ev.Type = "*net.OpError" },
		"source": func(ev *kernel.ErrorEvent) { ev.Source = kernel.ErrorSourceWeb },
		"stack":  func(ev *kernel.ErrorEvent) { ev.Stack = strings.ReplaceAll(stackScrubbed, "GetStatus", "GetAccount") },
	} {
		ev := base
		change(&ev)
		if fingerprint(ev) == fp {
			t.Errorf("a change of the %s keeps the fingerprint", name)
		}
	}
	// Without a frame of this repository, the scrubbed message counts.
	foreign := base
	foreign.Stack = "goroutine 1 [running]:\nnet/http.f()\n\tx.go:1 +0x1\n"
	other := foreign
	other.Message = "something else"
	if fingerprint(foreign) == fingerprint(other) {
		t.Error("the message does not count for a stack without frames of this repository")
	}
}

func TestExportLimiter(t *testing.T) {
	t.Parallel()
	clock := &stepClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	var n int
	sink := &countingSink{receive: func() { n++ }}
	l := newExportLimiter(sink, clock)
	ctx := context.Background()
	l.ReceiveError(ctx, kernel.ErrorEvent{Fingerprint: "a"})
	l.ReceiveError(ctx, kernel.ErrorEvent{Fingerprint: "a"})
	if n != 1 {
		t.Fatalf("exports of one fingerprint within a minute = %d, want 1", n)
	}
	clock.now = clock.now.Add(time.Minute)
	l.ReceiveError(ctx, kernel.ErrorEvent{Fingerprint: "a"})
	if n != 2 {
		t.Fatalf("export after a minute = %d, want 2", n)
	}
	for i := range 100 {
		l.ReceiveError(ctx, kernel.ErrorEvent{Fingerprint: strings.Repeat("b", i+1)})
	}
	if n != 2+exportsPerMinute-1 {
		t.Errorf("exports in a minute = %d, want %d", n-1, exportsPerMinute)
	}
	if want := 1 + 100 - (exportsPerMinute - 1); sink.dropped != want {
		t.Errorf("dropped = %d, want %d", sink.dropped, want)
	}
}

// countingSink counts the reports it receives and those dropped before it.
type countingSink struct {
	receive func()
	dropped int
}

func (s *countingSink) ReceiveError(context.Context, kernel.ErrorEvent) { s.receive() }
func (s *countingSink) CountDropped(context.Context)                    { s.dropped++ }

type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time { return c.now }

func TestDebugStackFormStillHolds(t *testing.T) {
	t.Parallel()
	// The scrubbing expects these forms of debug.Stack(); a Go release that changes them
	// fails here first.
	s := string(debug.Stack())
	if !strings.HasPrefix(s, "goroutine ") || !strings.Contains(s, "\nruntime/debug.Stack()\n\t") {
		t.Errorf("debug.Stack() changed its form:\n%s", s)
	}
}
