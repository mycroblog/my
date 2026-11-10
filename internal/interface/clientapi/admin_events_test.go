// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	reportedEventID         = "01890a5d-ac96-774b-bcce-b302099a8001"
	reportedEventRevisionID = "01890a5d-ac96-774b-bcce-b302099a8002"
	laterEventRevisionID    = "01890a5d-ac96-774b-bcce-b302099a8003"
)

// fakeEventModeration sees the event reportedEventID, organized by reportTargetID, for every
// reporter, and cancels it once.
type fakeEventModeration struct {
	cancelled []appgathering.ModeratorCancelInput
}

func (f *fakeEventModeration) ReportTarget(_ context.Context, _, id string) (appgathering.ReportTarget, error) {
	if id != reportedEventID {
		return appgathering.ReportTarget{}, appgathering.ErrNotFound
	}
	return appgathering.ReportTarget{GatheringID: id, RevisionID: reportedEventRevisionID, OrganizerAccountID: reportTargetID}, nil
}

func (f *fakeEventModeration) CancelByModerator(_ context.Context, in appgathering.ModeratorCancelInput) error {
	switch {
	case !gathering.ValidNote(in.Note):
		return &gathering.SettingsError{Field: gathering.FieldNote}
	case in.GatheringID != reportedEventID:
		return appgathering.ErrNotFound
	case len(f.cancelled) > 0:
		return appgathering.ErrClosed
	}
	f.cancelled = append(f.cancelled, in)
	return nil
}

func (f *fakeEventModeration) EventRecords(_ context.Context, id string) (appgathering.EventRecords, error) {
	if id != reportedEventID {
		return appgathering.EventRecords{}, appgathering.ErrNotFound
	}
	at := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	sched := gathering.Schedule{StartsAt: at.Add(24 * time.Hour), EndsAt: at.Add(26 * time.Hour)}
	first := gathering.Settings{
		Title: "Meetup", Description: "Bring snacks", Visibility: gathering.VisibilityPublic, TimeZone: "Asia/Tokyo",
		PlaceDisclosure: gathering.DisclosureApproved,
		Places:          []gathering.Place{{Kind: gathering.PlacePhysical, Name: "Hall", Address: "1-1 Somewhere"}},
	}
	later := first
	later.Title, later.Places = "Meetup (moved)", nil
	return appgathering.EventRecords{
		Record: appgathering.Record{
			Gathering: appgathering.Gathering{ID: id, OrganizerAccountID: reportTargetID, CreatedAt: at},
			Head:      appgathering.Head{GatheringID: id, State: gathering.StateCancelled, Schedule: sched},
			Cancellation: &appgathering.Cancellation{
				GatheringID: id, Reason: gathering.CancelModerator, CancelledByAccountID: aliceAccountID, Note: "spam", CancelledAt: at.Add(time.Hour),
			},
		},
		Revisions: []appgathering.Revision{
			{ID: reportedEventRevisionID, GatheringID: id, Settings: first, RevisedByAccountID: reportTargetID, RevisedAt: at},
			{ID: laterEventRevisionID, GatheringID: id, Settings: later, RevisedByAccountID: reportTargetID, RevisedAt: at.Add(time.Minute)},
		},
	}, nil
}

// newReportsEnvWithEvents is newReportsEnv with the events of events.
func newReportsEnvWithEvents(t *testing.T, reports *fakeReports, events clientapi.EventModeration) *authEnv {
	t.Helper()
	return newAuthEnv(t, func(o *clientapi.Options) {
		o.EventModeration = events
		accounts := fakeAccounts{}
		for id, a := range o.Accounts.(fakeAccounts) {
			accounts[id] = a
		}
		accounts[reportTargetID] = appaccount.Actor{
			Account:   account.Account{ID: reportTargetID, Domain: "remote.example", URI: "https://remote.example/users/bob", CreatedAt: createdAt},
			Handle:    &account.Handle{AccountID: reportTargetID, Username: "bob", Domain: "remote.example"},
			Lifecycle: account.InitialLifecycleState(reportTargetID),
		}
		o.Accounts = accounts
		o.Reports = reports
	})
}

func TestCreateReportOfAnEvent(t *testing.T) {
	reports := &fakeReports{}
	e := newReportsEnvWithEvents(t, reports, &fakeEventModeration{})
	res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "event_id": reportedEventID, "category": "spam"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	got := decodeJSON[clientapi.Report](t, res)
	if got.EventId == nil || *got.EventId != reportedEventID {
		t.Errorf("report = %+v, want the event", got)
	}
	want := moderation.ReportGathering{GatheringID: reportedEventID, RevisionID: reportedEventRevisionID}
	if in := reports.got[0]; in.Gathering == nil || *in.Gathering != want || in.TargetAccountID != reportTargetID {
		t.Errorf("filed %+v, want the event, the revision the reporter sees, and its organizer", in)
	}
	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"an event that the user does not see", map[string]any{"account_id": reportTargetID, "event_id": reportOne, "category": "spam"}, "event_id"},
		{"an account that is not the organizer", map[string]any{"account_id": aliceAccountID, "event_id": reportedEventID, "category": "spam"}, "account_id"},
	} {
		res := e.postReport(t, "write", tc.body)
		p := decodeProblem(t, res)
		if res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, tc.field) {
			t.Errorf("%s = %d %+v, want 422 on %s", tc.name, res.StatusCode, p, tc.field)
		}
	}
	if len(reports.got) != 1 {
		t.Errorf("filed %d reports, want the first alone", len(reports.got))
	}

	// Without the events, a report names no event.
	e = newReportsEnv(t, &fakeReports{})
	if res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "event_id": reportedEventID, "category": "spam"}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("without the events = %d, want 422", res.StatusCode)
	}
}

func TestCancelEventAsModerator(t *testing.T) {
	events := &fakeEventModeration{}
	e, _ := newModerationEnvWith(t, func(o *clientapi.Options) { o.EventModeration = events })
	path := "/admin/events/" + reportedEventID + "/cancel"
	if res := e.sendJSON(t, http.MethodPost, path, "admin-read", `{}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("with admin:read = %d, want 403", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, path, "admin", `{"reason":"spam"}`); res.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel = %d, want 204", res.StatusCode)
	}
	want := appgathering.ModeratorCancelInput{GatheringID: reportedEventID, ByUserID: userOf(aliceAccountID), ByAccountID: aliceAccountID, Note: "spam"}
	if len(events.cancelled) != 1 || events.cancelled[0] != want {
		t.Errorf("cancelled %+v, want %+v", events.cancelled, want)
	}
	if res := e.sendJSON(t, http.MethodPost, path, "admin", ""); res.StatusCode != http.StatusConflict {
		t.Errorf("cancel of a cancelled event = %d, want 409", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/admin/events/"+reportOne+"/cancel", "admin", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("cancel of an unknown event = %d, want 404", res.StatusCode)
	}
}

func TestReportDetailShowsTheEvent(t *testing.T) {
	e, f := newModerationEnvWith(t, func(o *clientapi.Options) { o.EventModeration = &fakeEventModeration{} })
	f.reports[0].Report.Gathering = &moderation.ReportGathering{GatheringID: reportedEventID, RevisionID: reportedEventRevisionID}
	res := e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportOne, "admin-read", "")
	got := decodeJSON[clientapi.AdminReportDetail](t, res)
	ev := got.Event
	if res.StatusCode != http.StatusOK || got.EventId == nil || *got.EventId != reportedEventID || ev == nil {
		t.Fatalf("detail = %d %+v, want the event", res.StatusCode, got)
	}
	if ev.ReportedRevisionId != reportedEventRevisionID || len(ev.Revisions) != 2 || ev.Revisions[0].Id != reportedEventRevisionID ||
		ev.Revisions[0].Title != "Meetup" || len(ev.Revisions[0].Places) != 1 || ev.Revisions[0].Places[0].Address == nil ||
		ev.Revisions[1].Title != "Meetup (moved)" || len(ev.Revisions[1].Places) != 0 {
		t.Errorf("revisions = %+v, want both with the places of the reported one", ev.Revisions)
	}
	if c := ev.Cancellation; ev.State != "cancelled" || c == nil || c.Reason != "moderator" || c.Note == nil || *c.Note != "spam" ||
		c.CancelledByAccountId == nil || *c.CancelledByAccountId != aliceAccountID {
		t.Errorf("event = %+v, want its cancellation by the moderator", ev)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportTwo, "admin-read", "")
	if got := decodeJSON[clientapi.AdminReportDetail](t, res); got.Event != nil || got.EventId != nil {
		t.Errorf("a report of no event = %+v", got)
	}
}
