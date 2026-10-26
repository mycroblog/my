// SPDX-License-Identifier: AGPL-3.0-only

package turncloudflare_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	"github.com/mycroblog/my/internal/adapter/voice/turncloudflare"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeAPI is Cloudflare's TURN API as its documentation describes it.
type fakeAPI struct {
	mu       sync.Mutex
	issued   int
	ttls     []int64
	revoked  []string
	status   int // the status of every response when not 0
	username string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer token-1" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if f.status != 0 {
		http.Error(w, "failed", f.status)
		return
	}
	switch {
	case r.URL.Path == "/v1/turn/keys/key-1/credentials/generate-ice-servers":
		var body struct {
			TTL int64 `json:"ttl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		f.issued++
		f.ttls = append(f.ttls, body.TTL)
		f.username = "user-" + strconv.Itoa(f.issued)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"iceServers": []map[string]any{
			{"urls": []string{"stun:stun.cloudflare.com:3478"}},
			{"urls": []string{"turn:turn.cloudflare.com:3478?transport=udp", "turns:turn.cloudflare.com:5349?transport=tcp"},
				"username": f.username, "credential": "secret-" + f.username},
		}})
	case len(r.URL.Path) > len("/revoke") && r.URL.Path[len(r.URL.Path)-len("/revoke"):] == "/revoke":
		name := r.URL.Path[len("/v1/turn/keys/key-1/credentials/") : len(r.URL.Path)-len("/revoke")]
		f.revoked = append(f.revoked, name)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

type harness struct {
	api    *fakeAPI
	clock  *testkit.Clock
	source *turncloudflare.Source
}

func newHarness(t *testing.T) harness {
	t.Helper()
	api := &fakeAPI{}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	clock := testkit.VoiceTransportClock()
	s, err := turncloudflare.New(turncloudflare.Options{
		KeyID: "key-1", APIToken: "token-1", KV: testkit.NewKeyValueStore(clock), Clock: clock,
		HTTPClient: srv.Client(), BaseURL: srv.URL + "/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return harness{api: api, clock: clock, source: s}
}

var session = appvoice.Session{ID: "session-1", AccountID: "account-1"}

func TestIssueAndCache(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	c, err := h.source.Credentials(ctx, session, 6*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if c.Reused || !c.ExpiresAt.Equal(h.clock.Now().Add(6*time.Hour)) || len(c.Servers) != 2 {
		t.Fatalf("Credentials = %+v, want two new servers that expire in 6h", c)
	}
	if turn := c.Servers[1]; turn.Username != "user-1" || turn.Credential != "secret-user-1" || len(turn.URLs) != 2 {
		t.Errorf("TURN server = %+v", turn)
	}
	if !slices.Equal(h.api.ttls, []int64{6 * 3600}) {
		t.Errorf("ttl sent = %v, want [21600]", h.api.ttls)
	}

	h.clock.Advance(time.Hour)
	again, err := h.source.Credentials(ctx, session, 6*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Reused || !again.ExpiresAt.Equal(c.ExpiresAt) || again.Servers[1].Username != "user-1" || h.api.issued != 1 {
		t.Errorf("second Credentials = %+v after %d calls, want the cached ones", again, h.api.issued)
	}

	// Close to the expiry, a renewal gets new credentials.
	h.clock.Advance(4*time.Hour + 50*time.Minute)
	renewed, err := h.source.Credentials(ctx, session, 6*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Reused || renewed.Servers[1].Username != "user-2" || h.api.issued != 2 {
		t.Errorf("renewed Credentials = %+v, want new ones", renewed)
	}

	// Another session never gets these.
	other, err := h.source.Credentials(ctx, appvoice.Session{ID: "session-2"}, time.Hour)
	if err != nil || other.Reused || other.Servers[1].Username != "user-3" {
		t.Errorf("Credentials of another session = %+v, %v", other, err)
	}
}

func TestTTLIsCapped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c, err := h.source.Credentials(t.Context(), session, 72*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.api.ttls, []int64{48 * 3600}) || !c.ExpiresAt.Equal(h.clock.Now().Add(turncloudflare.MaxTTL)) {
		t.Errorf("ttl sent = %v, ExpiresAt = %s, want 48h", h.api.ttls, c.ExpiresAt)
	}
}

func TestRevoke(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	if err := h.source.Revoke(ctx, session); err != nil || len(h.api.revoked) != 0 {
		t.Fatalf("Revoke without credentials = %v, revoked %v; want nothing", err, h.api.revoked)
	}
	if _, err := h.source.Credentials(ctx, session, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := h.source.Revoke(ctx, session); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.api.revoked, []string{"user-1"}) {
		t.Errorf("revoked = %v, want [user-1]", h.api.revoked)
	}
	// Revoking again is idempotent, and the session gets new credentials after it.
	if err := h.source.Revoke(ctx, session); err != nil || len(h.api.revoked) != 1 {
		t.Errorf("second Revoke = %v, revoked %v", err, h.api.revoked)
	}
	c, err := h.source.Credentials(ctx, session, time.Hour)
	if err != nil || c.Reused || c.Servers[1].Username != "user-2" {
		t.Errorf("Credentials after Revoke = %+v, %v; want new ones", c, err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	h.api.status = http.StatusInternalServerError
	if _, err := h.source.Credentials(ctx, session, time.Hour); err == nil {
		t.Fatal("Credentials with the API failing = nil error")
	}
	// The p2p transport turns the failure into a failed grant (503) and counts it.
	meter := testkit.NewMeter()
	tr := p2p.New(p2p.Options{TURN: h.source, Clock: h.clock, Meter: meter})
	if _, err := tr.Grant(ctx, appvoice.RoomRef{ID: "r"}, session, true); err == nil {
		t.Error("Grant with the API failing = nil error")
	}
	if n := meter.Sum(p2p.TURNCredentials.Name, map[string]string{"my.voice.turn": "cloudflare", "my.outcome": "failed"}); n != 1 {
		t.Errorf("failed credentials = %v, want 1", n)
	}

	h.api.status = 0
	if _, err := h.source.Credentials(ctx, session, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.api.status = http.StatusBadGateway
	if err := h.source.Revoke(ctx, session); err == nil {
		t.Error("Revoke with the API failing = nil error, want one for voice.apply to retry")
	}
	// A revoke that failed keeps the credentials to revoke on the retry.
	h.api.status = 0
	if err := h.source.Revoke(ctx, session); err != nil || !slices.Equal(h.api.revoked, []string{"user-1"}) {
		t.Errorf("retried Revoke = %v, revoked %v", err, h.api.revoked)
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
}

func TestNewNeedsTheKey(t *testing.T) {
	t.Parallel()
	clock := testkit.VoiceTransportClock()
	if _, err := turncloudflare.New(turncloudflare.Options{APIToken: "t", KV: testkit.NewKeyValueStore(clock), Clock: clock}); err == nil {
		t.Error("New without a key ID = nil error")
	}
}
