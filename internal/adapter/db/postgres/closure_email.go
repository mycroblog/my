// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ListVerifiedEmailUsers implements identity.Repository.
func (r *IdentityRepository) ListVerifiedEmailUsers(ctx context.Context, after string, limit int) ([]string, error) {
	rowLimit, err := toInt32(limit, "row limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListVerifiedPrimaryEmailUsers(ctx, postgresgen.ListVerifiedPrimaryEmailUsersParams{After: toNullString(after), RowLimit: rowLimit})
	return ids, wrap(err, "list users with a verified primary email")
}

// InsertServiceClosureEmail implements identity.Repository.
func (r *IdentityRepository) InsertServiceClosureEmail(ctx context.Context, tx uow.Tx, e identity.ServiceClosureEmailRecord) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertServiceClosureEmail(ctx, postgresgen.InsertServiceClosureEmailParams{
		ID: e.ID, ClosureEventID: e.ClosureEventID, Email: string(e.Email), UserID: e.UserID, QueuedAt: e.QueuedAt,
	})
	return n > 0, wrap(err, "insert service closure email")
}

// ServiceClosureEmailQueued implements identity.Repository.
func (r *IdentityRepository) ServiceClosureEmailQueued(ctx context.Context, closureEventID string, e identity.ClosureEmail) (bool, error) {
	ok, err := r.db.reader().ServiceClosureEmailQueued(ctx, postgresgen.ServiceClosureEmailQueuedParams{ClosureEventID: closureEventID, Email: string(e)})
	return ok, wrap(err, "find service closure emails")
}
