// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqs

package platform_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

func sqsConfig(t *testing.T) *config.Config {
	t.Helper()
	// queue = sqs needs a shared database; the test runs it on SQLite all the same.
	cfg := loadConfig(t, map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_QUEUE": "sqs",
		"MY_QUEUE_SQS_QUEUE_URL": "https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs",
	})
	cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
	return cfg
}

// TestSQSQueue checks the wiring of queue = sqs: an enqueued job is signaled, no runner polls
// the table, and the endpoints of Cloud Tasks are not served.
func TestSQSQueue(t *testing.T) {
	waker := &testkit.Waker{}
	var runs atomic.Int32
	s := startApp(t, sqsConfig(t), platform.AppOptions{Waker: waker}, platform.ServeOptions{},
		func(a *platform.App) {
			a.Handlers.Register("test.sqs", func(context.Context, queue.Job) queue.Result {
				runs.Add(1)
				return queue.Done()
			})
		})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.sqs"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if wakes := waker.Take(); len(wakes) != 1 || wakes[0].JobID == "" {
		t.Fatalf("signals = %+v, want one for the job", wakes)
	}
	// A pull runner would take the job within its poll interval.
	time.Sleep(queue.DefaultPollInterval + 500*time.Millisecond)
	if n := runs.Load(); n != 0 {
		t.Fatalf("the job ran %d times without a receiver", n)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/_internal/tick", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("tick = %d, want 404", resp.StatusCode)
	}
}
