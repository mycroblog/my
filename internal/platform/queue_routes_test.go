// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_cloudtasks

package platform_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	defaultTasksQueue = "projects/p/locations/asia-northeast1/queues/my-jobs"
	mailTasksQueue    = "projects/p/locations/asia-northeast1/queues/my-mail"
)

// routedWakers records the signals of each queue of the routes, by the queue's name.
type routedWakers struct {
	mu     sync.Mutex
	wakers map[string]*testkit.Waker
}

func (r *routedWakers) of(q string) *testkit.Waker {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.wakers[q]
	if !ok {
		w = &testkit.Waker{}
		r.wakers[q] = w
	}
	return w
}

// take returns the job IDs of the signals that the queue q got since the last take.
func (r *routedWakers) take(q string) []string {
	var ids []string
	for _, w := range r.of(q).Take() {
		ids = append(ids, w.JobID)
	}
	return ids
}

// routedTasksApp starts an app with queue = cloudtasks whose signals reach the wakers of r, with
// the given value of queue_cloudtasks_routes.
func routedTasksApp(t *testing.T, r *routedWakers, routes string, setup func(*platform.App)) *server {
	t.Helper()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_QUEUE": "cloudtasks",
		"MY_QUEUE_CLOUDTASKS_QUEUE":           defaultTasksQueue,
		"MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT": "tasks@p.iam.gserviceaccount.com",
		"MY_QUEUE_CLOUDTASKS_URL":             "https://jobs.example.test",
		"MY_QUEUE_CLOUDTASKS_ROUTES":          routes,
	})
	cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
	return startApp(t, cfg, platform.AppOptions{
		Waker:             r.of(defaultTasksQueue),
		PushAuthenticator: pushAuth{},
		RouteWaker:        func(q config.QueueRoute) queue.Waker { return r.of(q.Queue) },
	}, platform.ServeOptions{}, func(a *platform.App) {
		if setup != nil {
			setup(a)
		}
	})
}

func TestQueueRoutesSendSignalsToTheirQueue(t *testing.T) {
	r := &routedWakers{wakers: map[string]*testkit.Waker{}}
	s := routedTasksApp(t, r, "notification.*="+mailTasksQueue, nil)

	// The signal is the job's ID, on the queue of the job's type.
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "notification.push"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	routed := r.take(mailTasksQueue)
	if len(routed) != 1 || !kernel.ValidID(routed[0]) {
		t.Fatalf("the routed queue got %v, want a job ID", routed)
	}
	if got := r.take(defaultTasksQueue); len(got) != 0 {
		t.Errorf("the default queue got %v for a routed type", got)
	}

	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "media.process"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if got := r.take(defaultTasksQueue); len(got) != 1 {
		t.Errorf("the default queue got %v, want the signal of a type without a route", got)
	}
	if got := r.take(mailTasksQueue); len(got) != 0 {
		t.Errorf("the routed queue got %v for another type", got)
	}

	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(s.logs.String(), `"routes_hash"`) && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if logs := s.logs.String(); !strings.Contains(logs, mailTasksQueue) {
		t.Errorf("the startup log lacks the routes:\n%s", logs)
	}
}

// Which queue a signal came from does not matter to the receiver, so a misrouted signal, as from a
// process with other routes, still runs its job.
func TestQueueRoutesMisroutedSignalStillRuns(t *testing.T) {
	r := &routedWakers{wakers: map[string]*testkit.Waker{}}
	var ran sync.WaitGroup
	ran.Add(1)
	s := routedTasksApp(t, r, "notification.*="+mailTasksQueue, func(a *platform.App) {
		a.Handlers.Register("test.misrouted", func(context.Context, queue.Job) queue.Result {
			ran.Done()
			return queue.Done()
		})
	})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.misrouted"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	id := r.take(defaultTasksQueue)
	if len(id) != 1 {
		t.Fatalf("signals = %v", id)
	}
	if code := post(t, s.url+"/_internal/jobs", "good", id[0]); code != http.StatusNoContent {
		t.Fatalf("delivery = %d", code)
	}
	ran.Wait()
}

// A route that matches no job type stops the startup, whichever role the process has.
func TestQueueRoutesUnknownTypeFailsStartup(t *testing.T) {
	for _, role := range []string{"web", "worker", "all"} {
		cfg := loadConfig(t, map[string]string{
			"MY_ROLE": role, "MY_DATABASE": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_QUEUE": "cloudtasks",
			"MY_QUEUE_CLOUDTASKS_QUEUE":           defaultTasksQueue,
			"MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT": "tasks@p.iam.gserviceaccount.com",
			"MY_QUEUE_CLOUDTASKS_ROUTES":          "nonexistent.*=" + mailTasksQueue,
		})
		cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
		a, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Waker: &testkit.Waker{}, RouteWaker: func(config.QueueRoute) queue.Waker { return &testkit.Waker{} }})
		if err == nil {
			_ = a.Close()
			t.Fatalf("role %s: NewApp succeeded", role)
		}
		if !strings.Contains(err.Error(), "nonexistent.*") {
			t.Errorf("role %s: NewApp = %v, want it to name the pattern", role, err)
		}
	}
}
