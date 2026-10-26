// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the delivery attempts and the health of remote servers (federation.md §7.3).

// InsertDeliveryAttempt implements federation.Repository.
func (r *FederationRepository) InsertDeliveryAttempt(ctx context.Context, tx uow.Tx, a federation.DeliveryAttempt) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	status := sql.NullInt64{Int64: int64(a.HTTPStatus), Valid: a.HTTPStatus != 0}
	err = q.InsertDeliveryAttempt(ctx, sqlitegen.InsertDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome), SignatureScheme: toNullString(a.Scheme),
		HttpStatus: status, Error: toNullString(a.Error), AttemptedAt: toMillis(a.AttemptedAt),
	})
	return wrap(err, "insert delivery attempt")
}

// LockInstanceHealth implements federation.Repository. SQLite serializes writers, so the read
// needs no lock.
func (r *FederationRepository) LockInstanceHealth(ctx context.Context, tx uow.Tx, domain string) (federation.InstanceHealth, error) {
	q, err := queries(tx)
	if err != nil {
		return federation.InstanceHealth{}, err
	}
	row, err := q.LockInstanceHealth(ctx, domain)
	if err != nil {
		return federation.InstanceHealth{}, notFound(err, appfederation.ErrNotFound, "instance health of "+domain)
	}
	return instanceHealth(row), nil
}

// InsertAvailabilityEvent implements federation.Repository.
func (r *FederationRepository) InsertAvailabilityEvent(ctx context.Context, tx uow.Tx, e federation.AvailabilityEvent) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceAvailabilityEvent(ctx, sqlitegen.InsertInstanceAvailabilityEventParams{
		ID: e.ID, Domain: e.Domain, Kind: string(e.Kind), Reason: string(e.Reason), OccurredAt: toMillis(e.OccurredAt),
	})
	return wrap(err, "insert instance availability event")
}

// FindDelivery implements federation.Repository.
func (r *FederationRepository) FindDelivery(ctx context.Context, id string) (federation.Delivery, error) {
	row, err := r.db.reader().GetDelivery(ctx, id)
	if err != nil {
		return federation.Delivery{}, notFound(err, appfederation.ErrNotFound, "delivery "+id)
	}
	return federation.Delivery{
		ID: row.ID, OutboundActivityID: row.OutboundActivityID, InboxURL: federation.IRI(row.InboxUrl),
		Domain: row.Domain, CreatedAt: fromMillis(row.CreatedAt),
	}, nil
}

// ListDeliveryAttempts implements federation.Repository.
func (r *FederationRepository) ListDeliveryAttempts(ctx context.Context, deliveryID string) ([]federation.DeliveryAttempt, error) {
	rows, err := r.db.reader().ListDeliveryAttempts(ctx, deliveryID)
	if err != nil {
		return nil, wrap(err, "list delivery attempts")
	}
	out := make([]federation.DeliveryAttempt, len(rows))
	for i, row := range rows {
		out[i] = federation.DeliveryAttempt{
			ID: row.ID, DeliveryID: row.DeliveryID, Outcome: federation.DeliveryOutcome(row.Outcome),
			Scheme: row.SignatureScheme.String, HTTPStatus: int(row.HttpStatus.Int64), Error: row.Error.String,
			AttemptedAt: fromMillis(row.AttemptedAt),
		}
	}
	return out, nil
}

// FindInstanceHealth implements federation.Repository.
func (r *FederationRepository) FindInstanceHealth(ctx context.Context, domain string) (federation.InstanceHealth, error) {
	row, err := r.db.reader().GetInstanceHealth(ctx, domain)
	if err != nil {
		return federation.InstanceHealth{}, notFound(err, appfederation.ErrNotFound, "instance health of "+domain)
	}
	return instanceHealth(row), nil
}

// ListAvailabilityEvents implements federation.Repository.
func (r *FederationRepository) ListAvailabilityEvents(ctx context.Context, domain, before string, limit int) ([]federation.AvailabilityEvent, error) {
	rows, err := r.db.reader().ListInstanceAvailabilityEvents(ctx, sqlitegen.ListInstanceAvailabilityEventsParams{
		Domain: domain, Before: before, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list instance availability events")
	}
	out := make([]federation.AvailabilityEvent, len(rows))
	for i, row := range rows {
		out[i] = federation.AvailabilityEvent{
			ID: row.ID, Domain: row.Domain, Kind: federation.AvailabilityKind(row.Kind),
			Reason: federation.AvailabilityReason(row.Reason), OccurredAt: fromMillis(row.OccurredAt),
		}
	}
	return out, nil
}

// ListUnavailableInstances implements federation.Repository.
func (r *FederationRepository) ListUnavailableInstances(ctx context.Context, after string, limit int) ([]federation.InstanceHealth, error) {
	rows, err := r.db.reader().ListUnavailableInstances(ctx, sqlitegen.ListUnavailableInstancesParams{
		AfterDomain: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list unavailable instances")
	}
	out := make([]federation.InstanceHealth, len(rows))
	for i, row := range rows {
		out[i] = instanceHealth(row)
	}
	return out, nil
}

// ListInstanceHealth implements federation.Repository.
func (r *FederationRepository) ListInstanceHealth(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.InstanceHealth, error) {
	rows, err := r.db.reader().ListInstanceHealth(ctx, sqlitegen.ListInstanceHealthParams{
		State: string(state), AfterDomain: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list instance health")
	}
	out := make([]federation.InstanceHealth, len(rows))
	for i, row := range rows {
		out[i] = instanceHealth(row)
	}
	return out, nil
}

// CountInstanceHealth implements federation.Repository.
func (r *FederationRepository) CountInstanceHealth(ctx context.Context) (map[federation.InstanceState]int64, error) {
	rows, err := r.db.reader().CountInstanceHealth(ctx)
	if err != nil {
		return nil, wrap(err, "count instance health")
	}
	out := make(map[federation.InstanceState]int64, len(rows))
	for _, row := range rows {
		out[federation.InstanceState(row.State)] = row.Servers
	}
	return out, nil
}

func instanceHealth(row sqlitegen.InstanceHealth) federation.InstanceHealth {
	return federation.InstanceHealth{
		Domain: row.Domain, State: federation.InstanceState(row.State), Since: fromMillis(row.Since),
		FailureDays: int(row.FailureDays), LastFailureDay: row.LastFailureDay,
	}
}
