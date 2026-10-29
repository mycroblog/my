// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_cloudtasks

package platform_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

// pushAuth accepts the bearer token "good" for the endpoints of https://jobs.example.test.
type pushAuth struct{}

func (pushAuth) Authenticate(_ context.Context, token, endpoint string) error {
	if token != "good" || !strings.HasPrefix(endpoint, "https://jobs.example.test/_internal/") {
		return fmt.Errorf("%w: %s for %s", queue.ErrPushUnauthenticated, token, endpoint)
	}
	return nil
}

func cloudTasksConfig(t *testing.T, role string) *config.Config {
	t.Helper()
	// queue = cloudtasks needs a shared database; the test runs it on SQLite all the same.
	cfg := loadConfig(t, map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_ROLE": role, "MY_QUEUE": "cloudtasks",
		"MY_QUEUE_CLOUDTASKS_QUEUE":           "projects/p/locations/asia-northeast1/queues/my-jobs",
		"MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT": "tasks@p.iam.gserviceaccount.com",
		"MY_QUEUE_CLOUDTASKS_URL":             "https://jobs.example.test",
	})
	cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
	return cfg
}

func post(t *testing.T, url, token, body string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestCloudTasksQueue checks the wiring of queue = cloudtasks: an enqueued job is signaled, no
// runner polls the table, and the signal's delivery to /_internal/jobs runs it.
func TestCloudTasksQueue(t *testing.T) {
	waker := &testkit.Waker{}
	var runs atomic.Int32
	s := startApp(t, cloudTasksConfig(t, "all"), platform.AppOptions{Waker: waker, PushAuthenticator: pushAuth{}}, platform.ServeOptions{},
		func(a *platform.App) {
			a.Handlers.Register("test.cloudtasks", func(context.Context, queue.Job) queue.Result {
				runs.Add(1)
				return queue.Done()
			})
		})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.cloudtasks"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	wakes := waker.Take()
	if len(wakes) != 1 || wakes[0].JobID == "" {
		t.Fatalf("signals = %+v, want one for the job", wakes)
	}
	// A pull runner would take the job within its poll interval.
	time.Sleep(queue.DefaultPollInterval + 500*time.Millisecond)
	if n := runs.Load(); n != 0 {
		t.Fatalf("the job ran %d times before its signal arrived", n)
	}

	if code := post(t, s.url+"/_internal/jobs", "", wakes[0].JobID); code != http.StatusUnauthorized {
		t.Errorf("signal without a token = %d, want 401", code)
	}
	if code := post(t, s.url+"/_internal/jobs", "good", wakes[0].JobID); code != http.StatusNoContent {
		t.Errorf("signal = %d, want 204", code)
	}
	if n := runs.Load(); n != 1 {
		t.Errorf("runs after the signal = %d, want 1", n)
	}
	if code := post(t, s.url+"/_internal/jobs", "good", wakes[0].JobID); code != http.StatusNoContent {
		t.Errorf("duplicate signal = %d, want 204", code)
	}
	if n := runs.Load(); n != 1 {
		t.Errorf("runs after a duplicate signal = %d, want 1", n)
	}
	if code := post(t, s.url+"/_internal/tick", "good", ""); code != http.StatusNoContent {
		t.Errorf("tick = %d, want 204", code)
	}
}

// TestCloudTasksEndpointsByRole checks that the worker role serves the endpoints and the web role
// does not.
func TestCloudTasksEndpointsByRole(t *testing.T) {
	for role, want := range map[string]int{"worker": http.StatusNoContent, "web": http.StatusNotFound} {
		s := startApp(t, cloudTasksConfig(t, role), platform.AppOptions{Waker: &testkit.Waker{}, PushAuthenticator: pushAuth{}}, platform.ServeOptions{}, nil)
		if code := post(t, s.url+"/_internal/tick", "good", ""); code != want {
			t.Errorf("tick on role %s = %d, want %d", role, code, want)
		}
	}
}
