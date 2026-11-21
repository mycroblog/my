// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The posts of the boards of the events (ADR-0161 §17): status_gathering_boards.

// InsertStatusBoard implements content.BoardRepository.
func (r *StatusRepository) InsertStatusBoard(ctx context.Context, tx uow.Tx, statusID, gatheringID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatusGatheringBoard(ctx, contentgen.InsertStatusGatheringBoardParams{StatusID: statusID, GatheringID: gatheringID})
	return sqlite.Wrap(err, "insert status gathering board")
}

// StatusBoardsAmong implements content.BoardRepository.
func (r *StatusRepository) StatusBoardsAmong(ctx context.Context, tx uow.Tx, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	q, err := r.readIn(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListStatusGatheringBoards(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status gathering boards")
	}
	for _, row := range rows {
		out[row.StatusID] = row.GatheringID
	}
	return out, nil
}

// ListLiveBoardStatusIDs implements content.BoardRepository.
func (r *StatusRepository) ListLiveBoardStatusIDs(ctx context.Context, gatheringID, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListLiveBoardStatusIDs(ctx, contentgen.ListLiveBoardStatusIDsParams{
		GatheringID: gatheringID, Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list board statuses of "+gatheringID)
	}
	return ids, nil
}

// ListLiveBoardStatusIDsOfAuthor implements content.BoardRepository.
func (r *StatusRepository) ListLiveBoardStatusIDsOfAuthor(ctx context.Context, authorAccountID, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListLiveBoardStatusIDsOfAuthor(ctx, contentgen.ListLiveBoardStatusIDsOfAuthorParams{
		AuthorAccountID: authorAccountID, Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list board statuses by "+authorAccountID)
	}
	return ids, nil
}

// ListLiveBoardStatusesOf implements content.BoardRepository.
func (r *StatusRepository) ListLiveBoardStatusesOf(ctx context.Context, tx uow.Tx, gatheringIDs []string, limit int) ([]content.Status, error) {
	if len(gatheringIDs) == 0 {
		return nil, nil
	}
	q, err := r.readIn(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(gatheringIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLiveBoardStatusesOfGatherings(ctx, contentgen.ListLiveBoardStatusesOfGatheringsParams{GatheringIds: list, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list board statuses of events")
	}
	out := make([]content.Status, 0, len(rows))
	for _, row := range rows {
		out = append(out, statusFrom(row))
	}
	return out, nil
}

// ListBoardStatusIDs implements content.BoardRepository.
func (r *StatusRepository) ListBoardStatusIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.readIn(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListBoardStatusIDs(ctx)
	return ids, sqlite.Wrap(err, "list board statuses")
}

// ListBoardGatheringIDs implements content.BoardRepository.
func (r *StatusRepository) ListBoardGatheringIDs(ctx context.Context, after string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListBoardGatheringIDs(ctx, contentgen.ListBoardGatheringIDsParams{After: after, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list the events of boards")
	}
	return ids, nil
}
