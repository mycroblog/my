// SPDX-License-Identifier: AGPL-3.0-only

package db_test

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// relayTarget records the jobs relayed into it, in order. Its first Enqueue stalls until
// release is closed, as a slow queue service would.
type relayTarget struct {
	mu      sync.Mutex
	seqs    []string
	stall   bool
	entered chan struct{}
	release chan struct{}
}

func (r *relayTarget) Enqueue(ctx context.Context, jobs ...queue.NewJob) error {
	r.mu.Lock()
	stall := r.stall
	r.stall = false
	r.mu.Unlock()
	if stall {
		close(r.entered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range jobs {
		var v struct{ Seq string }
		_ = json.Unmarshal(j.Payload, &v)
		r.seqs = append(r.seqs, v.Seq)
	}
	return nil
}

// testOverlappingRelays runs two relays of one outbox at the same time: while the first one
// stalls in the target queue, the second one must not move the events after it, or the jobs of
// an ordering key would reach the queue out of order.
func testOverlappingRelays(t *testing.T, d uow.UnitOfWork, first, second *queuedb.Outbox) {
	t.Helper()
	enqueue := func(seq string) {
		t.Helper()
		err := d.Tx(t.Context(), func(tx uow.Tx) error {
			return first.EnqueueTx(t.Context(), tx, queue.NewJob{
				Type: "test.ordered", OrderingKey: "key", Payload: json.RawMessage(`{"Seq":"` + seq + `"}`),
			})
		})
		if err != nil {
			t.Fatalf("EnqueueTx: %v", err)
		}
	}
	target := &relayTarget{stall: true, entered: make(chan struct{}), release: make(chan struct{})}
	enqueue("1")
	done := make(chan error, 1)
	go func() {
		_, err := first.Relay(t.Context(), target, 0)
		done <- err
	}()
	select {
	case <-target.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the first relay did not reach the target")
	}
	enqueue("2")
	if _, err := second.Relay(t.Context(), target, 0); err != nil {
		t.Fatalf("second Relay: %v", err)
	}
	close(target.release)
	if err := <-done; err != nil {
		t.Fatalf("first Relay: %v", err)
	}
	// Whichever relay goes on, the rest of the outbox follows.
	for range 2 {
		if _, err := second.Relay(t.Context(), target, 0); err != nil {
			t.Fatalf("Relay: %v", err)
		}
		if _, err := first.Relay(t.Context(), target, 0); err != nil {
			t.Fatalf("Relay: %v", err)
		}
	}
	if want := []string{"1", "2"}; !slices.Equal(target.seqs, want) {
		t.Errorf("relayed %v, want %v: an older job reached the queue after a newer one", target.seqs, want)
	}
}
