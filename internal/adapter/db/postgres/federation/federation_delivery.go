// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the delivery attempts and the health of remote servers (federation.md §7.3).

// InsertDeliveryAttempt implements federation.Repository.
func (r *Repository) InsertDeliveryAttempt(ctx context.Context, tx uow.Tx, a federation.DeliveryAttempt) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var status *int32
	if a.HTTPStatus != 0 {
		s, err := postgres.ToInt32(a.HTTPStatus, "http status")
		if err != nil {
			return err
		}
		status = &s
	}
	err = q.InsertDeliveryAttempt(ctx, federationgen.InsertDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome), SignatureScheme: postgres.ToNullString(a.Scheme),
		StatusCode: status, Error: postgres.ToNullString(a.Error), AttemptedAt: a.AttemptedAt,
	})
	return postgres.Wrap(err, "insert delivery attempt")
}

// LockInstanceHealth implements federation.Repository with SELECT ... FOR UPDATE.
func (r *Repository) LockInstanceHealth(ctx context.Context, tx uow.Tx, domain string) (federation.InstanceHealth, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.InstanceHealth{}, err
	}
	row, err := q.LockInstanceHealth(ctx, domain)
	if err != nil {
		return federation.InstanceHealth{}, postgres.NotFound(err, appfederation.ErrNotFound, "instance health of "+domain)
	}
	return instanceHealth(row), nil
}

// InsertAvailabilityEvent implements federation.Repository.
func (r *Repository) InsertAvailabilityEvent(ctx context.Context, tx uow.Tx, e federation.AvailabilityEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceAvailabilityEvent(ctx, federationgen.InsertInstanceAvailabilityEventParams{
		ID: e.ID, Domain: e.Domain, Kind: string(e.Kind), Reason: string(e.Reason), OccurredAt: e.OccurredAt,
	})
	return postgres.Wrap(err, "insert instance availability event")
}

// FindDelivery implements federation.Repository.
func (r *Repository) FindDelivery(ctx context.Context, id string) (federation.Delivery, error) {
	row, err := r.db.reader().GetDelivery(ctx, id)
	if err != nil {
		return federation.Delivery{}, postgres.NotFound(err, appfederation.ErrNotFound, "delivery "+id)
	}
	return federation.Delivery{
		ID: row.ID, OutboundActivityID: row.OutboundActivityID, InboxURL: federation.IRI(row.InboxUrl),
		Domain: row.Domain, CreatedAt: postgres.UTC(row.CreatedAt),
	}, nil
}

// FindDeliveryContext implements federation.Repository.
func (r *Repository) FindDeliveryContext(ctx context.Context, id string) (appfederation.DeliveryContext, error) {
	row, err := r.db.reader().GetDeliveryContext(ctx, id)
	if err != nil {
		return appfederation.DeliveryContext{}, postgres.NotFound(err, appfederation.ErrNotFound, "delivery "+id)
	}
	out := appfederation.DeliveryContext{
		Delivery: federation.Delivery{
			ID: row.ID, OutboundActivityID: row.OutboundActivityID, InboxURL: federation.IRI(row.InboxUrl),
			Domain: row.Domain, CreatedAt: postgres.UTC(row.CreatedAt),
		},
		Activity: federation.OutboundActivity{
			ID: row.OutboundActivityID, ActivityURI: federation.IRI(row.ActivityUri), Type: federation.ActivityType(row.Type),
			ActorAccountID: row.ActorAccountID, ObjectKind: federation.OutboundObjectKind(row.ObjectType),
			ObjectID: row.ObjectID, CreatedAt: postgres.UTC(row.ActivityCreatedAt),
		},
		Document: row.Document, Health: federation.InstanceAvailable,
	}
	if row.HealthState != nil {
		out.Health, out.HealthRow = federation.InstanceState(*row.HealthState), true
	}
	return out, nil
}

// ListDeliveryAttempts implements federation.Repository.
func (r *Repository) ListDeliveryAttempts(ctx context.Context, deliveryID string) ([]federation.DeliveryAttempt, error) {
	rows, err := r.db.reader().ListDeliveryAttempts(ctx, deliveryID)
	if err != nil {
		return nil, postgres.Wrap(err, "list delivery attempts")
	}
	out := make([]federation.DeliveryAttempt, len(rows))
	for i, row := range rows {
		status := 0
		if row.StatusCode != nil {
			status = int(*row.StatusCode)
		}
		out[i] = federation.DeliveryAttempt{
			ID: row.ID, DeliveryID: row.DeliveryID, Outcome: federation.DeliveryOutcome(row.Outcome),
			Scheme: postgres.FromNullString(row.SignatureScheme), HTTPStatus: status, Error: postgres.FromNullString(row.Error),
			AttemptedAt: postgres.UTC(row.AttemptedAt),
		}
	}
	return out, nil
}

// FindInstanceHealth implements federation.Repository.
func (r *Repository) FindInstanceHealth(ctx context.Context, domain string) (federation.InstanceHealth, error) {
	row, err := r.db.reader().GetInstanceHealth(ctx, domain)
	if err != nil {
		return federation.InstanceHealth{}, postgres.NotFound(err, appfederation.ErrNotFound, "instance health of "+domain)
	}
	return instanceHealth(row), nil
}

// ListAvailabilityEvents implements federation.Repository.
func (r *Repository) ListAvailabilityEvents(ctx context.Context, domain, before string, limit int) ([]federation.AvailabilityEvent, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInstanceAvailabilityEvents(ctx, federationgen.ListInstanceAvailabilityEventsParams{
		Domain: domain, Before: postgres.ToNullString(before), BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list instance availability events")
	}
	out := make([]federation.AvailabilityEvent, len(rows))
	for i, row := range rows {
		out[i] = federation.AvailabilityEvent{
			ID: row.ID, Domain: row.Domain, Kind: federation.AvailabilityKind(row.Kind),
			Reason: federation.AvailabilityReason(row.Reason), OccurredAt: postgres.UTC(row.OccurredAt),
		}
	}
	return out, nil
}

// ListUnavailableInstances implements federation.Repository.
func (r *Repository) ListUnavailableInstances(ctx context.Context, after string, limit int) ([]federation.InstanceHealth, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListUnavailableInstances(ctx, federationgen.ListUnavailableInstancesParams{
		AfterDomain: after, BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list unavailable instances")
	}
	out := make([]federation.InstanceHealth, len(rows))
	for i, row := range rows {
		out[i] = instanceHealth(row)
	}
	return out, nil
}

// ListInstanceHealth implements federation.Repository.
func (r *Repository) ListInstanceHealth(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.InstanceHealth, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInstanceHealth(ctx, federationgen.ListInstanceHealthParams{
		State: string(state), AfterDomain: after, BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list instance health")
	}
	out := make([]federation.InstanceHealth, len(rows))
	for i, row := range rows {
		out[i] = instanceHealth(row)
	}
	return out, nil
}

// CountInstanceHealth implements federation.Repository.
func (r *Repository) CountInstanceHealth(ctx context.Context) (map[federation.InstanceState]int64, error) {
	rows, err := r.db.reader().CountInstanceHealth(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "count instance health")
	}
	out := make(map[federation.InstanceState]int64, len(rows))
	for _, row := range rows {
		out[federation.InstanceState(row.State)] = row.Servers
	}
	return out, nil
}

func instanceHealth(row federationgen.InstanceHealth) federation.InstanceHealth {
	return federation.InstanceHealth{
		Domain: row.Domain, State: federation.InstanceState(row.State), Since: postgres.UTC(row.Since),
		FailureDays: int(row.FailureDays), LastFailureDay: row.LastFailureDay,
	}
}
