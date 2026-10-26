// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
// the host leaves only with a choice; and the room ends.
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

	// The endpoint of LiveKit's events answers 404 with voice = p2p.
	if code := postWebhook(t, app.Handler(), "", []byte("{}")); code != http.StatusNotFound {
		t.Errorf("LiveKit webhook with voice = p2p = %d, want 404", code)
	}
}

// postWebhook posts body to the endpoint of LiveKit's events with the Authorization header auth
// and returns the status.
func postWebhook(t *testing.T, h http.Handler, auth string, body []byte) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/voice/webhooks/livekit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/webhook+json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// liveKitWebhookToken signs a token of LiveKit's webhooks for body: HS256 with secret, the API
// key in iss, and the base64 SHA-256 of body in sha256.
func liveKitWebhookToken(key, secret string, body []byte) string {
	enc := base64.RawURLEncoding
	sum := sha256.Sum256(body)
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{
		"iss": key, "nbf": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "sha256": base64.StdEncoding.EncodeToString(sum[:]),
	})
	signing := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + enc.EncodeToString(mac.Sum(nil))
}

// TestVoiceLiveKit starts the server with voice = livekit and checks the endpoint of LiveKit's
// events (ADR-0102 §4): a delivery signed with the API secret is accepted, and one that does not
// verify, such as a tampered body, is refused.
func TestVoiceLiveKit(t *testing.T) {
	t.Parallel()
	const key, secret = "APIkey", "a-secret-of-the-livekit-api-key"
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_VOICE": "livekit", "MY_VOICE_LIVEKIT_URL": "wss://livekit.example.org",
		"MY_VOICE_LIVEKIT_API_KEY": key, "MY_VOICE_LIVEKIT_API_SECRET": secret,
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !app.VoiceRooms.Enabled() {
		t.Fatal("voice = livekit is not enabled")
	}
	body := []byte(`{"event":"participant_left","id":"EV_1","createdAt":"1793000000","room":{"name":"room-1"},"participant":{"identity":"session-1"}}`)
	token := liveKitWebhookToken(key, secret, body)
	if code := postWebhook(t, app.Handler(), token, body); code != http.StatusNoContent {
		t.Errorf("signed delivery = %d, want 204", code)
	}
	tampered := bytes.Replace(body, []byte("session-1"), []byte("session-2"), 1)
	if code := postWebhook(t, app.Handler(), token, tampered); code != http.StatusBadRequest {
		t.Errorf("tampered delivery = %d, want 400", code)
	}
	if code := postWebhook(t, app.Handler(), liveKitWebhookToken(key, "another secret", body), body); code != http.StatusBadRequest {
		t.Errorf("delivery signed with another secret = %d, want 400", code)
	}
	if code := postWebhook(t, app.Handler(), "", body); code != http.StatusBadRequest {
		t.Errorf("unsigned delivery = %d, want 400", code)
	}
}

// TestVoiceParticipantsThroughAPI drives the participant operations of voice = p2p on SQLite
// through the client API (ADR-0100 §4, §5, §7, §11): a listener who may not speak raises a hand
// and speaks after the host's grant, the host invites a user as a speaker and makes a co-host,
// a participant may not moderate, and a block by the host takes the blocked participant out of
// the room through graph.blocked.
func TestVoiceParticipantsThroughAPI(t *testing.T) {
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
	member := func(name string) testUser {
		t.Helper()
		r, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple"})
		if err != nil {
			t.Fatal(err)
		}
		return testUser{id: r.AccountID, token: issueTokens(t, app, r.User.ID, "").AccessToken}
	}
	bob, carol := member("bob"), member("carol")

	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk"}, &opened), http.StatusCreated, "open")
	room := "/voice/rooms/" + opened.Room.Id
	e.want(e.do(http.MethodPost, room+"/participation", bob.token, nil, nil), http.StatusOK, "join by bob")
	if p := e.want(e.do(http.MethodPost, room+"/speaker", bob.token, nil, nil), http.StatusForbidden, "speak without a grant"); p != clientapi.ProblemTypeVoiceCannotSpeak {
		t.Errorf("speak without a grant = %s", p)
	}
	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodPost, room+"/hand", bob.token, nil, &got), http.StatusOK, "raise a hand")
	if len(got.Participants) != 2 || !got.Participants[1].HandRaised {
		t.Errorf("after raising a hand = %+v", got.Participants)
	}
	e.want(e.do(http.MethodPost, room+"/participants/"+bob.id+"/grant", alice.token, nil, nil), http.StatusOK, "grant")
	e.want(e.do(http.MethodPost, room+"/speaker", bob.token, nil, &got), http.StatusOK, "speak after the grant")
	if got.Participants[1].Role != clientapi.VoiceRoleSpeaker || got.Participants[1].HandRaised {
		t.Errorf("after speaking = %+v", got.Participants[1])
	}
	if p := e.want(e.do(http.MethodPost, room+"/participants/"+alice.id+"/mute", bob.token, nil, nil), http.StatusForbidden, "mute by a participant"); p != clientapi.ProblemTypeVoiceNotHostOrCohost {
		t.Errorf("mute by a participant = %s", p)
	}

	var inv clientapi.VoiceRoomInvitation
	e.want(e.do(http.MethodPost, room+"/invitations", alice.token, map[string]any{"account_id": carol.id, "role": "speaker"}, &inv), http.StatusCreated, "invite")
	if inv.Account.Id != carol.id || inv.Role != clientapi.VoiceRoleSpeaker {
		t.Errorf("invitation = %+v", inv)
	}
	if p := e.want(e.do(http.MethodPost, room+"/invitations", alice.token, map[string]any{"account_id": carol.id}, nil), http.StatusConflict, "invite again"); p != clientapi.ProblemTypeVoiceAlreadyInvited {
		t.Errorf("invite again = %s", p)
	}
	var invs clientapi.VoiceRoomInvitationPage
	e.want(e.do(http.MethodGet, room+"/invitations", alice.token, nil, &invs), http.StatusOK, "invitations")
	if len(invs.Items) != 1 || invs.Items[0].Account.Id != carol.id {
		t.Errorf("invitations = %+v", invs)
	}
	e.want(e.do(http.MethodPost, room+"/participation", carol.token, nil, nil), http.StatusOK, "join by carol")
	e.want(e.do(http.MethodPost, room+"/speaker", carol.token, nil, nil), http.StatusOK, "speak after an invitation as a speaker")
	e.want(e.do(http.MethodPost, room+"/participants/"+carol.id+"/cohost", alice.token, nil, &got), http.StatusOK, "make a co-host")
	if !got.Participants[2].Cohost {
		t.Errorf("after making a co-host = %+v", got.Participants[2])
	}

	e.want(e.do(http.MethodPost, "/accounts/"+bob.id+"/block", alice.token, nil, nil), http.StatusOK, "block by the host")
	if _, err := app.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	e.want(e.do(http.MethodGet, room, alice.token, nil, &got), http.StatusOK, "get after the block")
	for _, p := range got.Participants {
		if p.Account.Id == bob.id {
			t.Errorf("the participant blocked by the host is still in the room: %+v", got.Participants)
		}
	}
	e.want(e.do(http.MethodGet, room, bob.token, nil, nil), http.StatusNotFound, "get by the blocked participant")
	e.want(e.do(http.MethodDelete, room+"/participants/"+carol.id, alice.token, nil, nil), http.StatusUnprocessableEntity, "remove a co-host")
}

// postSora posts body to an endpoint of Sora's webhooks with Basic authentication and returns
// the status and the body of the answer.
func postSora(t *testing.T, h http.Handler, path, user, pass string, body []byte) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// TestVoiceSora starts the server with voice = sora on a self-hosted Sora and checks the
// endpoints of Sora's webhooks (ADR-0102 §4, §5): with the Basic authentication of the
// webhooks, the auth webhook answers 200 and refuses a connection without a join value, and
// the events endpoint answers 200 with a JSON object; without it, both refuse.
func TestVoiceSora(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_VOICE": "sora", "MY_VOICE_SORA_MODE": "self_hosted",
		"MY_VOICE_SORA_SIGNALING_URLS": "wss://sora.example.org/signaling", "MY_VOICE_SORA_API_URL": "https://sora-api.example.org",
		"MY_VOICE_SORA_WEBHOOK_USERNAME": "sora", "MY_VOICE_SORA_WEBHOOK_PASSWORD": "hunter2",
		"MY_VOICE_JOIN_SECRET": "a-join-secret-of-at-least-32-bytes!",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !app.VoiceRooms.Enabled() {
		t.Fatal("voice = sora is not enabled")
	}
	auth := []byte(`{"channel_id":"room-1","connection_id":"CONN1","role":"recvonly","metadata":{"join":"forged"}}`)
	code, body := postSora(t, app.Handler(), "/voice/webhooks/sora/auth", "sora", "hunter2", auth)
	if code != http.StatusOK || !strings.Contains(body, `"allowed":false`) {
		t.Errorf("auth webhook = %d %s, want 200 and a refusal", code, body)
	}
	if code, _ := postSora(t, app.Handler(), "/voice/webhooks/sora/auth", "sora", "wrong", auth); code != http.StatusUnauthorized {
		t.Errorf("auth webhook with a wrong password = %d, want 401", code)
	}
	event := []byte(`{"type":"connection.destroyed","id":"EV_1","channel_id":"room-1","client_id":"session-1","data":{"reason":"normal"}}`)
	if code, body := postSora(t, app.Handler(), "/voice/webhooks/sora/events", "sora", "hunter2", event); code != http.StatusOK || body != "{}" {
		t.Errorf("events webhook = %d %s, want 200 {}", code, body)
	}
	if code, _ := postSora(t, app.Handler(), "/voice/webhooks/sora/events", "other", "hunter2", event); code != http.StatusBadRequest {
		t.Errorf("events webhook with a wrong user = %d, want 400", code)
	}
	if code := postWebhook(t, app.Handler(), "", []byte("{}")); code != http.StatusNotFound {
		t.Errorf("LiveKit webhook with voice = sora = %d, want 404", code)
	}
}

// TestPermissionsPolicyAllowsTheMicrophoneOnlyWithVoice checks the decider's answer 46 in
// #1345: Permissions-Policy allows the microphone for the server's own origin while the voice
// chat is on, and denies it while it is off.
func TestPermissionsPolicyAllowsTheMicrophoneOnlyWithVoice(t *testing.T) {
	t.Parallel()
	for voice, want := range map[string]string{
		"off": "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
		"p2p": "camera=(), microphone=(self), geolocation=(), payment=(), usb=()",
	} {
		t.Run(voice, func(t *testing.T) {
			t.Parallel()
			cfg := loadConfig(t, map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": voice, "MY_VOICE_STUN_URLS": "stun:stun.example.org",
			})
			app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			w := httptest.NewRecorder()
			app.Handler().ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, platform.PathLiveness, nil))
			if got := w.Header().Get("Permissions-Policy"); got != want {
				t.Errorf("Permissions-Policy = %q, want %q", got, want)
			}
		})
	}
}
