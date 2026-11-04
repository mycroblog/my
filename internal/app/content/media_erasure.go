// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/uow"
)

// MediaErasureDeps are the ports of the erasure of deleted accounts' media files.
type MediaErasureDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository MediaRepository
	Blobs      blob.Store
	// Referrers keep the files that the instance still uses, such as the instance settings'
	// images. The account's own profile is not one: its revisions go with the account.
	Referrers []MediaReferrer
}

// MediaEraser erases the uploads of deleted local accounts with their blobs (data-model §8). It
// runs after the StatusEraser, so that a status revision attaches a file only while the status
// is kept for the moderation records (ADR-0058); such a file stays until a later run, and so
// does the account's erasure. A file that a referrer references stays as the instance's.
//
// The blobs go before the rows, outside any transaction (ADR-0003): the rows are the record of
// what is left to delete, so a failure leaves rows that the next run deletes again rather than
// blobs that nothing refers to. Deleting a missing blob succeeds, so each step is safe to repeat.
// A deleted account attaches nothing new, so a file cannot become attached between the two.
type MediaEraser struct{ d MediaErasureDeps }

// NewMediaEraser returns the content module's eraser of media files on d.
func NewMediaEraser(d MediaErasureDeps) *MediaEraser { return &MediaEraser{d: d} }

// EraseAccountData implements account.DataEraser: one batch of the account's uploads after the
// cursor (a media file ID).
func (e *MediaEraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, cursor string, limit int) (appaccount.ErasureStep, error) {
	out := appaccount.ErasureStep{Next: cursor}
	uploads, err := e.d.Repository.ListOwnerUploads(ctx, t.AccountID, cursor, limit)
	if err != nil {
		return out, err
	}
	var ids []string
	for _, u := range uploads {
		if u.Attached {
			out.Kept = true
			out.Next = u.MediaID
			continue
		}
		referenced, err := e.referenced(ctx, u.MediaID)
		if err != nil {
			return out, err
		}
		if !referenced {
			keys, err := mediaBlobKeys(ctx, e.d.Repository, u.MediaID)
			if err != nil {
				return out, err
			}
			for _, k := range keys {
				if err := e.d.Blobs.Delete(ctx, k); err != nil {
					return out, err
				}
			}
			ids = append(ids, u.MediaID)
		}
		out.Next = u.MediaID
	}
	if len(ids) > 0 {
		err := e.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			for _, id := range ids {
				if _, err := e.d.Repository.DeleteUnattachedMedia(ctx, tx, id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return appaccount.ErasureStep{Next: cursor}, err
		}
	}
	out.Done = len(uploads) < limit
	return out, nil
}

func (e *MediaEraser) referenced(ctx context.Context, mediaID string) (bool, error) {
	for _, r := range e.d.Referrers {
		ok, err := r.IsMediaReferenced(ctx, mediaID)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// mediaBlobKeys returns the blob keys of an upload: the file as received, the poster of a video
// or audio file, and the variants of every processing result.
func mediaBlobKeys(ctx context.Context, r MediaRepository, mediaID string) ([]string, error) {
	var keys []string
	if up, err := r.FindUploadedMedia(ctx, mediaID); err == nil {
		keys = append(keys, up.BlobKey, posterKey(mediaID))
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	results, err := r.ListProcessingResults(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	for _, res := range results {
		for _, v := range res.Variants {
			keys = append(keys, v.BlobKey)
		}
	}
	return keys, nil
}
