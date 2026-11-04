// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The OAuth clients that users register for themselves (ADR-0090 §1, §2): a signed-in user
// registers public and confidential clients, changes and deletes them, issues a confidential
// client a new secret, and issues tokens of a client to their own account without a redirect
// (for bots and scripts, ADR-0093). A client stops working while the user who registered it
// cannot use the app.

// ClientSecretPrefix marks the secrets of confidential clients for secret scanners
// (ADR-0019 §4).
const ClientSecretPrefix = "my_cs_"

// Errors of the use cases of the clients that users register.
var (
	// ErrOAuthAppRegistrationDisabled refuses a registration while the instance setting
	// user_app_registration is off.
	ErrOAuthAppRegistrationDisabled = errors.New("identity: users may not register OAuth apps")
	// ErrTooManyOAuthApps refuses a registration by a user who has identity.MaxOwnedOAuthClients.
	ErrTooManyOAuthApps = errors.New("identity: too many OAuth apps")
	// ErrPublicOAuthApp refuses a new secret for a public client, which has none.
	ErrPublicOAuthApp = errors.New("identity: a public OAuth app has no secret")
	// ErrOAuthAppDisabled refuses tokens of a client that an administrator disabled.
	ErrOAuthAppDisabled = errors.New("identity: the OAuth app is disabled")
)

// OAuthAppRepository is the persistence of the clients that users register (data-model §4.1).
// It is part of Repository. Every clientID argument is the ID of an oauth_clients row.
type OAuthAppRepository interface {
	// InsertOAuthClient inserts a new client.
	InsertOAuthClient(ctx context.Context, tx uow.Tx, c identity.OAuthClient) error
	// ListOwnedOAuthClients returns the clients the user registered that are not deleted,
	// oldest first, as their newest revision and secret make them.
	ListOwnedOAuthClients(ctx context.Context, ownerUserID string) ([]identity.OAuthClient, error)
	// CountOwnedOAuthClients counts the clients the user registered that are not deleted.
	CountOwnedOAuthClients(ctx context.Context, tx uow.Tx, ownerUserID string) (int, error)
	InsertOAuthClientRevision(ctx context.Context, tx uow.Tx, r identity.OAuthClientRevision) error
	InsertOAuthClientSecretRotation(ctx context.Context, tx uow.Tx, r identity.OAuthClientSecretRotation) error
	// RevokeTokensOfOwnedClients revokes every token of the clients the user registered whose
	// access or refresh token has not expired at at and that has no revocation yet.
	RevokeTokensOfOwnedClients(ctx context.Context, tx uow.Tx, ownerUserID string, reason identity.TokenRevocationReason, at time.Time) error
}

// OAuthApp is a client that the user registered, as its owner sees it.
type OAuthApp struct {
	Client       identity.OAuthClient
	RedirectURIs []string
	// Disabled is true while an administrator has disabled the client.
	Disabled bool
}

// OAuthAppInput is a new client: what identity.ValidateOAuthClientSettings checks, and whether
// the client is confidential, which never changes.
type OAuthAppInput struct {
	Name         string
	Website      string
	RedirectURIs []string
	Scope        string
	Confidential bool
}

// RegisteredOAuthApp is a new client with its secret, which is shown only now; "" for a public
// client.
type RegisteredOAuthApp struct {
	OAuthApp
	ClientSecret string
}

// OAuthAppChanges is what an owner changes of a client; a nil field stays.
type OAuthAppChanges struct {
	Name         *string
	Website      *string
	RedirectURIs []string
	Scope        *string
}

// userAppRegistration reports whether users may register clients (the instance setting
// user_app_registration); without the setting, they may.
func (s *Service) userAppRegistration(ctx context.Context) (bool, error) {
	if s.d.UserAppRegistration == nil {
		return true, nil
	}
	return s.d.UserAppRegistration(ctx)
}

// OAuthApps returns the clients the user registered and has not deleted, oldest first.
func (s *Service) OAuthApps(ctx context.Context, userID string) ([]OAuthApp, error) {
	clients, err := s.d.Repository.ListOwnedOAuthClients(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]OAuthApp, 0, len(clients))
	for _, c := range clients {
		app, err := s.oauthApp(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, app)
	}
	return out, nil
}

// OAuthApp returns the client with the ID (not its public client_id) that the user registered;
// ErrNotFound when there is none, it is deleted, or someone else registered it.
func (s *Service) OAuthApp(ctx context.Context, userID, id string) (OAuthApp, error) {
	c, err := s.ownedClient(ctx, userID, id)
	if err != nil {
		return OAuthApp{}, err
	}
	return s.oauthApp(ctx, c)
}

// ownedClient returns the client with the ID if the user registered it and it is not deleted.
func (s *Service) ownedClient(ctx context.Context, userID, id string) (identity.OAuthClient, error) {
	c, err := s.d.Repository.FindOAuthClientByID(ctx, id)
	if err != nil {
		return identity.OAuthClient{}, err
	}
	if !c.OwnedBy(userID) {
		return identity.OAuthClient{}, ErrNotFound
	}
	state, err := s.d.Repository.FindOAuthClientState(ctx, c.ID)
	if err != nil {
		return identity.OAuthClient{}, err
	}
	if state.Deleted {
		return identity.OAuthClient{}, ErrNotFound
	}
	return c, nil
}

// oauthApp returns c with its redirect URIs and whether it is disabled.
func (s *Service) oauthApp(ctx context.Context, c identity.OAuthClient) (OAuthApp, error) {
	_, uris, err := s.d.Repository.FindOAuthClient(ctx, c.ClientID)
	if err != nil {
		return OAuthApp{}, err
	}
	state, err := s.d.Repository.FindOAuthClientState(ctx, c.ID)
	if err != nil {
		return OAuthApp{}, err
	}
	return OAuthApp{Client: c, RedirectURIs: uris, Disabled: state.Disabled}, nil
}

// RegisterOAuthApp registers a client of the user (ADR-0090 §1). A confidential client gets a
// secret, returned only now; only its SHA-256 digest is stored (ADR-0019 §4). It fails with an
// error of identity.ValidateOAuthClientSettings for an input it rejects,
// ErrOAuthAppRegistrationDisabled while the instance setting is off, and ErrTooManyOAuthApps
// when the user has identity.MaxOwnedOAuthClients clients.
func (s *Service) RegisterOAuthApp(ctx context.Context, userID string, in OAuthAppInput) (RegisteredOAuthApp, error) {
	if allowed, err := s.userAppRegistration(ctx); err != nil {
		return RegisteredOAuthApp{}, err
	} else if !allowed {
		return RegisteredOAuthApp{}, ErrOAuthAppRegistrationDisabled
	}
	settings, err := identity.ValidateOAuthClientSettings(in.Name, in.Website, in.RedirectURIs, in.Scope)
	if err != nil {
		return RegisteredOAuthApp{}, err
	}
	now := s.d.Clock.Now()
	c := identity.OAuthClient{
		ID: s.d.IDs.NewID(), ClientID: newClientID(), Name: settings.Name, Website: settings.Website, Scope: settings.Scope,
		Registration: identity.ClientRegisteredByUser, OwnerUserID: userID, CreatedAt: now,
	}
	var secret string
	if in.Confidential {
		secret, c.SecretHash = NewToken(ClientSecretPrefix)
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		n, err := s.d.Repository.CountOwnedOAuthClients(ctx, tx, userID)
		if err != nil {
			return err
		}
		if n >= identity.MaxOwnedOAuthClients {
			return ErrTooManyOAuthApps
		}
		if err := s.d.Repository.InsertOAuthClient(ctx, tx, c); err != nil {
			return err
		}
		return s.d.Repository.ReplaceOAuthClientRedirectURIs(ctx, tx, c.ID, settings.RedirectURIs)
	})
	if err != nil {
		return RegisteredOAuthApp{}, err
	}
	return RegisteredOAuthApp{OAuthApp: OAuthApp{Client: c, RedirectURIs: settings.RedirectURIs}, ClientSecret: secret}, nil
}

// UpdateOAuthApp changes the client with the ID that the user registered: what changes names
// and keeps the rest, checked as by RegisterOAuthApp. Whether the client is confidential never
// changes. The tokens it issued keep working.
func (s *Service) UpdateOAuthApp(ctx context.Context, userID, id string, ch OAuthAppChanges) (OAuthApp, error) {
	app, err := s.OAuthApp(ctx, userID, id)
	if err != nil {
		return OAuthApp{}, err
	}
	c := app.Client
	name, website, uris, scope := c.Name, c.Website, app.RedirectURIs, string(c.Scope)
	if ch.Name != nil {
		name = *ch.Name
	}
	if ch.Website != nil {
		website = *ch.Website
	}
	if ch.RedirectURIs != nil {
		uris = ch.RedirectURIs
	}
	if ch.Scope != nil {
		scope = *ch.Scope
	}
	settings, err := identity.ValidateOAuthClientSettings(name, website, uris, scope)
	if err != nil {
		return OAuthApp{}, err
	}
	r := identity.OAuthClientRevision{
		ID: s.d.IDs.NewID(), ClientID: c.ID, Name: settings.Name, Website: settings.Website, Scope: settings.Scope,
		RevisedAt: s.d.Clock.Now(),
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertOAuthClientRevision(ctx, tx, r); err != nil {
			return err
		}
		return s.d.Repository.ReplaceOAuthClientRedirectURIs(ctx, tx, c.ID, settings.RedirectURIs)
	})
	if err != nil {
		return OAuthApp{}, err
	}
	app.Client, app.RedirectURIs = c.Revised(r), settings.RedirectURIs
	return app, nil
}

// DeleteOwnOAuthApp deletes the client with the ID that the user registered, with the reason
// owner, and revokes every token of it (ADR-0090 §1).
func (s *Service) DeleteOwnOAuthApp(ctx context.Context, userID, id string) error {
	c, err := s.ownedClient(ctx, userID, id)
	if err != nil {
		return err
	}
	return s.DeleteOAuthClient(ctx, c.ID, identity.ClientDeletedByOwner, userID)
}

// RotateOAuthAppSecret issues a new secret to the confidential client with the ID that the user
// registered and returns it; the old secret stops working at once (ADR-0090 §2). The tokens
// of the client keep working, but refreshing them takes the new secret. A public client fails
// with ErrPublicOAuthApp.
func (s *Service) RotateOAuthAppSecret(ctx context.Context, userID, id string) (string, error) {
	c, err := s.ownedClient(ctx, userID, id)
	if err != nil {
		return "", err
	}
	if c.Public() {
		return "", ErrPublicOAuthApp
	}
	secret, hash := NewToken(ClientSecretPrefix)
	r := identity.OAuthClientSecretRotation{ID: s.d.IDs.NewID(), ClientID: c.ID, SecretHash: hash, RotatedAt: s.d.Clock.Now()}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertOAuthClientSecretRotation(ctx, tx, r)
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}

// IssueOAuthAppTokens issues an access token and a refresh token of the client with the ID
// that the user registered to the user's own account, without a redirect (ADR-0090 §2): a
// usual pair with the usual lifetimes, refreshed and revoked as any other. An empty scope
// means the client's whole scope; a scope beyond the client's, or with a first-party-only
// scope, fails with an *OAuthError invalid_scope. A disabled client fails with
// ErrOAuthAppDisabled.
func (s *Service) IssueOAuthAppTokens(ctx context.Context, userID, id, scope string) (IssuedTokens, error) {
	app, err := s.OAuthApp(ctx, userID, id)
	if err != nil {
		return IssuedTokens{}, err
	}
	if app.Disabled {
		return IssuedTokens{}, ErrOAuthAppDisabled
	}
	c := app.Client
	granted, err := identity.ParseScope(scope)
	if err != nil {
		return IssuedTokens{}, oauthError(OAuthInvalidScope, "unknown scope")
	}
	if granted == "" {
		granted = c.Scope
	}
	if !granted.Within(c.Scope) || granted.HasFirstPartyOnly() {
		return IssuedTokens{}, oauthError(OAuthInvalidScope, "the client may not have this scope")
	}
	issued := s.newTokens(c, userID, granted, "", s.d.Clock.Now())
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertAccessToken(ctx, tx, issued.Token) })
	if err != nil {
		return IssuedTokens{}, err
	}
	return issued, nil
}

// ownerCanUseApps reports whether the user who registered a client may use the app, which the
// client needs to work: a suspended or deleted owner's clients stop working (ADR-0090 §1).
func (s *Service) ownerCanUseApps(ctx context.Context, ownerUserID string) (bool, error) {
	access, err := s.UserAccess(ctx, ownerUserID)
	if err != nil {
		return false, err
	}
	return access.Allowed() && access.AccountID != "" && access.AccountActive, nil
}

// revokeTokensOfOwnedClients revokes the tokens of every client the user registered, when the
// user can no longer use the app.
func revokeTokensOfOwnedClients(ctx context.Context, tx uow.Tx, repo Repository, userID string, at time.Time) error {
	if err := repo.RevokeTokensOfOwnedClients(ctx, tx, userID, identity.TokenRevokedClientDisabled, at); err != nil {
		return fmt.Errorf("identity: revoke the tokens of the clients of %s: %w", userID, err)
	}
	return nil
}
