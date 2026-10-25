// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// authServer is a fake token endpoint that issues tokens for the code "the-code" when the code_verifier matches
// the code_challenge that the authorization request sent.
type authServer struct {
	*httptest.Server
	mu          sync.Mutex
	challenge   string
	redirectURI string
}

func newAuthServer(t *testing.T) *authServer {
	t.Helper()
	a := &authServer{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != mykit.TokenPath {
			http.NotFound(w, r)
			return
		}
		id, secret, _ := r.BasicAuth()
		a.mu.Lock()
		challenge, redirectURI := a.challenge, a.redirectURI
		a.mu.Unlock()
		if id != "bot-app" || secret != "s3cret" || r.FormValue("grant_type") != "authorization_code" ||
			r.FormValue("code") != "the-code" || r.FormValue("redirect_uri") != redirectURI ||
			mykit.CodeChallenge(r.FormValue("code_verifier")) != challenge {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access", "token_type": "Bearer", "expires_in": 3600,
			"refresh_token": "refresh", "scope": r.FormValue("scope") + DefaultScope,
		})
	}))
	t.Cleanup(a.Close)
	return a
}

// approve plays the browser and the consent page: it checks the authorization URL and follows the redirect back
// with the query that the server would add (params, and the state of the request unless state is given).
func (a *authServer) approve(t *testing.T, params url.Values, state *string) func(string) error {
	return func(authorizeURL string) error {
		u, err := url.Parse(authorizeURL)
		if err != nil {
			return err
		}
		q := u.Query()
		if u.Path != authorizePath || q.Get("response_type") != "code" || q.Get("client_id") != "bot-app" ||
			q.Get("scope") != DefaultScope || q.Get("code_challenge_method") != "S256" {
			t.Errorf("authorization URL = %s", authorizeURL)
		}
		redirect := q.Get("redirect_uri")
		if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/callback") {
			t.Errorf("redirect_uri = %q, want a loopback URL", redirect)
		}
		a.mu.Lock()
		a.challenge, a.redirectURI = q.Get("code_challenge"), redirect
		a.mu.Unlock()
		back := url.Values{}
		for k, v := range params {
			back[k] = v
		}
		back.Set("state", q.Get("state"))
		if state != nil {
			back.Set("state", *state)
		}
		go func() {
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(redirect + "?" + back.Encode())
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func (a *authServer) config() mykit.OAuthConfig {
	return mykit.OAuthConfig{Server: a.URL, ClientID: "bot-app", ClientSecret: "s3cret"}
}

func TestLogin(t *testing.T) {
	a := newAuthServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tok, err := login(ctx, a.config(), DefaultScope, a.approve(t, url.Values{"code": {"the-code"}}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" {
		t.Errorf("token = %+v", tok)
	}
}

func TestLoginDenied(t *testing.T) {
	a := newAuthServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, err := login(ctx, a.config(), DefaultScope, a.approve(t, url.Values{"error": {"access_denied"}}, nil))
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("err = %v, want access_denied", err)
	}
}

// A redirect with another state is not the answer to the request: login keeps waiting.
func TestLoginWrongState(t *testing.T) {
	a := newAuthServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	other := "forged"
	_, err := login(ctx, a.config(), DefaultScope, a.approve(t, url.Values{"code": {"the-code"}}, &other))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
}
