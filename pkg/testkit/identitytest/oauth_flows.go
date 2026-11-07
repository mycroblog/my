// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// The OAuth cases of AccountRepositoryContract (ADR-0019 §4, ADR-0090).

const (
	// ContractWebRedirect is the redirect URI of the web client.
	ContractWebRedirect = "https://social.example/login/callback"
	contractAppClientID = "third-party-app"
	contractAppSecret   = "app-secret"
	contractAppRedirect = "https://app.example/callback"
	// ContractCodeVerifier is the PKCE code verifier of the web client's requests.
	ContractCodeVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk" // RFC 7636 Appendix B
)

// contractWebClient is the web client of the contract's identity service.
var contractWebClient = appidentity.WebClient{
	ClientID: appidentity.DefaultWebClientID, Name: "Web", RedirectURIs: []string{ContractWebRedirect},
}

// ThirdPartyApp registers a confidential client that may read and write, which the user
// appdev registered.
func (e *AccountEnv) ThirdPartyApp(t *testing.T) identity.OAuthClient {
	t.Helper()
	ctx := t.Context()
	c := identity.OAuthClient{
		ID: e.IDs.NewID(), ClientID: contractAppClientID, SecretHash: appidentity.HashToken(contractAppSecret),
		Name: "App", Website: "https://app.example/", Scope: identity.Scope("read write"),
		Registration: identity.ClientRegisteredByUser, OwnerUserID: e.Register(t, "appdev").User.ID, CreatedAt: e.Clock.Now(),
	}
	var stored identity.OAuthClient
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if stored, err = e.H.Identity.EnsureOAuthClient(ctx, tx, c); err != nil {
			return err
		}
		return e.H.Identity.ReplaceOAuthClientRedirectURIs(ctx, tx, stored.ID, []string{contractAppRedirect})
	})
	if err != nil {
		t.Fatalf("register a third-party client: %v", err)
	}
	return stored
}

var appCreds = appidentity.ClientCredentials{ClientID: contractAppClientID, ClientSecret: contractAppSecret}

// WebCreds are the client credentials of the web client.
var WebCreds = appidentity.ClientCredentials{ClientID: appidentity.DefaultWebClientID}

// WebAuthorizationRequest is an authorization request of the web client with PKCE.
func WebAuthorizationRequest() appidentity.AuthorizationRequest {
	return appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: appidentity.DefaultWebClientID, RedirectURI: ContractWebRedirect,
		State: "xyz", CodeChallenge: identity.S256Challenge(ContractCodeVerifier), CodeChallengeMethod: "S256",
	}
}

// code validates req and issues a code of it to the user.
func (e *AccountEnv) code(t *testing.T, req appidentity.AuthorizationRequest, userID string) string {
	t.Helper()
	return e.CodeInSession(t, req, identity.Session{UserID: userID})
}

// CodeInSession validates req and issues a code of it to the user of a browser session; a
// session without an ID issues the code outside one.
func (e *AccountEnv) CodeInSession(t *testing.T, req appidentity.AuthorizationRequest, session identity.Session) string {
	t.Helper()
	a, err := e.Identity.ValidateAuthorization(t.Context(), req)
	if err != nil {
		t.Fatalf("ValidateAuthorization = %v", err)
	}
	a.SessionID = session.ID
	code, err := e.Identity.IssueAuthorizationCode(t.Context(), a, session.UserID)
	if err != nil {
		t.Fatalf("IssueAuthorizationCode = %v", err)
	}
	return code
}

// WebTokens signs a user up and exchanges a code of the web client for tokens.
func (e *AccountEnv) WebTokens(t *testing.T) (appidentity.IssuedTokens, string) {
	t.Helper()
	return e.WebTokensFor(t, e.Register(t, "alice").User.ID)
}

func wantOAuthError(t *testing.T, err error, code, what string) {
	t.Helper()
	var oe *appidentity.OAuthError
	if !errors.As(err, &oe) || oe.Code != code {
		t.Errorf("%s = %v, want the OAuth error %s", what, err, code)
	}
}

// WantAccess fails t unless token authenticates when ok, or fails to when not.
func (e *AccountEnv) WantAccess(t *testing.T, token string, ok bool, what string) {
	t.Helper()
	_, err := e.Identity.AuthenticateAccessToken(t.Context(), token)
	if ok && err != nil {
		t.Errorf("AuthenticateAccessToken(%s) = %v, want it to work", what, err)
	}
	if !ok && !errors.Is(err, appidentity.ErrInvalidAccessToken) {
		t.Errorf("AuthenticateAccessToken(%s) = %v, want ErrInvalidAccessToken", what, err)
	}
}

func oauthRegistersTheWebClient(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	for range 2 {
		if err := e.Identity.EnsureWebClient(ctx); err != nil {
			t.Fatalf("EnsureWebClient = %v", err)
		}
	}
	c, uris, err := e.H.Identity.FindOAuthClient(ctx, appidentity.DefaultWebClientID)
	if err != nil {
		t.Fatalf("FindOAuthClient = %v", err)
	}
	if !c.FirstParty() || !c.Public() || c.Scope != identity.AllScopes() || c.Name != "Web" || c.OwnerUserID != "" {
		t.Errorf("web client = %+v, want a public first-party client with every scope", c)
	}
	if !reflect.DeepEqual(uris, []string{ContractWebRedirect}) {
		t.Errorf("redirect URIs = %q, want %q", uris, ContractWebRedirect)
	}

	// A new web origin replaces the redirect URIs and keeps the client.
	moved := e.newIdentityWith(func(d *appidentity.Deps) {
		d.WebClient.RedirectURIs = []string{"https://b.example/login/callback", "https://a.example/login/callback"}
	})
	if err := moved.EnsureWebClient(ctx); err != nil {
		t.Fatalf("EnsureWebClient(moved) = %v", err)
	}
	again, uris, err := e.H.Identity.FindOAuthClient(ctx, appidentity.DefaultWebClientID)
	if err != nil || again.ID != c.ID || !again.CreatedAt.Equal(c.CreatedAt) {
		t.Fatalf("client after moving = %+v, %v; want %s unchanged", again, err, c.ID)
	}
	if want := []string{"https://a.example/login/callback", "https://b.example/login/callback"}; !reflect.DeepEqual(uris, want) {
		t.Errorf("redirect URIs after moving = %q, want %q", uris, want)
	}
	if _, err := moved.ValidateAuthorization(ctx, WebAuthorizationRequest()); !errors.Is(err, appidentity.ErrInvalidRedirectURI) {
		t.Errorf("the old redirect URI = %v, want ErrInvalidRedirectURI", err)
	}

	// Without a start-up registration, the first request registers the client.
	lazy := e.newIdentityWith(func(d *appidentity.Deps) {
		d.WebClient = appidentity.WebClient{ClientID: "lazy-web", Name: "Lazy", RedirectURIs: []string{ContractWebRedirect}}
	})
	req := WebAuthorizationRequest()
	req.ClientID = "lazy-web"
	if _, err := lazy.ValidateAuthorization(ctx, req); err != nil {
		t.Errorf("ValidateAuthorization before EnsureWebClient = %v", err)
	}
}

func oauthValidatesAuthorizationRequests(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	e.ThirdPartyApp(t)
	ok, err := e.Identity.ValidateAuthorization(ctx, WebAuthorizationRequest())
	if err != nil {
		t.Fatalf("ValidateAuthorization = %v", err)
	}
	if ok.Scope != identity.AllScopes() || ok.State != "xyz" || ok.RedirectURI != ContractWebRedirect || !ok.Client.FirstParty() {
		t.Errorf("authorization = %+v, want the client's whole scope", ok)
	}
	cases := []struct {
		name   string
		change func(*appidentity.AuthorizationRequest)
		want   error  // a sentinel, or nil for an OAuth error
		code   string // the OAuth error code
	}{
		{"unknown client", func(r *appidentity.AuthorizationRequest) { r.ClientID = "nobody" }, appidentity.ErrUnknownClient, ""},
		{"no client", func(r *appidentity.AuthorizationRequest) { r.ClientID = "" }, appidentity.ErrUnknownClient, ""},
		{"no redirect_uri", func(r *appidentity.AuthorizationRequest) { r.RedirectURI = "" }, appidentity.ErrInvalidRedirectURI, ""},
		{"trailing slash", func(r *appidentity.AuthorizationRequest) { r.RedirectURI += "/" }, appidentity.ErrInvalidRedirectURI, ""},
		{"extra query", func(r *appidentity.AuthorizationRequest) { r.RedirectURI += "?next=/" }, appidentity.ErrInvalidRedirectURI, ""},
		{"prefix", func(r *appidentity.AuthorizationRequest) { r.RedirectURI = "https://social.example/login" }, appidentity.ErrInvalidRedirectURI, ""},
		{"upper-case host", func(r *appidentity.AuthorizationRequest) {
			r.RedirectURI = strings.Replace(r.RedirectURI, "social", "SOCIAL", 1)
		}, appidentity.ErrInvalidRedirectURI, ""},
		{"another client's URI", func(r *appidentity.AuthorizationRequest) { r.RedirectURI = contractAppRedirect }, appidentity.ErrInvalidRedirectURI, ""},
		{"implicit grant", func(r *appidentity.AuthorizationRequest) { r.ResponseType = "token" }, nil, appidentity.OAuthUnsupportedResponseType},
		{"plain PKCE", func(r *appidentity.AuthorizationRequest) {
			r.CodeChallengeMethod, r.CodeChallenge = "plain", ContractCodeVerifier
		}, nil, appidentity.OAuthInvalidRequest},
		{"no PKCE method", func(r *appidentity.AuthorizationRequest) { r.CodeChallengeMethod = "" }, nil, appidentity.OAuthInvalidRequest},
		{"no PKCE", func(r *appidentity.AuthorizationRequest) { r.CodeChallenge, r.CodeChallengeMethod = "", "" }, nil, appidentity.OAuthInvalidRequest},
		{"malformed challenge", func(r *appidentity.AuthorizationRequest) { r.CodeChallenge = "too-short" }, nil, appidentity.OAuthInvalidRequest},
		{"unknown scope", func(r *appidentity.AuthorizationRequest) { r.Scope = "read superuser" }, nil, appidentity.OAuthInvalidScope},
		{"long state", func(r *appidentity.AuthorizationRequest) { r.State = strings.Repeat("s", 1025) }, nil, appidentity.OAuthInvalidRequest},
	}
	for _, tc := range cases {
		req := WebAuthorizationRequest()
		tc.change(&req)
		_, err := e.Identity.ValidateAuthorization(ctx, req)
		if tc.want != nil {
			if !errors.Is(err, tc.want) {
				t.Errorf("%s: ValidateAuthorization = %v, want %v", tc.name, err, tc.want)
			}
			continue
		}
		wantOAuthError(t, err, tc.code, tc.name)
	}

	// Scopes are normalized and capped by the client's scope.
	app := appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: contractAppClientID, RedirectURI: contractAppRedirect, Scope: "write read write",
		CodeChallenge: identity.S256Challenge(ContractCodeVerifier), CodeChallengeMethod: "S256",
	}
	if a, err := e.Identity.ValidateAuthorization(ctx, app); err != nil || a.Scope != "read write" || a.Client.FirstParty() {
		t.Errorf("ValidateAuthorization(app) = %+v, %v; want the scope \"read write\"", a, err)
	}
	app.Scope = "read follow"
	_, err = e.Identity.ValidateAuthorization(ctx, app)
	wantOAuthError(t, err, appidentity.OAuthInvalidScope, "a scope beyond the client's")

	// A third-party client never gets a first-party-only scope (ADR-0090 §4), though its broad
	// scope write would grant write:supporters; the first-party web client may have them.
	for _, scope := range []string{"admin", "admin:read", "admin:write", "write:supporters", "read write:supporters"} {
		app.Scope = scope
		_, err = e.Identity.ValidateAuthorization(ctx, app)
		wantOAuthError(t, err, appidentity.OAuthInvalidScope, "the first-party-only scope "+scope+" of a third-party client")
	}
	web := WebAuthorizationRequest()
	web.Scope = "admin write:supporters"
	if _, err := e.Identity.ValidateAuthorization(ctx, web); err != nil {
		t.Errorf("ValidateAuthorization(first-party-only scopes of the web client) = %v", err)
	}

	// Tokens refer to their client by its row ID.
	byID, err := e.H.Identity.FindOAuthClientByID(ctx, ok.Client.ID)
	if err != nil || byID.ID != ok.Client.ID || !byID.FirstParty() {
		t.Errorf("FindOAuthClientByID = %+v, %v; want the web client", byID, err)
	}
	if _, err := e.H.Identity.FindOAuthClientByID(ctx, e.IDs.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindOAuthClientByID(unknown) = %v, want ErrNotFound", err)
	}
}

func oauthExchangesCodesWithPKCE(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.Register(t, "alice")
	e.ThirdPartyApp(t)
	code := e.code(t, WebAuthorizationRequest(), reg.User.ID)
	if !strings.HasPrefix(code, appidentity.AuthorizationCodePrefix) {
		t.Errorf("code = %q, want the prefix %s", code, appidentity.AuthorizationCodePrefix)
	}
	if _, err := e.H.Identity.FindAuthorizationCode(ctx, []byte(code)); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("the code is stored in plain text: %v", err)
	}
	exchange := func(change func(*appidentity.CodeExchange)) (appidentity.IssuedTokens, error) {
		in := appidentity.CodeExchange{
			ClientCredentials: WebCreds, Code: code, RedirectURI: ContractWebRedirect, CodeVerifier: ContractCodeVerifier,
		}
		if change != nil {
			change(&in)
		}
		return e.Identity.ExchangeAuthorizationCode(ctx, in)
	}
	for _, tc := range []struct {
		name   string
		change func(*appidentity.CodeExchange)
		code   string
	}{
		{"wrong verifier", func(in *appidentity.CodeExchange) { in.CodeVerifier = strings.Repeat("a", 43) }, appidentity.OAuthInvalidGrant},
		{"the challenge as verifier", func(in *appidentity.CodeExchange) { in.CodeVerifier = identity.S256Challenge(ContractCodeVerifier) }, appidentity.OAuthInvalidGrant},
		{"short verifier", func(in *appidentity.CodeExchange) { in.CodeVerifier = ContractCodeVerifier[:42] }, appidentity.OAuthInvalidGrant},
		{"no verifier", func(in *appidentity.CodeExchange) { in.CodeVerifier = "" }, appidentity.OAuthInvalidRequest},
		{"no redirect_uri", func(in *appidentity.CodeExchange) { in.RedirectURI = "" }, appidentity.OAuthInvalidRequest},
		{"other redirect_uri", func(in *appidentity.CodeExchange) { in.RedirectURI = ContractWebRedirect + "/" }, appidentity.OAuthInvalidGrant},
		{"other client", func(in *appidentity.CodeExchange) { in.ClientCredentials = appCreds }, appidentity.OAuthInvalidGrant},
		{"unknown client", func(in *appidentity.CodeExchange) { in.ClientID = "nobody" }, appidentity.OAuthInvalidClient},
		{"secret for a public client", func(in *appidentity.CodeExchange) { in.ClientSecret = "x" }, appidentity.OAuthInvalidClient},
		{"unknown code", func(in *appidentity.CodeExchange) { in.Code = appidentity.AuthorizationCodePrefix + "unknown" }, appidentity.OAuthInvalidGrant},
	} {
		_, err := exchange(tc.change)
		wantOAuthError(t, err, tc.code, tc.name)
	}

	// The failures did not use the code up.
	now := e.Clock.Now()
	got, err := exchange(nil)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode = %v", err)
	}
	if !strings.HasPrefix(got.AccessToken, appidentity.AccessTokenPrefix) || !strings.HasPrefix(got.RefreshToken, appidentity.RefreshTokenPrefix) {
		t.Errorf("tokens = %q, %q; want the prefixes %s and %s", got.AccessToken, got.RefreshToken,
			appidentity.AccessTokenPrefix, appidentity.RefreshTokenPrefix)
	}
	tok, err := e.Identity.AuthenticateAccessToken(ctx, got.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken = %v", err)
	}
	if tok.ID != got.Token.ID || tok.FamilyID != tok.ID || tok.UserID != reg.User.ID || tok.Scope != identity.AllScopes() ||
		tok.ClientID != got.Client.ID || !tok.ExpiresAt.Equal(now.Add(appidentity.AccessTokenTTL)) ||
		!tok.RefreshExpiresAt.Equal(now.Add(appidentity.RefreshTokenTTL)) {
		t.Errorf("token = %+v, want a new family of %s expiring after %v", tok, reg.User.ID, appidentity.AccessTokenTTL)
	}
	if !bytes.Equal(tok.AccessTokenHash, appidentity.HashToken(got.AccessToken)) ||
		!bytes.Equal(tok.RefreshTokenHash, appidentity.HashToken(got.RefreshToken)) {
		t.Error("the stored token does not hold the SHA-256 digests of the tokens")
	}
	if _, _, err := e.H.Identity.FindAccessTokenByAccessHash(ctx, []byte(got.AccessToken)); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("the access token is stored in plain text: %v", err)
	}

	// The access token expires.
	e.Clock.Advance(appidentity.AccessTokenTTL - time.Millisecond)
	e.WantAccess(t, got.AccessToken, true, "just before expiry")
	e.Clock.Advance(time.Millisecond)
	e.WantAccess(t, got.AccessToken, false, "expired")
	e.WantAccess(t, "", false, "empty")
	e.WantAccess(t, appidentity.AccessTokenPrefix+"unknown", false, "unknown")
	e.WantAccess(t, got.RefreshToken, false, "a refresh token")
}

func oauthCodesWorkOnce(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.Register(t, "alice")
	code := e.code(t, WebAuthorizationRequest(), reg.User.ID)
	in := appidentity.CodeExchange{ClientCredentials: WebCreds, Code: code, RedirectURI: ContractWebRedirect, CodeVerifier: ContractCodeVerifier}
	first, err := e.Identity.ExchangeAuthorizationCode(ctx, in)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode = %v", err)
	}
	if _, err := e.H.Identity.FindAuthorizationCodeRedemption(ctx, appidentity.HashToken(code)); err != nil {
		t.Errorf("FindAuthorizationCodeRedemption = %v, want the redemption", err)
	}
	_, err = e.Identity.ExchangeAuthorizationCode(ctx, in)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "the second exchange")
	// The reuse revoked what the code granted.
	e.WantAccess(t, first.AccessToken, false, "a token of a reused code")
	_, err = e.Identity.RefreshWebClient(ctx, first.RefreshToken)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "refreshing a token of a reused code")

	// A code expires after its lifetime.
	late := e.code(t, WebAuthorizationRequest(), reg.User.ID)
	e.Clock.Advance(appidentity.AuthorizationCodeTTL)
	in.Code = late
	_, err = e.Identity.ExchangeAuthorizationCode(ctx, in)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "an expired code")
}

func oauthRotatesRefreshTokens(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	first, _ := e.WebTokens(t)
	e.Clock.Advance(time.Minute)
	second, err := e.Identity.RefreshWebClient(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshWebClient = %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Fatal("refreshing kept a token")
	}
	if second.Token.FamilyID != first.Token.ID || second.Token.RotatedFromTokenID != first.Token.ID || second.Token.Scope != first.Token.Scope {
		t.Errorf("rotated token = %+v, want it in the family of %s", second.Token, first.Token.ID)
	}
	e.WantAccess(t, first.AccessToken, false, "the rotated access token")
	e.WantAccess(t, second.AccessToken, true, "the new access token")
	if _, state, err := e.H.Identity.FindAccessTokenByRefreshHash(ctx, appidentity.HashToken(first.RefreshToken)); err != nil || !state.Revoked {
		t.Errorf("the rotated token: revoked = %v, %v; want true", state.Revoked, err)
	}

	third, err := e.Identity.RefreshWebClient(ctx, second.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshWebClient(second) = %v", err)
	}
	// Using a used refresh token again revokes the whole family, including the newest tokens.
	_, err = e.Identity.RefreshWebClient(ctx, first.RefreshToken)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "a reused refresh token")
	e.WantAccess(t, third.AccessToken, false, "the newest access token after a reuse")
	_, err = e.Identity.RefreshWebClient(ctx, third.RefreshToken)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "the newest refresh token after a reuse")

	// Another family of the same user is not affected.
	other, _ := e.WebTokensFor(t, third.Token.UserID)
	if _, err := e.Identity.RefreshWebClient(ctx, other.RefreshToken); err != nil {
		t.Errorf("RefreshWebClient(another family) = %v", err)
	}
}

func oauthRefreshTokensExpire(t *testing.T, e *AccountEnv) {
	got, _ := e.WebTokens(t)
	e.Clock.Advance(appidentity.RefreshTokenTTL)
	_, err := e.Identity.RefreshWebClient(t.Context(), got.RefreshToken)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "an expired refresh token")
	_, err = e.Identity.RefreshWebClient(t.Context(), "")
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "an empty refresh token")
}

func oauthRefreshesThirdPartyTokens(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.Register(t, "alice")
	e.ThirdPartyApp(t)
	req := appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: contractAppClientID, RedirectURI: contractAppRedirect,
		CodeChallenge: identity.S256Challenge(ContractCodeVerifier), CodeChallengeMethod: "S256",
	}
	code := e.code(t, req, reg.User.ID)
	exchange := appidentity.CodeExchange{ClientCredentials: appCreds, Code: code, RedirectURI: contractAppRedirect, CodeVerifier: ContractCodeVerifier}
	wrong := exchange
	wrong.ClientSecret = "wrong"
	_, err := e.Identity.ExchangeAuthorizationCode(ctx, wrong)
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "a wrong client secret")
	wrong.ClientSecret = ""
	_, err = e.Identity.ExchangeAuthorizationCode(ctx, wrong)
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "no client secret")
	got, err := e.Identity.ExchangeAuthorizationCode(ctx, exchange)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode(app) = %v", err)
	}

	_, err = e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: appCreds, RefreshToken: got.RefreshToken, Scope: "read write follow"})
	wantOAuthError(t, err, appidentity.OAuthInvalidScope, "widening the scope")
	_, err = e.Identity.RefreshWebClient(ctx, got.RefreshToken)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "the app's refresh token through the web client's cookie")
	narrowed, err := e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: appCreds, RefreshToken: got.RefreshToken, Scope: "read"})
	if err != nil || narrowed.Token.Scope != "read" {
		t.Fatalf("RefreshAccessToken(read) = %+v, %v; want the scope read", narrowed.Token, err)
	}

	// The web client refreshes only through its cookie.
	web, _ := e.WebTokensFor(t, reg.User.ID)
	_, err = e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: WebCreds, RefreshToken: web.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthUnauthorizedClient, "the web client's refresh_token grant")
	_, err = e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: appCreds, RefreshToken: web.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "the web client's refresh token presented by the app")
	if _, err := e.Identity.RefreshWebClient(ctx, web.RefreshToken); err != nil {
		t.Errorf("RefreshWebClient after the refused grants = %v", err)
	}
}

func oauthRevokesTokens(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	e.ThirdPartyApp(t)
	got, userID := e.WebTokens(t)
	rotated, err := e.Identity.RefreshWebClient(ctx, got.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := e.WebTokensFor(t, userID)
	wantFamily := func(familyID string, want bool, what string) {
		t.Helper()
		if got, err := e.Identity.TokenFamilyActive(ctx, familyID); err != nil || got != want {
			t.Errorf("TokenFamilyActive(%s) = %v, %v; want %v", what, got, err, want)
		}
	}
	wantFamily(rotated.Token.FamilyID, true, "a rotated family")
	if rotated.Token.FamilyID != got.Token.FamilyID {
		t.Errorf("the rotated token's family %s, want %s", rotated.Token.FamilyID, got.Token.FamilyID)
	}

	// Unknown tokens and tokens of other clients are ignored.
	if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: WebCreds, Token: "unknown"}); err != nil {
		t.Errorf("RevokeToken(unknown) = %v, want nil", err)
	}
	if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: appCreds, Token: rotated.AccessToken}); err != nil {
		t.Errorf("RevokeToken(another client's token) = %v, want nil", err)
	}
	e.WantAccess(t, rotated.AccessToken, true, "a token another client tried to revoke")
	err = e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: appidentity.ClientCredentials{ClientID: contractAppClientID}, Token: rotated.AccessToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "revoking without the client secret")

	// Revoking the access token ends the refresh token of the family too.
	if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: WebCreds, Token: rotated.AccessToken}); err != nil {
		t.Fatalf("RevokeToken = %v", err)
	}
	e.WantAccess(t, rotated.AccessToken, false, "a revoked token")
	_, err = e.Identity.RefreshWebClient(ctx, rotated.RefreshToken)
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "the refresh token of a revoked family")

	// Revoking by refresh token works the same.
	if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: WebCreds, Token: other.RefreshToken, TokenTypeHint: "refresh_token"}); err != nil {
		t.Fatalf("RevokeToken(refresh) = %v", err)
	}
	e.WantAccess(t, other.AccessToken, false, "the access token of a family revoked by its refresh token")
	wantFamily(rotated.Token.FamilyID, false, "a revoked family")
	wantFamily(other.Token.FamilyID, false, "a family revoked by its refresh token")
	wantFamily(testkit.NamedID("unknown"), false, "an unknown family")

	// A family whose refresh token expired has ended too.
	last, _ := e.WebTokensFor(t, userID)
	wantFamily(last.Token.FamilyID, true, "a new family")
	e.Clock.Advance(appidentity.RefreshTokenTTL + time.Second)
	wantFamily(last.Token.FamilyID, false, "an expired family")
}

func oauthWebSignOutEndsTheBrowserSession(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	e.SessionUser(t)
	e.ThirdPartyApp(t)
	exchange := func(req appidentity.AuthorizationRequest, creds appidentity.ClientCredentials, session identity.Session) appidentity.IssuedTokens {
		t.Helper()
		got, err := e.Identity.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
			ClientCredentials: creds, Code: e.CodeInSession(t, req, session), RedirectURI: req.RedirectURI, CodeVerifier: ContractCodeVerifier,
		})
		if err != nil {
			t.Fatalf("ExchangeAuthorizationCode(%s) = %v", creds.ClientID, err)
		}
		return got
	}
	appRequest := appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: contractAppClientID, RedirectURI: contractAppRedirect,
		CodeChallenge: identity.S256Challenge(ContractCodeVerifier), CodeChallengeMethod: "S256",
	}

	// Revoking a third-party client's token leaves the browser session alone.
	browser := e.SignIn(t, appidentity.SignInInput{})
	app := exchange(appRequest, appCreds, browser.Session)
	if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: appCreds, Token: app.AccessToken}); err != nil {
		t.Fatalf("RevokeToken(app) = %v", err)
	}
	if _, err := e.Identity.Authenticate(ctx, browser.Token); err != nil {
		t.Fatalf("Authenticate after the app's revocation = %v, want the session to stay", err)
	}

	// The web client's sign-out, even with a token rotated since, ends the session that
	// authorized it and no other.
	other := e.SignIn(t, appidentity.SignInInput{})
	web := exchange(WebAuthorizationRequest(), WebCreds, browser.Session)
	exchange(WebAuthorizationRequest(), WebCreds, other.Session)
	rotated, err := e.Identity.RefreshWebClient(ctx, web.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // revoking twice is harmless
		if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: WebCreds, Token: rotated.AccessToken}); err != nil {
			t.Fatalf("RevokeToken(web) = %v", err)
		}
	}
	e.wantInvalidSession(t, browser.Token, "the session of a signed-out web client")
	if _, err := e.Identity.Authenticate(ctx, other.Token); err != nil {
		t.Errorf("Authenticate(another browser) = %v, want its session to stay", err)
	}

	// A session that ended before the sign-out, and a code issued outside a session, are fine.
	signedOut := e.SignIn(t, appidentity.SignInInput{})
	late := exchange(WebAuthorizationRequest(), WebCreds, signedOut.Session)
	if err := e.Identity.SignOut(ctx, signedOut.Token); err != nil {
		t.Fatal(err)
	}
	outside := exchange(WebAuthorizationRequest(), WebCreds, identity.Session{UserID: signedOut.Session.UserID})
	for _, tok := range []string{late.AccessToken, outside.AccessToken} {
		if err := e.Identity.RevokeToken(ctx, appidentity.RevokeRequest{ClientCredentials: WebCreds, Token: tok}); err != nil {
			t.Errorf("RevokeToken = %v", err)
		}
		e.WantAccess(t, tok, false, "a revoked token")
	}
	if _, err := e.Identity.Authenticate(ctx, other.Token); err != nil {
		t.Errorf("Authenticate(another browser) after the other sign-outs = %v, want its session to stay", err)
	}
}

// WebTokensFor exchanges a code of the web client for tokens of an existing user.
func (e *AccountEnv) WebTokensFor(t *testing.T, userID string) (appidentity.IssuedTokens, string) {
	t.Helper()
	code := e.code(t, WebAuthorizationRequest(), userID)
	got, err := e.Identity.ExchangeAuthorizationCode(t.Context(), appidentity.CodeExchange{
		ClientCredentials: WebCreds, Code: code, RedirectURI: ContractWebRedirect, CodeVerifier: ContractCodeVerifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode = %v", err)
	}
	return got, userID
}
