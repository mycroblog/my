// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.DeletedPurgeRepository = (*StatusRepository)(nil)

// ListDeletedStatusesToPurge implements content.DeletedPurgeRepository.
func (r *StatusRepository) ListDeletedStatusesToPurge(ctx context.Context, tx uow.Tx, pq appcontent.DeletedPurgeQuery) ([]content.Deletion, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDeletedStatusesToPurge(ctx, contentgen.ListDeletedStatusesToPurgeParams{
		After: pq.After, DeletedBefore: sqlite.ToMillis(pq.Before), BatchLimit: int64(pq.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list deleted statuses to purge")
	}
	out := make([]content.Deletion, 0, len(rows))
	for _, row := range rows {
		out = append(out, deletionFrom(row))
	}
	return out, nil
}

// PurgeDeletedStatuses implements content.DeletedPurgeRepository.
func (r *StatusRepository) PurgeDeletedStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	if err := q.EraseLocalStatusesOfStatuses(ctx, ids); err != nil {
		return sqlite.Wrap(err, "purge deleted statuses")
	}
	return r.PurgeStatuses(ctx, tx, statusIDs)
}
