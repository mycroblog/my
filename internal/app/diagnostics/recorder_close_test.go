// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

type blockingRepo struct {
	Repository
	entered  chan struct{}
	inflight atomic.Int32
	writes   atomic.Int32
}

func (r *blockingRepo) AddCount(ctx context.Context, _ Count) error {
	r.inflight.Add(1)
	defer r.inflight.Add(-1)
	r.writes.Add(1)
	select {
	case r.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (r *blockingRepo) PutSample(ctx context.Context, _ Sample) error { return ctx.Err() }

// noTx runs no function: RecordFirst then only adds the count and puts the sample.
type noTx struct{ uow.UnitOfWork }

func (noTx) Tx(context.Context, func(uow.Tx) error) error { return nil }

type fixedIDs struct{ kernel.IDGenerator }

func (fixedIDs) NewID() string { return "id" }

func TestCloseCancelsTheWriteInFlightAtTheDeadline(t *testing.T) {
	repo := &blockingRepo{entered: make(chan struct{}, 1)}
	rec := NewRecorder(RecorderOptions{
		Service:  NewService(Deps{UnitOfWork: noTx{}, Repository: repo, IDs: fixedIDs{}}),
		Interval: time.Hour,
	})
	rec.ReceiveError(t.Context(), kernel.ErrorEvent{Fingerprint: "fp", Time: time.Now()})
	select {
	case <-repo.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the write did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := rec.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close = %v, want the deadline", err)
	}
	if n := repo.inflight.Load(); n != 0 {
		t.Errorf("%d writes still run after Close returned", n)
	}
	before := repo.writes.Load()
	rec.Flush(t.Context())
	if repo.writes.Load() != before {
		t.Error("Flush wrote after Close returned")
	}
}
