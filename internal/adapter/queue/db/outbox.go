// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// Outbox keeps jobs in outbox_events until Relay hands them to a queue outside the database
// (Transactional Outbox, ADR-0004).
type Outbox struct {
	db   uow.UnitOfWork
	st   store
	opts Options
	// holder names this Outbox in the relay lease.
	holder string
}

var _ queue.Outbox = (*Outbox)(nil)

func newOutbox(d uow.UnitOfWork, st store, opts Options) *Outbox {
	opts.check()
	if opts.RelayLease <= 0 {
		opts.RelayLease = DefaultRelayLease
	}
	return &Outbox{db: d, st: st, opts: opts, holder: opts.IDs.NewID()}
}

// EnqueueTx implements queue.Outbox: it writes the jobs to outbox_events in tx. Idempotency
// keys take effect when Relay enqueues the jobs.
func (o *Outbox) EnqueueTx(ctx context.Context, tx uow.Tx, jobs ...queue.NewJob) error {
	built, err := build(ctx, o.opts, jobs)
	if err != nil {
		return err
	}
	for _, j := range built {
		if err := o.st.insertOutbox(ctx, tx, j); err != nil {
			return fmt.Errorf("queue/db: outbox %s: %w", j.Type, err)
		}
	}
	return nil
}

// DefaultRelayBatch is how many outbox events Relay moves per call when limit <= 0.
const DefaultRelayBatch = 100

// DefaultRelayLease is how long one Relay call holds the relay lease when
// Options.RelayLease is not positive.
const DefaultRelayLease = 30 * time.Second

// relayLeaseName names the lease of the outbox in outbox_relay_leases.
const relayLeaseName = "outbox_events"

// ErrRelayLeaseLost is returned by Relay when its lease expired while it enqueued: another
// relay may have taken the events over, so it leaves them in the outbox.
var ErrRelayLeaseLost = errors.New("queue/db: outbox relay lease lost")

// Relay moves up to limit outbox events, oldest first, into target and returns how many it
// moved. It enqueues outside any transaction and deletes the events afterwards, so a crash in
// between enqueues them again on the next call: delivery is at-least-once, and the jobs'
// idempotency keys and idempotent handlers absorb the duplicates.
//
// One relay runs at a time, whatever the number of processes: Relay first takes the lease of
// the outbox for Options.RelayLease, and moves nothing (0, nil) while another relay holds it.
// So the events reach target in the order of the outbox, and an event does not reach it again
// after a later one, which keeps the order of the jobs of an ordering key. That holds while
// each enqueue ends within the lease: Relay cancels the enqueue at the lease's end, and when
// the lease ran out it leaves the events for the next relay and returns ErrRelayLeaseLost.
func (o *Outbox) Relay(ctx context.Context, target queue.JobQueue, limit int) (int, error) {
	if limit <= 0 {
		limit = DefaultRelayBatch
	}
	// The deadline of the enqueue is taken before the lease, so that it ends first.
	leaseEnd := time.Now().Add(o.opts.RelayLease)
	now := o.opts.Clock.Now()
	var (
		events []queue.Job
		held   bool
	)
	err := o.db.Tx(ctx, func(tx uow.Tx) error {
		events = nil
		var err error
		if held, err = o.st.acquireRelayLease(ctx, tx, relayLeaseName, o.holder, now, now.Add(o.opts.RelayLease)); err != nil || !held {
			return err
		}
		events, err = o.st.listOutbox(ctx, tx, limit)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("queue/db: read outbox: %w", err)
	}
	if !held {
		return 0, nil
	}
	if len(events) > 0 {
		jobs := make([]queue.NewJob, len(events))
		for i, e := range events {
			jobs[i] = queue.NewJob{
				Type:           e.Type,
				Payload:        e.Payload,
				IdempotencyKey: e.IdempotencyKey,
				OrderingKey:    e.OrderingKey,
				RunAt:          e.RunAt,
				TraceParent:    e.TraceParent,
			}
		}
		enqueueCtx, cancel := context.WithDeadline(ctx, leaseEnd)
		err := target.Enqueue(enqueueCtx, jobs...)
		cancel()
		if err != nil {
			return 0, fmt.Errorf("queue/db: relay outbox: %w", err)
		}
	}
	// The record context outlives ctx, so that enqueued events are deleted and the lease
	// ends; the lease bounds it.
	recordCtx, cancel := context.WithDeadline(context.WithoutCancel(ctx), leaseEnd)
	defer cancel()
	err = o.db.Tx(recordCtx, func(tx uow.Tx) error {
		ok, err := o.st.endRelayLease(recordCtx, tx, relayLeaseName, o.holder, o.opts.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return ErrRelayLeaseLost
		}
		for _, e := range events {
			if err := o.st.deleteOutbox(recordCtx, tx, e.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("queue/db: delete relayed outbox events: %w", err)
	}
	return len(events), nil
}

// RunRelay relays outbox events into target until ctx ends, polling every interval while the
// outbox is empty. It logs errors and keeps going.
func (o *Outbox) RunRelay(ctx context.Context, target queue.JobQueue, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = queue.DefaultPollInterval
	}
	if logger == nil {
		logger = slog.Default()
	}
	for {
		n, err := o.Relay(ctx, target, 0)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.ErrorContext(ctx, "outbox relay", slog.Any("error", err))
		}
		if n > 0 && err == nil {
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
