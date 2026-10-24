// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The dynamic client registration cases of AccountRepositoryContract (RFC 7591, ADR-0090 §1):
// the registration of public clients and the deletion of the ones nobody uses.

const dynamicRedirect = "https://dynamic.example/cb" //nolint:gosec // G101: a URL, not a credential

// dynamicService returns an identity service on the env's harness that accepts dynamic client
// registration.
func (e *accountEnv) dynamicService(enabled bool) *appidentity.Service {
	return e.newIdentityWith(func(d *appidentity.Deps) {
		d.DynamicClientRegistration = func(context.Context) (bool, error) { return enabled, nil }
	})
}

func (e *accountEnv) registerDynamic(t *testing.T, s *appidentity.Service) appidentity.DynamicRegistration {
	t.Helper()
	reg, err := s.RegisterDynamicClient(t.Context(), appidentity.DynamicRegistrationRequest{
		RedirectURIs: []string{dynamicRedirect}, ClientName: "Dynamic app",
	})
	if err != nil {
		t.Fatalf("RegisterDynamicClient = %v", err)
	}
	return reg
}

// dynamicTokens authorizes the dynamic client for the user and exchanges the code for tokens.
func (e *accountEnv) dynamicTokens(t *testing.T, s *appidentity.Service, clientID, userID string) appidentity.IssuedTokens {
	t.Helper()
	a, err := s.ValidateAuthorization(t.Context(), appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: clientID, RedirectURI: dynamicRedirect,
		CodeChallenge: identity.S256Challenge(contractCodeVerifier), CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("ValidateAuthorization(dynamic client) = %v", err)
	}
	code, err := s.IssueAuthorizationCode(t.Context(), a, userID)
	if err != nil {
		t.Fatalf("IssueAuthorizationCode = %v", err)
	}
	got, err := s.ExchangeAuthorizationCode(t.Context(), appidentity.CodeExchange{
		ClientCredentials: appidentity.ClientCredentials{ClientID: clientID}, Code: code,
		RedirectURI: dynamicRedirect, CodeVerifier: contractCodeVerifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode(dynamic client) = %v", err)
	}
	return got
}

func oauthRegistersDynamicClients(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	userID := e.register(t, "alice").User.ID
	s := e.dynamicService(true)

	// The scope is the requested one without the first-party-only scopes and unknown ones.
	reg, err := s.RegisterDynamicClient(ctx, appidentity.DynamicRegistrationRequest{
		RedirectURIs: []string{dynamicRedirect, dynamicRedirect, "http://127.0.0.1/cb"}, ClientName: "Dynamic app",
		ClientURI: "https://dynamic.example/", Scope: "write read admin write:supporters bogus",
		GrantTypes: []string{"refresh_token", "authorization_code"},
	})
	if err != nil {
		t.Fatalf("RegisterDynamicClient = %v", err)
	}
	c := reg.Client
	if c.Registration != identity.ClientRegisteredDynamically || !c.Public() || c.FirstParty() || c.Scope != "read write" ||
		c.Name != "Dynamic app" || c.Website != "https://dynamic.example/" || reg.TokenEndpointAuthMethod != "none" {
		t.Errorf("registered client = %+v, %+v", c, reg)
	}
	if len(reg.RedirectURIs) != 2 {
		t.Errorf("redirect URIs = %v, want the 2 distinct ones", reg.RedirectURIs)
	}
	stored, uris, err := e.h.Identity.FindOAuthClient(ctx, c.ClientID)
	if err != nil || stored.Scope != "read write" || stored.Registration != identity.ClientRegisteredDynamically || len(uris) != 2 {
		t.Fatalf("FindOAuthClient = %+v, %v, %v", stored, uris, err)
	}

	// Authorization through the token exchange works for the registered client, without a secret.
	got := e.dynamicTokens(t, s, c.ClientID, userID)
	e.wantAccess(t, got.AccessToken, true, "the access token of a dynamic client")
	if got.Token.Scope != "read write" {
		t.Errorf("token scope = %q, want read write", got.Token.Scope)
	}

	// Without scope, grant_types, or a name, the defaults apply.
	bare, err := s.RegisterDynamicClient(ctx, appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}})
	if err != nil || bare.Client.Scope != identity.ThirdPartyScopes() || bare.Client.Name != bare.Client.ClientID ||
		len(bare.GrantTypes) != 1 || bare.GrantTypes[0] != "authorization_code" {
		t.Errorf("bare registration = %+v, %v", bare, err)
	}

	// Metadata the server refuses.
	for name, tc := range map[string]struct {
		req  appidentity.DynamicRegistrationRequest
		code string
	}{
		"client_secret_basic": {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}, TokenEndpointAuthMethod: "client_secret_basic"}, appidentity.OAuthInvalidClientMetadata},
		"no redirect URI":     {appidentity.DynamicRegistrationRequest{}, appidentity.OAuthInvalidRedirectURI},
		"http redirect URI":   {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{"http://dynamic.example/cb"}}, appidentity.OAuthInvalidRedirectURI},
		"localhost redirect":  {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{"http://localhost/cb"}}, appidentity.OAuthInvalidRedirectURI},
		"implicit grant":      {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}, GrantTypes: []string{"implicit"}}, appidentity.OAuthInvalidClientMetadata},
		"token response":      {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}, ResponseTypes: []string{"token"}}, appidentity.OAuthInvalidClientMetadata},
		"software statement":  {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}, SoftwareStatement: "x.y.z"}, appidentity.OAuthInvalidSoftwareStatement},
		"http client_uri":     {appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}, ClientURI: "http://dynamic.example/"}, appidentity.OAuthInvalidClientMetadata},
	} {
		_, err := s.RegisterDynamicClient(ctx, tc.req)
		wantOAuthError(t, err, tc.code, name)
	}

	// The instance setting turns registration off.
	if _, err := e.dynamicService(false).RegisterDynamicClient(ctx, appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}}); !errors.Is(err, appidentity.ErrDynamicRegistrationDisabled) {
		t.Errorf("RegisterDynamicClient(disabled) = %v, want ErrDynamicRegistrationDisabled", err)
	}
	if _, err := e.identity.RegisterDynamicClient(ctx, appidentity.DynamicRegistrationRequest{RedirectURIs: []string{dynamicRedirect}}); !errors.Is(err, appidentity.ErrDynamicRegistrationDisabled) {
		t.Errorf("RegisterDynamicClient(no setting) = %v, want ErrDynamicRegistrationDisabled", err)
	}
}

func oauthDeletesUnusedDynamicClients(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	userID := e.register(t, "alice").User.ID
	s := e.dynamicService(true)
	day := 24 * time.Hour

	wantKnown := func(clientID string, known bool, what string) {
		t.Helper()
		_, err := s.ValidateAuthorization(ctx, appidentity.AuthorizationRequest{
			ResponseType: "code", ClientID: clientID, RedirectURI: dynamicRedirect,
			CodeChallenge: identity.S256Challenge(contractCodeVerifier), CodeChallengeMethod: "S256",
		})
		if known != (err == nil) {
			t.Errorf("%s: ValidateAuthorization = %v, want known = %v", what, err, known)
		}
	}
	sweep := func(want int, what string) {
		t.Helper()
		for i := range 2 { // safe to repeat: the second run finds nothing
			n, err := s.DeleteUnusedDynamicClients(ctx)
			if i == 1 {
				want = 0
			}
			if err != nil || n != want {
				t.Fatalf("%s: DeleteUnusedDynamicClients (run %d) = %d, %v; want %d", what, i+1, n, err, want)
			}
		}
	}

	third := e.thirdPartyApp(t) // a user's client is never swept
	never := e.registerDynamic(t, s).Client
	used := e.registerDynamic(t, s).Client
	usedTokens := e.dynamicTokens(t, s, used.ClientID, userID)
	active := e.registerDynamic(t, s).Client // its token is used 89 days in
	activeTokens := e.dynamicTokens(t, s, active.ClientID, userID)
	revoked := e.registerDynamic(t, s).Client
	revokedTokens := e.dynamicTokens(t, s, revoked.ClientID, userID)
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Identity.RevokeTokenFamily(ctx, tx, revokedTokens.Token.FamilyID, identity.TokenRevokedUser, e.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}

	sweep(0, "right after the registration")

	e.clock.Advance(25 * time.Hour)
	fresh := e.registerDynamic(t, s).Client
	sweep(1, "after a day: the client that was never used")
	wantKnown(never.ClientID, false, "the unused client")
	wantKnown(used.ClientID, true, "the client with a token")
	wantKnown(fresh.ClientID, true, "the client registered just now")

	// The deletion is an event with the reason unused, and the client works no more.
	state, err := e.h.Identity.FindOAuthClientState(ctx, never.ID)
	if err != nil || !state.Deleted {
		t.Errorf("FindOAuthClientState(unused client) = %+v, %v; want deleted", state, err)
	}

	e.clock.Advance(88 * day) // 89 days after the first registration
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Identity.UpsertTokenFamilyLastUsed(ctx, tx, activeTokens.Token.FamilyID, e.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	sweep(1, "after 89 days: the client that was never used since a day")
	wantKnown(fresh.ClientID, false, "the client never used")
	wantKnown(used.ClientID, true, "the client with tokens issued 89 days ago")
	wantKnown(revoked.ClientID, true, "the client with revoked tokens issued 89 days ago")

	e.clock.Advance(2 * day) // 91 days
	sweep(2, "after 91 days: the clients idle for 90 days")
	wantKnown(used.ClientID, false, "the idle client")
	wantKnown(revoked.ClientID, false, "the idle client with revoked tokens")
	wantKnown(active.ClientID, true, "the client whose token family was used 2 days ago")
	e.wantAccess(t, usedTokens.AccessToken, false, "the access token of a deleted client")
	if _, _, err := e.h.Identity.FindOAuthClient(ctx, third.ClientID); err != nil {
		t.Errorf("FindOAuthClient(user's client) = %v; the clean-up must keep it", err)
	}
	if _, err := e.identity.ValidateAuthorization(ctx, appAuthorizationRequest()); err != nil {
		t.Errorf("ValidateAuthorization(user's client) = %v; the clean-up must keep it", err)
	}
}
