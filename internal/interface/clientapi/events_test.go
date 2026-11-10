// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	eventID        = "01890a5d-ac96-774b-bcce-b302099a8401"
	confirmationID = "01890a5d-ac96-774b-bcce-b302099a8402"
)

// fakeEvents is one event organized by alice that the viewer sees with its voice place, with a
// co-organizer whose account is gone (left out) and one who is there, and the error of the next
// calls. The physical place stays hidden from the viewer.
type fakeEvents struct {
	clientapi.Events
	err error
	// room is the room of the voice place, nil for none.
	room    *appgathering.RoomState
	created []appgathering.CreateInput
	patches []appgathering.Patch
	answers []gathering.Answer
	queries []appgathering.ListQuery
}

var eventStart = time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

func (f *fakeEvents) view() appgathering.View {
	return appgathering.View{
		Record: appgathering.Record{
			Gathering: appgathering.Gathering{ID: eventID, OrganizerAccountID: aliceAccountID, CreatedAt: eventStart.Add(-48 * time.Hour)},
			Head: appgathering.Head{
				GatheringID: eventID, OrganizerAccountID: aliceAccountID, State: gathering.StateScheduled, GoingCount: 1,
				Schedule: gathering.Schedule{StartsAt: eventStart, EndsAt: eventStart.Add(2 * time.Hour)},
			},
			Confirmation: appgathering.Confirmation{
				ID: confirmationID, GatheringID: eventID, Schedule: gathering.Schedule{StartsAt: eventStart, EndsAt: eventStart.Add(2 * time.Hour)},
			},
			Revision: appgathering.Revision{Settings: gathering.Settings{
				Title: "Picnic", Visibility: gathering.VisibilityPublic, TimeZone: "Asia/Tokyo", PlaceDisclosure: gathering.DisclosureApproved,
				Places: []gathering.Place{
					{Kind: gathering.PlacePhysical, Name: "Secret park", Address: "1-1 Hidden"},
					{Kind: gathering.PlaceVoice, Speakers: gathering.SpeakersManual},
				},
			}},
			Coorganizers: []appgathering.Coorganizer{{AccountID: bobAccountID}, {AccountID: aliceAccountID}},
			Revisions:    1,
		},
		Viewer:       appgathering.ViewerState{Answer: gathering.AnswerMaybe},
		Places:       []gathering.Place{{Kind: gathering.PlaceVoice, Speakers: gathering.SpeakersManual}},
		HiddenPlaces: []gathering.PlaceKind{gathering.PlacePhysical},
		Room:         f.room,
	}
}

// TestEventRoom checks that the voice place of an event carries the room of its current date: its
// ID once it opened, the reason once it could not open, and neither before (ADR-0161 §10).
func TestEventRoom(t *testing.T) {
	const roomID = "01890a5d-ac96-774b-bcce-b302099a8403"
	for _, tc := range []struct {
		name   string
		room   *appgathering.RoomState
		id     string
		reason string
	}{
		{"before the start", &appgathering.RoomState{}, "", ""},
		{"opened", &appgathering.RoomState{RoomID: roomID}, roomID, ""},
		{"could not open", &appgathering.RoomState{Unavailable: gathering.RoomHostingElsewhere}, "", "hosting_elsewhere"},
	} {
		e := newEventsEnv(t, &fakeEvents{room: tc.room})
		ev := decode[clientapi.Event](t, e.send(t, http.MethodGet, "/events/"+eventID, "read-events", ""))
		if len(ev.Places) != 1 {
			t.Fatalf("%s: places = %+v", tc.name, ev.Places)
		}
		p := ev.Places[0]
		if got := deref(p.RoomId); got != tc.id {
			t.Errorf("%s: room_id = %q, want %q", tc.name, got, tc.id)
		}
		if got := ""; p.RoomUnavailable != nil {
			got = string(*p.RoomUnavailable)
			if got != tc.reason {
				t.Errorf("%s: room_unavailable = %q, want %q", tc.name, got, tc.reason)
			}
		} else if tc.reason != "" {
			t.Errorf("%s: room_unavailable missing, want %q", tc.name, tc.reason)
		}
	}
}

func (f *fakeEvents) Create(_ context.Context, _ string, in appgathering.CreateInput) (appgathering.View, error) {
	f.created = append(f.created, in)
	return f.view(), f.err
}

func (f *fakeEvents) Get(context.Context, string, string) (appgathering.View, error) {
	return f.view(), f.err
}

func (f *fakeEvents) List(_ context.Context, _ string, q appgathering.ListQuery) ([]appgathering.View, *appgathering.ListKey, error) {
	f.queries = append(f.queries, q)
	return []appgathering.View{f.view()}, &appgathering.ListKey{StartsAt: eventStart, GatheringID: eventID}, f.err
}

func (f *fakeEvents) Update(_ context.Context, _, _ string, p appgathering.Patch) (appgathering.View, error) {
	f.patches = append(f.patches, p)
	return f.view(), f.err
}

func (f *fakeEvents) Answer(_ context.Context, _, _ string, a gathering.Answer) (appgathering.View, error) {
	f.answers = append(f.answers, a)
	return f.view(), f.err
}

func newEventsEnv(t *testing.T, f clientapi.Events) *authEnv {
	t.Helper()
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Events = f })
	e.auth.tokens["events"] = bearer("read:events write:events", aliceAccountID)
	e.auth.tokens["read-events"] = bearer("read:events", aliceAccountID)
	return e
}

func TestEvents(t *testing.T) {
	f := &fakeEvents{}
	e := newEventsEnv(t, f)

	res := e.send(t, http.MethodGet, "/events/"+eventID, "read-events", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("get = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	// A place that the viewer does not see never reaches the client: only its kind does.
	if strings.Contains(string(body), "Secret park") || strings.Contains(string(body), "1-1 Hidden") {
		t.Errorf("the response carries a hidden place: %s", body)
	}
	var ev clientapi.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Organizer.Id != aliceAccountID || len(ev.Coorganizers) != 1 || ev.Coorganizers[0].Id != aliceAccountID || len(ev.Places) != 1 || ev.Places[0].Kind != clientapi.EventPlaceKindVoice ||
		len(ev.HiddenPlaces) != 1 || ev.HiddenPlaces[0] != clientapi.EventPlaceKindPhysical || ev.Viewer.Answer == nil ||
		*ev.Viewer.Answer != clientapi.EventAnswerMaybe || !ev.StartsAt.Equal(eventStart) || ev.TimeZone != "Asia/Tokyo" {
		t.Errorf("get = %+v", ev)
	}

	res = e.send(t, http.MethodGet, "/events?limit=1", "read-events", "")
	page := decode[clientapi.EventPage](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("list = %+v", page)
	}
	if res := e.send(t, http.MethodGet, "/events?cursor="+*page.NextCursor, "read-events", ""); res.StatusCode != http.StatusOK {
		t.Errorf("next page = %d", res.StatusCode)
	}
	if q := f.queries[len(f.queries)-1]; q.After == nil || q.After.GatheringID != eventID || !q.After.StartsAt.Equal(eventStart) {
		t.Errorf("queries = %+v", f.queries)
	}
	if res := e.send(t, http.MethodGet, "/events?cursor=broken", "read-events", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a broken cursor = %d, want 400", res.StatusCode)
	}

	res = e.sendJSON(t, http.MethodPost, "/events", "events", `{"title":"Picnic","starts_at":"2026-12-01T21:00:00+09:00","ends_at":"2026-12-01T23:00:00+09:00",`+
		`"visibility":"mutuals","places":[{"kind":"url","name":"Stream","url":"https://stream.example/live"}]}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", res.StatusCode)
	}
	if in := f.created[0]; in.Settings.Visibility != gathering.VisibilityMutuals || !in.Schedule.StartsAt.Equal(eventStart) ||
		len(in.Settings.Places) != 1 || in.Settings.Places[0].URL != "https://stream.example/live" {
		t.Errorf("create input = %+v", in)
	}
	if res := e.sendJSON(t, http.MethodPatch, "/events/"+eventID, "events", `{"title":"New"}`); res.StatusCode != http.StatusOK || *f.patches[0].Title != "New" {
		t.Errorf("update = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPatch, "/events/"+eventID, "events", `{"starts_at":"2026-12-02T12:00:00Z"}`); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a start without an end = %d, want 422", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPut, "/events/"+eventID+"/rsvp", "events", `{"answer":"going"}`); res.StatusCode != http.StatusOK || f.answers[0] != gathering.AnswerGoing {
		t.Errorf("answer = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPut, "/events/"+eventID+"/rsvp", "read-events", `{"answer":"going"}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("answer with read:events = %d, want 403", res.StatusCode)
	}
}

func TestEventProblems(t *testing.T) {
	f := &fakeEvents{}
	e := newEventsEnv(t, f)
	for _, tc := range []struct {
		err    error
		status int
		typ    string
	}{
		{appgathering.ErrNotFound, http.StatusNotFound, "about:blank"},
		{appgathering.ErrFull, http.StatusConflict, clientapi.ProblemTypeEventFull},
		{appgathering.ErrClosed, http.StatusConflict, clientapi.ProblemTypeEventClosed},
		{appgathering.ErrInvalidAnswer, http.StatusUnprocessableEntity, "about:blank"},
		{&gathering.SettingsError{Field: gathering.FieldPlaces}, http.StatusUnprocessableEntity, "about:blank"},
	} {
		f.err = tc.err
		res := e.sendJSON(t, http.MethodPut, "/events/"+eventID+"/rsvp", "events", `{"answer":"going"}`)
		if res.StatusCode != tc.status || problemType(t, res) != tc.typ {
			t.Errorf("%v: %d, want %d %s", tc.err, res.StatusCode, tc.status, tc.typ)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
		typ    string
	}{
		{appgathering.ErrNotEligible, http.StatusForbidden, clientapi.ProblemTypeEventNotEligible},
		{appgathering.ErrVoiceUnavailable, http.StatusUnprocessableEntity, clientapi.ProblemTypeEventVoiceUnavailable},
		{appgathering.ErrNotOrganizer, http.StatusForbidden, clientapi.ProblemTypeEventNotOrganizer},
		{appgathering.ErrOrganizerOnly, http.StatusForbidden, clientapi.ProblemTypeEventOrganizerOnly},
	} {
		f.err = tc.err
		res := e.sendJSON(t, http.MethodPatch, "/events/"+eventID, "events", `{"title":"New"}`)
		if res.StatusCode != tc.status || problemType(t, res) != tc.typ {
			t.Errorf("%v: %d, want %d %s", tc.err, res.StatusCode, tc.status, tc.typ)
		}
	}
}
