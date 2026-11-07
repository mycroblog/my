// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ListVerifiedEmailUsers implements identity.Repository.
func (r *Repository) ListVerifiedEmailUsers(ctx context.Context, after string, limit int) ([]string, error) {
	rowLimit, err := postgres.ToInt32(limit, "row limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListVerifiedPrimaryEmailUsers(ctx, identitygen.ListVerifiedPrimaryEmailUsersParams{After: postgres.ToNullString(after), RowLimit: rowLimit})
	return ids, postgres.Wrap(err, "list users with a verified primary email")
}

// InsertServiceClosureEmail implements identity.Repository.
func (r *Repository) InsertServiceClosureEmail(ctx context.Context, tx uow.Tx, e identity.ServiceClosureEmailRecord) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertServiceClosureEmail(ctx, identitygen.InsertServiceClosureEmailParams{
		ID: e.ID, ClosureEventID: e.ClosureEventID, Email: string(e.Email), UserID: e.UserID, QueuedAt: e.QueuedAt,
	})
	return n > 0, postgres.Wrap(err, "insert service closure email")
}

// ServiceClosureEmailQueued implements identity.Repository.
func (r *Repository) ServiceClosureEmailQueued(ctx context.Context, closureEventID string, e identity.ClosureEmail) (bool, error) {
	ok, err := r.db.reader().ServiceClosureEmailQueued(ctx, identitygen.ServiceClosureEmailQueuedParams{ClosureEventID: closureEventID, Email: string(e)})
	return ok, postgres.Wrap(err, "find service closure emails")
}
