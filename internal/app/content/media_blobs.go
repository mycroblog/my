// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// pendingBlobGrace is how long a blob that is being written may stay unrecorded before the
// cleanup deletes it as abandoned: far longer than an upload request or a processing job runs.
const pendingBlobGrace = 24 * time.Hour

// errBlobAbandoned reports a blob that the cleanup deleted as abandoned before the row that
// refers to it was recorded; the attempt that wrote it starts again under a new key.
var errBlobAbandoned = errors.New("content: a blob being written was deleted as abandoned")

// blobJournal keeps the work list of blob deletions (media_blob_deletions), so that a blob of
// the media files or of the remote media cache is never left with nothing that refers to it.
// The blob store cannot list its keys, so every key is in a row of the database from before its
// blob is written until after its blob is deleted:
//
//   - a writer reserves its keys, due after pendingBlobGrace, before it writes the blobs, and
//     keeps them in the transaction that records the rows that refer to them; a failed or losing
//     attempt discards them, and what it cannot delete the cleanup deletes once due;
//   - a deletion releases the keys of the rows it deletes in the same transaction, due at once,
//     and then deletes the blobs; what fails stays for the next cleanup.
//
// Deleting a missing blob succeeds, so every step is safe to repeat.
type blobJournal struct {
	uow    uow.UnitOfWork
	repo   MediaRepository
	blobs  blob.Store
	clock  kernel.Clock
	logger *slog.Logger
}

// reserve records keys, due after the grace period, before their blobs are written.
func (j blobJournal) reserve(ctx context.Context, keys ...string) error {
	due := j.clock.Now().Add(pendingBlobGrace)
	return j.uow.Tx(ctx, func(tx uow.Tx) error {
		for _, k := range keys {
			if err := j.repo.RecordBlobDeletion(ctx, tx, k, due); err != nil {
				return err
			}
		}
		return nil
	})
}

// keep removes the records of reserved keys in tx, the transaction that records the rows that
// refer to their blobs. It fails with errBlobAbandoned when a record is gone: the cleanup took
// the blob as abandoned.
func (j blobJournal) keep(ctx context.Context, tx uow.Tx, keys ...string) error {
	for _, k := range keys {
		ok, err := j.repo.ForgetBlobDeletion(ctx, tx, k)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s", errBlobAbandoned, k)
		}
	}
	return nil
}

// release records keys due at once in tx, the transaction that deletes the rows that refer to
// their blobs.
func (j blobJournal) release(ctx context.Context, tx uow.Tx, keys ...string) error {
	now := j.clock.Now()
	for _, k := range keys {
		if err := j.repo.RecordBlobDeletion(ctx, tx, k, now); err != nil {
			return err
		}
	}
	return nil
}

// discard deletes the blobs that an attempt wrote but did not record, even when ctx is done.
// What it cannot delete stays reserved, and the cleanup deletes it once due.
func (j blobJournal) discard(ctx context.Context, keys ...string) {
	j.deleteNow(context.WithoutCancel(ctx), keys...)
}

// deleteNow deletes the blobs of keys, then their records. A failure is logged and leaves the
// record for the cleanup.
func (j blobJournal) deleteNow(ctx context.Context, keys ...string) int {
	deleted := 0
	for _, k := range keys {
		if err := j.blobs.Delete(ctx, k); err != nil {
			j.logger.WarnContext(ctx, "media blob not deleted; the cleanup retries", slog.String("key", k), slog.Any("error", err))
			continue
		}
		err := j.uow.Tx(ctx, func(tx uow.Tx) error {
			_, err := j.repo.ForgetBlobDeletion(ctx, tx, k)
			return err
		})
		if err != nil {
			j.logger.WarnContext(ctx, "media blob deletion not recorded; the cleanup retries", slog.String("key", k), slog.Any("error", err))
			continue
		}
		deleted++
	}
	return deleted
}

// sweep deletes the blobs whose deletion is due, and returns how many it deleted. One run looks
// at a bounded number of keys; the next run continues.
func (j blobJournal) sweep(ctx context.Context) (int, error) {
	deleted := 0
	for range cleanupMaxBatches {
		keys, err := j.repo.ListDueBlobDeletions(ctx, j.clock.Now(), cleanupBatch)
		if err != nil {
			return deleted, err
		}
		n := j.deleteNow(ctx, keys...)
		deleted += n
		if len(keys) < cleanupBatch || n == 0 {
			break
		}
	}
	return deleted, nil
}
