// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.AccountAdminRepository = (*Repository)(nil)

// ListRestrictedAccountIDs implements moderation.AccountAdminRepository.
func (r *Repository) ListRestrictedAccountIDs(ctx context.Context, restriction moderation.Restriction, now time.Time, before string, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListRestrictedAccountIDs(ctx, moderationgen.ListRestrictedAccountIDsParams{
		Restriction: string(restriction), Now: now, Before: postgres.ToNullString(before), Lim: lim,
	})
	return ids, postgres.Wrap(err, "list restricted accounts")
}
