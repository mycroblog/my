// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/webhookgen"
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
func NewRepository(d *postgres.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertEndpoint implements appwebhook.Repository.
func (r *Repository) InsertEndpoint(ctx context.Context, tx uow.Tx, id, createdByUserID string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertWebhookEndpoint(ctx, webhookgen.InsertWebhookEndpointParams{
		ID: id, CreatedByUserID: postgres.ToNullString(createdByUserID), CreatedAt: createdAt,
	}), "insert webhook endpoint")
}

// InsertRevision implements appwebhook.Repository.
func (r *Repository) InsertRevision(ctx context.Context, tx uow.Tx, rev appwebhook.Revision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertWebhookEndpointRevision(ctx, webhookgen.InsertWebhookEndpointRevisionParams{
		ID: rev.ID, EndpointID: rev.EndpointID, Url: rev.Settings.URL, Description: postgres.ToNullString(rev.Settings.Description),
		RevisedByUserID: postgres.ToNullString(rev.RevisedByUserID), RevisedAt: rev.RevisedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert webhook endpoint revision")
	}
	for _, e := range rev.Settings.Events {
		err := q.InsertWebhookEndpointRevisionEvent(ctx, webhookgen.InsertWebhookEndpointRevisionEventParams{
			RevisionID: rev.ID, EventType: string(e),
		})
		if err != nil {
			return postgres.Wrap(err, "insert webhook endpoint revision event")
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
	return postgres.Wrap(q.InsertWebhookSecret(ctx, webhookgen.InsertWebhookSecretParams{
		ID: s.ID, EndpointID: s.EndpointID, SealedSecret: s.Sealed, CreatedByUserID: postgres.ToNullString(s.CreatedByUserID),
		CreatedAt: s.CreatedAt,
	}), "insert webhook secret")
}

// InsertStateChange implements appwebhook.Repository.
func (r *Repository) InsertStateChange(ctx context.Context, tx uow.Tx, c appwebhook.StateChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertWebhookEndpointStateChange(ctx, webhookgen.InsertWebhookEndpointStateChangeParams{
		ID: c.ID, EndpointID: c.EndpointID, State: string(c.State), Reason: string(c.Reason),
		ChangedByUserID: postgres.ToNullString(c.ChangedByUserID), ChangedAt: c.ChangedAt,
	}), "insert webhook endpoint state change")
}

// UpsertHead implements appwebhook.Repository.
func (r *Repository) UpsertHead(ctx context.Context, tx uow.Tx, h appwebhook.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertWebhookEndpointHead(ctx, webhookgen.UpsertWebhookEndpointHeadParams{
		EndpointID: h.EndpointID, RevisionID: h.RevisionID, State: string(h.State), SecretID: h.SecretID,
	}), "upsert webhook endpoint head")
}

// HeadForUpdate implements appwebhook.Repository; it locks the row until the transaction ends.
func (r *Repository) HeadForUpdate(ctx context.Context, tx uow.Tx, endpointID string) (appwebhook.Head, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appwebhook.Head{}, err
	}
	row, err := q.GetWebhookEndpointHeadForUpdate(ctx, endpointID)
	if err != nil {
		return appwebhook.Head{}, postgres.NotFound(err, appwebhook.ErrNotFound, "webhook endpoint "+endpointID)
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
		return webhook.Endpoint{}, postgres.NotFound(pgx.ErrNoRows, appwebhook.ErrNotFound, "webhook endpoint "+id)
	}
	return out[0], nil
}

// ListEndpoints implements appwebhook.Repository.
func (r *Repository) ListEndpoints(ctx context.Context) ([]webhook.Endpoint, error) {
	return r.endpoints(ctx, "")
}

func (r *Repository) endpoints(ctx context.Context, id string) ([]webhook.Endpoint, error) {
	rows, err := r.db.reader().ListWebhookEndpoints(ctx, postgres.ToNullString(id))
	if err != nil {
		return nil, postgres.Wrap(err, "list webhook endpoints")
	}
	revisions := make([]string, len(rows))
	out := make([]webhook.Endpoint, len(rows))
	byRevision := make(map[string]int, len(rows))
	for i, row := range rows {
		revisions[i] = row.RevisionID
		byRevision[row.RevisionID] = i
		out[i] = webhook.Endpoint{
			ID: row.ID, CreatedByUserID: postgres.FromNullString(row.CreatedByUserID), CreatedAt: postgres.UTC(row.CreatedAt),
			RevisionID: row.RevisionID, RevisedAt: postgres.UTC(row.RevisedAt),
			Settings:       webhook.EndpointSettings{URL: row.Url, Description: postgres.FromNullString(row.Description), Events: []webhook.EventType{}},
			State:          webhook.State(row.State),
			StateReason:    webhook.StateReason(row.StateReason),
			StateChangedAt: postgres.UTC(row.StateChangedAt),
			SecretID:       row.SecretID,
		}
		if row.FirstFailureAttemptID != nil {
			out[i].Failures = &webhook.Failures{
				FirstFailureAttemptID: *row.FirstFailureAttemptID, FirstFailureAt: postgres.UTC(*row.FirstFailedAt),
				Consecutive: int(*row.ConsecutiveFailures),
			}
		}
	}
	if len(rows) == 0 {
		return out, nil
	}
	events, err := r.db.reader().ListWebhookRevisionEvents(ctx, revisions)
	if err != nil {
		return nil, postgres.Wrap(err, "list webhook revision events")
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
	return ids, postgres.Wrap(err, "list subscribed webhook endpoints")
}

// Secrets implements appwebhook.Repository.
func (r *Repository) Secrets(ctx context.Context, endpointID string, limit int) ([]webhook.Secret, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListWebhookSecrets(ctx, webhookgen.ListWebhookSecretsParams{EndpointID: endpointID, Lim: lim})
	if err != nil {
		return nil, postgres.Wrap(err, "list webhook secrets")
	}
	out := make([]webhook.Secret, len(rows))
	for i, row := range rows {
		out[i] = webhook.Secret{
			ID: row.ID, EndpointID: row.EndpointID, Sealed: row.SealedSecret, CreatedByUserID: postgres.FromNullString(row.CreatedByUserID),
			CreatedAt: postgres.UTC(row.CreatedAt),
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
	return postgres.Wrap(q.InsertWebhookMessage(ctx, webhookgen.InsertWebhookMessageParams{
		ID: m.ID, EventType: string(m.EventType), Body: m.Body, CreatedAt: m.CreatedAt,
	}), "insert webhook message")
}

// InsertDelivery implements appwebhook.Repository.
func (r *Repository) InsertDelivery(ctx context.Context, tx uow.Tx, d webhook.Delivery) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertWebhookDelivery(ctx, webhookgen.InsertWebhookDeliveryParams{
		ID: d.ID, MessageID: d.MessageID, EndpointID: d.EndpointID, CreatedAt: d.CreatedAt,
	}), "insert webhook delivery")
}

// FindSend implements appwebhook.Repository.
func (r *Repository) FindSend(ctx context.Context, deliveryID string) (appwebhook.Send, error) {
	row, err := r.db.reader().GetWebhookSend(ctx, deliveryID)
	if err != nil {
		return appwebhook.Send{}, postgres.NotFound(err, appwebhook.ErrNotFound, "webhook delivery "+deliveryID)
	}
	return appwebhook.Send{
		Delivery: webhook.Delivery{ID: row.ID, MessageID: row.MessageID, EndpointID: row.EndpointID, CreatedAt: postgres.UTC(row.CreatedAt)},
		Message: webhook.Message{
			ID: row.MessageID, EventType: webhook.EventType(row.EventType), Body: row.Body, CreatedAt: postgres.UTC(row.MessageCreatedAt),
		},
		URL: row.Url, Description: postgres.FromNullString(row.Description), State: webhook.State(row.State), Succeeded: row.Succeeded,
	}, nil
}

// InsertAttempt implements appwebhook.Repository.
func (r *Repository) InsertAttempt(ctx context.Context, tx uow.Tx, a webhook.Attempt) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertWebhookDeliveryAttempt(ctx, webhookgen.InsertWebhookDeliveryAttemptParams{
		ID: a.ID, DeliveryID: a.DeliveryID, Outcome: string(a.Outcome),
		StatusCode: statusCode(a.StatusCode),
		DurationMs: a.Duration.Milliseconds(), AttemptedAt: a.AttemptedAt,
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
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, postgres.Wrap(err, "get webhook endpoint failures")
	}
	return &webhook.Failures{
		FirstFailureAttemptID: row.FirstFailureAttemptID, FirstFailureAt: postgres.UTC(row.FirstFailedAt), Consecutive: int(row.ConsecutiveFailures),
	}, nil
}

// AddFailure implements appwebhook.Repository.
func (r *Repository) AddFailure(ctx context.Context, tx uow.Tx, endpointID, attemptID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.AddWebhookEndpointFailure(ctx, webhookgen.AddWebhookEndpointFailureParams{
		EndpointID: endpointID, FirstFailureAttemptID: attemptID, FirstFailedAt: at,
	}), "add webhook endpoint failure")
}

// DeleteFailures implements appwebhook.Repository.
func (r *Repository) DeleteFailures(ctx context.Context, tx uow.Tx, endpointID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteWebhookEndpointFailures(ctx, endpointID), "delete webhook endpoint failures")
}

// ListDeliveries implements appwebhook.Repository.
func (r *Repository) ListDeliveries(ctx context.Context, endpointID, before string, limit int) ([]webhook.DeliveryRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	return r.deliveries(ctx, webhookgen.ListWebhookDeliveriesParams{EndpointID: endpointID, Before: postgres.ToNullString(before), Lim: lim})
}

// FindDelivery implements appwebhook.Repository.
func (r *Repository) FindDelivery(ctx context.Context, endpointID, deliveryID string) (webhook.DeliveryRecord, error) {
	if deliveryID == "" {
		return webhook.DeliveryRecord{}, appwebhook.ErrNotFound
	}
	out, err := r.deliveries(ctx, webhookgen.ListWebhookDeliveriesParams{EndpointID: endpointID, ID: postgres.ToNullString(deliveryID), Lim: 1})
	if err != nil {
		return webhook.DeliveryRecord{}, err
	}
	if len(out) == 0 {
		return webhook.DeliveryRecord{}, postgres.NotFound(pgx.ErrNoRows, appwebhook.ErrNotFound, "webhook delivery "+deliveryID)
	}
	return out[0], nil
}

func (r *Repository) deliveries(ctx context.Context, p webhookgen.ListWebhookDeliveriesParams) ([]webhook.DeliveryRecord, error) {
	rows, err := r.db.reader().ListWebhookDeliveries(ctx, p)
	if err != nil {
		return nil, postgres.Wrap(err, "list webhook deliveries")
	}
	out := make([]webhook.DeliveryRecord, len(rows))
	for i, row := range rows {
		out[i] = webhook.DeliveryRecord{
			Delivery:  webhook.Delivery{ID: row.ID, MessageID: row.MessageID, EndpointID: row.EndpointID, CreatedAt: postgres.UTC(row.CreatedAt)},
			EventType: webhook.EventType(row.EventType), Attempts: int(row.Attempts), Succeeded: row.Succeeded,
		}
		if row.LastID != nil {
			out[i].Last = &webhook.Attempt{
				ID: *row.LastID, DeliveryID: row.ID, Outcome: webhook.Outcome(postgres.FromNullString(row.LastOutcome)),
				StatusCode: int(derefInt(row.LastStatusCode)), Duration: time.Duration(derefInt(row.LastDurationMs)) * time.Millisecond,
				AttemptedAt: postgres.UTC(*row.LastAttemptedAt),
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
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return 0, err
	}
	ids, err := q.ListWebhookMessagesBefore(ctx, webhookgen.ListWebhookMessagesBeforeParams{Before: t, Lim: lim})
	if err != nil || len(ids) == 0 {
		return 0, postgres.Wrap(err, "list old webhook messages")
	}
	list := ids
	if err := q.DeleteWebhookAttemptsOfMessages(ctx, list); err != nil {
		return 0, postgres.Wrap(err, "delete webhook delivery attempts")
	}
	if err := q.DeleteWebhookDeliveriesOfMessages(ctx, list); err != nil {
		return 0, postgres.Wrap(err, "delete webhook deliveries")
	}
	n, err := q.DeleteWebhookMessages(ctx, list)
	return n, postgres.Wrap(err, "delete webhook messages")
}

// statusCode returns the status_code column of an attempt: NULL for none.
func statusCode(code int) *int32 {
	if code == 0 {
		return nil
	}
	n := int32(min(code, math.MaxInt32)) //nolint:gosec // G115: bounded by min
	return &n
}

func derefInt[T int32 | int64](n *T) T {
	if n == nil {
		return 0
	}
	return *n
}

// ExistingEndpointIDs implements appwebhook.Repository.
func (r *Repository) ExistingEndpointIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.db.reader().ListExistingWebhookEndpointIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list existing webhook endpoints")
	}
	return out, nil
}
