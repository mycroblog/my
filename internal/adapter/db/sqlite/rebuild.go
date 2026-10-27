// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// This file holds the sources and the emptying of the projections that `my projection
// rebuild` derives again (data-model §9) and that no other file of the module covers.

// DeleteAllExclusions implements notification.Repository.
func (r *NotificationRepository) DeleteAllExclusions(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllNotificationExclusions(ctx); err != nil {
		return wrap(err, "empty notification exclusions")
	}
	return wrap(q.DeleteAllNotificationDomainExclusions(ctx), "empty notification domain exclusions")
}

// DeleteAllDeletedStatuses implements notification.Repository.
func (r *NotificationRepository) DeleteAllDeletedStatuses(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllNotificationDeletedStatuses(ctx), "empty notification deleted statuses")
}

// ListNotifiedStatusIDs implements notification.Repository.
func (r *NotificationRepository) ListNotifiedStatusIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListNotifiedStatusIDs(ctx)
	if err != nil {
		return nil, wrap(err, "list notified statuses")
	}
	out := make([]string, len(rows))
	for i, id := range rows {
		out[i] = id.String
	}
	return out, nil
}

// ListRemoteHeads implements account.Repository.
func (r *AccountRepository) ListRemoteHeads(ctx context.Context, tx uow.Tx) ([]account.RemoteHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRemoteAccountHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list remote account heads")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = remoteHeadFrom(row)
	}
	return out, nil
}

// ListLatestRemoteSnapshotHeads implements account.Repository.
func (r *AccountRepository) ListLatestRemoteSnapshotHeads(ctx context.Context, tx uow.Tx) ([]account.RemoteHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLatestRemoteAccountSnapshots(ctx)
	if err != nil {
		return nil, wrap(err, "list latest remote account snapshots")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = account.RemoteHead{
			AccountID: row.AccountID, SnapshotID: row.ID, InboxURL: row.InboxUrl,
			SharedInboxURL: row.SharedInboxUrl.String, LastFetchedAt: fromMillis(row.FetchedAt),
		}
	}
	return out, nil
}

// DeleteAllRemoteHeads implements account.Repository.
func (r *AccountRepository) DeleteAllRemoteHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllRemoteAccountHeads(ctx), "empty remote account heads")
}
