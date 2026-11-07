// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertRule implements instance.RuleRepository.
func (r *Repository) InsertRule(ctx context.Context, tx uow.Tx, id string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertInstanceRule(ctx, instancegen.InsertInstanceRuleParams{ID: id, CreatedAt: sqlite.ToMillis(createdAt)}), "insert instance rule")
}

// InsertRuleRevision implements instance.RuleRepository.
func (r *Repository) InsertRuleRevision(ctx context.Context, tx uow.Tx, rev instance.RuleRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceRuleRevision(ctx, instancegen.InsertInstanceRuleRevisionParams{
		ID: rev.ID, RuleID: rev.RuleID, Position: int64(rev.Position), Text: rev.Text, Hint: rev.Hint,
		RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID), RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	return sqlite.Wrap(err, "insert instance rule revision")
}

// InsertRuleRetirement implements instance.RuleRepository.
func (r *Repository) InsertRuleRetirement(ctx context.Context, tx uow.Tx, ret instance.RuleRetirement) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInstanceRuleRetirement(ctx, instancegen.InsertInstanceRuleRetirementParams{
		RuleID: ret.RuleID, RetiredByUserID: sqlite.ToNullString(ret.RetiredByUserID), RetiredAt: sqlite.ToMillis(ret.RetiredAt),
	})
	return n > 0, sqlite.Wrap(err, "insert instance rule retirement")
}

// Rule implements instance.RuleRepository.
func (r *Repository) Rule(ctx context.Context, tx uow.Tx, id string) (instance.Rule, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.Rule{}, err
	}
	return findRule(ctx, q, id)
}

// FindRule implements instance.RuleRepository.
func (r *Repository) FindRule(ctx context.Context, id string) (instance.Rule, error) {
	return findRule(ctx, r.db.reader(), id)
}

func findRule(ctx context.Context, q *instancegen.Queries, id string) (instance.Rule, error) {
	row, err := q.GetInstanceRule(ctx, id)
	if err != nil {
		return instance.Rule{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance rule "+id)
	}
	return ruleOf(instancegen.ListAllInstanceRulesRow(row)), nil
}

// ListRules implements instance.RuleRepository.
func (r *Repository) ListRules(ctx context.Context, includeRetired bool) ([]instance.Rule, error) {
	q := r.db.reader()
	var rules []instance.Rule
	if includeRetired {
		rows, err := q.ListAllInstanceRules(ctx)
		if err != nil {
			return nil, sqlite.Wrap(err, "list instance rules")
		}
		rules = make([]instance.Rule, len(rows))
		for i, row := range rows {
			rules[i] = ruleOf(row)
		}
	} else {
		rows, err := q.ListInstanceRulesInEffect(ctx)
		if err != nil {
			return nil, sqlite.Wrap(err, "list instance rules")
		}
		rules = make([]instance.Rule, len(rows))
		for i, row := range rows {
			rules[i] = ruleOf(instancegen.ListAllInstanceRulesRow{
				ID: row.ID, CreatedAt: row.CreatedAt, RevisionID: row.RevisionID, Position: row.Position, Text: row.Text, Hint: row.Hint,
				RevisedByUserID: row.RevisedByUserID, RevisedAt: row.RevisedAt,
			})
		}
	}
	return rules, nil
}

func ruleOf(row instancegen.ListAllInstanceRulesRow) instance.Rule {
	rule := instance.Rule{
		ID: row.ID, CreatedAt: sqlite.FromMillis(row.CreatedAt),
		Revision: instance.RuleRevision{
			ID: row.RevisionID, RuleID: row.ID, Position: int(row.Position), Text: row.Text, Hint: row.Hint,
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
		},
	}
	if row.RetiredAt.Valid {
		rule.Retirement = &instance.RuleRetirement{
			RuleID: row.ID, RetiredByUserID: row.RetiredByUserID.String, RetiredAt: sqlite.FromMillis(row.RetiredAt.Int64),
		}
	}
	return rule
}

// RuleHead implements instance.RuleRepository.
func (r *Repository) RuleHead(ctx context.Context, tx uow.Tx, ruleID string) (instance.RuleHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.RuleHead{}, err
	}
	row, err := q.GetInstanceRuleHead(ctx, ruleID)
	if err != nil {
		return instance.RuleHead{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance rule head "+ruleID)
	}
	return ruleHead(row), nil
}

// UpsertRuleHead implements instance.RuleRepository.
func (r *Repository) UpsertRuleHead(ctx context.Context, tx uow.Tx, h instance.RuleHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceRuleHead(ctx, instancegen.UpsertInstanceRuleHeadParams{RuleID: h.RuleID, RevisionID: h.RevisionID, Position: int64(h.Position)})
	return sqlite.Wrap(err, "upsert instance rule head")
}

// DeleteRuleHead implements instance.RuleRepository.
func (r *Repository) DeleteRuleHead(ctx context.Context, tx uow.Tx, ruleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteInstanceRuleHead(ctx, ruleID), "delete instance rule head")
}

// DeleteAllRuleHeads implements instance.RuleRepository.
func (r *Repository) DeleteAllRuleHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllInstanceRuleHeads(ctx), "delete instance rule heads")
}

// ListRuleHeads implements instance.RuleRepository.
func (r *Repository) ListRuleHeads(ctx context.Context, tx uow.Tx) ([]instance.RuleHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceRuleHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance rule heads")
	}
	out := make([]instance.RuleHead, len(rows))
	for i, row := range rows {
		out[i] = ruleHead(row)
	}
	return out, nil
}

// ListRuleRevisionHeads implements instance.RuleRepository.
func (r *Repository) ListRuleRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.RuleHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceRuleRevisionHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance rule revisions")
	}
	out := make([]instance.RuleHead, len(rows))
	for i, row := range rows {
		out[i] = instance.RuleHead{RuleID: row.RuleID, RevisionID: row.ID, Position: int(row.Position)}
	}
	return out, nil
}

// ListRetiredRuleIDs implements instance.RuleRepository.
func (r *Repository) ListRetiredRuleIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListRetiredInstanceRuleIDs(ctx)
	return ids, sqlite.Wrap(err, "list retired instance rules")
}

func ruleHead(row instancegen.InstanceRuleHead) instance.RuleHead {
	return instance.RuleHead{RuleID: row.RuleID, RevisionID: row.RevisionID, Position: int(row.Position)}
}
