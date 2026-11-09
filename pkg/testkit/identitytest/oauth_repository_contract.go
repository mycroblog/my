// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract on the OAuth 2.0 authorization server's persistence in
// the identity repository: clients, authorization codes, and tokens, and the disablement and
// deletion of a client with the revocation of its tokens, which the erasure of an account also
// uses (identity.ErasureRepository).

// oauthClient ensures a client registered by owner with one redirect URI and returns it.
func (r *identityRepo) oauthClient(t *testing.T, owner string) identity.OAuthClient {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	c := identity.OAuthClient{
		ID: id, ClientID: "client-" + id, SecretHash: []byte("secret of " + id), Name: "App " + id, Website: "https://app.example",
		Scope: identity.Scope("read write"), Registration: identity.ClientRegisteredByUser, OwnerUserID: owner, CreatedAt: r.clock.Now(),
	}
	var stored identity.OAuthClient
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if stored, err = r.h.Identity.EnsureOAuthClient(ctx, tx, c); err != nil {
			return err
		}
		return r.h.Identity.ReplaceOAuthClientRedirectURIs(ctx, tx, stored.ID, []string{"https://app.example/callback"})
	})
	return stored
}

// token issues a token of a new family of client to user, valid for an hour with a refresh token
// for a day, and returns it.
func (r *identityRepo) token(t *testing.T, clientID, userID string) identity.AccessToken {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	tok := identity.AccessToken{
		ID: id, FamilyID: r.ids.NewID(), ClientID: clientID, UserID: userID,
		AccessTokenHash: []byte("access " + id), RefreshTokenHash: []byte("refresh " + id), Scope: identity.Scope("read"),
		IssuedAt: r.clock.Now(), ExpiresAt: r.clock.Now().Add(time.Hour), RefreshExpiresAt: r.clock.Now().Add(24 * time.Hour),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.LockTokenGrants(ctx, tx, userID); err != nil {
			return err
		}
		return r.h.Identity.InsertAccessToken(ctx, tx, tok)
	})
	return tok
}

// tokenRevoked reports whether a token has a revocation.
func (r *identityRepo) tokenRevoked(t *testing.T, tok identity.AccessToken) bool {
	t.Helper()
	_, state, err := r.h.Identity.FindAccessTokenByAccessHash(t.Context(), tok.AccessTokenHash)
	if err != nil {
		t.Fatalf("FindAccessTokenByAccessHash = %v", err)
	}
	return state.Revoked
}

func oauthRepoClients(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	c := r.oauthClient(t, alice)
	again := c
	again.ID, again.Name = r.ids.NewID(), "Another name"
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.h.Identity.EnsureOAuthClient(ctx, tx, again)
		if err == nil && (got.ID != c.ID || got.Name != c.Name) {
			t.Errorf("EnsureOAuthClient of a taken client_id = %+v, want the stored %+v", got, c)
		}
		return err
	})
	got, uris, err := r.h.Identity.FindOAuthClient(ctx, c.ClientID)
	if err != nil || got.ID != c.ID || got.OwnerUserID != alice || got.Registration != identity.ClientRegisteredByUser || got.Scope != c.Scope ||
		!slices.Equal(got.SecretHash, c.SecretHash) || !slices.Equal(uris, []string{"https://app.example/callback"}) {
		t.Errorf("FindOAuthClient = %+v, %v, %v", got, uris, err)
	}
	if got, err := r.h.Identity.FindOAuthClientByID(ctx, c.ID); err != nil || got.ClientID != c.ClientID {
		t.Errorf("FindOAuthClientByID = %+v, %v", got, err)
	}
	if got, err := r.h.Identity.ExistingOAuthClients(ctx, []string{c.ID, r.ids.NewID()}); err != nil || !maps.Equal(got, map[string]bool{c.ID: true}) {
		t.Errorf("ExistingOAuthClients = %v, %v; want the client alone", got, err)
	}
	if _, _, err := r.h.Identity.FindOAuthClient(ctx, "unknown"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindOAuthClient(unknown) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Identity.ReplaceOAuthClientRedirectURIs(ctx, tx, c.ID, []string{"https://b.example/cb", "https://a.example/cb"})
	})
	if _, uris, err := r.h.Identity.FindOAuthClient(ctx, c.ClientID); err != nil || !slices.Equal(uris, []string{"https://a.example/cb", "https://b.example/cb"}) {
		t.Errorf("redirect URIs after the replacement = %v, %v; want both, sorted", uris, err)
	}

	// An authorization code is exchanged once.
	code := identity.AuthorizationCode{
		CodeHash: []byte("code"), ClientID: c.ID, UserID: alice, RedirectURI: "https://a.example/cb", Scope: identity.Scope("read"),
		CodeChallenge: "challenge", IssuedAt: r.clock.Now(), ExpiresAt: r.clock.Now().Add(time.Minute),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertAuthorizationCode(ctx, tx, code) })
	if got, err := r.h.Identity.FindAuthorizationCode(ctx, code.CodeHash); err != nil || got.ClientID != c.ID || got.CodeChallenge != "challenge" || !got.ExpiresAt.Equal(code.ExpiresAt) {
		t.Errorf("FindAuthorizationCode = %+v, %v", got, err)
	}
	if _, err := r.h.Identity.FindAuthorizationCodeRedemption(ctx, code.CodeHash); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindAuthorizationCodeRedemption before the exchange = %v, want ErrNotFound", err)
	}
	tok := r.token(t, c.ID, alice)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			ok, err := r.h.Identity.InsertAuthorizationCodeRedemption(ctx, tx, identity.AuthorizationCodeRedemption{CodeHash: code.CodeHash, TokenID: tok.ID, RedeemedAt: r.clock.Now()})
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("InsertAuthorizationCodeRedemption #%d = %t, want %t", i+1, ok, want)
			}
		}
		return nil
	})
	if got, err := r.h.Identity.FindAuthorizationCodeRedemption(ctx, code.CodeHash); err != nil || got.TokenID != tok.ID {
		t.Errorf("FindAuthorizationCodeRedemption = %+v, %v", got, err)
	}

	// Tokens are found by either hash, and a revocation keeps its first reason.
	got2, state, err := r.h.Identity.FindAccessTokenByRefreshHash(ctx, tok.RefreshTokenHash)
	if err != nil || got2.ID != tok.ID || got2.FamilyID != tok.FamilyID || state.Revoked || !state.Client.Usable() {
		t.Errorf("FindAccessTokenByRefreshHash = %+v, %+v, %v; want the usable token", got2, state, err)
	}
	if active, err := r.h.Identity.TokenFamilyActive(ctx, tok.FamilyID, r.clock.Now()); err != nil || !active {
		t.Errorf("TokenFamilyActive = %t, %v; want true", active, err)
	}
	if apps, err := r.h.Identity.ListAuthorizedAppTokens(ctx, alice, r.clock.Now()); err != nil || len(apps) != 1 {
		t.Errorf("ListAuthorizedAppTokens = %+v, %v; want one", apps, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			ok, err := r.h.Identity.InsertTokenRevocation(ctx, tx, identity.TokenRevocation{TokenID: tok.ID, Reason: identity.TokenRevokedUser, RevokedAt: r.clock.Now()})
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("InsertTokenRevocation #%d = %t, want %t", i+1, ok, want)
			}
		}
		return nil
	})
	if !r.tokenRevoked(t, tok) {
		t.Error("the token is not revoked")
	}
	if active, err := r.h.Identity.TokenFamilyActive(ctx, tok.FamilyID, r.clock.Now()); err != nil || active {
		t.Errorf("TokenFamilyActive after the revocation = %t, %v; want false", active, err)
	}

	// The revocations of a user's tokens.
	keep, other := r.token(t, c.ID, alice), r.token(t, c.ID, alice)
	bob := r.user(t)
	bobs := r.token(t, c.ID, bob)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Identity.RevokeOtherTokensOfUser(ctx, tx, alice, keep.FamilyID, identity.TokenRevokedPasswordChanged, r.clock.Now())
	})
	if r.tokenRevoked(t, keep) || !r.tokenRevoked(t, other) || r.tokenRevoked(t, bobs) {
		t.Error("RevokeOtherTokensOfUser: want only alice's other family revoked")
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Identity.RevokeTokensOfUser(ctx, tx, alice, identity.TokenRevokedModeration, r.clock.Now())
	})
	if !r.tokenRevoked(t, keep) || r.tokenRevoked(t, bobs) {
		t.Error("RevokeTokensOfUser: want every token of alice revoked, and bob's kept")
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		n, err := r.h.Identity.RevokeTokensOfUserAndClient(ctx, tx, bob, c.ID, identity.TokenRevokedUser, r.clock.Now())
		if err == nil && n != 1 {
			t.Errorf("RevokeTokensOfUserAndClient = %d, want 1", n)
		}
		return err
	})
	if !r.tokenRevoked(t, bobs) {
		t.Error("RevokeTokensOfUserAndClient left bob's token")
	}
}

func oauthRepoDisablement(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	c, other := r.oauthClient(t, alice), r.oauthClient(t, alice)
	live := r.token(t, c.ID, alice)
	expired := identity.AccessToken{
		ID: r.ids.NewID(), FamilyID: r.ids.NewID(), ClientID: c.ID, UserID: alice, AccessTokenHash: []byte("expired access"),
		RefreshTokenHash: []byte("expired refresh"), Scope: identity.Scope("read"),
		IssuedAt: r.clock.Now().Add(-48 * time.Hour), ExpiresAt: r.clock.Now().Add(-47 * time.Hour), RefreshExpiresAt: r.clock.Now().Add(-time.Hour),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertAccessToken(ctx, tx, expired) })
	otherToken := r.token(t, other.ID, alice)
	if s, err := r.h.Identity.FindOAuthClientState(ctx, c.ID); err != nil || s != (identity.OAuthClientState{}) {
		t.Errorf("FindOAuthClientState = %+v, %v; want usable", s, err)
	}
	if _, err := r.h.Identity.FindOAuthClientState(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindOAuthClientState(unknown) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			ok, err := r.h.Identity.InsertOAuthClientDisablement(ctx, tx, identity.OAuthClientDisablement{ID: r.ids.NewID(), ClientID: c.ID, DisabledByUserID: alice, DisabledAt: r.clock.Now()})
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("InsertOAuthClientDisablement #%d = %t, want %t", i+1, ok, want)
			}
		}
		return r.h.Identity.RevokeTokensOfClient(ctx, tx, c.ID, identity.TokenRevokedClientDisabled, r.clock.Now())
	})
	if s, err := r.h.Identity.FindOAuthClientState(ctx, c.ID); err != nil || !s.Disabled || s.Deleted {
		t.Errorf("FindOAuthClientState after the disablement = %+v, %v; want disabled", s, err)
	}
	if !r.tokenRevoked(t, live) || r.tokenRevoked(t, otherToken) {
		t.Error("RevokeTokensOfClient: want the client's live token revoked and the other client's kept")
	}
	if _, state, err := r.h.Identity.FindAccessTokenByAccessHash(ctx, expired.AccessTokenHash); err != nil || state.Revoked || !state.Client.Disabled {
		t.Errorf("expired token = %+v, %v; want it without a revocation, its client disabled", state, err)
	}
	// Many tokens at once, as each.
	byHash, err := r.h.Identity.FindAccessTokensByAccessHashes(ctx, [][]byte{expired.AccessTokenHash, live.AccessTokenHash, []byte("unknown")})
	if err != nil || len(byHash) != 2 {
		t.Errorf("FindAccessTokensByAccessHashes = %+v, %v; want the two tokens", byHash, err)
	}
	for _, tok := range []identity.AccessToken{expired, live} {
		one, state, err := r.h.Identity.FindAccessTokenByAccessHash(ctx, tok.AccessTokenHash)
		got := byHash[string(tok.AccessTokenHash)]
		if err != nil || !reflect.DeepEqual(got.Token, one) || got.State != state {
			t.Errorf("FindAccessTokensByAccessHashes[%s] = %+v; want %+v %+v, as FindAccessTokenByAccessHash (%v)", tok.ID, got, one, state, err)
		}
	}
	// Lifting enables the client again; a second lift finds nothing.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			ok, err := r.h.Identity.LiftOAuthClientDisablements(ctx, tx, c.ID, alice, r.clock.Now())
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("LiftOAuthClientDisablements #%d = %t, want %t", i+1, ok, want)
			}
		}
		return nil
	})
	if s, err := r.h.Identity.FindOAuthClientState(ctx, c.ID); err != nil || s.Disabled {
		t.Errorf("FindOAuthClientState after the lift = %+v, %v; want enabled", s, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			ok, err := r.h.Identity.InsertOAuthClientDeletion(ctx, tx, identity.OAuthClientDeletion{ClientID: other.ID, Reason: identity.ClientDeletedByOwner, DeletedByUserID: alice, DeletedAt: r.clock.Now()})
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("InsertOAuthClientDeletion #%d = %t, want %t", i+1, ok, want)
			}
		}
		return nil
	})
	if s, err := r.h.Identity.FindOAuthClientState(ctx, other.ID); err != nil || !s.Deleted {
		t.Errorf("FindOAuthClientState after the deletion = %+v, %v; want deleted", s, err)
	}
	list, err := r.h.Identity.ListOAuthClients(ctx, identity.ClientRegisteredByUser, "", 10)
	if err != nil || len(list) != 1 || list[0].ID != c.ID || list[0].Disabled {
		t.Errorf("ListOAuthClients = %+v, %v; want the client that is not deleted", list, err)
	}
}
