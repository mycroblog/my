// SPDX-License-Identifier: AGPL-3.0-only

package oauth_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/interface/oauth"
	"github.com/mycroblog/my/pkg/testkit"
)

func newLimitedServer(t *testing.T, id *fakeIdentity) http.Handler {
	t.Helper()
	clock := testkit.NewClock(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC))
	h, err := oauth.New(oauth.Options{
		Identity: id, WebOrigin: "https://web.example",
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock),
	})
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	h.Register(m)
	return m
}

func registerFrom(h http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "https://social.example/oauth/register", strings.NewReader(`{"redirect_uris":["https://app.example/cb"]}`))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func wantLimited(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	wantError(t, w, http.StatusTooManyRequests, "temporarily_unavailable", what)
	if w.Header().Get("Retry-After") == "" {
		t.Errorf("%s: no Retry-After", what)
	}
}

func TestRegisterIsLimitedPerIPAndAggregatesIPv6By64(t *testing.T) {
	h := newLimitedServer(t, &fakeIdentity{})
	for i := range 5 {
		// Every address of the /64 shares one limit.
		if w := registerFrom(h, fmt.Sprintf("[2001:db8:1:2::%x]:1234", i+1)); w.Code != http.StatusCreated {
			t.Fatalf("registration %d = %d %s", i, w.Code, w.Body.String())
		}
	}
	wantLimited(t, registerFrom(h, "[2001:db8:1:2:ffff::9]:1234"), "the 6th registration from the /64")
	if w := registerFrom(h, "[2001:db8:1:3::1]:1234"); w.Code != http.StatusCreated {
		t.Errorf("another /64 = %d, want 201", w.Code)
	}
	for i := range 5 {
		if w := registerFrom(h, "192.0.2.7:1"); w.Code != http.StatusCreated {
			t.Fatalf("IPv4 registration %d = %d", i, w.Code)
		}
	}
	wantLimited(t, registerFrom(h, "192.0.2.7:2"), "the 6th registration from one IPv4 address")
}

func TestRegisterIsLimitedForTheWholeInstance(t *testing.T) {
	h := newLimitedServer(t, &fakeIdentity{})
	for i := range oauth.RegisterLimit.Max {
		if w := registerFrom(h, fmt.Sprintf("198.51.%d.%d:1", i/200, i%200+1)); w.Code != http.StatusCreated {
			t.Fatalf("registration %d = %d %s", i, w.Code, w.Body.String())
		}
	}
	wantLimited(t, registerFrom(h, "203.0.113.1:1"), "the registration over the instance limit")
}

func TestTokenIsLimitedPerClientAndIP(t *testing.T) {
	h := newLimitedServer(t, &fakeIdentity{})
	req := func(client string) *httptest.ResponseRecorder {
		return post(h, "/oauth/token", url.Values{"client_id": {client}}, nil)
	}
	for range oauth.TokenLimit.Max {
		if w := req("app-a"); w.Code == http.StatusTooManyRequests {
			t.Fatal("limited before the limit")
		}
	}
	wantLimited(t, req("app-a"), "the request over the token limit")
	if w := req("app-b"); w.Code == http.StatusTooManyRequests {
		t.Error("another client shares the limit")
	}
}
