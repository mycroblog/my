// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// EnsureOAuthClient implements identity.Repository.
func (r *IdentityRepository) EnsureOAuthClient(ctx context.Context, tx uow.Tx, c identity.OAuthClient) (identity.OAuthClient, error) {
	q, err := queries(tx)
	if err != nil {
		return identity.OAuthClient{}, err
	}
	err = q.InsertOAuthClientIfAbsent(ctx, postgresgen.InsertOAuthClientIfAbsentParams{
		ID: c.ID, ClientID: c.ClientID, ClientSecretHash: c.SecretHash, Name: c.Name, Website: toNullString(c.Website),
		Scope: string(c.Scope), Registration: string(c.Registration), OwnerUserID: toNullString(c.OwnerUserID),
		CreatedAt: c.CreatedAt,
	})
	if err != nil {
		return identity.OAuthClient{}, wrap(err, "insert oauth client")
	}
	row, err := q.GetOAuthClientByClientID(ctx, c.ClientID)
	if err != nil {
		return identity.OAuthClient{}, wrap(err, "get oauth client")
	}
	return oauthClientFromRow(row), nil
}

// ReplaceOAuthClientRedirectURIs implements identity.Repository.
func (r *IdentityRepository) ReplaceOAuthClientRedirectURIs(ctx context.Context, tx uow.Tx, clientID string, uris []string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	current, err := q.ListOAuthClientRedirectURIs(ctx, clientID)
	if err != nil {
		return wrap(err, "list oauth client redirect uris")
	}
	for _, u := range current {
		if slices.Contains(uris, u) {
			continue
		}
		err := q.DeleteOAuthClientRedirectURI(ctx, postgresgen.DeleteOAuthClientRedirectURIParams{ClientID: clientID, RedirectUri: u})
		if err != nil {
			return wrap(err, "delete oauth client redirect uri")
		}
	}
	for _, u := range uris {
		err := q.InsertOAuthClientRedirectURIIfAbsent(ctx, postgresgen.InsertOAuthClientRedirectURIIfAbsentParams{ClientID: clientID, RedirectUri: u})
		if err != nil {
			return wrap(err, "insert oauth client redirect uri")
		}
	}
	return nil
}

// InsertAuthorizationCode implements identity.Repository.
func (r *IdentityRepository) InsertAuthorizationCode(ctx context.Context, tx uow.Tx, c identity.AuthorizationCode) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAuthorizationCode(ctx, postgresgen.InsertAuthorizationCodeParams{
		CodeHash: c.CodeHash, ClientID: c.ClientID, UserID: c.UserID, RedirectUri: c.RedirectURI, Scope: string(c.Scope),
		CodeChallenge: c.CodeChallenge, SessionID: toNullString(c.SessionID), IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt,
	})
	return wrap(err, "insert authorization code")
}

// InsertAuthorizationCodeRedemption implements identity.Repository.
func (r *IdentityRepository) InsertAuthorizationCodeRedemption(ctx context.Context, tx uow.Tx, red identity.AuthorizationCodeRedemption) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAuthorizationCodeRedemptionIfAbsent(ctx, postgresgen.InsertAuthorizationCodeRedemptionIfAbsentParams{
		CodeHash: red.CodeHash, TokenID: red.TokenID, RedeemedAt: red.RedeemedAt,
	})
	return n == 1, wrap(err, "insert authorization code redemption")
}

// InsertAccessToken implements identity.Repository.
func (r *IdentityRepository) InsertAccessToken(ctx context.Context, tx uow.Tx, t identity.AccessToken) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	var refreshExpires *time.Time
	if t.RefreshTokenHash != nil {
		refreshExpires = &t.RefreshExpiresAt
	}
	err = q.InsertAccessToken(ctx, postgresgen.InsertAccessTokenParams{
		ID: t.ID, FamilyID: t.FamilyID, ClientID: t.ClientID, UserID: t.UserID,
		AccessTokenHash: t.AccessTokenHash, RefreshTokenHash: t.RefreshTokenHash, Scope: string(t.Scope),
		RotatedFromTokenID: toNullString(t.RotatedFromTokenID), IssuedAt: t.IssuedAt,
		ExpiresAt: t.ExpiresAt, RefreshExpiresAt: refreshExpires,
	})
	return wrap(err, "insert access token")
}

// InsertTokenRevocation implements identity.Repository.
func (r *IdentityRepository) InsertTokenRevocation(ctx context.Context, tx uow.Tx, rev identity.TokenRevocation) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertTokenRevocationIfAbsent(ctx, postgresgen.InsertTokenRevocationIfAbsentParams{
		TokenID: rev.TokenID, Reason: string(rev.Reason), RevokedAt: rev.RevokedAt,
	})
	return n == 1, wrap(err, "insert token revocation")
}

// RevokeTokenFamily implements identity.Repository.
func (r *IdentityRepository) RevokeTokenFamily(ctx context.Context, tx uow.Tx, familyID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokenFamily(ctx, postgresgen.RevokeTokenFamilyParams{Reason: string(reason), RevokedAt: at, FamilyID: familyID})
	return wrap(err, "revoke token family")
}

// TokenFamilyActive implements identity.Repository.
func (r *IdentityRepository) TokenFamilyActive(ctx context.Context, familyID string, now time.Time) (bool, error) {
	active, err := r.db.reader().TokenFamilyActive(ctx, postgresgen.TokenFamilyActiveParams{FamilyID: familyID, Now: now})
	return active, wrap(err, "check token family")
}

// ListUnusedDynamicOAuthClients implements identity.Repository.
func (r *IdentityRepository) ListUnusedDynamicOAuthClients(ctx context.Context, unredeemedBefore, idleBefore, now time.Time, limit int) ([]string, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUnusedDynamicOAuthClients(ctx, postgresgen.ListUnusedDynamicOAuthClientsParams{
		UnredeemedBefore: unredeemedBefore, IdleBefore: idleBefore, Now: now, BatchLimit: n,
	})
	return ids, wrap(err, "list unused dynamic oauth clients")
}

// RevokeSessionOfTokenFamily implements identity.Repository.
func (r *IdentityRepository) RevokeSessionOfTokenFamily(ctx context.Context, tx uow.Tx, familyID string, reason identity.RevocationReason, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeSessionOfTokenFamily(ctx, postgresgen.RevokeSessionOfTokenFamilyParams{Reason: string(reason), RevokedAt: at, FamilyID: familyID})
	return wrap(err, "revoke the session of a token family")
}

// FindOAuthClient implements identity.Repository.
func (r *IdentityRepository) FindOAuthClient(ctx context.Context, clientID string) (identity.OAuthClient, []string, error) {
	q := r.db.reader()
	row, err := q.GetOAuthClientByClientID(ctx, clientID)
	if err != nil {
		return identity.OAuthClient{}, nil, notFound(err, appidentity.ErrNotFound, "oauth client")
	}
	uris, err := q.ListOAuthClientRedirectURIs(ctx, row.ID)
	if err != nil {
		return identity.OAuthClient{}, nil, wrap(err, "list oauth client redirect uris")
	}
	c, err := currentOAuthClient(ctx, q, oauthClientFromRow(row))
	return c, uris, err
}

// FindOAuthClientByID implements identity.Repository.
func (r *IdentityRepository) FindOAuthClientByID(ctx context.Context, id string) (identity.OAuthClient, error) {
	q := r.db.reader()
	row, err := q.GetOAuthClientByID(ctx, id)
	if err != nil {
		return identity.OAuthClient{}, notFound(err, appidentity.ErrNotFound, "oauth client")
	}
	return currentOAuthClient(ctx, q, oauthClientFromRow(row))
}

// FindAuthorizationCode implements identity.Repository.
func (r *IdentityRepository) FindAuthorizationCode(ctx context.Context, codeHash []byte) (identity.AuthorizationCode, error) {
	row, err := r.db.reader().GetAuthorizationCode(ctx, codeHash)
	if err != nil {
		return identity.AuthorizationCode{}, notFound(err, appidentity.ErrNotFound, "authorization code")
	}
	return identity.AuthorizationCode{
		CodeHash: row.CodeHash, ClientID: row.ClientID, UserID: row.UserID, RedirectURI: row.RedirectUri,
		Scope: identity.Scope(row.Scope), CodeChallenge: row.CodeChallenge, SessionID: fromNullString(row.SessionID),
		IssuedAt: utc(row.IssuedAt), ExpiresAt: utc(row.ExpiresAt),
	}, nil
}

// FindAuthorizationCodeRedemption implements identity.Repository.
func (r *IdentityRepository) FindAuthorizationCodeRedemption(ctx context.Context, codeHash []byte) (identity.AuthorizationCodeRedemption, error) {
	row, err := r.db.reader().GetAuthorizationCodeRedemption(ctx, codeHash)
	if err != nil {
		return identity.AuthorizationCodeRedemption{}, notFound(err, appidentity.ErrNotFound, "authorization code redemption")
	}
	return identity.AuthorizationCodeRedemption{CodeHash: row.CodeHash, TokenID: row.TokenID, RedeemedAt: utc(row.RedeemedAt)}, nil
}

// FindAccessTokenByAccessHash implements identity.Repository.
func (r *IdentityRepository) FindAccessTokenByAccessHash(ctx context.Context, hash []byte) (identity.AccessToken, identity.TokenState, error) {
	row, err := r.db.reader().GetAccessTokenByAccessHash(ctx, hash)
	if err != nil {
		return identity.AccessToken{}, identity.TokenState{}, notFound(err, appidentity.ErrNotFound, "access token")
	}
	state := identity.TokenState{
		Revoked: row.Revoked, Client: identity.OAuthClientState{Disabled: row.ClientDisabled, Deleted: row.ClientDeleted},
	}
	return accessTokenFromRow(row), state, nil
}

// FindAccessTokenByRefreshHash implements identity.Repository.
func (r *IdentityRepository) FindAccessTokenByRefreshHash(ctx context.Context, hash []byte) (identity.AccessToken, identity.TokenState, error) {
	row, err := r.db.reader().GetAccessTokenByRefreshHash(ctx, hash)
	if err != nil {
		return identity.AccessToken{}, identity.TokenState{}, notFound(err, appidentity.ErrNotFound, "access token")
	}
	state := identity.TokenState{
		Revoked: row.Revoked, Client: identity.OAuthClientState{Disabled: row.ClientDisabled, Deleted: row.ClientDeleted},
	}
	return accessTokenFromRow(postgresgen.GetAccessTokenByAccessHashRow(row)), state, nil
}

func oauthClientFromRow(row postgresgen.OauthClient) identity.OAuthClient {
	return identity.OAuthClient{
		ID: row.ID, ClientID: row.ClientID, SecretHash: row.ClientSecretHash, Name: row.Name, Website: fromNullString(row.Website),
		Scope: identity.Scope(row.Scope), Registration: identity.OAuthClientRegistration(row.Registration), OwnerUserID: fromNullString(row.OwnerUserID),
		CreatedAt: utc(row.CreatedAt),
	}
}

func accessTokenFromRow(row postgresgen.GetAccessTokenByAccessHashRow) identity.AccessToken {
	t := identity.AccessToken{
		ID: row.ID, FamilyID: row.FamilyID, ClientID: row.ClientID, UserID: row.UserID,
		AccessTokenHash: row.AccessTokenHash, RefreshTokenHash: row.RefreshTokenHash, Scope: identity.Scope(row.Scope),
		RotatedFromTokenID: fromNullString(row.RotatedFromTokenID), IssuedAt: utc(row.IssuedAt), ExpiresAt: utc(row.ExpiresAt),
	}
	if row.RefreshExpiresAt != nil {
		t.RefreshExpiresAt = utc(*row.RefreshExpiresAt)
	}
	return t
}

// RevokeOtherTokensOfUser implements identity.Repository.
func (r *IdentityRepository) RevokeOtherTokensOfUser(ctx context.Context, tx uow.Tx, userID, keepFamilyID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeOtherTokensOfUser(ctx, postgresgen.RevokeOtherTokensOfUserParams{
		Reason: string(reason), RevokedAt: at, UserID: userID, KeepFamilyID: keepFamilyID,
	})
	return wrap(err, "revoke the other tokens of a user")
}

// RevokeTokensOfUser implements identity.Repository.
func (r *IdentityRepository) RevokeTokensOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokensOfUser(ctx, postgresgen.RevokeTokensOfUserParams{Reason: string(reason), RevokedAt: at, UserID: userID})
	return wrap(err, "revoke the tokens of a user")
}

// FindOAuthClientState implements identity.Repository.
func (r *IdentityRepository) FindOAuthClientState(ctx context.Context, clientID string) (identity.OAuthClientState, error) {
	row, err := r.db.reader().GetOAuthClientState(ctx, clientID)
	if err != nil {
		return identity.OAuthClientState{}, notFound(err, appidentity.ErrNotFound, "oauth client")
	}
	return identity.OAuthClientState{Disabled: row.Disabled, Deleted: row.Deleted}, nil
}

// RevokeTokensOfClient implements identity.Repository.
func (r *IdentityRepository) RevokeTokensOfClient(ctx context.Context, tx uow.Tx, clientID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokensOfClient(ctx, postgresgen.RevokeTokensOfClientParams{Reason: string(reason), RevokedAt: at, ClientID: clientID})
	return wrap(err, "revoke the tokens of a client")
}

// ListOAuthClients implements identity.Repository.
func (r *IdentityRepository) ListOAuthClients(ctx context.Context, registration identity.OAuthClientRegistration, before string, limit int) ([]identity.OAuthClientListing, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListOAuthClients(ctx, postgresgen.ListOAuthClientsParams{
		Registration: toNullString(string(registration)), Before: toNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, wrap(err, "list oauth clients")
	}
	out := make([]identity.OAuthClientListing, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClientListing{
			OAuthClient: oauthClientFromRow(postgresgen.OauthClient{
				ID: row.ID, ClientID: row.ClientID, ClientSecretHash: row.ClientSecretHash, Name: row.Name, Website: row.Website,
				Scope: row.Scope, Registration: row.Registration, OwnerUserID: row.OwnerUserID, CreatedAt: row.CreatedAt,
			}),
			Disabled: row.Disabled,
		}
	}
	return out, nil
}

// InsertOAuthClientDisablement implements identity.Repository.
func (r *IdentityRepository) InsertOAuthClientDisablement(ctx context.Context, tx uow.Tx, dis identity.OAuthClientDisablement) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientDisablementIfEnabled(ctx, postgresgen.InsertOAuthClientDisablementIfEnabledParams{
		ID: dis.ID, ClientID: dis.ClientID, DisabledByUserID: toNullString(dis.DisabledByUserID), DisabledAt: dis.DisabledAt,
	})
	return n == 1, wrap(err, "insert oauth client disablement")
}

// LiftOAuthClientDisablements implements identity.Repository.
func (r *IdentityRepository) LiftOAuthClientDisablements(ctx context.Context, tx uow.Tx, clientID, byUserID string, at time.Time) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.LiftOAuthClientDisablements(ctx, postgresgen.LiftOAuthClientDisablementsParams{
		LiftedByUserID: toNullString(byUserID), LiftedAt: at, ClientID: clientID,
	})
	return n > 0, wrap(err, "lift oauth client disablements")
}

// InsertOAuthClientDeletion implements identity.Repository.
func (r *IdentityRepository) InsertOAuthClientDeletion(ctx context.Context, tx uow.Tx, del identity.OAuthClientDeletion) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientDeletionIfAbsent(ctx, postgresgen.InsertOAuthClientDeletionIfAbsentParams{
		ClientID: del.ClientID, Reason: string(del.Reason), DeletedByUserID: toNullString(del.DeletedByUserID), DeletedAt: del.DeletedAt,
	})
	return n == 1, wrap(err, "insert oauth client deletion")
}

// UpsertTokenFamilyLastUsed implements identity.Repository.
func (r *IdentityRepository) UpsertTokenFamilyLastUsed(ctx context.Context, tx uow.Tx, familyID string, at time.Time) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertTokenFamilyLastUsed(ctx, postgresgen.UpsertTokenFamilyLastUsedParams{FamilyID: familyID, LastUsedAt: at})
	return wrap(err, "record the last use of a token family")
}

// ListAuthorizedAppTokens implements identity.Repository.
func (r *IdentityRepository) ListAuthorizedAppTokens(ctx context.Context, userID string, now time.Time) ([]identity.AuthorizedAppToken, error) {
	rows, err := r.db.reader().ListAuthorizedAppTokens(ctx, postgresgen.ListAuthorizedAppTokensParams{UserID: userID, Now: now})
	if err != nil {
		return nil, wrap(err, "list authorized app tokens")
	}
	out := make([]identity.AuthorizedAppToken, len(rows))
	for i, row := range rows {
		t := identity.AuthorizedAppToken{
			FamilyID: row.FamilyID, Scope: identity.Scope(row.TokenScope), AuthorizedAt: utc(row.AuthorizedAt),
			Client: oauthClientFromRow(postgresgen.OauthClient{
				ID: row.ID, ClientID: row.ClientID, ClientSecretHash: row.ClientSecretHash, Name: row.Name, Website: row.Website,
				Scope: row.Scope, Registration: row.Registration, OwnerUserID: row.OwnerUserID, CreatedAt: row.CreatedAt,
			}),
		}
		if row.LastUsedAt != nil {
			u := utc(*row.LastUsedAt)
			t.LastUsedAt = &u
		}
		out[i] = t
	}
	return out, nil
}

// RevokeTokensOfUserAndClient implements identity.Repository.
func (r *IdentityRepository) RevokeTokensOfUserAndClient(ctx context.Context, tx uow.Tx, userID, clientID string, reason identity.TokenRevocationReason, at time.Time) (int64, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.RevokeTokensOfUserAndClient(ctx, postgresgen.RevokeTokensOfUserAndClientParams{Reason: string(reason), RevokedAt: at, UserID: userID, ClientID: clientID})
	return n, wrap(err, "revoke the tokens of a user and a client")
}
