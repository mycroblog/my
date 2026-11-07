// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

var start = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func TestClockContract(t *testing.T) {
	kerneltest.ClockContract(t, testkit.NewClock(start))
}

func TestKeyManagerContract(t *testing.T) {
	kerneltest.KeyManagerContract(t, testkit.NewKeyManager("test"))
}

func TestClockStandsStillUntilMoved(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	c := testkit.NewClock(start.In(jst).Add(123_456 * time.Nanosecond))

	if got := c.Now(); got != start {
		t.Fatalf("NewClock: Now() = %v, want %v", got, start)
	}
	if got := c.Now(); got != start {
		t.Fatalf("Now() moved on its own to %v", got)
	}
	if got, want := c.Advance(1500*time.Microsecond), start.Add(time.Millisecond); got != want || c.Now() != want {
		t.Fatalf("Advance: got %v, Now() = %v, want %v", got, c.Now(), want)
	}
	if got, want := c.Advance(-time.Hour), start.Add(time.Millisecond-time.Hour); got != want {
		t.Fatalf("Advance backwards: got %v, want %v", got, want)
	}
	c.Set(start.Add(24 * time.Hour).In(jst))
	if got, want := c.Now(), start.Add(24*time.Hour); got != want {
		t.Fatalf("Set: Now() = %v, want %v", got, want)
	}
}

func TestIDGeneratorContract(t *testing.T) {
	// A stopped clock puts every ID in the same millisecond.
	c := testkit.NewClock(start)
	ids := kerneltest.IDGeneratorContract(t, testkit.NewIDGenerator(c), c)
	for _, id := range ids {
		if ms, _ := kerneltest.UUIDv7Millis(id); ms != start.UnixMilli() {
			t.Fatalf("ID %q has timestamp %d, want %d", id, ms, start.UnixMilli())
		}
	}
}

func TestIDGeneratorNewIDAtIsDeterministic(t *testing.T) {
	c := testkit.NewClock(start)
	g := testkit.NewIDGenerator(c)
	got := []string{
		g.NewID(),
		g.NewIDAt(start.Add(-time.Second)),
		g.NewIDAt(start.Add(time.Hour)), // rounded down to the clock's time
		g.NewIDAt(start.Add(-time.Second)),
	}
	want := []string{
		"01a14495-5600-7000-8000-000000000001",
		"01a14495-5218-7000-8000-000000000002",
		"01a14495-5600-7000-8000-000000000003",
		"01a14495-5218-7000-8000-000000000004",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IDs = %q, want %q", got, want)
		}
	}
}

func TestIDGeneratorIsDeterministic(t *testing.T) {
	run := func() []string {
		c := testkit.NewClock(start)
		g := testkit.NewIDGenerator(c)
		ids := []string{g.NewID(), g.NewID()}
		c.Advance(time.Second)
		return append(ids, g.NewID())
	}
	want := []string{
		"01a14495-5600-7000-8000-000000000001",
		"01a14495-5600-7000-8000-000000000002",
		"01a14495-59e8-7000-8000-000000000003",
	}
	for range 2 {
		got := run()
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("IDs = %q, want %q", got, want)
			}
		}
	}
}

func TestIDGeneratorStampsClockTime(t *testing.T) {
	c := testkit.NewClock(start)
	g := testkit.NewIDGenerator(c)
	c.Advance(90 * time.Minute)
	id := g.NewID()
	if ms, _ := kerneltest.UUIDv7Millis(id); ms != c.Now().UnixMilli() {
		t.Errorf("ID %q has timestamp %d, want %d", id, ms, c.Now().UnixMilli())
	}
	if floor := kernel.UUIDv7Floor(c.Now()); id <= floor {
		t.Errorf("ID %q does not sort above its millisecond's floor %q", id, floor)
	}
}

func TestIDGeneratorConcurrentIDsAreUnique(t *testing.T) {
	g := testkit.NewIDGenerator(testkit.NewClock(start))
	const workers, perWorker = 8, 1000
	results := make(chan string, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				results <- g.NewID()
			}
		})
	}
	wg.Wait()
	close(results)
	seen := make(map[string]bool, workers*perWorker)
	for id := range results {
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
}

func TestKeyValueStoreContract(t *testing.T) {
	kerneltest.KeyValueStoreContract(t, func(_ *testing.T, clock *testkit.Clock) kernel.KeyValueStore {
		return testkit.NewKeyValueStore(clock)
	})
}

func TestRateLimiterSlidesItsWindow(t *testing.T) {
	clock := testkit.NewClock(start) // start is on a minute boundary
	l := kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock)
	lim := kernel.Limit{Name: "test", Max: 5, Window: time.Minute}
	ctx := t.Context()
	for range 5 {
		if err := l.Record(ctx, lim, "k"); err != nil {
			t.Fatal(err)
		}
	}
	d, err := l.Check(ctx, lim, "k")
	// The next window starts in 60 s; there the 5 events weigh (60-t)/60*5, which drops to 4 at t = 12 s.
	if err != nil || d.Allowed || d.RetryAfter != 72*time.Second || d.Reset != time.Minute {
		t.Fatalf("Check after 5 events = %+v, %v; want refused, retry after 72s, reset in 1m", d, err)
	}
	clock.Advance(72*time.Second - time.Millisecond)
	if d, _ := l.Check(ctx, lim, "k"); d.Allowed {
		t.Fatalf("Check 1 ms before Retry-After = %+v, want refused", d)
	}
	clock.Advance(time.Millisecond)
	if d, _ := l.Check(ctx, lim, "k"); !d.Allowed || d.Remaining != 1 {
		t.Fatalf("Check at Retry-After = %+v, want allowed with 1 remaining", d)
	}
	// Take counts the event it allows.
	if d, _ := l.Take(ctx, lim, "k"); !d.Allowed || d.Remaining != 0 {
		t.Fatalf("Take = %+v, want allowed with 0 remaining", d)
	}
	d, _ = l.Take(ctx, lim, "k")
	if d.Allowed || d.RetryAfter <= 0 {
		t.Fatalf("Take over the limit = %+v, want refused", d)
	}
	clock.Advance(d.RetryAfter)
	if d, _ := l.Check(ctx, lim, "k"); !d.Allowed {
		t.Fatalf("Check at the refused Take's Retry-After = %+v, want allowed", d)
	}
}

func TestRateLimitedErrorMatchesErrRateLimited(t *testing.T) {
	var err error = &kernel.RateLimitedError{RetryAfter: time.Second}
	if !errors.Is(err, kernel.ErrRateLimited) {
		t.Fatal("errors.Is(RateLimitedError, ErrRateLimited) = false")
	}
}

func TestClientKey(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.1":                 "192.0.2.1",
		"::ffff:192.0.2.1":          "192.0.2.1",
		"2001:db8:1:2:3:4:5:6":      "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1%eth0": "2001:db8:1:2::/64",
		"not an ip":                 "not an ip",
	} {
		if got := kernel.ClientKey(in); got != want {
			t.Errorf("ClientKey(%q) = %q, want %q", in, got, want)
		}
	}
}
