// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
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
	n, err := q.InsertRelationshipSeverance(ctx, moderationgen.InsertRelationshipSeveranceParams{
		ID: s.ID, DomainActionID: sqlite.ToNullString(s.DomainActionID), ActionID: sqlite.ToNullString(s.ActionID), AccountID: s.AccountID,
		FollowersLost: int64(s.FollowersLost), FollowingLost: int64(s.FollowingLost), OccurredAt: sqlite.ToMillis(s.OccurredAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "insert relationship severance")
	}
	return n > 0, nil
}

// FindSeverance implements moderation.SeveranceRepository.
func (r *Repository) FindSeverance(ctx context.Context, accountID, id string) (moderation.SeveranceRecord, error) {
	row, err := r.db.reader().GetRelationshipSeverance(ctx, moderationgen.GetRelationshipSeveranceParams{ID: id, AccountID: accountID})
	if err != nil {
		return moderation.SeveranceRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "relationship severance "+id)
	}
	return severanceFrom(row), nil
}

// severanceFrom returns the severance of row with what it severed.
func severanceFrom(row moderationgen.GetRelationshipSeveranceRow) moderation.SeveranceRecord {
	return moderation.SeveranceRecord{
		Severance: moderation.Severance{
			ID: row.ID, DomainActionID: row.DomainActionID.String, ActionID: row.ActionID.String, AccountID: row.AccountID,
			FollowersLost: int(row.FollowersLost), FollowingLost: int(row.FollowingLost), OccurredAt: sqlite.FromMillis(row.OccurredAt),
		},
		Domain: row.TargetDomain.String, TargetAccountID: row.TargetAccountID.String,
	}
}
