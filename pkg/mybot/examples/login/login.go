// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// DefaultScope are the scopes that a bot of mybot needs, and write:accounts, with which it can set its bot flag
// (Config.SetBotFlag).
const DefaultScope = "read:accounts read:notifications read:statuses write:statuses write:accounts"

// authorizePath is the path of the server's authorization endpoint under its origin (ADR-0090).
const authorizePath = "/oauth/authorize"

// login runs the Authorization Code flow with PKCE for cfg's app: it waits for the redirect on a port of
// 127.0.0.1, hands open the URL of the authorization page, and exchanges the code that the redirect brings for
// the tokens. cfg.RedirectURI is set to the loopback redirect.
func login(ctx context.Context, cfg mykit.OAuthConfig, scope string, open func(authorizeURL string) error) (*mykit.Token, error) {
	server, err := url.Parse(cfg.Server)
	if err != nil || server.Host == "" {
		return nil, fmt.Errorf("the server %q is no URL such as https://example.com", cfg.Server)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	cfg.RedirectURI = "http://" + ln.Addr().String() + "/callback"

	verifier := mykit.NewCodeVerifier()
	state := randomString()
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {cfg.RedirectURI},
		"scope":                 {scope},
		"state":                 {state},
		"code_challenge":        {mykit.CodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	authorizeURL := server.Scheme + "://" + server.Host + authorizePath + "?" + q.Encode()

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		var res result
		switch {
		case subtle.ConstantTimeCompare([]byte(v.Get("state")), []byte(state)) != 1:
			// Not the answer to this request: keep waiting for it.
			http.Error(w, "The state does not match the request.", http.StatusBadRequest)
			return
		case v.Get("error") != "":
			res.err = fmt.Errorf("the authorization failed: %s %s", v.Get("error"), v.Get("error_description"))
		case v.Get("code") == "":
			res.err = errors.New("the redirect brought no code")
		default:
			res.code = v.Get("code")
		}
		select {
		case results <- res:
		default:
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if res.err != nil {
			_, _ = fmt.Fprintln(w, "The authorization failed. You can close this window.")
			return
		}
		_, _ = fmt.Fprintln(w, "The bot is authorized. You can close this window.")
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	if err := open(authorizeURL); err != nil {
		return nil, err
	}
	var res result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-results:
	}
	if res.err != nil {
		return nil, res.err
	}
	tok, err := cfg.Exchange(ctx, res.code, verifier)
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("the server returned no refresh token")
	}
	return tok, nil
}

// randomString returns 16 random bytes, base64url-encoded, for the state of the request.
func randomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return base64.RawURLEncoding.EncodeToString(b)
}
