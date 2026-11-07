// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// pollLockProbe is a content.Repository that records the order of the lock of a poll and the
// check of an account's earlier votes, and runs onLock in the transaction before the lock is
// taken, as for a vote that waits for the lock while others commit.
type pollLockProbe struct {
	appcontent.Repository
	mu     sync.Mutex
	calls  []string
	onLock func(ctx context.Context, tx uow.Tx)
}

func (p *pollLockProbe) record(call string) func(ctx context.Context, tx uow.Tx) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, call)
	if call != "lock" {
		return nil
	}
	f := p.onLock
	p.onLock = nil
	return f
}

func (p *pollLockProbe) LockPoll(ctx context.Context, tx uow.Tx, pollID string) error {
	if f := p.record("lock"); f != nil {
		f(ctx, tx)
	}
	return p.Repository.LockPoll(ctx, tx, pollID)
}

func (p *pollLockProbe) ListPollVotesTx(ctx context.Context, tx uow.Tx, pollID, accountID string) ([]content.PollVote, error) {
	p.record("check")
	return p.Repository.ListPollVotesTx(ctx, tx, pollID, accountID)
}

func (p *pollLockProbe) setOnLock(f func(ctx context.Context, tx uow.Tx)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onLock = f
}

// pollVotesDecidedUnderLock checks that the votes in a poll, local or remote, are decided with
// the poll locked (FR-112): an account's earlier votes in a remote poll are checked after the
// lock is taken, so that two ballots of one account cannot both pass; the counts of a local poll
// are recorded as an observation whose ID follows those of the votes that committed while the
// vote waited for the lock, so that the rebuild of poll_tallies keeps the newest counts; and a
// vote that would commit after the end of the poll is refused, although the poll was open when
// the vote began.
func pollVotesDecidedUnderLock(t *testing.T, e *remoteInteractionEnv) {
	ctx := t.Context()
	probe := &pollLockProbe{Repository: e.ih.Statuses}
	deps := e.interactionDeps
	deps.Statuses = probe
	interactions := appcontent.NewInteractions(deps)
	noVotes := func(pollID, accountID string) {
		t.Helper()
		votes, err := e.ih.Statuses.ListPollVotes(ctx, pollID, accountID)
		if err != nil && !errors.Is(err, appcontent.ErrNotFound) {
			t.Fatal(err)
		}
		if len(votes) != 0 {
			t.Errorf("votes of %s = %+v, want none", accountID, votes)
		}
	}
	newLocalPoll := func(author string) (content.Status, *content.Poll) {
		t.Helper()
		p, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "which?"}, Visibility: content.VisibilityPublic,
			Poll: &content.PollInput{Options: []string{"yes", "no"}, Duration: time.Hour},
		})
		if err != nil {
			t.Fatalf("Create with a poll = %v", err)
		}
		return p.Status, p.Revision.Poll
	}

	// A remote single-choice poll: the earlier votes are checked with the poll locked.
	const alice = "https://remote.example/users/alice"
	fan := e.user(t, "fan")
	other := e.user(t, "other")
	aliceID := e.actor(t, alice)
	e.follow(t, fan, aliceID)
	e.follow(t, other, aliceID)
	id := alice + "/notes/single"
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Hour), map[string]any{
		"type": "Question",
		"oneOf": []any{
			map[string]any{"type": "Note", "name": "yes", "replies": map[string]any{"totalItems": 0}},
			map[string]any{"type": "Note", "name": "no", "replies": map[string]any{"totalItems": 0}},
		},
		"endTime": e.clock.Now().Add(time.Hour).Format(time.RFC3339),
	})), appfederation.InboxHandled)
	remote := e.post(t, id)
	remotePoll := remote.Revision.Poll
	if remotePoll == nil || remotePoll.Multiple {
		t.Fatalf("poll = %+v, want a single-choice poll", remotePoll)
	}
	if _, err := interactions.VotePoll(ctx, fan, remote.Status.ID, remotePoll.ID, []int{0}); err != nil {
		t.Fatalf("VotePoll in the remote poll = %v", err)
	}
	if !slices.Equal(probe.calls, []string{"lock", "check"}) {
		t.Errorf("vote in a remote poll = %v, want the poll locked before the earlier votes are checked", probe.calls)
	}
	if _, err := interactions.VotePoll(ctx, fan, remote.Status.ID, remotePoll.ID, []int{1}); !errors.Is(err, content.ErrAlreadyVoted) {
		t.Errorf("second ballot in the remote poll = %v, want ErrAlreadyVoted", err)
	}

	// A local poll: the observation of a vote that committed while this one waited for the lock
	// does not become the newest one, which the rebuild keeps.
	author := e.user(t, "author")
	voter := e.user(t, "voter")
	st, local := newLocalPoll(author)
	probe.setOnLock(func(ctx context.Context, tx uow.Tx) {
		var none int64
		o := content.PollTallyObservation{
			ID: e.ids.NewID(), PollID: local.ID, Votes: []int64{0, 0}, VotersCount: &none, ObservedAt: e.clock.Now(),
		}
		if err := e.ih.Statuses.InsertPollObservation(ctx, tx, o); err != nil {
			t.Errorf("InsertPollObservation = %v", err)
		}
	})
	if _, err := interactions.VotePoll(ctx, voter, st.ID, local.ID, []int{1}); err != nil {
		t.Fatalf("VotePoll in the local poll = %v", err)
	}
	if err := appcontent.RebuildProjections(ctx, e.ih.UnitOfWork, e.ih.Statuses); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	tallies, err := e.ih.Statuses.FindPollTallies(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tallies) != 2 || tallies[0].Votes != 0 || tallies[1].Votes != 1 || tallies[1].VotersCount == nil || *tallies[1].VotersCount != 1 {
		t.Errorf("rebuilt tallies = %+v, want the counts after the vote", tallies)
	}

	// A vote that waits for the lock past the end of the poll is refused: a local account's in
	// a remote poll and in a local one, and a remote account's in a local one.
	late := func(context.Context, uow.Tx) { e.clock.Advance(2 * time.Hour) }
	probe.setOnLock(late)
	if _, err := interactions.VotePoll(ctx, other, remote.Status.ID, remotePoll.ID, []int{1}); !errors.Is(err, content.ErrPollClosed) {
		t.Errorf("VotePoll in the remote poll past its end = %v, want ErrPollClosed", err)
	}
	noVotes(remotePoll.ID, other)

	st, local = newLocalPoll(author)
	probe.setOnLock(late)
	if _, err := interactions.VotePoll(ctx, voter, st.ID, local.ID, []int{0}); !errors.Is(err, content.ErrPollClosed) {
		t.Errorf("VotePoll in the local poll past its end = %v, want ErrPollClosed", err)
	}
	noVotes(local.ID, voter)

	const bob = "https://remote.example/users/bob"
	bobID := e.actor(t, bob)
	st, local = newLocalPoll(author)
	probe.setOnLock(late)
	_, err = interactions.RecordRemotePollVote(ctx, appcontent.RemotePollVoteInput{
		URI: bob + "/votes/late", AccountID: bobID, StatusID: st.ID, Title: "yes",
	})
	if !errors.Is(err, content.ErrPollClosed) {
		t.Errorf("RecordRemotePollVote past the end = %v, want ErrPollClosed", err)
	}
	noVotes(local.ID, bobID)
}
