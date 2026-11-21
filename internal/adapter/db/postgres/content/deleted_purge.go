// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.DeletedPurgeRepository = (*StatusRepository)(nil)

// insertContentPurge records, with the deletion d, the purge of its status's rows that
// appcontent.ContentPurgeDue says is due; a deletion without one records nothing.
func insertContentPurge(ctx context.Context, q *contentgen.Queries, d content.Deletion) error {
	due, ok := appcontent.ContentPurgeDue(d)
	if !ok {
		return nil
	}
	err := q.InsertStatusContentPurge(ctx, contentgen.InsertStatusContentPurgeParams{StatusID: d.StatusID, DueAt: due})
	return postgres.Wrap(err, "insert status content purge")
}

// insertContentPurges is insertContentPurge of deletions with one origin and time.
func insertContentPurges(ctx context.Context, q *contentgen.Queries, ds []content.Deletion) error {
	due, ok := appcontent.ContentPurgeDue(ds[0])
	if !ok {
		return nil
	}
	ids := make([]string, len(ds))
	for i, d := range ds {
		ids[i] = d.StatusID
	}
	err := q.InsertStatusContentPurges(ctx, contentgen.InsertStatusContentPurgesParams{StatusIds: ids, DueAt: due})
	return postgres.Wrap(err, "insert status content purges")
}

// ListDueContentPurges implements content.DeletedPurgeRepository.
func (r *StatusRepository) ListDueContentPurges(ctx context.Context, tx uow.Tx, dueBefore time.Time, limit int) ([]appcontent.DueContentPurge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDueStatusContentPurges(ctx, contentgen.ListDueStatusContentPurgesParams{DueBefore: dueBefore, BatchLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "list due status content purges")
	}
	out := make([]appcontent.DueContentPurge, 0, len(rows))
	for _, row := range rows {
		out = append(out, appcontent.DueContentPurge{StatusID: row.StatusID, AuthorAccountID: row.AuthorAccountID, Present: row.Present})
	}
	return out, nil
}

// PurgeDeletedStatuses implements content.DeletedPurgeRepository.
func (r *StatusRepository) PurgeDeletedStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.EraseLocalStatusesOfStatuses(ctx, statusIDs); err != nil {
		return postgres.Wrap(err, "purge deleted statuses")
	}
	return r.PurgeStatuses(ctx, tx, statusIDs)
}

// PostponeContentPurges implements content.DeletedPurgeRepository.
func (r *StatusRepository) PostponeContentPurges(ctx context.Context, tx uow.Tx, statusIDs []string, dueAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.PostponeStatusContentPurges(ctx, contentgen.PostponeStatusContentPurgesParams{DueAt: dueAt, Ids: statusIDs})
	return postgres.Wrap(err, "postpone status content purges")
}

// DeleteContentPurges implements content.DeletedPurgeRepository.
func (r *StatusRepository) DeleteContentPurges(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.PurgeStatusContentPurges(ctx, statusIDs), "delete status content purges")
}
