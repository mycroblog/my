// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/hookgen"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// HookRepository implements apphook.Repository (data-model §4.16, ADR-0147 §6).
type HookRepository struct {
	db *DB
}

var _ apphook.Repository = (*HookRepository)(nil)

// HookRepository returns the hook module's repository on this database.
func (d *DB) HookRepository() *HookRepository { return &HookRepository{db: d} }

// InsertDecision implements apphook.Repository.
func (r *HookRepository) InsertDecision(ctx context.Context, tx uow.Tx, d hook.Decision) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecision(ctx, hookgen.InsertHookDecisionParams{
		ID: d.ID, Hook: string(d.Hook), SubjectKey: d.SubjectKey, AccountID: toNullString(d.AccountID), Mode: string(d.Mode),
		Outcome: string(d.Outcome), ReasonCode: string(d.Reason), Note: d.Note, LatencyMs: d.Latency.Milliseconds(), DecidedAt: d.DecidedAt,
	})
	return n > 0, wrap(err, "insert hook decision")
}

func hookRecord(id, name, subjectKey string, accountID *string, mode, outcome, reason, note string, latencyMs int64, decidedAt time.Time,
	overriddenID, overriddenBy *string, overriddenAt *time.Time,
) hook.Record {
	rec := hook.Record{Decision: hook.Decision{
		ID: id, Hook: hook.Name(name), SubjectKey: subjectKey, AccountID: fromNullString(accountID), Mode: hook.Mode(mode),
		Outcome: hook.Outcome(outcome), Reason: hook.Reason(reason), Note: note, Latency: time.Duration(latencyMs) * time.Millisecond, DecidedAt: decidedAt,
	}}
	if overriddenID != nil {
		rec.Override = &hook.Override{DecisionID: id, ByUserID: fromNullString(overriddenBy)}
		if overriddenAt != nil {
			rec.Override.At = *overriddenAt
		}
	}
	return rec
}

// RecordBySubject implements apphook.Repository.
func (r *HookRepository) RecordBySubject(ctx context.Context, n hook.Name, subjectKey string) (hook.Record, error) {
	row, err := r.db.reader().GetHookDecisionBySubject(ctx, hookgen.GetHookDecisionBySubjectParams{Hook: string(n), SubjectKey: subjectKey})
	if err != nil {
		return hook.Record{}, hookNotFound(err, subjectKey)
	}
	return hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
		row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt), nil
}

// RecordTx implements apphook.Repository.
func (r *HookRepository) RecordTx(ctx context.Context, tx uow.Tx, id string) (hook.Record, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return hook.Record{}, err
	}
	row, err := q.GetHookDecision(ctx, id)
	if err != nil {
		return hook.Record{}, hookNotFound(err, id)
	}
	return hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
		row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt), nil
}

func hookNotFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apphook.ErrNotFound
	}
	return wrap(err, "read hook decision "+what)
}

// InsertDecisionStatus implements apphook.Repository.
func (r *HookRepository) InsertDecisionStatus(ctx context.Context, tx uow.Tx, decisionID, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertHookDecisionStatus(ctx, hookgen.InsertHookDecisionStatusParams{DecisionID: decisionID, StatusID: statusID}), "insert hook decision status")
}

// InsertOverride implements apphook.Repository.
func (r *HookRepository) InsertOverride(ctx context.Context, tx uow.Tx, o hook.Override) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecisionOverride(ctx, hookgen.InsertHookDecisionOverrideParams{
		DecisionID: o.DecisionID, OverriddenByUserID: toNullString(o.ByUserID), OverriddenAt: o.At,
	})
	return n > 0, wrap(err, "insert hook decision override")
}

// ListRecords implements apphook.Repository. The decisions other than allow and the allow ones
// are two queries, so that the first can use the partial index that leaves allow out.
func (r *HookRepository) ListRecords(ctx context.Context, q apphook.ListQuery) ([]hook.Record, error) {
	if q.Outcome == hook.Allow {
		rows, err := r.db.reader().ListAllowHookDecisions(ctx, hookgen.ListAllowHookDecisionsParams{
			Before: toNullString(q.Before), Hook: string(q.Hook), Mode: string(q.Mode), RowLimit: int32(q.Limit), //nolint:gosec // a page's size
		})
		if err != nil {
			return nil, wrap(err, "list hook decisions")
		}
		out := make([]hook.Record, len(rows))
		for i, row := range rows {
			out[i] = hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
				row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt)
		}
		return out, nil
	}
	rows, err := r.db.reader().ListHookDecisions(ctx, hookgen.ListHookDecisionsParams{
		Before: toNullString(q.Before), Hook: string(q.Hook), Mode: string(q.Mode), Outcome: string(q.Outcome), RowLimit: int32(q.Limit), //nolint:gosec // a page's size
	})
	if err != nil {
		return nil, wrap(err, "list hook decisions")
	}
	out := make([]hook.Record, len(rows))
	for i, row := range rows {
		out[i] = hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
			row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt)
	}
	return out, nil
}

// Summarize implements apphook.Repository.
func (r *HookRepository) Summarize(ctx context.Context, since time.Time) ([]apphook.SummaryRow, error) {
	rows, err := r.db.reader().SummarizeHookDecisions(ctx, since)
	if err != nil {
		return nil, wrap(err, "summarize hook decisions")
	}
	out := make([]apphook.SummaryRow, len(rows))
	for i, row := range rows {
		out[i] = apphook.SummaryRow{
			Hook: hook.Name(row.Hook), Mode: hook.Mode(row.Mode), Outcome: hook.Outcome(row.Outcome), Decisions: row.Decisions,
			TotalLatency: time.Duration(row.TotalLatencyMs) * time.Millisecond, MaxLatency: time.Duration(row.MaxLatencyMs) * time.Millisecond,
		}
	}
	return out, nil
}

// ListExpired implements apphook.Repository.
func (r *HookRepository) ListExpired(ctx context.Context, q apphook.ExpiredQuery) ([]hook.Expired, error) {
	rows, err := r.db.reader().ListExpiredHookDecisions(ctx, hookgen.ListExpiredHookDecisionsParams{
		Before: q.Before, AfterID: toNullString(q.AfterID), AfterAt: q.AfterAt, RowLimit: int32(q.Limit), //nolint:gosec // a batch's size
	})
	if err != nil {
		return nil, wrap(err, "list expired hook decisions")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]hook.Expired, len(rows))
	ids := make([]string, len(rows))
	at := make(map[string]int, len(rows))
	for i, row := range rows {
		out[i] = hook.Expired{
			ID: row.ID, Hook: hook.Name(row.Hook), SubjectKey: row.SubjectKey, AccountID: fromNullString(row.AccountID), Mode: hook.Mode(row.Mode),
			Outcome: hook.Outcome(row.Outcome), DecidedAt: row.DecidedAt, Overridden: row.Overridden, AppealPending: row.AppealPending,
		}
		ids[i], at[row.ID] = row.ID, i
	}
	statuses, err := r.db.reader().ListStatusesOfHookDecisions(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list statuses of hook decisions")
	}
	for _, s := range statuses {
		e := &out[at[s.DecisionID]]
		e.StatusIDs = append(e.StatusIDs, s.StatusID)
	}
	return out, nil
}

// ListSensitivityMarks implements apphook.Repository.
func (r *HookRepository) ListSensitivityMarks(ctx context.Context, tx uow.Tx) ([]apphook.SensitivityMark, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListHookSensitivityMarks(ctx)
	if err != nil {
		return nil, wrap(err, "list hook sensitivity marks")
	}
	out := make([]apphook.SensitivityMark, len(rows))
	for i, row := range rows {
		out[i] = apphook.SensitivityMark{StatusID: row.StatusID, DecisionID: row.DecisionID}
	}
	return out, nil
}

// DecisionIDsOfAccount implements apphook.Repository.
func (r *HookRepository) DecisionIDsOfAccount(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListHookDecisionIDsOfAccount(ctx, hookgen.ListHookDecisionIDsOfAccountParams{AccountID: &accountID, Limit: int32(limit)}) //nolint:gosec // a batch's size
	return ids, wrap(err, "list hook decisions of account")
}

// DeleteDecisions implements apphook.Repository.
func (r *HookRepository) DeleteDecisions(ctx context.Context, tx uow.Tx, ids []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteHookDecisionStatuses(ctx, ids); err != nil {
		return wrap(err, "delete hook decision statuses")
	}
	if err := q.DeleteHookDecisionOverrides(ctx, ids); err != nil {
		return wrap(err, "delete hook decision overrides")
	}
	if err := q.DeleteHookDecisionAppealDecisions(ctx, ids); err != nil {
		return wrap(err, "delete hook decision appeal decisions")
	}
	if err := q.DeleteHookDecisionAppeals(ctx, ids); err != nil {
		return wrap(err, "delete hook decision appeals")
	}
	return wrap(q.DeleteHookDecisions(ctx, ids), "delete hook decisions")
}

// InsertAppeal implements apphook.Repository.
func (r *HookRepository) InsertAppeal(ctx context.Context, tx uow.Tx, a hook.Appeal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecisionAppeal(ctx, hookgen.InsertHookDecisionAppealParams{
		ID: a.ID, DecisionID: a.DecisionID, AccountID: a.AccountID, StatusText: a.StatusText, Comment: a.Comment, SubmittedAt: a.SubmittedAt,
	})
	return n > 0, wrap(err, "insert hook decision appeal")
}

func hookAppealRecord(row hookgen.GetHookDecisionAppealRow) hook.AppealRecord {
	out := hook.AppealRecord{
		Appeal: hook.Appeal{
			ID: row.AppealID, DecisionID: row.AppealDecisionID, AccountID: row.AppealAccountID,
			StatusText: row.StatusText, Comment: row.Comment, SubmittedAt: row.SubmittedAt,
		},
		Decision: hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
			row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt),
	}
	if row.Review != nil {
		out.Review = &hook.AppealDecision{AppealID: row.AppealID, Outcome: hook.AppealOutcome(*row.Review), ByUserID: fromNullString(row.ReviewedByUserID)}
		if row.ReviewedAt != nil {
			out.Review.DecidedAt = *row.ReviewedAt
		}
	}
	return out
}

func hookAppealNotFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apphook.ErrAppealNotFound
	}
	return wrap(err, "read hook decision appeal "+what)
}

// AppealTx implements apphook.Repository.
func (r *HookRepository) AppealTx(ctx context.Context, tx uow.Tx, id string) (hook.AppealRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return hook.AppealRecord{}, err
	}
	row, err := q.GetHookDecisionAppeal(ctx, id)
	if err != nil {
		return hook.AppealRecord{}, hookAppealNotFound(err, id)
	}
	return hookAppealRecord(row), nil
}

// Appeal implements apphook.Repository.
func (r *HookRepository) Appeal(ctx context.Context, id string) (hook.AppealRecord, error) {
	row, err := r.db.reader().GetHookDecisionAppeal(ctx, id)
	if err != nil {
		return hook.AppealRecord{}, hookAppealNotFound(err, id)
	}
	return hookAppealRecord(row), nil
}

// AppealOfDecision implements apphook.Repository.
func (r *HookRepository) AppealOfDecision(ctx context.Context, decisionID string) (hook.AppealRecord, error) {
	row, err := r.db.reader().GetHookDecisionAppealOfDecision(ctx, decisionID)
	if err != nil {
		return hook.AppealRecord{}, hookAppealNotFound(err, "of "+decisionID)
	}
	return hookAppealRecord(hookgen.GetHookDecisionAppealRow(row)), nil
}

// ListAppeals implements apphook.Repository.
func (r *HookRepository) ListAppeals(ctx context.Context, q apphook.AppealQuery) ([]hook.AppealRecord, error) {
	rows, err := r.db.reader().ListHookDecisionAppeals(ctx, hookgen.ListHookDecisionAppealsParams{
		Before: toNullString(q.Before), Pending: q.Pending, Outcome: string(q.Outcome), RowLimit: int32(q.Limit), //nolint:gosec // a page's size
	})
	if err != nil {
		return nil, wrap(err, "list hook decision appeals")
	}
	out := make([]hook.AppealRecord, len(rows))
	for i, row := range rows {
		out[i] = hookAppealRecord(hookgen.GetHookDecisionAppealRow(row))
	}
	return out, nil
}

// InsertAppealDecision implements apphook.Repository.
func (r *HookRepository) InsertAppealDecision(ctx context.Context, tx uow.Tx, d hook.AppealDecision) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecisionAppealDecision(ctx, hookgen.InsertHookDecisionAppealDecisionParams{
		AppealID: d.AppealID, Decision: string(d.Outcome), DecidedByUserID: toNullString(d.ByUserID), DecidedAt: d.DecidedAt,
	})
	return n > 0, wrap(err, "insert hook decision appeal decision")
}

// InsertExemption implements apphook.Repository.
func (r *HookRepository) InsertExemption(ctx context.Context, tx uow.Tx, e hook.Exemption) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertHookAccountExemption(ctx, hookgen.InsertHookAccountExemptionParams{
		Hook: string(e.Hook), AccountID: e.AccountID, ExemptedAt: e.ExemptedAt,
	}), "insert hook account exemption")
}

// Exempt implements apphook.Repository.
func (r *HookRepository) Exempt(ctx context.Context, n hook.Name, accountID string) (bool, error) {
	exempt, err := r.db.reader().HookAccountExempt(ctx, hookgen.HookAccountExemptParams{Hook: string(n), AccountID: accountID})
	return exempt, wrap(err, "read hook account exemption")
}

// DeleteAppealsOfAccount implements apphook.Repository.
func (r *HookRepository) DeleteAppealsOfAccount(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteHookDecisionAppealDecisionsOfAccount(ctx, accountID); err != nil {
		return wrap(err, "delete hook decision appeal decisions of account")
	}
	if err := q.DeleteHookDecisionAppealsOfAccount(ctx, accountID); err != nil {
		return wrap(err, "delete hook decision appeals of account")
	}
	return wrap(q.DeleteHookAccountExemptionsOfAccount(ctx, accountID), "delete hook account exemptions of account")
}

// ExistingStatusIDs implements apphook.Statuses on the content module's table.
func (r *StatusRepository) ExistingStatusIDs(ctx context.Context, ids []string) ([]string, error) {
	out, err := r.db.reader().ListExistingStatusIDs(ctx, ids)
	return out, wrap(err, "list existing statuses")
}
