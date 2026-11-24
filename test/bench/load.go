// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// The reads of the load: the routes of a client's first screen, and the posts of someone followed.
var reads = []string{
	"GET /accounts/verify_credentials",
	"GET /instance",
	"GET /preferences",
	"GET /announcements",
	"GET /timelines/home",
	"GET /notifications",
	"GET /accounts/{id}/statuses",
}

// The writes of the load, in turn.
var writes = []string{"POST /statuses", "POST /statuses/{id}/favourite"}

// load is the sequence of requests of a run: request n of it is always the same request.
type load struct {
	api    *api
	world  *world
	writes int // per hundred
	// tag keeps the Idempotency-Keys of one sequence apart from another's.
	tag string
}

// route returns the route of request n.
func (l *load) route(n int) string {
	if n%100 < l.writes {
		return writes[(n/100+n)%len(writes)]
	}
	return reads[n%len(reads)]
}

// send sends request n and returns its route and status; an error is a request that got no answer.
func (l *load) send(ctx context.Context, n int) (string, int, error) {
	people := l.world.people
	p := people[n%len(people)]
	route := l.route(n)
	var (
		status int
		err    error
	)
	switch route {
	case "GET /accounts/{id}/statuses":
		status, _, err = l.api.do(ctx, p, "GET", "/accounts/"+people[(n+1)%len(people)].ID+"/statuses", nil, nil)
	case "POST /statuses":
		body := map[string]any{"text": fmt.Sprintf("bench load %d", n), "visibility": "public", "language": "en"}
		status, _, err = l.api.do(ctx, p, "POST", "/statuses", body, map[string]string{"Idempotency-Key": fmt.Sprintf("bench-%s-%d", l.tag, n)})
	case "POST /statuses/{id}/favourite":
		id := l.world.statuses[(n*7919)%len(l.world.statuses)]
		status, _, err = l.api.do(ctx, p, "POST", "/statuses/"+id+"/favourite", map[string]any{}, nil)
	default:
		status, _, err = l.api.do(ctx, p, "GET", route[len("GET "):], nil, nil)
	}
	return route, status, err
}

// sample is one request the load timed.
type sample struct {
	route  string
	status int
	took   time.Duration
}

// run sends requests from..from+count-1 of the sequence from clients at once, or, when until is
// not zero, keeps sending from `from` on until then. It returns what it timed and the number of
// the next request of the sequence that no run has sent.
func (l *load) run(ctx context.Context, clients, from, count int, until time.Time) ([]sample, int, error) {
	var next atomic.Int64
	next.Store(int64(from))
	var mu sync.Mutex
	var out []sample
	var failed atomic.Pointer[error]
	var wg sync.WaitGroup
	for range clients {
		wg.Go(func() {
			var mine []sample
			for ctx.Err() == nil && failed.Load() == nil {
				n := int(next.Add(1) - 1)
				if until.IsZero() && n >= from+count || !until.IsZero() && time.Now().After(until) {
					break
				}
				at := time.Now()
				route, status, err := l.send(ctx, n)
				if err != nil {
					err = fmt.Errorf("request %d (%s): %w", n, route, err)
					failed.CompareAndSwap(nil, &err)
					break
				}
				mine = append(mine, sample{route: route, status: status, took: time.Since(at)})
			}
			mu.Lock()
			out = append(out, mine...)
			mu.Unlock()
		})
	}
	wg.Wait()
	// Each client took one number past the last it sent.
	end := int(next.Load())
	if err := failed.Load(); err != nil {
		return out, end, *err
	}
	return out, end, ctx.Err()
}

// levelOf sums up the samples of a level that took wall.
func levelOf(clients int, samples []sample, wall time.Duration) Level {
	lv := Level{Clients: clients, Requests: len(samples), Seconds: wall.Seconds(), Statuses: map[int]int{}}
	if wall > 0 {
		lv.Throughput = float64(len(samples)) / wall.Seconds()
	}
	all := make([]time.Duration, 0, len(samples))
	byRoute := map[string][]time.Duration{}
	errs := map[string]int{}
	for _, s := range samples {
		all = append(all, s.took)
		byRoute[s.route] = append(byRoute[s.route], s.took)
		lv.Statuses[s.status]++
		if s.status/100 != 2 {
			errs[s.route]++
		}
	}
	lv.Latency = latencyOf(all)
	for _, route := range slices.Sorted(maps.Keys(byRoute)) {
		ds := byRoute[route]
		lv.Routes = append(lv.Routes, RouteLatency{Route: route, Count: len(ds), Errors: errs[route], Latency: latencyOf(ds)})
	}
	return lv
}

// failures returns the requests of lv whose answer was not 2xx.
func (lv Level) failures() int {
	n := 0
	for status, count := range lv.Statuses {
		if status/100 != 2 {
			n += count
		}
	}
	return n
}
