// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
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
		ID: a.ID, ActionID: a.ActionID, AccountID: a.AccountID, Body: a.Text, SubmittedAt: a.SubmittedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert moderation appeal")
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
		AppealID: d.AppealID, Decision: string(d.Decision), PublicReason: postgres.ToNullString(d.PublicReason),
		DecidedByUserID: postgres.ToNullString(d.DecidedByUserID), DecidedAt: d.DecidedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert moderation appeal decision")
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
	return postgres.Wrap(q.UpsertAppealState(ctx, moderationgen.UpsertAppealStateParams{
		AppealID: st.AppealID, State: string(st.State), Since: st.Since,
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
		return moderation.AppealRecord{}, postgres.NotFound(err, appmoderation.ErrNotFound, "moderation appeal "+id)
	}
	return appealRecord(row), nil
}

// AppealsOfActions implements moderation.AppealRepository.
func (r *Repository) AppealsOfActions(ctx context.Context, actionIDs []string) ([]moderation.AppealRecord, error) {
	if len(actionIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.reader().ListModerationAppealsOfActions(ctx, actionIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list the appeals of moderation actions")
	}
	out := make([]moderation.AppealRecord, len(rows))
	for i, row := range rows {
		out[i] = appealRecord(moderationgen.GetModerationAppealRow(row))
	}
	return out, nil
}

// ListAppeals implements moderation.AppealRepository.
func (r *Repository) ListAppeals(ctx context.Context, q appmoderation.AppealQuery, limit int) ([]moderation.AppealRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListModerationAppeals(ctx, moderationgen.ListModerationAppealsParams{
		State: string(q.State), Before: postgres.ToNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation appeals")
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
	return n, postgres.Wrap(err, "count appeals")
}

func appealRecord(row moderationgen.GetModerationAppealRow) moderation.AppealRecord {
	rec := moderation.AppealRecord{Appeal: moderation.Appeal{
		ID: row.ID, ActionID: row.ActionID, AccountID: row.AccountID, Text: row.Body, SubmittedAt: postgres.UTC(row.SubmittedAt),
	}}
	if row.DecisionAppealID != nil && row.DecidedAt != nil {
		rec.Decision = &moderation.AppealDecision{
			AppealID: *row.DecisionAppealID, Decision: moderation.AppealDecisionKind(postgres.FromNullString(row.Decision)),
			PublicReason: postgres.FromNullString(row.PublicReason), DecidedByUserID: postgres.FromNullString(row.DecidedByUserID),
			DecidedAt: postgres.UTC(*row.DecidedAt),
		}
	}
	return rec
}
