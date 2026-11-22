// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeAttendees is the event of fakeEvents whose attendees are alice, its organizer, going, and
// bob, an attendee waiting for an approval, with the attendee and the error that the approvals
// answer with.
type fakeAttendees struct {
	fakeEvents
	attendee appgathering.Attendee
	err      error
}

func (f *fakeAttendees) Attendees(context.Context, string, string, string, int) ([]appgathering.Attendee, bool, error) {
	return []appgathering.Attendee{
		{GatheringID: eventID, AccountID: aliceAccountID, Answer: gathering.AnswerGoing, Approved: true, Organizes: true},
		{GatheringID: eventID, AccountID: bobAccountID, Answer: gathering.AnswerMaybe},
	}, false, nil
}

func (f *fakeAttendees) Approve(context.Context, string, string, string) (appgathering.Attendee, error) {
	return f.attendee, f.err
}

func (f *fakeAttendees) RevokeApproval(context.Context, string, string, string) (appgathering.Attendee, error) {
	return f.attendee, f.err
}

// TestEventAttendeesOrganize checks that the attendee list says who organizes the event, and that
// an approval of one who organizes is a 422 event-attendee-organizes (ADR-0161 §6).
func TestEventAttendeesOrganize(t *testing.T) {
	f := &fakeAttendees{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Events = f
		accounts := o.Accounts.(fakeAccounts)
		accounts[bobAccountID] = appaccount.Actor{
			Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: createdAt},
			Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
			Profile:   &account.ProfileRevision{DisplayName: "Bob"},
			Lifecycle: account.InitialLifecycleState(bobAccountID),
		}
	})
	e.auth.tokens["events"] = bearer("read:events write:events", aliceAccountID)
	e.auth.tokens["read-events"] = bearer("read:events", aliceAccountID)

	page := decode[clientapi.EventAttendeePage](t, e.send(t, http.MethodGet, "/events/"+eventID+"/attendees", "read-events", ""))
	if len(page.Items) != 2 {
		t.Fatalf("attendees = %+v", page)
	}
	if a := page.Items[0]; a.Account.Id != aliceAccountID || !a.Organizes || !a.Approved {
		t.Errorf("the organizer is listed as %+v, want organizes and approved", a)
	}
	if a := page.Items[1]; a.Account.Id != bobAccountID || a.Organizes || a.Approved {
		t.Errorf("the attendee is listed as %+v, want neither organizes nor approved", a)
	}

	path := "/events/" + eventID + "/attendees/" + bobAccountID + "/approval"
	f.attendee = appgathering.Attendee{GatheringID: eventID, AccountID: bobAccountID, Answer: gathering.AnswerMaybe, Approved: true}
	if a := decode[clientapi.EventAttendee](t, e.send(t, http.MethodPut, path, "events", "")); !a.Approved || a.Organizes {
		t.Errorf("approve = %+v", a)
	}
	f.attendee.Approved = false
	if a := decode[clientapi.EventAttendee](t, e.send(t, http.MethodDelete, path, "events", "")); a.Approved || a.Organizes {
		t.Errorf("revoke = %+v", a)
	}

	f.err = appgathering.ErrAttendeeOrganizes
	path = "/events/" + eventID + "/attendees/" + aliceAccountID + "/approval"
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		res := e.send(t, method, path, "events", "")
		if got := problemType(t, res); res.StatusCode != http.StatusUnprocessableEntity || got != clientapi.ProblemTypeEventAttendeeOrganizes {
			t.Errorf("%s the approval of one who organizes = %d %s, want 422 %s", method, res.StatusCode, got, clientapi.ProblemTypeEventAttendeeOrganizes)
		}
	}
}
