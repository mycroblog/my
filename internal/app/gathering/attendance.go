// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// ErrNotAttending reports an approval of an account that does not answer going or maybe.
var ErrNotAttending = errors.New("gathering: the account does not attend the event")

// Answer records the answer of accountID to the event id (ADR-0161 §8): going, maybe, or
// declined, changed as often as the account likes until the event starts. A new answer of going
// to an event whose attendees who answer going reached its capacity fails with ErrFull; maybe
// and declined never do. An answer of declined ends an approval. It fails with ErrNotFound,
// ErrInvalidAnswer, and ErrClosed for an event that started or was cancelled.
func (s *Service) Answer(ctx context.Context, accountID, id string, answer gathering.Answer) (View, error) {
	if !answer.Valid() {
		return View{}, ErrInvalidAnswer
	}
	if _, err := s.Get(ctx, accountID, id); err != nil {
		return View{}, err
	}
	now := s.now()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		// The lock serializes the answers of the event, so that the count of going holds.
		if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
			return err
		}
		rec, err := s.d.Repository.FindGathering(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Cancellation != nil || !now.Before(rec.Head.Schedule.StartsAt) {
			return ErrClosed
		}
		cur, err := s.d.Repository.FindAttendee(ctx, tx, id, accountID)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return err
		case cur.Answer == answer:
			return nil
		}
		capacity := rec.Revision.Settings.Capacity
		if answer == gathering.AnswerGoing && capacity > 0 && rec.Head.GoingCount >= capacity {
			return ErrFull
		}
		r := RSVP{ID: s.d.IDs.NewID(), GatheringID: id, AccountID: accountID, Answer: answer, AnsweredAt: now}
		if err := s.d.Repository.InsertRSVP(ctx, tx, r); err != nil {
			return err
		}
		return tx.Emit(ctx, Answered{RSVP: r})
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, accountID, id)
}

// Attendees returns at most limit attendees of the event id who answer going or maybe, in the
// order of their answers, after the answer afterRSVPID unless it is "", and whether more follow;
// every viewer of the event sees them (ADR-0161 §8). It fails with ErrNotFound.
func (s *Service) Attendees(ctx context.Context, accountID, id, afterRSVPID string, limit int) ([]Attendee, bool, error) {
	if _, err := s.Get(ctx, accountID, id); err != nil {
		return nil, false, err
	}
	as, err := s.d.Repository.Attendees(ctx, nil, id, afterRSVPID, limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(as) > limit {
		return as[:limit], true, nil
	}
	return as, false, nil
}

// Approve approves the answer of accountID, an attendee who answers going or maybe, by
// byAccountID, the organizer or a co-organizer (ADR-0161 §6): the attendee sees the places that
// the event shows to the approved. An approved attendee stays approved. It fails with ErrNotFound,
// ErrNotOrganizer, ErrClosed, and ErrNotAttending.
func (s *Service) Approve(ctx context.Context, byAccountID, id, accountID string) (Attendee, error) {
	return s.changeApproval(ctx, byAccountID, id, accountID, gathering.Approved)
}

// RevokeApproval revokes the approval of accountID, by the organizer or a co-organizer. An
// attendee who is not approved stays so. It fails as Approve does, but for ErrNotAttending, and
// with ErrNotFound for an account that did not answer.
func (s *Service) RevokeApproval(ctx context.Context, byAccountID, id, accountID string) (Attendee, error) {
	return s.changeApproval(ctx, byAccountID, id, accountID, gathering.ApprovalRevoked)
}

func (s *Service) changeApproval(ctx context.Context, byAccountID, id, accountID string, change gathering.ApprovalChange) (Attendee, error) {
	if _, err := s.organizing(ctx, byAccountID, id); err != nil {
		return Attendee{}, err
	}
	var out Attendee
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
			return err
		}
		a, err := s.d.Repository.FindAttendee(ctx, tx, id, accountID)
		switch {
		case errors.Is(err, ErrNotFound) && change == gathering.Approved:
			return ErrNotAttending
		case err != nil:
			return err
		case change == gathering.Approved && !a.Answer.Attending():
			return ErrNotAttending
		case a.Approved == (change == gathering.Approved):
			out = a
			return nil
		}
		ap := Approval{ID: s.d.IDs.NewID(), GatheringID: id, AccountID: accountID, Change: change, ChangedByAccountID: byAccountID, ChangedAt: s.now()}
		if err := s.d.Repository.InsertApproval(ctx, tx, ap); err != nil {
			return err
		}
		if err := tx.Emit(ctx, ApprovalChanged{Approval: ap}); err != nil {
			return err
		}
		out, err = s.d.Repository.FindAttendee(ctx, tx, id, accountID)
		return err
	})
	return out, err
}
