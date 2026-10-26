// SPDX-License-Identifier: AGPL-3.0-only

package sora

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	testJoinSecret = "a-join-secret-of-at-least-32-bytes!"
	testAPIKey     = "a-sora-cloud-api-key" //nolint:gosec // G101: a made-up key of the tests
	testProject    = "PROJECT1"
	testUser       = "sora"
	testPassword   = "hunter2"
	testHeaderName = "X-Proxy-Auth"
	testHeaderVal  = "proxy-secret"
)

// newTestTransport returns a Transport of mode on a fakeServer.
func newTestTransport(t *testing.T, mode string, clock *testkit.Clock) (*Transport, *fakeServer) {
	t.Helper()
	o := Options{
		Mode: mode, SignalingURLs: []string{"wss://sora1.example.org/signaling", "wss://sora2.example.org/signaling"},
		JoinSecret: testJoinSecret, Clock: clock,
	}
	var f *fakeServer
	switch mode {
	case ModeSelfHosted:
		ff, srv := newFakeServer(t, testHeaderName, testHeaderVal)
		f = ff
		o.APIURL, o.APIHeaderName, o.APIHeaderValue = srv.URL+"/", testHeaderName, testHeaderVal
		o.WebhookUsername, o.WebhookPassword = testUser, testPassword
		o.HTTPClient = srv.Client()
	case ModeCloud:
		ff, srv := newFakeServer(t, "Authorization", "Bearer "+testAPIKey)
		f = ff
		o.APIURL, o.CloudProjectID, o.CloudAPIKey = srv.URL+"/sora-api", testProject, testAPIKey
		o.HTTPClient = srv.Client()
	}
	tr, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return tr, f
}

var modes = []string{ModeSelfHosted, ModeCloud}

func TestVoiceTransportContract(t *testing.T) {
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			testkit.VoiceTransportContract(t, func(t *testing.T) testkit.VoiceTransportHarness {
				clock := testkit.VoiceTransportClock()
				tr, f := newTestTransport(t, mode, clock)
				return testkit.VoiceTransportHarness{
					Transport: tr, Clock: clock,
					Connect: func(_ *testing.T, room appvoice.RoomRef, s appvoice.Session) { f.connect(tr.channel(room.ID), s.ID) },
				}
			})
		})
	}
}

func TestNew(t *testing.T) {
	base := Options{
		Mode: ModeSelfHosted, SignalingURLs: []string{"wss://sora.example.org/signaling"}, APIURL: "https://sora-api.example.org",
		WebhookUsername: testUser, WebhookPassword: testPassword, JoinSecret: testJoinSecret, HTTPClient: &http.Client{},
		Clock: testkit.VoiceTransportClock(),
	}
	if _, err := New(base); err != nil {
		t.Fatalf("New: %v", err)
	}
	cloud := base
	cloud.Mode, cloud.APIURL, cloud.CloudProjectID, cloud.CloudAPIKey = ModeCloud, "", testProject, testAPIKey
	tr, err := New(cloud)
	if err != nil || tr.o.APIURL != CloudAPIURL {
		t.Fatalf("New(cloud) = %v, %v; want the API URL of Sora Cloud", tr, err)
	}
	for name, mut := range map[string]func(o *Options){
		"unknown mode":         func(o *Options) { o.Mode = "other" },
		"no API URL":           func(o *Options) { o.APIURL = "" },
		"no webhook password":  func(o *Options) { o.WebhookPassword = "" },
		"header without value": func(o *Options) { o.APIHeaderName = "X-Auth" },
		"no signaling URL":     func(o *Options) { o.SignalingURLs = nil },
		"no join secret":       func(o *Options) { o.JoinSecret = "" },
		"no HTTP client":       func(o *Options) { o.HTTPClient = nil },
		"cloud without key":    func(o *Options) { o.Mode, o.CloudProjectID = ModeCloud, testProject },
	} {
		o := base
		mut(&o)
		if _, err := New(o); err == nil {
			t.Errorf("%s: New succeeded", name)
		}
	}
}

func TestGrant(t *testing.T) {
	for _, mode := range modes {
		clock := testkit.VoiceTransportClock()
		tr, _ := newTestTransport(t, mode, clock)
		room := appvoice.RoomRef{ID: "room-1"}
		s := appvoice.Session{ID: "session-1", AccountID: "account-1"}
		for _, speak := range []bool{true, false} {
			g, err := tr.Grant(t.Context(), room, s, speak)
			if err != nil {
				t.Fatal(err)
			}
			wantRole, wantChannel := role(speak), tr.channel(room.ID)
			if g.Kind != Kind || g.URL != "wss://sora1.example.org/signaling" || !g.ExpiresAt.Equal(clock.Now().Add(GrantTTL)) || len(g.ICE) != 0 {
				t.Errorf("%s Grant(%t) = %+v", mode, speak, g)
			}
			if g.Options["role"] != wantRole || g.Options["channel_id"] != wantChannel ||
				g.Options["signaling_urls"] != "wss://sora1.example.org/signaling,wss://sora2.example.org/signaling" {
				t.Errorf("%s Grant(%t) options = %v", mode, speak, g.Options)
			}
			v, err := verifyJoin(g.Token, testJoinSecret, clock.Now())
			if err != nil || v != (joinValue{Room: room.ID, Session: s.ID, Role: wantRole, ExpiresAt: g.ExpiresAt.Unix()}) {
				t.Errorf("%s Grant(%t) join value = %+v, %v", mode, speak, v, err)
			}
			if strings.Contains(g.Token, s.AccountID) || strings.Contains(g.Options["access_token"], s.AccountID) {
				t.Errorf("%s Grant(%t) names the account", mode, speak)
			}
			if mode == ModeSelfHosted {
				if _, ok := g.Options["access_token"]; ok {
					t.Errorf("self_hosted Grant(%t) has an access token", speak)
				}
				continue
			}
			c := decodeAccessToken(t, g.Options["access_token"], testAPIKey)
			if c != (accessClaims{ChannelID: "room-1@" + testProject, Role: wantRole, NotBefore: clock.Now().Unix(), ExpiresAt: g.ExpiresAt.Unix()}) {
				t.Errorf("cloud Grant(%t) access token = %+v", speak, c)
			}
		}
	}
}

// decodeAccessToken verifies an HS256 JWT with key and returns its claims.
func decodeAccessToken(t *testing.T, token, key string) accessClaims {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("access token %q is not a JWT", token)
	}
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(parts[0] + "." + parts[1]))
	if sig, _ := base64.RawURLEncoding.DecodeString(parts[2]); !hmac.Equal(sig, h.Sum(nil)) {
		t.Fatal("access token signature does not verify")
	}
	head, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if string(head) != `{"alg":"HS256","typ":"JWT"}` {
		t.Errorf("header = %s", head)
	}
	b, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c accessClaims
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestJoinValueRejects(t *testing.T) {
	now := testkit.VoiceTransportClock().Now()
	good, err := signJoin(joinValue{Room: "r", Session: "s", Role: roleRecvOnly, ExpiresAt: now.Add(time.Minute).Unix()}, testJoinSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyJoin(good, testJoinSecret, now); err != nil {
		t.Fatalf("verifyJoin: %v", err)
	}
	payload, sig, _ := strings.Cut(good, ".")
	other, _ := base64.RawURLEncoding.DecodeString(payload)
	forged := base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(other), `"recvonly"`, `"sendrecv"`, 1))) + "." + sig
	badRole, _ := signJoin(joinValue{Room: "r", Session: "s", Role: "sendonly", ExpiresAt: now.Add(time.Minute).Unix()}, testJoinSecret)
	for name, c := range map[string]struct {
		value string
		at    time.Time
	}{
		"forged role":    {forged, now},
		"other secret":   {good, now},
		"expired":        {good, now.Add(time.Minute)},
		"no signature":   {payload, now},
		"empty":          {"", now},
		"bad base64":     {"%%%." + sig, now},
		"sendonly grant": {badRole, now},
	} {
		secret := testJoinSecret
		if name == "other secret" {
			secret = "another-join-secret-of-32-bytes-or-more"
		}
		if _, err := verifyJoin(c.value, secret, c.at); !errors.Is(err, ErrJoinValue) {
			t.Errorf("%s: verifyJoin = %v, want ErrJoinValue", name, err)
		}
	}
}

func TestSpeakingAndMute(t *testing.T) {
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			clock := testkit.VoiceTransportClock()
			tr, f := newTestTransport(t, mode, clock)
			room := appvoice.RoomRef{ID: "room-2"}
			ch := tr.channel(room.ID)
			a, b := appvoice.Session{ID: "session-a"}, appvoice.Session{ID: "session-b"}
			ca, cb := f.connect(ch, a.ID), f.connect(ch, b.ID)
			blocked := func(want ...string) {
				t.Helper()
				if got := f.blocked(ch); !slices.Equal(got, want) && (len(want) != 0 || len(got) != 0) {
					t.Errorf("blocked = %v, want %v", got, want)
				}
			}

			// An allow reconnects without blocking anything.
			if reconnect, err := tr.SetSpeaking(t.Context(), room, a, true); err != nil || !reconnect {
				t.Fatalf("SetSpeaking(true) = %t, %v; want a reconnect", reconnect, err)
			}
			blocked()
			// A revoke blocks the audio of the session's connection at once and reconnects.
			if reconnect, err := tr.SetSpeaking(t.Context(), room, a, false); err != nil || !reconnect {
				t.Fatalf("SetSpeaking(false) = %t, %v; want a reconnect", reconnect, err)
			}
			blocked(ca)
			f.mu.Lock()
			rules := f.filters[ch].Rules
			f.mu.Unlock()
			want := [][]rule{{{Field: "connection_id", Operator: "is_in", Values: []string{ca}}, {Field: "kind", Operator: "is_in", Values: []string{"audio"}}}}
			if !jsonEqual(t, rules, want) {
				t.Errorf("filter rules = %+v, want %+v", rules, want)
			}
			// An Unmute never lifts a revoke; Mute adds b, twice is once.
			if err := tr.Unmute(t.Context(), room, a); err != nil {
				t.Fatal(err)
			}
			blocked(ca)
			for range 2 {
				if err := tr.Mute(t.Context(), room, b); err != nil {
					t.Fatal(err)
				}
			}
			blocked(ca, cb)
			// A mute and a revoke of one session: the allow lifts the revoke only.
			if err := tr.Mute(t.Context(), room, a); err != nil {
				t.Fatal(err)
			}
			if _, err := tr.SetSpeaking(t.Context(), room, a, true); err != nil {
				t.Fatal(err)
			}
			blocked(ca, cb)
			if err := tr.Unmute(t.Context(), room, a); err != nil {
				t.Fatal(err)
			}
			blocked(cb)
			// A muted session stays muted when it connects again.
			cb2 := f.connect(ch, b.ID)
			if err := tr.Remove(t.Context(), room, appvoice.Session{ID: "nobody"}); err != nil {
				t.Fatal(err)
			}
			if _, err := tr.SetSpeaking(t.Context(), room, a, true); err != nil {
				t.Fatal(err)
			}
			blocked(cb, cb2)
			// The last unmute deletes the filter; an unmute without one does nothing.
			for range 2 {
				if err := tr.Unmute(t.Context(), room, b); err != nil {
					t.Fatal(err)
				}
			}
			f.mu.Lock()
			_, ok := f.filters[ch]
			f.mu.Unlock()
			if ok {
				t.Error("the filter stays without a blocked session")
			}

			// A listener's recvonly connection sends no audio: nothing to block.
			f.connectAs(ch, "session-l", roleRecvOnly)
			if _, err := tr.SetSpeaking(t.Context(), room, appvoice.Session{ID: "session-l"}, false); err != nil {
				t.Fatal(err)
			}
			blocked()
			// A revoked session that keeps a sendrecv connection, as a client that does not
			// connect again recvonly, loses it when the role is applied again.
			c := appvoice.Session{ID: "session-c"}
			cc := f.connect(ch, c.ID)
			if _, err := tr.SetSpeaking(t.Context(), room, c, false); err != nil {
				t.Fatal(err)
			}
			blocked(cc)
			if _, err := tr.SetSpeaking(t.Context(), room, c, false); err != nil {
				t.Fatal(err)
			}
			for _, conn := range f.conns(ch) {
				if conn.ConnectionID == cc {
					t.Errorf("the revoked session keeps its sendrecv connection %s", cc)
				}
			}
			blocked()
			// A session that is not connected has nothing to mute now, but is muted when it
			// connects.
			if err := tr.Mute(t.Context(), room, appvoice.Session{ID: "absent"}); err != nil {
				t.Errorf("Mute of an absent session: %v", err)
			}
			blocked()
			cx := f.connect(ch, "absent")
			if _, err := tr.SetSpeaking(t.Context(), room, appvoice.Session{ID: "absent"}, true); err != nil {
				t.Fatal(err)
			}
			blocked(cx)
		})
	}
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	return string(x) == string(y)
}

func TestFilterVersionConflict(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, f := newTestTransport(t, ModeSelfHosted, clock)
	room := appvoice.RoomRef{ID: "room-v"}
	f.connect(room.ID, "s1")
	f.connect(room.ID, "s2")
	if err := tr.Mute(t.Context(), room, appvoice.Session{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	// Another change races: the version Sora has is not the one the adapter read.
	var e *APIError
	next := tr.o.HTTPClient.Transport
	tr.o.HTTPClient.Transport = versionSwap{f: f, next: next}
	err := tr.Mute(t.Context(), room, appvoice.Session{ID: "s2"})
	if !errors.As(err, &e) || e.Status != http.StatusBadRequest || !strings.Contains(e.Body, "INVALID-VERSION") {
		t.Errorf("Mute that raced = %v, want INVALID-VERSION", err)
	}
}

// versionSwap changes the version of the filter after the adapter listed it, as a change that
// raced would.
type versionSwap struct {
	f    *fakeServer
	next http.RoundTripper
}

func (v versionSwap) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := v.next.RoundTrip(r)
	if r.Header.Get("x-sora-target") == targetListForwardingFilters {
		v.f.mu.Lock()
		for _, fl := range v.f.filters {
			fl.Version = "raced"
		}
		v.f.mu.Unlock()
	}
	return res, err
}

func TestRemoveCloseAndDuplicates(t *testing.T) {
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			clock := testkit.VoiceTransportClock()
			tr, f := newTestTransport(t, mode, clock)
			room := appvoice.RoomRef{ID: "room-3"}
			ch := tr.channel(room.ID)
			old := f.connect(ch, "session-a")
			f.connect(ch, "session-b")
			newer := f.connect(ch, "session-a")
			// The newest connection of a duplicated client_id stays.
			if err := tr.Deduplicate(t.Context(), room, appvoice.Session{ID: "session-a"}); err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, c := range f.conns(ch) {
				ids = append(ids, c.ConnectionID)
			}
			if slices.Contains(ids, old) || !slices.Contains(ids, newer) || len(ids) != 2 {
				t.Errorf("connections after Deduplicate = %v, want the newest of session-a and session-b", ids)
			}
			// Sessions lists each session once and disconnects the older duplicates too.
			newestB := f.connect(ch, "session-b")
			ss, err := tr.Sessions(t.Context(), room)
			if err != nil {
				t.Fatal(err)
			}
			want := []appvoice.SessionRef{{ID: "session-a", Connection: newer}, {ID: "session-b", Connection: newestB}}
			if !slices.Equal(ss, want) || len(f.conns(ch)) != 2 {
				t.Errorf("Sessions = %v with connections %v, want %v, one connection each", ss, f.conns(ch), want)
			}
			if err := tr.Remove(t.Context(), room, appvoice.Session{ID: "session-b"}); err != nil {
				t.Fatal(err)
			}
			if got := f.conns(ch); len(got) != 1 || got[0].ClientID != "session-a" {
				t.Errorf("connections after Remove = %v", got)
			}
			if err := tr.Close(t.Context(), room); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			calls := slices.Clone(f.calls)
			f.mu.Unlock()
			if !slices.Contains(calls, targetTerminateSession+" "+ch) {
				t.Errorf("calls = %v, want TerminateSession of %s", calls, ch)
			}
		})
	}
}

func TestCallFailure(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	for _, mode := range modes {
		tr, _ := newTestTransport(t, mode, clock)
		// The reverse proxy or Sora Cloud refuses a wrong credential.
		tr.o.APIHeaderValue, tr.o.CloudAPIKey = "wrong", "wrong"
		_, err := tr.Sessions(t.Context(), appvoice.RoomRef{ID: "room"})
		var e *APIError
		if !errors.As(err, &e) || e.Status != http.StatusUnauthorized {
			t.Errorf("%s: Sessions with a wrong credential = %v, want 401", mode, err)
		}
	}
}

// basic returns the header of a delivery of a self-hosted Sora with Basic authentication.
func basic(user, pass string) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	return h
}

// signed returns the header of a delivery of Sora Cloud, signed with key at ts.
func signed(key string, ts time.Time, body []byte) http.Header {
	t := strconv.FormatInt(ts.Unix(), 10)
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(t + "." + string(body)))
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("sora-cloud-signature", "t="+t+", v1="+hex.EncodeToString(m.Sum(nil)))
	return h
}

// header returns the authentic header of a delivery of body in mode.
func header(mode string, now time.Time, body []byte) http.Header {
	if mode == ModeCloud {
		return signed(testAPIKey, now, body)
	}
	return basic(testUser, testPassword)
}

func TestWebhookAuthentication(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	now := clock.Now()
	body := []byte(`{"type":"connection.created","id":"EV_1","channel_id":"room-1@` + testProject + `","client_id":"session-1"}`)
	tampered := []byte(strings.Replace(string(body), "session-1", "session-2", 1))

	self, _ := newTestTransport(t, ModeSelfHosted, clock)
	for name, h := range map[string]http.Header{
		"wrong user":     basic("other", testPassword),
		"wrong password": basic(testUser, "wrong"),
		"no auth":        {},
		"bearer":         {"Authorization": {"Bearer " + testPassword}},
		"signature only": signed(testAPIKey, now, body),
	} {
		if _, err := self.ParseEvents(t.Context(), h, body); !errors.Is(err, ErrWebhookAuth) {
			t.Errorf("self_hosted %s: ParseEvents = %v, want ErrWebhookAuth", name, err)
		}
		if status, _, err := self.AuthorizeConnect(t.Context(), h, body, allowAll); status != http.StatusUnauthorized || !errors.Is(err, ErrWebhookAuth) {
			t.Errorf("self_hosted %s: AuthorizeConnect = %d, %v; want 401", name, status, err)
		}
	}
	if _, err := self.ParseEvents(t.Context(), basic(testUser, testPassword), body); err != nil {
		t.Errorf("self_hosted with Basic: %v", err)
	}

	cloud, _ := newTestTransport(t, ModeCloud, clock)
	good := signed(testAPIKey, now, body)
	for name, c := range map[string]struct {
		h    http.Header
		body []byte
	}{
		"tampered body":   {good, tampered},
		"wrong key":       {signed("another key", now, body), body},
		"too old":         {signed(testAPIKey, now.Add(-SignatureTolerance-time.Second), body), body},
		"from the future": {signed(testAPIKey, now.Add(SignatureTolerance+time.Second), body), body},
		"no signature":    {http.Header{}, body},
		"no timestamp":    {http.Header{"Sora-Cloud-Signature": {"v1=" + strings.Repeat("0", 64)}}, body},
		"not hex":         {http.Header{"Sora-Cloud-Signature": {"t=" + strconv.FormatInt(now.Unix(), 10) + ", v1=zz"}}, body},
		"basic only":      {basic(testUser, testPassword), body},
	} {
		if _, err := cloud.ParseEvents(t.Context(), c.h, c.body); !errors.Is(err, ErrWebhookAuth) {
			t.Errorf("cloud %s: ParseEvents = %v, want ErrWebhookAuth", name, err)
		}
		if status, _, err := cloud.AuthorizeConnect(t.Context(), c.h, c.body, allowAll); status != http.StatusUnauthorized || !errors.Is(err, ErrWebhookAuth) {
			t.Errorf("cloud %s: AuthorizeConnect = %d, %v; want 401", name, status, err)
		}
	}
	if _, err := cloud.ParseEvents(t.Context(), good, body); err != nil {
		t.Errorf("cloud signed: %v", err)
	}
	// The signature without the space after the comma verifies too.
	compact := http.Header{"Sora-Cloud-Signature": {strings.ReplaceAll(good.Get("sora-cloud-signature"), ", ", ",")}}
	if _, err := cloud.ParseEvents(t.Context(), compact, body); err != nil {
		t.Errorf("cloud signed without a space: %v", err)
	}
}

func allowAll(appvoice.ConnectAttempt) appvoice.ConnectDecision {
	return appvoice.ConnectDecision{Allowed: true}
}

func TestAuthorizeConnect(t *testing.T) {
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			clock := testkit.VoiceTransportClock()
			tr, _ := newTestTransport(t, mode, clock)
			room := appvoice.RoomRef{ID: "room-4"}
			s := appvoice.Session{ID: "session-4", AccountID: "account-4"}
			speaker, err := tr.Grant(t.Context(), room, s, true)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := tr.Grant(t.Context(), room, s, false)
			if err != nil {
				t.Fatal(err)
			}
			other, err := tr.Grant(t.Context(), appvoice.RoomRef{ID: "room-other"}, s, true)
			if err != nil {
				t.Fatal(err)
			}
			ch := tr.channel(room.ID)
			request := func(channel, role string, metadata any) []byte {
				b, _ := json.Marshal(map[string]any{
					"id": "AUTH_1", "channel_id": channel, "connection_id": "CONN1", "role": role, "client_id": "chosen-by-client",
					"metadata": metadata, "audio": true, "video": true,
				})
				return b
			}
			refuseBanned := func(appvoice.ConnectAttempt) appvoice.ConnectDecision {
				return appvoice.ConnectDecision{Reason: "banned from the room"}
			}
			long := func(appvoice.ConnectAttempt) appvoice.ConnectDecision {
				return appvoice.ConnectDecision{Reason: strings.Repeat("あ", 50)}
			}
			cases := map[string]struct {
				body   []byte
				decide func(appvoice.ConnectAttempt) appvoice.ConnectDecision
				// want is the attempt decide sees; nil when the adapter refuses first.
				want   *appvoice.ConnectAttempt
				reason string
			}{
				"speaker sendrecv": {
					body: request(ch, "sendrecv", map[string]any{"join": speaker.Token, "access_token": speaker.Options["access_token"]}), decide: allowAll,
					want: &appvoice.ConnectAttempt{Room: room, Session: s.ID, Speaker: true, Token: speaker.Token},
				},
				"speaker recvonly": {
					body: request(ch, "recvonly", map[string]any{"join": speaker.Token}), decide: allowAll,
					want: &appvoice.ConnectAttempt{Room: room, Session: s.ID, Token: speaker.Token},
				},
				"listener recvonly": {
					body: request(ch, "recvonly", map[string]any{"join": listener.Token}), decide: allowAll,
					want: &appvoice.ConnectAttempt{Room: room, Session: s.ID, Token: listener.Token},
				},
				"listener sendrecv": {body: request(ch, "sendrecv", map[string]any{"join": listener.Token}), reason: reasonRole},
				"sendonly":          {body: request(ch, "sendonly", map[string]any{"join": speaker.Token}), reason: reasonRole},
				"another channel":   {body: request(ch, "sendrecv", map[string]any{"join": other.Token}), reason: reasonChannel},
				"no metadata":       {body: request(ch, "recvonly", nil), reason: reasonJoin},
				"no join value":     {body: request(ch, "recvonly", map[string]any{"access_token": "x"}), reason: reasonJoin},
				"forged join value": {body: request(ch, "recvonly", map[string]any{"join": listener.Token + "x"}), reason: reasonJoin},
				"not JSON":          {body: []byte("not json"), reason: reasonBadRequest},
				"no channel":        {body: request("", "recvonly", map[string]any{"join": listener.Token}), reason: reasonBadRequest},
				"refused by the room": {
					body: request(ch, "recvonly", map[string]any{"join": listener.Token}), decide: refuseBanned,
					want: &appvoice.ConnectAttempt{Room: room, Session: s.ID, Token: listener.Token}, reason: "banned from the room",
				},
				"long reason": {
					body: request(ch, "recvonly", map[string]any{"join": listener.Token}), decide: long,
					want: &appvoice.ConnectAttempt{Room: room, Session: s.ID, Token: listener.Token}, reason: strings.Repeat("あ", 33),
				},
			}
			if mode == ModeCloud {
				cases["channel of another project"] = struct {
					body   []byte
					decide func(appvoice.ConnectAttempt) appvoice.ConnectDecision
					want   *appvoice.ConnectAttempt
					reason string
				}{body: request(room.ID+"@OTHER", "recvonly", map[string]any{"join": listener.Token}), reason: reasonChannel}
			}
			for name, c := range cases {
				var got []appvoice.ConnectAttempt
				decide := func(a appvoice.ConnectAttempt) appvoice.ConnectDecision {
					got = append(got, a)
					if c.decide == nil {
						return allowAll(a)
					}
					return c.decide(a)
				}
				status, res, err := tr.AuthorizeConnect(t.Context(), header(mode, clock.Now(), c.body), c.body, decide)
				if err != nil || status != http.StatusOK {
					t.Errorf("%s: AuthorizeConnect = %d, %v; want 200", name, status, err)
					continue
				}
				switch {
				case c.want == nil && len(got) != 0:
					t.Errorf("%s: decide saw %+v, want the adapter to refuse first", name, got)
				case c.want != nil && (len(got) != 1 || got[0] != *c.want):
					t.Errorf("%s: decide saw %+v, want %+v", name, got, *c.want)
				}
				var r map[string]any
				if err := json.Unmarshal(res, &r); err != nil {
					t.Fatalf("%s: response %s: %v", name, res, err)
				}
				if c.reason != "" {
					if r["allowed"] != false || r["reason"] != c.reason || len(r) != 2 {
						t.Errorf("%s: response = %v, want a refusal for %q", name, r, c.reason)
					}
					if reason, _ := r["reason"].(string); len(reason) > maxReasonBytes {
						t.Errorf("%s: reason of %d bytes", name, len(reason))
					}
					continue
				}
				want := map[string]any{"allowed": true, "client_id": s.ID, "audio": true, "audio_codec_type": "OPUS", "video": false}
				if !jsonEqual(t, r, want) {
					t.Errorf("%s: response = %v, want %v", name, r, want)
				}
			}
		})
	}
}

func TestParseEvents(t *testing.T) {
	for _, mode := range modes {
		clock := testkit.VoiceTransportClock()
		tr, _ := newTestTransport(t, mode, clock)
		ch := tr.channel("room-5")
		at := "2026-10-26T09:00:00.123456Z"
		occurred, _ := time.Parse(time.RFC3339Nano, at)
		for name, c := range map[string]struct {
			event map[string]any
			want  []appvoice.Event
		}{
			"created": {
				map[string]any{"type": "connection.created", "id": "EV_1", "timestamp": at, "channel_id": ch, "client_id": "s", "connection_id": "C1"},
				[]appvoice.Event{{ID: "EV_1", Kind: appvoice.MediaSessionJoined, Room: appvoice.RoomRef{ID: "room-5"}, Session: "s", Connection: "C1", OccurredAt: occurred}},
			},
			"destroyed": {
				map[string]any{"type": "connection.destroyed", "id": "EV_2", "timestamp": at, "channel_id": ch, "client_id": "s", "connection_id": "C2", "data": map[string]any{"reason": "normal"}},
				[]appvoice.Event{{ID: "EV_2", Kind: appvoice.MediaSessionLeft, Room: appvoice.RoomRef{ID: "room-5"}, Session: "s", Connection: "C2", OccurredAt: occurred}},
			},
			"replaced by a duplicate": {
				map[string]any{"type": "connection.destroyed", "id": "EV_3", "channel_id": ch, "client_id": "s", "data": map[string]any{"reason": "duplicate_client_id"}},
				nil,
			},
			"session destroyed": {
				map[string]any{"type": "session.destroyed", "id": "EV_4", "timestamp": at, "channel_id": ch, "session_id": "S1", "reason": "terminated_api"},
				[]appvoice.Event{{ID: "EV_4", Kind: appvoice.MediaRoomFinished, Room: appvoice.RoomRef{ID: "room-5"}, OccurredAt: occurred}},
			},
			"session created": {map[string]any{"type": "session.created", "id": "EV_5", "channel_id": ch}, nil},
			"updated":         {map[string]any{"type": "connection.updated", "id": "EV_6", "channel_id": ch, "client_id": "s"}, nil},
		} {
			body, _ := json.Marshal(c.event)
			evs, err := tr.ParseEvents(t.Context(), header(mode, clock.Now(), body), body)
			if err != nil {
				t.Errorf("%s %s: %v", mode, name, err)
				continue
			}
			if !slices.Equal(evs, c.want) {
				t.Errorf("%s %s: ParseEvents = %+v, want %+v", mode, name, evs, c.want)
			}
		}
		for name, ev := range map[string]map[string]any{
			"no type":      {"id": "EV_7", "channel_id": ch},
			"no id":        {"type": "connection.created", "channel_id": ch, "client_id": "s"},
			"no client_id": {"type": "connection.created", "id": "EV_8", "channel_id": ch},
			"no channel":   {"type": "session.destroyed", "id": "EV_9"},
		} {
			body, _ := json.Marshal(ev)
			if _, err := tr.ParseEvents(t.Context(), header(mode, clock.Now(), body), body); !errors.Is(err, ErrWebhookBody) {
				t.Errorf("%s %s: ParseEvents = %v, want ErrWebhookBody", mode, name, err)
			}
		}
	}
	// A channel of another project of Sora Cloud is not a room.
	clock := testkit.VoiceTransportClock()
	cloud, _ := newTestTransport(t, ModeCloud, clock)
	body := []byte(`{"type":"connection.created","id":"EV_1","channel_id":"room-5@OTHER","client_id":"s"}`)
	if _, err := cloud.ParseEvents(t.Context(), signed(testAPIKey, clock.Now(), body), body); !errors.Is(err, ErrWebhookBody) {
		t.Errorf("channel of another project: %v, want ErrWebhookBody", err)
	}
}

func TestTruncate(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 3, "abc"},
		{"あいう", 4, "あ"},
		{"あいう", 6, "あい"},
	} {
		if got := truncate(c.in, c.n); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestNotFoundIsDone(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, _ := newTestTransport(t, ModeSelfHosted, clock)
	// A connection that is gone already counts as disconnected.
	if err := tr.disconnect(t.Context(), appvoice.RoomRef{ID: "room"}, "CONN-GONE"); err != nil {
		t.Errorf("disconnect of a connection that is gone: %v", err)
	}
	for name, c := range map[string]struct {
		err  error
		want bool
	}{
		"connection":    {&APIError{Status: 400, Body: `{"error_type":"CONNECTION-NOT-FOUND"}`}, true},
		"session":       {&APIError{Status: 400, Body: `{"error_type":"SESSION-NOT-FOUND"}`}, true},
		"other 400":     {&APIError{Status: 400, Body: `{"error_type":"INVALID-VERSION"}`}, false},
		"proxy 404":     {&APIError{Status: 404, Body: `<html>not found</html>`}, false},
		"unauthorized":  {&APIError{Status: 401, Body: `{"error_type":"SESSION-NOT-FOUND"}`}, false},
		"not API error": {errors.New("network"), false},
	} {
		if got := isNotFound(c.err); got != c.want {
			t.Errorf("%s: isNotFound = %t, want %t", name, got, c.want)
		}
	}
}
