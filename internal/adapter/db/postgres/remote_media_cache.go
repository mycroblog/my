// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	return wrap(q.UpsertRemoteMediaCache(ctx, postgresgen.UpsertRemoteMediaCacheParams{
		Url: e.URL, BlobKey: e.BlobKey, ContentType: e.ContentType, FileSize: e.FileSize,
		FetchedAt: e.FetchedAt, ExpiresAt: e.ExpiresAt, LastAccessedAt: e.LastAccessedAt,
	}), "upsert remote media cache")
}

// TouchRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) TouchRemoteMediaCache(ctx context.Context, tx uow.Tx, url string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.TouchRemoteMediaCache(ctx, postgresgen.TouchRemoteMediaCacheParams{Url: url, LastAccessedAt: at}), "touch remote media cache")
}

// DeleteRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) DeleteRemoteMediaCache(ctx context.Context, tx uow.Tx, url string, fetchedAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteRemoteMediaCache(ctx, postgresgen.DeleteRemoteMediaCacheParams{Url: url, FetchedAt: fetchedAt})
	return n > 0, wrap(err, "delete remote media cache")
}

// FindRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) FindRemoteMediaCache(ctx context.Context, url string) (content.RemoteMediaCacheEntry, error) {
	row, err := r.db.reader().GetRemoteMediaCache(ctx, url)
	if err != nil {
		return content.RemoteMediaCacheEntry{}, notFound(err, appcontent.ErrNotFound, "remote media cache "+url)
	}
	return remoteMediaCacheEntry(row), nil
}

// ListExpiredRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) ListExpiredRemoteMediaCache(ctx context.Context, now time.Time, limit int) ([]content.RemoteMediaCacheEntry, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListExpiredRemoteMediaCache(ctx, postgresgen.ListExpiredRemoteMediaCacheParams{Now: now, BatchLimit: n})
	if err != nil {
		return nil, wrap(err, "list expired remote media cache")
	}
	return remoteMediaCacheEntries(rows), nil
}

// ListLeastRecentlyAccessedRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) ListLeastRecentlyAccessedRemoteMediaCache(ctx context.Context, limit int) ([]content.RemoteMediaCacheEntry, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLeastRecentlyAccessedRemoteMediaCache(ctx, n)
	if err != nil {
		return nil, wrap(err, "list least recently accessed remote media cache")
	}
	return remoteMediaCacheEntries(rows), nil
}

// RemoteMediaCacheSize implements content.MediaRepository.
func (r *MediaRepository) RemoteMediaCacheSize(ctx context.Context) (int64, error) {
	n, err := r.db.reader().SumRemoteMediaCacheSize(ctx)
	return n, wrap(err, "sum remote media cache size")
}

// LocalMediaSize implements content.MediaRepository.
func (r *MediaRepository) LocalMediaSize(ctx context.Context) (int64, error) {
	n, err := r.db.reader().SumLocalMediaSize(ctx)
	return n, wrap(err, "sum local media size")
}

// IsCustomEmojiImage implements content.MediaRepository.
func (r *MediaRepository) IsCustomEmojiImage(ctx context.Context, mediaID string) (bool, error) {
	ok, err := r.db.reader().IsCustomEmojiImage(ctx, mediaID)
	return ok, wrap(err, "is custom emoji image")
}

func remoteMediaCacheEntries(rows []postgresgen.RemoteMediaCache) []content.RemoteMediaCacheEntry {
	out := make([]content.RemoteMediaCacheEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, remoteMediaCacheEntry(row))
	}
	return out
}

func remoteMediaCacheEntry(row postgresgen.RemoteMediaCache) content.RemoteMediaCacheEntry {
	return content.RemoteMediaCacheEntry{
		URL: row.Url, BlobKey: row.BlobKey, ContentType: row.ContentType, FileSize: row.FileSize,
		FetchedAt: utc(row.FetchedAt), ExpiresAt: utc(row.ExpiresAt), LastAccessedAt: utc(row.LastAccessedAt),
	}
}

// ListCurrentCustomEmojis implements content.MediaRepository.
func (r *MediaRepository) ListCurrentCustomEmojis(ctx context.Context, ids []string) ([]content.CurrentCustomEmoji, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.reader().ListCurrentCustomEmojiImages(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list current custom emoji")
	}
	out := make([]content.CurrentCustomEmoji, 0, len(rows))
	for _, row := range rows {
		out = append(out, content.CurrentCustomEmoji{
			Emoji: content.CustomEmoji{
				ID: row.ID, Shortcode: row.Shortcode, Domain: row.Domain, URI: fromNullString(row.Uri), CreatedAt: utc(row.CreatedAt),
			},
			Image: content.CustomEmojiImage{ID: row.ImageID, EmojiID: row.ID, MediaID: row.ImageMediaID, RevisedAt: utc(row.ImageRevisedAt)},
			Settings: content.CustomEmojiSettings{
				EmojiID: row.ID, Enabled: row.Enabled, VisibleInPicker: row.VisibleInPicker, Category: row.Category,
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
	n, err := q.InsertRemoteMediaCache(ctx, postgresgen.InsertRemoteMediaCacheParams{
		Url: e.URL, BlobKey: e.BlobKey, ContentType: e.ContentType, FileSize: e.FileSize,
		FetchedAt: e.FetchedAt, ExpiresAt: e.ExpiresAt, LastAccessedAt: e.LastAccessedAt,
	})
	return n > 0, wrap(err, "insert remote media cache")
}

// LockRemoteMediaCache implements content.MediaRepository.
func (r *MediaRepository) LockRemoteMediaCache(ctx context.Context, tx uow.Tx, url string) (content.RemoteMediaCacheEntry, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.RemoteMediaCacheEntry{}, err
	}
	row, err := q.LockRemoteMediaCache(ctx, url)
	if err != nil {
		return content.RemoteMediaCacheEntry{}, notFound(err, appcontent.ErrNotFound, "remote media cache "+url)
	}
	return remoteMediaCacheEntry(row), nil
}

// RecordBlobDeletion implements content.MediaRepository.
func (r *MediaRepository) RecordBlobDeletion(ctx context.Context, tx uow.Tx, key string, deleteAfter time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertMediaBlobDeletion(ctx, postgresgen.UpsertMediaBlobDeletionParams{BlobKey: key, DeleteAfter: deleteAfter}), "record blob deletion")
}

// ForgetBlobDeletion implements content.MediaRepository.
func (r *MediaRepository) ForgetBlobDeletion(ctx context.Context, tx uow.Tx, key string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteMediaBlobDeletion(ctx, key)
	return n > 0, wrap(err, "forget blob deletion")
}

// ListDueBlobDeletions implements content.MediaRepository.
func (r *MediaRepository) ListDueBlobDeletions(ctx context.Context, now time.Time, limit int) ([]string, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	keys, err := r.db.reader().ListDueMediaBlobDeletions(ctx, postgresgen.ListDueMediaBlobDeletionsParams{Now: now, BatchLimit: n})
	return keys, wrap(err, "list due blob deletions")
}
