// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestVoiceSettingsThroughAdminAPI drives the voice chat's instance settings on SQLite
// (ADR-0100 §2, §9, §13): their defaults, a change by an administrator that the audit log
// records, the values they refuse, and a moderator without manage_settings.
func TestVoiceSettingsThroughAdminAPI(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	admin := e.user("alice")
	r, err := e.app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Identity.GrantRole(t.Context(), identity.RoleChangeInput{UserID: r.User.ID, RoleKey: domainidentity.RoleModerator}); err != nil {
		t.Fatal(err)
	}
	moderator := issueTokens(t, e.app, r.User.ID, "").AccessToken
	keys := []clientapi.InstanceSettingKey{
		clientapi.InstanceSettingKeyVoiceRoomStarters, clientapi.InstanceSettingKeyVoicePublicSummary, clientapi.InstanceSettingKeyRetentionVoiceParticipants,
	}
	read := func() map[clientapi.InstanceSettingKey]clientapi.AdminSetting {
		t.Helper()
		var list clientapi.AdminSettingList
		e.want(e.do(http.MethodGet, "/admin/settings", admin.token, nil, &list), http.StatusOK, "GET /admin/settings")
		out := map[clientapi.InstanceSettingKey]clientapi.AdminSetting{}
		for _, s := range list.Items {
			out[s.Key] = s
		}
		return out
	}
	values := func(got map[clientapi.InstanceSettingKey]clientapi.AdminSetting) []string {
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = got[k].Value
		}
		return out
	}

	got := read()
	if v := values(got); strings.Join(v, ",") != "permitted,false,90" {
		t.Errorf("defaults = %v, want permitted, false, 90", v)
	}
	if s := got[clientapi.InstanceSettingKeyRetentionVoiceParticipants]; s.Minimum == nil || *s.Minimum != 30 || s.Kind != clientapi.InstanceSettingKindInt {
		t.Errorf("retention_voice_participants = %+v, want an int of at least 30", s)
	}
	if s := got[clientapi.InstanceSettingKeyVoiceRoomStarters]; s.Choices == nil || strings.Join(*s.Choices, ",") != "everyone,permitted" {
		t.Errorf("voice_room_starters = %+v, want the choices everyone and permitted", s)
	}

	for _, bad := range []map[string]string{
		{"voice_room_starters": "nobody"},
		{"voice_public_summary": "yes"},
		{"retention_voice_participants": "29"},
	} {
		e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": bad}, nil), http.StatusUnprocessableEntity, "PATCH an invalid value")
	}
	e.want(e.do(http.MethodPatch, "/admin/settings", moderator, map[string]any{"values": map[string]string{"voice_room_starters": "everyone"}}, nil),
		http.StatusForbidden, "PATCH by a moderator")
	if v := values(read()); strings.Join(v, ",") != "permitted,false,90" {
		t.Errorf("after the refused changes = %v, want the defaults", v)
	}

	e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": map[string]string{
		"voice_room_starters": "everyone", "voice_public_summary": "TRUE", "retention_voice_participants": "30",
	}}, nil), http.StatusOK, "PATCH the voice settings")
	got = read()
	if v := values(got); strings.Join(v, ",") != "everyone,true,30" {
		t.Errorf("after the change = %v, want everyone, true, 30", v)
	}
	for _, k := range keys {
		if !got[k].Stored {
			t.Errorf("%s is not stored", k)
		}
	}
	settings, err := e.app.Instance.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.VoiceRoomStarters() != "everyone" || !settings.VoicePublicSummary() {
		t.Errorf("settings = %s, %v; want everyone, true", settings.VoiceRoomStarters(), settings.VoicePublicSummary())
	}

	log, err := e.app.Moderation.AuditLog(t.Context(), appmoderation.AuditQuery{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	audited := map[string]bool{}
	for _, entry := range log {
		if entry.Action == moderation.AuditSettingChanged && entry.TargetKind == moderation.AuditTargetSetting {
			audited[entry.TargetID] = true
		}
	}
	for _, k := range keys {
		if !audited[string(k)] {
			t.Errorf("the change of %s is not in the audit log: %+v", k, log)
		}
	}
}

// TestAppVoice checks the voice chat's value that the composition root passes on (ADR-0101 §1),
// and the warning at start of p2p without STUN and TURN.
func TestAppVoice(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_VOICE": "p2p", "MY_VOICE_P2P_MAX_PARTICIPANTS": "6",
		"MY_VOICE_MAX_ROOM_DURATION": "2h", "MY_VOICE_MAX_OPEN_ROOMS": "3",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	want := platform.Voice{Method: platform.VoiceP2P, P2PMaxParticipants: 6, MaxRoomDuration: 2 * time.Hour, MaxOpenRooms: 3}
	if app.Voice != want || !app.Voice.Enabled() || app.Voice.RoomCapacity() != 6 {
		t.Errorf("Voice = %+v, want %+v, enabled, rooms of 6", app.Voice, want)
	}
	if !strings.Contains(logs.String(), "env=MY_VOICE ") || !strings.Contains(logs.String(), "only browsers on the same network connect") {
		t.Errorf("logs = %s, want the warning of p2p without STUN and TURN", logs.String())
	}

	off := platform.Voice{Method: platform.VoiceOff}
	if off.Enabled() || (platform.Voice{}).Enabled() {
		t.Error("voice = off is enabled")
	}
	if sfu := (platform.Voice{Method: platform.VoiceLiveKit, P2PMaxParticipants: 10, MaxParticipants: 100}); sfu.RoomCapacity() != 100 {
		t.Errorf("livekit room capacity = %d, want 100", sfu.RoomCapacity())
	}
}

// TestVoiceRoomsThroughAPI drives the rooms of voice = p2p on SQLite through the client API
// (ADR-0100 §2, §5, §6): the administrator, whose role has start_voice_rooms, opens a room; a
// user without the permission may not, but joins it, sends a heartbeat, and sees it in the list;
// the host leaves only with a choice; and the room ends. voice = livekit has no transport yet.
func TestVoiceRoomsThroughAPI(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	r, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bob := issueTokens(t, app, r.User.ID, "").AccessToken

	if p := e.want(e.do(http.MethodPost, "/voice/rooms", bob, map[string]any{"name": "Bob's"}, nil), http.StatusForbidden, "open by bob"); p != clientapi.ProblemTypeVoiceNotEligible {
		t.Errorf("open by bob = %s", p)
	}
	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk", "hashtags": []string{"go"}}, &opened), http.StatusCreated, "open")
	if opened.Grant.Kind != "p2p" || len(opened.Grant.IceServers) != 1 || opened.Room.Host.Id != alice.id {
		t.Errorf("open = %+v", opened)
	}
	room := "/voice/rooms/" + opened.Room.Id
	var joined clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, room+"/participation", bob, nil, &joined), http.StatusOK, "join")
	e.want(e.do(http.MethodPost, room+"/heartbeat", bob, map[string]any{"session": joined.Session}, nil), http.StatusNoContent, "heartbeat")
	var page clientapi.VoiceRoomPage
	e.want(e.do(http.MethodGet, "/voice/rooms?tag=GO", bob, nil, &page), http.StatusOK, "list")
	if len(page.Items) != 1 || page.Items[0].ParticipantsCount != 2 {
		t.Errorf("list = %+v", page)
	}
	if p := e.want(e.do(http.MethodDelete, room+"/participation", alice.token, nil, nil), http.StatusConflict, "leave by the host"); p != clientapi.ProblemTypeVoiceHostMustChoose {
		t.Errorf("leave by the host = %s", p)
	}
	e.want(e.do(http.MethodDelete, room, alice.token, nil, nil), http.StatusNoContent, "end")
	e.want(e.do(http.MethodGet, room, bob, nil, nil), http.StatusNotFound, "get after the end")

	sfu := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_VOICE": "livekit", "MY_VOICE_LIVEKIT_URL": "wss://livekit.example.org",
		"MY_VOICE_LIVEKIT_API_KEY": "key", "MY_VOICE_LIVEKIT_API_SECRET": "secret",
	})
	if _, err := platform.NewApp(t.Context(), sfu, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)}); err == nil || !strings.Contains(err.Error(), "no transport") {
		t.Errorf("NewApp with voice = livekit: %v, want no transport", err)
	}
}
