// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"database/sql"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/federationgen"
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
	status := sql.NullInt64{Int64: int64(a.HTTPStatus), Valid: a.HTTPStatus != 0}
	err = q.InsertDeliveryAttempt(ctx, federationgen.InsertDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome), SignatureScheme: sqlite.ToNullString(a.Scheme),
		StatusCode: status, Error: sqlite.ToNullString(a.Error), AttemptedAt: sqlite.ToMillis(a.AttemptedAt),
	})
	return sqlite.Wrap(err, "insert delivery attempt")
}

// LockInstanceHealth implements federation.Repository. SQLite serializes writers, so the read
// needs no lock.
func (r *Repository) LockInstanceHealth(ctx context.Context, tx uow.Tx, domain string) (federation.InstanceHealth, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.InstanceHealth{}, err
	}
	row, err := q.LockInstanceHealth(ctx, domain)
	if err != nil {
		return federation.InstanceHealth{}, sqlite.NotFound(err, appfederation.ErrNotFound, "instance health of "+domain)
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
		ID: e.ID, Domain: e.Domain, Kind: string(e.Kind), Reason: string(e.Reason), OccurredAt: sqlite.ToMillis(e.OccurredAt),
	})
	return sqlite.Wrap(err, "insert instance availability event")
}

// FindDelivery implements federation.Repository.
func (r *Repository) FindDelivery(ctx context.Context, id string) (federation.Delivery, error) {
	row, err := r.db.reader().GetDelivery(ctx, id)
	if err != nil {
		return federation.Delivery{}, sqlite.NotFound(err, appfederation.ErrNotFound, "delivery "+id)
	}
	return federation.Delivery{
		ID: row.ID, OutboundActivityID: row.OutboundActivityID, InboxURL: federation.IRI(row.InboxUrl),
		Domain: row.Domain, CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}, nil
}

// ListDeliveryAttempts implements federation.Repository.
func (r *Repository) ListDeliveryAttempts(ctx context.Context, deliveryID string) ([]federation.DeliveryAttempt, error) {
	rows, err := r.db.reader().ListDeliveryAttempts(ctx, deliveryID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list delivery attempts")
	}
	out := make([]federation.DeliveryAttempt, len(rows))
	for i, row := range rows {
		out[i] = federation.DeliveryAttempt{
			ID: row.ID, DeliveryID: row.DeliveryID, Outcome: federation.DeliveryOutcome(row.Outcome),
			Scheme: row.SignatureScheme.String, HTTPStatus: int(row.StatusCode.Int64), Error: row.Error.String,
			AttemptedAt: sqlite.FromMillis(row.AttemptedAt),
		}
	}
	return out, nil
}

// FindInstanceHealth implements federation.Repository.
func (r *Repository) FindInstanceHealth(ctx context.Context, domain string) (federation.InstanceHealth, error) {
	row, err := r.db.reader().GetInstanceHealth(ctx, domain)
	if err != nil {
		return federation.InstanceHealth{}, sqlite.NotFound(err, appfederation.ErrNotFound, "instance health of "+domain)
	}
	return instanceHealth(row), nil
}

// ListAvailabilityEvents implements federation.Repository.
func (r *Repository) ListAvailabilityEvents(ctx context.Context, domain, before string, limit int) ([]federation.AvailabilityEvent, error) {
	rows, err := r.db.reader().ListInstanceAvailabilityEvents(ctx, federationgen.ListInstanceAvailabilityEventsParams{
		Domain: domain, Before: before, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance availability events")
	}
	out := make([]federation.AvailabilityEvent, len(rows))
	for i, row := range rows {
		out[i] = federation.AvailabilityEvent{
			ID: row.ID, Domain: row.Domain, Kind: federation.AvailabilityKind(row.Kind),
			Reason: federation.AvailabilityReason(row.Reason), OccurredAt: sqlite.FromMillis(row.OccurredAt),
		}
	}
	return out, nil
}

// ListUnavailableInstances implements federation.Repository.
func (r *Repository) ListUnavailableInstances(ctx context.Context, after string, limit int) ([]federation.InstanceHealth, error) {
	rows, err := r.db.reader().ListUnavailableInstances(ctx, federationgen.ListUnavailableInstancesParams{
		AfterDomain: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list unavailable instances")
	}
	out := make([]federation.InstanceHealth, len(rows))
	for i, row := range rows {
		out[i] = instanceHealth(row)
	}
	return out, nil
}

// ListInstanceHealth implements federation.Repository.
func (r *Repository) ListInstanceHealth(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.InstanceHealth, error) {
	rows, err := r.db.reader().ListInstanceHealth(ctx, federationgen.ListInstanceHealthParams{
		State: string(state), AfterDomain: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance health")
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
		return nil, sqlite.Wrap(err, "count instance health")
	}
	out := make(map[federation.InstanceState]int64, len(rows))
	for _, row := range rows {
		out[federation.InstanceState(row.State)] = row.Servers
	}
	return out, nil
}

func instanceHealth(row federationgen.InstanceHealth) federation.InstanceHealth {
	return federation.InstanceHealth{
		Domain: row.Domain, State: federation.InstanceState(row.State), Since: sqlite.FromMillis(row.Since),
		FailureDays: int(row.FailureDays), LastFailureDay: row.LastFailureDay,
	}
}
