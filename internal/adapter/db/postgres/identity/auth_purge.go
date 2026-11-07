// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
)

// DeleteExpiredTokenFamilies implements identity.AuthRecordPurgeRepository. The children go before
// their parents, and the list and the deletes share tx.
func (r *Repository) DeleteExpiredTokenFamilies(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredTokenFamilies(ctx, identitygen.ListExpiredTokenFamiliesParams{ExpiredBefore: t, BatchLimit: batch})
	if err != nil {
		return 0, postgres.Wrap(err, "list expired token families")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return int64(len(ids)), postgres.RunSteps(ctx, ids, "delete token families",
		q.DeleteCodesOfTokenFamilies, q.DeleteCodeRedemptionsOfTokenFamilies, q.DeleteRevocationsOfTokenFamilies,
		q.DeleteLastUsedOfTokenFamilies, q.DeleteTokensOfTokenFamilies)
}

// DeleteExpiredUnredeemedCodes implements identity.AuthRecordPurgeRepository.
func (r *Repository) DeleteExpiredUnredeemedCodes(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteExpiredUnredeemedCodes(ctx, identitygen.DeleteExpiredUnredeemedCodesParams{ExpiredBefore: t, BatchLimit: batch})
	return n, postgres.Wrap(err, "delete expired authorization codes")
}

// DeleteExpiredSessions implements identity.AuthRecordPurgeRepository.
func (r *Repository) DeleteExpiredSessions(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredSessions(ctx, identitygen.ListExpiredSessionsParams{ExpiredBefore: t, BatchLimit: batch})
	if err != nil {
		return 0, postgres.Wrap(err, "list expired sessions")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return int64(len(ids)), postgres.RunSteps(ctx, ids, "delete sessions",
		q.DeleteRevocationsOfSessions, q.DeleteActivityOfSessions, q.DeleteSessions)
}

// DeleteExpiredUserTokens implements identity.AuthRecordPurgeRepository.
func (r *Repository) DeleteExpiredUserTokens(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredUserTokens(ctx, identitygen.ListExpiredUserTokensParams{ExpiredBefore: t, BatchLimit: batch})
	if err != nil {
		return 0, postgres.Wrap(err, "list expired user tokens")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return int64(len(ids)), postgres.RunSteps(ctx, ids, "delete user tokens",
		q.DeleteRedemptionsOfUserTokens, q.DeleteUserTokens)
}
