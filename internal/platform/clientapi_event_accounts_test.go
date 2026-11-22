// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// suspend suspends the account of user.
func (e *eventBoardEnv) suspend(user string) {
	e.t.Helper()
	if _, err := e.app.Moderation.IssueAccountAction(e.t.Context(), appmoderation.AccountActionInput{
		TargetAccountID: e.ids[user], Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	}); err != nil {
		e.t.Fatalf("suspend %s: %v", user, err)
	}
}

// TestEventAccountsFollowThePresentabilityRule checks that the accounts of an event follow the
// rule of every other account list: an attendee who blocks the viewer, or whom moderation
// suspended, is left out of the attendees, while the cursor goes on over the stored answers; and
// an organizer that the viewer may not see is shown as unavailable, with its ID and nothing of its
// profile.
func TestEventAccountsFollowThePresentabilityRule(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina", "mona", "max", "sam")
	ev := e.createEvent("readers", nil)
	for _, name := range []string{"gina", "mona", "max"} {
		e.answer(name, ev.Id, gathering.AnswerGoing)
	}
	if _, err := e.app.Graph.Block(t.Context(), e.ids["gina"], e.ids["sam"]); err != nil {
		t.Fatal(err)
	}
	e.suspend("max")
	e.drain()

	attendees := func(user, query string) (ids []string, next *string) {
		t.Helper()
		var page clientapi.EventAttendeePage
		if res := e.send(user, http.MethodGet, "/events/"+ev.Id+"/attendees"+query, nil, nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET attendees as %s = %s", user, e.outcome(res))
		}
		for _, a := range page.Items {
			ids = append(ids, a.Account.Id)
		}
		return ids, page.NextCursor
	}
	all, _ := attendees("alice", "")
	if want := []string{e.ids["gina"], e.ids["mona"]}; !slices.Equal(all, want) {
		t.Errorf("attendees for alice = %v, want %v: the suspended max left out", all, want)
	}
	var seen []string
	query := "?limit=1"
	for range 10 {
		ids, next := attendees("sam", query)
		seen = append(seen, ids...)
		if next == nil {
			break
		}
		query = "?limit=1&cursor=" + *next
	}
	if want := []string{e.ids["mona"]}; !slices.Equal(seen, want) {
		t.Errorf("attendees for sam, page by page = %v, want %v: gina blocks sam", seen, want)
	}

	e.suspend("alice")
	e.drain()
	got := e.event("gina", ev.Id)
	if o := got.Organizer; o.Id != e.ids["alice"] || o.Unavailable == nil || !*o.Unavailable || o.Username != "" || o.DisplayName != "" {
		t.Errorf("the organizer after the suspension = %+v, want alice's ID, unavailable, and nothing of her profile", o)
	}
}
