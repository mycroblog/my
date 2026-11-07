// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// UpsertLinkCardDomainRule implements instance.LinkCardDomainRepository.
func (r *Repository) UpsertLinkCardDomainRule(ctx context.Context, tx uow.Tx, rule instance.LinkCardDomainRule) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertLinkCardDomainRule(ctx, instancegen.UpsertLinkCardDomainRuleParams{
		Domain: rule.Domain, Mode: string(rule.Mode), Note: rule.Note, UpdatedAt: sqlite.ToMillis(rule.UpdatedAt),
	}), "upsert link card domain rule")
}

// DeleteLinkCardDomainRule implements instance.LinkCardDomainRepository.
func (r *Repository) DeleteLinkCardDomainRule(ctx context.Context, tx uow.Tx, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteLinkCardDomainRule(ctx, domain)
	return n > 0, sqlite.Wrap(err, "delete link card domain rule")
}

// CountLinkCardDomainRules implements instance.LinkCardDomainRepository.
func (r *Repository) CountLinkCardDomainRules(ctx context.Context, tx uow.Tx) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountLinkCardDomainRules(ctx)
	return int(n), sqlite.Wrap(err, "count link card domain rules")
}

// HasLinkCardDomainRule implements instance.LinkCardDomainRepository.
func (r *Repository) HasLinkCardDomainRule(ctx context.Context, tx uow.Tx, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	found, err := q.HasLinkCardDomainRule(ctx, domain)
	return found, sqlite.Wrap(err, "find link card domain rule")
}

// ListLinkCardDomainRules implements instance.LinkCardDomainRepository.
func (r *Repository) ListLinkCardDomainRules(ctx context.Context) ([]instance.LinkCardDomainRule, error) {
	rows, err := r.db.reader().ListLinkCardDomainRules(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list link card domain rules")
	}
	out := make([]instance.LinkCardDomainRule, len(rows))
	for i, row := range rows {
		out[i] = instance.LinkCardDomainRule{
			Domain: row.Domain, Mode: instance.LinkCardDomainMode(row.Mode), Note: row.Note, UpdatedAt: sqlite.FromMillis(row.UpdatedAt),
		}
	}
	return out, nil
}

// LinkCardDomainModes implements instance.LinkCardDomainRepository.
func (r *Repository) LinkCardDomainModes(ctx context.Context, domains []string) ([]instance.LinkCardDomainMode, error) {
	list, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLinkCardDomainModes(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list link card domain modes")
	}
	out := make([]instance.LinkCardDomainMode, len(rows))
	for i, mode := range rows {
		out[i] = instance.LinkCardDomainMode(mode)
	}
	return out, nil
}
