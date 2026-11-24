// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
)

// unreadableRobots is a robots.txt that cannot be read, and what the reason of it reads.
type unreadableRobots struct {
	name     string
	response robotsResponse
	reason   string
}

// unreadableRobotsCases are the answers of a robots.txt that the issue names: a 5xx, a 429, and a
// timeout of the request while the job's own context is still live.
func unreadableRobotsCases() []unreadableRobots {
	timeout := &url.Error{Op: "Get", URL: robotsFile, Err: fmt.Errorf("read: %w", context.DeadlineExceeded)}
	return []unreadableRobots{
		{"500", robotsResponse{status: http.StatusInternalServerError, contentType: "text/plain"}, "status 500"},
		{"429", robotsResponse{status: http.StatusTooManyRequests, contentType: "text/plain"}, "status 429"},
		{"a timeout", robotsResponse{err: timeout}, "fetch: " + timeout.Error()},
	}
}

// fetchJob returns the handler of JobFetchLinkCard and its retry policy, and the job of a status
// that links robotsPage.
func (e *robotsCardsEnv) fetchJob() (queue.Handler, queue.RetryPolicy, queue.Job) {
	e.t.Helper()
	reg := &queue.JobHandlerRegistry{}
	e.cards.RegisterJobs(reg, queue.NewScheduler(nil, e.clock, slog.New(slog.DiscardHandler)))
	h, policy, ok := reg.Lookup(content.JobFetchLinkCard)
	if !ok {
		e.t.Fatalf("no handler of %s", content.JobFetchLinkCard)
	}
	const id = "status:unreadable"
	e.repo.setLink(id, robotsPage)
	payload, err := json.Marshal(map[string]string{"status_id": id})
	if err != nil {
		e.t.Fatal(err)
	}
	return h, policy, queue.Job{ID: id, Type: content.JobFetchLinkCard, Payload: payload, RunAt: e.clock.Now()}
}

// TestLinkCardRobotsUnreadableReason checks that the error of a fetch whose robots.txt cannot be
// read asks for a retry and says why: the status, or the error of the request.
func TestLinkCardRobotsUnreadableReason(t *testing.T) {
	t.Parallel()
	for _, tc := range unreadableRobotsCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newRobotsCardsEnv(t)
			e.page(robotsPage, "Page", "")
			e.web.pages[robotsFile] = tc.response
			e.repo.setLink("status:a", robotsPage)
			_, err := e.cards.Fetch(t.Context(), "status:a")
			if !errors.Is(err, content.ErrLinkCardRetryLater) {
				t.Fatalf("Fetch = %v, want ErrLinkCardRetryLater", err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("Fetch = %q, without the reason %q", err, tc.reason)
			}
			if e.web.count(robotsPage) != 0 {
				t.Error("requested the page")
			}
		})
	}
}

// TestLinkCardRobotsUnreadableAttempts checks the job's outcome at each run: a retry after
// LinkCardRobotsUnreachableTTL that names the reason, until the last run, which ends the job
// without a card and keeps a card that exists.
func TestLinkCardRobotsUnreadableAttempts(t *testing.T) {
	t.Parallel()
	for _, tc := range unreadableRobotsCases() {
		for _, stale := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, a card before: %v", tc.name, stale), func(t *testing.T) {
				t.Parallel()
				e := newRobotsCardsEnv(t)
				e.page(robotsPage, "New", "")
				e.web.pages[robotsFile] = tc.response
				if stale {
					e.stale(robotsPage)
				}
				h, policy, job := e.fetchJob()
				if policy.MaxAttempts != content.LinkCardFetchAttempts {
					t.Fatalf("MaxAttempts = %d, want %d", policy.MaxAttempts, content.LinkCardFetchAttempts)
				}
				for n := 1; n < content.LinkCardFetchAttempts; n++ {
					job.Attempt = n
					r := h(t.Context(), job)
					if !r.IsRetry() || !r.IsRemote() || r.After() != content.LinkCardRobotsUnreachableTTL {
						t.Fatalf("attempt %d: retry %v, remote %v, after %v; want a remote retry after %v",
							n, r.IsRetry(), r.IsRemote(), r.After(), content.LinkCardRobotsUnreachableTTL)
					}
					if r.Err() == nil || !strings.Contains(r.Err().Error(), tc.reason) {
						t.Errorf("attempt %d: error %v, without the reason %q", n, r.Err(), tc.reason)
					}
					e.clock.Advance(r.After())
				}
				job.Attempt = content.LinkCardFetchAttempts
				if r := h(t.Context(), job); !r.IsDone() {
					t.Fatalf("last attempt: done %v, retry %v, fail %v, error %v; want done",
						r.IsDone(), r.IsRetry(), r.IsFail(), r.Err())
				}
				c, ok := e.repo.card(robotsPage)
				if stale && (!ok || c.Title != "Kept") {
					t.Errorf("card = %+v, %v; want the card that was there", c, ok)
				}
				if !stale && ok {
					t.Errorf("card = %+v; want none", c)
				}
				if e.web.count(robotsPage) != 0 {
					t.Error("requested the page")
				}
				e.wantNoFailureMemory()
			})
		}
	}
}

// recordingSource is a burstSource that keeps the errors that the runner gives it.
type recordingSource struct {
	*burstSource
	released []string
	buried   []string
}

func (s *recordingSource) Release(ctx context.Context, w string, j queue.Job, attempt int, runAt time.Time, lastErr string) error {
	s.released = append(s.released, lastErr)
	return s.burstSource.Release(ctx, w, j, attempt, runAt, lastErr)
}

func (s *recordingSource) Bury(ctx context.Context, w string, j queue.Job, lastErr string, failedAt time.Time) error {
	s.buried = append(s.buried, lastErr)
	return s.burstSource.Bury(ctx, w, j, lastErr, failedAt)
}

// runUnreadable runs the job of robotsPage through a runner until it ends, moving the clock to
// each next run.
func (e *robotsCardsEnv) runUnreadable() *recordingSource {
	e.t.Helper()
	_, _, job := e.fetchJob()
	src := &recordingSource{burstSource: &burstSource{jobs: []*queue.Job{&job}}}
	reg := &queue.JobHandlerRegistry{}
	e.cards.RegisterJobs(reg, queue.NewScheduler(nil, e.clock, slog.New(slog.DiscardHandler)))
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: e.clock, Logger: slog.New(slog.DiscardHandler), Errors: &reporter{}})
	for runs := 0; len(src.jobs) > 0; {
		n, err := r.Drain(e.t.Context())
		if err != nil {
			e.t.Fatal(err)
		}
		runs += n
		if len(src.jobs) == 0 {
			break
		}
		e.clock.Advance(max(src.jobs[0].RunAt.Sub(e.clock.Now()), 0))
		if runs > 10*content.LinkCardFetchAttempts {
			e.t.Fatalf("%d runs and the job still waits", runs)
		}
	}
	return src
}

// TestLinkCardRobotsUnreadableEndsTheJob checks a job whose robots.txt stays unreadable, through
// a runner: it spends its attempts and is completed, not buried, and the status has no card.
func TestLinkCardRobotsUnreadableEndsTheJob(t *testing.T) {
	t.Parallel()
	for _, tc := range unreadableRobotsCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newRobotsCardsEnv(t)
			e.page(robotsPage, "Page", "")
			e.web.pages[robotsFile] = tc.response
			src := e.runUnreadable()
			if len(src.dead) != 0 || src.done != 1 {
				t.Errorf("dead %d %q, done %d; want one completed job", len(src.dead), src.buried, src.done)
			}
			if src.maxAttempt != content.LinkCardFetchAttempts {
				t.Errorf("highest attempt %d, want %d", src.maxAttempt, content.LinkCardFetchAttempts)
			}
			for _, msg := range append(src.released, src.buried...) {
				if strings.Contains(msg, "gave up after") {
					t.Errorf("error %q", msg)
				}
			}
			for _, msg := range src.released {
				if msg != "" && !strings.Contains(msg, tc.reason) {
					t.Errorf("error %q, without the reason %q", msg, tc.reason)
				}
			}
			if _, ok := e.repo.card(robotsPage); ok {
				t.Error("the status has a card")
			}
			if e.web.count(robotsPage) != 0 {
				t.Error("requested the page")
			}
		})
	}
}
