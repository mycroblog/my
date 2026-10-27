// SPDX-License-Identifier: AGPL-3.0-only

// Package webhook holds the use cases of the instance's webhooks (ADR-0060, FR-722): the
// endpoints that the holders of manage_webhooks set up, the messages that the events of the other
// modules become in their transactions, and the webhook.deliver job that signs and sends them,
// retries them, and disables an endpoint that keeps failing. The adapters of Repository live in
// internal/adapter/db/*; their contract test suite is testkit.WebhookRepositoryContract.
package webhook

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/webhook"
)

var (
	// ErrNotFound reports an unknown or deleted endpoint, or an unknown delivery.
	ErrNotFound = errors.New("webhook: not found")
	// ErrDeleted reports a change to a deleted endpoint.
	ErrDeleted = errors.New("webhook: the endpoint is deleted")
)

// Revision is one revision of an endpoint's settings (webhook_endpoint_revisions with its
// webhook_endpoint_revision_events).
type Revision struct {
	ID              string
	EndpointID      string
	Settings        webhook.EndpointSettings
	RevisedByUserID string // "" for the CLI
	RevisedAt       time.Time
}

// StateChange is a change of an endpoint's state (webhook_endpoint_state_changes).
type StateChange struct {
	ID              string
	EndpointID      string
	State           webhook.State
	Reason          webhook.StateReason
	ChangedByUserID string // "" for the CLI and the server
	ChangedAt       time.Time
}

// Head is the current revision, state, and secret of an endpoint (webhook_endpoint_heads).
type Head struct {
	EndpointID string
	RevisionID string
	State      webhook.State
	SecretID   string
}

// Send is what the webhook.deliver job reads to send a delivery: the delivery with its message,
// the endpoint's current URL, description, and state, and whether an attempt already succeeded.
type Send struct {
	Delivery    webhook.Delivery
	Message     webhook.Message
	URL         string
	Description string
	State       webhook.State
	Succeeded   bool
}

// Repository is the persistence port of the webhook module.
type Repository interface {
	// InsertEndpoint inserts an endpoint; the revision, secret, and state change that complete it
	// are inserted in the same transaction.
	InsertEndpoint(ctx context.Context, tx uow.Tx, id, createdByUserID string, createdAt time.Time) error
	InsertRevision(ctx context.Context, tx uow.Tx, r Revision) error
	InsertSecret(ctx context.Context, tx uow.Tx, s webhook.Secret) error
	InsertStateChange(ctx context.Context, tx uow.Tx, c StateChange) error
	// UpsertHead sets the head of an endpoint (the projector of EndpointChanged).
	UpsertHead(ctx context.Context, tx uow.Tx, h Head) error
	// HeadForUpdate returns the head of an endpoint in tx, locking it on the databases that lock
	// rows; it fails with ErrNotFound for an unknown endpoint.
	HeadForUpdate(ctx context.Context, tx uow.Tx, endpointID string) (Head, error)
	// FindEndpoint returns an endpoint that is not deleted, with its settings, state, and failures;
	// it fails with ErrNotFound otherwise.
	FindEndpoint(ctx context.Context, id string) (webhook.Endpoint, error)
	// ListEndpoints returns the endpoints that are not deleted, newest first.
	ListEndpoints(ctx context.Context) ([]webhook.Endpoint, error)
	// ExistingEndpointIDs returns, in ID order, the endpoints among ids that are not deleted.
	ExistingEndpointIDs(ctx context.Context, ids []string) ([]string, error)
	// SubscribedEndpoints returns in tx, in ID order, the enabled endpoints whose current revision
	// subscribes to event.
	SubscribedEndpoints(ctx context.Context, tx uow.Tx, event webhook.EventType) ([]string, error)
	// Secrets returns up to limit secrets of an endpoint, newest first.
	Secrets(ctx context.Context, endpointID string, limit int) ([]webhook.Secret, error)

	InsertMessage(ctx context.Context, tx uow.Tx, m webhook.Message) error
	InsertDelivery(ctx context.Context, tx uow.Tx, d webhook.Delivery) error
	// FindSend returns what sending a delivery needs; it fails with ErrNotFound for an unknown
	// delivery (also one past its retention).
	FindSend(ctx context.Context, deliveryID string) (Send, error)
	// InsertAttempt records an attempt of a delivery.
	InsertAttempt(ctx context.Context, tx uow.Tx, a webhook.Attempt) error
	// Failures returns in tx the run of failures of an endpoint, nil without one.
	Failures(ctx context.Context, tx uow.Tx, endpointID string) (*webhook.Failures, error)
	// AddFailure records a failed attempt at at in the run of failures of an endpoint, in one
	// statement: it starts the run with the attempt, or adds one to the run that goes on, so
	// that concurrent failures each count. DeleteFailures ends the run (the projectors of
	// AttemptRecorded).
	AddFailure(ctx context.Context, tx uow.Tx, endpointID, attemptID string, at time.Time) error
	DeleteFailures(ctx context.Context, tx uow.Tx, endpointID string) error
	// ListDeliveries returns up to limit deliveries of an endpoint, newest first, before the
	// delivery before ("" for the first page).
	ListDeliveries(ctx context.Context, endpointID, before string, limit int) ([]webhook.DeliveryRecord, error)
	// FindDelivery returns a delivery of an endpoint; it fails with ErrNotFound otherwise.
	FindDelivery(ctx context.Context, endpointID, deliveryID string) (webhook.DeliveryRecord, error)
	// DeleteMessagesBefore deletes up to limit messages created before t, with their deliveries
	// and attempts, and returns how many messages it deleted.
	DeleteMessagesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)

	ProjectionRepository
}
