// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
)

// StatusHasNotifications implements notification.Repository.
func (r *Repository) StatusHasNotifications(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.StatusHasNotifications(ctx, sqlite.ToNullString(statusID))
	return ok, sqlite.Wrap(err, "find notifications of a status")
}
