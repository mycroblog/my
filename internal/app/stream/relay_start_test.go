// SPDX-License-Identifier: AGPL-3.0-only

package stream_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
)

// The start of the listener on a broker whose Listen is slow or fails, as the creation of a
// Pub/Sub subscription is on a new Cloud Run instance (#3441). All run on the bubble's clock.

// listenCall is one call of startBus.Listen: when it began, and when its context ended, if it did
// before the call returned.
type listenCall struct {
	start time.Time
	ended time.Time // zero when the context did not end during the call
}

// startBus is a stream.Bus on a MemoryBroker whose Listen fails the first fail calls at once,
// waits hang first (or until its context ends) on each call that does not fail, and records
// its calls.
type startBus struct {
	stream.Bus

	mu    sync.Mutex
	fail  int           // the first fail calls fail at once
	hang  int           // after them, the next hang calls wait until their context ends
	delay time.Duration // every other call waits delay before it listens
	calls []listenCall
}

var errCreate = errors.New("create subscription: rpc error: code = Internal")

func (b *startBus) Listen(ctx context.Context, receive func([]byte)) (stream.Listener, error) {
	b.mu.Lock()
	n := len(b.calls)
	b.calls = append(b.calls, listenCall{start: time.Now()})
	fail, hang, delay := n < b.fail, n >= b.fail && n < b.fail+b.hang, b.delay
	b.mu.Unlock()
	if fail {
		return nil, errCreate
	}
	var wait <-chan time.Time
	if !hang {
		wait = time.After(delay)
	}
	select {
	case <-wait:
	case <-ctx.Done():
		b.mu.Lock()
		b.calls[n].ended = time.Now()
		b.mu.Unlock()
		return nil, ctx.Err()
	}
	return b.Bus.Listen(ctx, receive)
}

func (b *startBus) Calls() []listenCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]listenCall(nil), b.calls...)
}

// subscribeResult is what a Subscribe of another goroutine returned, and when.
type subscribeResult struct {
	s   stream.Subscription
	err error
	at  time.Time
}

func subscribeAsync(ctx context.Context, r *stream.Relay, accountID string) <-chan subscribeResult {
	ch := make(chan subscribeResult, 1)
	go func() {
		s, err := r.Subscribe(ctx, accountID)
		ch <- subscribeResult{s: s, err: err, at: time.Now()}
	}()
	return ch
}

// A Listen slower than the 10 seconds a stream used to wait, as the first creation of a
// subscription on a new instance is, still gives the streams that wait for it, and those that
// arrive during it, their subscriptions instead of ErrUnavailable (a 503).
func TestRelaySlowListen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 15 * time.Second
		bus := &startBus{Bus: stream.NewMemoryBroker().Bus(), delay: delay}
		r := stream.NewRelay(bus, stream.RelayOptions{})
		defer closeRelay(t, r)

		start := time.Now()
		first := subscribeAsync(t.Context(), r, aliceID)
		advance(12 * time.Second)
		select {
		case res := <-first:
			t.Fatalf("Subscribe returned before the listener was ready: %v", res.err)
		default:
		}
		second := subscribeAsync(t.Context(), r, bobID)
		for name, ch := range map[string]<-chan subscribeResult{"first": first, "second": second} {
			res := <-ch
			if res.err != nil {
				t.Fatalf("%s Subscribe during a slow Listen: %v, want a subscription", name, res.err)
			}
			if waited := res.at.Sub(start); waited != delay {
				t.Errorf("%s Subscribe returned at %s, want at %s, once Listen returned", name, waited, delay)
			}
			defer res.s.Close()
		}
		if calls := bus.Calls(); len(calls) != 1 {
			t.Errorf("Listen calls = %d, want 1", len(calls))
		}
		if got := r.Len(); got != 2 {
			t.Errorf("Len = %d, want 2", got)
		}
	})
}

// After Listen fails, the start tries again with a backoff, and a waiting Subscribe gets its
// subscription once a call succeeds within ReadyTimeout.
func TestRelayRetriesAFailedListen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := &startBus{Bus: stream.NewMemoryBroker().Bus(), fail: 3}
		r := stream.NewRelay(bus, stream.RelayOptions{})
		defer closeRelay(t, r)

		start := time.Now()
		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatalf("Subscribe after 3 failed Listens: %v, want a subscription", err)
		}
		defer s.Close()
		if waited := time.Since(start); waited >= stream.DefaultReadyTimeout {
			t.Errorf("Subscribe waited %s, want less than %s", waited, stream.DefaultReadyTimeout)
		}
		calls := bus.Calls()
		if len(calls) != 4 {
			t.Fatalf("Listen calls = %d, want 4", len(calls))
		}
		// A backoff of about a second, doubling: never a tight loop, never more than 8 seconds.
		for i := 1; i < len(calls); i++ {
			gap := calls[i].start.Sub(calls[i-1].start)
			if gap < 500*time.Millisecond || gap > 12*time.Second {
				t.Errorf("Listen call %d came %s after the previous one, want a backoff of 1s doubling to 8s", i+1, gap)
			}
		}
		if gap := calls[1].start.Sub(calls[0].start); gap > 2*time.Second {
			t.Errorf("the first retry came %s after the failure, want about a second", gap)
		}
	})
}

// An attempt whose Listen hangs is cancelled at ListenAttemptTimeout, and the next attempt
// starts; a Subscribe waiting for the start gets its subscription from it.
func TestRelayCancelsAHangingListen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := &startBus{Bus: stream.NewMemoryBroker().Bus(), hang: 1}
		r := stream.NewRelay(bus, stream.RelayOptions{})
		defer closeRelay(t, r)

		start := time.Now()
		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatalf("Subscribe after a hanging Listen: %v, want a subscription", err)
		}
		defer s.Close()
		if waited := time.Since(start); waited <= stream.ListenAttemptTimeout || waited >= stream.DefaultReadyTimeout {
			t.Errorf("Subscribe waited %s, want between %s and %s", waited, stream.ListenAttemptTimeout, stream.DefaultReadyTimeout)
		}
		calls := bus.Calls()
		if len(calls) != 2 {
			t.Fatalf("Listen calls = %d, want 2", len(calls))
		}
		if calls[0].ended.IsZero() {
			t.Fatal("the hanging Listen was not cancelled")
		}
		if got := calls[0].ended.Sub(calls[0].start); got != stream.ListenAttemptTimeout {
			t.Errorf("the hanging Listen was cancelled after %s, want %s", got, stream.ListenAttemptTimeout)
		}
		if !calls[1].start.After(calls[0].ended) {
			t.Errorf("the next attempt began at %s, before the hanging one ended at %s", calls[1].start, calls[0].ended)
		}
	})
}

// A start whose every Listen fails gives ErrUnavailable at ReadyTimeout, and ends at
// StartTimeout: after it, Listen is not called again until the next Subscribe.
func TestRelayUnavailableWhenEveryListenFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := &startBus{Bus: stream.NewMemoryBroker().Bus(), fail: 1 << 30}
		r := stream.NewRelay(bus, stream.RelayOptions{})
		defer closeRelay(t, r)

		start := time.Now()
		if _, err := r.Subscribe(t.Context(), aliceID); !errors.Is(err, stream.ErrUnavailable) {
			t.Fatalf("Subscribe when every Listen fails: %v, want ErrUnavailable", err)
		}
		if waited := time.Since(start); waited != stream.DefaultReadyTimeout {
			t.Errorf("Subscribe waited %s, want %s", waited, stream.DefaultReadyTimeout)
		}
		if got := len(bus.Calls()); got < 2 {
			t.Errorf("Listen calls within ReadyTimeout = %d, want the failed one retried", got)
		}

		advance(stream.DefaultStartTimeout)
		calls := bus.Calls()
		if last := calls[len(calls)-1].start.Sub(start); last >= stream.DefaultStartTimeout {
			t.Errorf("the last Listen began %s after the start, want before StartTimeout %s", last, stream.DefaultStartTimeout)
		}
		advance(time.Minute)
		if got := len(bus.Calls()); got != len(calls) {
			t.Errorf("Listen calls after the start ended = %d, want %d", got, len(calls))
		}
		if got := r.Len(); got != 0 {
			t.Errorf("Len = %d, want 0", got)
		}
	})
}

// With ReadyTimeout longer than StartTimeout, Subscribe reports ErrUnavailable once the start
// ends, not at ReadyTimeout.
func TestRelayUnavailableOnceTheStartEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const startTimeout = 40 * time.Second
		bus := &startBus{Bus: stream.NewMemoryBroker().Bus(), fail: 1 << 30}
		r := stream.NewRelay(bus, stream.RelayOptions{ReadyTimeout: 2 * time.Minute, StartTimeout: startTimeout})
		defer closeRelay(t, r)

		start := time.Now()
		if _, err := r.Subscribe(t.Context(), aliceID); !errors.Is(err, stream.ErrUnavailable) {
			t.Fatalf("Subscribe when every Listen fails: %v, want ErrUnavailable", err)
		}
		if waited := time.Since(start); waited > startTimeout {
			t.Errorf("Subscribe waited %s, want at most StartTimeout %s", waited, startTimeout)
		}
		if got := len(bus.Calls()); got < 2 {
			t.Errorf("Listen calls = %d, want the failed one retried", got)
		}
	})
}
