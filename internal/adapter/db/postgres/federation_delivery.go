// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the delivery attempts and the health of remote servers (federation.md §7.3).

// InsertDeliveryAttempt implements federation.Repository.
func (r *FederationRepository) InsertDeliveryAttempt(ctx context.Context, tx uow.Tx, a federation.DeliveryAttempt) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var status *int32
	if a.HTTPStatus != 0 {
		s, err := toInt32(a.HTTPStatus, "http status")
		if err != nil {
			return err
		}
		status = &s
	}
	err = q.InsertDeliveryAttempt(ctx, postgresgen.InsertDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome), SignatureScheme: toNullString(a.Scheme),
		HttpStatus: status, Error: toNullString(a.Error), AttemptedAt: a.AttemptedAt,
	})
	return wrap(err, "insert delivery attempt")
}

// LockInstanceHealth implements federation.Repository with SELECT ... FOR UPDATE.
func (r *FederationRepository) LockInstanceHealth(ctx context.Context, tx uow.Tx, domain string) (federation.InstanceHealth, error) {
	q, err := r.db.queries(tx)
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
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceAvailabilityEvent(ctx, postgresgen.InsertInstanceAvailabilityEventParams{
		ID: e.ID, Domain: e.Domain, Kind: string(e.Kind), Reason: string(e.Reason), OccurredAt: e.OccurredAt,
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
		Domain: row.Domain, CreatedAt: utc(row.CreatedAt),
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
		status := 0
		if row.HttpStatus != nil {
			status = int(*row.HttpStatus)
		}
		out[i] = federation.DeliveryAttempt{
			ID: row.ID, DeliveryID: row.DeliveryID, Outcome: federation.DeliveryOutcome(row.Outcome),
			Scheme: fromNullString(row.SignatureScheme), HTTPStatus: status, Error: fromNullString(row.Error),
			AttemptedAt: utc(row.AttemptedAt),
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
	n, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInstanceAvailabilityEvents(ctx, postgresgen.ListInstanceAvailabilityEventsParams{
		Domain: domain, Before: toNullString(before), BatchLimit: n,
	})
	if err != nil {
		return nil, wrap(err, "list instance availability events")
	}
	out := make([]federation.AvailabilityEvent, len(rows))
	for i, row := range rows {
		out[i] = federation.AvailabilityEvent{
			ID: row.ID, Domain: row.Domain, Kind: federation.AvailabilityKind(row.Kind),
			Reason: federation.AvailabilityReason(row.Reason), OccurredAt: utc(row.OccurredAt),
		}
	}
	return out, nil
}

// ListUnavailableInstances implements federation.Repository.
func (r *FederationRepository) ListUnavailableInstances(ctx context.Context, after string, limit int) ([]federation.InstanceHealth, error) {
	n, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListUnavailableInstances(ctx, postgresgen.ListUnavailableInstancesParams{
		AfterDomain: after, BatchLimit: n,
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
	n, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInstanceHealth(ctx, postgresgen.ListInstanceHealthParams{
		State: string(state), AfterDomain: after, BatchLimit: n,
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

func instanceHealth(row postgresgen.InstanceHealth) federation.InstanceHealth {
	return federation.InstanceHealth{
		Domain: row.Domain, State: federation.InstanceState(row.State), Since: utc(row.Since),
		FailureDays: int(row.FailureDays), LastFailureDay: row.LastFailureDay,
	}
}
