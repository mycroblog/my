// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// EnsureOAuthClient implements identity.Repository.
func (r *Repository) EnsureOAuthClient(ctx context.Context, tx uow.Tx, c identity.OAuthClient) (identity.OAuthClient, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.OAuthClient{}, err
	}
	err = q.InsertOAuthClientIfAbsent(ctx, identitygen.InsertOAuthClientIfAbsentParams{
		ID: c.ID, ClientID: c.ClientID, ClientSecretHash: c.SecretHash, Name: c.Name, Website: postgres.ToNullString(c.Website),
		Scope: string(c.Scope), Registration: string(c.Registration), OwnerUserID: postgres.ToNullString(c.OwnerUserID),
		CreatedAt: c.CreatedAt,
	})
	if err != nil {
		return identity.OAuthClient{}, postgres.Wrap(err, "insert oauth client")
	}
	row, err := q.GetOAuthClientByClientID(ctx, c.ClientID)
	if err != nil {
		return identity.OAuthClient{}, postgres.Wrap(err, "get oauth client")
	}
	return oauthClientFromRow(row), nil
}

// ReplaceOAuthClientRedirectURIs implements identity.Repository.
func (r *Repository) ReplaceOAuthClientRedirectURIs(ctx context.Context, tx uow.Tx, clientID string, uris []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	current, err := q.ListOAuthClientRedirectURIs(ctx, clientID)
	if err != nil {
		return postgres.Wrap(err, "list oauth client redirect uris")
	}
	for _, u := range current {
		if slices.Contains(uris, u) {
			continue
		}
		err := q.DeleteOAuthClientRedirectURI(ctx, identitygen.DeleteOAuthClientRedirectURIParams{OauthClientID: clientID, RedirectUri: u})
		if err != nil {
			return postgres.Wrap(err, "delete oauth client redirect uri")
		}
	}
	for _, u := range uris {
		err := q.InsertOAuthClientRedirectURIIfAbsent(ctx, identitygen.InsertOAuthClientRedirectURIIfAbsentParams{OauthClientID: clientID, RedirectUri: u})
		if err != nil {
			return postgres.Wrap(err, "insert oauth client redirect uri")
		}
	}
	return nil
}

// InsertAuthorizationCode implements identity.Repository.
func (r *Repository) InsertAuthorizationCode(ctx context.Context, tx uow.Tx, c identity.AuthorizationCode) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAuthorizationCode(ctx, identitygen.InsertAuthorizationCodeParams{
		CodeHash: c.CodeHash, OauthClientID: c.ClientID, UserID: c.UserID, RedirectUri: c.RedirectURI, Scope: string(c.Scope),
		CodeChallenge: c.CodeChallenge, SessionID: postgres.ToNullString(c.SessionID), IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt,
	})
	return postgres.Wrap(err, "insert authorization code")
}

// InsertAuthorizationCodeRedemption implements identity.Repository.
func (r *Repository) InsertAuthorizationCodeRedemption(ctx context.Context, tx uow.Tx, red identity.AuthorizationCodeRedemption) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAuthorizationCodeRedemptionIfAbsent(ctx, identitygen.InsertAuthorizationCodeRedemptionIfAbsentParams{
		CodeHash: red.CodeHash, TokenID: red.TokenID, RedeemedAt: red.RedeemedAt,
	})
	return n == 1, postgres.Wrap(err, "insert authorization code redemption")
}

// InsertAccessToken implements identity.Repository.
func (r *Repository) InsertAccessToken(ctx context.Context, tx uow.Tx, t identity.AccessToken) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var refreshExpires *time.Time
	if t.RefreshTokenHash != nil {
		refreshExpires = &t.RefreshExpiresAt
	}
	err = q.InsertAccessToken(ctx, identitygen.InsertAccessTokenParams{
		ID: t.ID, FamilyID: t.FamilyID, OauthClientID: t.ClientID, UserID: t.UserID,
		AccessTokenHash: t.AccessTokenHash, RefreshTokenHash: t.RefreshTokenHash, Scope: string(t.Scope),
		RotatedFromTokenID: postgres.ToNullString(t.RotatedFromTokenID), IssuedAt: t.IssuedAt,
		ExpiresAt: t.ExpiresAt, RefreshExpiresAt: refreshExpires,
	})
	return postgres.Wrap(err, "insert access token")
}

// InsertTokenRevocation implements identity.Repository.
func (r *Repository) InsertTokenRevocation(ctx context.Context, tx uow.Tx, rev identity.TokenRevocation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertTokenRevocationIfAbsent(ctx, identitygen.InsertTokenRevocationIfAbsentParams{
		TokenID: rev.TokenID, Reason: string(rev.Reason), RevokedAt: rev.RevokedAt,
	})
	return n == 1, postgres.Wrap(err, "insert token revocation")
}

// RevokeTokenFamily implements identity.Repository.
func (r *Repository) RevokeTokenFamily(ctx context.Context, tx uow.Tx, familyID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokenFamily(ctx, identitygen.RevokeTokenFamilyParams{Reason: string(reason), RevokedAt: at, FamilyID: familyID})
	return postgres.Wrap(err, "revoke token family")
}

// LockTokenGrants implements identity.Repository with a transaction-scoped advisory lock on the
// user, so that issuing and revoking the user's tokens serialize under READ COMMITTED.
func (r *Repository) LockTokenGrants(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.LockUserTokenGrants(ctx, "identity:oauth_grants:"+userID), "lock the token grants")
}

// SessionRevoked implements identity.Repository.
func (r *Repository) SessionRevoked(ctx context.Context, tx uow.Tx, sessionID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	revoked, err := q.SessionRevoked(ctx, sessionID)
	return revoked, postgres.Wrap(err, "check session revocation")
}

// TokenFamilyActive implements identity.Repository.
func (r *Repository) TokenFamilyActive(ctx context.Context, familyID string, now time.Time) (bool, error) {
	active, err := r.db.reader().TokenFamilyActive(ctx, identitygen.TokenFamilyActiveParams{FamilyID: familyID, Now: now})
	return active, postgres.Wrap(err, "check token family")
}

// ListUnusedDynamicOAuthClients implements identity.Repository.
func (r *Repository) ListUnusedDynamicOAuthClients(ctx context.Context, unredeemedBefore, recordsKeptSince, idleBefore, now time.Time, limit int) ([]string, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUnusedDynamicOAuthClients(ctx, identitygen.ListUnusedDynamicOAuthClientsParams{
		UnredeemedBefore: unredeemedBefore, RecordsKeptSince: recordsKeptSince, IdleBefore: idleBefore, Now: now, BatchLimit: n,
	})
	return ids, postgres.Wrap(err, "list unused dynamic oauth clients")
}

// RevokeSessionOfTokenFamily implements identity.Repository.
func (r *Repository) RevokeSessionOfTokenFamily(ctx context.Context, tx uow.Tx, familyID string, reason identity.RevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeSessionOfTokenFamily(ctx, identitygen.RevokeSessionOfTokenFamilyParams{Reason: string(reason), RevokedAt: at, FamilyID: familyID})
	return postgres.Wrap(err, "revoke the session of a token family")
}

// FindOAuthClient implements identity.Repository.
func (r *Repository) FindOAuthClient(ctx context.Context, clientID string) (identity.OAuthClient, []string, error) {
	q := r.db.reader()
	row, err := q.GetOAuthClientByClientID(ctx, clientID)
	if err != nil {
		return identity.OAuthClient{}, nil, postgres.NotFound(err, appidentity.ErrNotFound, "oauth client")
	}
	uris, err := q.ListOAuthClientRedirectURIs(ctx, row.ID)
	if err != nil {
		return identity.OAuthClient{}, nil, postgres.Wrap(err, "list oauth client redirect uris")
	}
	c, err := currentOAuthClient(ctx, q, oauthClientFromRow(row))
	return c, uris, err
}

// FindOAuthClientByID implements identity.Repository.
func (r *Repository) FindOAuthClientByID(ctx context.Context, id string) (identity.OAuthClient, error) {
	q := r.db.reader()
	row, err := q.GetOAuthClientByID(ctx, id)
	if err != nil {
		return identity.OAuthClient{}, postgres.NotFound(err, appidentity.ErrNotFound, "oauth client")
	}
	return currentOAuthClient(ctx, q, oauthClientFromRow(row))
}

// FindAuthorizationCode implements identity.Repository.
func (r *Repository) FindAuthorizationCode(ctx context.Context, codeHash []byte) (identity.AuthorizationCode, error) {
	row, err := r.db.reader().GetAuthorizationCode(ctx, codeHash)
	if err != nil {
		return identity.AuthorizationCode{}, postgres.NotFound(err, appidentity.ErrNotFound, "authorization code")
	}
	return identity.AuthorizationCode{
		CodeHash: row.CodeHash, ClientID: row.OauthClientID, UserID: row.UserID, RedirectURI: row.RedirectUri,
		Scope: identity.Scope(row.Scope), CodeChallenge: row.CodeChallenge, SessionID: postgres.FromNullString(row.SessionID),
		IssuedAt: postgres.UTC(row.IssuedAt), ExpiresAt: postgres.UTC(row.ExpiresAt),
	}, nil
}

// FindAuthorizationCodeRedemption implements identity.Repository.
func (r *Repository) FindAuthorizationCodeRedemption(ctx context.Context, codeHash []byte) (identity.AuthorizationCodeRedemption, error) {
	row, err := r.db.reader().GetAuthorizationCodeRedemption(ctx, codeHash)
	if err != nil {
		return identity.AuthorizationCodeRedemption{}, postgres.NotFound(err, appidentity.ErrNotFound, "authorization code redemption")
	}
	return identity.AuthorizationCodeRedemption{CodeHash: row.CodeHash, TokenID: row.TokenID, RedeemedAt: postgres.UTC(row.RedeemedAt)}, nil
}

// FindAccessTokenByAccessHash implements identity.Repository.
func (r *Repository) FindAccessTokenByAccessHash(ctx context.Context, hash []byte) (identity.AccessToken, identity.TokenState, error) {
	row, err := r.db.reader().GetAccessTokenByAccessHash(ctx, hash)
	if err != nil {
		return identity.AccessToken{}, identity.TokenState{}, postgres.NotFound(err, appidentity.ErrNotFound, "access token")
	}
	state := identity.TokenState{
		Revoked: row.Revoked, Client: identity.OAuthClientState{Disabled: row.ClientDisabled, Deleted: row.ClientDeleted},
	}
	return accessTokenFromRow(row), state, nil
}

// FindAccessTokensByAccessHashes implements identity.Repository.
func (r *Repository) FindAccessTokensByAccessHashes(ctx context.Context, hashes [][]byte) (map[string]appidentity.TokenWithState, error) {
	out := map[string]appidentity.TokenWithState{}
	if len(hashes) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListAccessTokensByAccessHashes(ctx, hashes)
	if err != nil {
		return nil, postgres.Wrap(err, "list access tokens")
	}
	for _, row := range rows {
		state := identity.TokenState{
			Revoked: row.Revoked, Client: identity.OAuthClientState{Disabled: row.ClientDisabled, Deleted: row.ClientDeleted},
		}
		out[string(row.AccessTokenHash)] = appidentity.TokenWithState{Token: accessTokenFromRow(identitygen.GetAccessTokenByAccessHashRow(row)), State: state}
	}
	return out, nil
}

// ExistingOAuthClients implements identity.Repository.
func (r *Repository) ExistingOAuthClients(ctx context.Context, clientIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(clientIDs) == 0 {
		return out, nil
	}
	ids, err := r.db.reader().ListOAuthClientIDsAmong(ctx, clientIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list oauth clients")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// FindAccessTokenByRefreshHash implements identity.Repository.
func (r *Repository) FindAccessTokenByRefreshHash(ctx context.Context, hash []byte) (identity.AccessToken, identity.TokenState, error) {
	row, err := r.db.reader().GetAccessTokenByRefreshHash(ctx, hash)
	if err != nil {
		return identity.AccessToken{}, identity.TokenState{}, postgres.NotFound(err, appidentity.ErrNotFound, "access token")
	}
	state := identity.TokenState{
		Revoked: row.Revoked, Client: identity.OAuthClientState{Disabled: row.ClientDisabled, Deleted: row.ClientDeleted},
	}
	return accessTokenFromRow(identitygen.GetAccessTokenByAccessHashRow(row)), state, nil
}

func oauthClientFromRow(row identitygen.OauthClient) identity.OAuthClient {
	return identity.OAuthClient{
		ID: row.ID, ClientID: row.ClientID, SecretHash: row.ClientSecretHash, Name: row.Name, Website: postgres.FromNullString(row.Website),
		Scope: identity.Scope(row.Scope), Registration: identity.OAuthClientRegistration(row.Registration), OwnerUserID: postgres.FromNullString(row.OwnerUserID),
		CreatedAt: postgres.UTC(row.CreatedAt),
	}
}

func accessTokenFromRow(row identitygen.GetAccessTokenByAccessHashRow) identity.AccessToken {
	t := identity.AccessToken{
		ID: row.ID, FamilyID: row.FamilyID, ClientID: row.OauthClientID, UserID: row.UserID,
		AccessTokenHash: row.AccessTokenHash, RefreshTokenHash: row.RefreshTokenHash, Scope: identity.Scope(row.Scope),
		RotatedFromTokenID: postgres.FromNullString(row.RotatedFromTokenID), IssuedAt: postgres.UTC(row.IssuedAt), ExpiresAt: postgres.UTC(row.ExpiresAt),
	}
	if row.RefreshExpiresAt != nil {
		t.RefreshExpiresAt = postgres.UTC(*row.RefreshExpiresAt)
	}
	return t
}

// RevokeOtherTokensOfUser implements identity.Repository.
func (r *Repository) RevokeOtherTokensOfUser(ctx context.Context, tx uow.Tx, userID, keepFamilyID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeOtherTokensOfUser(ctx, identitygen.RevokeOtherTokensOfUserParams{
		Reason: string(reason), RevokedAt: at, UserID: userID, KeepFamilyID: keepFamilyID,
	})
	return postgres.Wrap(err, "revoke the other tokens of a user")
}

// RevokeTokensOfUser implements identity.Repository.
func (r *Repository) RevokeTokensOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokensOfUser(ctx, identitygen.RevokeTokensOfUserParams{Reason: string(reason), RevokedAt: at, UserID: userID})
	return postgres.Wrap(err, "revoke the tokens of a user")
}

// FindOAuthClientState implements identity.Repository.
func (r *Repository) FindOAuthClientState(ctx context.Context, clientID string) (identity.OAuthClientState, error) {
	row, err := r.db.reader().GetOAuthClientState(ctx, clientID)
	if err != nil {
		return identity.OAuthClientState{}, postgres.NotFound(err, appidentity.ErrNotFound, "oauth client")
	}
	return identity.OAuthClientState{Disabled: row.Disabled, Deleted: row.Deleted}, nil
}

// RevokeTokensOfClient implements identity.Repository.
func (r *Repository) RevokeTokensOfClient(ctx context.Context, tx uow.Tx, clientID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokensOfClient(ctx, identitygen.RevokeTokensOfClientParams{Reason: string(reason), RevokedAt: at, OauthClientID: clientID})
	return postgres.Wrap(err, "revoke the tokens of a client")
}

// ListOAuthClients implements identity.Repository.
func (r *Repository) ListOAuthClients(ctx context.Context, registration identity.OAuthClientRegistration, before string, limit int) ([]identity.OAuthClientListing, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListOAuthClients(ctx, identitygen.ListOAuthClientsParams{
		Registration: postgres.ToNullString(string(registration)), Before: postgres.ToNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list oauth clients")
	}
	out := make([]identity.OAuthClientListing, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClientListing{
			OAuthClient: oauthClientFromRow(identitygen.OauthClient{
				ID: row.ID, ClientID: row.ClientID, ClientSecretHash: row.ClientSecretHash, Name: row.Name, Website: row.Website,
				Scope: row.Scope, Registration: row.Registration, OwnerUserID: row.OwnerUserID, CreatedAt: row.CreatedAt,
			}),
			Disabled: row.Disabled,
		}
	}
	return out, nil
}

// InsertOAuthClientDisablement implements identity.Repository.
func (r *Repository) InsertOAuthClientDisablement(ctx context.Context, tx uow.Tx, dis identity.OAuthClientDisablement) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientDisablementIfEnabled(ctx, identitygen.InsertOAuthClientDisablementIfEnabledParams{
		ID: dis.ID, OauthClientID: dis.ClientID, DisabledByUserID: postgres.ToNullString(dis.DisabledByUserID), DisabledAt: dis.DisabledAt,
	})
	return n == 1, postgres.Wrap(err, "insert oauth client disablement")
}

// LiftOAuthClientDisablements implements identity.Repository.
func (r *Repository) LiftOAuthClientDisablements(ctx context.Context, tx uow.Tx, clientID, byUserID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.LiftOAuthClientDisablements(ctx, identitygen.LiftOAuthClientDisablementsParams{
		LiftedByUserID: postgres.ToNullString(byUserID), LiftedAt: at, OauthClientID: clientID,
	})
	return n > 0, postgres.Wrap(err, "lift oauth client disablements")
}

// InsertOAuthClientDeletion implements identity.Repository.
func (r *Repository) InsertOAuthClientDeletion(ctx context.Context, tx uow.Tx, del identity.OAuthClientDeletion) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientDeletionIfAbsent(ctx, identitygen.InsertOAuthClientDeletionIfAbsentParams{
		OauthClientID: del.ClientID, Reason: string(del.Reason), DeletedByUserID: postgres.ToNullString(del.DeletedByUserID), DeletedAt: del.DeletedAt,
	})
	return n == 1, postgres.Wrap(err, "insert oauth client deletion")
}

// UpsertTokenFamilyLastUsed implements identity.Repository.
func (r *Repository) UpsertTokenFamilyLastUsed(ctx context.Context, tx uow.Tx, familyID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertTokenFamilyLastUsed(ctx, identitygen.UpsertTokenFamilyLastUsedParams{FamilyID: familyID, LastUsedAt: at})
	return postgres.Wrap(err, "record the last use of a token family")
}

// ListAuthorizedAppTokens implements identity.Repository.
func (r *Repository) ListAuthorizedAppTokens(ctx context.Context, userID string, now time.Time) ([]identity.AuthorizedAppToken, error) {
	rows, err := r.db.reader().ListAuthorizedAppTokens(ctx, identitygen.ListAuthorizedAppTokensParams{UserID: userID, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list authorized app tokens")
	}
	out := make([]identity.AuthorizedAppToken, len(rows))
	for i, row := range rows {
		t := identity.AuthorizedAppToken{
			FamilyID: row.FamilyID, Scope: identity.Scope(row.TokenScope), AuthorizedAt: postgres.UTC(row.AuthorizedAt),
			Client: oauthClientFromRow(identitygen.OauthClient{
				ID: row.ID, ClientID: row.ClientID, ClientSecretHash: row.ClientSecretHash, Name: row.Name, Website: row.Website,
				Scope: row.Scope, Registration: row.Registration, OwnerUserID: row.OwnerUserID, CreatedAt: row.CreatedAt,
			}),
		}
		if row.LastUsedAt != nil {
			u := postgres.UTC(*row.LastUsedAt)
			t.LastUsedAt = &u
		}
		out[i] = t
	}
	return out, nil
}

// RevokeTokensOfUserAndClient implements identity.Repository.
func (r *Repository) RevokeTokensOfUserAndClient(ctx context.Context, tx uow.Tx, userID, clientID string, reason identity.TokenRevocationReason, at time.Time) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.RevokeTokensOfUserAndClient(ctx, identitygen.RevokeTokensOfUserAndClientParams{Reason: string(reason), RevokedAt: at, UserID: userID, OauthClientID: clientID})
	return n, postgres.Wrap(err, "revoke the tokens of a user and a client")
}

// ListBrowserSessions implements identity.Repository.
func (r *Repository) ListBrowserSessions(ctx context.Context, userID string, now time.Time) ([]identity.BrowserSession, error) {
	rows, err := r.db.reader().ListBrowserSessions(ctx, identitygen.ListBrowserSessionsParams{UserID: userID, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list browser sessions")
	}
	out := make([]identity.BrowserSession, len(rows))
	for i, row := range rows {
		s := identity.BrowserSession{ID: row.ID, CreatedAt: postgres.UTC(row.CreatedAt)}
		if row.Ip != nil {
			s.IP = *row.Ip
		}
		if row.UserAgent != nil {
			s.UserAgent = *row.UserAgent
		}
		if row.LastSeenAt != nil {
			at := postgres.UTC(*row.LastSeenAt)
			s.LastSeenAt = &at
		}
		out[i] = s
	}
	return out, nil
}

// ListBrowserSessionFamilies implements identity.Repository.
func (r *Repository) ListBrowserSessionFamilies(ctx context.Context, userID string, now time.Time) ([]identity.BrowserSessionFamily, error) {
	rows, err := r.db.reader().ListBrowserSessionFamilies(ctx, identitygen.ListBrowserSessionFamiliesParams{UserID: userID, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list browser session families")
	}
	out := make([]identity.BrowserSessionFamily, 0, len(rows))
	for _, row := range rows {
		if row.SessionID == nil {
			continue
		}
		f := identity.BrowserSessionFamily{SessionID: *row.SessionID, FamilyID: row.FamilyID}
		if row.LastUsedAt != nil {
			at := postgres.UTC(*row.LastUsedAt)
			f.LastUsedAt = &at
		}
		out = append(out, f)
	}
	return out, nil
}

// RevokeFirstPartyTokensOfSession implements identity.Repository.
func (r *Repository) RevokeFirstPartyTokensOfSession(ctx context.Context, tx uow.Tx, sessionID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeFirstPartyTokensOfSession(ctx, identitygen.RevokeFirstPartyTokensOfSessionParams{Reason: string(reason), RevokedAt: at, SessionID: &sessionID})
	return postgres.Wrap(err, "revoke the first-party tokens of a session")
}
