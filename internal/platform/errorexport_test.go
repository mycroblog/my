// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// sender is an ErrorSender that answers with err and waits for release when it is set.
type sender struct {
	mu      sync.Mutex
	sent    []kernel.ErrorEvent
	err     func(ev kernel.ErrorEvent) error
	entered chan struct{}
	release chan struct{}
}

func (s *sender) SendError(ctx context.Context, ev kernel.ErrorEvent) error {
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	s.sent = append(s.sent, ev)
	s.mu.Unlock()
	if s.err != nil {
		return s.err(ev)
	}
	return nil
}

type droppedErr struct{}

func (droppedErr) Error() string { return "rate limited" }
func (droppedErr) Dropped() bool { return true }

func exported(m *testkit.Meter, outcome string) float64 {
	return m.Sum("my.errors.exported", map[string]string{"my.outcome": outcome})
}

func TestExportQueueOutcomes(t *testing.T) {
	m := testkit.NewMeter()
	s := &sender{err: func(ev kernel.ErrorEvent) error {
		switch ev.Where {
		case "dropped":
			return fmt.Errorf("wrapped: %w", droppedErr{})
		case "failed":
			return errors.New("receiver answered 500")
		case "panic":
			panic("sender broke")
		}
		return nil
	}}
	q := platform.NewExportQueue(s, m)
	for _, where := range []string{"ok", "dropped", "failed", "panic", "ok"} {
		q.ReceiveError(t.Context(), kernel.ErrorEvent{Where: where})
	}
	if err := q.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := []float64{exported(m, "success"), exported(m, "dropped"), exported(m, "failed")}; got[0] != 2 || got[1] != 1 || got[2] != 2 {
		t.Errorf("success, dropped, failed = %v, want [2 1 2]", got)
	}
	// After Close, reports are dropped.
	q.ReceiveError(t.Context(), kernel.ErrorEvent{Where: "late"})
	if got := exported(m, "dropped"); got != 2 {
		t.Errorf("dropped after Close = %v, want 2", got)
	}
	if err := m.Err(); err != nil {
		t.Error(err)
	}
}

func TestExportQueueDropsWhenFull(t *testing.T) {
	m := testkit.NewMeter()
	s := &sender{entered: make(chan struct{}, 1), release: make(chan struct{})}
	q := platform.NewExportQueue(s, m)
	q.ReceiveError(t.Context(), kernel.ErrorEvent{})
	<-s.entered // the first report is being sent; the queue is empty
	for range 1000 + 5 {
		q.ReceiveError(t.Context(), kernel.ErrorEvent{})
	}
	if got := exported(m, "dropped"); got != 5 {
		t.Errorf("dropped = %v, want 5 beyond the queue of 1,000", got)
	}
	close(s.release)
	if err := q.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := exported(m, "success"); got != 1001 {
		t.Errorf("success = %v, want 1001", got)
	}
}

func TestExportQueueDrainTimeout(t *testing.T) {
	m := testkit.NewMeter()
	s := &sender{release: make(chan struct{})} // never released: every send waits for its context
	q := platform.NewExportQueue(s, m)
	q.ReceiveError(t.Context(), kernel.ErrorEvent{})
	q.ReceiveError(t.Context(), kernel.ErrorEvent{})
	start := time.Now()
	if err := q.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 2*time.Second || d > 5*time.Second {
		t.Errorf("Close took %v, want about 2s", d)
	}
}

// TestSentryExporter wires errors_exporter = sentry: a report of the App reaches the intake of
// errors_sentry_dsn by the time the App closes.
func TestSentryExporter(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
		auth   string
	)
	intake := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, r.URL.Path+"\n"+string(b))
		auth = r.Header.Get("X-Sentry-Auth")
		mu.Unlock()
	}))
	defer intake.Close()

	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_ERRORS_EXPORTER": "sentry", "MY_ERRORS_ENVIRONMENT": "staging",
		"MY_ERRORS_SENTRY_DSN": strings.Replace(intake.URL, "http://", "http://key@", 1) + "/5",
	})
	a, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Version: "9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	a.ErrorReporter.Report(t.Context(), kernel.NewErrorEvent(errors.New("boom"), "job.type"))
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("intake got %d envelopes, want 1", len(bodies))
	}
	for _, want := range []string{"/api/5/envelope/\n", `"environment":"staging"`, `"release":"9.9.9"`, `"my.error.where":"job.type"`, `"value":"boom"`} {
		if !strings.Contains(bodies[0], want) {
			t.Errorf("envelope lacks %s:\n%s", want, bodies[0])
		}
	}
	if !strings.Contains(auth, "sentry_key=key") || !strings.Contains(auth, "sentry_client=my/9.9.9") {
		t.Errorf("X-Sentry-Auth = %q", auth)
	}
}

func TestSentryExporterBadDSN(t *testing.T) {
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_ERRORS_EXPORTER": "sentry", "MY_ERRORS_SENTRY_DSN": "https://o0.ingest.sentry.io/5",
	})
	_, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err == nil || !strings.Contains(err.Error(), "public key") {
		t.Fatalf("NewApp = %v, want the DSN error", err)
	}
}

// slowStop is a sender that needs a while to give up after its context ends.
type slowStop struct {
	finished chan struct{}
	entered  chan struct{}
}

func (s *slowStop) SendError(ctx context.Context, _ kernel.ErrorEvent) error {
	s.entered <- struct{}{}
	<-ctx.Done()
	time.Sleep(200 * time.Millisecond)
	close(s.finished)
	return ctx.Err()
}

func TestExportQueueCloseWaitsForTheSendInFlight(t *testing.T) {
	s := &slowStop{finished: make(chan struct{}), entered: make(chan struct{}, 1)}
	q := platform.NewExportQueue(s, testkit.NewMeter())
	q.ReceiveError(t.Context(), kernel.ErrorEvent{})
	<-s.entered
	if err := q.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.finished:
	default:
		t.Error("a send was still running when Close returned")
	}
}
