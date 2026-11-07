// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// ListRemoteHeads implements account.Repository.
func (r *Repository) ListRemoteHeads(ctx context.Context, tx uow.Tx) ([]account.RemoteHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRemoteAccountHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list remote account heads")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = remoteHeadFrom(row)
	}
	return out, nil
}

// ListLatestRemoteSnapshotHeads implements account.Repository.
func (r *Repository) ListLatestRemoteSnapshotHeads(ctx context.Context, tx uow.Tx) ([]account.RemoteHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLatestRemoteAccountSnapshots(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list latest remote account snapshots")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = account.RemoteHead{
			AccountID: row.AccountID, SnapshotID: row.ID, InboxURL: row.InboxUrl,
			SharedInboxURL: row.SharedInboxUrl.String, LastFetchedAt: sqlite.FromMillis(row.FetchedAt),
		}
	}
	return out, nil
}

// DeleteAllRemoteHeads implements account.Repository.
func (r *Repository) DeleteAllRemoteHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllRemoteAccountHeads(ctx), "empty remote account heads")
}
