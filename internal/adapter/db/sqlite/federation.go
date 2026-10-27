// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// FederationRepository implements federation.Repository on SQLite.
type FederationRepository struct {
	db *DB
}

var _ appfederation.Repository = (*FederationRepository)(nil)

// FederationRepository returns the federation module's repository on d.
func (d *DB) FederationRepository() *FederationRepository { return &FederationRepository{db: d} }

// InsertOutboundActivity implements federation.Repository.
func (r *FederationRepository) InsertOutboundActivity(ctx context.Context, tx uow.Tx, a federation.OutboundActivity) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOutboundActivity(ctx, sqlitegen.InsertOutboundActivityParams{
		ID: a.ID, ActivityUri: string(a.ActivityURI), Type: string(a.Type), ActorAccountID: a.ActorAccountID,
		ObjectType: string(a.ObjectKind), ObjectID: a.ObjectID, CreatedAt: toMillis(a.CreatedAt),
	})
	return wrap(err, "insert outbound activity")
}

// InsertOutboundDocument implements federation.Repository.
func (r *FederationRepository) InsertOutboundDocument(ctx context.Context, tx uow.Tx, outboundActivityID string, document []byte, builtAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOutboundActivityDocument(ctx, sqlitegen.InsertOutboundActivityDocumentParams{
		OutboundActivityID: outboundActivityID, Document: document, BuiltAt: toMillis(builtAt),
	})
	return n == 1, wrap(err, "insert outbound activity document")
}

// InsertDelivery implements federation.Repository.
func (r *FederationRepository) InsertDelivery(ctx context.Context, tx uow.Tx, d federation.Delivery) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertDelivery(ctx, sqlitegen.InsertDeliveryParams{
		ID: d.ID, OutboundActivityID: d.OutboundActivityID, InboxUrl: string(d.InboxURL), Domain: d.Domain,
		CreatedAt: toMillis(d.CreatedAt),
	})
	return n == 1, wrap(err, "insert delivery")
}

// UpsertInstanceHealth implements federation.Repository.
func (r *FederationRepository) UpsertInstanceHealth(ctx context.Context, tx uow.Tx, h federation.InstanceHealth) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceHealth(ctx, sqlitegen.UpsertInstanceHealthParams{
		Domain: h.Domain, State: string(h.State), Since: toMillis(h.Since),
		FailureDays: int64(h.FailureDays), LastFailureDay: h.LastFailureDay,
	})
	return wrap(err, "upsert instance health")
}

// FindOutboundActivity implements federation.Repository.
func (r *FederationRepository) FindOutboundActivity(ctx context.Context, id string) (federation.OutboundActivity, error) {
	row, err := r.db.reader().GetOutboundActivity(ctx, id)
	if err != nil {
		return federation.OutboundActivity{}, notFound(err, appfederation.ErrNotFound, "outbound activity "+id)
	}
	return outboundActivity(row), nil
}

// ListOutboundActivitiesOf implements federation.Repository.
func (r *FederationRepository) ListOutboundActivitiesOf(ctx context.Context, kind federation.OutboundObjectKind, objectID string) ([]federation.OutboundActivity, error) {
	rows, err := r.db.reader().ListOutboundActivitiesOfObject(ctx, sqlitegen.ListOutboundActivitiesOfObjectParams{
		ObjectType: string(kind), ObjectID: objectID,
	})
	if err != nil {
		return nil, wrap(err, "list outbound activities")
	}
	out := make([]federation.OutboundActivity, len(rows))
	for i, row := range rows {
		out[i] = outboundActivity(row)
	}
	return out, nil
}

func outboundActivity(row sqlitegen.OutboundActivity) federation.OutboundActivity {
	return federation.OutboundActivity{
		ID: row.ID, ActivityURI: federation.IRI(row.ActivityUri), Type: federation.ActivityType(row.Type),
		ActorAccountID: row.ActorAccountID, ObjectKind: federation.OutboundObjectKind(row.ObjectType),
		ObjectID: row.ObjectID, CreatedAt: fromMillis(row.CreatedAt),
	}
}

// FindOutboundDocument implements federation.Repository.
func (r *FederationRepository) FindOutboundDocument(ctx context.Context, outboundActivityID string) ([]byte, error) {
	doc, err := r.db.reader().GetOutboundActivityDocument(ctx, outboundActivityID)
	if err != nil {
		return nil, notFound(err, appfederation.ErrNotFound, "document of outbound activity "+outboundActivityID)
	}
	return doc, nil
}

// ListDeliveries implements federation.Repository.
func (r *FederationRepository) ListDeliveries(ctx context.Context, outboundActivityID, after string, limit int) ([]federation.Delivery, error) {
	rows, err := r.db.reader().ListDeliveriesOfOutboundActivity(ctx, sqlitegen.ListDeliveriesOfOutboundActivityParams{
		OutboundActivityID: outboundActivityID, AfterID: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list deliveries")
	}
	out := make([]federation.Delivery, len(rows))
	for i, row := range rows {
		out[i] = federation.Delivery{
			ID: row.ID, OutboundActivityID: row.OutboundActivityID, InboxURL: federation.IRI(row.InboxUrl),
			Domain: row.Domain, CreatedAt: fromMillis(row.CreatedAt),
		}
	}
	return out, nil
}

// InstanceState implements federation.Repository.
func (r *FederationRepository) InstanceState(ctx context.Context, domain string) (federation.InstanceState, error) {
	row, err := r.db.reader().GetInstanceHealth(ctx, domain)
	if errors.Is(err, sql.ErrNoRows) {
		return federation.InstanceAvailable, nil
	}
	if err != nil {
		return "", wrap(err, "get instance health")
	}
	return federation.InstanceState(row.State), nil
}
