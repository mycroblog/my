// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// UpsertRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) UpsertRemoteMediaCache(ctx context.Context, tx uow.Tx, e content.RemoteMediaCacheEntry) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertRemoteMediaCache(ctx, contentgen.UpsertRemoteMediaCacheParams{
		Url: e.URL, BlobKey: e.BlobKey, ContentType: e.ContentType, FileSize: e.FileSize,
		FetchedAt: sqlite.ToMillis(e.FetchedAt), ExpiresAt: sqlite.ToMillis(e.ExpiresAt), LastAccessedAt: sqlite.ToMillis(e.LastAccessedAt),
	}), "upsert remote media cache")
}

// TouchRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) TouchRemoteMediaCache(ctx context.Context, tx uow.Tx, url string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.TouchRemoteMediaCache(ctx, contentgen.TouchRemoteMediaCacheParams{Url: url, LastAccessedAt: sqlite.ToMillis(at)}), "touch remote media cache")
}

// DeleteRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) DeleteRemoteMediaCache(ctx context.Context, tx uow.Tx, url string, fetchedAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteRemoteMediaCache(ctx, contentgen.DeleteRemoteMediaCacheParams{Url: url, FetchedAt: sqlite.ToMillis(fetchedAt)})
	return n > 0, sqlite.Wrap(err, "delete remote media cache")
}

// FindRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) FindRemoteMediaCache(ctx context.Context, url string) (content.RemoteMediaCacheEntry, error) {
	row, err := r.db.reader().GetRemoteMediaCache(ctx, url)
	if err != nil {
		return content.RemoteMediaCacheEntry{}, sqlite.NotFound(err, appcontent.ErrNotFound, "remote media cache "+url)
	}
	return remoteMediaCacheEntry(row), nil
}

// ListExpiredRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) ListExpiredRemoteMediaCache(ctx context.Context, now time.Time, limit int) ([]content.RemoteMediaCacheEntry, error) {
	rows, err := r.db.reader().ListExpiredRemoteMediaCache(ctx, contentgen.ListExpiredRemoteMediaCacheParams{Now: sqlite.ToMillis(now), BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list expired remote media cache")
	}
	return remoteMediaCacheEntries(rows), nil
}

// ListLeastRecentlyAccessedRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) ListLeastRecentlyAccessedRemoteMediaCache(ctx context.Context, limit int) ([]content.RemoteMediaCacheEntry, error) {
	rows, err := r.db.reader().ListLeastRecentlyAccessedRemoteMediaCache(ctx, int64(limit))
	if err != nil {
		return nil, sqlite.Wrap(err, "list least recently accessed remote media cache")
	}
	return remoteMediaCacheEntries(rows), nil
}

// RemoteMediaCacheSize implements content.MediaRepository.
func (r *MediaRepository) RemoteMediaCacheSize(ctx context.Context) (int64, error) {
	n, err := r.db.reader().SumRemoteMediaCacheSize(ctx)
	return n, sqlite.Wrap(err, "sum remote media cache size")
}

// LocalMediaSize implements content.MediaRepository.
func (r *MediaRepository) LocalMediaSize(ctx context.Context) (int64, error) {
	n, err := r.db.reader().SumLocalMediaSize(ctx)
	return n, sqlite.Wrap(err, "sum local media size")
}

// IsCustomEmojiImage implements content.MediaRepository.
func (r *MediaRepository) IsCustomEmojiImage(ctx context.Context, mediaID string) (bool, error) {
	ok, err := r.db.reader().IsCustomEmojiImage(ctx, mediaID)
	return ok, sqlite.Wrap(err, "is custom emoji image")
}

func remoteMediaCacheEntries(rows []contentgen.RemoteMediaCache) []content.RemoteMediaCacheEntry {
	out := make([]content.RemoteMediaCacheEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, remoteMediaCacheEntry(row))
	}
	return out
}

func remoteMediaCacheEntry(row contentgen.RemoteMediaCache) content.RemoteMediaCacheEntry {
	return content.RemoteMediaCacheEntry{
		URL: row.Url, BlobKey: row.BlobKey, ContentType: row.ContentType, FileSize: row.FileSize,
		FetchedAt: sqlite.FromMillis(row.FetchedAt), ExpiresAt: sqlite.FromMillis(row.ExpiresAt), LastAccessedAt: sqlite.FromMillis(row.LastAccessedAt),
	}
}

// ListCurrentCustomEmojis implements content.MediaRepository.
func (r *MediaRepository) ListCurrentCustomEmojis(ctx context.Context, ids []string) ([]content.CurrentCustomEmoji, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListCurrentCustomEmojiImages(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list current custom emoji")
	}
	out := make([]content.CurrentCustomEmoji, 0, len(rows))
	for _, row := range rows {
		out = append(out, content.CurrentCustomEmoji{
			Emoji: content.CustomEmoji{
				ID: row.ID, Shortcode: row.Shortcode, Domain: row.Domain, URI: row.Uri.String, CreatedAt: sqlite.FromMillis(row.CreatedAt),
			},
			Image: content.CustomEmojiImage{ID: row.ImageID, EmojiID: row.ID, MediaID: row.ImageMediaID, RevisedAt: sqlite.FromMillis(row.ImageRevisedAt)},
			Settings: content.CustomEmojiSettings{
				EmojiID: row.ID, Enabled: row.Enabled != 0, VisibleInPicker: row.VisibleInPicker != 0, Category: row.Category,
			},
		})
	}
	return out, nil
}

// InsertRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) InsertRemoteMediaCache(ctx context.Context, tx uow.Tx, e content.RemoteMediaCacheEntry) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteMediaCache(ctx, contentgen.InsertRemoteMediaCacheParams{
		Url: e.URL, BlobKey: e.BlobKey, ContentType: e.ContentType, FileSize: e.FileSize,
		FetchedAt: sqlite.ToMillis(e.FetchedAt), ExpiresAt: sqlite.ToMillis(e.ExpiresAt), LastAccessedAt: sqlite.ToMillis(e.LastAccessedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert remote media cache")
}

// LockRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) LockRemoteMediaCache(ctx context.Context, tx uow.Tx, url string) (content.RemoteMediaCacheEntry, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.RemoteMediaCacheEntry{}, err
	}
	row, err := q.LockRemoteMediaCache(ctx, url)
	if err != nil {
		return content.RemoteMediaCacheEntry{}, sqlite.NotFound(err, appcontent.ErrNotFound, "remote media cache "+url)
	}
	return remoteMediaCacheEntry(row), nil
}

// RecordBlobDeletion implements content.MediaRepository.
func (r *MediaRepository) RecordBlobDeletion(ctx context.Context, tx uow.Tx, key string, deleteAfter time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertMediaBlobDeletion(ctx, contentgen.UpsertMediaBlobDeletionParams{BlobKey: key, DeleteAfter: sqlite.ToMillis(deleteAfter)}), "record blob deletion")
}

// ForgetBlobDeletion implements content.MediaRepository.
func (r *MediaRepository) ForgetBlobDeletion(ctx context.Context, tx uow.Tx, key string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteMediaBlobDeletion(ctx, key)
	return n > 0, sqlite.Wrap(err, "forget blob deletion")
}

// ListDueBlobDeletions implements content.MediaRepository.
func (r *MediaRepository) ListDueBlobDeletions(ctx context.Context, now time.Time, limit int) ([]string, error) {
	keys, err := r.db.reader().ListDueMediaBlobDeletions(ctx, contentgen.ListDueMediaBlobDeletionsParams{Now: sqlite.ToMillis(now), BatchLimit: int64(limit)})
	return keys, sqlite.Wrap(err, "list due blob deletions")
}
