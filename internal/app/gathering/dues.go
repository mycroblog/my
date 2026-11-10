// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The due times of the events and the reminders of the accounts (ADR-0161 §12, §13):
// gathering_dues holds, for each event with something ahead, the next time that gathering.tick
// handles it, a reminder or the start. The commands derive it again in their transactions: the
// creation and a new date from scratch, an answer and a change of the reminders of an attendee
// keeping a time that is already due, and the cancellation by deleting it.

// Due is the next due time of an event (gathering_dues).
type Due struct {
	GatheringID string
	At          time.Time
}

// DueRepository is the persistence of the due times, of the reminders that the accounts chose,
// and of the recipients of the notifications of the events.
type DueRepository interface {
	// FindDue returns the next due time of an event; ok is false when it has none.
	FindDue(ctx context.Context, tx uow.Tx, gatheringID string) (at time.Time, ok bool, err error)
	// SetDue writes the next due time of an event.
	SetDue(ctx context.Context, tx uow.Tx, gatheringID string, at time.Time) error
	// DeleteDue removes the due time of an event, if any.
	DeleteDue(ctx context.Context, tx uow.Tx, gatheringID string) error
	// DeleteAllDues empties gathering_dues, for a rebuild.
	DeleteAllDues(ctx context.Context, tx uow.Tx) error
	// DueGatherings returns at most limit events whose next due time is at or before now, the
	// earliest first.
	DueGatherings(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]Due, error)

	// AttendeeReminders returns the reminders that the attendees of an event who answer going or
	// maybe chose, in minutes before the start, each once, and whether one of them has not chosen,
	// so that the defaults apply to it.
	AttendeeReminders(ctx context.Context, tx uow.Tx, gatheringID string) (minutes []int, defaults bool, err error)
	// Reminders returns the reminders of each of accountIDs that chose them, by account, in
	// minutes before the start from the earliest; an account that chose none has an empty slice,
	// and one that has not chosen has no entry.
	Reminders(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string][]int, error)
	// SetReminders records the reminders that an account chose, none included.
	SetReminders(ctx context.Context, tx uow.Tx, accountID string, minutes []int, at time.Time) error
	// DeleteReminders forgets the reminders of an account, which then has the defaults.
	DeleteReminders(ctx context.Context, tx uow.Tx, accountID string) error
	// AttendingGatherings returns at most limit events that are scheduled and have not started at
	// now, that accountID answers going or maybe, by the nearest start first.
	AttendingGatherings(ctx context.Context, tx uow.Tx, accountID string, now time.Time, limit int) ([]string, error)

	// Audience returns at most limit accounts with IDs after after ("" for the first) that answer
	// the event going or maybe, and with invitees those invited to it by an invitation not revoked,
	// each once, in the order of their IDs.
	Audience(ctx context.Context, tx uow.Tx, gatheringID string, invitees bool, after string, limit int) ([]string, error)
	// AudienceFacts reads what the records of an event say of each of accountIDs, by account, with
	// one query.
	AudienceFacts(ctx context.Context, tx uow.Tx, gatheringID string, accountIDs []string) (map[string]ViewerFacts, error)
}

// remindersOf returns the reminders of an account from what Reminders read: its own, or the
// defaults.
func remindersOf(chosen map[string][]int, accountID string) []int {
	if m, ok := chosen[accountID]; ok {
		return m
	}
	return gathering.DefaultReminders
}

// duesOf returns what the event rec has to do (gathering.Dues), with the reminders of its
// attendees.
func duesOf(ctx context.Context, tx uow.Tx, repo DueRepository, rec Record) ([]gathering.Due, error) {
	minutes, defaults, err := repo.AttendeeReminders(ctx, tx, rec.ID)
	if err != nil {
		return nil, err
	}
	if defaults {
		minutes = append(minutes, gathering.DefaultReminders...)
	}
	return gathering.Dues(rec.Confirmation.Schedule.StartsAt, minutes), nil
}

// refreshDue derives the due time of the event id again inside tx, as of now. Unless fresh, a due
// time that came already stays, so that the next gathering.tick still handles what was due before
// the change; fresh (the creation and a new date) derives it from now.
func refreshDue(ctx context.Context, tx uow.Tx, repo Repository, id string, now time.Time, fresh bool) error {
	rec, err := repo.FindGathering(ctx, tx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if rec.Cancellation != nil || !rec.Scheduled() {
		return repo.DeleteDue(ctx, tx, id)
	}
	if !fresh {
		at, ok, err := repo.FindDue(ctx, tx, id)
		if err != nil {
			return err
		}
		if ok && !at.After(now) {
			return nil
		}
	}
	dues, err := duesOf(ctx, tx, repo, rec)
	if err != nil {
		return err
	}
	next, ok := gathering.NextDue(dues, now)
	if !ok {
		return repo.DeleteDue(ctx, tx, id)
	}
	return repo.SetDue(ctx, tx, id, next)
}

// RegisterDueProjectors registers the projectors that keep gathering_dues: the creation, a new
// date, and the confirmation of a date poll derive the due time from scratch, an answer derives it again with the attendee's
// reminders, and the cancellation deletes it. They run after RegisterProjectors, whose
// projections they read.
func RegisterDueProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		g := e.(Created).Gathering
		return refreshDue(ctx, tx, repo, g.ID, g.CreatedAt, true)
	})
	r.Register(EventRescheduled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(Rescheduled).Confirmation
		return refreshDue(ctx, tx, repo, c.GatheringID, c.ConfirmedAt, true)
	})
	r.Register(EventAnswered, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		a := e.(Answered).RSVP
		return refreshDue(ctx, tx, repo, a.GatheringID, a.AnsweredAt, false)
	})
	r.Register(EventCancelled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteDue(ctx, tx, e.(Cancelled).Cancellation.GatheringID)
	})
	registerPollDueProjectors(r, repo)
}

// rebuildBatch is how many events RebuildDues reads at a time.
const rebuildBatch = 500

// RebuildDues empties gathering_dues and derives it again, as of now, from the events that are
// scheduled and have not ended, in one transaction (data-model §9).
func RebuildDues(ctx context.Context, u uow.UnitOfWork, repo Repository, now time.Time) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllDues(ctx, tx); err != nil {
			return err
		}
		var after *ListKey
		for {
			recs, err := repo.UpcomingGatherings(ctx, tx, now, after, rebuildBatch)
			if err != nil {
				return err
			}
			for _, rec := range recs {
				if err := refreshDue(ctx, tx, repo, rec.ID, now, true); err != nil {
					return err
				}
			}
			if len(recs) < rebuildBatch {
				return nil
			}
			last := recs[len(recs)-1]
			after = &ListKey{StartsAt: last.Head.Schedule.StartsAt, GatheringID: last.ID}
		}
	})
	if err != nil {
		return fmt.Errorf("rebuild gathering_dues: %w", err)
	}
	return nil
}

// Reminders are the reminders of an account: in minutes before the start, from the earliest, and
// whether they are the defaults because the account has not chosen.
type Reminders struct {
	Minutes  []int
	Defaults bool
}

// Reminders returns the reminders of accountID (ADR-0161 §12).
func (s *Service) Reminders(ctx context.Context, accountID string) (Reminders, error) {
	chosen, err := s.d.Repository.Reminders(ctx, nil, []string{accountID})
	if err != nil {
		return Reminders{}, err
	}
	m, ok := chosen[accountID]
	if !ok {
		return Reminders{Minutes: slices.Clone(gathering.DefaultReminders), Defaults: true}, nil
	}
	return Reminders{Minutes: m}, nil
}

// attendingBatch bounds the events whose due times a change of the reminders derives again; an
// account that answers more upcoming events gets the change on those from their next answer or
// date.
const attendingBatch = 1000

// SetReminders changes the reminders of accountID to minutes (none included; nil restores the
// defaults), and derives again the due times of the upcoming events it answers going or maybe. It
// fails with gathering.ErrInvalidReminders.
func (s *Service) SetReminders(ctx context.Context, accountID string, minutes []int) (Reminders, error) {
	var normalized []int
	if minutes != nil {
		var err error
		if normalized, err = gathering.NormalizeReminders(minutes); err != nil {
			return Reminders{}, err
		}
	}
	now := s.now()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if minutes == nil {
			err = s.d.Repository.DeleteReminders(ctx, tx, accountID)
		} else {
			err = s.d.Repository.SetReminders(ctx, tx, accountID, normalized, now)
		}
		if err != nil {
			return err
		}
		ids, err := s.d.Repository.AttendingGatherings(ctx, tx, accountID, now, attendingBatch)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := refreshDue(ctx, tx, s.d.Repository, id, now, false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Reminders{}, err
	}
	return s.Reminders(ctx, accountID)
}
