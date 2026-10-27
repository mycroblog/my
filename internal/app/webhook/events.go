// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// Domain events of the webhook module.
const (
	// EventEndpointChanged follows every write of an endpoint's revision, secret, or state, with
	// its new head.
	EventEndpointChanged = "webhook.endpoint_changed"
	// EventAttemptRecorded follows every delivery attempt.
	EventAttemptRecorded = "webhook.attempt_recorded"
	// EventEndpointDisabled follows the server's disabling of an endpoint that answered 410 Gone
	// or kept failing; the notification module tells the holders of manage_webhooks.
	EventEndpointDisabled = "webhook.endpoint_disabled"
)

// EndpointChanged carries the new head of an endpoint.
type EndpointChanged struct{ Head Head }

// EventType implements uow.Event.
func (EndpointChanged) EventType() string { return EventEndpointChanged }

// AttemptRecorded carries an attempt of a delivery to an endpoint.
type AttemptRecorded struct {
	EndpointID string
	Attempt    webhook.Attempt
}

// EventType implements uow.Event.
func (AttemptRecorded) EventType() string { return EventAttemptRecorded }

// EndpointDisabled carries the state change of an endpoint that the server disabled.
type EndpointDisabled struct {
	EndpointID    string
	StateChangeID string
	Reason        webhook.StateReason
	At            time.Time
}

// EventType implements uow.Event.
func (EndpointDisabled) EventType() string { return EventEndpointDisabled }

// RegisterProjectors registers the projectors of the module's own projections: the endpoint heads
// from EndpointChanged, and the runs of failures from AttemptRecorded.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventEndpointChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertHead(ctx, tx, e.(EndpointChanged).Head)
	})
	r.Register(EventAttemptRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(AttemptRecorded)
		if !ev.Attempt.Failed() {
			return repo.DeleteFailures(ctx, tx, ev.EndpointID)
		}
		return repo.AddFailure(ctx, tx, ev.EndpointID, ev.Attempt.ID, ev.Attempt.AttemptedAt)
	})
}
