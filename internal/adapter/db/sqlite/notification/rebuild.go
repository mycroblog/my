// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
)

// This file holds the sources and the emptying of the projections that `my projection
// rebuild` derives again (data-model §9) and that no other file of the module covers.

// DeleteAllExclusions implements notification.Repository.
func (r *Repository) DeleteAllExclusions(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllNotificationExclusions(ctx); err != nil {
		return sqlite.Wrap(err, "empty notification exclusions")
	}
	return sqlite.Wrap(q.DeleteAllNotificationDomainExclusions(ctx), "empty notification domain exclusions")
}

// DeleteAllDeletedStatuses implements notification.Repository.
func (r *Repository) DeleteAllDeletedStatuses(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllNotificationDeletedStatuses(ctx), "empty notification deleted statuses")
}

// ListNotifiedStatusIDs implements notification.Repository.
func (r *Repository) ListNotifiedStatusIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListNotifiedStatusIDs(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list notified statuses")
	}
	out := make([]string, len(rows))
	for i, id := range rows {
		out[i] = id.String
	}
	return out, nil
}
