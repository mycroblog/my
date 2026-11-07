// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// remoteSendsPollVotes checks the votes of local accounts in a remote poll (FR-112): a vote is
// recorded per choice and shown to the voter, and each choice goes to the poll's author as the
// Create of a Note named after the option in reply to the Question, as Mastodon sends it, which
// reads back as such; a second vote, choices the poll does not take, a poll that an edit
// replaced, and a poll that has ended are refused and send nothing.
func remoteSendsPollVotes(t *testing.T, e *remoteInteractionEnv) {
	const alice = "https://remote.example/users/alice"
	fan := e.user(t, "fan")
	other := e.user(t, "other")
	aliceID := e.actor(t, alice)
	e.follow(t, fan, aliceID)
	e.follow(t, other, aliceID)
	id := alice + "/notes/poll"
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Hour), map[string]any{
		"type": "Question", "votersCount": 3,
		"anyOf": []any{
			map[string]any{"type": "Note", "name": "yes", "replies": map[string]any{"totalItems": 2}},
			map[string]any{"type": "Note", "name": "no", "replies": map[string]any{"totalItems": 1}},
			map[string]any{"type": "Note", "name": "maybe", "replies": map[string]any{"totalItems": 0}},
		},
		"endTime": e.clock.Now().Add(time.Hour).Format(time.RFC3339),
	})), appfederation.InboxHandled)
	p := e.post(t, id)
	st, poll := p.Status, p.Revision.Poll
	if poll == nil || !poll.Multiple || len(poll.Options) != 3 {
		t.Fatalf("poll = %+v, want the three options of the Question", poll)
	}
	ctx := t.Context()

	for _, c := range []struct {
		name    string
		pollID  string
		choices []int
		want    error
	}{
		{"no choice", poll.ID, nil, content.ErrInvalidPollChoices},
		{"unknown choice", poll.ID, []int{3}, content.ErrInvalidPollChoices},
		{"replaced poll", e.ids.NewID(), []int{0}, content.ErrPollChanged},
	} {
		if _, err := e.interactions.VotePoll(ctx, fan, st.ID, c.pollID, c.choices); !errors.Is(err, c.want) {
			t.Errorf("VotePoll(%s) = %v, want %v", c.name, err, c.want)
		}
	}
	votes, err := e.interactions.VotePoll(ctx, fan, st.ID, poll.ID, []int{2, 0})
	if err != nil {
		t.Fatalf("VotePoll = %v", err)
	}
	if len(votes) != 2 || votes[0].Position != 0 || votes[1].Position != 2 {
		t.Fatalf("votes = %+v, want the choices 0 and 2", votes)
	}
	if _, err := e.interactions.VotePoll(ctx, fan, st.ID, poll.ID, []int{1}); !errors.Is(err, content.ErrAlreadyVoted) {
		t.Errorf("second VotePoll = %v, want ErrAlreadyVoted", err)
	}

	// The voter sees the vote; the counts stay those the poll's server reported.
	reader := appcontent.NewReader(appcontent.ReaderDeps{
		Statuses: e.ih.Statuses, Interactions: e.ih.Interactions, Accounts: e.accounts, Relationships: e.graph, Clock: e.clock,
	})
	view, err := reader.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: fan}, st.ID)
	if err != nil {
		t.Fatalf("View = %v", err)
	}
	if v := view.Poll; v == nil || !slices.Equal(v.OwnVotes, []int{0, 2}) || !slices.Equal(v.Votes, []int64{2, 1, 0}) ||
		v.VotersCount == nil || *v.VotersCount != 3 || v.Closed {
		t.Errorf("poll seen by the voter = %+v", view.Poll)
	}
	if view, err := reader.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: other}, st.ID); err != nil ||
		view.Poll == nil || len(view.Poll.OwnVotes) != 0 {
		t.Errorf("poll seen by another account = %+v, %v; want no vote", view.Poll, err)
	}

	sent := e.pollVotesSent(t, st.ID)
	if len(sent) != 2 {
		t.Fatalf("activities = %+v, want a Create per choice", sent)
	}
	fanIRI := federation.IRI(e.localIRI(fan))
	for i, s := range sent {
		if s.Type != federation.ActivityCreate || !slices.Equal(s.Inboxes, []string{alice + "/inbox"}) {
			t.Errorf("activity %d = %s to %v, want a Create to the author", i, s.Type, s.Inboxes)
		}
		var doc struct {
			ID     string   `json:"id"`
			Actor  string   `json:"actor"`
			To     []string `json:"to"`
			Object struct {
				ID           string   `json:"id"`
				Type         string   `json:"type"`
				Name         string   `json:"name"`
				InReplyTo    string   `json:"inReplyTo"`
				AttributedTo string   `json:"attributedTo"`
				To           []string `json:"to"`
			} `json:"object"`
		}
		if err := json.Unmarshal(s.Raw, &doc); err != nil {
			t.Fatal(err)
		}
		want := []string{"yes", "maybe"}[i]
		o := doc.Object
		if doc.Actor != string(fanIRI) || doc.ID != votes[i].URI+"/activity" || !slices.Equal(doc.To, []string{alice}) ||
			o.ID != votes[i].URI || o.Type != "Note" || o.Name != want || o.InReplyTo != id || o.AttributedTo != string(fanIRI) ||
			!slices.Equal(o.To, []string{alice}) {
			t.Errorf("vote %d = %s, want the Note %q in reply to %s", i, s.Raw, want, id)
		}
		// The document reads back as a Create of the fan's Note in reply to the poll.
		n, err := appfederation.Normalize(s.Raw, e.clock.Now())
		if err != nil || n.Activity == nil || n.Activity.Actor != fanIRI || n.Activity.Type != federation.ActivityCreate {
			t.Errorf("Normalize(%s) = %+v, %v; want a Create of the fan", s.Raw, n, err)
		}
	}

	// A poll that has ended takes no vote.
	e.clock.Advance(2 * time.Hour)
	if _, err := e.interactions.VotePoll(ctx, other, st.ID, poll.ID, []int{1}); !errors.Is(err, content.ErrPollClosed) {
		t.Errorf("VotePoll after the end = %v, want ErrPollClosed", err)
	}
	if got := e.pollVotesSent(t, st.ID); len(got) != 2 {
		t.Errorf("activities after refused votes = %d, want the 2 of the first vote", len(got))
	}
}

// remoteVotesInLocalPolls checks a local poll across servers (FR-112): a local status posts
// its poll with no votes; a Note with a name in reply to it, as Mastodon and Misskey send votes,
// is a vote counted with the local ones, once per Note, never a reply, and a second choice in a
// single-choice poll, an unknown choice, and a vote after the end are dropped; the votes enqueue
// the Update of the poll after PollUpdateDelay, whose Question carries the counts to the
// voters' servers; and at its end the author and the local voters are notified, and the Update
// tells that the poll is closed.
func remoteVotesInLocalPolls(t *testing.T, e *remoteInteractionEnv) {
	ctx := t.Context()
	author := e.user(t, "author")
	voter := e.user(t, "voter")
	const bob = "https://remote.example/users/bob"
	e.actor(t, bob)
	p, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "which?"}, Visibility: content.VisibilityPublic,
		Poll: &content.PollInput{Options: []string{"yes", "no"}, Duration: time.Hour},
	})
	if err != nil {
		t.Fatalf("Create with a poll = %v", err)
	}
	st, poll := p.Status, p.Revision.Poll
	if poll == nil || poll.Multiple || poll.ClosesAt == nil || len(poll.Options) != 2 {
		t.Fatalf("poll = %+v, want a single-choice poll of two options with an end", poll)
	}
	counts := func(want []int64, voters int64) {
		t.Helper()
		view, err := e.reader.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: author}, st.ID)
		if err != nil {
			t.Fatalf("View = %v", err)
		}
		if v := view.Poll; v == nil || !slices.Equal(v.Votes, want) || v.VotersCount == nil || *v.VotersCount != voters {
			t.Errorf("poll = %+v, want the votes %v of %d voters", view.Poll, want, voters)
		}
	}
	counts([]int64{0, 0}, 0)

	vote := func(id, name string) []byte {
		return remoteActivity(t, "Create", id+"/activity", bob, map[string]any{
			"id": id, "type": "Note", "attributedTo": bob, "name": name, "inReplyTo": st.URI,
			"to": []string{e.localIRI(author)}, "published": e.clock.Now().Format(time.RFC3339),
		})
	}
	e.expect(t, vote(bob+"/votes/1", "yes"), appfederation.InboxHandled)
	counts([]int64{1, 0}, 1)
	if e.stored(t, bob+"/votes/1") {
		t.Error("the vote was stored as a reply")
	}
	e.expect(t, vote(bob+"/votes/1", "yes"), appfederation.InboxHandled) // the same Note again
	e.expect(t, vote(bob+"/votes/2", "no"), appfederation.InboxDropped)  // a second choice
	e.expect(t, vote(bob+"/votes/3", "maybe"), appfederation.InboxDropped)
	if _, err := e.interactions.VotePoll(ctx, voter, st.ID, poll.ID, []int{1}); err != nil {
		t.Fatalf("VotePoll = %v", err)
	}
	counts([]int64{1, 1}, 2)

	// The votes enqueue one Update after PollUpdateDelay; it reaches the remote voter.
	updates := e.runPollJobs(t)
	if len(updates) != 1 || updates[0].RunAt.Before(e.clock.Now().Add(appfederation.PollUpdateDelay-time.Minute)) {
		t.Fatalf("poll update jobs = %+v, want one after the delay", updates)
	}
	// A run of the job again, as after a crash before the queue marked it done, records nothing.
	updateJob := e.ids.NewID()
	for range 2 {
		if err := e.pollUpdates.Run(ctx, st.ID, poll.ID, updateJob); err != nil {
			t.Fatalf("PollUpdates.Run = %v", err)
		}
	}
	e.runPollJobs(t)
	sent := e.questionsSent(t, st.ID)
	if len(sent) != 1 || !slices.Contains(sent[0].Inboxes, bob+"/inbox") {
		t.Fatalf("Updates = %+v, want one to the remote voter", sent)
	}
	q := sent[0].question(t)
	if q.Type != "Question" || q.Updated != "" || q.Closed != "" || q.VotersCount != 2 || len(q.OneOf) != 2 ||
		q.OneOf[0].Name != "yes" || q.OneOf[0].Replies.TotalItems != 1 || q.OneOf[1].Replies.TotalItems != 1 ||
		q.EndTime != federation.FormatTime(*poll.ClosesAt) {
		t.Errorf("Question = %s", sent[0].Raw)
	}

	// At the end, the author and the local voter are notified, and the Update tells it at once,
	// also while a job of the votes waits (the queue of the env keeps every key it saw).
	e.clock.Advance(2 * time.Hour)
	e.expect(t, vote(bob+"/votes/4", "no"), appfederation.InboxDropped)
	if wait, err := e.pollCloser.Close(ctx, poll.ID); err != nil || wait != 0 {
		t.Fatalf("Close = %s, %v", wait, err)
	}
	if _, err := e.pollCloser.Close(ctx, poll.ID); err != nil {
		t.Fatalf("Close again = %v", err)
	}
	closing := e.runPollJobs(t)
	if len(closing) != 1 || !closing[0].RunAt.IsZero() {
		t.Fatalf("poll update jobs at the end = %+v, want one to run at once", closing)
	}
	if n := e.notified(t, author, "", notification.TypePollClosed); n != 1 {
		t.Errorf("poll_closed notifications of the author = %d, want 1", n)
	}
	if n := e.notified(t, voter, author, notification.TypePollClosed); n != 1 {
		t.Errorf("poll_closed notifications of the voter = %d, want 1", n)
	}
	if err := e.pollUpdates.Run(ctx, st.ID, poll.ID, e.ids.NewID()); err != nil {
		t.Fatalf("PollUpdates.Run = %v", err)
	}
	e.runPollJobs(t)
	sent = e.questionsSent(t, st.ID)
	if len(sent) != 2 {
		t.Fatalf("Updates = %d, want 2", len(sent))
	}
	if q := sent[1].question(t); q.Closed != q.EndTime || q.Closed == "" {
		t.Errorf("Question at the end = %s, want it closed", sent[1].Raw)
	}
}

// questionDoc is the object of the Update of a local poll.
type questionDoc struct {
	Type        string `json:"type"`
	Updated     string `json:"updated"`
	EndTime     string `json:"endTime"`
	Closed      string `json:"closed"`
	VotersCount int64  `json:"votersCount"`
	OneOf       []struct {
		Name    string `json:"name"`
		Replies struct {
			TotalItems int64 `json:"totalItems"`
		} `json:"replies"`
	} `json:"oneOf"`
}

func (s remoteInteractionSent) question(t *testing.T) questionDoc {
	t.Helper()
	var doc struct {
		Object questionDoc `json:"object"`
	}
	if err := json.Unmarshal(s.Raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Object
}

// runPollJobs runs the pending fan-out jobs and returns the JobPollUpdate jobs, which it leaves
// to the caller.
func (e *remoteInteractionEnv) runPollJobs(t *testing.T) []queue.NewJob {
	t.Helper()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	var updates []queue.NewJob
	for {
		nj, ok := e.jobs.pop()
		if !ok {
			return updates
		}
		switch nj.Type {
		case appfederation.JobPollUpdate:
			updates = append(updates, nj)
		case appfederation.JobFanOut:
			j, err := nj.Build(e.ids.NewID(), e.clock.Now())
			if err != nil {
				t.Fatal(err)
			}
			if r := fanOut(t.Context(), j); !r.IsDone() {
				t.Fatalf("fan-out %s = %+v (%v), want Done", j.Payload, r, r.Err())
			}
		}
	}
}

// questionsSent returns the Updates recorded about a local status, oldest first, with their
// documents and inboxes.
func (e *remoteInteractionEnv) questionsSent(t *testing.T, statusID string) []remoteInteractionSent {
	t.Helper()
	ctx := t.Context()
	acts, err := e.ih.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundStatus, statusID)
	if err != nil {
		t.Fatal(err)
	}
	var out []remoteInteractionSent
	for _, a := range acts {
		if a.Type != federation.ActivityUpdate {
			continue
		}
		s := remoteInteractionSent{Type: a.Type}
		if s.Raw, err = e.ih.Federation.FindOutboundDocument(ctx, a.ID); err != nil {
			t.Fatalf("FindOutboundDocument(%s): %v", a.ID, err)
		}
		ds, err := e.ih.Federation.ListDeliveries(ctx, a.ID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			s.Inboxes = append(s.Inboxes, string(d.InboxURL))
		}
		out = append(out, s)
	}
	return out
}

// pollVotesSent runs the pending fan-out jobs of votes and returns the activities recorded about
// the votes in the poll of a status, oldest first, with their documents and inboxes.
func (e *remoteInteractionEnv) pollVotesSent(t *testing.T, statusID string) []remoteInteractionSent {
	t.Helper()
	ctx := t.Context()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	for {
		nj, ok := e.jobs.pop()
		if !ok {
			break
		}
		if nj.Type != appfederation.JobFanOut {
			continue
		}
		var p struct {
			ID string `json:"outbound_activity_id"`
		}
		if err := json.Unmarshal(nj.Payload, &p); err != nil {
			t.Fatal(err)
		}
		act, err := e.ih.Federation.FindOutboundActivity(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if act.ObjectKind != federation.OutboundPollVote {
			continue
		}
		j, err := nj.Build(e.ids.NewID(), e.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if r := fanOut(ctx, j); !r.IsDone() {
			t.Fatalf("fan-out %s = %+v (%v), want Done", j.Payload, r, r.Err())
		}
	}
	acts, err := e.ih.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundPollVote, statusID)
	if err != nil {
		t.Fatalf("ListOutboundActivitiesOf: %v", err)
	}
	out := []remoteInteractionSent{}
	for _, a := range acts {
		s := remoteInteractionSent{Type: a.Type}
		if s.Raw, err = e.ih.Federation.FindOutboundDocument(ctx, a.ID); err != nil {
			t.Fatalf("FindOutboundDocument(%s): %v", a.ID, err)
		}
		ds, err := e.ih.Federation.ListDeliveries(ctx, a.ID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			s.Inboxes = append(s.Inboxes, string(d.InboxURL))
		}
		out = append(out, s)
	}
	return out
}
