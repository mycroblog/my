// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The OAuth app cases of AccountRepositoryContract: the clients that users register for
// themselves, their secrets, and the tokens they issue to their owner (ADR-0090 §1, §2).

const contractOwnAppRedirect = "https://bot.example/callback"

// ownAppInput is a client that may read and write statuses.
func ownAppInput(confidential bool) appidentity.OAuthAppInput {
	return appidentity.OAuthAppInput{
		Name: "  My bot ", Website: "https://bot.example/", RedirectURIs: []string{contractOwnAppRedirect, "http://127.0.0.1/cb"},
		Scope: "write:statuses read", Confidential: confidential,
	}
}

func (e *accountEnv) registerOwnApp(t *testing.T, userID string, confidential bool) appidentity.RegisteredOAuthApp {
	t.Helper()
	reg, err := e.identity.RegisterOAuthApp(t.Context(), userID, ownAppInput(confidential))
	if err != nil {
		t.Fatalf("RegisterOAuthApp = %v", err)
	}
	return reg
}

// ownAppTokensFor exchanges a code of the app for tokens of the user, as a third-party app gets
// them through the consent screen.
func (e *accountEnv) ownAppTokensFor(t *testing.T, app appidentity.OAuthApp, creds appidentity.ClientCredentials, userID string) appidentity.IssuedTokens {
	t.Helper()
	req := appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: app.Client.ClientID, RedirectURI: contractOwnAppRedirect, Scope: "read",
		CodeChallenge: identity.S256Challenge(contractCodeVerifier), CodeChallengeMethod: "S256",
	}
	got, err := e.identity.ExchangeAuthorizationCode(t.Context(), appidentity.CodeExchange{
		ClientCredentials: creds, Code: e.code(t, req, userID), RedirectURI: contractOwnAppRedirect, CodeVerifier: contractCodeVerifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode(own app) = %v", err)
	}
	return got
}

func oauthAppsRegisterAndChange(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").User.ID
	bob := e.register(t, "bob").User.ID

	conf := e.registerOwnApp(t, alice, true)
	c := conf.Client
	if !strings.HasPrefix(conf.ClientSecret, appidentity.ClientSecretPrefix) || c.Name != "My bot" || c.Scope != "read write:statuses" ||
		c.Registration != identity.ClientRegisteredByUser || c.OwnerUserID != alice || c.FirstParty() {
		t.Errorf("RegisterOAuthApp = %+v with secret %q, want a confidential app of alice", c, conf.ClientSecret)
	}
	// The secret is returned once and stored as its SHA-256 digest only.
	stored, err := e.h.Identity.FindOAuthClientByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored.SecretHash, appidentity.HashToken(conf.ClientSecret)) || bytes.Contains(stored.SecretHash, []byte(conf.ClientSecret)) {
		t.Errorf("stored secret = %x, want the SHA-256 digest of the secret only", stored.SecretHash)
	}
	got, err := e.identity.OAuthApp(ctx, alice, c.ID)
	if err != nil || got.Client.SecretHash == nil || len(got.RedirectURIs) != 2 || got.Disabled {
		t.Errorf("OAuthApp = %+v, %v; want the app with its redirect URIs", got, err)
	}
	if _, err := e.identity.OAuthApp(ctx, bob, c.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("OAuthApp(another user) = %v, want ErrNotFound", err)
	}

	pub := e.registerOwnApp(t, alice, false)
	if pub.ClientSecret != "" || !pub.Client.Public() {
		t.Errorf("RegisterOAuthApp(public) = %+v with secret %q, want no secret", pub.Client, pub.ClientSecret)
	}
	if list, err := e.identity.OAuthApps(ctx, alice); err != nil || len(list) != 2 || list[0].Client.ID != c.ID || list[1].Client.ID != pub.Client.ID {
		t.Errorf("OAuthApps = %+v, %v; want both apps, oldest first", list, err)
	}
	if list, err := e.identity.OAuthApps(ctx, bob); err != nil || len(list) != 0 {
		t.Errorf("OAuthApps(bob) = %+v, %v; want none", list, err)
	}

	for _, tc := range []struct {
		name string
		edit func(*appidentity.OAuthAppInput)
		want error
	}{
		{"empty name", func(in *appidentity.OAuthAppInput) { in.Name = " " }, identity.ErrInvalidOAuthClientName},
		{"website without a host", func(in *appidentity.OAuthAppInput) { in.Website = "https:///x" }, identity.ErrInvalidOAuthClientWebsite},
		{"localhost redirect", func(in *appidentity.OAuthAppInput) { in.RedirectURIs = []string{"http://localhost/cb"} }, identity.ErrInvalidOAuthClientRedirectURIs},
		{"no redirect", func(in *appidentity.OAuthAppInput) { in.RedirectURIs = nil }, identity.ErrInvalidOAuthClientRedirectURIs},
		{"first-party-only scope", func(in *appidentity.OAuthAppInput) { in.Scope = "read admin:read" }, identity.ErrInvalidOAuthClientScope},
		{"empty scope", func(in *appidentity.OAuthAppInput) { in.Scope = "" }, identity.ErrInvalidOAuthClientScope},
	} {
		in := ownAppInput(true)
		tc.edit(&in)
		if _, err := e.identity.RegisterOAuthApp(ctx, alice, in); !errors.Is(err, tc.want) {
			t.Errorf("RegisterOAuthApp(%s) = %v, want %v", tc.name, err, tc.want)
		}
	}

	// The consent screen names the user who registered the app.
	a, err := e.identity.ValidateAuthorization(ctx, appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: c.ClientID, RedirectURI: contractOwnAppRedirect,
		CodeChallenge: identity.S256Challenge(contractCodeVerifier), CodeChallengeMethod: "S256",
	})
	if err != nil || a.OwnerUsername != "alice" {
		t.Errorf("ValidateAuthorization = owner %q, %v; want alice", a.OwnerUsername, err)
	}

	// Another user authorizes the app; the client authenticates with its secret.
	creds := appidentity.ClientCredentials{ClientID: c.ClientID, ClientSecret: conf.ClientSecret}
	bobTokens := e.ownAppTokensFor(t, conf.OAuthApp, creds, bob)
	e.wantAccess(t, bobTokens.AccessToken, true, "a token of a user-registered app")

	// A change applies to the client at once and keeps its tokens.
	name, website, scope := "Renamed bot", "", "read"
	changed, err := e.identity.UpdateOAuthApp(ctx, alice, c.ID, appidentity.OAuthAppChanges{
		Name: &name, Website: &website, Scope: &scope, RedirectURIs: []string{"com.example.bot:/cb"},
	})
	if err != nil {
		t.Fatalf("UpdateOAuthApp = %v", err)
	}
	if changed.Client.Name != name || changed.Client.Website != "" || changed.Client.Scope != "read" ||
		len(changed.RedirectURIs) != 1 || changed.RedirectURIs[0] != "com.example.bot:/cb" {
		t.Errorf("UpdateOAuthApp = %+v, want the changes", changed)
	}
	found, uris, err := e.h.Identity.FindOAuthClient(ctx, c.ClientID)
	if err != nil || found.Name != name || found.Website != "" || found.Scope != "read" || len(uris) != 1 ||
		!bytes.Equal(found.SecretHash, stored.SecretHash) || found.Public() {
		t.Errorf("FindOAuthClient after the change = %+v %v, %v; want the change with the same secret", found, uris, err)
	}
	e.wantAccess(t, bobTokens.AccessToken, true, "a token of a changed app")
	bad := "x admin"
	if _, err := e.identity.UpdateOAuthApp(ctx, alice, c.ID, appidentity.OAuthAppChanges{Scope: &bad}); !errors.Is(err, identity.ErrInvalidOAuthClientScope) {
		t.Errorf("UpdateOAuthApp(bad scope) = %v, want ErrInvalidOAuthClientScope", err)
	}
	if _, err := e.identity.UpdateOAuthApp(ctx, bob, c.ID, appidentity.OAuthAppChanges{Name: &name}); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("UpdateOAuthApp(another user) = %v, want ErrNotFound", err)
	}

	// A new secret replaces the old one at once.
	if _, err := e.identity.RotateOAuthAppSecret(ctx, alice, pub.Client.ID); !errors.Is(err, appidentity.ErrPublicOAuthApp) {
		t.Errorf("RotateOAuthAppSecret(public) = %v, want ErrPublicOAuthApp", err)
	}
	secret, err := e.identity.RotateOAuthAppSecret(ctx, alice, c.ID)
	if err != nil || !strings.HasPrefix(secret, appidentity.ClientSecretPrefix) || secret == conf.ClientSecret {
		t.Fatalf("RotateOAuthAppSecret = %q, %v; want a new secret", secret, err)
	}
	_, err = e.identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: creds, RefreshToken: bobTokens.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "refreshing with the old secret")
	creds.ClientSecret = secret
	refreshed, err := e.identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: creds, RefreshToken: bobTokens.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshAccessToken(new secret) = %v", err)
	}
	if again, err := e.h.Identity.FindOAuthClientByID(ctx, c.ID); err != nil || !bytes.Equal(again.SecretHash, appidentity.HashToken(secret)) {
		t.Errorf("stored secret after the rotation = %x, %v; want the digest of the new secret", again.SecretHash, err)
	}

	// Deleting the app revokes its tokens, also those of other users.
	if err := e.identity.DeleteOwnOAuthApp(ctx, bob, c.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("DeleteOwnOAuthApp(another user) = %v, want ErrNotFound", err)
	}
	if err := e.identity.DeleteOwnOAuthApp(ctx, alice, c.ID); err != nil {
		t.Fatalf("DeleteOwnOAuthApp = %v", err)
	}
	e.wantAccess(t, refreshed.AccessToken, false, "a token of a deleted app")
	if _, err := e.identity.OAuthApp(ctx, alice, c.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("OAuthApp(deleted) = %v, want ErrNotFound", err)
	}
	if err := e.identity.DeleteOwnOAuthApp(ctx, alice, c.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("DeleteOwnOAuthApp(deleted) = %v, want ErrNotFound", err)
	}
	if list, err := e.identity.OAuthApps(ctx, alice); err != nil || len(list) != 1 || list[0].Client.ID != pub.Client.ID {
		t.Errorf("OAuthApps after the deletion = %+v, %v; want the public app only", list, err)
	}
}

func oauthAppsIssueTokensToTheirOwner(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").User.ID
	bob := e.register(t, "bob").User.ID
	reg := e.registerOwnApp(t, alice, true)
	id := reg.Client.ID
	creds := appidentity.ClientCredentials{ClientID: reg.Client.ClientID, ClientSecret: reg.ClientSecret}

	_, err := e.identity.IssueOAuthAppTokens(ctx, alice, id, "read write")
	wantOAuthError(t, err, appidentity.OAuthInvalidScope, "tokens beyond the app's scope")
	if _, err := e.identity.IssueOAuthAppTokens(ctx, bob, id, ""); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("IssueOAuthAppTokens(another user) = %v, want ErrNotFound", err)
	}
	whole, err := e.identity.IssueOAuthAppTokens(ctx, alice, id, "")
	if err != nil || whole.Token.Scope != reg.Client.Scope || whole.Token.UserID != alice {
		t.Errorf("IssueOAuthAppTokens(whole scope) = %+v, %v; want the app's scope for alice", whole.Token, err)
	}
	got, err := e.identity.IssueOAuthAppTokens(ctx, alice, id, "read:statuses")
	if err != nil {
		t.Fatalf("IssueOAuthAppTokens = %v", err)
	}
	if got.Token.Scope != "read:statuses" || got.Token.ClientID != id || got.Token.ExpiresAt.Sub(got.Token.IssuedAt) != appidentity.AccessTokenTTL ||
		got.Token.RefreshExpiresAt.Sub(got.Token.IssuedAt) != appidentity.RefreshTokenTTL || got.RefreshToken == "" {
		t.Errorf("IssueOAuthAppTokens = %+v, want a usual pair of the app for alice", got.Token)
	}
	bearer, err := e.identity.AuthenticateBearer(ctx, got.AccessToken)
	if err != nil || bearer.UserID != alice || bearer.FirstParty {
		t.Errorf("AuthenticateBearer = %+v, %v; want alice through a third-party app", bearer, err)
	}

	// The refresh token rotates, and a used one revokes the whole family.
	rotated, err := e.identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: creds, RefreshToken: got.RefreshToken})
	if err != nil || rotated.Token.FamilyID != got.Token.FamilyID || rotated.Token.RotatedFromTokenID != got.Token.ID {
		t.Fatalf("RefreshAccessToken = %+v, %v; want a rotation in the family", rotated.Token, err)
	}
	e.wantAccess(t, got.AccessToken, false, "the access token a refresh replaced")
	e.wantAccess(t, rotated.AccessToken, true, "the rotated access token")
	_, err = e.identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: creds, RefreshToken: got.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "reusing a refresh token")
	e.wantAccess(t, rotated.AccessToken, false, "the access token of a family with a reused refresh token")
	e.wantAccess(t, whole.AccessToken, true, "the access token of another family")

	// A disabled app issues no tokens.
	if err := e.identity.DisableOAuthClient(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if app, err := e.identity.OAuthApp(ctx, alice, id); err != nil || !app.Disabled {
		t.Errorf("OAuthApp(disabled) = %+v, %v; want it disabled", app, err)
	}
	if _, err := e.identity.IssueOAuthAppTokens(ctx, alice, id, ""); !errors.Is(err, appidentity.ErrOAuthAppDisabled) {
		t.Errorf("IssueOAuthAppTokens(disabled) = %v, want ErrOAuthAppDisabled", err)
	}
}

func oauthAppsLimitRegistrations(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").User.ID
	var first appidentity.RegisteredOAuthApp
	for i := range identity.MaxOwnedOAuthClients {
		reg := e.registerOwnApp(t, alice, false)
		if i == 0 {
			first = reg
		}
	}
	if _, err := e.identity.RegisterOAuthApp(ctx, alice, ownAppInput(false)); !errors.Is(err, appidentity.ErrTooManyOAuthApps) {
		t.Errorf("RegisterOAuthApp(26th) = %v, want ErrTooManyOAuthApps", err)
	}
	if err := e.identity.DeleteOwnOAuthApp(ctx, alice, first.Client.ID); err != nil {
		t.Fatal(err)
	}
	e.registerOwnApp(t, alice, false)

	off := e.newIdentityWith(func(d *appidentity.Deps) {
		d.UserAppRegistration = func(context.Context) (bool, error) { return false, nil }
	})
	bob := e.register(t, "bob").User.ID
	if _, err := off.RegisterOAuthApp(ctx, bob, ownAppInput(true)); !errors.Is(err, appidentity.ErrOAuthAppRegistrationDisabled) {
		t.Errorf("RegisterOAuthApp(setting off) = %v, want ErrOAuthAppRegistrationDisabled", err)
	}
	if list, err := off.OAuthApps(ctx, alice); err != nil || len(list) != identity.MaxOwnedOAuthClients {
		t.Errorf("OAuthApps(setting off) = %d apps, %v; want the apps registered before", len(list), err)
	}
}

func oauthAppsStopWithTheirOwner(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	owner := e.register(t, "alice")
	bob := e.register(t, "bob").User.ID
	reg := e.registerOwnApp(t, owner.User.ID, true)
	creds := appidentity.ClientCredentials{ClientID: reg.Client.ClientID, ClientSecret: reg.ClientSecret}
	tokens := e.ownAppTokensFor(t, reg.OAuthApp, creds, bob)

	// While moderation forbids the owner to log in, the app cannot be authorized or refresh.
	restricted := e.newIdentityWith(func(d *appidentity.Deps) { d.Logins = loginPolicy{owner.AccountID: true} })
	req := appidentity.AuthorizationRequest{
		ResponseType: "code", ClientID: reg.Client.ClientID, RedirectURI: contractOwnAppRedirect,
		CodeChallenge: identity.S256Challenge(contractCodeVerifier), CodeChallengeMethod: "S256",
	}
	if _, err := restricted.ValidateAuthorization(ctx, req); !errors.Is(err, appidentity.ErrUnknownClient) {
		t.Errorf("ValidateAuthorization(app of a restricted owner) = %v, want ErrUnknownClient", err)
	}
	_, err := restricted.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: creds, RefreshToken: tokens.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "refreshing a token of an app of a restricted owner")

	// The projectors of moderation and account deletion revoke the tokens of the owner's apps.
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Identity.RevokeTokensOfOwnedClients(ctx, tx, owner.User.ID, identity.TokenRevokedClientDisabled, e.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	e.wantAccess(t, tokens.AccessToken, false, "a token of an app of a restricted owner")
	other := e.registerOwnApp(t, bob, false)
	if _, err := e.identity.OAuthApp(ctx, bob, other.Client.ID); err != nil {
		t.Errorf("OAuthApp(another owner's app) = %v", err)
	}
}
