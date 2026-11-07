// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertSession implements identity.Repository.
func (r *Repository) InsertSession(ctx context.Context, tx uow.Tx, s identity.Session) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSession(ctx, identitygen.InsertSessionParams{
		ID: s.ID, UserID: s.UserID, TokenHash: s.TokenHash,
		CreatedAt: sqlite.ToMillis(s.CreatedAt), ExpiresAt: sqlite.ToMillis(s.ExpiresAt),
	})
	return sqlite.Wrap(err, "insert session")
}

// InsertSessionRevocation implements identity.Repository.
func (r *Repository) InsertSessionRevocation(ctx context.Context, tx uow.Tx, rev identity.SessionRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSessionRevocationIfAbsent(ctx, identitygen.InsertSessionRevocationIfAbsentParams{
		SessionID: rev.SessionID, Reason: string(rev.Reason), RevokedAt: sqlite.ToMillis(rev.RevokedAt),
	})
	return sqlite.Wrap(err, "insert session revocation")
}

// UpsertSessionActivity implements identity.Repository.
func (r *Repository) UpsertSessionActivity(ctx context.Context, tx uow.Tx, a identity.SessionActivity) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertSessionActivity(ctx, identitygen.UpsertSessionActivityParams{SessionID: a.SessionID, LastSeenAt: sqlite.ToMillis(a.LastSeenAt)})
	return sqlite.Wrap(err, "upsert session activity")
}

// FindUnrevokedSession implements identity.Repository.
func (r *Repository) FindUnrevokedSession(ctx context.Context, tokenHash []byte) (identity.Session, identity.SessionActivity, error) {
	row, err := r.db.reader().GetUnrevokedSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return identity.Session{}, identity.SessionActivity{}, sqlite.NotFound(err, appidentity.ErrNotFound, "session")
	}
	s := identity.Session{
		ID: row.ID, UserID: row.UserID, TokenHash: row.TokenHash,
		CreatedAt: sqlite.FromMillis(row.CreatedAt), ExpiresAt: sqlite.FromMillis(row.ExpiresAt),
	}
	a := identity.SessionActivity{SessionID: row.ID}
	if row.LastSeenAt.Valid {
		a.LastSeenAt = sqlite.FromMillis(row.LastSeenAt.Int64)
	}
	return s, a, nil
}

// RevokeSessionsOfUser implements identity.Repository.
func (r *Repository) RevokeSessionsOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.RevocationReason, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.RevokeSessionsOfUser(ctx, identitygen.RevokeSessionsOfUserParams{Reason: string(reason), RevokedAt: sqlite.ToMillis(at), UserID: userID})
	return sqlite.Wrap(err, "revoke the sessions of a user")
}
