// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
)

// Domain events of the gathering module. A command writes the rows of the event first and then
// emits it in the same transaction; the projectors read those rows, and the notifications of the
// events subscribe to them.
const (
	// EventCreated follows an event, its first revision, and its first date.
	EventCreated = "gathering.created"
	// EventRevised follows a revision of the settings.
	EventRevised = "gathering.revised"
	// EventRescheduled follows a new date of an event.
	EventRescheduled = "gathering.rescheduled"
	// EventInvited follows an invitation.
	EventInvited = "gathering.invited"
	// EventInvitationRevoked follows the revocation of an invitation.
	EventInvitationRevoked = "gathering.invitation_revoked"
	// EventAnswered follows an answer.
	EventAnswered = "gathering.answered"
	// EventApprovalChanged follows an attendee approved or the approval revoked.
	EventApprovalChanged = "gathering.approval_changed"
	// EventCoorganizerChanged follows a co-organizer assigned or removed.
	EventCoorganizerChanged = "gathering.coorganizer_changed"
	// EventCancelled follows the cancellation of an event.
	EventCancelled = "gathering.cancelled"
)

// Created carries an event that was created, with its date (Confirmation), or with the
// candidates of its date poll and the zero Confirmation.
type Created struct {
	Gathering    Gathering
	Revision     Revision
	Confirmation Confirmation
	Candidates   []Candidate
}

// EventType implements uow.Event.
func (Created) EventType() string { return EventCreated }

// Revised carries a revision of the settings and the revision it follows.
type Revised struct {
	Revision Revision
	Previous Revision
}

// EventType implements uow.Event.
func (Revised) EventType() string { return EventRevised }

// Rescheduled carries a new date of an event and the date it replaces.
type Rescheduled struct {
	Confirmation Confirmation
	Previous     Confirmation
}

// EventType implements uow.Event.
func (Rescheduled) EventType() string { return EventRescheduled }

// Invited carries an invitation.
type Invited struct{ Invitation Invitation }

// EventType implements uow.Event.
func (Invited) EventType() string { return EventInvited }

// InvitationRevoked carries the revocation of an invitation.
type InvitationRevoked struct {
	Invitation Invitation
	Revocation InvitationRevocation
}

// EventType implements uow.Event.
func (InvitationRevoked) EventType() string { return EventInvitationRevoked }

// Answered carries an answer.
type Answered struct{ RSVP RSVP }

// EventType implements uow.Event.
func (Answered) EventType() string { return EventAnswered }

// ApprovalChanged carries an approval or its revocation.
type ApprovalChanged struct{ Approval Approval }

// EventType implements uow.Event.
func (ApprovalChanged) EventType() string { return EventApprovalChanged }

// CoorganizerChanged carries a co-organizer assigned or removed.
type CoorganizerChanged struct{ Change CoorganizerChange }

// EventType implements uow.Event.
func (CoorganizerChanged) EventType() string { return EventCoorganizerChanged }

// Cancelled carries the cancellation of an event.
type Cancelled struct{ Cancellation Cancellation }

// EventType implements uow.Event.
func (Cancelled) EventType() string { return EventCancelled }

// RegisterProjectors registers the projectors of the module's own projections: each event that
// changes them derives again the rows of gathering_heads, gathering_attendees, and
// gathering_coorganizers that it touches, with the same rules as RebuildProjections.
func RegisterProjectors(r *uow.Projectors, repo ProjectionRepository) {
	r.Register(EventCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(Created).Gathering.ID)
	})
	r.Register(EventRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(Revised).Revision.GatheringID)
	})
	r.Register(EventRescheduled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(Rescheduled).Confirmation.GatheringID)
	})
	r.Register(EventCancelled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(Cancelled).Cancellation.GatheringID)
	})
	r.Register(EventAnswered, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		a := e.(Answered).RSVP
		// The head counts the attendees, so they are derived first.
		if err := repo.RefreshAttendee(ctx, tx, a.GatheringID, a.AccountID); err != nil {
			return err
		}
		return repo.RefreshHead(ctx, tx, a.GatheringID)
	})
	r.Register(EventApprovalChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		a := e.(ApprovalChanged).Approval
		return repo.RefreshAttendee(ctx, tx, a.GatheringID, a.AccountID)
	})
	r.Register(EventCoorganizerChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshCoorganizers(ctx, tx, e.(CoorganizerChanged).Change.GatheringID)
	})
	registerPollProjectors(r, repo)
}

// RebuildProjections derives gathering_attendees, gathering_heads, and gathering_coorganizers
// again from the events, in one transaction.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo ProjectionRepository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildProjections(ctx, tx) })
	if err != nil {
		return fmt.Errorf("rebuild the gathering projections: %w", err)
	}
	return nil
}
