// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
)

// TestServeAbandonsAStuckJob sees Serve return within its shutdown timeout and job stop timeout
// while a job ignores the cancellation of its context: the job is abandoned, to run again once
// its lease expires.
func TestServeAbandonsAStuckJob(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	opts := platform.ServeOptions{ShutdownTimeout: 200 * time.Millisecond, JobStopTimeout: 200 * time.Millisecond}
	s := start(t, cfg, opts, func(a *platform.App) {
		a.Handlers.Register("test.stuck", func(context.Context, queue.Job) queue.Result {
			close(started)
			<-release // ignores the cancellation
			return queue.Done()
		})
	})
	waitServing(t, s)
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.stuck"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the job did not start")
	}
	begin := time.Now()
	s.stop()
	select {
	case err := <-s.done:
		s.done <- err // for the cleanup of start
		if err == nil || !strings.Contains(err.Error(), "shutdown timeout") {
			t.Errorf("Serve = %v, want the shutdown timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve waited for a job that ignores its cancellation")
	}
	if d := time.Since(begin); d > 3*time.Second {
		t.Errorf("Serve took %v to stop", d)
	}
	if !strings.Contains(s.logs.String(), "job runner abandoned") {
		t.Errorf("no log line of the abandoned job:\n%s", s.logs.String())
	}
}
