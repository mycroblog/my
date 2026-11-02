// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

const (
	hostA      = "https://a.example"
	hostB      = "https://b.example"
	hostAFirst = hostA + "/1"
	hostANext  = hostA + "/2"
	hostBPage  = hostB + "/1"
	hostBImage = hostB + "/i.png"
)

// try runs one Fetch of the status that links u and returns its postponement, nil when it ran.
func (e *robotsCardsEnv) try(u string) *content.LinkCardPostponed {
	e.t.Helper()
	id := "status:" + u
	e.repo.setLink(id, u)
	retry, err := e.cards.Fetch(e.t.Context(), id)
	var p *content.LinkCardPostponed
	if errors.As(err, &p) {
		return p
	}
	if err != nil || retry {
		e.t.Fatalf("Fetch(%s) = %v, %v", u, retry, err)
	}
	return nil
}

// wantPostponed checks that one Fetch of u is postponed by after, because of host.
func (e *robotsCardsEnv) wantPostponed(u, host string, after time.Duration) {
	e.t.Helper()
	p := e.try(u)
	if p == nil || p.Host != host || p.After != after {
		e.t.Fatalf("Fetch(%s) postponed = %+v, want %s for %v", u, p, host, after)
	}
}

func (e *robotsCardsEnv) wantNoFailureMemory() {
	e.t.Helper()
	if ttl := e.kv.ttl(content.LinkCardFailureNamespace, ""); ttl != 0 {
		e.t.Errorf("failure memory with TTL %v", ttl)
	}
}

// throttle makes u answer with a status and, unless it is empty, a Retry-After.
func (e *robotsCardsEnv) throttle(u string, status int, retryAfter string) {
	var h http.Header
	if retryAfter != "" {
		h = http.Header{"Retry-After": {retryAfter}}
	}
	e.web.pages[u] = robotsResponse{status: status, contentType: "text/html", header: h}
}

// One fetch runs at a time per host, and another host does not wait for it.
func TestLinkCardHostOneFetchAtATime(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	const jobs = 8
	release := make(chan struct{})
	started := make(chan struct{}, jobs)
	for i := range jobs {
		u := fmt.Sprintf("%s/%d", hostA, i)
		e.page(u, "A", "")
		p := e.web.pages[u]
		p.before = func() {
			started <- struct{}{}
			<-release
		}
		e.web.pages[u] = p
		e.repo.setLink("status:"+u, u)
	}
	e.page(hostBPage, "B", "")
	results := make(chan error, jobs)
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Go(func() {
			u := fmt.Sprintf("%s/%d", hostA, i)
			_, err := e.cards.Fetch(context.Background(), "status:"+u)
			results <- err
		})
	}
	var postponed int
	for range jobs - 1 {
		var p *content.LinkCardPostponed
		if err := <-results; !errors.As(err, &p) || p.Host != "a.example" {
			t.Fatalf("Fetch = %v, want it postponed", err)
		}
		postponed++
	}
	<-started
	if e.try(hostBPage) != nil {
		t.Error("the fetch from another host was postponed")
	}
	if _, ok := e.repo.card(hostBPage); !ok {
		t.Error("no card from another host while the first host is busy")
	}
	close(release)
	wg.Wait()
	if err := <-results; err != nil {
		t.Errorf("the fetch that ran = %v", err)
	}
	requests := 0
	for i := range jobs {
		requests += e.web.count(fmt.Sprintf("%s/%d", hostA, i))
	}
	if requests != 1 || postponed != jobs-1 {
		t.Errorf("%d requests and %d postponed, want 1 and %d", requests, postponed, jobs-1)
	}
	e.wantNoFailureMemory()
}

// The next fetch from a host starts an interval after the end of the last one.
func TestLinkCardHostInterval(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(hostAFirst, "A", "")
	e.page(hostANext, "Next", "")
	e.page(hostBPage, "B", "")
	first := e.web.pages[hostAFirst]
	first.before = func() { e.clock.Advance(5 * time.Second) } // the time of the fetch does not count
	e.web.pages[hostAFirst] = first
	if e.try(hostAFirst) != nil {
		t.Fatal("the first fetch was postponed")
	}
	e.wantPostponed(hostANext, "a.example", content.LinkCardHostInterval)
	if e.try(hostBPage) != nil {
		t.Error("another host waited")
	}
	if got := e.web.count(hostANext); got != 0 {
		t.Errorf("requested the page %d times inside the interval", got)
	}
	e.clock.Advance(content.LinkCardHostInterval - time.Millisecond)
	if e.try(hostANext) == nil {
		t.Fatal("fetched before the interval ended")
	}
	e.clock.Advance(content.LinkCardHostInterval + time.Millisecond)
	if e.try(hostANext) != nil {
		t.Fatal("still postponed after the interval")
	}
	if c, ok := e.repo.card(hostANext); !ok || c.Title != "Next" {
		t.Errorf("card = %+v, %v", c, ok)
	}
	e.wantNoFailureMemory()
}

// Postponed jobs are told to come back at distinct times, so that they do not all wake at once.
func TestLinkCardHostSpreadsPostponedJobs(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(hostAFirst, "A", "")
	if e.try(hostAFirst) != nil {
		t.Fatal("postponed")
	}
	for i := range 3 {
		u := fmt.Sprintf("%s/n%d", hostA, i)
		e.wantPostponed(u, "a.example", time.Duration(i+1)*content.LinkCardHostInterval)
	}
}

// burstSource is a queue in memory that keeps what the burst tests look at.
type burstSource struct {
	queue.Source
	jobs       []*queue.Job
	done       int
	dead       []queue.Job
	retries    int
	maxAttempt int
	runs       int
}

func (s *burstSource) Claim(_ context.Context, _ string, now time.Time, _ time.Duration) (queue.Job, bool, error) {
	var next *queue.Job
	for _, j := range s.jobs {
		if !j.RunAt.After(now) && (next == nil || j.RunAt.Before(next.RunAt)) {
			next = j
		}
	}
	if next == nil {
		return queue.Job{}, false, nil
	}
	next.Attempt++
	s.maxAttempt = max(s.maxAttempt, next.Attempt)
	return *next, true, nil
}

func (s *burstSource) remove(id string) {
	s.jobs = slices.DeleteFunc(s.jobs, func(j *queue.Job) bool { return j.ID == id })
}

func (s *burstSource) Complete(_ context.Context, _ string, j queue.Job) error {
	s.remove(j.ID)
	s.done++
	return nil
}

func (s *burstSource) Release(_ context.Context, _ string, j queue.Job, attempt int, runAt time.Time, _ string) error {
	if attempt > 0 {
		s.retries++
	}
	for _, q := range s.jobs {
		if q.ID == j.ID {
			q.Attempt, q.RunAt = attempt, runAt
		}
	}
	return nil
}

func (s *burstSource) Bury(_ context.Context, _ string, j queue.Job, _ string, _ time.Time) error {
	s.remove(j.ID)
	s.dead = append(s.dead, j)
	return nil
}

// reporter counts the errors reported to the operator.
type reporter struct {
	kernel.ErrorReporter
	reports int
}

func (r *reporter) Report(context.Context, kernel.ErrorEvent) { r.reports++ }

// runBurst runs jobs of the URLs through the job's handler and a runner with the default retries,
// moving the clock to each next job, and returns the source and the times the pages were requested.
func (e *robotsCardsEnv) runBurst(urls []string) *burstSource {
	e.t.Helper()
	src := &burstSource{}
	for i, u := range urls {
		id := fmt.Sprintf("status:%d", i)
		e.repo.setLink(id, u)
		payload, err := json.Marshal(map[string]string{"status_id": id})
		if err != nil {
			e.t.Fatal(err)
		}
		src.jobs = append(src.jobs, &queue.Job{ID: id, Type: content.JobFetchLinkCard, Payload: payload, RunAt: e.clock.Now()})
	}
	reg := &queue.JobHandlerRegistry{}
	e.cards.RegisterJobs(reg, queue.NewScheduler(nil, e.clock, slog.New(slog.DiscardHandler)))
	errs := &reporter{}
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: e.clock, Logger: slog.New(slog.DiscardHandler), Errors: errs})
	runs := 0
	for len(src.jobs) > 0 {
		n, err := r.Drain(e.t.Context())
		if err != nil {
			e.t.Fatal(err)
		}
		runs += n
		if len(src.jobs) == 0 {
			break
		}
		next := src.jobs[0].RunAt
		for _, j := range src.jobs {
			if j.RunAt.Before(next) {
				next = j.RunAt
			}
		}
		e.clock.Advance(max(next.Sub(e.clock.Now()), 0))
		if runs > 100*len(urls) {
			e.t.Fatalf("%d runs for %d jobs and %d still waiting", runs, len(urls), len(src.jobs))
		}
	}
	if errs.reports != 0 {
		e.t.Errorf("%d errors reported to the operator", errs.reports)
	}
	src.runs = runs
	return src
}

// A burst far larger than the attempts of a job ends with every card and no dead job, one fetch
// per interval.
func TestLinkCardHostBurstBeyondTheAttempts(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	const jobs = 10 * queue.DefaultMaxAttempts * 3
	var times []time.Time
	urls := make([]string, 0, jobs)
	for i := range jobs {
		u := fmt.Sprintf("%s/%d", hostA, i)
		e.page(u, "A", "")
		p := e.web.pages[u]
		p.before = func() { times = append(times, e.clock.Now()) }
		e.web.pages[u] = p
		urls = append(urls, u)
	}
	src := e.runBurst(urls)
	if len(src.dead) != 0 || src.done != jobs || src.retries != 0 || src.maxAttempt != 1 {
		t.Errorf("dead %d, done %d, retries %d, highest attempt %d", len(src.dead), src.done, src.retries, src.maxAttempt)
	}
	if src.runs > 2*jobs+1 {
		t.Errorf("%d runs for %d jobs: the jobs do not wake one at a time", src.runs, jobs)
	}
	if len(times) != jobs {
		t.Fatalf("%d page requests, want %d", len(times), jobs)
	}
	for i := 1; i < len(times); i++ {
		if d := times[i].Sub(times[i-1]); d < content.LinkCardHostInterval {
			t.Fatalf("requests %d and %d are %v apart", i-1, i, d)
		}
	}
	for _, u := range urls {
		if _, ok := e.repo.card(u); !ok {
			t.Errorf("no card of %s", u)
		}
	}
	e.wantNoFailureMemory()
}

// A pause in the middle of a burst holds the rest of the host, and the paused job comes back.
func TestLinkCardHostPauseInABurst(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	var times []time.Time
	var urls []string
	for i := range 30 {
		u := fmt.Sprintf("%s/%d", hostA, i)
		e.page(u, "A", "")
		p := e.web.pages[u]
		p.before = func() { times = append(times, e.clock.Now()) }
		e.web.pages[u] = p
		urls = append(urls, u)
	}
	ok := e.web.pages[urls[3]]
	e.web.pages[urls[3]] = robotsResponse{
		status: http.StatusTooManyRequests, contentType: "text/html", header: http.Header{"Retry-After": {"120"}},
		before: func() {
			times = append(times, e.clock.Now())
			e.web.mu.Lock()
			defer e.web.mu.Unlock()
			e.web.pages[urls[3]] = ok
		},
	}
	src := e.runBurst(urls)
	if len(src.dead) != 0 || src.done != len(urls) || src.retries != 0 {
		t.Errorf("dead %d, done %d, retries %d", len(src.dead), src.done, src.retries)
	}
	if src.runs > 4*len(urls) {
		t.Errorf("%d runs for %d jobs", src.runs, len(urls))
	}
	if e.web.count(urls[3]) != 2 {
		t.Errorf("the paused URL was requested %d times, want 2", e.web.count(urls[3]))
	}
	longest := time.Duration(0)
	for i := 1; i < len(times); i++ {
		longest = max(longest, times[i].Sub(times[i-1]))
	}
	if longest < 120*time.Second {
		t.Errorf("the longest gap is %v, want the pause", longest)
	}
	if _, ok := e.repo.card(urls[3]); !ok {
		t.Error("the paused URL got no card")
	}
	e.wantNoFailureMemory()
}

// A 429 or a 503 pauses the host for its Retry-After, bounded, or for the default, and the URL is
// not remembered as failed, keeps its card, and is fetched when the pause ends.
func TestLinkCardRetryAfter(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name, value string
		want        time.Duration
	}{
		{"seconds", "120", 2 * time.Minute},
		{"spaces", " 90 ", 90 * time.Second},
		{"zero", "0", content.LinkCardHostInterval},
		{"a date", start.Add(5 * time.Minute).Format(http.TimeFormat), 5 * time.Minute},
		{"a date in the past", start.Add(-time.Hour).Format(http.TimeFormat), content.LinkCardHostInterval},
		{"a far date", start.Add(1000 * time.Hour).Format(http.TimeFormat), content.MaxLinkCardHostPause},
		{"too many seconds", "86400", content.MaxLinkCardHostPause},
		{"more seconds than a number holds", "99999999999999999999999", content.MaxLinkCardHostPause},
		{"negative", "-5", content.DefaultLinkCardHostPause},
		{"a fraction", "1.5", content.DefaultLinkCardHostPause},
		{"words", "soon", content.DefaultLinkCardHostPause},
		{"absent", "", content.DefaultLinkCardHostPause},
	}
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		for _, c := range cases {
			t.Run(fmt.Sprintf("%d %s", status, c.name), func(t *testing.T) {
				t.Parallel()
				e := newRobotsCardsEnv(t)
				e.stale(hostAFirst)
				e.throttle(hostAFirst, status, c.value)
				e.wantPostponed(hostAFirst, "a.example", c.want)
				if got := e.kv.ttl(content.LinkCardHostNotBeforeNamespace, "a.example"); got != c.want {
					t.Errorf("pause TTL = %v, want %v", got, c.want)
				}
				if got := e.meter.sum(fmt.Sprintf("%s http.response.status_code=%d", content.LinkCardHostPauses.Name, status)); got != 1 {
					t.Errorf("pauses counted = %v", got)
				}
				if card, ok := e.repo.card(hostAFirst); !ok || card.Title != "Kept" {
					t.Errorf("the card went: %+v, %v", card, ok)
				}
				e.wantNoFailureMemory()

				// Another URL of the host waits for the end of the pause, and requests nothing.
				e.page(hostANext, "Next", "")
				e.wantPostponed(hostANext, "a.example", c.want+content.LinkCardHostInterval)
				if e.web.count(hostANext) != 0 {
					t.Error("requested another URL of the paused host")
				}
				if e.try(hostBPage) != nil {
					t.Error("another host waited")
				}

				e.clock.Advance(c.want)
				e.page(hostAFirst, "Fresh", "")
				if e.try(hostAFirst) != nil {
					t.Fatal("still postponed after the pause")
				}
				if card, _ := e.repo.card(hostAFirst); card.Title != "Fresh" {
					t.Errorf("card after the pause = %+v", card)
				}
			})
		}
	}
	t.Run("another status does not pause", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.throttle(hostAFirst, http.StatusInternalServerError, "120")
		if e.try(hostAFirst) != nil {
			t.Fatal("postponed")
		}
		if e.kv.ttl(content.LinkCardFailureNamespace, "") != content.LinkCardFailureTTL {
			t.Error("the failure is not remembered")
		}
		if got := e.kv.ttl(content.LinkCardHostNotBeforeNamespace, "a.example"); got != content.LinkCardHostInterval {
			t.Errorf("host time TTL = %v, want the interval", got)
		}
	})
}

// The answer of a redirect's target pauses that host, and a redirect to a paused host requests
// nothing from it.
func TestLinkCardHostLimitsAtRedirects(t *testing.T) {
	t.Parallel()
	t.Run("the target answers 429", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.web.redirects[hostAFirst] = hostBPage
		e.throttle(hostBPage, http.StatusTooManyRequests, "30")
		e.wantPostponed(hostAFirst, "b.example", 30*time.Second)
		if got := e.kv.ttl(content.LinkCardHostNotBeforeNamespace, "b.example"); got != 30*time.Second {
			t.Errorf("pause of the target = %v", got)
		}
		if got := e.kv.ttl(content.LinkCardHostNotBeforeNamespace, "a.example"); got != content.LinkCardHostInterval {
			t.Errorf("time of the first host = %v, want the interval", got)
		}
		e.wantNoFailureMemory()
	})
	t.Run("a redirect to a paused host", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.throttle(hostBPage, http.StatusServiceUnavailable, "30")
		e.wantPostponed(hostBPage, "b.example", 30*time.Second)
		const target = hostB + "/2"
		e.page(target, "Target", "")
		e.web.redirects[hostAFirst] = target
		e.wantPostponed(hostAFirst, "b.example", 31*time.Second)
		if e.web.count(target) != 0 {
			t.Error("requested the paused host")
		}
		if e.kv.ttl(content.LinkCardRobotsNamespace, "https://b.example") == content.LinkCardRobotsUnreachableTTL {
			t.Error("the postponement changed what is known of robots.txt")
		}
		e.wantNoFailureMemory()
		e.clock.Advance(31 * time.Second)
		if e.try(hostAFirst) != nil {
			t.Fatal("postponed after the pause")
		}
		if c, ok := e.repo.card(hostAFirst); !ok || c.Title != "Target" {
			t.Errorf("card = %+v, %v", c, ok)
		}
	})
	t.Run("a redirect within the host is one fetch", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.web.redirects[hostAFirst] = hostANext
		e.page(hostANext, "Moved", "")
		if e.try(hostAFirst) != nil {
			t.Fatal("postponed")
		}
		if _, ok := e.repo.card(hostAFirst); !ok {
			t.Error("no card")
		}
	})
	t.Run("a refused host is refused, however busy", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.throttle(hostBPage, http.StatusTooManyRequests, "30")
		e.wantPostponed(hostBPage, "b.example", 30*time.Second)
		e.domain.refused["b.example"] = true
		e.web.redirects[hostAFirst] = hostB + "/2"
		if e.try(hostAFirst) != nil {
			t.Error("a redirect to a refused host was postponed")
		}
		before := e.web.count(hostBPage)
		if e.try(hostBPage) != nil {
			t.Error("a refused host was postponed")
		}
		if e.web.count(hostBPage) != before {
			t.Error("requested a refused host")
		}
		e.wantNoFailureMemory()
	})
}

// An image waits for its host as a page does, and its page is not requested again.
func TestLinkCardHostLimitsAtTheImage(t *testing.T) {
	t.Parallel()
	t.Run("another host answers 429", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(hostAFirst, "A", hostBImage)
		e.throttle(hostBImage, http.StatusTooManyRequests, "45")
		e.wantPostponed(hostAFirst, "b.example", 45*time.Second)
		if _, ok := e.repo.card(hostAFirst); ok {
			t.Error("the card did not wait for the image")
		}
		if got := e.kv.ttl(content.LinkCardHostNotBeforeNamespace, "b.example"); got != 45*time.Second {
			t.Errorf("pause of the image's host = %v", got)
		}
		e.wantNoFailureMemory()
		e.web.pages[hostBImage] = robotsResponse{status: http.StatusOK, contentType: "image/png", body: robotsPNG(t)}
		e.clock.Advance(45 * time.Second)
		if e.try(hostAFirst) != nil {
			t.Fatal("postponed after the pause")
		}
		if c, ok := e.repo.card(hostAFirst); !ok || c.ImageMediaID == "" || c.Title != "A" {
			t.Errorf("card = %+v, %v", c, ok)
		}
		if got := e.web.count(hostAFirst); got != 1 {
			t.Errorf("the page was requested %d times", got)
		}
		if e.kv.ttl(content.LinkCardPendingNamespace, "") != 0 {
			t.Error("the kept page was not dropped")
		}
	})
	t.Run("the host of the page", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		const image = hostA + "/i.png"
		e.page(hostAFirst, "A", image)
		e.web.pages[image] = robotsResponse{status: http.StatusOK, contentType: "image/png", body: robotsPNG(t)}
		e.wantPostponed(hostAFirst, "a.example", content.LinkCardHostInterval)
		if got := e.kv.ttl(content.LinkCardPendingNamespace, ""); got != content.LinkCardPendingTTL {
			t.Errorf("kept page TTL = %v", got)
		}
		e.clock.Advance(content.LinkCardHostInterval)
		if e.try(hostAFirst) != nil {
			t.Fatal("postponed again")
		}
		if c, ok := e.repo.card(hostAFirst); !ok || c.ImageMediaID == "" {
			t.Errorf("card = %+v, %v", c, ok)
		}
		if e.web.count(hostAFirst) != 1 || e.web.count(image) != 1 {
			t.Errorf("requests: page %d, image %d", e.web.count(hostAFirst), e.web.count(image))
		}
	})
	t.Run("an image that always answers 503", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(hostAFirst, "A", hostBImage)
		e.throttle(hostBImage, http.StatusServiceUnavailable, "")
		for range content.LinkCardThrottleTries - 1 {
			e.wantPostponed(hostAFirst, "b.example", content.DefaultLinkCardHostPause)
			e.clock.Advance(content.DefaultLinkCardHostPause)
		}
		if e.try(hostAFirst) != nil {
			t.Fatal("still waiting after the last try")
		}
		if c, ok := e.repo.card(hostAFirst); !ok || c.ImageMediaID != "" {
			t.Errorf("card = %+v, %v; want one without the image", c, ok)
		}
		if got := e.web.count(hostAFirst); got != 1 {
			t.Errorf("the page was requested %d times", got)
		}
		e.wantNoFailureMemory()
	})
}

// A page that always answers 429 ends like any other that makes no card, with no dead job.
func TestLinkCardHostThrottledPageEnds(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.throttle(hostAFirst, http.StatusTooManyRequests, "10")
	src := e.runBurst([]string{hostAFirst})
	if len(src.dead) != 0 || src.done != 1 || src.retries != 0 {
		t.Errorf("dead %d, done %d, retries %d", len(src.dead), src.done, src.retries)
	}
	if got := e.web.count(hostAFirst); got != content.LinkCardThrottleTries {
		t.Errorf("%d requests, want %d", got, content.LinkCardThrottleTries)
	}
	if e.kv.ttl(content.LinkCardFailureNamespace, "") != content.LinkCardFailureTTL {
		t.Error("the URL is not remembered as failed")
	}
	if e.kv.ttl(content.LinkCardThrottleNamespace, "") != 0 {
		t.Error("the count of the answers stayed")
	}
}

// A 429 or a 503 of a robots.txt keeps its row of the table of responses, and pauses the host.
func TestLinkCardHostPauseAtRobots(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(hostAFirst, "A", "")
	e.throttle(hostA+"/robots.txt", http.StatusTooManyRequests, "30")
	id := "status:" + hostAFirst
	e.repo.setLink(id, hostAFirst)
	if _, err := e.cards.Fetch(t.Context(), id); !errors.Is(err, content.ErrLinkCardRetryLater) {
		t.Fatalf("Fetch = %v", err)
	}
	if got := e.kv.ttl(content.LinkCardRobotsNamespace, hostA); got != content.LinkCardRobotsUnreachableTTL {
		t.Errorf("rules TTL = %v", got)
	}
	if got := e.kv.ttl(content.LinkCardHostNotBeforeNamespace, "a.example"); got != 30*time.Second {
		t.Errorf("pause = %v", got)
	}
	e.wantNoFailureMemory()
	if e.web.count(hostAFirst) != 0 {
		t.Error("requested the page")
	}

	// A file that its host's limits hold back is not requested, and nothing is known about it.
	f := newRobotsCardsEnv(t)
	f.page(hostAFirst, "A", "")
	f.page(hostANext, "Next", "")
	f.web.pages[hostA+"/robots.txt"] = robotsResponse{status: http.StatusOK, contentType: "text/plain"}
	if _, err := f.kv.CompareAndSwap(t.Context(), content.LinkCardHostLockNamespace, "a.example", nil, []byte("other"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if f.try(hostAFirst) == nil {
		t.Fatal("not postponed")
	}
	if f.web.count(hostA+"/robots.txt") != 0 || f.kv.ttl(content.LinkCardRobotsNamespace, hostA) != 0 {
		t.Error("touched the robots.txt of a busy host")
	}
}

// The setting about robots.txt changes nothing of the limits.
func TestLinkCardHostLimitsIgnoringRobots(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.ignore = true
	e.page(hostAFirst, "A", "")
	if e.try(hostAFirst) != nil {
		t.Fatal("postponed")
	}
	e.wantPostponed(hostANext, "a.example", content.LinkCardHostInterval)
}

// A worker that stops in the middle of a fetch leaves a lock that ends by itself, and every entry
// of the limits has a TTL.
func TestLinkCardHostCrashSafety(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(hostAFirst, "A", "")
	if _, err := e.kv.CompareAndSwap(t.Context(), content.LinkCardHostLockNamespace, "a.example", nil, []byte("gone"), content.LinkCardHostLockTTL); err != nil {
		t.Fatal(err)
	}
	if e.try(hostAFirst) == nil {
		t.Fatal("not postponed while another fetch holds the host")
	}
	e.clock.Advance(content.LinkCardHostLockTTL)
	if e.try(hostAFirst) != nil {
		t.Fatal("the lock of a stopped worker did not end")
	}
	e.throttle(hostANext, http.StatusTooManyRequests, "100000")
	e.clock.Advance(content.LinkCardHostInterval)
	if e.try(hostANext) == nil {
		t.Fatal("not postponed")
	}
	for _, ns := range []string{
		content.LinkCardHostLockNamespace, content.LinkCardHostNotBeforeNamespace, content.LinkCardHostQueueNamespace,
		content.LinkCardThrottleNamespace, content.LinkCardPendingNamespace,
	} {
		for name, entry := range e.kv.entries {
			if strings.HasPrefix(name, ns+"/") && entry.ttl <= 0 {
				t.Errorf("%s has no TTL", name)
			}
		}
	}
	e.clock.Advance(content.MaxLinkCardHostPause)
	e.page(hostANext, "Next", "")
	if e.try(hostANext) != nil {
		t.Error("the host is still paused after the longest pause")
	}
}

// A failure of the store is an error, and leaves the host free.
func TestLinkCardHostStoreFailure(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(hostAFirst, "A", "")
	e.repo.setLink("s", hostAFirst)
	e.kv.setGetErr(content.LinkCardHostNotBeforeNamespace, errors.New("store down"))
	var p *content.LinkCardPostponed
	if _, err := e.cards.Fetch(t.Context(), "s"); err == nil || errors.As(err, &p) {
		t.Fatalf("Fetch = %v", err)
	}
	if e.web.count(hostAFirst) != 0 {
		t.Error("requested the page")
	}
	e.kv.setGetErr(content.LinkCardHostNotBeforeNamespace, nil)
	if e.try(hostAFirst) != nil {
		t.Error("the failure left the host locked")
	}
}
