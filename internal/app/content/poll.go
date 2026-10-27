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

// This file holds the votes in polls (FR-112) and what a viewer sees of a poll. A vote in a
// remote poll is recorded in poll_votes and carried to the poll's server by the federation
// module, one Note per choice (federation.md §7.1); the counts of a remote poll stay those its
// server reports (poll_tallies), which include the vote once the server sends its next Update.
// The votes in a local poll, of local accounts and of remote ones (a Note in reply to the
// Question, federation.md §3.4), are counted again after each vote and recorded as a tally
// observation, so that poll_tallies follows the newest counts of every poll alike.

// EventPollVoted is emitted when a local account votes in a poll: the federation module delivers
// the votes in a remote poll to its author.
const EventPollVoted = "content.poll_voted"

// PollVoted is emitted when the votes of one account in a poll are inserted, one per choice.
type PollVoted struct {
	Votes []content.PollVote
	Poll  content.Poll
	// Status is the status whose current revision has the poll, and AuthorURI the ActivityPub
	// id of its author, who receives the votes in a remote poll.
	Status    StatusRef
	AuthorURI string
}

// EventType implements uow.Event.
func (PollVoted) EventType() string { return EventPollVoted }

// The refusals of votes, for the modules that cannot import the domain (content.ErrNoPoll and
// the others).
var (
	ErrNoPoll             = content.ErrNoPoll
	ErrPollClosed         = content.ErrPollClosed
	ErrAlreadyVoted       = content.ErrAlreadyVoted
	ErrInvalidPollChoices = content.ErrInvalidPollChoices
)

type (
	// Poll is content.Poll.
	Poll = content.Poll
	// PollVote is content.PollVote.
	PollVote = content.PollVote
)

// PollView is the poll of a status as a viewer sees it.
type PollView struct {
	Poll Poll
	// Votes are the votes of each choice, by position; a poll without counts yet has 0 for each.
	Votes []int64
	// VotersCount is the number of accounts that voted, nil when it is unknown (a multiple-choice
	// remote poll whose server did not report it).
	VotersCount *int64
	// OwnVotes are the positions the viewer voted for, in order; nil for an anonymous viewer.
	OwnVotes []int
	// Closed is whether the poll has ended (content.Poll.Closed).
	Closed bool
	// TotalsHidden is whether the votes of each choice are hidden until the poll ends
	// (content.Poll.TotalsHidden): readers show neither Votes nor the share of a choice, only
	// how many voted.
	TotalsHidden bool
}

// wallClock is the system's clock, for a Reader without one.
type wallClock struct{}

func (wallClock) Now() time.Time { return kernel.Normalize(time.Now()) }

// VotesCount is the sum of the votes of every choice.
func (v PollView) VotesCount() int64 {
	var n int64
	for _, c := range v.Votes {
		n += c
	}
	return n
}

// VotePoll makes accountID vote for choices, the positions of options, in the poll pollID of
// statusID, and returns the votes, one per choice. The poll must be the one of the status's
// current revision: one that an edit replaced fails with content.ErrPollChanged, and a status
// without a poll with content.ErrNoPoll. Choices that the poll does not take fail with
// content.ErrInvalidPollChoices. Like the other interactions, it needs a status the account may
// see and an account that may interact.
//
// The rest is decided in the transaction with the poll locked (Repository.LockPoll), local or
// remote, so that the votes in one poll are decided one at a time: a poll whose end has passed
// when the vote commits fails with content.ErrPollClosed, and an account that voted in the poll
// already, with any choices, fails with content.ErrAlreadyVoted.
func (s *Interactions) VotePoll(ctx context.Context, accountID, statusID, pollID string, choices []int) ([]content.PollVote, error) {
	if err := s.checkRestrictions(ctx, accountID, false); err != nil {
		return nil, err
	}
	st, err := s.visibleStatus(ctx, accountID, statusID)
	if err != nil {
		return nil, err
	}
	rev, err := s.d.Statuses.FindCurrentRevision(ctx, st.ID)
	if err != nil {
		return nil, err
	}
	poll := rev.Poll
	switch {
	case poll == nil:
		return nil, fmt.Errorf("status %s: %w", statusID, content.ErrNoPoll)
	case poll.ID != pollID:
		return nil, fmt.Errorf("poll %s of status %s: %w", pollID, statusID, content.ErrPollChanged)
	}
	author, err := s.d.Accounts.FindByID(ctx, st.AuthorAccountID)
	if err != nil {
		return nil, err
	}
	positions, err := poll.Choices(choices)
	if err != nil {
		return nil, err
	}
	var votes []content.PollVote
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		if err := s.d.Statuses.LockPoll(ctx, tx, poll.ID); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		if poll.Closed(now) {
			return fmt.Errorf("poll %s: %w", poll.ID, content.ErrPollClosed)
		}
		prior, err := s.d.Statuses.ListPollVotesTx(ctx, tx, poll.ID, accountID)
		if err != nil {
			return err
		}
		if len(prior) > 0 {
			return fmt.Errorf("poll %s: %w", poll.ID, content.ErrAlreadyVoted)
		}
		votes = make([]content.PollVote, len(positions))
		for i, p := range positions {
			id := s.d.IDs.NewID()
			votes[i] = content.PollVote{ID: id, PollID: poll.ID, AccountID: accountID, Position: p, VotedAt: now}
			if !st.IsLocal {
				votes[i].URI = content.PollVoteURI(s.d.Host, id)
			}
		}
		for _, v := range votes {
			ok, err := s.d.Statuses.InsertPollVote(ctx, tx, v)
			if err != nil {
				return err
			}
			if !ok { // a vote of the account committed first
				return fmt.Errorf("poll %s: %w", poll.ID, content.ErrAlreadyVoted)
			}
		}
		return s.voted(ctx, tx, PollVoted{Votes: votes, Poll: *poll, Status: RefOf(st), AuthorURI: author.Account.URI})
	})
	if err != nil {
		return nil, err
	}
	return votes, nil
}

// voted counts the votes of a local poll again after ev's votes, inside tx and with the poll
// locked (Repository.LockPoll), records the counts as a new tally observation (data-model
// §4.4), and emits ev. The observation's ID is taken with the lock held, so that the IDs of a
// poll's observations sort in the order of their counts, which the rebuild of poll_tallies
// follows.
func (s *Interactions) voted(ctx context.Context, tx uow.Tx, ev PollVoted) error {
	if ev.Status.IsLocal {
		counts, voters, err := s.d.Statuses.CountPollVotes(ctx, tx, ev.Poll.ID)
		if err != nil {
			return err
		}
		o := content.PollTallyObservation{
			ID: s.d.IDs.NewID(), PollID: ev.Poll.ID, VotersCount: &voters, ObservedAt: ev.Votes[0].VotedAt,
			Votes: make([]int64, len(ev.Poll.Options)),
		}
		for pos, n := range counts {
			if pos >= 0 && pos < len(o.Votes) {
				o.Votes[pos] = n
			}
		}
		if err := s.d.Statuses.InsertPollObservation(ctx, tx, o); err != nil {
			return err
		}
		if err := tx.Emit(ctx, PollTallyObserved{Observation: o}); err != nil {
			return err
		}
	}
	return tx.Emit(ctx, ev)
}

// RemotePollVoteInput is a remote account's choice in a local poll: the Create of a Note named
// after the choice, in reply to the Question (federation.md §3.4).
type RemotePollVoteInput struct {
	// URI is the id of the Note.
	URI       string
	AccountID string
	// StatusID is the local status the Note replies to.
	StatusID string
	// Title is the name of the Note, the title of the choice.
	Title string
	// VotedAt is the published time of the Note; zero, or a time in the future, is now.
	VotedAt time.Time
}

// RecordRemotePollVote records a remote account's choice in the poll of a local status and
// reports whether it is new; a Note recorded already, or a choice the account voted for
// already, records nothing and reports false. The status must be one the account may see, with
// a poll in its current revision (content.ErrNoPoll otherwise), the poll must not have ended
// when the vote commits, which is checked with the poll locked (content.ErrPollClosed), the title must name a choice (content.ErrInvalidPollChoices), and a
// single-choice poll takes one vote per account (content.ErrAlreadyVoted). The counts of the
// poll are recorded again, and PollVoted is emitted, as for a local account's vote.
func (s *Interactions) RecordRemotePollVote(ctx context.Context, in RemotePollVoteInput) (bool, error) {
	st, err := s.remoteVisibleStatus(ctx, in.AccountID, in.StatusID)
	if err != nil {
		return false, err
	}
	if !st.IsLocal {
		return false, fmt.Errorf("status %s is not local: %w", st.ID, content.ErrNoPoll)
	}
	rev, err := s.d.Statuses.FindCurrentRevision(ctx, st.ID)
	if err != nil {
		return false, err
	}
	poll := rev.Poll
	if poll == nil {
		return false, fmt.Errorf("status %s: %w", st.ID, content.ErrNoPoll)
	}
	at := s.remoteTime(in.VotedAt)
	position, ok := poll.OptionByTitle(in.Title)
	if !ok {
		return false, fmt.Errorf("%w: no choice %q in poll %s", content.ErrInvalidPollChoices, in.Title, poll.ID)
	}
	vote := content.PollVote{ID: s.d.IDs.NewIDAt(at), PollID: poll.ID, AccountID: in.AccountID, Position: position, URI: in.URI, VotedAt: at}
	var inserted bool
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted = false
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		if err := s.d.Statuses.LockPoll(ctx, tx, poll.ID); err != nil {
			return err
		}
		if poll.Closed(s.d.Clock.Now()) {
			return fmt.Errorf("poll %s: %w", poll.ID, content.ErrPollClosed)
		}
		if !poll.Multiple {
			prior, err := s.d.Statuses.ListPollVotesTx(ctx, tx, poll.ID, in.AccountID)
			if err != nil {
				return err
			}
			if len(prior) > 0 {
				if prior[0].URI == in.URI {
					return nil // the same Note again
				}
				return fmt.Errorf("poll %s: %w", poll.ID, content.ErrAlreadyVoted)
			}
		}
		ok, err := s.d.Statuses.InsertPollVote(ctx, tx, vote)
		if err != nil || !ok {
			return err
		}
		inserted = true
		return s.voted(ctx, tx, PollVoted{Votes: []content.PollVote{vote}, Poll: *poll, Status: RefOf(st)})
	})
	if err != nil {
		return false, err
	}
	return inserted, nil
}

// poll returns the poll of rev as viewerID sees it ("" for an anonymous viewer), nil when rev
// has none.
func (r *Reader) poll(ctx context.Context, rev content.Revision, viewerID string) (*PollView, error) {
	if rev.Poll == nil {
		return nil, nil
	}
	p := *rev.Poll
	now := r.d.Clock.Now()
	out := &PollView{Poll: p, Votes: make([]int64, len(p.Options)), Closed: p.Closed(now), TotalsHidden: p.TotalsHidden(now)}
	tallies, err := r.d.Statuses.FindPollTallies(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range tallies {
		if t.Position >= 0 && t.Position < len(out.Votes) {
			out.Votes[t.Position] = t.Votes
		}
		out.VotersCount = t.VotersCount
	}
	if out.VotersCount == nil && !p.Multiple {
		// One vote per voter: the votes are the voters.
		n := out.VotesCount()
		out.VotersCount = &n
	}
	if viewerID != "" {
		own, err := r.d.Statuses.ListPollVotes(ctx, p.ID, viewerID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		out.OwnVotes = make([]int, 0, len(own))
		for _, v := range own {
			out.OwnVotes = append(out.OwnVotes, v.Position)
		}
	}
	return out, nil
}
