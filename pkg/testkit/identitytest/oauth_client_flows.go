// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"errors"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// The OAuth client cases of AccountRepositoryContract: the disablement and the deletion of a
// client (ADR-0090 §1).

// AppTokensFor exchanges a code of the third-party app for tokens of the user.
func (e *AccountEnv) AppTokensFor(t *testing.T, userID string) appidentity.IssuedTokens {
	t.Helper()
	got, err := e.Identity.ExchangeAuthorizationCode(t.Context(), appidentity.CodeExchange{
		ClientCredentials: appCreds, Code: e.code(t, appAuthorizationRequest(), userID),
		RedirectURI: contractAppRedirect, CodeVerifier: ContractCodeVerifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode(app) = %v", err)
	}
	return got
}

func appAuthorizationRequest() appidentity.AuthorizationRequest {
	return appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: contractAppClientID, RedirectURI: contractAppRedirect,
		CodeChallenge: identity.S256Challenge(ContractCodeVerifier), CodeChallengeMethod: "S256",
	}
}

// wantClientRefused checks that the app cannot be authorized, exchange its pending code,
// refresh got, or use the access token of got.
func (e *AccountEnv) wantClientRefused(t *testing.T, got appidentity.IssuedTokens, pendingCode, what string) {
	t.Helper()
	ctx := t.Context()
	if _, err := e.Identity.ValidateAuthorization(ctx, appAuthorizationRequest()); !errors.Is(err, appidentity.ErrUnknownClient) {
		t.Errorf("ValidateAuthorization(%s) = %v, want ErrUnknownClient", what, err)
	}
	_, err := e.Identity.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
		ClientCredentials: appCreds, Code: pendingCode, RedirectURI: contractAppRedirect, CodeVerifier: ContractCodeVerifier,
	})
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "exchanging a code of "+what)
	_, err = e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: appCreds, RefreshToken: got.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "refreshing a token of "+what)
	e.WantAccess(t, got.AccessToken, false, "the access token of "+what)
}

// wantFamiliesRevoked checks that every token family of tokens has a revocation.
func (e *AccountEnv) wantFamiliesRevoked(t *testing.T, what string, tokens ...appidentity.IssuedTokens) {
	t.Helper()
	for _, tok := range tokens {
		if active, err := e.Identity.TokenFamilyActive(t.Context(), tok.Token.FamilyID); err != nil || active {
			t.Errorf("TokenFamilyActive(%s) = %v, %v; want false", what, active, err)
		}
		if _, state, err := e.H.Identity.FindAccessTokenByAccessHash(t.Context(), tok.Token.AccessTokenHash); err != nil || !state.Revoked {
			t.Errorf("the token of %s: revoked = %v, %v; want true", what, state.Revoked, err)
		}
	}
}

func oauthDisablesAndDeletesClients(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	userID := e.Register(t, "alice").User.ID
	app := e.ThirdPartyApp(t)
	web, _ := e.WebTokensFor(t, userID)
	webClient, _, err := e.H.Identity.FindOAuthClient(ctx, appidentity.DefaultWebClientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Identity.DisableOAuthClient(ctx, webClient.ID, ""); !errors.Is(err, appidentity.ErrFirstPartyClient) {
		t.Errorf("DisableOAuthClient(the web client) = %v, want ErrFirstPartyClient", err)
	}
	if err := e.Identity.DeleteOAuthClient(ctx, webClient.ID, identity.ClientDeletedByOwner, userID); !errors.Is(err, appidentity.ErrFirstPartyClient) {
		t.Errorf("DeleteOAuthClient(the web client) = %v, want ErrFirstPartyClient", err)
	}
	if err := e.Identity.DisableOAuthClient(ctx, testkit.NamedID("unknown"), ""); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("DisableOAuthClient(unknown) = %v, want ErrNotFound", err)
	}

	// The disablement event alone refuses the client, before any revocation.
	first := e.AppTokensFor(t, userID)
	pending := e.code(t, appAuthorizationRequest(), userID)
	err = e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.H.Identity.InsertOAuthClientDisablement(ctx, tx, identity.OAuthClientDisablement{
			ID: e.IDs.NewID(), ClientID: app.ID, DisabledAt: e.Clock.Now(),
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e.wantClientRefused(t, first, pending, "a client with a disablement")
	if err := e.Identity.EnableOAuthClient(ctx, app.ID, ""); err != nil {
		t.Fatalf("EnableOAuthClient = %v", err)
	}
	e.WantAccess(t, first.AccessToken, true, "a token of an enabled client")

	// Disabling revokes every family of the client and no other client's.
	second := e.AppTokensFor(t, userID)
	pending = e.code(t, appAuthorizationRequest(), userID)
	for range 2 { // safe to repeat
		if err := e.Identity.DisableOAuthClient(ctx, app.ID, userID); err != nil {
			t.Fatalf("DisableOAuthClient = %v", err)
		}
	}
	e.wantClientRefused(t, first, pending, "a disabled client")
	e.wantFamiliesRevoked(t, "a disabled client", first, second)
	e.WantAccess(t, web.AccessToken, true, "the web client's token")

	// Enabling the client again does not bring its tokens back.
	if err := e.Identity.EnableOAuthClient(ctx, app.ID, userID); err != nil {
		t.Fatalf("EnableOAuthClient = %v", err)
	}
	e.WantAccess(t, second.AccessToken, false, "a token revoked by the disablement")
	third := e.AppTokensFor(t, userID)
	e.WantAccess(t, third.AccessToken, true, "a token after the client was enabled")

	// The deletion event alone refuses the client too.
	pending = e.code(t, appAuthorizationRequest(), userID)
	err = e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.H.Identity.InsertOAuthClientDeletion(ctx, tx, identity.OAuthClientDeletion{
			ClientID: app.ID, Reason: identity.ClientDeletedByOwner, DeletedByUserID: userID, DeletedAt: e.Clock.Now(),
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e.wantClientRefused(t, third, pending, "a client with a deletion")

	// Deleting revokes every family; a deleted client stays refused even when enabled.
	for range 2 { // safe to repeat
		if err := e.Identity.DeleteOAuthClient(ctx, app.ID, identity.ClientDeletedUnused, ""); err != nil {
			t.Fatalf("DeleteOAuthClient = %v", err)
		}
	}
	e.wantFamiliesRevoked(t, "a deleted client", third)
	if err := e.Identity.EnableOAuthClient(ctx, app.ID, ""); err != nil {
		t.Fatalf("EnableOAuthClient(deleted) = %v", err)
	}
	e.wantClientRefused(t, third, pending, "a deleted client")
	e.WantAccess(t, web.AccessToken, true, "the web client's token")
}
