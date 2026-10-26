// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

func (f *fakeVoiceRooms) Summary(context.Context, string) (appvoice.Summary, error) {
	if f.summary == nil {
		return appvoice.Summary{}, appvoice.ErrNotFound
	}
	return *f.summary, f.err
}

func (f *fakeVoiceRooms) History(_ context.Context, _, before string, limit int) ([]appvoice.RoomRecord, bool, error) {
	f.historyAfter = append(f.historyAfter, before)
	ended := f.room().RoomRecord
	ended.Ending = &appvoice.Ending{RoomID: voiceRoomID, Reason: voice.EndHost, EndedAt: time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)}
	return []appvoice.RoomRecord{ended}, before == "" && limit == 1, f.err
}

func (f *fakeVoiceRooms) ProfileRoom(context.Context, string, string) (appvoice.RoomView, bool, error) {
	return f.room(), true, f.err
}

func (f *fakeVoiceRooms) Visible(context.Context, string, string) (appvoice.RoomView, error) {
	return f.room(), f.err
}

// TestVoiceRoomSummary checks the summary of a room without a token (ADR-0100 §9): the state
// always, the name, the description, and the host only when the room shows its summary, only
// the name of an ended one, and a 404 for an unknown room.
func TestVoiceRoomSummary(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	get := func() clientapi.VoiceRoomSummary {
		t.Helper()
		res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID+"/summary", "", "")
		if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("summary = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
		}
		return decode[clientapi.VoiceRoomSummary](t, res)
	}
	if res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID+"/summary", "", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("summary of an unknown room = %d, want 404", res.StatusCode)
	}
	f.summary = &appvoice.Summary{RoomID: voiceRoomID}
	if s := get(); s.State != clientapi.VoiceRoomSummaryStateOpen || s.Name != nil || s.Description != nil || s.Host != nil {
		t.Errorf("hidden summary = %+v, want the state only", s)
	}
	f.summary = &appvoice.Summary{RoomID: voiceRoomID, Shown: true, Name: "Talk", Description: "About", HostAccountID: aliceAccountID}
	if s := get(); s.Name == nil || *s.Name != "Talk" || s.Description == nil || *s.Description != "About" || s.Host == nil || s.Host.Id != aliceAccountID {
		t.Errorf("shown summary = %+v, want the name, the description, and the host", s)
	}
	f.summary = &appvoice.Summary{RoomID: voiceRoomID, Ended: true, Shown: true, Name: "Talk"}
	if s := get(); s.State != clientapi.VoiceRoomSummaryStateEnded || s.Name == nil || *s.Name != "Talk" || s.Description != nil || s.Host != nil {
		t.Errorf("summary of an ended room = %+v, want the name only", s)
	}
}

// TestVoiceRoomSummaryRateLimit checks that the summary, which needs no sign-in, counts against
// the limit of the client IP like the other anonymous requests.
func TestVoiceRoomSummaryRateLimit(t *testing.T) {
	f := &fakeVoiceRooms{summary: &appvoice.Summary{RoomID: voiceRoomID}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Voice = f
		o.AnonymousLimit = kernel.Limit{Name: "test.ip", Max: 1, Window: time.Minute}
	})
	if res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID+"/summary", "", ""); res.StatusCode != http.StatusOK ||
		!strings.HasPrefix(res.Header.Get("RateLimit-Policy"), `"ip";`) {
		t.Fatalf("first summary = %d, RateLimit-Policy %q", res.StatusCode, res.Header.Get("RateLimit-Policy"))
	}
	if res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID+"/summary", "", ""); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("summary over the limit = %d, want 429", res.StatusCode)
	}
}

// TestVoiceHistoryAndStartPost checks the history of the rooms the user hosted with its pages,
// and the start post of POST /voice/rooms with the 422 of a room that has none.
func TestVoiceHistoryAndStartPost(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	res := e.send(t, http.MethodGet, "/voice/history?limit=1", "read-voice", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("history = %d", res.StatusCode)
	}
	page := decode[clientapi.VoiceRoomHistoryPage](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].Name != "Talk" || page.Items[0].EndReason == nil ||
		*page.Items[0].EndReason != "host" || page.Items[0].EndedAt == nil {
		t.Fatalf("history = %+v", page)
	}
	if res := e.send(t, http.MethodGet, "/voice/history?cursor="+*page.NextCursor, "read-voice", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("next page = %d", res.StatusCode)
	}
	if f.historyAfter[1] != voiceRoomID {
		t.Errorf("the next page starts after %q, want %s", f.historyAfter[1], voiceRoomID)
	}
	if res := e.send(t, http.MethodGet, "/voice/history", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("history without a token = %d, want 401", res.StatusCode)
	}

	res = e.send(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk","post":{"text":"Come"}}`)
	if res.StatusCode != http.StatusCreated || len(f.opened) != 1 || f.opened[0].Post == nil || f.opened[0].Post.Text != "Come" {
		t.Fatalf("open with a start post = %d, %+v", res.StatusCode, f.opened)
	}
	f.err = appvoice.ErrNoStartPost
	if res := e.send(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk","visibility":"mutuals","post":{}}`); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("open a room of the mutuals with a start post = %d, want 422", res.StatusCode)
	}
}
