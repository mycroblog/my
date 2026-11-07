// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
)

// DeleteExpiredTokenFamilies implements identity.AuthRecordPurgeRepository. The children go before
// their parents, and the list and the deletes share tx.
func (r *IdentityRepository) DeleteExpiredTokenFamilies(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredTokenFamilies(ctx, identitygen.ListExpiredTokenFamiliesParams{ExpiredBefore: toMillis(t), BatchLimit: int64(limit)})
	if err != nil {
		return 0, wrap(err, "list expired token families")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return 0, err
	}
	return int64(len(ids)), runSteps(ctx, list, "delete token families",
		q.DeleteCodesOfTokenFamilies, q.DeleteCodeRedemptionsOfTokenFamilies, q.DeleteRevocationsOfTokenFamilies,
		q.DeleteLastUsedOfTokenFamilies, q.DeleteTokensOfTokenFamilies)
}

// DeleteExpiredUnredeemedCodes implements identity.AuthRecordPurgeRepository.
func (r *IdentityRepository) DeleteExpiredUnredeemedCodes(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteExpiredUnredeemedCodes(ctx, identitygen.DeleteExpiredUnredeemedCodesParams{ExpiredBefore: toMillis(t), BatchLimit: int64(limit)})
	return n, wrap(err, "delete expired authorization codes")
}

// DeleteExpiredSessions implements identity.AuthRecordPurgeRepository.
func (r *IdentityRepository) DeleteExpiredSessions(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredSessions(ctx, identitygen.ListExpiredSessionsParams{ExpiredBefore: toMillis(t), BatchLimit: int64(limit)})
	if err != nil {
		return 0, wrap(err, "list expired sessions")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return 0, err
	}
	return int64(len(ids)), runSteps(ctx, list, "delete sessions",
		q.DeleteRevocationsOfSessions, q.DeleteActivityOfSessions, q.DeleteSessions)
}

// DeleteExpiredUserTokens implements identity.AuthRecordPurgeRepository.
func (r *IdentityRepository) DeleteExpiredUserTokens(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := q.ListExpiredUserTokens(ctx, identitygen.ListExpiredUserTokensParams{ExpiredBefore: toMillis(t), BatchLimit: int64(limit)})
	if err != nil {
		return 0, wrap(err, "list expired user tokens")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return 0, err
	}
	return int64(len(ids)), runSteps(ctx, list, "delete user tokens",
		q.DeleteRedemptionsOfUserTokens, q.DeleteUserTokens)
}
