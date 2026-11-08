// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ appfederation.PurgeRepository = (*Repository)(nil)

// DeleteReceivedDocumentsBefore implements federation.PurgeRepository.
func (r *Repository) DeleteReceivedDocumentsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteReceivedActivityDocumentsBefore(ctx, federationgen.DeleteReceivedActivityDocumentsBeforeParams{
		ReceivedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete received documents")
}

// DeleteReceivedActivitiesBefore implements federation.PurgeRepository.
func (r *Repository) DeleteReceivedActivitiesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteReceivedActivitiesBefore(ctx, federationgen.DeleteReceivedActivitiesBeforeParams{
		ReceivedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete received activities")
}

// DeleteRemoteFollowTombstonesBefore implements federation.PurgeRepository.
func (r *Repository) DeleteRemoteFollowTombstonesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteRemoteFollowTombstonesBefore(ctx, federationgen.DeleteRemoteFollowTombstonesBeforeParams{
		ReceivedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete remote follow tombstones")
}

// DeleteRemoteObjectTombstonesBefore implements federation.PurgeRepository.
func (r *Repository) DeleteRemoteObjectTombstonesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteRemoteObjectTombstonesBefore(ctx, federationgen.DeleteRemoteObjectTombstonesBeforeParams{
		ReceivedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete remote object tombstones")
}

// DeleteOutboundDocumentsBefore implements federation.PurgeRepository.
func (r *Repository) DeleteOutboundDocumentsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteOutboundActivityDocumentsBefore(ctx, federationgen.DeleteOutboundActivityDocumentsBeforeParams{
		IDBefore: kernel.UUIDv7Floor(t.Add(time.Millisecond)), BuiltBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete outbound documents")
}

// DeleteDeliveryAttemptsBefore implements federation.PurgeRepository.
func (r *Repository) DeleteDeliveryAttemptsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteDeliveryAttemptsBefore(ctx, federationgen.DeleteDeliveryAttemptsBeforeParams{
		IDBefore: kernel.UUIDv7Floor(t.Add(time.Millisecond)), AttemptedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete delivery attempts")
}

// DeleteSupersededStatusCreates implements federation.PurgeRepository.
func (r *Repository) DeleteSupersededStatusCreates(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	ids, err := q.ListSupersededStatusCreates(ctx, federationgen.ListSupersededStatusCreatesParams{CreatedBefore: t, BatchLimit: batch})
	if err != nil {
		return 0, postgres.Wrap(err, "list superseded status creates")
	}
	return int64(len(ids)), deleteOutboundActivities(ctx, q, ids)
}

// DeleteExpiredOutboundActivities implements federation.PurgeRepository.
func (r *Repository) DeleteExpiredOutboundActivities(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredOutboundActivities(ctx, federationgen.ListExpiredOutboundActivitiesParams{CreatedBefore: t, BatchLimit: batch})
	if err != nil {
		return 0, postgres.Wrap(err, "list expired outbound activities")
	}
	return int64(len(ids)), deleteOutboundActivities(ctx, q, ids)
}

// deleteOutboundActivities deletes activities with their children, children first.
func deleteOutboundActivities(ctx context.Context, q *federationgen.Queries, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return postgres.RunSteps(ctx, ids, "delete outbound activities",
		q.DeleteAttemptsOfOutboundActivities, q.DeleteDeliveriesOfOutboundActivities,
		q.DeleteDocumentsOfOutboundActivities, q.DeleteCompletionsOfOutboundActivities, q.DeleteOutboundActivities)
}

// DeleteNodeInfoObservationsBefore implements federation.PurgeRepository.
func (r *Repository) DeleteNodeInfoObservationsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteNodeInfoObservationsBefore(ctx, federationgen.DeleteNodeInfoObservationsBeforeParams{
		IDBefore: kernel.UUIDv7Floor(t.Add(time.Millisecond)), ObservedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete nodeinfo observations")
}

// DeleteAvailabilityEventsBefore implements federation.PurgeRepository.
func (r *Repository) DeleteAvailabilityEventsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteAvailabilityEventsBefore(ctx, federationgen.DeleteAvailabilityEventsBeforeParams{
		IDBefore: kernel.UUIDv7Floor(t.Add(time.Millisecond)), OccurredBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete availability events")
}
