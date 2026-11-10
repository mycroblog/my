// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"encoding/json"

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
		ID: s.ID, DomainActionID: sqlite.ToNullString(s.DomainActionID), ActionID: sqlite.ToNullString(s.ActionID), AllowlistCutoffID: sqlite.ToNullString(s.AllowlistCutoffID), AccountID: s.AccountID,
		FollowersLost: int64(s.FollowersLost), FollowingLost: int64(s.FollowingLost), OccurredAt: sqlite.ToMillis(s.OccurredAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "insert relationship severance")
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
	type row struct {
		ID            string `json:"id"`
		AccountID     string `json:"account_id"`
		FollowersLost int    `json:"followers_lost"`
		FollowingLost int    `json:"following_lost"`
	}
	rows := make([]row, len(ss))
	for i, s := range ss {
		rows[i] = row{ID: s.ID, AccountID: s.AccountID, FollowersLost: s.FollowersLost, FollowingLost: s.FollowingLost}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	inserted, err := q.InsertRelationshipSeverances(ctx, moderationgen.InsertRelationshipSeverancesParams{
		DomainActionID: sqlite.ToNullString(ss[0].DomainActionID), ActionID: sqlite.ToNullString(ss[0].ActionID), AllowlistCutoffID: sqlite.ToNullString(ss[0].AllowlistCutoffID),
		OccurredAt: sqlite.ToMillis(ss[0].OccurredAt), Severances: string(list),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "insert relationship severances")
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
		return moderation.SeveranceRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "relationship severance "+id)
	}
	return severanceFrom(row), nil
}

// severanceFrom returns the severance of row with what it severed.
func severanceFrom(row moderationgen.GetRelationshipSeveranceRow) moderation.SeveranceRecord {
	return moderation.SeveranceRecord{
		Severance: moderation.Severance{
			ID: row.ID, DomainActionID: row.DomainActionID.String, ActionID: row.ActionID.String, AllowlistCutoffID: row.AllowlistCutoffID.String, AccountID: row.AccountID,
			FollowersLost: int(row.FollowersLost), FollowingLost: int(row.FollowingLost), OccurredAt: sqlite.FromMillis(row.OccurredAt),
		},
		Domain: row.TargetDomain, TargetAccountID: row.TargetAccountID.String,
	}
}
