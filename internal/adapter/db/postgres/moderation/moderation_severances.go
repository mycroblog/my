// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertSeverance implements moderation.SeveranceRepository.
func (r *Repository) InsertSeverance(ctx context.Context, tx uow.Tx, s moderation.Severance) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	followers, err := postgres.ToInt32(s.FollowersLost, "followers lost")
	if err != nil {
		return false, err
	}
	following, err := postgres.ToInt32(s.FollowingLost, "following lost")
	if err != nil {
		return false, err
	}
	n, err := q.InsertRelationshipSeverance(ctx, moderationgen.InsertRelationshipSeveranceParams{
		ID: s.ID, DomainActionID: postgres.ToNullString(s.DomainActionID), ActionID: postgres.ToNullString(s.ActionID), AllowlistCutoffID: postgres.ToNullString(s.AllowlistCutoffID), AccountID: s.AccountID,
		FollowersLost: followers, FollowingLost: following, OccurredAt: s.OccurredAt,
	})
	if err != nil {
		return false, postgres.Wrap(err, "insert relationship severance")
	}
	return n > 0, nil
}

// InsertSeverances implements moderation.SeveranceRepository.
func (r *Repository) InsertSeverances(ctx context.Context, tx uow.Tx, ss []moderation.Severance) (map[string]bool, error) {
	out := make(map[string]bool, len(ss))
	if len(ss) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	arg := moderationgen.InsertRelationshipSeverancesParams{
		DomainActionID: postgres.ToNullString(ss[0].DomainActionID), ActionID: postgres.ToNullString(ss[0].ActionID), AllowlistCutoffID: postgres.ToNullString(ss[0].AllowlistCutoffID), OccurredAt: ss[0].OccurredAt,
	}
	for _, s := range ss {
		arg.Ids, arg.AccountIds = append(arg.Ids, s.ID), append(arg.AccountIds, s.AccountID)
		followers, err := postgres.ToInt32(s.FollowersLost, "followers lost")
		if err != nil {
			return nil, err
		}
		following, err := postgres.ToInt32(s.FollowingLost, "following lost")
		if err != nil {
			return nil, err
		}
		arg.FollowersLost, arg.FollowingLost = append(arg.FollowersLost, followers), append(arg.FollowingLost, following)
	}
	inserted, err := q.InsertRelationshipSeverances(ctx, arg)
	if err != nil {
		return nil, postgres.Wrap(err, "insert relationship severances")
	}
	for _, id := range inserted {
		out[id] = true
	}
	return out, nil
}

// FindSeverance implements moderation.SeveranceRepository.
func (r *Repository) FindSeverance(ctx context.Context, accountID, id string) (moderation.SeveranceRecord, error) {
	row, err := r.db.reader().GetRelationshipSeverance(ctx, moderationgen.GetRelationshipSeveranceParams{ID: id, AccountID: accountID})
	if err != nil {
		return moderation.SeveranceRecord{}, postgres.NotFound(err, appmoderation.ErrNotFound, "relationship severance "+id)
	}
	return severanceFrom(row), nil
}

// severanceFrom returns the severance of row with what it severed.
func severanceFrom(row moderationgen.GetRelationshipSeveranceRow) moderation.SeveranceRecord {
	return moderation.SeveranceRecord{
		Severance: moderation.Severance{
			ID: row.ID, DomainActionID: postgres.FromNullString(row.DomainActionID), ActionID: postgres.FromNullString(row.ActionID),
			AllowlistCutoffID: postgres.FromNullString(row.AllowlistCutoffID), AccountID: row.AccountID,
			FollowersLost: int(row.FollowersLost), FollowingLost: int(row.FollowingLost), OccurredAt: postgres.UTC(row.OccurredAt),
		},
		Domain: row.TargetDomain, TargetAccountID: postgres.FromNullString(row.TargetAccountID),
	}
}
