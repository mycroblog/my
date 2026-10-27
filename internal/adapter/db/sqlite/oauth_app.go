// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// currentOAuthClient returns c as its newest revision and secret rotation make it. Only the
// clients users register have either, so the others are returned as they are.
func currentOAuthClient(ctx context.Context, q *sqlitegen.Queries, c identity.OAuthClient) (identity.OAuthClient, error) {
	if c.Registration != identity.ClientRegisteredByUser {
		return c, nil
	}
	rev, err := q.GetLatestOAuthClientRevision(ctx, c.ID)
	switch {
	case err == nil:
		c = c.Revised(identity.OAuthClientRevision{
			ID: rev.ID, ClientID: rev.ClientID, Name: rev.Name, Website: rev.Website.String, Scope: identity.Scope(rev.Scope),
			RevisedAt: fromMillis(rev.RevisedAt),
		})
	case !errors.Is(err, sql.ErrNoRows):
		return identity.OAuthClient{}, wrap(err, "get oauth client revision")
	}
	rot, err := q.GetLatestOAuthClientSecretRotation(ctx, c.ID)
	switch {
	case err == nil:
		c = c.WithSecret(identity.OAuthClientSecretRotation{
			ID: rot.ID, ClientID: rot.ClientID, SecretHash: rot.SecretHash, RotatedAt: fromMillis(rot.RotatedAt),
		})
	case !errors.Is(err, sql.ErrNoRows):
		return identity.OAuthClient{}, wrap(err, "get oauth client secret rotation")
	}
	return c, nil
}

// InsertOAuthClient implements identity.Repository.
func (r *IdentityRepository) InsertOAuthClient(ctx context.Context, tx uow.Tx, c identity.OAuthClient) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOAuthClient(ctx, sqlitegen.InsertOAuthClientParams{
		ID: c.ID, ClientID: c.ClientID, ClientSecretHash: c.SecretHash, Name: c.Name, Website: toNullString(c.Website),
		Scope: string(c.Scope), Registration: string(c.Registration), OwnerUserID: toNullString(c.OwnerUserID),
		CreatedAt: toMillis(c.CreatedAt),
	})
	return wrap(err, "insert oauth client")
}

// ListOwnedOAuthClients implements identity.Repository.
func (r *IdentityRepository) ListOwnedOAuthClients(ctx context.Context, ownerUserID string) ([]identity.OAuthClient, error) {
	q := r.db.reader()
	rows, err := q.ListOAuthClientsOfOwner(ctx, toNullString(ownerUserID))
	if err != nil {
		return nil, wrap(err, "list oauth clients of owner")
	}
	out := make([]identity.OAuthClient, len(rows))
	for i, row := range rows {
		if out[i], err = currentOAuthClient(ctx, q, oauthClientFromRow(row)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CountOwnedOAuthClients implements identity.Repository.
func (r *IdentityRepository) CountOwnedOAuthClients(ctx context.Context, tx uow.Tx, ownerUserID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountOAuthClientsOfOwner(ctx, toNullString(ownerUserID))
	return int(n), wrap(err, "count oauth clients of owner")
}

// InsertOAuthClientRevision implements identity.Repository.
func (r *IdentityRepository) InsertOAuthClientRevision(ctx context.Context, tx uow.Tx, rev identity.OAuthClientRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOAuthClientRevision(ctx, sqlitegen.InsertOAuthClientRevisionParams{
		ID: rev.ID, ClientID: rev.ClientID, Name: rev.Name, Website: toNullString(rev.Website), Scope: string(rev.Scope),
		RevisedAt: toMillis(rev.RevisedAt),
	})
	return wrap(err, "insert oauth client revision")
}

// InsertOAuthClientSecretRotation implements identity.Repository.
func (r *IdentityRepository) InsertOAuthClientSecretRotation(ctx context.Context, tx uow.Tx, rot identity.OAuthClientSecretRotation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOAuthClientSecretRotation(ctx, sqlitegen.InsertOAuthClientSecretRotationParams{
		ID: rot.ID, ClientID: rot.ClientID, SecretHash: rot.SecretHash, RotatedAt: toMillis(rot.RotatedAt),
	})
	return wrap(err, "insert oauth client secret rotation")
}

// RevokeTokensOfOwnedClients implements identity.Repository.
func (r *IdentityRepository) RevokeTokensOfOwnedClients(ctx context.Context, tx uow.Tx, ownerUserID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokensOfOwnedClients(ctx, sqlitegen.RevokeTokensOfOwnedClientsParams{
		Reason: string(reason), RevokedAt: toMillis(at), OwnerUserID: toNullString(ownerUserID),
	})
	return wrap(err, "revoke the tokens of the clients of a user")
}
