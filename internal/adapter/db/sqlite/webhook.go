// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// WebhookRepository implements appwebhook.Repository (data-model §4.11, ADR-0060).
type WebhookRepository struct {
	db *DB
}

var _ appwebhook.Repository = (*WebhookRepository)(nil)

// WebhookRepository returns the webhook module's repository on this database.
func (d *DB) WebhookRepository() *WebhookRepository { return &WebhookRepository{db: d} }

// InsertEndpoint implements appwebhook.Repository.
func (r *WebhookRepository) InsertEndpoint(ctx context.Context, tx uow.Tx, id, createdByUserID string, createdAt time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertWebhookEndpoint(ctx, sqlitegen.InsertWebhookEndpointParams{
		ID: id, CreatedByUserID: toNullString(createdByUserID), CreatedAt: toMillis(createdAt),
	}), "insert webhook endpoint")
}

// InsertRevision implements appwebhook.Repository.
func (r *WebhookRepository) InsertRevision(ctx context.Context, tx uow.Tx, rev appwebhook.Revision) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertWebhookEndpointRevision(ctx, sqlitegen.InsertWebhookEndpointRevisionParams{
		ID: rev.ID, EndpointID: rev.EndpointID, Url: rev.Settings.URL, Description: toNullString(rev.Settings.Description),
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	})
	if err != nil {
		return wrap(err, "insert webhook endpoint revision")
	}
	for _, e := range rev.Settings.Events {
		err := q.InsertWebhookEndpointRevisionEvent(ctx, sqlitegen.InsertWebhookEndpointRevisionEventParams{
			RevisionID: rev.ID, EventType: string(e),
		})
		if err != nil {
			return wrap(err, "insert webhook endpoint revision event")
		}
	}
	return nil
}

// InsertSecret implements appwebhook.Repository.
func (r *WebhookRepository) InsertSecret(ctx context.Context, tx uow.Tx, s webhook.Secret) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertWebhookSecret(ctx, sqlitegen.InsertWebhookSecretParams{
		ID: s.ID, EndpointID: s.EndpointID, SealedSecret: s.Sealed, CreatedByUserID: toNullString(s.CreatedByUserID),
		CreatedAt: toMillis(s.CreatedAt),
	}), "insert webhook secret")
}

// InsertStateChange implements appwebhook.Repository.
func (r *WebhookRepository) InsertStateChange(ctx context.Context, tx uow.Tx, c appwebhook.StateChange) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertWebhookEndpointStateChange(ctx, sqlitegen.InsertWebhookEndpointStateChangeParams{
		ID: c.ID, EndpointID: c.EndpointID, State: string(c.State), Reason: string(c.Reason),
		ChangedByUserID: toNullString(c.ChangedByUserID), ChangedAt: toMillis(c.ChangedAt),
	}), "insert webhook endpoint state change")
}

// UpsertHead implements appwebhook.Repository.
func (r *WebhookRepository) UpsertHead(ctx context.Context, tx uow.Tx, h appwebhook.Head) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertWebhookEndpointHead(ctx, sqlitegen.UpsertWebhookEndpointHeadParams{
		EndpointID: h.EndpointID, RevisionID: h.RevisionID, State: string(h.State), SecretID: h.SecretID,
	}), "upsert webhook endpoint head")
}

// HeadForUpdate implements appwebhook.Repository. A SQLite transaction holds the write lock, so
// the row needs no lock of its own.
func (r *WebhookRepository) HeadForUpdate(ctx context.Context, tx uow.Tx, endpointID string) (appwebhook.Head, error) {
	q, err := queries(tx)
	if err != nil {
		return appwebhook.Head{}, err
	}
	row, err := q.GetWebhookEndpointHead(ctx, endpointID)
	if err != nil {
		return appwebhook.Head{}, notFound(err, appwebhook.ErrNotFound, "webhook endpoint "+endpointID)
	}
	return appwebhook.Head{EndpointID: row.EndpointID, RevisionID: row.RevisionID, State: webhook.State(row.State), SecretID: row.SecretID}, nil
}

// FindEndpoint implements appwebhook.Repository.
func (r *WebhookRepository) FindEndpoint(ctx context.Context, id string) (webhook.Endpoint, error) {
	if id == "" {
		return webhook.Endpoint{}, appwebhook.ErrNotFound
	}
	out, err := r.endpoints(ctx, id)
	if err != nil {
		return webhook.Endpoint{}, err
	}
	if len(out) == 0 {
		return webhook.Endpoint{}, notFound(sql.ErrNoRows, appwebhook.ErrNotFound, "webhook endpoint "+id)
	}
	return out[0], nil
}

// ListEndpoints implements appwebhook.Repository.
func (r *WebhookRepository) ListEndpoints(ctx context.Context) ([]webhook.Endpoint, error) {
	return r.endpoints(ctx, "")
}

func (r *WebhookRepository) endpoints(ctx context.Context, id string) ([]webhook.Endpoint, error) {
	rows, err := r.db.reader().ListWebhookEndpoints(ctx, id)
	if err != nil {
		return nil, wrap(err, "list webhook endpoints")
	}
	revisions := make([]string, len(rows))
	out := make([]webhook.Endpoint, len(rows))
	byRevision := make(map[string]int, len(rows))
	for i, row := range rows {
		revisions[i] = row.RevisionID
		byRevision[row.RevisionID] = i
		out[i] = webhook.Endpoint{
			ID: row.ID, CreatedByUserID: row.CreatedByUserID.String, CreatedAt: fromMillis(row.CreatedAt),
			RevisionID: row.RevisionID, RevisedAt: fromMillis(row.RevisedAt),
			Settings:       webhook.EndpointSettings{URL: row.Url, Description: row.Description.String, Events: []webhook.EventType{}},
			State:          webhook.State(row.State),
			StateReason:    webhook.StateReason(row.StateReason),
			StateChangedAt: fromMillis(row.StateChangedAt),
			SecretID:       row.SecretID,
		}
		if row.FirstFailureAttemptID.Valid {
			out[i].Failures = &webhook.Failures{
				FirstFailureAttemptID: row.FirstFailureAttemptID.String, FirstFailureAt: fromMillis(row.FirstFailedAt.Int64),
				Consecutive: int(row.ConsecutiveFailures.Int64),
			}
		}
	}
	if len(rows) == 0 {
		return out, nil
	}
	list, err := jsonList(revisions)
	if err != nil {
		return nil, err
	}
	events, err := r.db.reader().ListWebhookRevisionEvents(ctx, list)
	if err != nil {
		return nil, wrap(err, "list webhook revision events")
	}
	for _, e := range events {
		i := byRevision[e.RevisionID]
		out[i].Settings.Events = append(out[i].Settings.Events, webhook.EventType(e.EventType))
	}
	return out, nil
}

// SubscribedEndpoints implements appwebhook.Repository.
func (r *WebhookRepository) SubscribedEndpoints(ctx context.Context, tx uow.Tx, event webhook.EventType) ([]string, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListSubscribedWebhookEndpoints(ctx, string(event))
	return ids, wrap(err, "list subscribed webhook endpoints")
}

// Secrets implements appwebhook.Repository.
func (r *WebhookRepository) Secrets(ctx context.Context, endpointID string, limit int) ([]webhook.Secret, error) {
	rows, err := r.db.reader().ListWebhookSecrets(ctx, sqlitegen.ListWebhookSecretsParams{EndpointID: endpointID, Lim: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list webhook secrets")
	}
	out := make([]webhook.Secret, len(rows))
	for i, row := range rows {
		out[i] = webhook.Secret{
			ID: row.ID, EndpointID: row.EndpointID, Sealed: row.SealedSecret, CreatedByUserID: row.CreatedByUserID.String,
			CreatedAt: fromMillis(row.CreatedAt),
		}
	}
	return out, nil
}

// InsertMessage implements appwebhook.Repository.
func (r *WebhookRepository) InsertMessage(ctx context.Context, tx uow.Tx, m webhook.Message) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertWebhookMessage(ctx, sqlitegen.InsertWebhookMessageParams{
		ID: m.ID, EventType: string(m.EventType), Body: m.Body, CreatedAt: toMillis(m.CreatedAt),
	}), "insert webhook message")
}

// InsertDelivery implements appwebhook.Repository.
func (r *WebhookRepository) InsertDelivery(ctx context.Context, tx uow.Tx, d webhook.Delivery) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertWebhookDelivery(ctx, sqlitegen.InsertWebhookDeliveryParams{
		ID: d.ID, MessageID: d.MessageID, EndpointID: d.EndpointID, CreatedAt: toMillis(d.CreatedAt),
	}), "insert webhook delivery")
}

// FindSend implements appwebhook.Repository.
func (r *WebhookRepository) FindSend(ctx context.Context, deliveryID string) (appwebhook.Send, error) {
	row, err := r.db.reader().GetWebhookSend(ctx, deliveryID)
	if err != nil {
		return appwebhook.Send{}, notFound(err, appwebhook.ErrNotFound, "webhook delivery "+deliveryID)
	}
	return appwebhook.Send{
		Delivery: webhook.Delivery{ID: row.ID, MessageID: row.MessageID, EndpointID: row.EndpointID, CreatedAt: fromMillis(row.CreatedAt)},
		Message: webhook.Message{
			ID: row.MessageID, EventType: webhook.EventType(row.EventType), Body: row.Body, CreatedAt: fromMillis(row.MessageCreatedAt),
		},
		URL: row.Url, Description: row.Description.String, State: webhook.State(row.State), Succeeded: row.Succeeded != 0,
	}, nil
}

// InsertAttempt implements appwebhook.Repository.
func (r *WebhookRepository) InsertAttempt(ctx context.Context, tx uow.Tx, a webhook.Attempt) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertWebhookDeliveryAttempt(ctx, sqlitegen.InsertWebhookDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome),
		StatusCode: sql.NullInt64{Int64: int64(a.StatusCode), Valid: a.StatusCode != 0},
		DurationMs: a.Duration.Milliseconds(), AttemptedAt: toMillis(a.AttemptedAt),
	}), "insert webhook delivery attempt")
}

// Failures implements appwebhook.Repository.
func (r *WebhookRepository) Failures(ctx context.Context, tx uow.Tx, endpointID string) (*webhook.Failures, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetWebhookEndpointFailures(ctx, endpointID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, wrap(err, "get webhook endpoint failures")
	}
	return &webhook.Failures{
		FirstFailureAttemptID: row.FirstFailureAttemptID, FirstFailureAt: fromMillis(row.FirstFailedAt), Consecutive: int(row.ConsecutiveFailures),
	}, nil
}

// AddFailure implements appwebhook.Repository.
func (r *WebhookRepository) AddFailure(ctx context.Context, tx uow.Tx, endpointID, attemptID string, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.AddWebhookEndpointFailure(ctx, sqlitegen.AddWebhookEndpointFailureParams{
		EndpointID: endpointID, FirstFailureAttemptID: attemptID, FirstFailedAt: toMillis(at),
	}), "add webhook endpoint failure")
}

// DeleteFailures implements appwebhook.Repository.
func (r *WebhookRepository) DeleteFailures(ctx context.Context, tx uow.Tx, endpointID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteWebhookEndpointFailures(ctx, endpointID), "delete webhook endpoint failures")
}

// ListDeliveries implements appwebhook.Repository.
func (r *WebhookRepository) ListDeliveries(ctx context.Context, endpointID, before string, limit int) ([]webhook.DeliveryRecord, error) {
	return r.deliveries(ctx, sqlitegen.ListWebhookDeliveriesParams{EndpointID: endpointID, Before: before, Lim: int64(limit)})
}

// FindDelivery implements appwebhook.Repository.
func (r *WebhookRepository) FindDelivery(ctx context.Context, endpointID, deliveryID string) (webhook.DeliveryRecord, error) {
	if deliveryID == "" {
		return webhook.DeliveryRecord{}, appwebhook.ErrNotFound
	}
	out, err := r.deliveries(ctx, sqlitegen.ListWebhookDeliveriesParams{EndpointID: endpointID, ID: deliveryID, Lim: 1})
	if err != nil {
		return webhook.DeliveryRecord{}, err
	}
	if len(out) == 0 {
		return webhook.DeliveryRecord{}, notFound(sql.ErrNoRows, appwebhook.ErrNotFound, "webhook delivery "+deliveryID)
	}
	return out[0], nil
}

func (r *WebhookRepository) deliveries(ctx context.Context, p sqlitegen.ListWebhookDeliveriesParams) ([]webhook.DeliveryRecord, error) {
	rows, err := r.db.reader().ListWebhookDeliveries(ctx, p)
	if err != nil {
		return nil, wrap(err, "list webhook deliveries")
	}
	out := make([]webhook.DeliveryRecord, len(rows))
	for i, row := range rows {
		out[i] = webhook.DeliveryRecord{
			Delivery:  webhook.Delivery{ID: row.ID, MessageID: row.MessageID, EndpointID: row.EndpointID, CreatedAt: fromMillis(row.CreatedAt)},
			EventType: webhook.EventType(row.EventType), Attempts: int(row.Attempts), Succeeded: row.Succeeded != 0,
		}
		if row.LastID.Valid {
			out[i].Last = &webhook.Attempt{
				ID: row.LastID.String, DeliveryID: row.ID, Outcome: webhook.Outcome(row.LastOutcome.String),
				StatusCode: int(row.LastStatusCode.Int64), Duration: time.Duration(row.LastDurationMs.Int64) * time.Millisecond,
				AttemptedAt: fromMillis(row.LastAttemptedAt.Int64),
			}
		}
	}
	return out, nil
}

// DeleteMessagesBefore implements appwebhook.Repository.
func (r *WebhookRepository) DeleteMessagesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := q.ListWebhookMessagesBefore(ctx, sqlitegen.ListWebhookMessagesBeforeParams{Before: toMillis(t), Lim: int64(limit)})
	if err != nil || len(ids) == 0 {
		return 0, wrap(err, "list old webhook messages")
	}
	list, err := jsonList(ids)
	if err != nil {
		return 0, err
	}
	if err := q.DeleteWebhookAttemptsOfMessages(ctx, list); err != nil {
		return 0, wrap(err, "delete webhook delivery attempts")
	}
	if err := q.DeleteWebhookDeliveriesOfMessages(ctx, list); err != nil {
		return 0, wrap(err, "delete webhook deliveries")
	}
	n, err := q.DeleteWebhookMessages(ctx, list)
	return n, wrap(err, "delete webhook messages")
}

// ExistingEndpointIDs implements appwebhook.Repository.
func (r *WebhookRepository) ExistingEndpointIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListExistingWebhookEndpointIDs(ctx, list)
	if err != nil {
		return nil, wrap(err, "list existing webhook endpoints")
	}
	return out, nil
}
