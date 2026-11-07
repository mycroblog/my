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

// ListUnliftedActions implements moderation.Repository.
func (r *Repository) ListUnliftedActions(ctx context.Context, tx uow.Tx) ([]moderation.Action, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListUnliftedModerationActions(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list unlifted moderation actions")
	}
	out := make([]moderation.Action, len(rows))
	for i, row := range rows {
		out[i] = moderation.Action{
			ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind), ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt),
		}
	}
	return out, nil
}

// ListUnliftedDomainActions implements moderation.Repository.
func (r *Repository) ListUnliftedDomainActions(ctx context.Context, tx uow.Tx) ([]moderation.DomainAction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListUnliftedDomainModerationActions(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list unlifted domain moderation actions")
	}
	out := make([]moderation.DomainAction, len(rows))
	for i, row := range rows {
		out[i] = moderation.DomainAction{
			ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt),
		}
	}
	return out, nil
}

// ListUnliftedActionStatuses implements moderation.Repository.
func (r *Repository) ListUnliftedActionStatuses(ctx context.Context, tx uow.Tx, kind moderation.ActionKind) ([]appmoderation.ActionStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListUnliftedModerationActionStatuses(ctx, string(kind))
	if err != nil {
		return nil, sqlite.Wrap(err, "list unlifted moderation action statuses")
	}
	out := make([]appmoderation.ActionStatus, len(rows))
	for i, row := range rows {
		out[i] = appmoderation.ActionStatus{ActionID: row.ActionID, StatusID: row.StatusID}
	}
	return out, nil
}

// ListAllAccountRestrictionRows implements moderation.Repository.
func (r *Repository) ListAllAccountRestrictionRows(ctx context.Context, tx uow.Tx) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllAccountRestrictionRows(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// ListAllDomainRestrictionRows implements moderation.Repository.
func (r *Repository) ListAllDomainRestrictionRows(ctx context.Context, tx uow.Tx) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllDomainRestrictionRows(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.Domain, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// DeleteAllAccountRestrictions implements moderation.Repository.
func (r *Repository) DeleteAllAccountRestrictions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty account restrictions", (*moderationgen.Queries).DeleteAllAccountRestrictions)
}

// DeleteAllDomainRestrictions implements moderation.Repository.
func (r *Repository) DeleteAllDomainRestrictions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty domain restrictions", (*moderationgen.Queries).DeleteAllDomainRestrictions)
}

// RebuildReportStates implements moderation.Repository.
func (r *Repository) RebuildReportStates(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild report states", (*moderationgen.Queries).DeleteAllReportStates, (*moderationgen.Queries).RebuildReportStates)
}

// RebuildAppealStates implements moderation.Repository.
func (r *Repository) RebuildAppealStates(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild appeal states", (*moderationgen.Queries).DeleteAllAppealStates, (*moderationgen.Queries).RebuildAppealStates)
}
