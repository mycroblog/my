// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// currentOAuthClient returns c as its newest revision and secret rotation make it. Only the
// clients users register have either, so the others are returned as they are.
func currentOAuthClient(ctx context.Context, q *identitygen.Queries, c identity.OAuthClient) (identity.OAuthClient, error) {
	if c.Registration != identity.ClientRegisteredByUser {
		return c, nil
	}
	rev, err := q.GetLatestOAuthClientRevision(ctx, c.ID)
	switch {
	case err == nil:
		c = c.Revised(identity.OAuthClientRevision{
			ID: rev.ID, ClientID: rev.OauthClientID, Name: rev.Name, Website: rev.Website.String, Scope: identity.Scope(rev.Scope),
			RevisedAt: sqlite.FromMillis(rev.RevisedAt),
		})
	case !errors.Is(err, sql.ErrNoRows):
		return identity.OAuthClient{}, sqlite.Wrap(err, "get oauth client revision")
	}
	rot, err := q.GetLatestOAuthClientSecretRotation(ctx, c.ID)
	switch {
	case err == nil:
		c = c.WithSecret(identity.OAuthClientSecretRotation{
			ID: rot.ID, ClientID: rot.OauthClientID, SecretHash: rot.SecretHash, RotatedAt: sqlite.FromMillis(rot.RotatedAt),
		})
	case !errors.Is(err, sql.ErrNoRows):
		return identity.OAuthClient{}, sqlite.Wrap(err, "get oauth client secret rotation")
	}
	return c, nil
}

// InsertOAuthClient implements identity.Repository.
func (r *Repository) InsertOAuthClient(ctx context.Context, tx uow.Tx, c identity.OAuthClient) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOAuthClient(ctx, identitygen.InsertOAuthClientParams{
		ID: c.ID, ClientID: c.ClientID, ClientSecretHash: c.SecretHash, Name: c.Name, Website: sqlite.ToNullString(c.Website),
		Scope: string(c.Scope), Registration: string(c.Registration), OwnerUserID: sqlite.ToNullString(c.OwnerUserID),
		CreatedAt: sqlite.ToMillis(c.CreatedAt),
	})
	return sqlite.Wrap(err, "insert oauth client")
}

// ListOwnedOAuthClients implements identity.Repository.
func (r *Repository) ListOwnedOAuthClients(ctx context.Context, ownerUserID string) ([]identity.OAuthClient, error) {
	q := r.db.reader()
	rows, err := q.ListOAuthClientsOfOwner(ctx, sqlite.ToNullString(ownerUserID))
	if err != nil {
		return nil, sqlite.Wrap(err, "list oauth clients of owner")
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
func (r *Repository) CountOwnedOAuthClients(ctx context.Context, tx uow.Tx, ownerUserID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountOAuthClientsOfOwner(ctx, sqlite.ToNullString(ownerUserID))
	return int(n), sqlite.Wrap(err, "count oauth clients of owner")
}

// InsertOAuthClientRevision implements identity.Repository.
func (r *Repository) InsertOAuthClientRevision(ctx context.Context, tx uow.Tx, rev identity.OAuthClientRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOAuthClientRevision(ctx, identitygen.InsertOAuthClientRevisionParams{
		ID: rev.ID, OauthClientID: rev.ClientID, Name: rev.Name, Website: sqlite.ToNullString(rev.Website), Scope: string(rev.Scope),
		RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	return sqlite.Wrap(err, "insert oauth client revision")
}

// InsertOAuthClientSecretRotation implements identity.Repository.
func (r *Repository) InsertOAuthClientSecretRotation(ctx context.Context, tx uow.Tx, rot identity.OAuthClientSecretRotation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOAuthClientSecretRotation(ctx, identitygen.InsertOAuthClientSecretRotationParams{
		ID: rot.ID, OauthClientID: rot.ClientID, SecretHash: rot.SecretHash, RotatedAt: sqlite.ToMillis(rot.RotatedAt),
	})
	return sqlite.Wrap(err, "insert oauth client secret rotation")
}

// RevokeTokensOfOwnedClients implements identity.Repository.
func (r *Repository) RevokeTokensOfOwnedClients(ctx context.Context, tx uow.Tx, ownerUserID string, reason identity.TokenRevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeTokensOfOwnedClients(ctx, identitygen.RevokeTokensOfOwnedClientsParams{
		Reason: string(reason), RevokedAt: sqlite.ToMillis(at), OwnerUserID: sqlite.ToNullString(ownerUserID),
	})
	return sqlite.Wrap(err, "revoke the tokens of the clients of a user")
}
