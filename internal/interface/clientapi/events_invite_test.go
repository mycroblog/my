// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// invitingEvents is fakeEvents organized by the viewer, which stores every invitation it is asked
// for.
type invitingEvents struct {
	*fakeEvents
	invited []string
}

func (f *invitingEvents) Get(ctx context.Context, accountID, id string) (appgathering.View, error) {
	v, err := f.fakeEvents.Get(ctx, accountID, id)
	v.Viewer.Organizer = true
	return v, err
}

func (f *invitingEvents) Invite(_ context.Context, byAccountID, id, accountID string) (appgathering.Invitation, error) {
	f.invited = append(f.invited, accountID)
	return appgathering.Invitation{ID: eventID, GatheringID: id, InviteeAccountID: accountID, InvitedByAccountID: byAccountID, InvitedAt: time.Now()}, nil
}

// TestInviteToEventRefusesAnAccountTheViewerMayNotSee checks that inviting an account that the
// viewer may not see (here one that is gone) answers 422 event-invalid-invitee without storing
// the invitation, rather than storing it and answering 404.
func TestInviteToEventRefusesAnAccountTheViewerMayNotSee(t *testing.T) {
	f := &invitingEvents{fakeEvents: &fakeEvents{}}
	e := newEventsEnv(t, f)
	res := e.send(t, http.MethodPost, "/events/"+eventID+"/invitations", "events", `{"account_id":"`+bobAccountID+`"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeEventInvalidInvitee {
		t.Errorf("invite an account that is gone = %d %s, want 422 %s", res.StatusCode, p.Type, clientapi.ProblemTypeEventInvalidInvitee)
	}
	if len(f.invited) != 0 {
		t.Errorf("stored invitations of %v, want none", f.invited)
	}
}
