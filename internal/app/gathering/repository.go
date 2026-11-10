// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// ErrNotFound reports an event, an attendee, or an invitation that does not exist; the use cases
// also report an event that the viewer does not see with it.
var ErrNotFound = errors.New("gathering: not found")

// Gathering is an event (gatherings).
type Gathering struct {
	ID                 string
	OrganizerAccountID string
	CreatedAt          time.Time
}

// Revision is a revision of the settings of an event (gathering_revisions and its places).
type Revision struct {
	ID                 string
	GatheringID        string
	Settings           gathering.Settings
	RevisedByAccountID string
	RevisedAt          time.Time
}

// Confirmation is the date of an event fixed (gathering_schedule_confirmations).
type Confirmation struct {
	ID                   string
	GatheringID          string
	Schedule             gathering.Schedule
	ConfirmedByAccountID string
	ConfirmedAt          time.Time
}

// Invitation is an invitation to an event (gathering_invitations).
type Invitation struct {
	ID                 string
	GatheringID        string
	InviteeAccountID   string
	InvitedByAccountID string
	InvitedAt          time.Time
}

// InvitationRevocation is the revocation of an invitation (gathering_invitation_revocations).
type InvitationRevocation struct {
	InvitationID       string
	RevokedByAccountID string
	RevokedAt          time.Time
}

// InvitationRecord is an invitation with whether it was revoked.
type InvitationRecord struct {
	Invitation
	Revoked bool
}

// RSVP is an answer of an account to an event (gathering_rsvps).
type RSVP struct {
	ID          string
	GatheringID string
	AccountID   string
	Answer      gathering.Answer
	AnsweredAt  time.Time
}

// Approval is an attendee approved or the approval revoked (gathering_rsvp_approvals).
type Approval struct {
	ID                 string
	GatheringID        string
	AccountID          string
	Change             gathering.ApprovalChange
	ChangedByAccountID string
	ChangedAt          time.Time
}

// CoorganizerChange is a co-organizer assigned or removed (gathering_coorganizer_changes).
type CoorganizerChange struct {
	ID          string
	GatheringID string
	AccountID   string
	Change      gathering.CoorganizerChange
	ChangedAt   time.Time
}

// Cancellation is the cancellation of an event (gathering_cancellations).
type Cancellation struct {
	GatheringID string
	Reason      gathering.CancelReason
	// CancelledByAccountID is "" for an automatic cancellation.
	CancelledByAccountID string
	// Note is the reason that the organizer or a moderator gave, "" for none.
	Note        string
	CancelledAt time.Time
}

// Head is the current revision and date of an event, its state, and how many attendees answer
// going (gathering_heads).
type Head struct {
	GatheringID        string
	OrganizerAccountID string
	RevisionID         string
	ConfirmationID     string
	State              gathering.State
	Schedule           gathering.Schedule
	GoingCount         int
}

// Attendee is the current answer of an account to an event (gathering_attendees).
type Attendee struct {
	GatheringID string
	AccountID   string
	// RSVPID is the newest answer, whose ID orders the attendee list.
	RSVPID   string
	Answer   gathering.Answer
	Approved bool
}

// Coorganizer is a current co-organizer of an event (gathering_coorganizers); ChangeID, the
// assignment, orders the nominations.
type Coorganizer struct {
	AccountID string
	ChangeID  string
}

// Record is an event with its head, its current revision and date, its co-organizers in the
// order of their nominations, its cancellation, and how many revisions it has.
type Record struct {
	Gathering
	Head         Head
	Revision     Revision
	Confirmation Confirmation
	Coorganizers []Coorganizer
	// Cancellation is nil for an event that was not cancelled.
	Cancellation *Cancellation
	// Revisions counts the revisions of the settings, the first included.
	Revisions int
}

// Event returns what the policy functions read of the event.
func (r Record) Event() gathering.Event {
	s := r.Revision.Settings
	return gathering.Event{OrganizerAccountID: r.OrganizerAccountID, Visibility: s.Visibility, PlaceDisclosure: s.PlaceDisclosure}
}

// Coorganizes reports whether accountID is a co-organizer of the event.
func (r Record) Coorganizes(accountID string) bool {
	for _, c := range r.Coorganizers {
		if c.AccountID == accountID {
			return true
		}
	}
	return false
}

// organizerIDs returns the organizer and the co-organizers, in that order.
func (r Record) organizerIDs() []string {
	ids := []string{r.OrganizerAccountID}
	for _, c := range r.Coorganizers {
		ids = append(ids, c.AccountID)
	}
	return ids
}

// ListKey is the position of an event in the list of the upcoming events: by the nearest start,
// then by ID.
type ListKey struct {
	StartsAt    time.Time
	GatheringID string
}

// ViewerFacts are what the event's own records say of an account: invited by an invitation not
// revoked, its current answer ("" for none), and whether the answer is approved.
type ViewerFacts struct {
	Invited  bool
	Answer   gathering.Answer
	Approved bool
}

// Repository is the persistence port of the gathering module (data-model §4.19). The writes take
// the transaction of the command; the reads run in tx, or outside any transaction when tx is nil.
type Repository interface {
	ProjectionRepository
	DueRepository

	// InsertGathering records an event.
	InsertGathering(ctx context.Context, tx uow.Tx, g Gathering) error
	// InsertRevision records a revision of the settings with its places in their order.
	InsertRevision(ctx context.Context, tx uow.Tx, r Revision) error
	// InsertConfirmation records the date of an event fixed.
	InsertConfirmation(ctx context.Context, tx uow.Tx, c Confirmation) error
	// InsertInvitation records an invitation and reports whether it did: an account invited to the
	// event already, revoked or not, keeps its first invitation.
	InsertInvitation(ctx context.Context, tx uow.Tx, i Invitation) (bool, error)
	// InsertInvitationRevocation revokes an invitation and reports whether it did: a revoked
	// invitation keeps its first revocation.
	InsertInvitationRevocation(ctx context.Context, tx uow.Tx, r InvitationRevocation) (bool, error)
	// InsertRSVP records an answer.
	InsertRSVP(ctx context.Context, tx uow.Tx, r RSVP) error
	// InsertApproval records an approval or its revocation.
	InsertApproval(ctx context.Context, tx uow.Tx, a Approval) error
	// InsertCoorganizerChange records a co-organizer assigned or removed.
	InsertCoorganizerChange(ctx context.Context, tx uow.Tx, c CoorganizerChange) error
	// InsertCancellation cancels an event and reports whether it did: an event cancelled already
	// keeps its first cancellation.
	InsertCancellation(ctx context.Context, tx uow.Tx, c Cancellation) (bool, error)

	// LockGathering locks an event until tx ends, so that the commands of one event serialize
	// (the capacity holds); the transaction reads the event after it. A database whose write
	// transactions run one at a time takes no lock.
	LockGathering(ctx context.Context, tx uow.Tx, id string) error

	// FindGathering returns an event as a Record; ErrNotFound for an unknown event.
	FindGathering(ctx context.Context, tx uow.Tx, id string) (Record, error)
	// UpcomingGatherings returns at most limit events that are scheduled and have not ended at
	// now, by the nearest start first and then by ID, after the position after unless it is nil.
	UpcomingGatherings(ctx context.Context, tx uow.Tx, now time.Time, after *ListKey, limit int) ([]Record, error)
	// FindAttendee returns the current answer of an account; ErrNotFound when it has not answered.
	FindAttendee(ctx context.Context, tx uow.Tx, gatheringID, accountID string) (Attendee, error)
	// Attendees returns at most limit attendees of an event who answer going or maybe, in the
	// order of their answers, after the answer afterRSVPID unless it is "".
	Attendees(ctx context.Context, tx uow.Tx, gatheringID, afterRSVPID string, limit int) ([]Attendee, error)
	// FindInvitation returns the invitation of an account to an event; ErrNotFound without one.
	FindInvitation(ctx context.Context, tx uow.Tx, gatheringID, accountID string) (InvitationRecord, error)
	// Invitations returns at most limit invitations of an event that were not revoked, in the
	// order they were made, after the invitation afterID unless it is "".
	Invitations(ctx context.Context, tx uow.Tx, gatheringID, afterID string, limit int) ([]Invitation, error)
	// ViewerFacts reads what the records of each of gatheringIDs say of accountID, by event, with
	// one query.
	ViewerFacts(ctx context.Context, tx uow.Tx, accountID string, gatheringIDs []string) (map[string]ViewerFacts, error)

	// EraseRevisions empties the title and the description, and deletes the places, of at most
	// limit revisions that accountID wrote and that still hold any of them, and returns how many
	// it erased (ADR-0161 §14).
	EraseRevisions(ctx context.Context, tx uow.Tx, accountID string, limit int) (int, error)
}

// ProjectionRepository derives the projections of the gathering module (data-model §9) from the
// events: gathering_attendees from the answers and the approvals, gathering_heads from the events,
// their revisions, confirmations, cancellations, and attendees, and gathering_coorganizers from
// the co-organizer changes.
type ProjectionRepository interface {
	// RefreshHead derives the head of an event again.
	RefreshHead(ctx context.Context, tx uow.Tx, gatheringID string) error
	// RefreshAttendee derives again the attendee row of an account of an event.
	RefreshAttendee(ctx context.Context, tx uow.Tx, gatheringID, accountID string) error
	// RefreshCoorganizers derives again the co-organizer rows of an event.
	RefreshCoorganizers(ctx context.Context, tx uow.Tx, gatheringID string) error
	// RebuildProjections empties gathering_attendees, gathering_heads, and gathering_coorganizers
	// and derives them again from every event.
	RebuildProjections(ctx context.Context, tx uow.Tx) error
}
