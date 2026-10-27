// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// KeyValueStoreFactory returns a new, empty store whose entries expire by clock. The factory
// registers the cleanup of everything it creates with t.Cleanup.
type KeyValueStoreFactory func(t *testing.T, clock *Clock) kernel.KeyValueStore

// KeyValueStoreContract verifies a kernel.KeyValueStore adapter: values round-trip and are
// copied, namespaces are separate, Set replaces the value and the expiry, Delete is idempotent,
// entries expire after their TTL and never without one, IncrBy starts missing and expired
// counters and keeps the expiry of live ones, concurrent IncrBy calls lose no increment,
// CompareAndSwap writes and deletes only an entry that holds the value it expects (absent when
// it expects nil), and exactly one of concurrent swaps from one value wins, DeleteExpired removes
// only expired entries and at most its limit per call, and a kernel.RateLimiter on the store
// enforces a limit.
func KeyValueStoreContract(t *testing.T, newStore KeyValueStoreFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, s kernel.KeyValueStore, clock *Clock)
	}{
		{"RoundTrips", kvRoundTrips},
		{"SeparatesNamespaces", kvSeparatesNamespaces},
		{"SetReplaces", kvSetReplaces},
		{"Deletes", kvDeletes},
		{"Expires", kvExpires},
		{"Increments", kvIncrements},
		{"IncrementsConcurrently", kvIncrementsConcurrently},
		{"ComparesAndSwaps", kvComparesAndSwaps},
		{"SwapsOnceConcurrently", kvSwapsOnceConcurrently},
		{"DeletesExpired", kvDeletesExpired},
		{"BacksARateLimiter", kvBacksARateLimiter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
			tc.run(t, newStore(t, clock), clock)
		})
	}
}

func kvWant(t *testing.T, s kernel.KeyValueStore, ns, key, want string) {
	t.Helper()
	got, err := s.Get(t.Context(), ns, key)
	if err != nil || string(got) != want {
		t.Fatalf("Get(%s, %s) = %q, %v; want %q", ns, key, got, err, want)
	}
}

func kvWantMissing(t *testing.T, s kernel.KeyValueStore, ns, key string) {
	t.Helper()
	if got, err := s.Get(t.Context(), ns, key); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Fatalf("Get(%s, %s) = %q, %v; want ErrKeyNotFound", ns, key, got, err)
	}
}

func kvSet(t *testing.T, s kernel.KeyValueStore, ns, key, value string, ttl time.Duration) {
	t.Helper()
	if err := s.Set(t.Context(), ns, key, []byte(value), ttl); err != nil {
		t.Fatalf("Set(%s, %s): %v", ns, key, err)
	}
}

func kvIncr(t *testing.T, s kernel.KeyValueStore, ns, key string, n int64, ttl time.Duration) int64 {
	t.Helper()
	got, err := s.IncrBy(t.Context(), ns, key, n, ttl)
	if err != nil {
		t.Fatalf("IncrBy(%s, %s, %d): %v", ns, key, n, err)
	}
	return got
}

func kvRoundTrips(t *testing.T, s kernel.KeyValueStore, _ *Clock) {
	kvWantMissing(t, s, "ns", "k")
	value := []byte{0, 1, 2, 0xff, 'x'}
	if err := s.Set(t.Context(), "ns", "k:with/odd chars ☃", value, 0); err != nil {
		t.Fatal(err)
	}
	value[0] = 9 // the store keeps its own copy
	got, err := s.Get(t.Context(), "ns", "k:with/odd chars ☃")
	if err != nil || !bytes.Equal(got, []byte{0, 1, 2, 0xff, 'x'}) {
		t.Fatalf("Get = %v, %v; want the bytes that were set", got, err)
	}
	kvSet(t, s, "ns", "empty", "", 0)
	kvWant(t, s, "ns", "empty", "")
}

func kvSeparatesNamespaces(t *testing.T, s kernel.KeyValueStore, _ *Clock) {
	kvSet(t, s, "a", "k", "in a", 0)
	kvSet(t, s, "b", "k", "in b", 0)
	kvWant(t, s, "a", "k", "in a")
	kvWant(t, s, "b", "k", "in b")
	if err := s.Delete(t.Context(), "a", "k"); err != nil {
		t.Fatal(err)
	}
	kvWantMissing(t, s, "a", "k")
	kvWant(t, s, "b", "k", "in b")
}

func kvSetReplaces(t *testing.T, s kernel.KeyValueStore, clock *Clock) {
	kvSet(t, s, "ns", "k", "first", time.Minute)
	kvSet(t, s, "ns", "k", "second", 0)
	clock.Advance(time.Hour)
	kvWant(t, s, "ns", "k", "second") // the second Set removed the expiry
	kvSet(t, s, "ns", "k", "third", time.Minute)
	clock.Advance(time.Minute)
	kvWantMissing(t, s, "ns", "k")
}

func kvDeletes(t *testing.T, s kernel.KeyValueStore, _ *Clock) {
	kvSet(t, s, "ns", "k", "v", 0)
	for range 2 {
		if err := s.Delete(t.Context(), "ns", "k"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	}
	kvWantMissing(t, s, "ns", "k")
}

func kvExpires(t *testing.T, s kernel.KeyValueStore, clock *Clock) {
	kvSet(t, s, "ns", "short", "v", time.Second)
	kvSet(t, s, "ns", "forever", "v", 0)
	clock.Advance(999 * time.Millisecond)
	kvWant(t, s, "ns", "short", "v")
	clock.Advance(time.Millisecond)
	kvWantMissing(t, s, "ns", "short")
	clock.Advance(1000 * time.Hour)
	kvWant(t, s, "ns", "forever", "v")
}

func kvIncrements(t *testing.T, s kernel.KeyValueStore, clock *Clock) {
	if got := kvIncr(t, s, "ns", "c", 3, time.Minute); got != 3 {
		t.Fatalf("IncrBy on a missing key = %d, want 3", got)
	}
	clock.Advance(30 * time.Second)
	// A live counter keeps the expiry of its first write.
	if got := kvIncr(t, s, "ns", "c", -1, time.Hour); got != 2 {
		t.Fatalf("IncrBy(-1) = %d, want 2", got)
	}
	kvWant(t, s, "ns", "c", "2")
	clock.Advance(30 * time.Second)
	kvWantMissing(t, s, "ns", "c")
	if got := kvIncr(t, s, "ns", "c", 5, 0); got != 5 {
		t.Fatalf("IncrBy on an expired key = %d, want 5", got)
	}
	clock.Advance(1000 * time.Hour)
	kvWant(t, s, "ns", "c", "5")
	if got := kvIncr(t, s, "ns", "c", 1<<40, 0); got != 5+1<<40 {
		t.Fatalf("IncrBy(2^40) = %d", got)
	}
}

func kvIncrementsConcurrently(t *testing.T, s kernel.KeyValueStore, _ *Clock) {
	const workers, each = 8, 10
	var wg sync.WaitGroup
	errs := make(chan error, workers*each)
	for range workers {
		wg.Go(func() {
			for range each {
				if _, err := s.IncrBy(t.Context(), "ns", "c", 1, time.Hour); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent IncrBy: %v", err)
	}
	kvWant(t, s, "ns", "c", "80")
}

func kvSwap(t *testing.T, s kernel.KeyValueStore, key string, old, value []byte, ttl time.Duration) bool {
	t.Helper()
	ok, err := s.CompareAndSwap(t.Context(), "ns", key, old, value, ttl)
	if err != nil {
		t.Fatalf("CompareAndSwap(%s, %q, %q): %v", key, old, value, err)
	}
	return ok
}

func kvComparesAndSwaps(t *testing.T, s kernel.KeyValueStore, clock *Clock) {
	// Absent: only a swap that expects absence writes.
	if kvSwap(t, s, "k", []byte("a"), []byte("b"), 0) {
		t.Fatal("swap from a value of an absent entry succeeded")
	}
	kvWantMissing(t, s, "ns", "k")
	if !kvSwap(t, s, "k", nil, []byte("a"), time.Minute) {
		t.Fatal("swap of an absent entry failed")
	}
	kvWant(t, s, "ns", "k", "a")
	if kvSwap(t, s, "k", nil, []byte("x"), 0) {
		t.Fatal("swap expecting absence of a live entry succeeded")
	}
	// Present: only a swap that expects its value writes, with the new expiry.
	if kvSwap(t, s, "k", []byte("z"), []byte("x"), 0) {
		t.Fatal("swap from another value succeeded")
	}
	if !kvSwap(t, s, "k", []byte("a"), []byte("b"), time.Hour) {
		t.Fatal("swap from the value failed")
	}
	kvWant(t, s, "ns", "k", "b")
	clock.Advance(30 * time.Minute)
	kvWant(t, s, "ns", "k", "b")
	// A nil value deletes the entry when it holds old.
	if kvSwap(t, s, "k", []byte("a"), nil, 0) {
		t.Fatal("delete from another value succeeded")
	}
	if !kvSwap(t, s, "k", []byte("b"), nil, 0) {
		t.Fatal("delete from the value failed")
	}
	kvWantMissing(t, s, "ns", "k")
	// An expired entry is absent.
	kvSet(t, s, "ns", "e", "old", time.Minute)
	clock.Advance(2 * time.Minute)
	if kvSwap(t, s, "e", []byte("old"), []byte("new"), 0) {
		t.Fatal("swap from the value of an expired entry succeeded")
	}
	if !kvSwap(t, s, "e", nil, []byte("new"), 0) {
		t.Fatal("swap of an expired entry failed")
	}
	kvWant(t, s, "ns", "e", "new")
	clock.Advance(24 * time.Hour)
	kvWant(t, s, "ns", "e", "new")
}

func kvSwapsOnceConcurrently(t *testing.T, s kernel.KeyValueStore, _ *Clock) {
	kvSet(t, s, "ns", "k", "0", time.Hour)
	const workers = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)
	errs := make(chan error, workers)
	for i := range workers {
		wg.Go(func() {
			ok, err := s.CompareAndSwap(t.Context(), "ns", "k", []byte("0"), []byte(strconv.Itoa(i+1)), time.Hour)
			if err != nil {
				errs <- err
				return
			}
			if ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent CompareAndSwap: %v", err)
	}
	if wins != 1 {
		t.Fatalf("%d concurrent swaps from one value won, want 1", wins)
	}
}

func kvDeletesExpired(t *testing.T, s kernel.KeyValueStore, clock *Clock) {
	kvSet(t, s, "ns", "a", "v", time.Second)
	kvIncr(t, s, "other", "b", 1, time.Second)
	kvSet(t, s, "ns", "c", "v", time.Second)
	kvSet(t, s, "ns", "live", "v", time.Hour)
	kvSet(t, s, "ns", "forever", "v", 0)
	clock.Advance(time.Minute)
	if n, err := s.DeleteExpired(t.Context(), 0); err != nil || n != 0 {
		t.Fatalf("DeleteExpired(0) = %d, %v; want 0", n, err)
	}
	// A batch removes at most its limit, and the next batch continues.
	n, err := s.DeleteExpired(t.Context(), 2)
	if err != nil || n != 2 {
		t.Fatalf("DeleteExpired(2) = %d, %v; want 2", n, err)
	}
	n, err = s.DeleteExpired(t.Context(), 2)
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpired(2) again = %d, %v; want the last 1", n, err)
	}
	kvWant(t, s, "ns", "live", "v")
	kvWant(t, s, "ns", "forever", "v")
	if n, err := s.DeleteExpired(t.Context(), 100); err != nil || n != 0 {
		t.Fatalf("DeleteExpired with none expired = %d, %v; want 0", n, err)
	}
	// An expired counter that IncrBy started again is live and stays.
	kvIncr(t, s, "other", "b", 5, time.Second)
	clock.Advance(time.Minute)
	kvIncr(t, s, "other", "b", 7, time.Hour)
	if n, err := s.DeleteExpired(t.Context(), 100); err != nil || n != 0 {
		t.Fatalf("DeleteExpired after restarting a counter = %d, %v; want 0", n, err)
	}
	kvWant(t, s, "other", "b", "7")
}

func kvBacksARateLimiter(t *testing.T, s kernel.KeyValueStore, clock *Clock) {
	l := kernel.NewRateLimiter(s, clock)
	lim := kernel.Limit{Name: "contract", Max: 3, Window: time.Minute}
	for i := range 3 {
		if d, err := l.Take(t.Context(), lim, "client"); err != nil || !d.Allowed {
			t.Fatalf("Take #%d = %+v, %v; want allowed", i+1, d, err)
		}
	}
	d, err := l.Take(t.Context(), lim, "client")
	if err != nil || d.Allowed || d.RetryAfter <= 0 {
		t.Fatalf("Take #4 = %+v, %v; want refused with a Retry-After", d, err)
	}
	if d, err := l.Check(t.Context(), lim, "other client"); err != nil || !d.Allowed || d.Remaining != 3 {
		t.Fatalf("Check(other client) = %+v, %v; want allowed with 3 remaining", d, err)
	}
	clock.Advance(2 * time.Minute)
	if d, err := l.Check(t.Context(), lim, "client"); err != nil || !d.Allowed {
		t.Fatalf("Check after two windows = %+v, %v; want allowed", d, err)
	}
}
