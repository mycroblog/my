// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
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

func (f *fakeVoiceRooms) Icons(_ context.Context, icons []voice.Icon) (map[voice.Icon]appvoice.IconView, error) {
	out := map[voice.Icon]appvoice.IconView{}
	for _, i := range icons {
		switch {
		case i.Unicode != "":
			out[i] = appvoice.IconView{Unicode: i.Unicode}
		case f.customEmoji != nil && f.customEmoji.ID == i.CustomEmojiID:
			out[i] = appvoice.IconView{Custom: f.customEmoji}
		}
	}
	return out, nil
}

// TestVoiceRoomIcon checks the icon of a room (FR-923) on the room and the summary: a Unicode emoji by itself, a custom emoji with its image, none for a custom emoji
// that can no longer be shown, and the icon of the settings passed to opening and changing a
// room, with the empty icon of a change that removes it.
func TestVoiceRoomIcon(t *testing.T) {
	const blob = "0190a6f2-0000-7000-8000-0000000000e1"
	f := &fakeVoiceRooms{icon: voice.Icon{Unicode: "\U0001F3B5"}}
	e := newVoiceEnv(t, f)
	room := func() clientapi.VoiceRoom {
		t.Helper()
		res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID, "read-voice", "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("get = %d", res.StatusCode)
		}
		return decode[clientapi.VoiceRoom](t, res)
	}
	if r := room(); r.Icon == nil || r.Icon.Name != "\U0001F3B5" || r.Icon.Emoji != nil {
		t.Errorf("icon = %+v, want the Unicode emoji", r.Icon)
	}
	f.icon = voice.Icon{CustomEmojiID: blob}
	if r := room(); r.Icon != nil {
		t.Errorf("icon of an emoji that cannot be shown = %+v, want none", r.Icon)
	}
	f.customEmoji = &appcontent.CustomEmojiView{ID: blob, Shortcode: "blob", URL: "https://example.com/blob.png", StaticURL: "https://example.com/blob.png"}
	if r := room(); r.Icon == nil || r.Icon.Name != "blob" || r.Icon.Emoji == nil || r.Icon.Emoji.Url != "https://example.com/blob.png" {
		t.Errorf("icon = %+v, want the custom emoji with its image", r.Icon)
	}
	f.summary = &appvoice.Summary{RoomID: voiceRoomID, Shown: true, Name: "Talk", Icon: voice.Icon{CustomEmojiID: blob}}
	res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID+"/summary", "", "")
	if s := decode[clientapi.VoiceRoomSummary](t, res); s.Icon == nil || s.Icon.Name != "blob" {
		t.Errorf("summary icon = %+v, want the custom emoji", s.Icon)
	}

	if res := e.send(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk","icon":":blob:"}`); res.StatusCode != http.StatusCreated ||
		len(f.opened) != 1 || f.opened[0].Icon != ":blob:" {
		t.Fatalf("open with an icon = %d, %+v", res.StatusCode, f.opened)
	}
	if res := e.sendJSON(t, http.MethodPatch, "/voice/rooms/"+voiceRoomID, "voice", `{"icon":""}`); res.StatusCode != http.StatusOK ||
		len(f.patches) != 1 || f.patches[0].Icon == nil || *f.patches[0].Icon != "" {
		t.Fatalf("remove the icon = %d, %+v", res.StatusCode, f.patches)
	}
	f.err = &voice.SettingsError{Field: voice.FieldIcon}
	if res := e.send(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk","icon":"blob@remote.example"}`); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("open with a remote emoji = %d, want 422", res.StatusCode)
	}
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
