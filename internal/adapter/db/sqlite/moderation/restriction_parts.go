// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AccountRestrictionsInEffectOf implements moderation.Repository.
func (r *Repository) AccountRestrictionsInEffectOf(ctx context.Context, accountIDs []string, now time.Time) (map[string][]moderation.Restriction, error) {
	out := map[string][]moderation.Restriction{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountRestrictionsInEffectOf(ctx, moderationgen.ListAccountRestrictionsInEffectOfParams{AccountIds: list, Now: sqlite.ToMillis(now)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list account restrictions in effect")
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
	list, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainRestrictionsInEffectOf(ctx, moderationgen.ListDomainRestrictionsInEffectOfParams{Domains: list, Now: sqlite.ToMillis(now)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain restrictions in effect")
	}
	for _, row := range rows {
		out[row.Domain] = append(out[row.Domain], moderation.Restriction(row.Restriction))
	}
	return out, nil
}
