// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertRule implements instance.RuleRepository.
func (r *InstanceRepository) InsertRule(ctx context.Context, tx uow.Tx, id string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertInstanceRule(ctx, sqlitegen.InsertInstanceRuleParams{ID: id, CreatedAt: toMillis(createdAt)}), "insert instance rule")
}

// InsertRuleRevision implements instance.RuleRepository.
func (r *InstanceRepository) InsertRuleRevision(ctx context.Context, tx uow.Tx, rev instance.RuleRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceRuleRevision(ctx, sqlitegen.InsertInstanceRuleRevisionParams{
		ID: rev.ID, RuleID: rev.RuleID, Position: int64(rev.Position), Text: rev.Text, Hint: rev.Hint,
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	})
	return wrap(err, "insert instance rule revision")
}

// InsertRuleRetirement implements instance.RuleRepository.
func (r *InstanceRepository) InsertRuleRetirement(ctx context.Context, tx uow.Tx, ret instance.RuleRetirement) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInstanceRuleRetirement(ctx, sqlitegen.InsertInstanceRuleRetirementParams{
		RuleID: ret.RuleID, RetiredByUserID: toNullString(ret.RetiredByUserID), RetiredAt: toMillis(ret.RetiredAt),
	})
	return n > 0, wrap(err, "insert instance rule retirement")
}

// Rule implements instance.RuleRepository.
func (r *InstanceRepository) Rule(ctx context.Context, tx uow.Tx, id string) (instance.Rule, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.Rule{}, err
	}
	return findRule(ctx, q, id)
}

// FindRule implements instance.RuleRepository.
func (r *InstanceRepository) FindRule(ctx context.Context, id string) (instance.Rule, error) {
	return findRule(ctx, r.db.reader(), id)
}

func findRule(ctx context.Context, q *sqlitegen.Queries, id string) (instance.Rule, error) {
	row, err := q.GetInstanceRule(ctx, id)
	if err != nil {
		return instance.Rule{}, notFound(err, appinstance.ErrNotFound, "instance rule "+id)
	}
	return ruleOf(sqlitegen.ListAllInstanceRulesRow(row)), nil
}

// ListRules implements instance.RuleRepository.
func (r *InstanceRepository) ListRules(ctx context.Context, includeRetired bool) ([]instance.Rule, error) {
	q := r.db.reader()
	var rules []instance.Rule
	if includeRetired {
		rows, err := q.ListAllInstanceRules(ctx)
		if err != nil {
			return nil, wrap(err, "list instance rules")
		}
		rules = make([]instance.Rule, len(rows))
		for i, row := range rows {
			rules[i] = ruleOf(row)
		}
	} else {
		rows, err := q.ListInstanceRulesInEffect(ctx)
		if err != nil {
			return nil, wrap(err, "list instance rules")
		}
		rules = make([]instance.Rule, len(rows))
		for i, row := range rows {
			rules[i] = ruleOf(sqlitegen.ListAllInstanceRulesRow{
				ID: row.ID, CreatedAt: row.CreatedAt, RevisionID: row.RevisionID, Position: row.Position, Text: row.Text, Hint: row.Hint,
				RevisedByUserID: row.RevisedByUserID, RevisedAt: row.RevisedAt,
			})
		}
	}
	return rules, nil
}

func ruleOf(row sqlitegen.ListAllInstanceRulesRow) instance.Rule {
	rule := instance.Rule{
		ID: row.ID, CreatedAt: fromMillis(row.CreatedAt),
		Revision: instance.RuleRevision{
			ID: row.RevisionID, RuleID: row.ID, Position: int(row.Position), Text: row.Text, Hint: row.Hint,
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: fromMillis(row.RevisedAt),
		},
	}
	if row.RetiredAt.Valid {
		rule.Retirement = &instance.RuleRetirement{
			RuleID: row.ID, RetiredByUserID: row.RetiredByUserID.String, RetiredAt: fromMillis(row.RetiredAt.Int64),
		}
	}
	return rule
}

// RuleHead implements instance.RuleRepository.
func (r *InstanceRepository) RuleHead(ctx context.Context, tx uow.Tx, ruleID string) (instance.RuleHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.RuleHead{}, err
	}
	row, err := q.GetInstanceRuleHead(ctx, ruleID)
	if err != nil {
		return instance.RuleHead{}, notFound(err, appinstance.ErrNotFound, "instance rule head "+ruleID)
	}
	return ruleHead(row), nil
}

// UpsertRuleHead implements instance.RuleRepository.
func (r *InstanceRepository) UpsertRuleHead(ctx context.Context, tx uow.Tx, h instance.RuleHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceRuleHead(ctx, sqlitegen.UpsertInstanceRuleHeadParams{RuleID: h.RuleID, RevisionID: h.RevisionID, Position: int64(h.Position)})
	return wrap(err, "upsert instance rule head")
}

// DeleteRuleHead implements instance.RuleRepository.
func (r *InstanceRepository) DeleteRuleHead(ctx context.Context, tx uow.Tx, ruleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteInstanceRuleHead(ctx, ruleID), "delete instance rule head")
}

// DeleteAllRuleHeads implements instance.RuleRepository.
func (r *InstanceRepository) DeleteAllRuleHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllInstanceRuleHeads(ctx), "delete instance rule heads")
}

// ListRuleHeads implements instance.RuleRepository.
func (r *InstanceRepository) ListRuleHeads(ctx context.Context, tx uow.Tx) ([]instance.RuleHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceRuleHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list instance rule heads")
	}
	out := make([]instance.RuleHead, len(rows))
	for i, row := range rows {
		out[i] = ruleHead(row)
	}
	return out, nil
}

// ListRuleRevisionHeads implements instance.RuleRepository.
func (r *InstanceRepository) ListRuleRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.RuleHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceRuleRevisionHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list instance rule revisions")
	}
	out := make([]instance.RuleHead, len(rows))
	for i, row := range rows {
		out[i] = instance.RuleHead{RuleID: row.RuleID, RevisionID: row.ID, Position: int(row.Position)}
	}
	return out, nil
}

// ListRetiredRuleIDs implements instance.RuleRepository.
func (r *InstanceRepository) ListRetiredRuleIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListRetiredInstanceRuleIDs(ctx)
	return ids, wrap(err, "list retired instance rules")
}

func ruleHead(row sqlitegen.InstanceRuleHead) instance.RuleHead {
	return instance.RuleHead{RuleID: row.RuleID, RevisionID: row.RevisionID, Position: int(row.Position)}
}
