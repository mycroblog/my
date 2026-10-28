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
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The discord kind of external login provider (ADR-0106 §1, §4): discord.com with endpoints
// fixed in the code. The subject is the snowflake id of GET /users/@me (a string; username and
// global_name are for display and the sign-up defaults only), the email address counts as
// verified only when the user object says verified, and the guild condition reads
// GET /users/@me/guilds with the guilds scope on every sign-in.

// Discord's endpoints.
const (
	discordAuthorizeURL = "https://discord.com/oauth2/authorize"
	discordTokenURL     = "https://discord.com/api/oauth2/token" //nolint:gosec // an endpoint, not a credential
	discordAPI          = "https://discord.com/api"
)

// Discord scopes: identify is what GET /users/@me needs, email adds the address and its
// verified flag to it, and guilds is what GET /users/@me/guilds needs.
const (
	discordScopeIdentify = "identify"
	discordScopeEmail    = "email"
	discordScopeGuilds   = "guilds"
)

const (
	// discordGuildsPerPage is the largest page of GET /users/@me/guilds.
	discordGuildsPerPage = 200
	// discordGuildsMaxPages bounds the pages read from it (a user is in at most 200 guilds, or
	// 100 without Nitro, so one page is the usual answer).
	discordGuildsMaxPages = 5
)

// beginDiscord starts a sign-in, a link, or a confirmation with a discord provider, whose
// cookie seals t. Discord has no way to ask the user to sign in again, so a confirmation only
// shows that the user is signed in at Discord (ADR-0106 §3.7).
func (s *Service) beginDiscord(ctx context.Context, p identity.LoginProvider, t externalLoginToken) (ExternalLoginStart, error) {
	cookie, err := s.sealJSON(ctx, t, externalLoginAAD)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	scopes := []string{discordScopeIdentify, discordScopeEmail}
	if len(p.Settings.Guilds) > 0 {
		scopes = append(scopes, discordScopeGuilds)
	}
	scopes = append(scopes, p.Settings.Scopes...)
	slices.Sort(scopes)
	q := url.Values{
		"response_type": {"code"}, "client_id": {p.Settings.ClientID}, "redirect_uri": {s.externalCallbackURL(p.ID)},
		"scope": {strings.Join(slices.Compact(scopes), " ")}, "state": {t.State},
		"code_challenge": {pkceChallenge(t.Verifier)}, "code_challenge_method": {"S256"},
	}
	return ExternalLoginStart{AuthorizationURL: discordAuthorizeURL + "?" + q.Encode(), Cookie: cookie, ExpiresAt: time.UnixMilli(t.IssuedAt).Add(ExternalLoginTTL)}, nil
}

// discordIdentity redeems code at Discord and returns the account as the claims of an ID token
// that offerExternalSignUp reads: the subject is the snowflake id, preferred_username the
// username, and name the display name. It fails with ErrExternalLoginDenied when the provider
// names guilds and the user belongs to none of them, and with an error wrapping
// ErrExternalLoginFailed when Discord cannot be reached or refuses.
func (s *Service) discordIdentity(ctx context.Context, p identity.LoginProvider, code string, t externalLoginToken) (IDToken, error) {
	fail := func(err error) (IDToken, error) {
		return IDToken{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	secret, err := s.loginProviderClientSecret(ctx, p.ID)
	if err != nil {
		return IDToken{}, err
	}
	access, err := s.discordAccessToken(ctx, p.Settings.ClientID, secret, code, s.externalCallbackURL(p.ID), t.Verifier)
	if err != nil {
		return fail(err)
	}
	var user struct {
		ID         string  `json:"id"`
		Username   string  `json:"username"`
		GlobalName *string `json:"global_name"`
		Email      *string `json:"email"`
		Verified   bool    `json:"verified"`
	}
	if err := s.discordGet(ctx, access, "/users/@me", &user); err != nil {
		return fail(err)
	}
	if !identity.IsDiscordSnowflake(user.ID) {
		return fail(errors.New("GET /users/@me answered no usable id"))
	}
	if len(p.Settings.Guilds) > 0 {
		member, err := s.discordMemberOf(ctx, access, p.Settings.Guilds)
		if err != nil {
			return fail(err)
		}
		if !member {
			return IDToken{}, ErrExternalLoginDenied
		}
	}
	tok := IDToken{Issuer: "https://discord.com", Subject: user.ID, Claims: map[string]json.RawMessage{}}
	names := map[string]string{"preferred_username": user.Username}
	if user.GlobalName != nil {
		names["name"] = *user.GlobalName
	}
	for claim, v := range names {
		if b, err := json.Marshal(v); err == nil && v != "" {
			tok.Claims[claim] = b
		}
	}
	// The address counts as verified only when Discord says so; otherwise the sign-up asks for
	// an address and verifies it.
	if user.Email != nil && *user.Email != "" && user.Verified {
		tok.Email, tok.EmailVerified = *user.Email, true
	}
	return tok, nil
}

// discordAccessToken redeems an authorization code (RFC 6749 §4.1.3, RFC 7636 §4.5). The token
// is used for the sign-in and dropped.
func (s *Service) discordAccessToken(ctx context.Context, clientID, secret, code, redirectURI, verifier string) (string, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID}, "client_secret": {secret},
		"code": {code}, "redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: discordTokenURL, Purpose: kernel.HTTPPurposeLoginProvider, NoRedirects: true,
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

// discordGet reads a JSON document of Discord's REST API with the user's token.
func (s *Service) discordGet(ctx context.Context, token, path string, v any) error {
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodGet, URL: discordAPI + path, Purpose: kernel.HTTPPurposeLoginProvider, NoRedirects: true,
		Header: http.Header{"Authorization": {"Bearer " + token}, "Accept": {"application/json"}},
	})
	if err != nil {
		return err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return err
	}
	name := strings.SplitN(path, "?", 2)[0]
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s answered %d", name, resp.StatusCode)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("GET %s: %w", name, err)
	}
	return nil
}

// discordMemberOf reports whether the user belongs to one of the guilds, given by their ids.
func (s *Service) discordMemberOf(ctx context.Context, token string, allowed []string) (bool, error) {
	after := ""
	for page := 1; page <= discordGuildsMaxPages; page++ {
		path := fmt.Sprintf("/users/@me/guilds?limit=%d", discordGuildsPerPage)
		if after != "" {
			path += "&after=" + url.QueryEscape(after)
		}
		var guilds []struct {
			ID string `json:"id"`
		}
		if err := s.discordGet(ctx, token, path, &guilds); err != nil {
			return false, err
		}
		for _, g := range guilds {
			if slices.Contains(allowed, g.ID) {
				return true, nil
			}
		}
		if len(guilds) < discordGuildsPerPage {
			break
		}
		// The page is ordered by id, so the last id starts the next one.
		after = guilds[len(guilds)-1].ID
	}
	return false, nil
}
