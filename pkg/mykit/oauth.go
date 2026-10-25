// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenPath is the path of the server's token endpoint under its origin.
const TokenPath = "/oauth/token" //nolint:gosec // G101: a path, not a credential

// OAuthConfig is the client's registration on a server's authorization server (ADR-0090).
type OAuthConfig struct {
	// Server is the origin of the server, such as "https://example.com".
	Server string
	// ClientID is the client_id of the registration, or the URL of the client's metadata document.
	ClientID string
	// ClientSecret is the secret of a confidential client; empty for a public client.
	ClientSecret string
	// RedirectURI is the redirect_uri that the authorization request sent.
	RedirectURI string
	// HTTPClient sends the requests to the token endpoint. Nil means a new http.Client; the library never uses
	// http.DefaultClient.
	HTTPClient *http.Client
}

// NewCodeVerifier returns a new PKCE code_verifier (RFC 7636 §4.1): 32 random bytes, base64url-encoded.
func NewCodeVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return base64.RawURLEncoding.EncodeToString(b)
}

// CodeChallenge returns the S256 code_challenge of a code_verifier (RFC 7636 §4.2), the only method the server
// accepts.
func CodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Exchange exchanges the authorization code that the redirect brought for the tokens, with the code_verifier
// whose challenge the authorization request sent.
func (c OAuthConfig) Exchange(ctx context.Context, code, verifier string) (*Token, error) {
	return c.token(ctx, map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  c.RedirectURI,
	})
}

// OAuthError is an error answer of the token endpoint (RFC 6749 §5.2).
type OAuthError struct {
	// Status is the HTTP status of the answer.
	Status int
	// Code is the error code, such as "invalid_grant".
	Code        string
	Description string
}

func (e *OAuthError) Error() string {
	s := "mykit: oauth: " + e.Code
	if e.Description != "" {
		s += ": " + e.Description
	}
	return s
}

// tokenResponse is the body of a successful answer of the token endpoint (RFC 6749 §5.1).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// token posts a grant to the token endpoint, with the client's credentials, and reads the tokens.
func (c OAuthConfig) token(ctx context.Context, params map[string]string) (*Token, error) {
	origin, err := parseOrigin(c.Server)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	for k, v := range params {
		if v != "" {
			form.Set(k, v)
		}
	}
	if c.ClientSecret == "" {
		form.Set("client_id", c.ClientID)
	}
	endpoint := origin.String() + TokenPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)
	if c.ClientSecret != "" {
		// RFC 6749 §2.3.1: the credentials are form-encoded before they go into Basic.
		req.SetBasicAuth(url.QueryEscape(c.ClientID), url.QueryEscape(c.ClientSecret))
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	now := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &NetworkError{Method: req.Method, URL: endpoint, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProblemBytes))
	if err != nil {
		return nil, &NetworkError{Method: req.Method, URL: endpoint, Err: err}
	}
	if resp.StatusCode != http.StatusOK {
		oe := &OAuthError{Status: resp.StatusCode}
		if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "application/json" {
			var v struct {
				Error            string `json:"error"`
				ErrorDescription string `json:"error_description"`
			}
			if json.Unmarshal(body, &v) == nil {
				oe.Code, oe.Description = v.Error, v.ErrorDescription
			}
		}
		if oe.Code == "" {
			oe.Code = http.StatusText(resp.StatusCode)
		}
		return nil, oe
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, &OAuthError{Status: resp.StatusCode, Code: "invalid_response", Description: err.Error()}
	}
	if tr.AccessToken == "" || !strings.EqualFold(tr.TokenType, "bearer") {
		return nil, &OAuthError{Status: resp.StatusCode, Code: "invalid_response", Description: "no bearer access token"}
	}
	tok := &Token{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, Scope: tr.Scope}
	if tr.ExpiresIn > 0 {
		tok.Expiry = now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok, nil
}
