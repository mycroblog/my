// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The moderation of the events (ADR-0161 §14): the target of a report of an event, a moderator
// cancelling an event with its audit log entry, the records of a reported event that the
// moderators read, and the cancellation of the upcoming events of an organizer who was suspended
// or deleted. The retention of the answers and the invitations is in retention.go.

// JobCancelOrganized cancels the events that an account organizes and that have not started,
// after the account was suspended or deleted (ADR-0161 §14); the payload is cancelPayload.
const JobCancelOrganized = "gathering.cancel_organized"

// cancelOrganizedBatch is the number of events that one read of JobCancelOrganized cancels; the
// job reads again until none is left.
const cancelOrganizedBatch = 100

var errInvalidPayload = errors.New("gathering: invalid job payload")

// ModerationRepository is the part of Repository that the moderation of the events uses.
type ModerationRepository interface {
	// Revisions returns the revisions of the settings of an event with their places, oldest
	// first.
	Revisions(ctx context.Context, tx uow.Tx, gatheringID string) ([]Revision, error)
	// OrganizedUpcoming returns at most limit events, in ID order, that accountID organizes,
	// that were not cancelled, and that have not started at now.
	OrganizedUpcoming(ctx context.Context, tx uow.Tx, accountID string, now time.Time, limit int) ([]string, error)
}

// ReportTarget is what a report of an event points at: the event, the revision of its settings
// that the reporter sees, and its organizer, whom the report targets (ADR-0161 §14).
type ReportTarget struct {
	GatheringID        string
	RevisionID         string
	OrganizerAccountID string
}

// ReportTarget returns the target of a report of an event that accountID sees
// (gathering.CanSee), a cancelled one included; ErrNotFound for any other event.
func (s *Service) ReportTarget(ctx context.Context, accountID, id string) (ReportTarget, error) {
	v, err := s.Get(ctx, accountID, id)
	if err != nil {
		return ReportTarget{}, err
	}
	return ReportTarget{GatheringID: v.ID, RevisionID: v.Revision.ID, OrganizerAccountID: v.OrganizerAccountID}, nil
}

// ModeratorCancelInput is a moderator cancelling an event.
type ModeratorCancelInput struct {
	GatheringID string
	// ByUserID and ByAccountID are the moderator's user, whom the audit log names, and local
	// account, whom the cancellation names.
	ByUserID    string
	ByAccountID string
	// Note is the reason, plain text of at most gathering.MaxNoteLength characters, "" for none.
	// The event shows it with its cancellation.
	Note string
}

// CancelByModerator cancels an event for a holder of manage_reports, whom the caller checked
// (moderator, ADR-0161 §14): the cancellation records the moderator and the reason, and the audit
// log records event_cancelled in the same transaction, labelled with the event's ID: the title
// goes with the erasure of the account that wrote it, and the moderators read it from the report.
// The attendees and the invited users are notified as of any cancellation. An unknown event fails
// with ErrNotFound, one cancelled already with ErrClosed, and a reason that does not hold with a
// *gathering.SettingsError of the note.
func (s *Service) CancelByModerator(ctx context.Context, in ModeratorCancelInput) error {
	if !gathering.ValidNote(in.Note) {
		return &gathering.SettingsError{Field: gathering.FieldNote}
	}
	if _, err := s.d.Repository.FindGathering(ctx, nil, in.GatheringID); err != nil {
		return err
	}
	now := s.now()
	actor, user := moderation.AuditActor(in.ByUserID)
	auditID := s.d.IDs.NewID()
	c := Cancellation{
		GatheringID: in.GatheringID, Reason: gathering.CancelModerator, CancelledByAccountID: in.ByAccountID,
		Note: in.Note, CancelledAt: now,
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockGathering(ctx, tx, in.GatheringID); err != nil {
			return err
		}
		ok, err := s.d.Repository.InsertCancellation(ctx, tx, c)
		if err != nil {
			return err
		}
		if !ok {
			return ErrClosed
		}
		if err := tx.Emit(ctx, Cancelled{Cancellation: c}); err != nil {
			return err
		}
		return tx.Emit(ctx, moderation.AuditRecorded{Entry: moderation.AuditEntry{
			ID: auditID, ActorKind: actor, ActorUserID: user, Action: moderation.AuditEventCancelled,
			TargetKind: moderation.AuditTargetEvent, TargetID: in.GatheringID,
			TargetLabel: in.GatheringID, SourceEventID: in.GatheringID, OccurredAt: now,
		}})
	})
}

// EventRecords are what the moderators read of a reported event (ADR-0161 §14): the event with
// its current date and its cancellation, and every revision of its settings with all its places,
// whoever they were shown to.
type EventRecords struct {
	Record
	// Revisions are the revisions of the settings, oldest first.
	Revisions []Revision
}

// EventRecords returns the records of an event for the moderators who read a report of it;
// ErrNotFound for an unknown event.
func (s *Service) EventRecords(ctx context.Context, id string) (EventRecords, error) {
	rec, err := s.d.Repository.FindGathering(ctx, nil, id)
	if err != nil {
		return EventRecords{}, err
	}
	revs, err := s.d.Repository.Revisions(ctx, nil, id)
	if err != nil {
		return EventRecords{}, err
	}
	return EventRecords{Record: rec, Revisions: revs}, nil
}

// cancelPayload is the payload of JobCancelOrganized: the organizer and why its events end.
type cancelPayload struct {
	Account string                 `json:"account"`
	Reason  gathering.CancelReason `json:"reason"`
}

// RegisterModerationProjectors subscribes the events to the suspensions of
// moderation.account_suspension_changed and to the deletions of account.lifecycle_event_recorded:
// each enqueues JobCancelOrganized in the transaction of its event for an account that organizes
// an event that has not started. A lifted suspension restores nothing.
func (s *Service) RegisterModerationProjectors(r *uow.Projectors) {
	r.Register(moderation.EventAccountSuspensionChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(moderation.AccountSuspensionChanged)
		if !ev.Suspended {
			return nil
		}
		return s.enqueueCancelOrganized(ctx, tx, ev.AccountID, gathering.CancelOrganizerSuspended,
			"suspension:"+strconv.FormatInt(ev.ChangedAt.UnixNano(), 10))
	})
	r.Register(account.EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(account.LifecycleEventRecorded)
		if !ev.ClosesLocalAccount() {
			return nil
		}
		return s.enqueueCancelOrganized(ctx, tx, ev.Event.AccountID, gathering.CancelOrganizerDeleted, "lifecycle:"+ev.Event.ID)
	})
}

// enqueueCancelOrganized enqueues JobCancelOrganized in tx when accountID organizes an event that
// has not started.
func (s *Service) enqueueCancelOrganized(ctx context.Context, tx uow.Tx, accountID string, reason gathering.CancelReason, cause string) error {
	if s.d.Outbox == nil {
		return nil
	}
	ids, err := s.d.Repository.OrganizedUpcoming(ctx, tx, accountID, s.now(), 1)
	if err != nil || len(ids) == 0 {
		return err
	}
	b, err := json.Marshal(cancelPayload{Account: accountID, Reason: reason})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobCancelOrganized, Payload: b, IdempotencyKey: JobCancelOrganized + ":" + accountID + ":" + cause,
	})
}

// RegisterModerationJobs registers the handlers of JobCancelOrganized and JobPurgeRecords on
// reg, and the schedule of JobPurgeRecords on sched unless it is nil.
func (s *Service) RegisterModerationJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobCancelOrganized, s.runCancelOrganized)
	s.registerPurgeJob(reg, sched)
}

// runCancelOrganized runs JobCancelOrganized.
func (s *Service) runCancelOrganized(ctx context.Context, j queue.Job) queue.Result {
	var p cancelPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("decode %s: %w", JobCancelOrganized, err))
	}
	if !p.Reason.Valid() || p.Account == "" {
		return queue.Fail(fmt.Errorf("%s: %w", JobCancelOrganized, errInvalidPayload))
	}
	if _, err := s.CancelOrganized(ctx, p.Account, p.Reason); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// CancelOrganized cancels, with reason and no one who cancelled them, the events that accountID
// organizes and that have not started, one transaction for each, and returns how many it
// cancelled. It cancels nothing while the account is active again, that is, while it is neither
// suspended nor deleted: a suspension lifted before the job ran keeps the events. Each
// cancellation notifies as the organizer's own does. It is safe to repeat.
func (s *Service) CancelOrganized(ctx context.Context, accountID string, reason gathering.CancelReason) (int, error) {
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if m.Active {
		return 0, nil
	}
	n := 0
	for {
		now := s.now()
		ids, err := s.d.Repository.OrganizedUpcoming(ctx, nil, accountID, now, cancelOrganizedBatch)
		if err != nil {
			return n, err
		}
		for _, id := range ids {
			c := Cancellation{GatheringID: id, Reason: reason, CancelledAt: now}
			var did bool
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
					return err
				}
				ok, err := s.d.Repository.InsertCancellation(ctx, tx, c)
				if did = ok; err != nil || !ok {
					return err
				}
				return tx.Emit(ctx, Cancelled{Cancellation: c})
			})
			if err != nil {
				return n, fmt.Errorf("gathering: cancel the event %s of its organizer: %w", id, err)
			}
			if did {
				n++
			}
		}
		if len(ids) < cancelOrganizedBatch {
			return n, nil
		}
	}
}
