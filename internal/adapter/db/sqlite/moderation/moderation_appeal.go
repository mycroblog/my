// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.AppealRepository = (*Repository)(nil)

// InsertAppeal implements moderation.AppealRepository.
func (r *Repository) InsertAppeal(ctx context.Context, tx uow.Tx, a moderation.Appeal) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertModerationAppeal(ctx, moderationgen.InsertModerationAppealParams{
		ID: a.ID, ActionID: a.ActionID, AccountID: a.AccountID, Body: a.Text, SubmittedAt: sqlite.ToMillis(a.SubmittedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert moderation appeal")
	}
	if n == 0 {
		return fmt.Errorf("appeal of moderation action %s: %w", a.ActionID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// InsertAppealDecision implements moderation.AppealRepository.
func (r *Repository) InsertAppealDecision(ctx context.Context, tx uow.Tx, d moderation.AppealDecision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertModerationAppealDecision(ctx, moderationgen.InsertModerationAppealDecisionParams{
		AppealID: d.AppealID, Decision: string(d.Decision), PublicReason: sqlite.ToNullString(d.PublicReason),
		DecidedByUserID: sqlite.ToNullString(d.DecidedByUserID), DecidedAt: sqlite.ToMillis(d.DecidedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert moderation appeal decision")
	}
	if n == 0 {
		return fmt.Errorf("decision on appeal %s: %w", d.AppealID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// UpsertAppealStatus implements moderation.AppealRepository.
func (r *Repository) UpsertAppealStatus(ctx context.Context, tx uow.Tx, st moderation.AppealStatus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertAppealState(ctx, moderationgen.UpsertAppealStateParams{
		AppealID: st.AppealID, State: string(st.State), Since: sqlite.ToMillis(st.Since),
	}), "upsert appeal state")
}

// Appeal implements moderation.AppealRepository.
func (r *Repository) Appeal(ctx context.Context, tx uow.Tx, id string) (moderation.AppealRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.AppealRecord{}, err
	}
	return getAppeal(ctx, q, id)
}

// FindAppeal implements moderation.AppealRepository.
func (r *Repository) FindAppeal(ctx context.Context, id string) (moderation.AppealRecord, error) {
	return getAppeal(ctx, r.db.reader(), id)
}

func getAppeal(ctx context.Context, q *moderationgen.Queries, id string) (moderation.AppealRecord, error) {
	row, err := q.GetModerationAppeal(ctx, id)
	if err != nil {
		return moderation.AppealRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "moderation appeal "+id)
	}
	return appealRecord(row), nil
}

// AppealsOfActions implements moderation.AppealRepository.
func (r *Repository) AppealsOfActions(ctx context.Context, actionIDs []string) ([]moderation.AppealRecord, error) {
	if len(actionIDs) == 0 {
		return nil, nil
	}
	ids, err := sqlite.JSONList(actionIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListModerationAppealsOfActions(ctx, ids)
	if err != nil {
		return nil, sqlite.Wrap(err, "list the appeals of moderation actions")
	}
	out := make([]moderation.AppealRecord, len(rows))
	for i, row := range rows {
		out[i] = appealRecord(moderationgen.GetModerationAppealRow(row))
	}
	return out, nil
}

// ListAppeals implements moderation.AppealRepository.
func (r *Repository) ListAppeals(ctx context.Context, q appmoderation.AppealQuery, limit int) ([]moderation.AppealRecord, error) {
	rows, err := r.db.reader().ListModerationAppeals(ctx, moderationgen.ListModerationAppealsParams{
		State: string(q.State), Before: q.Before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list moderation appeals")
	}
	out := make([]moderation.AppealRecord, len(rows))
	for i, row := range rows {
		out[i] = appealRecord(moderationgen.GetModerationAppealRow(row))
	}
	return out, nil
}

// CountAppeals implements moderation.AppealRepository.
func (r *Repository) CountAppeals(ctx context.Context, state moderation.AppealState) (int64, error) {
	n, err := r.db.reader().CountAppealsInState(ctx, string(state))
	return n, sqlite.Wrap(err, "count appeals")
}

func appealRecord(row moderationgen.GetModerationAppealRow) moderation.AppealRecord {
	rec := moderation.AppealRecord{Appeal: moderation.Appeal{
		ID: row.ID, ActionID: row.ActionID, AccountID: row.AccountID, Text: row.Body, SubmittedAt: sqlite.FromMillis(row.SubmittedAt),
	}}
	if row.DecisionAppealID.Valid {
		rec.Decision = &moderation.AppealDecision{
			AppealID: row.DecisionAppealID.String, Decision: moderation.AppealDecisionKind(row.Decision.String),
			PublicReason: row.PublicReason.String, DecidedByUserID: row.DecidedByUserID.String,
			DecidedAt: sqlite.FromMillis(row.DecidedAt.Int64),
		}
	}
	return rec
}
