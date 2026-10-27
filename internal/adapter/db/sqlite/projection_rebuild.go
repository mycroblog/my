// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// This file holds the sources and the emptying of the projections of moderation, identity,
// content (the sensitivity overrides and the poll tallies), and timeline (the restrictions)
// that `my projection rebuild` derives again (data-model §9).

// execAll runs the queries in order inside tx, for a rebuild that empties a projection and
// derives it again in SQL.
func (d *DB) execAll(ctx context.Context, tx uow.Tx, what string, steps ...func(*sqlitegen.Queries, context.Context) error) error {
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := step(q, ctx); err != nil {
			return wrap(err, what)
		}
	}
	return nil
}

// ListUnliftedActions implements moderation.Repository.
func (r *ModerationRepository) ListUnliftedActions(ctx context.Context, tx uow.Tx) ([]moderation.Action, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListUnliftedModerationActions(ctx)
	if err != nil {
		return nil, wrap(err, "list unlifted moderation actions")
	}
	out := make([]moderation.Action, len(rows))
	for i, row := range rows {
		out[i] = moderation.Action{
			ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind), ExpiresAt: fromNullMillis(row.ExpiresAt),
		}
	}
	return out, nil
}

// ListUnliftedDomainActions implements moderation.Repository.
func (r *ModerationRepository) ListUnliftedDomainActions(ctx context.Context, tx uow.Tx) ([]moderation.DomainAction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListUnliftedDomainModerationActions(ctx)
	if err != nil {
		return nil, wrap(err, "list unlifted domain moderation actions")
	}
	out := make([]moderation.DomainAction, len(rows))
	for i, row := range rows {
		out[i] = moderation.DomainAction{
			ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), ExpiresAt: fromNullMillis(row.ExpiresAt),
		}
	}
	return out, nil
}

// ListUnliftedActionStatuses implements moderation.Repository.
func (r *ModerationRepository) ListUnliftedActionStatuses(ctx context.Context, tx uow.Tx, kind moderation.ActionKind) ([]appmoderation.ActionStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListUnliftedModerationActionStatuses(ctx, string(kind))
	if err != nil {
		return nil, wrap(err, "list unlifted moderation action statuses")
	}
	out := make([]appmoderation.ActionStatus, len(rows))
	for i, row := range rows {
		out[i] = appmoderation.ActionStatus{ActionID: row.ActionID, StatusID: row.StatusID}
	}
	return out, nil
}

// ListAllAccountRestrictionRows implements moderation.Repository.
func (r *ModerationRepository) ListAllAccountRestrictionRows(ctx context.Context, tx uow.Tx) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllAccountRestrictionRows(ctx)
	if err != nil {
		return nil, wrap(err, "list account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// DeleteAllAccountRestrictions implements moderation.Repository.
func (r *ModerationRepository) DeleteAllAccountRestrictions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty account restrictions", (*sqlitegen.Queries).DeleteAllAccountRestrictions)
}

// DeleteAllDomainRestrictions implements moderation.Repository.
func (r *ModerationRepository) DeleteAllDomainRestrictions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty domain restrictions", (*sqlitegen.Queries).DeleteAllDomainRestrictions)
}

// RebuildReportStates implements moderation.Repository.
func (r *ModerationRepository) RebuildReportStates(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild report states", (*sqlitegen.Queries).DeleteAllReportStates, (*sqlitegen.Queries).RebuildReportStates)
}

// RebuildAppealStates implements moderation.Repository.
func (r *ModerationRepository) RebuildAppealStates(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild appeal states", (*sqlitegen.Queries).DeleteAllAppealStates, (*sqlitegen.Queries).RebuildAppealStates)
}

// RebuildUserRoles implements identity.Repository.
func (r *IdentityRepository) RebuildUserRoles(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild user roles", (*sqlitegen.Queries).DeleteAllUserRoles, (*sqlitegen.Queries).RebuildUserRoles)
}

// RebuildRegistrationStates implements identity.Repository.
func (r *IdentityRepository) RebuildRegistrationStates(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild registration states",
		(*sqlitegen.Queries).DeleteAllUserRegistrationStates, (*sqlitegen.Queries).RebuildUserRegistrationStates)
}

// RebuildPrimaryEmails implements identity.Repository.
func (r *IdentityRepository) RebuildPrimaryEmails(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild primary emails", (*sqlitegen.Queries).DeleteAllUserPrimaryEmails, (*sqlitegen.Queries).RebuildUserPrimaryEmails)
}

// RebuildHeads implements webhook.Repository.
func (r *WebhookRepository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild webhook endpoint heads",
		(*sqlitegen.Queries).DeleteAllWebhookEndpointHeads, (*sqlitegen.Queries).RebuildWebhookEndpointHeads)
}

// RebuildFailures implements webhook.Repository.
func (r *WebhookRepository) RebuildFailures(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild webhook endpoint failures",
		(*sqlitegen.Queries).DeleteDerivableWebhookEndpointFailures, (*sqlitegen.Queries).RebuildWebhookEndpointFailures)
}

// DeleteAllSensitivityOverrides implements content.Repository.
func (r *StatusRepository) DeleteAllSensitivityOverrides(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty status sensitivity overrides", (*sqlitegen.Queries).DeleteAllStatusSensitivityOverrides)
}

// RebuildPollTallies implements content.Repository.
func (r *StatusRepository) RebuildPollTallies(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild poll tallies", (*sqlitegen.Queries).DeleteAllPollTallies, (*sqlitegen.Queries).RebuildPollTallies)
}

// ListRestrictions implements timeline.Repository.
func (r *TimelineRepository) ListRestrictions(ctx context.Context, tx uow.Tx) ([]apptimeline.AuthorRestriction, []apptimeline.DomainRestriction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, nil, err
	}
	authorRows, err := q.ListAllFeedAuthorRestrictions(ctx)
	if err != nil {
		return nil, nil, wrap(err, "list feed author restrictions")
	}
	domainRows, err := q.ListAllFeedDomainRestrictions(ctx)
	if err != nil {
		return nil, nil, wrap(err, "list feed domain restrictions")
	}
	authors := make([]apptimeline.AuthorRestriction, len(authorRows))
	for i, row := range authorRows {
		authors[i] = apptimeline.AuthorRestriction{
			AccountID: row.AccountID, Restriction: timeline.Restriction(row.Restriction), EffectiveUntil: fromNullMillis(row.EffectiveUntil),
		}
	}
	domains := make([]apptimeline.DomainRestriction, len(domainRows))
	for i, row := range domainRows {
		domains[i] = apptimeline.DomainRestriction{
			Domain: row.Domain, Restriction: timeline.Restriction(row.Restriction), EffectiveUntil: fromNullMillis(row.EffectiveUntil),
		}
	}
	return authors, domains, nil
}

// DeleteRestrictions implements timeline.Repository.
func (r *TimelineRepository) DeleteRestrictions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty feed restrictions",
		(*sqlitegen.Queries).DeleteAllFeedAuthorRestrictions, (*sqlitegen.Queries).DeleteAllFeedDomainRestrictions)
}
