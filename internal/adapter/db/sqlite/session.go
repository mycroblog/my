// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertSession implements identity.Repository.
func (r *IdentityRepository) InsertSession(ctx context.Context, tx uow.Tx, s identity.Session) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSession(ctx, sqlitegen.InsertSessionParams{
		ID: s.ID, UserID: s.UserID, TokenHash: s.TokenHash,
		CreatedAt: toMillis(s.CreatedAt), ExpiresAt: toMillis(s.ExpiresAt),
	})
	return wrap(err, "insert session")
}

// InsertSessionRevocation implements identity.Repository.
func (r *IdentityRepository) InsertSessionRevocation(ctx context.Context, tx uow.Tx, rev identity.SessionRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSessionRevocationIfAbsent(ctx, sqlitegen.InsertSessionRevocationIfAbsentParams{
		SessionID: rev.SessionID, Reason: string(rev.Reason), RevokedAt: toMillis(rev.RevokedAt),
	})
	return wrap(err, "insert session revocation")
}

// UpsertSessionActivity implements identity.Repository.
func (r *IdentityRepository) UpsertSessionActivity(ctx context.Context, tx uow.Tx, a identity.SessionActivity) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertSessionActivity(ctx, sqlitegen.UpsertSessionActivityParams{SessionID: a.SessionID, LastSeenAt: toMillis(a.LastSeenAt)})
	return wrap(err, "upsert session activity")
}

// FindUnrevokedSession implements identity.Repository.
func (r *IdentityRepository) FindUnrevokedSession(ctx context.Context, tokenHash []byte) (identity.Session, identity.SessionActivity, error) {
	row, err := r.db.reader().GetUnrevokedSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return identity.Session{}, identity.SessionActivity{}, notFound(err, appidentity.ErrNotFound, "session")
	}
	s := identity.Session{
		ID: row.ID, UserID: row.UserID, TokenHash: row.TokenHash,
		CreatedAt: fromMillis(row.CreatedAt), ExpiresAt: fromMillis(row.ExpiresAt),
	}
	a := identity.SessionActivity{SessionID: row.ID}
	if row.LastSeenAt.Valid {
		a.LastSeenAt = fromMillis(row.LastSeenAt.Int64)
	}
	return s, a, nil
}

// RevokeSessionsOfUser implements identity.Repository.
func (r *IdentityRepository) RevokeSessionsOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.RevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeSessionsOfUser(ctx, sqlitegen.RevokeSessionsOfUserParams{Reason: string(reason), RevokedAt: toMillis(at), UserID: userID})
	return wrap(err, "revoke the sessions of a user")
}
