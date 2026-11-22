// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// call sends method to path as user with an Idempotency-Key of its own, and returns its outcome.
func (e *eventBoardEnv) call(user, method, path string, body, out any) string {
	e.t.Helper()
	e.keys++
	return e.outcome(e.send(user, method, path, body, map[string]string{"Idempotency-Key": fmt.Sprintf("organizers-%d", e.keys)}, out))
}

// attendees lists the attendees of eventID as user, by account ID.
func (e *eventBoardEnv) attendees(user, eventID string) map[string]clientapi.EventAttendee {
	e.t.Helper()
	var page clientapi.EventAttendeePage
	if got := e.outcome(e.send(user, http.MethodGet, "/events/"+eventID+"/attendees", nil, nil, &page)); got != "200" {
		e.t.Fatalf("GET /events/%s/attendees as %s = %s", eventID, user, got)
	}
	out := map[string]clientapi.EventAttendee{}
	for _, a := range page.Items {
		out[a.Account.Id] = a
	}
	return out
}

// TestEventOrganizersAreNeverAskedForApproval checks through the API that the organizer and the
// co-organizers are never waiting for an approval (ADR-0161 §6): the attendee list says they
// organize and are approved, their event says they are approved, an approval or its revocation of
// one of them is a 422 event-attendee-organizes even before they answer, a change of the
// co-organizers shows at once, and an ordinary attendee is approved and revoked as before.
func TestEventOrganizersAreNeverAskedForApproval(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "cora", "mona", "gina")
	ev := e.createEvent("", map[string]any{"places": []map[string]any{{"kind": "url", "name": "Stream", "url": "https://stream.example/live"}}})
	base := "/events/" + ev.Id
	// alice organizes and answers going; cora co-organizes and never answers; mona answers going
	// and is named a co-organizer then; gina answers maybe.
	for _, user := range []string{"alice", "mona"} {
		if got := e.call(user, http.MethodPut, base+"/rsvp", map[string]any{"answer": "going"}, nil); got != "200" {
			t.Fatalf("PUT rsvp as %s = %s", user, got)
		}
	}
	if got := e.call("gina", http.MethodPut, base+"/rsvp", map[string]any{"answer": "maybe"}, nil); got != "200" {
		t.Fatalf("PUT rsvp as gina = %s", got)
	}
	for _, user := range []string{"cora", "mona"} {
		if got := e.call("alice", http.MethodPut, base+"/coorganizers/"+e.ids[user], nil, nil); got != "200" {
			t.Fatalf("PUT coorganizers/%s = %s", user, got)
		}
	}

	as := e.attendees("gina", ev.Id)
	if len(as) != 3 {
		t.Errorf("attendees = %+v, want alice, mona, and gina", as)
	}
	for _, user := range []string{"alice", "mona"} {
		if a := as[e.ids[user]]; !a.Organizes || !a.Approved {
			t.Errorf("the organizing attendee %s is listed as organizes %v, approved %v, want both", user, a.Organizes, a.Approved)
		}
	}
	if a := as[e.ids["gina"]]; a.Organizes || a.Approved {
		t.Errorf("the attendee gina is listed as organizes %v, approved %v, want neither", a.Organizes, a.Approved)
	}
	for user, want := range map[string]bool{"alice": true, "cora": true, "mona": true, "gina": false} {
		got := e.event(user, ev.Id)
		if got.Viewer.Approved != want || (len(got.HiddenPlaces) == 0) != want {
			t.Errorf("the event as %s: viewer.approved %v with hidden places %v, want approved %v", user, got.Viewer.Approved, got.HiddenPlaces, want)
		}
	}

	organizes := "422 " + clientapi.ProblemTypeEventAttendeeOrganizes
	for _, by := range []string{"alice", "cora"} {
		for _, user := range []string{"alice", "cora", "mona"} {
			for _, method := range []string{http.MethodPut, http.MethodDelete} {
				if got := e.call(by, method, base+"/attendees/"+e.ids[user]+"/approval", nil, nil); got != organizes {
					t.Errorf("%s the approval of %s as %s = %s, want %s", method, user, by, got, organizes)
				}
			}
		}
	}

	// An ordinary attendee is approved and revoked as before.
	var a clientapi.EventAttendee
	if got := e.call("cora", http.MethodPut, base+"/attendees/"+e.ids["gina"]+"/approval", nil, &a); got != "200" || !a.Approved || a.Organizes {
		t.Errorf("approve gina = %s %+v, want 200 approved", got, a)
	}
	if got := e.event("gina", ev.Id); !got.Viewer.Approved || len(got.HiddenPlaces) != 0 {
		t.Errorf("the event as an approved attendee: viewer.approved %v, hidden places %v", got.Viewer.Approved, got.HiddenPlaces)
	}
	a = clientapi.EventAttendee{}
	if got := e.call("alice", http.MethodDelete, base+"/attendees/"+e.ids["gina"]+"/approval", nil, &a); got != "200" || a.Approved || a.Organizes {
		t.Errorf("revoke gina = %s %+v, want 200 not approved", got, a)
	}

	// A removed co-organizer, never approved, goes back to waiting at once.
	if got := e.call("alice", http.MethodDelete, base+"/coorganizers/"+e.ids["mona"], nil, nil); got != "200" {
		t.Fatalf("DELETE coorganizers/mona = %s", got)
	}
	if a := e.attendees("gina", ev.Id)[e.ids["mona"]]; a.Organizes || a.Approved {
		t.Errorf("a removed co-organizer is listed as organizes %v, approved %v, want neither", a.Organizes, a.Approved)
	}
	if got := e.event("mona", ev.Id); got.Viewer.Approved || got.Viewer.Coorganizer || len(got.HiddenPlaces) == 0 {
		t.Errorf("the event as a removed co-organizer: %+v, hidden places %v", got.Viewer, got.HiddenPlaces)
	}
	a = clientapi.EventAttendee{}
	if got := e.call("alice", http.MethodPut, base+"/attendees/"+e.ids["mona"]+"/approval", nil, &a); got != "200" || !a.Approved || a.Organizes {
		t.Errorf("approve a removed co-organizer = %s %+v, want 200 approved", got, a)
	}
}
