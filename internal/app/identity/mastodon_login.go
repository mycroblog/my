// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The mastodon login provider (ADR-0106 §1): one Mastodon server that the administrator names.
// The server reads the server's authorization server metadata (RFC 8414), which Mastodon serves
// from 4.3 on, and refuses a server without it. Creating the provider registers a client at the
// server (POST /api/v1/apps) with the callback URL and the profile scope, unless the
// administrator gives the client_id and client_secret registered there. A sign-in asks for the
// profile scope only; the account's identifier is the id of GET
// /api/v1/accounts/verify_credentials, and its username and display name only fill the
// registration form. The server gives no email address, so a sign-up verifies it by mail. While
// the server's domain has a domain action (FR-702), nobody signs in, signs up, or links with it.

var (
	// ErrMastodonServer reports a Mastodon server that cannot be a login provider: it cannot be
	// reached, or it serves no usable authorization server metadata (a server older than 4.3).
	ErrMastodonServer = errors.New("identity: the Mastodon server cannot be a login provider")
	// ErrMastodonRegistration reports a Mastodon server that refused to register the client;
	// the administrator registers one there and gives its client_id and client_secret.
	ErrMastodonRegistration = errors.New("identity: the Mastodon server did not register a client")
)

// MastodonScope is the only scope that a sign-in with a Mastodon server asks for: it lets the
// server read the account of verify_credentials and nothing else.
const MastodonScope = "profile"

// mastodonServer is the part of a Mastodon server's authorization server metadata that a sign-in
// uses.
type mastodonServer struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	// AppRegistrationEndpoint is Mastodon's POST /api/v1/apps.
	AppRegistrationEndpoint string   `json:"app_registration_endpoint"`
	Issuer                  string   `json:"issuer"`
	ScopesSupported         []string `json:"scopes_supported"`
}

// mastodonDiscover reads the authorization server metadata of the Mastodon server at server
// (https://<host>, RFC 8414 §3). It fails with an error wrapping ErrMastodonServer when the
// server cannot be reached or the metadata is missing, names another issuer, lacks an endpoint,
// puts an endpoint on another origin, or does not offer the profile scope.
func (s *Service) mastodonDiscover(ctx context.Context, server string) (mastodonServer, error) {
	if s.d.HTTP == nil {
		return mastodonServer{}, fmt.Errorf("%w: no HTTP client", ErrMastodonServer)
	}
	var m mastodonServer
	status, err := s.mastodonJSON(ctx, &kernel.HTTPRequest{URL: server + "/.well-known/oauth-authorization-server"}, &m)
	if err != nil {
		return mastodonServer{}, fmt.Errorf("%w: %s: %w", ErrMastodonServer, server, err)
	}
	if status != http.StatusOK {
		return mastodonServer{}, fmt.Errorf("%w: %s answered %d for its metadata (Mastodon 4.3 or later is required)", ErrMastodonServer, server, status)
	}
	if strings.TrimSuffix(m.Issuer, "/") != server {
		return mastodonServer{}, fmt.Errorf("%w: %s names the issuer %q", ErrMastodonServer, server, m.Issuer)
	}
	if m.AppRegistrationEndpoint == "" {
		m.AppRegistrationEndpoint = server + "/api/v1/apps"
	}
	for _, e := range []string{m.AuthorizationEndpoint, m.TokenEndpoint, m.AppRegistrationEndpoint} {
		if !sameOrigin(e, server) {
			return mastodonServer{}, fmt.Errorf("%w: %s names the endpoint %q", ErrMastodonServer, server, e)
		}
	}
	if !slices.Contains(m.ScopesSupported, MastodonScope) {
		return mastodonServer{}, fmt.Errorf("%w: %s does not offer the %s scope", ErrMastodonServer, server, MastodonScope)
	}
	return m, nil
}

// sameOrigin reports whether raw is an https URL on the origin of server (https://<host>).
func sameOrigin(raw, server string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && "https://"+strings.ToLower(u.Host) == server
}

// mastodonJSON sends req to a Mastodon server through kernel.HTTPClient
// (kernel.HTTPPurposeLoginProvider, no redirects) and decodes a 2xx answer into v. It returns the
// status.
func (s *Service) mastodonJSON(ctx context.Context, req *kernel.HTTPRequest, v any) (int, error) {
	req.Purpose, req.NoRedirects = kernel.HTTPPurposeLoginProvider, true
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.d.HTTP.Do(ctx, req)
	if err != nil {
		return 0, err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return 0, err
	}
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, nil
	}
	if err := json.Unmarshal(body, v); err != nil {
		return resp.StatusCode, fmt.Errorf("decode the answer of %s: %w", req.URL, err)
	}
	return resp.StatusCode, nil
}

// prepareMastodonProvider checks the server of new mastodon settings and returns the settings
// and the client secret to keep: the ones the administrator gave, or, when both are empty, the
// ones of a client that it registers at the server for the provider providerID. It fails with
// the errors of LoginProviderSettings.Validate, with ErrMastodonServer, and with
// ErrMastodonRegistration.
func (s *Service) prepareMastodonProvider(ctx context.Context, providerID string, settings identity.LoginProviderSettings, clientSecret string) (identity.LoginProviderSettings, string, error) {
	register := settings.ClientID == "" && strings.TrimSpace(clientSecret) == ""
	// The settings are checked before the server is asked, with a stand-in for the client_id
	// that the registration gives.
	check := settings
	if register {
		check.ClientID = "registered"
	}
	if err := check.Validate(); err != nil {
		return settings, clientSecret, err
	}
	m, err := s.mastodonDiscover(ctx, settings.Issuer)
	if err != nil || !register {
		return settings, clientSecret, err
	}
	var app struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	form := url.Values{
		"client_name":   {s.mastodonClientName()},
		"redirect_uris": {s.externalCallbackURL(providerID)},
		"scopes":        {MastodonScope},
		"website":       {strings.TrimSuffix(s.d.BaseURL, "/")},
	}
	status, err := s.mastodonJSON(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: m.AppRegistrationEndpoint,
		Header: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		Body:   []byte(form.Encode()),
	}, &app)
	switch {
	case err != nil:
		return settings, clientSecret, fmt.Errorf("%w: %s: %w", ErrMastodonRegistration, settings.Issuer, err)
	case status/100 != 2 || app.ClientID == "" || app.ClientSecret == "":
		return settings, clientSecret, fmt.Errorf("%w: %s answered %d", ErrMastodonRegistration, settings.Issuer, status)
	}
	settings.ClientID = app.ClientID
	return settings, app.ClientSecret, nil
}

// mastodonClientName is the name of the client that a Mastodon server shows its users: this
// server's host.
func (s *Service) mastodonClientName() string {
	if u, err := url.Parse(s.d.BaseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return s.d.BaseURL
}

// checkMastodonServerChange refuses to move a mastodon provider to another server: its client
// is registered at the first one. The administrator sets up a new provider instead.
func checkMastodonServerChange(old, settings identity.LoginProviderSettings) error {
	if old.Kind == identity.LoginProviderMastodon && settings.Issuer != old.Issuer {
		return fmt.Errorf("%w: the server of a mastodon provider does not change; set up a new provider", identity.ErrInvalidLoginProviderURL)
	}
	return nil
}

// checkLoginProviderServer refuses a mastodon provider whose server's domain has a domain action
// in effect (ADR-0106 §1), with an error wrapping ErrLoginProviderUnavailable.
func (s *Service) checkLoginProviderServer(ctx context.Context, p identity.LoginProvider) error {
	if p.Settings.Kind != identity.LoginProviderMastodon || s.d.LoginProviderDomainRestricted == nil {
		return nil
	}
	u, err := url.Parse(p.Settings.Issuer)
	if err != nil {
		return fmt.Errorf("%w: provider %s: %w", ErrLoginProviderUnavailable, p.ID, err)
	}
	restricted, err := s.d.LoginProviderDomainRestricted(ctx, u.Hostname())
	if err != nil {
		return err
	}
	if restricted {
		return fmt.Errorf("%w: provider %s: %s has a domain action", ErrLoginProviderUnavailable, p.ID, u.Hostname())
	}
	return nil
}

// beginMastodonLogin starts a sign-in with a mastodon provider, as BeginExternalLogin does.
func (s *Service) beginMastodonLogin(ctx context.Context, p identity.LoginProvider, returnTo string) (ExternalLoginStart, error) {
	now := s.d.Clock.Now()
	t := externalLoginToken{
		IssuedAt: now.UnixMilli(), ProviderID: p.ID, ReturnTo: returnTo, State: randomToken(), Verifier: randomToken(),
	}
	authURL, err := s.mastodonAuthorizationURL(ctx, p, t.State, t.Verifier, false)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	cookie, err := s.sealJSON(ctx, t, externalLoginAAD)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	return ExternalLoginStart{AuthorizationURL: authURL, Cookie: cookie, ExpiresAt: now.Add(ExternalLoginTTL)}, nil
}

// mastodonAuthorizationURL returns the authorization request of a mastodon provider with state and
// the PKCE challenge of verifier. forceLogin asks the server to have the user sign in again
// (force_login=true), which a confirmation with the provider needs (ADR-0106 §3.7). It fails
// with an error wrapping ErrExternalLoginFailed when the server's metadata cannot be used.
func (s *Service) mastodonAuthorizationURL(ctx context.Context, p identity.LoginProvider, state, verifier string, forceLogin bool) (string, error) {
	m, err := s.mastodonDiscover(ctx, p.Settings.Issuer)
	if err != nil {
		return "", fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	u, err := url.Parse(m.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("%w: provider %s: authorization endpoint: %w", ErrExternalLoginFailed, p.ID, err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", p.Settings.ClientID)
	q.Set("redirect_uri", s.externalCallbackURL(p.ID))
	q.Set("scope", MastodonScope)
	q.Set("state", state)
	challenge := sha256.Sum256([]byte(verifier))
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("code_challenge_method", "S256")
	if forceLogin {
		q.Set("force_login", "true")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// mastodonAccount exchanges code for an access token of a mastodon provider and returns the
// account of verify_credentials as an IDToken: its id as the subject, its username as the
// preferred_username, and its display name as the name; no email address. It fails with an
// error wrapping ErrInvalidIDToken for an account without a usable id, and with one wrapping
// ErrExternalLoginFailed when the server cannot be reached or refuses the code or the token.
func (s *Service) mastodonAccount(ctx context.Context, p identity.LoginProvider, code string, t externalLoginToken) (IDToken, error) {
	m, err := s.mastodonDiscover(ctx, p.Settings.Issuer)
	if err != nil {
		return IDToken{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	secret, err := s.loginProviderClientSecret(ctx, p.ID)
	if err != nil {
		return IDToken{}, err
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.externalCallbackURL(p.ID)},
		"code_verifier": {t.Verifier},
	}
	// RFC 6749 §2.3.1: the credentials are form-encoded before Basic encodes them.
	basic := base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(p.Settings.ClientID) + ":" + url.QueryEscape(secret)))
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	status, err := s.mastodonJSON(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: m.TokenEndpoint,
		Header: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Authorization": {"Basic " + basic}},
		Body:   []byte(form.Encode()),
	}, &tokens)
	switch {
	case err != nil:
		return IDToken{}, fmt.Errorf("%w: provider %s: token endpoint: %w", ErrExternalLoginFailed, p.ID, err)
	case status != http.StatusOK || tokens.AccessToken == "":
		return IDToken{}, fmt.Errorf("%w: provider %s: token endpoint answered %d", ErrExternalLoginFailed, p.ID, status)
	}
	// The access token is used for this one request and never kept (ADR-0106 §3).
	var acct struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	}
	status, err = s.mastodonJSON(ctx, &kernel.HTTPRequest{
		URL:    p.Settings.Issuer + "/api/v1/accounts/verify_credentials",
		Header: http.Header{"Authorization": {"Bearer " + tokens.AccessToken}},
	}, &acct)
	switch {
	case err != nil:
		return IDToken{}, fmt.Errorf("%w: provider %s: verify_credentials: %w", ErrExternalLoginFailed, p.ID, err)
	case status != http.StatusOK:
		return IDToken{}, fmt.Errorf("%w: provider %s: verify_credentials answered %d", ErrExternalLoginFailed, p.ID, status)
	}
	if err := identity.ValidateExternalSubject(acct.ID); err != nil {
		return IDToken{}, fmt.Errorf("%w: %w", ErrInvalidIDToken, err)
	}
	claims := map[string]json.RawMessage{}
	for name, v := range map[string]string{"preferred_username": acct.Username, "name": acct.DisplayName} {
		if v != "" {
			raw, _ := json.Marshal(v) // a string always marshals
			claims[name] = raw
		}
	}
	return IDToken{Issuer: p.Settings.Issuer, Subject: acct.ID, Claims: claims}, nil
}
