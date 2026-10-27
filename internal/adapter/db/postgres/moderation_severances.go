// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertSeverance implements moderation.SeveranceRepository.
func (r *ModerationRepository) InsertSeverance(ctx context.Context, tx uow.Tx, s moderation.Severance) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	followers, err := toInt32(s.FollowersLost, "followers lost")
	if err != nil {
		return false, err
	}
	following, err := toInt32(s.FollowingLost, "following lost")
	if err != nil {
		return false, err
	}
	n, err := q.InsertRelationshipSeverance(ctx, postgresgen.InsertRelationshipSeveranceParams{
		ID: s.ID, DomainActionID: toNullString(s.DomainActionID), ActionID: toNullString(s.ActionID), AccountID: s.AccountID,
		FollowersLost: followers, FollowingLost: following, OccurredAt: s.OccurredAt,
	})
	if err != nil {
		return false, wrap(err, "insert relationship severance")
	}
	return n > 0, nil
}

// FindSeverance implements moderation.SeveranceRepository.
func (r *ModerationRepository) FindSeverance(ctx context.Context, accountID, id string) (moderation.SeveranceRecord, error) {
	row, err := r.db.reader().GetRelationshipSeverance(ctx, postgresgen.GetRelationshipSeveranceParams{ID: id, AccountID: accountID})
	if err != nil {
		return moderation.SeveranceRecord{}, notFound(err, appmoderation.ErrNotFound, "relationship severance "+id)
	}
	return moderation.SeveranceRecord{
		Severance: moderation.Severance{
			ID: row.ID, DomainActionID: fromNullString(row.DomainActionID), ActionID: fromNullString(row.ActionID), AccountID: row.AccountID,
			FollowersLost: int(row.FollowersLost), FollowingLost: int(row.FollowingLost), OccurredAt: utc(row.OccurredAt),
		},
		Domain: fromNullString(row.TargetDomain), TargetAccountID: fromNullString(row.TargetAccountID),
	}, nil
}
