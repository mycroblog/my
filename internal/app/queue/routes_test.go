// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestWakeRouterChoosesTheMostSpecificRoute(t *testing.T) {
	fallback, prefix, longer, exact := &testkit.Waker{}, &testkit.Waker{}, &testkit.Waker{}, &testkit.Waker{}
	router := queue.NewWakeRouter(fallback, []queue.WakeRoute{
		{Pattern: "federation.*", Waker: prefix},
		{Pattern: "federation.deliver", Waker: exact},
		{Pattern: "federation.inbox.*", Waker: longer},
	})
	wakes := []queue.Wake{
		{JobID: "1", Type: "federation.deliver"},
		{JobID: "2", Type: "federation.fanout"},
		{JobID: "3", Type: "federation.inbox.retry"},
		{JobID: "4", Type: "notification.push"},
		{JobID: "5"},
	}
	if err := router.Wake(t.Context(), wakes); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	for _, tc := range []struct {
		name  string
		waker *testkit.Waker
		want  []string
	}{
		{"the name wins over the prefix", exact, []string{"1"}},
		{"the prefix", prefix, []string{"2"}},
		{"the longer prefix", longer, []string{"3"}},
		{"no route and no type", fallback, []string{"4", "5"}},
	} {
		var got []string
		for _, w := range tc.waker.Take() {
			got = append(got, w.JobID)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: signals %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A signal whose queue fails stays on that queue: it is not sent to another, and the other routes
// still get theirs. The sweep sends it again.
func TestWakeRouterDoesNotOverflowToAnotherQueue(t *testing.T) {
	fallback, routed := &testkit.Waker{}, &testkit.Waker{}
	router := queue.NewWakeRouter(fallback, []queue.WakeRoute{{Pattern: "federation.*", Waker: routed}})
	down := errors.New("the queue is down")
	routed.Fail(down)
	err := router.Wake(t.Context(), []queue.Wake{{JobID: "1", Type: "federation.deliver"}, {JobID: "2", Type: "media.process"}})
	if !errors.Is(err, down) {
		t.Fatalf("Wake = %v, want the error of the route", err)
	}
	got := fallback.Take()
	if len(got) != 1 || got[0].JobID != "2" {
		t.Errorf("default queue got %v, want only its own signal", got)
	}
}

func TestWakeRouterTypesOfRoutes(t *testing.T) {
	router := queue.NewWakeRouter(&testkit.Waker{}, []queue.WakeRoute{
		{Pattern: "federation.*", Waker: &testkit.Waker{}},
		{Pattern: "federation.deliver", Waker: &testkit.Waker{}},
		{Pattern: "media.*", Waker: &testkit.Waker{}},
	})
	groups, err := router.TypesOfRoutes([]string{"media.process", "federation.inbox", "federation.deliver", "federation.fanout", "notification.push"})
	if err != nil {
		t.Fatalf("TypesOfRoutes: %v", err)
	}
	want := [][]string{{"federation.fanout", "federation.inbox"}, {"federation.deliver"}, {"media.process"}}
	if !slices.EqualFunc(groups, want, slices.Equal[[]string]) {
		t.Errorf("TypesOfRoutes = %v, want %v", groups, want)
	}
	_, err = router.TypesOfRoutes([]string{"federation.deliver"})
	if err == nil || !strings.Contains(err.Error(), "media.*") {
		t.Errorf("TypesOfRoutes without a match = %v, want it to name media.*", err)
	}
}

// routedFinder records the reads of a sweep with routes.
type routedFinder struct {
	of     map[string][]queue.Wake
	except []queue.Wake
	limits []int
	failOn string
	asked  []string
}

func (f *routedFinder) DueJobs(context.Context, time.Time, time.Time, int) ([]queue.Wake, error) {
	f.asked = append(f.asked, "all")
	return nil, nil
}

func (f *routedFinder) DueJobsOfTypes(_ context.Context, types []string, _, _ time.Time, limit int) ([]queue.Wake, error) {
	key := strings.Join(types, ",")
	f.asked = append(f.asked, key)
	f.limits = append(f.limits, limit)
	if key == f.failOn {
		return nil, errors.New("read failed")
	}
	return slices.Clone(f.of[key]), nil
}

func (f *routedFinder) DueJobsExceptTypes(_ context.Context, types []string, _, _ time.Time, limit int) ([]queue.Wake, error) {
	f.asked = append(f.asked, "except "+strings.Join(types, ","))
	f.limits = append(f.limits, limit)
	return slices.Clone(f.except), nil
}

func (f *routedFinder) NextJob(context.Context, string) (queue.Wake, bool, error) {
	return queue.Wake{}, false, nil
}

func TestWakeSweeperReadsEachRouteApart(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	fallback, routed := &testkit.Waker{}, &testkit.Waker{}
	router := queue.NewWakeRouter(fallback, []queue.WakeRoute{{Pattern: "federation.*", Waker: routed}})
	finder := &routedFinder{
		of:     map[string][]queue.Wake{"federation.deliver": {{JobID: "1", Type: "federation.deliver"}}},
		except: []queue.Wake{{JobID: "2", Type: "media.process"}},
	}
	sweeper := queue.NewWakeSweeper(queue.WakeSweepDeps{Finder: finder, Waker: router, Clock: clock, Batch: 7})
	sweeper.UseRoutes([][]string{{"federation.deliver"}})

	n, err := sweeper.Sweep(t.Context())
	if err != nil || n != 2 {
		t.Fatalf("Sweep = %d, %v, want 2", n, err)
	}
	if want := []string{"federation.deliver", "except federation.deliver"}; !slices.Equal(finder.asked, want) {
		t.Errorf("reads = %v, want %v", finder.asked, want)
	}
	if !slices.Equal(finder.limits, []int{7, 7}) {
		t.Errorf("limits = %v, want a batch per route", finder.limits)
	}
	if got := routed.Take(); len(got) != 1 || got[0].JobID != "1" || !got[0].At.Equal(clock.Now()) {
		t.Errorf("the route got %+v", got)
	}
	if got := fallback.Take(); len(got) != 1 || got[0].JobID != "2" {
		t.Errorf("the default queue got %+v", got)
	}
}

func TestWakeSweeperKeepsGoingAfterAFailedRoute(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	waker := &testkit.Waker{}
	finder := &routedFinder{failOn: "a", of: map[string][]queue.Wake{"b": {{JobID: "1", Type: "b"}}}}
	sweeper := queue.NewWakeSweeper(queue.WakeSweepDeps{Finder: finder, Waker: waker, Clock: clock})
	sweeper.UseRoutes([][]string{{"a"}, {"b"}})
	n, err := sweeper.Sweep(t.Context())
	if err == nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want the signal of b and the error of a", n, err)
	}
	if !slices.Contains(finder.asked, "except a,b") {
		t.Errorf("reads = %v, want the default queue's with every routed type", finder.asked)
	}
}

// Without routes the sweep reads as it always did.
func TestWakeSweeperWithoutRoutesReadsAllJobs(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	finder := &routedFinder{}
	sweeper := queue.NewWakeSweeper(queue.WakeSweepDeps{Finder: finder, Waker: &testkit.Waker{}, Clock: clock})
	if _, err := sweeper.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !slices.Equal(finder.asked, []string{"all"}) {
		t.Errorf("reads = %v, want only the read of all jobs", finder.asked)
	}
}
