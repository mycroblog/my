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

// InsertSessionOrigin implements identity.OriginRepository.
func (r *IdentityRepository) InsertSessionOrigin(ctx context.Context, tx uow.Tx, o identity.SessionOrigin) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSessionOrigin(ctx, sqlitegen.InsertSessionOriginParams{
		SessionID: o.SessionID, UserID: o.UserID, Ip: toNullString(o.IP), UserAgent: toNullString(o.UserAgent),
		CreatedAt: toMillis(o.CreatedAt),
	})
	return wrap(err, "insert session origin")
}

// InsertRegistrationOrigin implements identity.OriginRepository.
func (r *IdentityRepository) InsertRegistrationOrigin(ctx context.Context, tx uow.Tx, o identity.RegistrationOrigin) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationOrigin(ctx, sqlitegen.InsertRegistrationOriginParams{
		UserID: o.UserID, Ip: o.IP, RecordedAt: toMillis(o.RecordedAt),
	})
	return wrap(err, "insert registration origin")
}

// FindSessionOrigin implements identity.OriginRepository.
func (r *IdentityRepository) FindSessionOrigin(ctx context.Context, sessionID string) (identity.SessionOrigin, error) {
	row, err := r.db.reader().GetSessionOrigin(ctx, sessionID)
	if err != nil {
		return identity.SessionOrigin{}, notFound(err, appidentity.ErrNotFound, "session origin")
	}
	return identity.SessionOrigin{
		SessionID: row.SessionID, UserID: row.UserID, IP: row.Ip.String, UserAgent: row.UserAgent.String,
		CreatedAt: fromMillis(row.CreatedAt),
	}, nil
}

// FindRegistrationOrigin implements identity.OriginRepository.
func (r *IdentityRepository) FindRegistrationOrigin(ctx context.Context, userID string) (identity.RegistrationOrigin, error) {
	row, err := r.db.reader().GetRegistrationOrigin(ctx, userID)
	if err != nil {
		return identity.RegistrationOrigin{}, notFound(err, appidentity.ErrNotFound, "registration origin")
	}
	return identity.RegistrationOrigin{UserID: row.UserID, IP: row.Ip, RecordedAt: fromMillis(row.RecordedAt)}, nil
}

// DeleteSessionOriginsBefore implements identity.OriginRepository.
func (r *IdentityRepository) DeleteSessionOriginsBefore(ctx context.Context, tx uow.Tx, t time.Time, except []string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := jsonList(except)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteSessionOriginsBefore(ctx, sqlitegen.DeleteSessionOriginsBeforeParams{
		CreatedBefore: toMillis(t), ExceptUserIds: ids, BatchLimit: int64(limit),
	})
	return n, wrap(err, "delete session origins")
}

// DeleteRegistrationOriginsBefore implements identity.OriginRepository.
func (r *IdentityRepository) DeleteRegistrationOriginsBefore(ctx context.Context, tx uow.Tx, t time.Time, except []string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := jsonList(except)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteRegistrationOriginsBefore(ctx, sqlitegen.DeleteRegistrationOriginsBeforeParams{
		RecordedBefore: toMillis(t), ExceptUserIds: ids, BatchLimit: int64(limit),
	})
	return n, wrap(err, "delete registration origins")
}
