// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"errors"
	"sync"
	"time"
)

// expirySkew is how long before its expiry a token counts as expired, so that it does not expire on the way.
const expirySkew = 30 * time.Second

// Token is an OAuth 2.0 access token and the refresh token that renews it.
type Token struct {
	AccessToken string
	// RefreshToken renews the access token. The server rotates it on every use (RFC 9700 §4.14): the
	// previous one stops working, so the new one must be saved.
	RefreshToken string
	// Expiry is when the access token expires; zero means unknown.
	Expiry time.Time
	// Scope is the space-separated scopes of the token.
	Scope string
}

// valid reports whether the access token is set and not about to expire at now.
func (t *Token) valid(now time.Time) bool {
	return t != nil && t.AccessToken != "" && (t.Expiry.IsZero() || now.Add(expirySkew).Before(t.Expiry))
}

// TokenSource gives the access token of a request. The library never stores tokens itself.
type TokenSource interface {
	Token(ctx context.Context) (*Token, error)
}

// Refresher is a TokenSource that can renew its token when the server refuses it with a 401. The client calls
// Refresh once per request, with the token the server refused, and sends the request again with the new one.
type Refresher interface {
	TokenSource
	Refresh(ctx context.Context, refused *Token) (*Token, error)
}

// StaticToken is a TokenSource of one access token that is never renewed, for tests and short tasks. Use a
// RefreshingTokenSource for anything that runs longer than the token lives (an hour).
func StaticToken(accessToken string) TokenSource {
	return staticToken{&Token{AccessToken: accessToken}}
}

type staticToken struct{ tok *Token }

func (s staticToken) Token(context.Context) (*Token, error) { return s.tok, nil }

// RefreshingTokenSource is the TokenSource of a client that holds a refresh token: it renews the access token
// with the refresh_token grant when it expires or the server refuses it. Renewals are serialized, so that
// concurrent requests that meet a 401 renew it once: a used refresh token sent again revokes the whole token
// family. Every renewal passes the new tokens to OnTokens, which must save the rotated refresh token.
type RefreshingTokenSource struct {
	oauth    OAuthConfig
	onTokens func(context.Context, *Token) error
	now      func() time.Time

	mu  sync.Mutex
	tok *Token
}

// NewRefreshingTokenSource returns the source that starts from tok (from Exchange, or saved before) and renews
// it through the token endpoint of cfg. onTokens receives the tokens of every renewal; when it fails, the new
// tokens are kept in the source and the request that needed them fails with its error.
func NewRefreshingTokenSource(cfg OAuthConfig, tok *Token, onTokens func(context.Context, *Token) error) *RefreshingTokenSource {
	return &RefreshingTokenSource{oauth: cfg, onTokens: onTokens, now: time.Now, tok: tok}
}

// Token returns the current access token, renewed first when it has expired.
func (s *RefreshingTokenSource) Token(ctx context.Context) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tok.valid(s.now()) {
		return s.tok, nil
	}
	return s.renew(ctx)
}

// Refresh renews the token after the server refused it. When another request renewed it since refused was
// handed out, it returns that token instead of renewing again.
func (s *RefreshingTokenSource) Refresh(ctx context.Context, refused *Token) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tok != nil && refused != nil && s.tok.AccessToken != refused.AccessToken && s.tok.valid(s.now()) {
		return s.tok, nil
	}
	return s.renew(ctx)
}

// renew runs the refresh_token grant; s.mu is held.
func (s *RefreshingTokenSource) renew(ctx context.Context) (*Token, error) {
	if s.tok == nil || s.tok.RefreshToken == "" {
		return nil, errors.New("mykit: the token has expired and there is no refresh token")
	}
	tok, err := s.oauth.token(ctx, map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": s.tok.RefreshToken,
	})
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = s.tok.RefreshToken
	}
	s.tok = tok
	if s.onTokens != nil {
		if err := s.onTokens(ctx, tok); err != nil {
			return nil, err
		}
	}
	return tok, nil
}
