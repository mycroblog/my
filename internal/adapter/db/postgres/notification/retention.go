// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
)

// StatusHasNotifications implements notification.Repository.
func (r *Repository) StatusHasNotifications(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.StatusHasNotifications(ctx, &statusID)
	return ok, postgres.Wrap(err, "find notifications of a status")
}
