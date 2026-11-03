// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// Retention of the inbox and delivery records (ADR-0034, data-model §4.8, FR-803).
const (
	// JobPurge deletes the inbox and delivery records past their retention; no payload.
	JobPurge = "federation.purge"
	// PurgeInterval is the period of the purge's schedule.
	PurgeInterval = time.Hour
	// ReceivedDocumentRetention is how long a received document is kept, for investigation and
	// fixtures; ReceivedActivityRetention how long the record of a received activity is.
	ReceivedDocumentRetention = 7 * 24 * time.Hour
	ReceivedActivityRetention = 30 * 24 * time.Hour
	// RemoteObjectTombstoneRetention is how long a tombstone of an unknown object is kept.
	RemoteObjectTombstoneRetention = 30 * 24 * time.Hour
	// RemoteFollowTombstoneRetention is how long a tombstone of an unknown Follow is kept.
	RemoteFollowTombstoneRetention = 30 * 24 * time.Hour
	// OutboundDocumentRetention is how long the body of an outbound activity is kept, well
	// past the last retry of its deliveries (DeliveryRetryPolicy).
	OutboundDocumentRetention = 30 * 24 * time.Hour
	// DeliveryAttemptRetention is how long the record of a delivery attempt is kept (NFR-OPS-3).
	DeliveryAttemptRetention = 30 * 24 * time.Hour
	// OutboundActivityRetention is how long an outbound activity and its deliveries are kept. A
	// status's Create stays longer: its deliveries are the inboxes of the status's later Update
	// and Delete, so it goes this long after the Delete of the status or of its actor.
	OutboundActivityRetention = 30 * 24 * time.Hour
	// InstanceObservationRetention is how long a NodeInfo observation or an availability event of
	// a server is kept; the newest of each server always stays.
	InstanceObservationRetention = 90 * 24 * time.Hour
	// outboundPurgeBatch bounds the activities of one transaction, each with a delivery per inbox.
	outboundPurgeBatch = 100
	// PurgeBatch is the number of rows one statement deletes; purgeMaxBatches bounds the
	// batches of one table in one run, and the next run continues.
	PurgeBatch      = 1000
	purgeMaxBatches = 20
)

// PurgeRepository is the part of the federation module's persistence that the purge uses; each
// method deletes inside tx at most limit rows, the oldest first, in one statement, and returns
// how many it deleted. The database adapters' federation repositories implement it and pass
// testkit.RetentionContract.
type PurgeRepository interface {
	// DeleteReceivedDocumentsBefore deletes the documents of activities received before t.
	DeleteReceivedDocumentsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteReceivedActivityAnswersBefore deletes the records of answers made before t, at most
	// limit, and returns how many.
	DeleteReceivedActivityAnswersBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteReceivedActivitiesBefore deletes the records of activities received before t whose
	// documents are gone.
	DeleteReceivedActivitiesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteRemoteFollowTombstonesBefore deletes the tombstones of unknown Follows received
	// before t.
	DeleteRemoteFollowTombstonesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteRemoteObjectTombstonesBefore deletes the tombstones received before t.
	DeleteRemoteObjectTombstonesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteOutboundDocumentsBefore deletes the outbound documents built before t.
	DeleteOutboundDocumentsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteDeliveryAttemptsBefore deletes the delivery attempts made before t.
	DeleteDeliveryAttemptsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteSupersededStatusCreates deletes the Creates of statuses whose Delete, or the Delete of
	// whose actor (not the service closure's), was recorded before t, with their deliveries.
	DeleteSupersededStatusCreates(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteExpiredOutboundActivities deletes the activities recorded before t with their
	// documents, deliveries, attempts, and fan-out completions, except a status's Create, the
	// service closure's Deletes, and a Delete that a kept Create depends on.
	DeleteExpiredOutboundActivities(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteNodeInfoObservationsBefore deletes the NodeInfo observations made before t, except the
	// newest of each server.
	DeleteNodeInfoObservationsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteAvailabilityEventsBefore deletes the availability events that occurred before t, except
	// the newest of each server.
	DeleteAvailabilityEventsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
}

// PurgeDeps are the ports of the purge.
type PurgeDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository PurgeRepository
	Clock      kernel.Clock
	// Batch is the number of rows per statement; zero means PurgeBatch.
	Batch int
}

// Purger runs JobPurge: it deletes received documents after ReceivedDocumentRetention, the
// records of received activities and the tombstones of unknown Follows and objects after 30 days,
// the bodies of outbound activities and the delivery attempts after 30 days, the outbound
// activities with their deliveries after OutboundActivityRetention, and the observations of servers
// after InstanceObservationRetention. Each statement deletes a bounded batch in a transaction of its own; a document always goes before the record
// of its activity, and a Follow tombstone before the object tombstones, whose row with its
// activity_uri marks it used: the marker is never older than the tombstone it marks, so a used
// tombstone never outlives its marker and drops a Follow again.
type Purger struct{ d PurgeDeps }

// NewPurger returns the purge on d.
func NewPurger(d PurgeDeps) *Purger {
	if d.Batch <= 0 {
		d.Batch = PurgeBatch
	}
	return &Purger{d: d}
}

// RegisterJobs registers the JobPurge handler and its hourly schedule.
func (p *Purger) RegisterJobs(h *queue.JobHandlerRegistry, s *queue.Scheduler) {
	h.Register(JobPurge, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := p.Run(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	s.Add(queue.Schedule{Name: JobPurge, Every: PurgeInterval, Job: queue.NewJob{Type: JobPurge}})
}

// Run deletes the records past their retention, at most purgeMaxBatches batches per table, and
// returns how many rows it deleted. It is safe to repeat.
func (p *Purger) Run(ctx context.Context) (int64, error) {
	now := p.d.Clock.Now()
	r := p.d.Repository
	steps := []struct {
		name   string
		before time.Time
		del    func(context.Context, uow.Tx, time.Time, int) (int64, error)
		batch  int
	}{
		{"received documents", now.Add(-ReceivedDocumentRetention), r.DeleteReceivedDocumentsBefore, 0},
		{"received activities", now.Add(-ReceivedActivityRetention), r.DeleteReceivedActivitiesBefore, 0},
		{"received activity answers", now.Add(-ReceivedActivityRetention), r.DeleteReceivedActivityAnswersBefore, 0},
		{"remote follow tombstones", now.Add(-RemoteFollowTombstoneRetention), r.DeleteRemoteFollowTombstonesBefore, 0},
		{"remote object tombstones", now.Add(-RemoteObjectTombstoneRetention), r.DeleteRemoteObjectTombstonesBefore, 0},
		{"outbound documents", now.Add(-OutboundDocumentRetention), r.DeleteOutboundDocumentsBefore, 0},
		{"delivery attempts", now.Add(-DeliveryAttemptRetention), r.DeleteDeliveryAttemptsBefore, 0},
		// The Creates go first: a Delete stays as long as a Create depends on it.
		{"superseded status creates", now.Add(-OutboundActivityRetention), r.DeleteSupersededStatusCreates, outboundPurgeBatch},
		{"outbound activities", now.Add(-OutboundActivityRetention), r.DeleteExpiredOutboundActivities, outboundPurgeBatch},
		{"nodeinfo observations", now.Add(-InstanceObservationRetention), r.DeleteNodeInfoObservationsBefore, 0},
		{"availability events", now.Add(-InstanceObservationRetention), r.DeleteAvailabilityEventsBefore, 0},
	}
	var total int64
	for _, s := range steps {
		batch := p.d.Batch
		if s.batch > 0 && s.batch < batch {
			batch = s.batch
		}
		for range purgeMaxBatches {
			var n int64
			err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				n, err = s.del(ctx, tx, s.before, batch)
				return err
			})
			if err != nil {
				return total, fmt.Errorf("federation: purge %s: %w", s.name, err)
			}
			total += n
			if n < int64(batch) {
				break
			}
		}
	}
	return total, nil
}
