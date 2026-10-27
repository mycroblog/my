// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	err = q.InsertSession(ctx, postgresgen.InsertSessionParams{
		ID: s.ID, UserID: s.UserID, TokenHash: s.TokenHash,
		CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt,
	})
	return wrap(err, "insert session")
}

// InsertSessionRevocation implements identity.Repository.
func (r *IdentityRepository) InsertSessionRevocation(ctx context.Context, tx uow.Tx, rev identity.SessionRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSessionRevocationIfAbsent(ctx, postgresgen.InsertSessionRevocationIfAbsentParams{
		SessionID: rev.SessionID, Reason: string(rev.Reason), RevokedAt: rev.RevokedAt,
	})
	return wrap(err, "insert session revocation")
}

// UpsertSessionActivity implements identity.Repository.
func (r *IdentityRepository) UpsertSessionActivity(ctx context.Context, tx uow.Tx, a identity.SessionActivity) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertSessionActivity(ctx, postgresgen.UpsertSessionActivityParams{SessionID: a.SessionID, LastSeenAt: a.LastSeenAt})
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
		CreatedAt: utc(row.CreatedAt), ExpiresAt: utc(row.ExpiresAt),
	}
	a := identity.SessionActivity{SessionID: row.ID}
	if row.LastSeenAt != nil {
		a.LastSeenAt = utc(*row.LastSeenAt)
	}
	return s, a, nil
}

// RevokeSessionsOfUser implements identity.Repository.
func (r *IdentityRepository) RevokeSessionsOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.RevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeSessionsOfUser(ctx, postgresgen.RevokeSessionsOfUserParams{Reason: string(reason), RevokedAt: at, UserID: userID})
	return wrap(err, "revoke the sessions of a user")
}
