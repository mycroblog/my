// SPDX-License-Identifier: MIT

package integration_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform/platformtest"

	"github.com/mycroblog/my/pkg/mykit"
)

// TestEventDatePoll creates an event with candidate dates with mykit, answers a candidate as
// another user, changes the answer, and confirms the candidate, after which the event is
// scheduled and the poll takes no more answers.
func TestEventDatePoll(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})
	ctx := t.Context()
	zone := "Asia/Tokyo"

	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	candidates := []mykit.EventCandidateCreate{
		{StartsAt: start, EndsAt: start.Add(2 * time.Hour)},
		{StartsAt: start.Add(24 * time.Hour), EndsAt: start.Add(26 * time.Hour)},
	}
	created, err := aliceAPI.CreateEventWithResponse(ctx, mykit.CreateEventJSONRequestBody{Title: "Picnic", TimeZone: &zone, Candidates: &candidates})
	if err != nil {
		t.Fatal(err)
	}
	if created.JSON201 == nil || created.JSON201.State != mykit.EventStateUnscheduled {
		t.Fatalf("createEvent = %d %s, want an unscheduled event", created.StatusCode(), created.Body)
	}
	id := created.JSON201.Id

	list, err := bobAPI.ListEventCandidatesWithResponse(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if list.JSON200 == nil || len(list.JSON200.Items) != 2 {
		t.Fatalf("listEventCandidates = %d %s, want two candidates", list.StatusCode(), list.Body)
	}
	first := list.JSON200.Items[0].Id
	for _, v := range []mykit.EventVote{mykit.EventVoteYes, mykit.EventVoteMaybe} {
		res, err := bobAPI.VoteEventCandidateWithResponse(ctx, id, first, mykit.VoteEventCandidateJSONRequestBody{Vote: v})
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON200 == nil || res.JSON200.Vote == nil || *res.JSON200.Vote != v {
			t.Fatalf("voteEventCandidate(%s) = %d %s", v, res.StatusCode(), res.Body)
		}
	}
	votes, err := aliceAPI.ListEventCandidateVotesWithResponse(ctx, id, first, &mykit.ListEventCandidateVotesParams{})
	if err != nil {
		t.Fatal(err)
	}
	if votes.JSON200 == nil || len(votes.JSON200.Items) != 1 || votes.JSON200.Items[0].Account.Id != bob.AccountID ||
		votes.JSON200.Items[0].Vote != mykit.EventVoteMaybe {
		t.Fatalf("listEventCandidateVotes = %d %s, want bob's maybe", votes.StatusCode(), votes.Body)
	}

	confirmed, err := aliceAPI.ConfirmEventCandidateWithResponse(ctx, id, first)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.JSON200 == nil || confirmed.JSON200.State != mykit.EventStateScheduled || !confirmed.JSON200.StartsAt.Equal(start) {
		t.Fatalf("confirmEventCandidate = %d %s, want the event scheduled at %v", confirmed.StatusCode(), confirmed.Body, start)
	}
	var problem *mykit.Problem
	_, err = bobAPI.VoteEventCandidateWithResponse(ctx, id, first, mykit.VoteEventCandidateJSONRequestBody{Vote: mykit.EventVoteNo})
	if !errors.As(err, &problem) || problem.Status != http.StatusConflict || problem.Type != "/api/v1/problems/event-scheduled" {
		t.Fatalf("voteEventCandidate after the confirmation = %v, want a 409 event-scheduled", err)
	}
}
