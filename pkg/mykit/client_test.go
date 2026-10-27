// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestAPI returns a client of srv whose waits are recorded instead of slept.
func newTestAPI(t *testing.T, srv *httptest.Server, cfg Config) (*API, *[]time.Duration) {
	t.Helper()
	cfg.Server = srv.URL
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = srv.Client()
	}
	api, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	waits := &[]time.Duration{}
	api.transport.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		*waits = append(*waits, d)
		return ctx.Err()
	}
	return api, waits
}

func writeProblem(w http.ResponseWriter, status int, typ string, extra map[string]any) {
	body := map[string]any{"type": typ, "title": http.StatusText(status), "status": status}
	for k, v := range extra {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestNewRejectsAServerThatIsNoOrigin(t *testing.T) {
	for _, s := range []string{"", "example.com", "ftp://example.com", "https://example.com/api/v1", "https://example.com?x=1"} {
		if _, err := New(Config{Server: s}); err == nil {
			t.Errorf("New(%q) did not fail", s)
		}
	}
	if _, err := New(Config{Server: "https://example.com/"}); err != nil {
		t.Errorf("New with a trailing slash: %v", err)
	}
}

func TestHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/instance" {
			t.Errorf("path = %s", r.URL.Path)
		}
		got = r.Header.Clone()
		writeJSON(w, map[string]any{})
	}))
	defer srv.Close()
	api, _ := newTestAPI(t, srv, Config{TokenSource: StaticToken("at"), AcceptLanguage: "ja", UserAgent: "mybot/1.0"})
	if _, err := api.GetInstanceWithResponse(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := got.Get("Authorization"); v != "Bearer at" {
		t.Errorf("Authorization = %q", v)
	}
	if v := got.Get("Accept-Language"); v != "ja" {
		t.Errorf("Accept-Language = %q", v)
	}
	if v := got.Get("User-Agent"); v != "mybot/1.0 mykit-go" {
		t.Errorf("User-Agent = %q", v)
	}
	if v := got.Get("Idempotency-Key"); v != "" {
		t.Errorf("a GET got Idempotency-Key %q", v)
	}
}

func TestProblem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/instance":
			writeProblem(w, http.StatusConflict, "/api/v1/problems/blocking", map[string]any{"detail": "Unblock it first.", "reason": "blocked"})
		default:
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, "<html>bad gateway</html>")
		}
	}))
	defer srv.Close()
	api, _ := newTestAPI(t, srv, Config{})

	_, err := api.GetInstanceWithResponse(context.Background())
	var p *Problem
	if !errors.As(err, &p) {
		t.Fatalf("err = %v, want a *Problem", err)
	}
	if p.Status != http.StatusConflict || p.Type != "/api/v1/problems/blocking" || p.Detail == nil || *p.Detail != "Unblock it first." {
		t.Errorf("problem = %+v", p)
	}
	if reason, _ := p.AdditionalProperties["reason"].(string); reason != "blocked" {
		t.Errorf("reason = %v", p.AdditionalProperties["reason"])
	}
	if IsNetworkError(err) {
		t.Error("a problem is a network error")
	}

	_, err = api.VerifyAccountCredentialsWithResponse(context.Background())
	if !errors.As(err, &p) {
		t.Fatalf("err = %v, want a *Problem", err)
	}
	if p.Status != http.StatusBadGateway || p.Type != ProblemTypeAboutBlank || p.Title != "Bad Gateway" {
		t.Errorf("problem of a non-problem answer = %+v", p)
	}
}

func TestNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	api, _ := newTestAPI(t, srv, Config{})
	srv.Close()
	_, err := api.GetInstanceWithResponse(context.Background())
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want a *NetworkError", err)
	}
	var p *Problem
	if errors.As(err, &p) {
		t.Error("a network error is a problem")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv2 := httptest.NewServer(http.NotFoundHandler())
	defer srv2.Close()
	api2, _ := newTestAPI(t, srv2, Config{})
	if _, err := api2.GetInstanceWithResponse(ctx); !errors.Is(err, context.Canceled) || !IsNetworkError(err) {
		t.Errorf("err of a cancelled request = %v", err)
	}
}

// TestNetworkErrorCarriesTheIdempotencyKey checks that a post that got no answer reports the key it was sent
// with, so that the caller can send it again with the same key.
func TestNetworkErrorCarriesTheIdempotencyKey(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		// The connection ends without an answer.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()
	api, _ := newTestAPI(t, srv, Config{})
	_, err := api.CreateStatusWithResponse(context.Background(), nil, CreateStatusJSONRequestBody{})
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want a *NetworkError", err)
	}
	mu.Lock()
	if len(keys) != 1 || keys[0] == "" || ne.IdempotencyKey != keys[0] {
		t.Errorf("IdempotencyKey = %q, keys sent %q; want the key sent", ne.IdempotencyKey, keys)
	}
	mu.Unlock()
	if _, err := api.GetInstanceWithResponse(context.Background()); !errors.As(err, &ne) || ne.IdempotencyKey != "" {
		t.Errorf("err of a GET = %v, want a *NetworkError without a key", err)
	}
}

// tokenServer is a server whose token endpoint renews rt<n> into at<n+1> and rt<n+1>, and whose
// verify_credentials accepts only the access token that ok names.
type tokenServer struct {
	*httptest.Server
	refreshes atomic.Int32
	ok        atomic.Value // string
	// before401 is called before each 401, to hold it.
	before401 func()
}

func newTokenServer(t *testing.T) *tokenServer {
	ts := &tokenServer{}
	ts.ok.Store("at2")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("client_id") != "app" {
			t.Errorf("token request form = %v", r.PostForm)
		}
		n := ts.refreshes.Add(1)
		rt := r.PostForm.Get("refresh_token")
		if want := "rt" + strconv.Itoa(int(n)); rt != want {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]string{"error": "invalid_grant"})
			return
		}
		writeJSON(w, map[string]any{
			"access_token": "at" + strconv.Itoa(int(n)+1), "token_type": "Bearer", "expires_in": 3600,
			"refresh_token": "rt" + strconv.Itoa(int(n)+1), "scope": "read",
		})
	})
	mux.HandleFunc("GET /api/v1/accounts/verify_credentials", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+ts.ok.Load().(string) {
			writeJSON(w, map[string]any{})
			return
		}
		if ts.before401 != nil {
			ts.before401()
		}
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeProblem(w, http.StatusUnauthorized, "/api/v1/problems/invalid-token", nil)
	})
	ts.Server = httptest.NewServer(mux)
	return ts
}

func TestRefreshAfter401(t *testing.T) {
	ts := newTokenServer(t)
	defer ts.Close()
	var saved []*Token
	src := NewRefreshingTokenSource(OAuthConfig{Server: ts.URL, ClientID: "app", HTTPClient: ts.Client()},
		&Token{AccessToken: "at1", RefreshToken: "rt1"},
		func(_ context.Context, tok *Token) error {
			saved = append(saved, tok)
			return nil
		})
	api, _ := newTestAPI(t, ts.Server, Config{TokenSource: src})

	if _, err := api.VerifyAccountCredentialsWithResponse(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := ts.refreshes.Load(); n != 1 {
		t.Errorf("refreshes = %d, want 1", n)
	}
	if len(saved) != 1 || saved[0].AccessToken != "at2" || saved[0].RefreshToken != "rt2" || saved[0].Expiry.IsZero() {
		t.Errorf("onTokens got %+v, want the rotated tokens", saved)
	}

	// A second 401 after the refresh stands: the request is not refreshed again.
	ts.ok.Store("nothing")
	_, err := api.VerifyAccountCredentialsWithResponse(context.Background())
	var p *Problem
	if !errors.As(err, &p) || p.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want the 401 problem", err)
	}
	if n := ts.refreshes.Load(); n != 2 {
		t.Errorf("refreshes = %d, want 2 (one per request)", n)
	}
}

func TestStaticTokenIsNotRefreshed(t *testing.T) {
	ts := newTokenServer(t)
	defer ts.Close()
	api, _ := newTestAPI(t, ts.Server, Config{TokenSource: StaticToken("at1")})
	_, err := api.VerifyAccountCredentialsWithResponse(context.Background())
	var p *Problem
	if !errors.As(err, &p) || p.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want the 401 problem", err)
	}
	if n := ts.refreshes.Load(); n != 0 {
		t.Errorf("refreshes = %d, want 0", n)
	}
}

func TestConcurrent401sRefreshOnce(t *testing.T) {
	const n = 8
	ts := newTokenServer(t)
	defer ts.Close()
	// Hold every 401 until all n requests have met one, so that they all refresh at once.
	var arrived sync.WaitGroup
	arrived.Add(n)
	ts.before401 = func() {
		arrived.Done()
		arrived.Wait()
	}
	var saves atomic.Int32
	src := NewRefreshingTokenSource(OAuthConfig{Server: ts.URL, ClientID: "app", HTTPClient: ts.Client()},
		&Token{AccessToken: "at1", RefreshToken: "rt1"},
		func(context.Context, *Token) error {
			saves.Add(1)
			return nil
		})
	api, _ := newTestAPI(t, ts.Server, Config{TokenSource: src})

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			_, err := api.VerifyAccountCredentialsWithResponse(context.Background())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if got := ts.refreshes.Load(); got != 1 {
		t.Errorf("refreshes = %d, want 1: a reused refresh token revokes the family", got)
	}
	if got := saves.Load(); got != 1 {
		t.Errorf("onTokens calls = %d, want 1", got)
	}
}

func TestTokenRenewsAnExpiredToken(t *testing.T) {
	ts := newTokenServer(t)
	defer ts.Close()
	src := NewRefreshingTokenSource(OAuthConfig{Server: ts.URL, ClientID: "app", HTTPClient: ts.Client()},
		&Token{AccessToken: "at1", RefreshToken: "rt1", Expiry: time.Now().Add(-time.Minute)}, nil)
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at2" {
		t.Errorf("token = %q, want the renewed at2", tok.AccessToken)
	}
	if tok, _ := src.Token(context.Background()); tok.AccessToken != "at2" || ts.refreshes.Load() != 1 {
		t.Errorf("a valid token was renewed again: %q after %d refreshes", tok.AccessToken, ts.refreshes.Load())
	}
}

// TestTokenRetriesAFailedSave checks that the tokens of a renewal whose save failed are saved again before the
// source hands them out, and that the source does not renew them again meanwhile.
func TestTokenRetriesAFailedSave(t *testing.T) {
	ts := newTokenServer(t)
	defer ts.Close()
	saveErr := errors.New("disk full")
	failing := 2
	var saved []string
	src := NewRefreshingTokenSource(OAuthConfig{Server: ts.URL, ClientID: "app", HTTPClient: ts.Client()},
		&Token{AccessToken: "at1", RefreshToken: "rt1", Expiry: time.Now().Add(-time.Minute)},
		func(_ context.Context, tok *Token) error {
			if failing > 0 {
				failing--
				return saveErr
			}
			saved = append(saved, tok.RefreshToken)
			return nil
		})
	for i := range 2 {
		if tok, err := src.Token(context.Background()); !errors.Is(err, saveErr) {
			t.Fatalf("Token %d = %+v, %v; want the error of the save", i, tok, err)
		}
	}
	if _, err := src.Refresh(context.Background(), &Token{AccessToken: "at1"}); err != nil {
		t.Fatalf("Refresh after the saves failed: %v", err)
	}
	if n := ts.refreshes.Load(); n != 1 {
		t.Errorf("refreshes = %d, want 1: the renewed token is valid until it is saved", n)
	}
	if !slices.Equal(saved, []string{"rt2"}) {
		t.Errorf("saved = %v, want the rotated rt2 once", saved)
	}
	tok, err := src.Token(context.Background())
	if err != nil || tok.AccessToken != "at2" {
		t.Errorf("Token after the save = %+v, %v; want at2", tok, err)
	}
	if len(saved) != 1 {
		t.Errorf("saved = %v, want no save of tokens already saved", saved)
	}
}

func TestRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var keys sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := calls.Add(1)
		keys.Store(c, r.Header.Get("Idempotency-Key"))
		if c == 1 {
			w.Header().Set("Retry-After", "2")
			writeProblem(w, http.StatusTooManyRequests, ProblemTypeAboutBlank, nil)
			return
		}
		writeJSON(w, map[string]any{})
	}))
	defer srv.Close()

	t.Run("GET is retried after Retry-After", func(t *testing.T) {
		calls.Store(0)
		api, waits := newTestAPI(t, srv, Config{})
		if _, err := api.GetInstanceWithResponse(context.Background()); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != 2 || len(*waits) != 1 || (*waits)[0] != 2*time.Second {
			t.Errorf("calls = %d, waits = %v; want 2 calls after a 2s wait", calls.Load(), *waits)
		}
	})

	t.Run("POST without Idempotency-Key is not retried", func(t *testing.T) {
		calls.Store(0)
		api, waits := newTestAPI(t, srv, Config{})
		_, err := api.FavouriteStatusWithResponse(context.Background(), "01890a5d-ac96-774b-bcce-b302099a8057")
		var p *Problem
		if !errors.As(err, &p) || p.Status != http.StatusTooManyRequests {
			t.Fatalf("err = %v, want the 429 problem", err)
		}
		if calls.Load() != 1 || len(*waits) != 0 {
			t.Errorf("calls = %d, waits = %v; want no retry", calls.Load(), *waits)
		}
	})

	t.Run("POST with an Idempotency-Key is retried with the same key", func(t *testing.T) {
		calls.Store(0)
		api, waits := newTestAPI(t, srv, Config{})
		if _, err := api.CreateStatusWithResponse(context.Background(), nil, CreateStatusJSONRequestBody{}); err != nil {
			t.Fatal(err)
		}
		k1, _ := keys.Load(int32(1))
		k2, _ := keys.Load(int32(2))
		if calls.Load() != 2 || len(*waits) != 1 || k1 == "" || k1 != k2 {
			t.Errorf("calls = %d, waits = %v, keys %q %q; want one retry with the generated key", calls.Load(), *waits, k1, k2)
		}
	})

	t.Run("the caller's Idempotency-Key is kept", func(t *testing.T) {
		calls.Store(0)
		api, _ := newTestAPI(t, srv, Config{})
		key := "my-key"
		if _, err := api.CreateStatusWithResponse(context.Background(), &CreateStatusParams{IdempotencyKey: &key}, CreateStatusJSONRequestBody{}); err != nil {
			t.Fatal(err)
		}
		if k, _ := keys.Load(int32(2)); k != key {
			t.Errorf("key = %q, want %q", k, key)
		}
	})

	t.Run("an Idempotency-Key on an operation that takes none does not make it retried", func(t *testing.T) {
		calls.Store(0)
		api, waits := newTestAPI(t, srv, Config{})
		withKey := func(_ context.Context, req *http.Request) error {
			req.Header.Set("Idempotency-Key", "my-key")
			return nil
		}
		_, err := api.FavouriteStatusWithResponse(context.Background(), "01890a5d-ac96-774b-bcce-b302099a8057", withKey)
		var p *Problem
		if !errors.As(err, &p) || p.Status != http.StatusTooManyRequests {
			t.Fatalf("err = %v, want the 429 problem", err)
		}
		if calls.Load() != 1 || len(*waits) != 0 {
			t.Errorf("calls = %d, waits = %v; want no retry", calls.Load(), *waits)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		calls.Store(0)
		api, waits := newTestAPI(t, srv, Config{Retry: RetryPolicy{Disabled: true}})
		if _, err := api.GetInstanceWithResponse(context.Background()); err == nil {
			t.Fatal("no error")
		}
		if calls.Load() != 1 || len(*waits) != 0 {
			t.Errorf("calls = %d, waits = %v; want no retry", calls.Load(), *waits)
		}
	})

	t.Run("Retry-After beyond MaxWait", func(t *testing.T) {
		calls.Store(0)
		api, waits := newTestAPI(t, srv, Config{Retry: RetryPolicy{MaxWait: time.Second}})
		if _, err := api.GetInstanceWithResponse(context.Background()); err == nil {
			t.Fatal("no error")
		}
		if calls.Load() != 1 || len(*waits) != 0 {
			t.Errorf("calls = %d, waits = %v; want no retry", calls.Load(), *waits)
		}
	})
}

func TestRetryGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeProblem(w, http.StatusServiceUnavailable, "/api/v1/problems/maintenance", nil)
	}))
	defer srv.Close()
	api, waits := newTestAPI(t, srv, Config{Retry: RetryPolicy{MaxRetries: 2}})
	_, err := api.GetInstanceWithResponse(context.Background())
	var p *Problem
	if !errors.As(err, &p) || p.Type != "/api/v1/problems/maintenance" {
		t.Fatalf("err = %v, want the maintenance problem", err)
	}
	if calls.Load() != 3 || len(*waits) != 2 {
		t.Errorf("calls = %d, waits = %v; want 3 calls", calls.Load(), *waits)
	}
	// Without Retry-After the waits back off from a second.
	if (*waits)[0] < time.Second || (*waits)[1] < 2*time.Second {
		t.Errorf("waits = %v, want a backoff from 1s", *waits)
	}
}

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header string
		want   time.Duration
		ok     bool
	}{
		{"3", 3 * time.Second, true},
		{"0", 0, true},
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second, true},
		{"120", 120 * time.Second, false},
		// Seconds beyond what a time.Duration holds wait for the longest one, never a wrapped short one.
		{"9223372037", math.MaxInt64, false},
		{"18446744073", math.MaxInt64, false},
		{"9223372036854775807", math.MaxInt64, false},
		{"99999999999999999999", math.MaxInt64, false},
	} {
		got, ok := retryDelay(tc.header, 0, time.Minute, now)
		if got != tc.want || ok != tc.ok {
			t.Errorf("retryDelay(%q) = %v, %v; want %v, %v", tc.header, got, ok, tc.want, tc.ok)
		}
	}
}
