// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AccountRestrictionsInEffectOf implements moderation.Repository.
func (r *Repository) AccountRestrictionsInEffectOf(ctx context.Context, accountIDs []string, now time.Time) (map[string][]moderation.Restriction, error) {
	out := map[string][]moderation.Restriction{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListAccountRestrictionsInEffectOf(ctx, moderationgen.ListAccountRestrictionsInEffectOfParams{AccountIds: accountIDs, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list account restrictions in effect")
	}
	for _, row := range rows {
		out[row.AccountID] = append(out[row.AccountID], moderation.Restriction(row.Restriction))
	}
	return out, nil
}

// DomainRestrictionsInEffectOf implements moderation.Repository.
func (r *Repository) DomainRestrictionsInEffectOf(ctx context.Context, domains []string, now time.Time) (map[string][]moderation.Restriction, error) {
	out := map[string][]moderation.Restriction{}
	if len(domains) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListDomainRestrictionsInEffectOf(ctx, moderationgen.ListDomainRestrictionsInEffectOfParams{Domains: domains, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain restrictions in effect")
	}
	for _, row := range rows {
		out[row.Domain] = append(out[row.Domain], moderation.Restriction(row.Restriction))
	}
	return out, nil
}
