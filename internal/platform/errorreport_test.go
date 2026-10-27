// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

// errorSink keeps the events it receives and whether it was closed.
type errorSink struct {
	mu     sync.Mutex
	events []kernel.ErrorEvent
	closed bool
}

func (s *errorSink) ReceiveError(_ context.Context, ev kernel.ErrorEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *errorSink) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *errorSink) got() ([]kernel.ErrorEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]kernel.ErrorEvent(nil), s.events...), s.closed
}

type panickingSink struct{}

func (panickingSink) ReceiveError(context.Context, kernel.ErrorEvent) { panic("sink broke") }

func TestErrorReporterFillsAndFansOut(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	r := platform.NewErrorReporter("1.2.3", "web", "staging", testkit.NewClock(now), nil)
	first, second := &errorSink{}, &errorSink{}
	r.AddSink(first)
	r.AddSink(panickingSink{})
	r.AddSink(second)

	r.Report(context.Background(), kernel.NewErrorEvent(errors.New("db down"), "job.type"))
	for _, s := range []*errorSink{first, second} {
		evs, _ := s.got()
		if len(evs) != 1 {
			t.Fatalf("sink got %d events, want 1 (a panicking sink must not stop the others)", len(evs))
		}
		ev := evs[0]
		if ev.Version != "1.2.3" || ev.Role != "web" || ev.Environment != "staging" || !ev.Time.Equal(now) || ev.Source != kernel.ErrorSourceServer {
			t.Errorf("event not filled: %+v", ev)
		}
		if ev.Message != "db down" || ev.Where != "job.type" || ev.Kind != kernel.ErrorKindError {
			t.Errorf("event changed: %+v", ev)
		}
	}

	// What the caller sets stays.
	r.Report(context.Background(), kernel.ErrorEvent{Source: kernel.ErrorSourceWeb, Version: "web-9", Time: now.Add(time.Hour)})
	evs, _ := first.got()
	if ev := evs[1]; ev.Source != kernel.ErrorSourceWeb || ev.Version != "web-9" || !ev.Time.Equal(now.Add(time.Hour)) {
		t.Errorf("event overwritten: %+v", ev)
	}
}

func TestErrorReporterRecover(t *testing.T) {
	r := platform.NewErrorReporter("dev", "all", "production", testkit.NewClock(time.Now()), nil)
	s := &errorSink{}
	r.AddSink(s)
	func() {
		defer r.Recover(context.Background(), "client_api GET /api/v1/x")
		panic("handler broke")
	}()
	evs, _ := s.got()
	if len(evs) != 1 || evs[0].Kind != kernel.ErrorKindPanic || evs[0].Message != "handler broke" || evs[0].Where != "client_api GET /api/v1/x" {
		t.Fatalf("events = %+v", evs)
	}
	// Without a running Serve, StopProcess has nothing to stop.
	if r.StopProcess(errors.New("x")) {
		t.Error("StopProcess = true without Serve")
	}
}

func TestErrorExporterNotImplemented(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	cfg.ErrorsExporter = "sentry" // validation refuses it; NewApp must too
	if _, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{}); err == nil || !strings.Contains(err.Error(), "errors_exporter") {
		t.Fatalf("NewApp = %v, want the errors_exporter error", err)
	}
}

// TestServeStopsOnGoroutinePanic sees the panic of a goroutine of kernel.Go reported to the
// App's sinks, stop Serve with an error, and the sinks flushed when the App closes.
func TestServeStopsOnGoroutinePanic(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline", "MY_ERRORS_ENVIRONMENT": "staging"})
	sink := &errorSink{}
	s := start(t, cfg, platform.ServeOptions{ShutdownTimeout: 2 * time.Second}, func(a *platform.App) {
		a.ErrorReporter.AddSink(sink)
	})
	waitServing(t, s)

	kernel.Go(context.Background(), "test.loop", s.app.ErrorReporter, func(context.Context) { panic("loop broke") })
	select {
	case err := <-s.done:
		s.done <- err // for the cleanup of start
		if err == nil || !strings.Contains(err.Error(), "test.loop") {
			t.Errorf("Serve = %v, want the goroutine's panic", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop")
	}
	evs, _ := sink.got()
	if len(evs) != 1 || evs[0].Where != "test.loop" || !evs[0].Fatal || evs[0].Environment != "staging" || evs[0].Role != string(config.RoleAll) {
		t.Errorf("events = %+v", evs)
	}
	if err := s.app.Close(); err != nil {
		t.Fatal(err)
	}
	if _, closed := sink.got(); !closed {
		t.Error("App.Close did not close the sink")
	}
}

// waitServing waits until Serve answers the health check, so that its shutdown hook is set.
func waitServing(t *testing.T, s *server) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.logs.String(), `"msg":"started"`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Serve did not start")
}
