// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the records of the approvals of interactions (ADR-0155 §8): the requests of
// replies, boosts, and quotes that their target's author approves, the answers to them, the
// interaction_request_states projection, and this server's authorizations. The commands that
// ask and answer are in approval_commands.go.

// The approvals of interactions, as part of this module's public API.
type (
	InteractionRequest       = content.InteractionRequest
	InteractionDirection     = content.InteractionDirection
	InteractionEvent         = content.InteractionEvent
	InteractionEventKind     = content.InteractionEventKind
	InteractionState         = content.InteractionState
	InteractionRequestState  = content.InteractionRequestState
	InteractionAuthorization = content.InteractionAuthorization
)

// Directions, answers, and states of interaction requests.
const (
	DirectionInbound           = content.DirectionInbound
	DirectionOutbound          = content.DirectionOutbound
	DirectionLocal             = content.DirectionLocal
	DirectionRemote            = content.DirectionRemote
	InteractionEventAccepted   = content.InteractionEventAccepted
	InteractionEventRejected   = content.InteractionEventRejected
	InteractionEventRevoked    = content.InteractionEventRevoked
	InteractionRequestPending  = content.InteractionRequestPending
	InteractionRequestAccepted = content.InteractionRequestAccepted
	InteractionRequestRejected = content.InteractionRequestRejected
	InteractionRequestRevoked  = content.InteractionRequestRevoked
)

// InteractionAuthorizationURI returns the ActivityPub id of an authorization of kind of host.
func InteractionAuthorizationURI(host string, kind content.InteractionKind, id string) string {
	return content.InteractionAuthorizationURI(host, kind, id)
}

// InteractionRequestURI returns the ActivityPub id of the ReplyRequest or AnnounceRequest of the
// request id that this server sends.
func InteractionRequestURI(host, id string) string { return content.InteractionRequestURI(host, id) }

// OwnInteraction is the state of a request of an account's own interaction (ListOwnInteractions):
// its reply InteractingURI of TargetURI, or its boost of TargetURI.
type OwnInteraction struct {
	RequestID      string
	Kind           content.InteractionKind
	TargetURI      string
	InteractingURI string
	State          content.InteractionRequestState
}

// ApprovalRepository stores the approvals of interactions (ADR-0155 §8).
type ApprovalRepository interface {
	// InsertInteractionRequest records a request. It reports false, writing nothing, when the
	// interaction has a request for the target already.
	InsertInteractionRequest(ctx context.Context, tx uow.Tx, r content.InteractionRequest) (bool, error)
	// FindInteractionRequest returns a request, ErrNotFound when there is none;
	// FindInteractionRequestFor finds the request of the interaction interactingURI of kind with
	// the target targetURI. Both read in tx, or outside any transaction when tx is nil.
	FindInteractionRequest(ctx context.Context, tx uow.Tx, id string) (content.InteractionRequest, error)
	FindInteractionRequestFor(ctx context.Context, tx uow.Tx, kind content.InteractionKind, interactingURI, targetURI string) (content.InteractionRequest, error)
	// ListInteractionRequestsAfter returns up to limit requests whose ID is above after ("" for
	// the first page), ordered by ID.
	ListInteractionRequestsAfter(ctx context.Context, tx uow.Tx, after string, limit int) ([]content.InteractionRequest, error)
	// InsertInteractionEvent records an answer to a request.
	InsertInteractionEvent(ctx context.Context, tx uow.Tx, e content.InteractionEvent) error
	// ListInteractionEvents returns the answers to a request in the order they occurred.
	ListInteractionEvents(ctx context.Context, tx uow.Tx, requestID string) ([]content.InteractionEvent, error)
	// ListPendingInteractionRequests returns up to limit requests of kinds (every kind when it is
	// empty) that wait for the answer of the author accountID, whose ID is below before ("" for
	// the newest), newest first.
	ListPendingInteractionRequests(ctx context.Context, accountID string, kinds []content.InteractionKind, before string, limit int) ([]content.InteractionRequest, error)
	// ListOwnInteractions returns the requests of the replies of accountID whose URI is among
	// uris, and of its boosts of the statuses among uris, with their states, with one query.
	ListOwnInteractions(ctx context.Context, accountID string, uris []string) ([]OwnInteraction, error)

	// InsertInteractionAuthorization records this server's approval of an interaction. It
	// reports false, writing nothing, when the interaction has one for the target status.
	InsertInteractionAuthorization(ctx context.Context, tx uow.Tx, a content.InteractionAuthorization) (bool, error)
	// FindInteractionAuthorization returns an approval with its revocation, ErrNotFound when
	// there is none; FindInteractionAuthorizationFor finds it by the kind, the interaction, and
	// the target status, in tx or outside any transaction when tx is nil.
	FindInteractionAuthorization(ctx context.Context, id string) (content.InteractionAuthorization, error)
	FindInteractionAuthorizationFor(ctx context.Context, tx uow.Tx, kind content.InteractionKind, interactingURI, targetStatusID string) (content.InteractionAuthorization, error)
	// InsertInteractionAuthorizationRevocation records the revocation of an approval, and
	// reports false, writing nothing, when it is revoked already.
	InsertInteractionAuthorizationRevocation(ctx context.Context, tx uow.Tx, authorizationID string, at time.Time) (bool, error)

	// InsertQuoteDetachment records that the author of a quoting status took its quote away;
	// recording it again does nothing.
	InsertQuoteDetachment(ctx context.Context, tx uow.Tx, statusID string, at time.Time) error

	// Projection, written only by this module's projectors and rebuild.

	UpsertInteractionState(ctx context.Context, tx uow.Tx, s content.InteractionState) error
	// FindInteractionState returns the state of a request, ErrNotFound before its projection,
	// in tx or outside any transaction when tx is nil.
	FindInteractionState(ctx context.Context, tx uow.Tx, requestID string) (content.InteractionState, error)
	// ListInteractionStates returns every state ordered by request ID, for verifying the
	// projection.
	ListInteractionStates(ctx context.Context) ([]content.InteractionState, error)
	DeleteAllInteractionStates(ctx context.Context, tx uow.Tx) error
}

// Event types of the approvals of interactions (data-model §4.4):
//   - EventInteractionRequested: an interaction that its target's author approves was recorded,
//     answered at once or pending. This module projects its state; the notification module
//     notifies the local author of a pending request; the federation module sends the
//     ReplyRequest or AnnounceRequest of a local account's interaction with a remote status.
//   - EventInteractionAnswered: a request was answered. This module projects the new state; the
//     notification module notifies the local account whose interaction it answers; the
//     federation module delivers the Accept or Reject of a remote account's interaction that
//     the handler of its request did not answer (Deferred).
//   - EventStatusReleased: a reply held out of its thread was approved. This module links it to
//     its parent and counts it; the other modules project it as they project the StatusCreated
//     they left it out of: the mentions are notified, and a local reply is published (timelines,
//     search, webhooks, and its Create).
const (
	EventInteractionRequested = "content.interaction_requested"
	EventInteractionAnswered  = "content.interaction_answered"
	EventStatusReleased       = "content.status_released"
)

// InteractionRequested is emitted when a request is inserted.
type InteractionRequested struct {
	Request content.InteractionRequest
	// State is the state after the answer given with the request, pending for none.
	State content.InteractionState
	// Target is the status interacted with, nil while it is not stored.
	Target *StatusRef
}

// EventType implements uow.Event.
func (InteractionRequested) EventType() string { return EventInteractionRequested }

// InteractionAnswered is emitted when an answer to a request is inserted.
type InteractionAnswered struct {
	Request content.InteractionRequest
	Event   content.InteractionEvent
	State   content.InteractionState
	// Target is the status interacted with, nil while it is not stored.
	Target *StatusRef
	// Deferred is whether the answer to a remote account's interaction with a local status must
	// still be delivered: the author's answer, and the answer to an interaction that came
	// without a request. The handler of a ReplyRequest, AnnounceRequest, or QuoteRequest
	// answers what it decides itself.
	Deferred bool
}

// EventType implements uow.Event.
func (InteractionAnswered) EventType() string { return EventInteractionAnswered }

// StatusReleased is emitted when a held reply is approved.
type StatusReleased struct {
	Status   content.Status
	Revision content.Revision
}

// EventType implements uow.Event.
func (StatusReleased) EventType() string { return EventStatusReleased }

// Published reports whether the release publishes the status: a local reply, which nothing
// showed before (StatusCreated.Unpublished).
func (e StatusReleased) Published() bool { return e.Status.IsLocal }

// RegisterApprovalProjectors registers the synchronous projector of interaction_request_states:
// the state of each request, derived from its answers by content.InteractionStateOf.
// RebuildInteractionStates applies the same rule.
func RegisterApprovalProjectors(r *uow.Projectors, repo ApprovalRepository) {
	r.Register(EventInteractionRequested, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertInteractionState(ctx, tx, e.(InteractionRequested).State)
	})
	r.Register(EventInteractionAnswered, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertInteractionState(ctx, tx, e.(InteractionAnswered).State)
	})
}

// requestRebuildPage is how many requests RebuildInteractionStates reads at a time.
const requestRebuildPage = 500

// RebuildInteractionStates empties interaction_request_states and derives it again from the
// requests and their answers (data-model §9).
func RebuildInteractionStates(ctx context.Context, u uow.UnitOfWork, repo ApprovalRepository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllInteractionStates(ctx, tx); err != nil {
			return err
		}
		for after := ""; ; {
			reqs, err := repo.ListInteractionRequestsAfter(ctx, tx, after, requestRebuildPage)
			if err != nil {
				return err
			}
			for _, r := range reqs {
				events, err := repo.ListInteractionEvents(ctx, tx, r.ID)
				if err != nil {
					return err
				}
				if err := repo.UpsertInteractionState(ctx, tx, content.InteractionStateOf(r, events)); err != nil {
					return err
				}
			}
			if len(reqs) < requestRebuildPage {
				return nil
			}
			after = reqs[len(reqs)-1].ID
		}
	})
	if err != nil {
		return fmt.Errorf("rebuild interaction_request_states: %w", err)
	}
	return nil
}

// approvals writes the records of the approvals in a transaction.
type approvals struct {
	repo  QuoteRepository
	ids   kernel.IDGenerator
	clock kernel.Clock
	host  string
}

// answer is an answer given with a request: its kind, "" for none, and the authorization.
type answer struct {
	kind          content.InteractionEventKind
	authorization string
	deferred      bool
}

// request records in tx the request r with the answer a, unless the interaction has a request
// for the target already. It returns the stored request with its state, and whether r is new.
func (p approvals) request(ctx context.Context, tx uow.Tx, r content.InteractionRequest, a answer, target *StatusRef) (content.InteractionRequest, content.InteractionState, bool, error) {
	if r.ID == "" {
		r.ID = p.ids.NewID()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = p.clock.Now()
	}
	inserted, err := p.repo.InsertInteractionRequest(ctx, tx, r)
	if err != nil {
		return content.InteractionRequest{}, content.InteractionState{}, false, err
	}
	if !inserted {
		stored, err := p.repo.FindInteractionRequestFor(ctx, tx, r.Kind, r.InteractingURI, r.TargetURI)
		if err != nil {
			return content.InteractionRequest{}, content.InteractionState{}, false, err
		}
		st, err := p.state(ctx, tx, stored)
		return stored, st, false, err
	}
	st := content.InitialInteractionState(r)
	var ev *content.InteractionEvent
	if a.kind != "" {
		e := content.InteractionEvent{ID: p.ids.NewID(), RequestID: r.ID, Kind: a.kind, AuthorizationURI: a.authorization, OccurredAt: r.CreatedAt}
		if err := p.repo.InsertInteractionEvent(ctx, tx, e); err != nil {
			return content.InteractionRequest{}, content.InteractionState{}, false, err
		}
		st, ev = st.Apply(e), &e
	}
	if err := tx.Emit(ctx, InteractionRequested{Request: r, State: st, Target: target}); err != nil {
		return content.InteractionRequest{}, content.InteractionState{}, false, err
	}
	if ev != nil {
		if err := tx.Emit(ctx, InteractionAnswered{Request: r, Event: *ev, State: st, Target: target, Deferred: a.deferred}); err != nil {
			return content.InteractionRequest{}, content.InteractionState{}, false, err
		}
	}
	return r, st, true, nil
}

// state returns the current state of r in tx: its projection, or its initial state before it.
func (p approvals) state(ctx context.Context, tx uow.Tx, r content.InteractionRequest) (content.InteractionState, error) {
	st, err := p.repo.FindInteractionState(ctx, tx, r.ID)
	if errors.Is(err, ErrNotFound) {
		return content.InteractionStateOf(r, nil), nil
	}
	return st, err
}

// answer records in tx the answer a to r, unless it changes nothing: the state stays the same.
// It returns the new state and whether it changed.
func (p approvals) answer(ctx context.Context, tx uow.Tx, r content.InteractionRequest, a answer, target *StatusRef) (content.InteractionState, bool, error) {
	cur, err := p.state(ctx, tx, r)
	if err != nil {
		return content.InteractionState{}, false, err
	}
	e := content.InteractionEvent{ID: p.ids.NewID(), RequestID: r.ID, Kind: a.kind, AuthorizationURI: a.authorization, OccurredAt: p.clock.Now()}
	next := cur.Apply(e)
	if next == cur {
		return cur, false, nil
	}
	if err := p.repo.InsertInteractionEvent(ctx, tx, e); err != nil {
		return content.InteractionState{}, false, err
	}
	return next, true, tx.Emit(ctx, InteractionAnswered{Request: r, Event: e, State: next, Target: target, Deferred: a.deferred})
}

// authorize records in tx this server's approval of the interaction interactingURI of kind by
// interactorID with the local status target, and returns its IRI: the one given before when the
// interaction was approved already.
func (p approvals) authorize(ctx context.Context, tx uow.Tx, kind content.InteractionKind, interactingURI, interactorID string, target content.Status) (string, error) {
	a := content.InteractionAuthorization{
		ID: p.ids.NewID(), Kind: kind, InteractingURI: interactingURI, InteractingAccountID: interactorID,
		TargetStatusID: target.ID, CreatedAt: p.clock.Now(),
	}
	inserted, err := p.repo.InsertInteractionAuthorization(ctx, tx, a)
	if err != nil {
		return "", err
	}
	if !inserted {
		stored, err := p.repo.FindInteractionAuthorizationFor(ctx, tx, kind, interactingURI, target.ID)
		if err != nil {
			return "", err
		}
		a = stored
	}
	return content.InteractionAuthorizationURI(p.host, kind, a.ID), nil
}
