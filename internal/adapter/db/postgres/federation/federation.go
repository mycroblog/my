// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Repository implements federation.Repository on PostgreSQL.
type Repository struct {
	db *moduleDB
}

var _ appfederation.Repository = (*Repository)(nil)

// NewRepository returns the federation module's repository on d.
func NewRepository(d *postgres.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertOutboundActivity implements federation.Repository.
func (r *Repository) InsertOutboundActivity(ctx context.Context, tx uow.Tx, a federation.OutboundActivity) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOutboundActivity(ctx, federationgen.InsertOutboundActivityParams{
		ID: a.ID, ActivityUri: string(a.ActivityURI), Type: string(a.Type), ActorAccountID: a.ActorAccountID,
		ObjectType: string(a.ObjectKind), ObjectID: a.ObjectID, CreatedAt: a.CreatedAt,
	})
	return postgres.Wrap(err, "insert outbound activity")
}

// InsertOutboundDocument implements federation.Repository.
func (r *Repository) InsertOutboundDocument(ctx context.Context, tx uow.Tx, outboundActivityID string, document []byte, builtAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOutboundActivityDocument(ctx, federationgen.InsertOutboundActivityDocumentParams{
		OutboundActivityID: outboundActivityID, Document: document, BuiltAt: builtAt,
	})
	return n == 1, postgres.Wrap(err, "insert outbound activity document")
}

// InsertDelivery implements federation.Repository.
func (r *Repository) InsertDelivery(ctx context.Context, tx uow.Tx, d federation.Delivery) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertDelivery(ctx, federationgen.InsertDeliveryParams{
		ID: d.ID, OutboundActivityID: d.OutboundActivityID, InboxUrl: string(d.InboxURL), Domain: d.Domain,
		CreatedAt: d.CreatedAt,
	})
	return n == 1, postgres.Wrap(err, "insert delivery")
}

// InsertDeliveries implements federation.Repository.
func (r *Repository) InsertDeliveries(ctx context.Context, tx uow.Tx, ds []federation.Delivery) (map[string]bool, error) {
	out := make(map[string]bool, len(ds))
	if len(ds) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(ds))
	activities := make([]string, len(ds))
	inboxes := make([]string, len(ds))
	domains := make([]string, len(ds))
	at := make([]time.Time, len(ds))
	for i, d := range ds {
		ids[i], activities[i], inboxes[i], domains[i], at[i] = d.ID, d.OutboundActivityID, string(d.InboxURL), d.Domain, d.CreatedAt
	}
	inserted, err := q.InsertDeliveries(ctx, federationgen.InsertDeliveriesParams{
		Ids: ids, OutboundActivityIds: activities, InboxUrls: inboxes, Domains: domains, CreatedAts: at,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "insert deliveries")
	}
	for _, id := range inserted {
		out[id] = true
	}
	return out, nil
}

// UpsertInstanceHealth implements federation.Repository.
func (r *Repository) UpsertInstanceHealth(ctx context.Context, tx uow.Tx, h federation.InstanceHealth) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	days, err := postgres.ToInt32(h.FailureDays, "failure days")
	if err != nil {
		return err
	}
	err = q.UpsertInstanceHealth(ctx, federationgen.UpsertInstanceHealthParams{
		Domain: h.Domain, State: string(h.State), Since: h.Since, FailureDays: days, LastFailureDay: h.LastFailureDay,
	})
	return postgres.Wrap(err, "upsert instance health")
}

// FindOutboundActivity implements federation.Repository.
func (r *Repository) FindOutboundActivity(ctx context.Context, id string) (federation.OutboundActivity, error) {
	row, err := r.db.reader().GetOutboundActivity(ctx, id)
	if err != nil {
		return federation.OutboundActivity{}, postgres.NotFound(err, appfederation.ErrNotFound, "outbound activity "+id)
	}
	return outboundActivity(row), nil
}

// ListOutboundActivitiesOf implements federation.Repository.
func (r *Repository) ListOutboundActivitiesOf(ctx context.Context, kind federation.OutboundObjectKind, objectID string) ([]federation.OutboundActivity, error) {
	rows, err := r.db.reader().ListOutboundActivitiesOfObject(ctx, federationgen.ListOutboundActivitiesOfObjectParams{
		ObjectType: string(kind), ObjectID: objectID,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list outbound activities")
	}
	out := make([]federation.OutboundActivity, len(rows))
	for i, row := range rows {
		out[i] = outboundActivity(row)
	}
	return out, nil
}

func outboundActivity(row federationgen.OutboundActivity) federation.OutboundActivity {
	return federation.OutboundActivity{
		ID: row.ID, ActivityURI: federation.IRI(row.ActivityUri), Type: federation.ActivityType(row.Type),
		ActorAccountID: row.ActorAccountID, ObjectKind: federation.OutboundObjectKind(row.ObjectType),
		ObjectID: row.ObjectID, CreatedAt: postgres.UTC(row.CreatedAt),
	}
}

// FindOutboundDocument implements federation.Repository.
func (r *Repository) FindOutboundDocument(ctx context.Context, outboundActivityID string) ([]byte, error) {
	doc, err := r.db.reader().GetOutboundActivityDocument(ctx, outboundActivityID)
	if err != nil {
		return nil, postgres.NotFound(err, appfederation.ErrNotFound, "document of outbound activity "+outboundActivityID)
	}
	return doc, nil
}

// ListDeliveries implements federation.Repository.
func (r *Repository) ListDeliveries(ctx context.Context, outboundActivityID, after string, limit int) ([]federation.Delivery, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDeliveriesOfOutboundActivity(ctx, federationgen.ListDeliveriesOfOutboundActivityParams{
		OutboundActivityID: outboundActivityID, AfterID: postgres.ToNullString(after), BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list deliveries")
	}
	out := make([]federation.Delivery, len(rows))
	for i, row := range rows {
		out[i] = federation.Delivery{
			ID: row.ID, OutboundActivityID: row.OutboundActivityID, InboxURL: federation.IRI(row.InboxUrl),
			Domain: row.Domain, CreatedAt: postgres.UTC(row.CreatedAt),
		}
	}
	return out, nil
}

// InstanceState implements federation.Repository.
func (r *Repository) InstanceState(ctx context.Context, domain string) (federation.InstanceState, error) {
	row, err := r.db.reader().GetInstanceHealth(ctx, domain)
	if errors.Is(err, pgx.ErrNoRows) {
		return federation.InstanceAvailable, nil
	}
	if err != nil {
		return "", postgres.Wrap(err, "get instance health")
	}
	return federation.InstanceState(row.State), nil
}

// InstanceStatesOf implements appfederation.Repository.
func (r *Repository) InstanceStatesOf(ctx context.Context, domains []string) (map[string]federation.InstanceState, error) {
	out := make(map[string]federation.InstanceState, len(domains))
	if len(domains) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListInstanceHealthOf(ctx, domains)
	if err != nil {
		return nil, postgres.Wrap(err, "list instance health")
	}
	for _, d := range domains {
		out[d] = federation.InstanceAvailable
	}
	for _, row := range rows {
		out[row.Domain] = federation.InstanceState(row.State)
	}
	return out, nil
}
