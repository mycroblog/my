// SPDX-License-Identifier: AGPL-3.0-only

package kerneltest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// HumanVerifierStub is the stub siteverify service that HumanVerifierContract gives the
// verifier under test.
type HumanVerifierStub struct {
	// VerifyURL is the stub's siteverify endpoint, to use instead of the service's.
	VerifyURL string
	// Addr is the stub's address on loopback. The verifier's HTTPClient treats exactly this
	// address and port as public and allows plain http on it.
	Addr netip.AddrPort
	// Host is the instance's host; the stub names it as the host where a valid token was solved.
	Host string
	// SiteKey and SecretKey are the keys to configure. The stub refuses other secret keys
	// with invalid-input-secret.
	SiteKey   string
	SecretKey string
	// Logger is where the verifier logs; the contract checks that no secret key reaches it.
	Logger *slog.Logger
}

// HumanVerifierService describes the service whose API the verifier under test speaks.
type HumanVerifierService struct {
	// ReportsAction is set when the service binds a token to the widget's action and reports
	// it back (Turnstile); the verifier must then refuse a token of another action.
	ReportsAction bool
	// SendsSiteKey is set when the verifier sends the site key with the token (hCaptcha).
	SendsSiteKey bool
}

// Stub tokens. A token the stub does not know is invalid-input-response.
const (
	hvTokenPass        = "token-pass"
	hvTokenExpired     = "token-expired"
	hvTokenUsed        = "token-used"
	hvTokenOtherHost   = "token-other-host"
	hvTokenUnavailable = "token-503"
	hvTokenInternal    = "token-internal-error"
	hvTokenGarbage     = "token-garbage"
	hvTokenSlow        = "token-slow"
	hvTokenHuge        = "token-huge"
)

// HumanVerifierContract verifies a kernel.HumanVerifier that speaks the siteverify API of
// ADR-0106 against an httptest stub of the service, without the network. It checks that a
// valid token passes; that an empty, invalid, expired, or used token, a token of another host
// name, and (when the service reports it) a token of another action fail with
// kernel.ErrHumanCheckFailed; that a 5xx answer, the service's internal error, an unreadable
// or oversized answer, a timeout, and a refused secret key are kernel.ErrHumanCheckUnavailable,
// the last with an ERROR log; that the request is one form POST with the secret and the token,
// the site key when the service wants it, and no remoteip, sent once without retries; and that
// the secret key never reaches the log.
func HumanVerifierContract(t *testing.T, svc HumanVerifierService, newVerifier func(t *testing.T, s HumanVerifierStub) kernel.HumanVerifier) {
	t.Helper()
	t.Run("PassesAValidToken", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "")
		if err := e.verifier.Verify(t.Context(), hvTokenPass+"."+kernel.HumanCheckActionSignIn, kernel.HumanCheckActionSignIn); err != nil {
			t.Fatalf("Verify(valid) = %v", err)
		}
		reqs := e.requests()
		if len(reqs) != 1 {
			t.Fatalf("the stub got %d requests, want 1", len(reqs))
		}
		r := reqs[0]
		if r.method != http.MethodPost || !strings.HasPrefix(r.contentType, "application/x-www-form-urlencoded") {
			t.Errorf("request = %s %q, want a form POST", r.method, r.contentType)
		}
		if got := r.form.Get("secret"); got != e.stub.SecretKey {
			t.Errorf("secret = %q, want the configured secret key", got)
		}
		if got := r.form.Get("response"); got != hvTokenPass+"."+kernel.HumanCheckActionSignIn {
			t.Errorf("response = %q, want the token", got)
		}
		if r.form.Has("remoteip") {
			t.Error("the request sends remoteip; the browser already reached the service itself (ADR-0106 §9)")
		}
		if r.form.Has("idempotency_key") {
			t.Error("the request sends idempotency_key; a token is never retried (ADR-0106 §9)")
		}
		if svc.SendsSiteKey && r.form.Get("sitekey") != e.stub.SiteKey {
			t.Errorf("sitekey = %q, want the configured site key", r.form.Get("sitekey"))
		}
		e.checkLogs(t)
	})
	t.Run("FailsBadTokens", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "")
		for _, token := range []string{"token-unknown", hvTokenExpired, hvTokenUsed, hvTokenOtherHost + "." + kernel.HumanCheckActionSignIn} {
			if err := e.verifier.Verify(t.Context(), token, kernel.HumanCheckActionSignIn); !errors.Is(err, kernel.ErrHumanCheckFailed) {
				t.Errorf("Verify(%s) = %v, want ErrHumanCheckFailed", token, err)
			}
		}
		e.checkLogs(t)
	})
	t.Run("FailsAnEmptyTokenWithoutARequest", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "")
		if err := e.verifier.Verify(t.Context(), "", kernel.HumanCheckActionSignUp); !errors.Is(err, kernel.ErrHumanCheckFailed) {
			t.Errorf("Verify(empty) = %v, want ErrHumanCheckFailed", err)
		}
		if n := len(e.requests()); n != 0 {
			t.Errorf("the stub got %d requests for an empty token", n)
		}
	})
	t.Run("ChecksTheAction", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "")
		err := e.verifier.Verify(t.Context(), hvTokenPass+"."+kernel.HumanCheckActionSignIn, kernel.HumanCheckActionSignUp)
		if svc.ReportsAction && !errors.Is(err, kernel.ErrHumanCheckFailed) {
			t.Errorf("Verify(sign_in token for sign_up) = %v, want ErrHumanCheckFailed", err)
		}
		if !svc.ReportsAction && err != nil {
			t.Errorf("Verify() = %v; the service does not report the action, so it cannot be checked", err)
		}
	})
	t.Run("ReportsAnUnavailableService", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "")
		for _, token := range []string{hvTokenUnavailable, hvTokenInternal, hvTokenGarbage, hvTokenHuge} {
			if err := e.verifier.Verify(t.Context(), token, kernel.HumanCheckActionSignIn); !errors.Is(err, kernel.ErrHumanCheckUnavailable) {
				t.Errorf("Verify(%s) = %v, want ErrHumanCheckUnavailable", token, err)
			}
		}
		// One request per token: a token is valid once, so nothing is retried.
		if n := len(e.requests()); n != 4 {
			t.Errorf("the stub got %d requests for 4 tokens, want 4 (no retries)", n)
		}
		e.checkLogs(t)
	})
	t.Run("TimesOut", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "")
		ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := e.verifier.Verify(ctx, hvTokenSlow, kernel.HumanCheckActionSignIn); !errors.Is(err, kernel.ErrHumanCheckUnavailable) {
			t.Errorf("Verify(slow) = %v, want ErrHumanCheckUnavailable", err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("Verify(slow) took %s after the context's deadline", d)
		}
	})
	t.Run("ReportsARefusedSecretKey", func(t *testing.T) {
		e := newHumanVerifierEnv(t, svc, newVerifier, "wrong-secret-key-0x5eCrE7")
		err := e.verifier.Verify(t.Context(), hvTokenPass+"."+kernel.HumanCheckActionSignIn, kernel.HumanCheckActionSignIn)
		if !errors.Is(err, kernel.ErrHumanCheckUnavailable) {
			t.Errorf("Verify(wrong secret) = %v, want ErrHumanCheckUnavailable", err)
		}
		if err != nil && strings.Contains(err.Error(), e.stub.SecretKey) {
			t.Errorf("the error carries the secret key: %v", err)
		}
		if !strings.Contains(e.logs.String(), `"level":"ERROR"`) {
			t.Errorf("a refused secret key logged no ERROR record; logs: %s", e.logs.String())
		}
		e.checkLogs(t)
	})
}

type humanVerifierRequest struct {
	method      string
	contentType string
	form        url.Values
}

type humanVerifierEnv struct {
	verifier kernel.HumanVerifier
	stub     HumanVerifierStub
	logs     *syncBuffer

	mu   sync.Mutex
	reqs []humanVerifierRequest
}

// newHumanVerifierEnv starts a stub; a non-empty secret configures the verifier with that
// secret key instead of the one the stub accepts.
func newHumanVerifierEnv(t *testing.T, svc HumanVerifierService, newVerifier func(*testing.T, HumanVerifierStub) kernel.HumanVerifier, secret string) *humanVerifierEnv {
	t.Helper()
	const host = "social.example"
	const accepted = "stub-secret-key-0x5eCrE7"
	e := &humanVerifierEnv{logs: &syncBuffer{}}
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.mu.Lock()
		e.reqs = append(e.reqs, humanVerifierRequest{method: r.Method, contentType: r.Header.Get("Content-Type"), form: r.PostForm})
		e.mu.Unlock()
		writeJSON := func(v map[string]any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(v)
		}
		if r.PostForm.Get("secret") != accepted {
			writeJSON(map[string]any{"success": false, "error-codes": []string{"invalid-input-secret"}})
			return
		}
		token := r.PostForm.Get("response")
		name, action, _ := strings.Cut(token, ".")
		passed := func(hostname string) {
			v := map[string]any{"success": true, "challenge_ts": "2026-10-19T00:00:00Z", "hostname": hostname, "error-codes": []string{}}
			if svc.ReportsAction {
				v["action"] = action
			}
			writeJSON(v)
		}
		switch name {
		case hvTokenPass:
			passed(host)
		case hvTokenOtherHost:
			passed("evil.example")
		case hvTokenExpired:
			writeJSON(map[string]any{"success": false, "error-codes": []string{"timeout-or-duplicate"}})
		case hvTokenUsed:
			writeJSON(map[string]any{"success": false, "error-codes": []string{"invalid-or-already-seen-response"}})
		case hvTokenUnavailable:
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		case hvTokenInternal:
			writeJSON(map[string]any{"success": false, "error-codes": []string{"internal-error"}})
		case hvTokenGarbage:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>maintenance</html>"))
		case hvTokenHuge:
			limits, _ := kernel.DefaultHTTPLimits(kernel.HTTPPurposeHumanCheck)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"hostname":"` + host + `","action":"` + kernel.HumanCheckActionSignIn + `","pad":"`))
			_, _ = w.Write(bytes.Repeat([]byte("a"), int(limits.MaxBodyBytes)))
			_, _ = w.Write([]byte(`"}`))
		case hvTokenSlow:
			select {
			case <-r.Context().Done():
			case <-release:
			}
		default:
			writeJSON(map[string]any{"success": false, "error-codes": []string{"invalid-input-response"}})
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs before srv.Close, which waits for the handlers
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := netip.ParseAddrPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		secret = accepted
	}
	e.stub = HumanVerifierStub{
		VerifyURL: srv.URL + "/siteverify",
		Addr:      addr,
		Host:      host,
		SiteKey:   "stub-site-key",
		SecretKey: secret,
		Logger:    slog.New(slog.NewJSONHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	e.verifier = newVerifier(t, e.stub)
	return e
}

func (e *humanVerifierEnv) requests() []humanVerifierRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]humanVerifierRequest(nil), e.reqs...)
}

// checkLogs fails when the secret key reached the log (ADR-0019 §6).
func (e *humanVerifierEnv) checkLogs(t *testing.T) {
	t.Helper()
	if strings.Contains(e.logs.String(), e.stub.SecretKey) {
		t.Errorf("the secret key reached the log: %s", e.logs.String())
	}
}

// syncBuffer is a bytes.Buffer safe for concurrent writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
