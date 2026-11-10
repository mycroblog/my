// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/webhookgen"
	"github.com/mycroblog/my/internal/app/uow"
	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// Repository implements appwebhook.Repository (data-model §4.11, ADR-0060).
type Repository struct {
	db *moduleDB
}

var _ appwebhook.Repository = (*Repository)(nil)

// NewRepository returns the webhook module's repository on this database.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertEndpoint implements appwebhook.Repository.
func (r *Repository) InsertEndpoint(ctx context.Context, tx uow.Tx, id, createdByUserID string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookEndpoint(ctx, webhookgen.InsertWebhookEndpointParams{
		ID: id, CreatedByUserID: sqlite.ToNullString(createdByUserID), CreatedAt: sqlite.ToMillis(createdAt),
	}), "insert webhook endpoint")
}

// InsertRevision implements appwebhook.Repository.
func (r *Repository) InsertRevision(ctx context.Context, tx uow.Tx, rev appwebhook.Revision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertWebhookEndpointRevision(ctx, webhookgen.InsertWebhookEndpointRevisionParams{
		ID: rev.ID, EndpointID: rev.EndpointID, Url: rev.Settings.URL, Description: sqlite.ToNullString(rev.Settings.Description),
		RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID), RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert webhook endpoint revision")
	}
	for _, e := range rev.Settings.Events {
		err := q.InsertWebhookEndpointRevisionEvent(ctx, webhookgen.InsertWebhookEndpointRevisionEventParams{
			RevisionID: rev.ID, EventType: string(e),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert webhook endpoint revision event")
		}
	}
	return nil
}

// InsertSecret implements appwebhook.Repository.
func (r *Repository) InsertSecret(ctx context.Context, tx uow.Tx, s webhook.Secret) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookSecret(ctx, webhookgen.InsertWebhookSecretParams{
		ID: s.ID, EndpointID: s.EndpointID, SealedSecret: s.Sealed, CreatedByUserID: sqlite.ToNullString(s.CreatedByUserID),
		CreatedAt: sqlite.ToMillis(s.CreatedAt),
	}), "insert webhook secret")
}

// InsertStateChange implements appwebhook.Repository.
func (r *Repository) InsertStateChange(ctx context.Context, tx uow.Tx, c appwebhook.StateChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookEndpointStateChange(ctx, webhookgen.InsertWebhookEndpointStateChangeParams{
		ID: c.ID, EndpointID: c.EndpointID, State: string(c.State), Reason: string(c.Reason),
		ChangedByUserID: sqlite.ToNullString(c.ChangedByUserID), ChangedAt: sqlite.ToMillis(c.ChangedAt),
	}), "insert webhook endpoint state change")
}

// UpsertHead implements appwebhook.Repository.
func (r *Repository) UpsertHead(ctx context.Context, tx uow.Tx, h appwebhook.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertWebhookEndpointHead(ctx, webhookgen.UpsertWebhookEndpointHeadParams{
		EndpointID: h.EndpointID, RevisionID: h.RevisionID, State: string(h.State), SecretID: h.SecretID,
	}), "upsert webhook endpoint head")
}

// HeadForUpdate implements appwebhook.Repository. A SQLite transaction holds the write lock, so
// the row needs no lock of its own.
func (r *Repository) HeadForUpdate(ctx context.Context, tx uow.Tx, endpointID string) (appwebhook.Head, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appwebhook.Head{}, err
	}
	row, err := q.GetWebhookEndpointHead(ctx, endpointID)
	if err != nil {
		return appwebhook.Head{}, sqlite.NotFound(err, appwebhook.ErrNotFound, "webhook endpoint "+endpointID)
	}
	return appwebhook.Head{EndpointID: row.EndpointID, RevisionID: row.RevisionID, State: webhook.State(row.State), SecretID: row.SecretID}, nil
}

// FindEndpoint implements appwebhook.Repository.
func (r *Repository) FindEndpoint(ctx context.Context, id string) (webhook.Endpoint, error) {
	if id == "" {
		return webhook.Endpoint{}, appwebhook.ErrNotFound
	}
	out, err := r.endpoints(ctx, id)
	if err != nil {
		return webhook.Endpoint{}, err
	}
	if len(out) == 0 {
		return webhook.Endpoint{}, sqlite.NotFound(sql.ErrNoRows, appwebhook.ErrNotFound, "webhook endpoint "+id)
	}
	return out[0], nil
}

// ListEndpoints implements appwebhook.Repository.
func (r *Repository) ListEndpoints(ctx context.Context) ([]webhook.Endpoint, error) {
	return r.endpoints(ctx, "")
}

func (r *Repository) endpoints(ctx context.Context, id string) ([]webhook.Endpoint, error) {
	rows, err := r.db.reader().ListWebhookEndpoints(ctx, id)
	if err != nil {
		return nil, sqlite.Wrap(err, "list webhook endpoints")
	}
	revisions := make([]string, len(rows))
	out := make([]webhook.Endpoint, len(rows))
	byRevision := make(map[string]int, len(rows))
	for i, row := range rows {
		revisions[i] = row.RevisionID
		byRevision[row.RevisionID] = i
		out[i] = webhook.Endpoint{
			ID: row.ID, CreatedByUserID: row.CreatedByUserID.String, CreatedAt: sqlite.FromMillis(row.CreatedAt),
			RevisionID: row.RevisionID, RevisedAt: sqlite.FromMillis(row.RevisedAt),
			Settings:       webhook.EndpointSettings{URL: row.Url, Description: row.Description.String, Events: []webhook.EventType{}},
			State:          webhook.State(row.State),
			StateReason:    webhook.StateReason(row.StateReason),
			StateChangedAt: sqlite.FromMillis(row.StateChangedAt),
			SecretID:       row.SecretID,
		}
		if row.FirstFailureAttemptID.Valid {
			out[i].Failures = &webhook.Failures{
				FirstFailureAttemptID: row.FirstFailureAttemptID.String, FirstFailureAt: sqlite.FromMillis(row.FirstFailedAt.Int64),
				Consecutive: int(row.ConsecutiveFailures.Int64),
			}
		}
	}
	if len(rows) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(revisions)
	if err != nil {
		return nil, err
	}
	events, err := r.db.reader().ListWebhookRevisionEvents(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list webhook revision events")
	}
	for _, e := range events {
		i := byRevision[e.RevisionID]
		out[i].Settings.Events = append(out[i].Settings.Events, webhook.EventType(e.EventType))
	}
	return out, nil
}

// SubscribedEndpoints implements appwebhook.Repository.
func (r *Repository) SubscribedEndpoints(ctx context.Context, tx uow.Tx, event webhook.EventType) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListSubscribedWebhookEndpoints(ctx, string(event))
	return ids, sqlite.Wrap(err, "list subscribed webhook endpoints")
}

// Secrets implements appwebhook.Repository.
func (r *Repository) Secrets(ctx context.Context, endpointID string, limit int) ([]webhook.Secret, error) {
	rows, err := r.db.reader().ListWebhookSecrets(ctx, webhookgen.ListWebhookSecretsParams{EndpointID: endpointID, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list webhook secrets")
	}
	out := make([]webhook.Secret, len(rows))
	for i, row := range rows {
		out[i] = webhook.Secret{
			ID: row.ID, EndpointID: row.EndpointID, Sealed: row.SealedSecret, CreatedByUserID: row.CreatedByUserID.String,
			CreatedAt: sqlite.FromMillis(row.CreatedAt),
		}
	}
	return out, nil
}

// InsertMessage implements appwebhook.Repository.
func (r *Repository) InsertMessage(ctx context.Context, tx uow.Tx, m webhook.Message) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookMessage(ctx, webhookgen.InsertWebhookMessageParams{
		ID: m.ID, EventType: string(m.EventType), Body: m.Body, CreatedAt: sqlite.ToMillis(m.CreatedAt),
	}), "insert webhook message")
}

// InsertDelivery implements appwebhook.Repository.
func (r *Repository) InsertDelivery(ctx context.Context, tx uow.Tx, d webhook.Delivery) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookDelivery(ctx, webhookgen.InsertWebhookDeliveryParams{
		ID: d.ID, MessageID: d.MessageID, EndpointID: d.EndpointID, CreatedAt: sqlite.ToMillis(d.CreatedAt),
	}), "insert webhook delivery")
}

// InsertDeliveries implements appwebhook.Repository.
func (r *Repository) InsertDeliveries(ctx context.Context, tx uow.Tx, ds []webhook.Delivery) error {
	if len(ds) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, endpoints := make([]string, len(ds)), make([]string, len(ds))
	for i, d := range ds {
		ids[i], endpoints[i] = d.ID, d.EndpointID
	}
	pairs, err := sqlite.JSONPairs(ids, endpoints)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookDeliveries(ctx, webhookgen.InsertWebhookDeliveriesParams{
		MessageID: ds[0].MessageID, CreatedAt: sqlite.ToMillis(ds[0].CreatedAt), Deliveries: pairs,
	}), "insert webhook deliveries")
}

// FindSend implements appwebhook.Repository.
func (r *Repository) FindSend(ctx context.Context, deliveryID string) (appwebhook.Send, error) {
	row, err := r.db.reader().GetWebhookSend(ctx, deliveryID)
	if err != nil {
		return appwebhook.Send{}, sqlite.NotFound(err, appwebhook.ErrNotFound, "webhook delivery "+deliveryID)
	}
	return appwebhook.Send{
		Delivery: webhook.Delivery{ID: row.ID, MessageID: row.MessageID, EndpointID: row.EndpointID, CreatedAt: sqlite.FromMillis(row.CreatedAt)},
		Message: webhook.Message{
			ID: row.MessageID, EventType: webhook.EventType(row.EventType), Body: row.Body, CreatedAt: sqlite.FromMillis(row.MessageCreatedAt),
		},
		URL: row.Url, Description: row.Description.String, State: webhook.State(row.State), Succeeded: row.Succeeded != 0,
	}, nil
}

// InsertAttempt implements appwebhook.Repository.
func (r *Repository) InsertAttempt(ctx context.Context, tx uow.Tx, a webhook.Attempt) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertWebhookDeliveryAttempt(ctx, webhookgen.InsertWebhookDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome),
		StatusCode: sql.NullInt64{Int64: int64(a.StatusCode), Valid: a.StatusCode != 0},
		DurationMs: a.Duration.Milliseconds(), AttemptedAt: sqlite.ToMillis(a.AttemptedAt),
	}), "insert webhook delivery attempt")
}

// Failures implements appwebhook.Repository.
func (r *Repository) Failures(ctx context.Context, tx uow.Tx, endpointID string) (*webhook.Failures, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetWebhookEndpointFailures(ctx, endpointID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, sqlite.Wrap(err, "get webhook endpoint failures")
	}
	return &webhook.Failures{
		FirstFailureAttemptID: row.FirstFailureAttemptID, FirstFailureAt: sqlite.FromMillis(row.FirstFailedAt), Consecutive: int(row.ConsecutiveFailures),
	}, nil
}

// AddFailure implements appwebhook.Repository.
func (r *Repository) AddFailure(ctx context.Context, tx uow.Tx, endpointID, attemptID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.AddWebhookEndpointFailure(ctx, webhookgen.AddWebhookEndpointFailureParams{
		EndpointID: endpointID, FirstFailureAttemptID: attemptID, FirstFailedAt: sqlite.ToMillis(at),
	}), "add webhook endpoint failure")
}

// DeleteFailures implements appwebhook.Repository.
func (r *Repository) DeleteFailures(ctx context.Context, tx uow.Tx, endpointID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteWebhookEndpointFailures(ctx, endpointID), "delete webhook endpoint failures")
}

// ListDeliveries implements appwebhook.Repository.
func (r *Repository) ListDeliveries(ctx context.Context, endpointID, before string, limit int) ([]webhook.DeliveryRecord, error) {
	return r.deliveries(ctx, webhookgen.ListWebhookDeliveriesParams{EndpointID: endpointID, Before: before, Lim: int64(limit)})
}

// FindDelivery implements appwebhook.Repository.
func (r *Repository) FindDelivery(ctx context.Context, endpointID, deliveryID string) (webhook.DeliveryRecord, error) {
	if deliveryID == "" {
		return webhook.DeliveryRecord{}, appwebhook.ErrNotFound
	}
	out, err := r.deliveries(ctx, webhookgen.ListWebhookDeliveriesParams{EndpointID: endpointID, ID: deliveryID, Lim: 1})
	if err != nil {
		return webhook.DeliveryRecord{}, err
	}
	if len(out) == 0 {
		return webhook.DeliveryRecord{}, sqlite.NotFound(sql.ErrNoRows, appwebhook.ErrNotFound, "webhook delivery "+deliveryID)
	}
	return out[0], nil
}

func (r *Repository) deliveries(ctx context.Context, p webhookgen.ListWebhookDeliveriesParams) ([]webhook.DeliveryRecord, error) {
	rows, err := r.db.reader().ListWebhookDeliveries(ctx, p)
	if err != nil {
		return nil, sqlite.Wrap(err, "list webhook deliveries")
	}
	out := make([]webhook.DeliveryRecord, len(rows))
	for i, row := range rows {
		out[i] = webhook.DeliveryRecord{
			Delivery:  webhook.Delivery{ID: row.ID, MessageID: row.MessageID, EndpointID: row.EndpointID, CreatedAt: sqlite.FromMillis(row.CreatedAt)},
			EventType: webhook.EventType(row.EventType), Attempts: int(row.Attempts), Succeeded: row.Succeeded != 0,
		}
		if row.LastID.Valid {
			out[i].Last = &webhook.Attempt{
				ID: row.LastID.String, DeliveryID: row.ID, Outcome: webhook.Outcome(row.LastOutcome.String),
				StatusCode: int(row.LastStatusCode.Int64), Duration: time.Duration(row.LastDurationMs.Int64) * time.Millisecond,
				AttemptedAt: sqlite.FromMillis(row.LastAttemptedAt.Int64),
			}
		}
	}
	return out, nil
}

// DeleteMessagesBefore implements appwebhook.Repository.
func (r *Repository) DeleteMessagesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := q.ListWebhookMessagesBefore(ctx, webhookgen.ListWebhookMessagesBeforeParams{Before: sqlite.ToMillis(t), Lim: int64(limit)})
	if err != nil || len(ids) == 0 {
		return 0, sqlite.Wrap(err, "list old webhook messages")
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return 0, err
	}
	if err := q.DeleteWebhookAttemptsOfMessages(ctx, list); err != nil {
		return 0, sqlite.Wrap(err, "delete webhook delivery attempts")
	}
	if err := q.DeleteWebhookDeliveriesOfMessages(ctx, list); err != nil {
		return 0, sqlite.Wrap(err, "delete webhook deliveries")
	}
	n, err := q.DeleteWebhookMessages(ctx, list)
	return n, sqlite.Wrap(err, "delete webhook messages")
}

// ExistingEndpointIDs implements appwebhook.Repository.
func (r *Repository) ExistingEndpointIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListExistingWebhookEndpointIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list existing webhook endpoints")
	}
	return out, nil
}
