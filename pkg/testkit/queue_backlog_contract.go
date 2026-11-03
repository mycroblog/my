// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"maps"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// QueueBacklogContract verifies a queue adapter whose harness Queue is a queue.BacklogReader:
// Backlog counts the due jobs that no worker holds by type, with the earliest due time, and the
// dead jobs, and counts a held job as ready again once its lease expires.
func QueueBacklogContract(t *testing.T, newHarness QueueFactory) {
	t.Helper()
	clock := NewClock(contractStart)
	h := newHarness(t, clock)
	reader, ok := h.Queue.(queue.BacklogReader)
	if !ok {
		t.Fatalf("%T is not a queue.BacklogReader", h.Queue)
	}
	ctx := t.Context()

	b, err := reader.Backlog(ctx, clock.Now())
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if len(b.Ready) != 0 || !b.OldestReadyAt.IsZero() || b.Dead != 0 {
		t.Fatalf("empty queue: Backlog = %+v", b)
	}

	if err := h.Queue.Enqueue(ctx,
		queue.NewJob{Type: "test.a"}, queue.NewJob{Type: "test.a"}, queue.NewJob{Type: "test.b"},
		queue.NewJob{Type: "test.c", RunAt: contractStart.Add(time.Hour)},
	); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	clock.Advance(10 * time.Second)

	// One job dies, and a worker holds another.
	dead, ok, err := h.Source.Claim(ctx, "w1", clock.Now(), time.Minute)
	if err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	if err := h.Source.Bury(ctx, "w1", dead, "boom", clock.Now()); err != nil {
		t.Fatalf("Bury: %v", err)
	}
	held, ok, err := h.Source.Claim(ctx, "w2", clock.Now(), time.Minute)
	if err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}

	want := map[string]int64{"test.a": 2, "test.b": 1}
	want[dead.Type]--
	want[held.Type]--
	maps.DeleteFunc(want, func(_ string, n int64) bool { return n == 0 })
	b, err = reader.Backlog(ctx, clock.Now())
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if !maps.Equal(b.Ready, want) || !b.OldestReadyAt.Equal(contractStart) || b.Dead != 1 {
		t.Fatalf("Backlog = %+v, want Ready %v, OldestReadyAt %v, Dead 1", b, want, contractStart)
	}
	if len(b.OldestReadyByType) != len(want) {
		t.Fatalf("OldestReadyByType = %v, want the types of %v", b.OldestReadyByType, want)
	}
	for jobType := range want {
		if !b.OldestReadyByType[jobType].Equal(contractStart) {
			t.Errorf("OldestReadyByType[%s] = %v, want %v", jobType, b.OldestReadyByType[jobType], contractStart)
		}
	}

	// Once the lease expires, the held job is ready again.
	clock.Advance(2 * time.Minute)
	want[held.Type]++
	b, err = reader.Backlog(ctx, clock.Now())
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if !maps.Equal(b.Ready, want) {
		t.Fatalf("after the lease: Ready = %v, want %v", b.Ready, want)
	}
}
