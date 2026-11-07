// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
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
		SessionID: o.SessionID, UserID: o.UserID, Ip: postgres.ToNullString(o.IP), UserAgent: postgres.ToNullString(o.UserAgent),
		CreatedAt: o.CreatedAt,
	})
	return postgres.Wrap(err, "insert session origin")
}

// InsertRegistrationOrigin implements identity.OriginRepository.
func (r *Repository) InsertRegistrationOrigin(ctx context.Context, tx uow.Tx, o identity.RegistrationOrigin) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationOrigin(ctx, identitygen.InsertRegistrationOriginParams{
		UserID: o.UserID, Ip: o.IP, RecordedAt: o.RecordedAt,
	})
	return postgres.Wrap(err, "insert registration origin")
}

// FindSessionOrigin implements identity.OriginRepository.
func (r *Repository) FindSessionOrigin(ctx context.Context, sessionID string) (identity.SessionOrigin, error) {
	row, err := r.db.reader().GetSessionOrigin(ctx, sessionID)
	if err != nil {
		return identity.SessionOrigin{}, postgres.NotFound(err, appidentity.ErrNotFound, "session origin")
	}
	return identity.SessionOrigin{
		SessionID: row.SessionID, UserID: row.UserID, IP: postgres.FromNullString(row.Ip), UserAgent: postgres.FromNullString(row.UserAgent),
		CreatedAt: postgres.UTC(row.CreatedAt),
	}, nil
}

// FindRegistrationOrigin implements identity.OriginRepository.
func (r *Repository) FindRegistrationOrigin(ctx context.Context, userID string) (identity.RegistrationOrigin, error) {
	row, err := r.db.reader().GetRegistrationOrigin(ctx, userID)
	if err != nil {
		return identity.RegistrationOrigin{}, postgres.NotFound(err, appidentity.ErrNotFound, "registration origin")
	}
	return identity.RegistrationOrigin{UserID: row.UserID, IP: row.Ip, RecordedAt: postgres.UTC(row.RecordedAt)}, nil
}

// DeleteSessionOriginsBefore implements identity.OriginRepository.
func (r *Repository) DeleteSessionOriginsBefore(ctx context.Context, tx uow.Tx, t time.Time, except []string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteSessionOriginsBefore(ctx, identitygen.DeleteSessionOriginsBeforeParams{
		CreatedBefore: t, ExceptUserIds: postgres.TextArray(except), BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete session origins")
}

// DeleteRegistrationOriginsBefore implements identity.OriginRepository.
func (r *Repository) DeleteRegistrationOriginsBefore(ctx context.Context, tx uow.Tx, t time.Time, except []string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteRegistrationOriginsBefore(ctx, identitygen.DeleteRegistrationOriginsBeforeParams{
		RecordedBefore: t, ExceptUserIds: postgres.TextArray(except), BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete registration origins")
}
