// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ListVerifiedEmailUsers implements identity.Repository.
func (r *IdentityRepository) ListVerifiedEmailUsers(ctx context.Context, after string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListVerifiedPrimaryEmailUsers(ctx, sqlitegen.ListVerifiedPrimaryEmailUsersParams{After: after, RowLimit: int64(limit)})
	return ids, wrap(err, "list users with a verified primary email")
}

// InsertServiceClosureEmail implements identity.Repository.
func (r *IdentityRepository) InsertServiceClosureEmail(ctx context.Context, tx uow.Tx, e identity.ServiceClosureEmailRecord) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertServiceClosureEmail(ctx, sqlitegen.InsertServiceClosureEmailParams{
		ID: e.ID, ClosureEventID: e.ClosureEventID, Email: string(e.Email), UserID: e.UserID, QueuedAt: toMillis(e.QueuedAt),
	})
	return n > 0, wrap(err, "insert service closure email")
}

// ServiceClosureEmailQueued implements identity.Repository.
func (r *IdentityRepository) ServiceClosureEmailQueued(ctx context.Context, closureEventID string, e identity.ClosureEmail) (bool, error) {
	ok, err := r.db.reader().ServiceClosureEmailQueued(ctx, sqlitegen.ServiceClosureEmailQueuedParams{ClosureEventID: closureEventID, Email: string(e)})
	return ok, wrap(err, "find service closure emails")
}
