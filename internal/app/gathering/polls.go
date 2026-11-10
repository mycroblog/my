// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The date polls of the events (ADR-0161 §7). An event created with candidates instead of a date
// is unscheduled: the people who see it answer each candidate yes, maybe, or no, and change their
// answers until the organizer or a co-organizer confirms one candidate. The confirmation is a
// gathering_schedule_confirmations row like any date, so the head, the due times of the
// reminders and the start, and the notifications follow it; it tells the voters and the invitees
// with event_schedule_confirmed. A later date is a new confirmation without a candidate.

// The errors of the date polls.
var (
	// ErrUnscheduled reports an operation that needs a date on an event whose date poll has not
	// been confirmed yet: an answer, and the calendar file.
	ErrUnscheduled = errors.New("gathering: the event has no date yet")
	// ErrScheduled reports an operation of the date poll on an event whose date was confirmed:
	// the poll takes no more candidates, votes, or confirmations.
	ErrScheduled = errors.New("gathering: the date of the event was confirmed")
	// ErrLastCandidate reports the withdrawal of the last candidate of a date poll, which would
	// leave the event without a date or a candidate.
	ErrLastCandidate = errors.New("gathering: the last candidate of a date poll stays")
	// ErrInvalidVote reports a vote that is not yes, maybe, or no.
	ErrInvalidVote = errors.New("gathering: invalid vote")
)

// Candidate is a candidate date of an event's date poll (gathering_candidates).
type Candidate struct {
	ID               string
	GatheringID      string
	Schedule         gathering.Schedule
	AddedByAccountID string
	AddedAt          time.Time
}

// CandidateWithdrawal is the withdrawal of a candidate (gathering_candidate_withdrawals).
type CandidateWithdrawal struct {
	CandidateID          string
	WithdrawnByAccountID string
	WithdrawnAt          time.Time
}

// CandidateRecord is a candidate with whether it was withdrawn and whether a confirmation fixed
// it.
type CandidateRecord struct {
	Candidate
	Withdrawn bool
	Confirmed bool
}

// CandidateTally is a candidate that was not withdrawn as a viewer sees it: how many accounts
// answered it each way, the viewer's own answer ("" for none), and whether a confirmation fixed
// it.
type CandidateTally struct {
	Candidate
	Yes, Maybe, No int
	ViewerVote     gathering.Vote
	Confirmed      bool
}

// Ballot is the answer of an account to a candidate (gathering_votes, a current value).
type Ballot struct {
	CandidateID string
	GatheringID string
	AccountID   string
	Vote        gathering.Vote
	VotedAt     time.Time
}

// PollRepository is the persistence of the date polls.
type PollRepository interface {
	// InsertCandidate records a candidate.
	InsertCandidate(ctx context.Context, tx uow.Tx, c Candidate) error
	// InsertCandidateWithdrawal withdraws a candidate and reports whether it did: a withdrawn
	// candidate keeps its first withdrawal.
	InsertCandidateWithdrawal(ctx context.Context, tx uow.Tx, w CandidateWithdrawal) (bool, error)
	// FindCandidate returns the candidate id of an event; ErrNotFound for an unknown one.
	FindCandidate(ctx context.Context, tx uow.Tx, gatheringID, id string) (CandidateRecord, error)
	// Candidates returns the candidates of an event that were not withdrawn, by the earliest
	// start, with the answers of accountID ("" for nobody).
	Candidates(ctx context.Context, tx uow.Tx, gatheringID, accountID string) ([]CandidateTally, error)
	// SetVote records the answer of an account to a candidate, replacing its previous one.
	SetVote(ctx context.Context, tx uow.Tx, b Ballot) error
	// Ballots returns at most limit answers to a candidate, of vote unless it is "", of the
	// accounts with IDs after afterAccountID ("" for the first), in the order of the account IDs.
	Ballots(ctx context.Context, tx uow.Tx, candidateID string, vote gathering.Vote, afterAccountID string, limit int) ([]Ballot, error)
	// InsertConfirmationCandidate records the candidate that a confirmation fixed.
	InsertConfirmationCandidate(ctx context.Context, tx uow.Tx, confirmationID, candidateID string) error
	// PollAudience returns at most limit accounts with IDs after after ("" for the first) that
	// voted in the date poll of an event or are invited to it by an invitation not revoked, each
	// once, in the order of their IDs.
	PollAudience(ctx context.Context, tx uow.Tx, gatheringID, after string, limit int) ([]string, error)
}

// Domain events of the date polls.
const (
	// EventCandidateAdded follows a candidate added to a date poll.
	EventCandidateAdded = "gathering.candidate_added"
	// EventCandidateWithdrawn follows the withdrawal of a candidate.
	EventCandidateWithdrawn = "gathering.candidate_withdrawn"
	// EventScheduleConfirmed follows the first date of an unscheduled event: a candidate
	// confirmed, or a date set without one.
	EventScheduleConfirmed = "gathering.schedule_confirmed"
)

// CandidateAdded carries a candidate added to the date poll of an event that exists already.
type CandidateAdded struct{ Candidate Candidate }

// EventType implements uow.Event.
func (CandidateAdded) EventType() string { return EventCandidateAdded }

// CandidateWithdrawn carries the withdrawal of a candidate.
type CandidateWithdrawn struct {
	Candidate  Candidate
	Withdrawal CandidateWithdrawal
}

// EventType implements uow.Event.
func (CandidateWithdrawn) EventType() string { return EventCandidateWithdrawn }

// ScheduleConfirmed carries the first date of an event and the candidate it came from ("" for a
// date set without one).
type ScheduleConfirmed struct {
	Confirmation Confirmation
	CandidateID  string
}

// EventType implements uow.Event.
func (ScheduleConfirmed) EventType() string { return EventScheduleConfirmed }

// Scheduled reports whether the event has a confirmed date; an event that polls its date has
// none until a candidate is confirmed.
func (r Record) Scheduled() bool { return r.Confirmation.ID != "" }

// registerPollProjectors registers the head of the event again after each change of its date
// poll (RegisterProjectors).
func registerPollProjectors(r *uow.Projectors, repo ProjectionRepository) {
	r.Register(EventCandidateAdded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(CandidateAdded).Candidate.GatheringID)
	})
	r.Register(EventCandidateWithdrawn, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(CandidateWithdrawn).Candidate.GatheringID)
	})
	r.Register(EventScheduleConfirmed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(ScheduleConfirmed).Confirmation.GatheringID)
	})
}

// registerPollDueProjectors derives the due times of an event from scratch when its date is
// confirmed, so that the reminders and the start follow it (RegisterDueProjectors).
func registerPollDueProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventScheduleConfirmed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(ScheduleConfirmed).Confirmation
		return refreshDue(ctx, tx, repo, c.GatheringID, c.ConfirmedAt, true)
	})
}

// registerPollNotificationProjectors enqueues the event_schedule_confirmed of a confirmation
// (RegisterNotificationProjectors).
func (s *Service) registerPollNotificationProjectors(r *uow.Projectors) {
	r.Register(EventScheduleConfirmed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(ScheduleConfirmed).Confirmation
		rec, err := s.d.Repository.FindGathering(ctx, tx, c.GatheringID)
		if err != nil {
			return err
		}
		return s.enqueueNotify(ctx, tx, notifyPayload{
			Kind: appnotification.EventScheduleConfirmed, Gathering: c.GatheringID, Ref: c.ID,
			Source: rec.OrganizerAccountID, Actor: c.ConfirmedByAccountID, At: c.ConfirmedAt,
		})
	})
}

// audience returns a page of the candidate recipients of the notification p: the voters and the
// invitees of a confirmation, and otherwise the attendees (and the invitees, by kind).
func (s *Service) audience(ctx context.Context, p notifyPayload, after string, limit int) ([]string, error) {
	if p.Kind == appnotification.EventScheduleConfirmed {
		return s.d.Repository.PollAudience(ctx, nil, p.Gathering, after, limit)
	}
	return s.d.Repository.Audience(ctx, nil, p.Gathering, invitees(p.Kind), after, limit)
}

// Candidates returns the candidates of the date poll of the event id that were not withdrawn, by
// the earliest start, as accountID sees them; an event created with a date has none. It fails
// with ErrNotFound.
func (s *Service) Candidates(ctx context.Context, accountID, id string) ([]CandidateTally, error) {
	if _, err := s.Get(ctx, accountID, id); err != nil {
		return nil, err
	}
	return s.d.Repository.Candidates(ctx, nil, id, accountID)
}

// candidate returns the candidate candidateID of the event id as accountID sees it.
func (s *Service) candidate(ctx context.Context, tx uow.Tx, accountID, id, candidateID string) (CandidateTally, error) {
	cs, err := s.d.Repository.Candidates(ctx, tx, id, accountID)
	if err != nil {
		return CandidateTally{}, err
	}
	for _, c := range cs {
		if c.ID == candidateID {
			return c, nil
		}
	}
	return CandidateTally{}, ErrNotFound
}

// polling returns the event id as accountID organizes it (Service.organizing) and checks that it
// still polls its date (ErrScheduled otherwise).
func (s *Service) polling(ctx context.Context, accountID, id string) (View, error) {
	v, err := s.organizing(ctx, accountID, id)
	if err != nil {
		return View{}, err
	}
	if v.Scheduled() {
		return View{}, ErrScheduled
	}
	return v, nil
}

// lockedPoll locks the event id in tx and returns it, checking again that it was not cancelled
// (ErrClosed) and that it still polls its date (ErrScheduled).
func (s *Service) lockedPoll(ctx context.Context, tx uow.Tx, id string) (Record, error) {
	if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
		return Record{}, err
	}
	rec, err := s.d.Repository.FindGathering(ctx, tx, id)
	switch {
	case err != nil:
		return Record{}, err
	case rec.Cancellation != nil:
		return Record{}, ErrClosed
	case rec.Scheduled():
		return Record{}, ErrScheduled
	}
	return rec, nil
}

// AddCandidate adds the candidate sc to the date poll of the event id, by its organizer or a
// co-organizer (ADR-0161 §4, §7), checked as a new date. It fails with ErrNotFound,
// ErrNotOrganizer, ErrClosed, ErrScheduled for an event whose date was confirmed, and a
// *gathering.SettingsError of FieldCandidates past gathering.MaxCandidates or for a candidate
// the poll has already.
func (s *Service) AddCandidate(ctx context.Context, accountID, id string, sc gathering.Schedule) (CandidateTally, error) {
	if _, err := s.polling(ctx, accountID, id); err != nil {
		return CandidateTally{}, err
	}
	now := s.now()
	sc = utc(sc)
	if err := sc.Validate(now); err != nil {
		return CandidateTally{}, err
	}
	c := Candidate{ID: s.d.IDs.NewID(), GatheringID: id, Schedule: sc, AddedByAccountID: accountID, AddedAt: now}
	var out CandidateTally
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.lockedPoll(ctx, tx, id); err != nil {
			return err
		}
		cs, err := s.d.Repository.Candidates(ctx, tx, id, "")
		if err != nil {
			return err
		}
		schedules := make([]gathering.Schedule, len(cs))
		for i, k := range cs {
			schedules[i] = k.Schedule
		}
		if len(cs) >= gathering.MaxCandidates || gathering.SameSchedule(schedules, sc) {
			return &gathering.SettingsError{Field: gathering.FieldCandidates}
		}
		if err := s.d.Repository.InsertCandidate(ctx, tx, c); err != nil {
			return err
		}
		if err := tx.Emit(ctx, CandidateAdded{Candidate: c}); err != nil {
			return err
		}
		out, err = s.candidate(ctx, tx, accountID, id, c.ID)
		return err
	})
	return out, err
}

// WithdrawCandidate withdraws the candidate candidateID of the date poll of the event id, by its
// organizer or a co-organizer; its answers stay, and those who answered it still see the event.
// It fails with ErrNotFound for an unknown or withdrawn candidate, ErrNotOrganizer, ErrClosed,
// ErrScheduled, and ErrLastCandidate.
func (s *Service) WithdrawCandidate(ctx context.Context, accountID, id, candidateID string) error {
	if _, err := s.polling(ctx, accountID, id); err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.lockedPoll(ctx, tx, id); err != nil {
			return err
		}
		c, err := s.d.Repository.FindCandidate(ctx, tx, id, candidateID)
		if err != nil {
			return err
		}
		if c.Withdrawn {
			return ErrNotFound
		}
		cs, err := s.d.Repository.Candidates(ctx, tx, id, "")
		if err != nil {
			return err
		}
		if len(cs) <= 1 {
			return ErrLastCandidate
		}
		w := CandidateWithdrawal{CandidateID: candidateID, WithdrawnByAccountID: accountID, WithdrawnAt: s.now()}
		ok, err := s.d.Repository.InsertCandidateWithdrawal(ctx, tx, w)
		if err != nil || !ok {
			return err
		}
		return tx.Emit(ctx, CandidateWithdrawn{Candidate: c.Candidate, Withdrawal: w})
	})
}

// Vote records the answer of accountID to the candidate candidateID of the event id: yes, maybe,
// or no, changed as often as the account likes until a candidate is confirmed (ADR-0161 §7).
// Everyone who sees the event may answer, and the answer keeps the event visible to the account
// (gathering.CanSee). It fails with ErrNotFound for an event the account does not see and for an
// unknown or withdrawn candidate, ErrInvalidVote, ErrClosed for a cancelled event, and
// ErrScheduled once the date was confirmed.
func (s *Service) Vote(ctx context.Context, accountID, id, candidateID string, vote gathering.Vote) (CandidateTally, error) {
	if !vote.Valid() {
		return CandidateTally{}, ErrInvalidVote
	}
	if _, err := s.Get(ctx, accountID, id); err != nil {
		return CandidateTally{}, err
	}
	var out CandidateTally
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.lockedPoll(ctx, tx, id); err != nil {
			return err
		}
		c, err := s.candidate(ctx, tx, accountID, id, candidateID)
		if err != nil {
			return err
		}
		if c.ViewerVote == vote {
			out = c
			return nil
		}
		b := Ballot{CandidateID: candidateID, GatheringID: id, AccountID: accountID, Vote: vote, VotedAt: s.now()}
		if err := s.d.Repository.SetVote(ctx, tx, b); err != nil {
			return err
		}
		out, err = s.candidate(ctx, tx, accountID, id, candidateID)
		return err
	})
	return out, err
}

// Ballots returns at most limit answers to the candidate candidateID of the event id, of vote
// unless it is "", after the account afterAccountID unless it is "", in the order of the account
// IDs, and whether more follow: everyone who sees the event sees who answered (ADR-0161 §7). It
// fails with ErrNotFound for an event the account does not see and for an unknown or withdrawn
// candidate, and with ErrInvalidVote.
func (s *Service) Ballots(ctx context.Context, accountID, id, candidateID string, vote gathering.Vote, afterAccountID string, limit int) ([]Ballot, bool, error) {
	if vote != "" && !vote.Valid() {
		return nil, false, ErrInvalidVote
	}
	if _, err := s.Get(ctx, accountID, id); err != nil {
		return nil, false, err
	}
	c, err := s.d.Repository.FindCandidate(ctx, nil, id, candidateID)
	if err != nil {
		return nil, false, err
	}
	if c.Withdrawn {
		return nil, false, ErrNotFound
	}
	bs, err := s.d.Repository.Ballots(ctx, nil, candidateID, vote, afterAccountID, limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(bs) > limit {
		return bs[:limit], true, nil
	}
	return bs, false, nil
}

// Confirm fixes the date of the event id to its candidate candidateID, by its organizer or a
// co-organizer (ADR-0161 §7): the poll then takes no more answers, the reminders and the start
// follow the date, and the voters and the invitees are told with event_schedule_confirmed. The
// candidate must still start after now. It fails with ErrNotFound for an unknown or withdrawn
// candidate, ErrNotOrganizer, ErrClosed, ErrScheduled for an event whose date was confirmed, and
// a *gathering.SettingsError of FieldStartsAt for a candidate that started.
func (s *Service) Confirm(ctx context.Context, accountID, id, candidateID string) (View, error) {
	if _, err := s.polling(ctx, accountID, id); err != nil {
		return View{}, err
	}
	now := s.now()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.lockedPoll(ctx, tx, id); err != nil {
			return err
		}
		c, err := s.d.Repository.FindCandidate(ctx, tx, id, candidateID)
		if err != nil {
			return err
		}
		if c.Withdrawn {
			return ErrNotFound
		}
		if !c.Schedule.StartsAt.After(now) {
			return &gathering.SettingsError{Field: gathering.FieldStartsAt}
		}
		conf := Confirmation{ID: s.d.IDs.NewID(), GatheringID: id, Schedule: c.Schedule, ConfirmedByAccountID: accountID, ConfirmedAt: now}
		if err := s.d.Repository.InsertConfirmation(ctx, tx, conf); err != nil {
			return err
		}
		if err := s.d.Repository.InsertConfirmationCandidate(ctx, tx, conf.ID, candidateID); err != nil {
			return err
		}
		return tx.Emit(ctx, ScheduleConfirmed{Confirmation: conf, CandidateID: candidateID})
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, accountID, id)
}
