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

// InsertSessionOrigin implements identity.OriginRepository.
func (r *Repository) InsertSessionOrigin(ctx context.Context, tx uow.Tx, o identity.SessionOrigin) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSessionOrigin(ctx, identitygen.InsertSessionOriginParams{
		SessionID: o.SessionID, UserID: o.UserID, Ip: sqlite.ToNullString(o.IP), UserAgent: sqlite.ToNullString(o.UserAgent),
		CreatedAt: sqlite.ToMillis(o.CreatedAt),
	})
	return sqlite.Wrap(err, "insert session origin")
}

// InsertRegistrationOrigin implements identity.OriginRepository.
func (r *Repository) InsertRegistrationOrigin(ctx context.Context, tx uow.Tx, o identity.RegistrationOrigin) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationOrigin(ctx, identitygen.InsertRegistrationOriginParams{
		UserID: o.UserID, Ip: o.IP, RecordedAt: sqlite.ToMillis(o.RecordedAt),
	})
	return sqlite.Wrap(err, "insert registration origin")
}

// FindSessionOrigin implements identity.OriginRepository.
func (r *Repository) FindSessionOrigin(ctx context.Context, sessionID string) (identity.SessionOrigin, error) {
	row, err := r.db.reader().GetSessionOrigin(ctx, sessionID)
	if err != nil {
		return identity.SessionOrigin{}, sqlite.NotFound(err, appidentity.ErrNotFound, "session origin")
	}
	return identity.SessionOrigin{
		SessionID: row.SessionID, UserID: row.UserID, IP: row.Ip.String, UserAgent: row.UserAgent.String,
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}, nil
}

// FindRegistrationOrigin implements identity.OriginRepository.
func (r *Repository) FindRegistrationOrigin(ctx context.Context, userID string) (identity.RegistrationOrigin, error) {
	row, err := r.db.reader().GetRegistrationOrigin(ctx, userID)
	if err != nil {
		return identity.RegistrationOrigin{}, sqlite.NotFound(err, appidentity.ErrNotFound, "registration origin")
	}
	return identity.RegistrationOrigin{UserID: row.UserID, IP: row.Ip, RecordedAt: sqlite.FromMillis(row.RecordedAt)}, nil
}

// DeleteSessionOriginsBefore implements identity.OriginRepository.
func (r *Repository) DeleteSessionOriginsBefore(ctx context.Context, tx uow.Tx, t time.Time, except []string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := sqlite.JSONList(except)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteSessionOriginsBefore(ctx, identitygen.DeleteSessionOriginsBeforeParams{
		CreatedBefore: sqlite.ToMillis(t), ExceptUserIds: ids, BatchLimit: int64(limit),
	})
	return n, sqlite.Wrap(err, "delete session origins")
}

// DeleteRegistrationOriginsBefore implements identity.OriginRepository.
func (r *Repository) DeleteRegistrationOriginsBefore(ctx context.Context, tx uow.Tx, t time.Time, except []string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := sqlite.JSONList(except)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteRegistrationOriginsBefore(ctx, identitygen.DeleteRegistrationOriginsBeforeParams{
		RecordedBefore: sqlite.ToMillis(t), ExceptUserIds: ids, BatchLimit: int64(limit),
	})
	return n, sqlite.Wrap(err, "delete registration origins")
}
