// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.AccountAdminRepository = (*Repository)(nil)

// ListRestrictedAccountIDs implements moderation.AccountAdminRepository.
func (r *Repository) ListRestrictedAccountIDs(ctx context.Context, restriction moderation.Restriction, now time.Time, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListRestrictedAccountIDs(ctx, moderationgen.ListRestrictedAccountIDsParams{
		Restriction: string(restriction), Now: sqlite.ToMillis(now), Before: before, Lim: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list restricted accounts")
}
