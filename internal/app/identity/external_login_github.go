// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The github kind of external login provider (ADR-0106 §1, §4): github.com with endpoints fixed
// in the code. The subject is the numeric id of GET /user (kept as a string; the login is for
// display only), the email address is the primary and verified entry of GET /user/emails, and
// the Organization condition reads GET /user/orgs with the read:org scope on every sign-in.
// GitHub Enterprise Server is not a target.

// GitHub's endpoints.
const (
	gitHubAuthorizeURL = "https://github.com/login/oauth/authorize"
	gitHubTokenURL     = "https://github.com/login/oauth/access_token" //nolint:gosec // an endpoint, not a credential
	gitHubAPI          = "https://api.github.com"
)

// GitHub scopes: user:email is what GET /user/emails needs (GET /user needs none), and read:org
// is what GET /user/orgs needs to list the Organizations with private membership.
const (
	gitHubScopeEmail = "user:email"
	gitHubScopeOrg   = "read:org"
)

const (
	gitHubOrgsPerPage = 100
	// gitHubOrgsMaxPages bounds the pages read from GET /user/orgs.
	gitHubOrgsMaxPages = 10
)

// beginGitHub starts a sign-in, a link, or a confirmation with a github provider, whose cookie
// seals t. GitHub has no way to ask the user to sign in again, so a confirmation only shows that
// the user is signed in at GitHub (ADR-0106 §3.7).
func (s *Service) beginGitHub(ctx context.Context, p identity.LoginProvider, t externalLoginToken) (ExternalLoginStart, error) {
	cookie, err := s.sealJSON(ctx, t, externalLoginAAD)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	scopes := []string{gitHubScopeEmail}
	if len(p.Settings.Organizations) > 0 {
		scopes = append(scopes, gitHubScopeOrg)
	}
	scopes = append(scopes, p.Settings.Scopes...)
	slices.Sort(scopes)
	q := url.Values{
		"response_type": {"code"}, "client_id": {p.Settings.ClientID}, "redirect_uri": {s.externalCallbackURL(p.ID)},
		"scope": {strings.Join(slices.Compact(scopes), " ")}, "state": {t.State},
		"code_challenge": {pkceChallenge(t.Verifier)}, "code_challenge_method": {"S256"},
	}
	return ExternalLoginStart{AuthorizationURL: gitHubAuthorizeURL + "?" + q.Encode(), Cookie: cookie, ExpiresAt: time.UnixMilli(t.IssuedAt).Add(ExternalLoginTTL)}, nil
}

// gitHubIdentity redeems code at GitHub and returns the account as the claims of an ID token
// that offerExternalSignUp reads: the subject is the numeric id, and preferred_username the
// login. It fails with ErrExternalLoginDenied when the provider names Organizations and the
// user belongs to none of them, and with an error wrapping ErrExternalLoginFailed when GitHub
// cannot be reached or refuses.
func (s *Service) gitHubIdentity(ctx context.Context, p identity.LoginProvider, code string, t externalLoginToken) (IDToken, error) {
	fail := func(err error) (IDToken, error) {
		return IDToken{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	secret, err := s.loginProviderClientSecret(ctx, p.ID)
	if err != nil {
		return IDToken{}, err
	}
	access, err := s.gitHubAccessToken(ctx, p.Settings.ClientID, secret, code, s.externalCallbackURL(p.ID), t.Verifier)
	if err != nil {
		return fail(err)
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := s.gitHubGet(ctx, access, "/user", &user); err != nil {
		return fail(err)
	}
	if user.ID <= 0 {
		return fail(errors.New("GET /user answered no id"))
	}
	if len(p.Settings.Organizations) > 0 {
		member, err := s.gitHubMemberOf(ctx, access, p.Settings.Organizations)
		if err != nil {
			return fail(err)
		}
		if !member {
			return IDToken{}, ErrExternalLoginDenied
		}
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := s.gitHubGet(ctx, access, "/user/emails", &emails); err != nil {
		return fail(err)
	}
	tok := IDToken{Issuer: "https://github.com", Subject: strconv.FormatInt(user.ID, 10), Claims: map[string]json.RawMessage{}}
	if login, err := json.Marshal(user.Login); err == nil && user.Login != "" {
		tok.Claims["preferred_username"] = login
	}
	// Only the primary and verified address counts; with none, the sign-up asks for an address
	// and verifies it.
	for _, e := range emails {
		if e.Primary && e.Verified {
			tok.Email, tok.EmailVerified = e.Email, true
		}
	}
	return tok, nil
}

// gitHubAccessToken redeems an authorization code (RFC 6749 §4.1.3, RFC 7636 §4.5). GitHub
// answers errors with status 200 and an error field. The token is used for the sign-in and
// dropped.
func (s *Service) gitHubAccessToken(ctx context.Context, clientID, secret, code, redirectURI, verifier string) (string, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID}, "client_secret": {secret},
		"code": {code}, "redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: gitHubTokenURL, Purpose: kernel.HTTPPurposeLoginProvider, NoRedirects: true,
		Header: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"application/json"}},
		Body:   []byte(form.Encode()),
	})
	if err != nil {
		return "", err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return "", err
	}
	var answer struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(body, &answer) // an answer that is not JSON has neither
	if resp.StatusCode != http.StatusOK || answer.Error != "" {
		return "", fmt.Errorf("token endpoint answered %d %q", resp.StatusCode, answer.Error)
	}
	if answer.AccessToken == "" {
		return "", errors.New("token endpoint answered no access_token")
	}
	return answer.AccessToken, nil
}

// gitHubGet reads a JSON document of GitHub's REST API with the user's token.
func (s *Service) gitHubGet(ctx context.Context, token, path string, v any) error {
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodGet, URL: gitHubAPI + path, Purpose: kernel.HTTPPurposeLoginProvider, NoRedirects: true,
		Header: http.Header{
			"Authorization":        {"Bearer " + token},
			"Accept":               {"application/vnd.github+json"},
			"X-GitHub-Api-Version": {"2022-11-28"},
		},
	})
	if err != nil {
		return err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s answered %d", strings.SplitN(path, "?", 2)[0], resp.StatusCode)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("GET %s: %w", strings.SplitN(path, "?", 2)[0], err)
	}
	return nil
}

// gitHubMemberOf reports whether the user belongs to one of the Organizations, given by their
// logins in lower case.
func (s *Service) gitHubMemberOf(ctx context.Context, token string, allowed []string) (bool, error) {
	for page := 1; page <= gitHubOrgsMaxPages; page++ {
		var orgs []struct {
			Login string `json:"login"`
		}
		if err := s.gitHubGet(ctx, token, fmt.Sprintf("/user/orgs?per_page=%d&page=%d", gitHubOrgsPerPage, page), &orgs); err != nil {
			return false, err
		}
		for _, o := range orgs {
			if slices.Contains(allowed, strings.ToLower(o.Login)) {
				return true, nil
			}
		}
		if len(orgs) < gitHubOrgsPerPage {
			break
		}
	}
	return false, nil
}
