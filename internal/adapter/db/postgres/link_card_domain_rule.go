// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// UpsertLinkCardDomainRule implements instance.LinkCardDomainRepository.
func (r *InstanceRepository) UpsertLinkCardDomainRule(ctx context.Context, tx uow.Tx, rule instance.LinkCardDomainRule) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertLinkCardDomainRule(ctx, postgresgen.UpsertLinkCardDomainRuleParams{
		Domain: rule.Domain, Mode: string(rule.Mode), Note: rule.Note, UpdatedAt: rule.UpdatedAt,
	}), "upsert link card domain rule")
}

// DeleteLinkCardDomainRule implements instance.LinkCardDomainRepository.
func (r *InstanceRepository) DeleteLinkCardDomainRule(ctx context.Context, tx uow.Tx, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteLinkCardDomainRule(ctx, domain)
	return n > 0, wrap(err, "delete link card domain rule")
}

// CountLinkCardDomainRules implements instance.LinkCardDomainRepository.
func (r *InstanceRepository) CountLinkCardDomainRules(ctx context.Context, tx uow.Tx) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountLinkCardDomainRules(ctx)
	return int(n), wrap(err, "count link card domain rules")
}

// HasLinkCardDomainRule implements instance.LinkCardDomainRepository.
func (r *InstanceRepository) HasLinkCardDomainRule(ctx context.Context, tx uow.Tx, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	found, err := q.HasLinkCardDomainRule(ctx, domain)
	return found, wrap(err, "find link card domain rule")
}

// ListLinkCardDomainRules implements instance.LinkCardDomainRepository.
func (r *InstanceRepository) ListLinkCardDomainRules(ctx context.Context) ([]instance.LinkCardDomainRule, error) {
	rows, err := r.db.reader().ListLinkCardDomainRules(ctx)
	if err != nil {
		return nil, wrap(err, "list link card domain rules")
	}
	out := make([]instance.LinkCardDomainRule, len(rows))
	for i, row := range rows {
		out[i] = instance.LinkCardDomainRule{
			Domain: row.Domain, Mode: instance.LinkCardDomainMode(row.Mode), Note: row.Note, UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

// LinkCardDomainModes implements instance.LinkCardDomainRepository.
func (r *InstanceRepository) LinkCardDomainModes(ctx context.Context, domains []string) ([]instance.LinkCardDomainMode, error) {
	rows, err := r.db.reader().ListLinkCardDomainModes(ctx, domains)
	if err != nil {
		return nil, wrap(err, "list link card domain modes")
	}
	out := make([]instance.LinkCardDomainMode, len(rows))
	for i, mode := range rows {
		out[i] = instance.LinkCardDomainMode(mode)
	}
	return out, nil
}
