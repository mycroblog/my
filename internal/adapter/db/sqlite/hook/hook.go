// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/hookgen"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// Repository implements apphook.Repository (data-model §4.16, ADR-0147 §6).
type Repository struct {
	db *moduleDB
}

var _ apphook.Repository = (*Repository)(nil)

// NewRepository returns the hook module's repository on this database.
func NewRepository(d *sqlite.DB) *Repository { return &Repository{db: &moduleDB{d}} }

// InsertDecision implements apphook.Repository.
func (r *Repository) InsertDecision(ctx context.Context, tx uow.Tx, d hook.Decision) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecision(ctx, hookgen.InsertHookDecisionParams{
		ID: d.ID, Hook: string(d.Hook), SubjectKey: d.SubjectKey, AccountID: sqlite.ToNullString(d.AccountID), Mode: string(d.Mode),
		Outcome: string(d.Outcome), ReasonCode: string(d.Reason), Note: d.Note, LatencyMs: d.Latency.Milliseconds(), DecidedAt: sqlite.ToMillis(d.DecidedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert hook decision")
}

func hookRecord(id, name, subjectKey string, accountID sql.NullString, mode, outcome, reason, note string, latencyMs, decidedAt int64,
	overriddenID, overriddenBy sql.NullString, overriddenAt sql.NullInt64,
) hook.Record {
	rec := hook.Record{Decision: hook.Decision{
		ID: id, Hook: hook.Name(name), SubjectKey: subjectKey, AccountID: accountID.String, Mode: hook.Mode(mode),
		Outcome: hook.Outcome(outcome), Reason: hook.Reason(reason), Note: note, Latency: time.Duration(latencyMs) * time.Millisecond, DecidedAt: sqlite.FromMillis(decidedAt),
	}}
	if overriddenID.Valid {
		rec.Override = &hook.Override{DecisionID: id, ByUserID: overriddenBy.String, At: sqlite.FromMillis(overriddenAt.Int64)}
	}
	return rec
}

// RecordBySubject implements apphook.Repository.
func (r *Repository) RecordBySubject(ctx context.Context, n hook.Name, subjectKey string) (hook.Record, error) {
	row, err := r.db.reader().GetHookDecisionBySubject(ctx, hookgen.GetHookDecisionBySubjectParams{Hook: string(n), SubjectKey: subjectKey})
	if err != nil {
		return hook.Record{}, hookNotFound(err, subjectKey)
	}
	return hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
		row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt), nil
}

// RecordTx implements apphook.Repository.
func (r *Repository) RecordTx(ctx context.Context, tx uow.Tx, id string) (hook.Record, error) {
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
	if errors.Is(err, sql.ErrNoRows) {
		return apphook.ErrNotFound
	}
	return sqlite.Wrap(err, "read hook decision "+what)
}

// InsertDecisionStatus implements apphook.Repository.
func (r *Repository) InsertDecisionStatus(ctx context.Context, tx uow.Tx, decisionID, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertHookDecisionStatus(ctx, hookgen.InsertHookDecisionStatusParams{DecisionID: decisionID, StatusID: statusID}), "insert hook decision status")
}

// InsertOverride implements apphook.Repository.
func (r *Repository) InsertOverride(ctx context.Context, tx uow.Tx, o hook.Override) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecisionOverride(ctx, hookgen.InsertHookDecisionOverrideParams{
		DecisionID: o.DecisionID, OverriddenByUserID: sqlite.ToNullString(o.ByUserID), OverriddenAt: sqlite.ToMillis(o.At),
	})
	return n > 0, sqlite.Wrap(err, "insert hook decision override")
}

// ListRecords implements apphook.Repository. The decisions other than allow and the allow ones
// are two queries, so that the first can use the partial index that leaves allow out.
func (r *Repository) ListRecords(ctx context.Context, q apphook.ListQuery) ([]hook.Record, error) {
	if q.Outcome == hook.Allow {
		rows, err := r.db.reader().ListAllowHookDecisions(ctx, hookgen.ListAllowHookDecisionsParams{
			Before: q.Before, Hook: string(q.Hook), Mode: string(q.Mode), RowLimit: int64(q.Limit),
		})
		if err != nil {
			return nil, sqlite.Wrap(err, "list hook decisions")
		}
		out := make([]hook.Record, len(rows))
		for i, row := range rows {
			out[i] = hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
				row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt)
		}
		return out, nil
	}
	rows, err := r.db.reader().ListHookDecisions(ctx, hookgen.ListHookDecisionsParams{
		Before: q.Before, Hook: string(q.Hook), Mode: string(q.Mode), Outcome: string(q.Outcome), RowLimit: int64(q.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list hook decisions")
	}
	out := make([]hook.Record, len(rows))
	for i, row := range rows {
		out[i] = hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
			row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt)
	}
	return out, nil
}

// Summarize implements apphook.Repository.
func (r *Repository) Summarize(ctx context.Context, since time.Time) ([]apphook.SummaryRow, error) {
	rows, err := r.db.reader().SummarizeHookDecisions(ctx, sqlite.ToMillis(since))
	if err != nil {
		return nil, sqlite.Wrap(err, "summarize hook decisions")
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
func (r *Repository) ListExpired(ctx context.Context, q apphook.ExpiredQuery) ([]hook.Expired, error) {
	rows, err := r.db.reader().ListExpiredHookDecisions(ctx, hookgen.ListExpiredHookDecisionsParams{
		Before: sqlite.ToMillis(q.Before), AfterID: q.AfterID, AfterAt: sqlite.ToMillis(q.AfterAt), RowLimit: int64(q.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list expired hook decisions")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]hook.Expired, len(rows))
	ids := make([]string, len(rows))
	at := make(map[string]int, len(rows))
	for i, row := range rows {
		out[i] = hook.Expired{
			ID: row.ID, Hook: hook.Name(row.Hook), SubjectKey: row.SubjectKey, AccountID: row.AccountID.String, Mode: hook.Mode(row.Mode),
			Outcome: hook.Outcome(row.Outcome), DecidedAt: sqlite.FromMillis(row.DecidedAt), Overridden: row.Overridden != 0, AppealPending: row.AppealPending != 0,
		}
		ids[i], at[row.ID] = row.ID, i
	}
	statuses, err := r.db.reader().ListStatusesOfHookDecisions(ctx, ids)
	if err != nil {
		return nil, sqlite.Wrap(err, "list statuses of hook decisions")
	}
	for _, s := range statuses {
		e := &out[at[s.DecisionID]]
		e.StatusIDs = append(e.StatusIDs, s.StatusID)
	}
	return out, nil
}

// ListSensitivityMarks implements apphook.Repository.
func (r *Repository) ListSensitivityMarks(ctx context.Context, tx uow.Tx) ([]apphook.SensitivityMark, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListHookSensitivityMarks(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list hook sensitivity marks")
	}
	out := make([]apphook.SensitivityMark, len(rows))
	for i, row := range rows {
		out[i] = apphook.SensitivityMark{StatusID: row.StatusID, DecisionID: row.DecisionID}
	}
	return out, nil
}

// DecisionIDsOfAccount implements apphook.Repository.
func (r *Repository) DecisionIDsOfAccount(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListHookDecisionIDsOfAccount(ctx, hookgen.ListHookDecisionIDsOfAccountParams{AccountID: sqlite.ToNullString(accountID), Limit: int64(limit)})
	return ids, sqlite.Wrap(err, "list hook decisions of account")
}

// DeleteDecisions implements apphook.Repository.
func (r *Repository) DeleteDecisions(ctx context.Context, tx uow.Tx, ids []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteHookDecisionStatuses(ctx, ids); err != nil {
		return sqlite.Wrap(err, "delete hook decision statuses")
	}
	if err := q.DeleteHookDecisionOverrides(ctx, ids); err != nil {
		return sqlite.Wrap(err, "delete hook decision overrides")
	}
	if err := q.DeleteHookDecisionAppealDecisions(ctx, ids); err != nil {
		return sqlite.Wrap(err, "delete hook decision appeal decisions")
	}
	if err := q.DeleteHookDecisionAppeals(ctx, ids); err != nil {
		return sqlite.Wrap(err, "delete hook decision appeals")
	}
	return sqlite.Wrap(q.DeleteHookDecisions(ctx, ids), "delete hook decisions")
}

// InsertAppeal implements apphook.Repository.
func (r *Repository) InsertAppeal(ctx context.Context, tx uow.Tx, a hook.Appeal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecisionAppeal(ctx, hookgen.InsertHookDecisionAppealParams{
		ID: a.ID, DecisionID: a.DecisionID, AccountID: a.AccountID, StatusText: a.StatusText, Comment: a.Comment, SubmittedAt: sqlite.ToMillis(a.SubmittedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert hook decision appeal")
}

func hookAppealRecord(row hookgen.GetHookDecisionAppealRow) hook.AppealRecord {
	out := hook.AppealRecord{
		Appeal: hook.Appeal{
			ID: row.AppealID, DecisionID: row.AppealDecisionID, AccountID: row.AppealAccountID,
			StatusText: row.StatusText, Comment: row.Comment, SubmittedAt: sqlite.FromMillis(row.SubmittedAt),
		},
		Decision: hookRecord(row.ID, row.Hook, row.SubjectKey, row.AccountID, row.Mode, row.Outcome, row.ReasonCode, row.Note, row.LatencyMs, row.DecidedAt,
			row.OverriddenDecisionID, row.OverriddenByUserID, row.OverriddenAt),
	}
	if row.Review.Valid {
		out.Review = &hook.AppealDecision{
			AppealID: row.AppealID, Outcome: hook.AppealOutcome(row.Review.String), ByUserID: row.ReviewedByUserID.String, DecidedAt: sqlite.FromMillis(row.ReviewedAt.Int64),
		}
	}
	return out
}

func hookAppealNotFound(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apphook.ErrAppealNotFound
	}
	return sqlite.Wrap(err, "read hook decision appeal "+what)
}

// AppealTx implements apphook.Repository.
func (r *Repository) AppealTx(ctx context.Context, tx uow.Tx, id string) (hook.AppealRecord, error) {
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
func (r *Repository) Appeal(ctx context.Context, id string) (hook.AppealRecord, error) {
	row, err := r.db.reader().GetHookDecisionAppeal(ctx, id)
	if err != nil {
		return hook.AppealRecord{}, hookAppealNotFound(err, id)
	}
	return hookAppealRecord(row), nil
}

// AppealOfDecision implements apphook.Repository.
func (r *Repository) AppealOfDecision(ctx context.Context, decisionID string) (hook.AppealRecord, error) {
	row, err := r.db.reader().GetHookDecisionAppealOfDecision(ctx, decisionID)
	if err != nil {
		return hook.AppealRecord{}, hookAppealNotFound(err, "of "+decisionID)
	}
	return hookAppealRecord(hookgen.GetHookDecisionAppealRow(row)), nil
}

// ListAppeals implements apphook.Repository.
func (r *Repository) ListAppeals(ctx context.Context, q apphook.AppealQuery) ([]hook.AppealRecord, error) {
	var pending int64
	if q.Pending {
		pending = 1
	}
	rows, err := r.db.reader().ListHookDecisionAppeals(ctx, hookgen.ListHookDecisionAppealsParams{
		Before: q.Before, Pending: pending, Outcome: string(q.Outcome), RowLimit: int64(q.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list hook decision appeals")
	}
	out := make([]hook.AppealRecord, len(rows))
	for i, row := range rows {
		out[i] = hookAppealRecord(hookgen.GetHookDecisionAppealRow(row))
	}
	return out, nil
}

// InsertAppealDecision implements apphook.Repository.
func (r *Repository) InsertAppealDecision(ctx context.Context, tx uow.Tx, d hook.AppealDecision) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHookDecisionAppealDecision(ctx, hookgen.InsertHookDecisionAppealDecisionParams{
		AppealID: d.AppealID, Decision: string(d.Outcome), DecidedByUserID: sqlite.ToNullString(d.ByUserID), DecidedAt: sqlite.ToMillis(d.DecidedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert hook decision appeal decision")
}

// InsertExemption implements apphook.Repository.
func (r *Repository) InsertExemption(ctx context.Context, tx uow.Tx, e hook.Exemption) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertHookAccountExemption(ctx, hookgen.InsertHookAccountExemptionParams{
		Hook: string(e.Hook), AccountID: e.AccountID, ExemptedAt: sqlite.ToMillis(e.ExemptedAt),
	}), "insert hook account exemption")
}

// Exempt implements apphook.Repository.
func (r *Repository) Exempt(ctx context.Context, n hook.Name, accountID string) (bool, error) {
	exempt, err := r.db.reader().HookAccountExempt(ctx, hookgen.HookAccountExemptParams{Hook: string(n), AccountID: accountID})
	return exempt != 0, sqlite.Wrap(err, "read hook account exemption")
}

// DeleteAppealsOfAccount implements apphook.Repository.
func (r *Repository) DeleteAppealsOfAccount(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteHookDecisionAppealDecisionsOfAccount(ctx, accountID); err != nil {
		return sqlite.Wrap(err, "delete hook decision appeal decisions of account")
	}
	if err := q.DeleteHookDecisionAppealsOfAccount(ctx, accountID); err != nil {
		return sqlite.Wrap(err, "delete hook decision appeals of account")
	}
	return sqlite.Wrap(q.DeleteHookAccountExemptionsOfAccount(ctx, accountID), "delete hook account exemptions of account")
}
