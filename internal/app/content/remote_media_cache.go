// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// RemoteMediaPolicy is the remote_media_policy instance setting (FR-409, ADR-0005): which
// remote media files the media proxy keeps a copy of.
type RemoteMediaPolicy string

// Values of RemoteMediaPolicy.
const (
	// RemoteMediaCacheNone keeps no copy: the proxy relays every request.
	RemoteMediaCacheNone RemoteMediaPolicy = "none"
	// RemoteMediaCacheAvatars keeps the avatars, headers, and custom emoji images.
	RemoteMediaCacheAvatars RemoteMediaPolicy = "avatars"
	// RemoteMediaCacheAll keeps every remote media file.
	RemoteMediaCacheAll RemoteMediaPolicy = "all"
)

// ProxySettings are the instance settings of the media proxy.
type ProxySettings struct {
	Policy RemoteMediaPolicy
	// CacheTTL is remote_media_cache_ttl: how long a copy is kept, and the max-age of the
	// proxy's responses.
	CacheTTL time.Duration
}

const (
	// JobCleanupRemoteMedia deletes the copies of the remote media cache past their expiry and
	// those over the size limit (FR-803); no payload.
	JobCleanupRemoteMedia = "media.cleanup_remote"
	// DefaultRemoteMediaCacheMaxSize is the default size limit of the remote media cache.
	DefaultRemoteMediaCacheMaxSize int64 = 1 << 30
	// remoteMediaPrefix starts the private blob keys of the cached copies.
	remoteMediaPrefix = "remote/"
	// cacheTouchInterval is the granularity of last_accessed_at: a copy served again within it
	// is not written again, so that serving a copy rarely writes to the database.
	cacheTouchInterval = time.Hour
)

// RegisterJobs registers the cleanup of the remote media cache on reg and its hourly schedule on
// sched.
func (p *MediaProxy) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobCleanupRemoteMedia, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := p.CleanupCache(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobCleanupRemoteMedia, Every: DefaultCleanupInterval, Job: queue.NewJob{Type: JobCleanupRemoteMedia}})
}

// cacheable reports whether the policy keeps a copy of m: every file under all, and under
// avatars the files that a profile uses as its avatar or header or a custom emoji as its image.
func (p *MediaProxy) cacheable(ctx context.Context, m content.RemoteMedia, policy RemoteMediaPolicy) (bool, error) {
	if p.d.Blobs == nil || p.d.UnitOfWork == nil || p.d.Clock == nil {
		return false, nil
	}
	switch policy {
	case RemoteMediaCacheAll:
		return true, nil
	case RemoteMediaCacheAvatars:
		if p.d.ProfileImages != nil {
			ok, err := p.d.ProfileImages.IsMediaReferenced(ctx, m.MediaID)
			if err != nil || ok {
				return ok, err
			}
		}
		return p.d.Repository.IsCustomEmojiImage(ctx, m.MediaID)
	default:
		return false, nil
	}
}

// remoteMediaKey returns a new blob key for a copy of a remote URL (or of its small variant): the
// SHA-256 of the URL and a random part. Each copy has a key of its own, so that the cleanup of
// an expired copy never deletes the blob of the copy that a refresh wrote in its place.
func remoteMediaKey(remoteURL string) string {
	sum := sha256.Sum256([]byte(remoteURL))
	return remoteMediaPrefix + hex.EncodeToString(sum[:]) + "-" + strings.ToLower(rand.Text())
}

// openCached serves the copy of m while it has not expired, and otherwise fetches the file,
// keeps a copy for ttl, and serves the copy. The copy is written whole before anything is
// served, so that a file over the size limit or cut short is neither kept nor sent.
func (p *MediaProxy) openCached(ctx context.Context, m content.RemoteMedia, ttl time.Duration) (*ProxiedMedia, error) {
	if pm, err := p.openCopy(ctx, m.RemoteURL, ttl); pm != nil || err != nil {
		if err == nil {
			err = p.checkEmojiCopy(m, pm)
		}
		if err != nil {
			return nil, err
		}
		return pm, nil
	}
	f, err := p.fetch(ctx, m)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.body.Close() }()
	body := &errReader{r: f.body}
	e, err := p.keepCopy(ctx, m.RemoteURL, body, f.contentType, ttl)
	if body.err != nil {
		return nil, readError(m, body.err)
	}
	if err != nil {
		return nil, fmt.Errorf("proxy media %s: keep a copy: %w", m.MediaID, err)
	}
	rc, info, err := p.d.Blobs.Get(ctx, e.BlobKey)
	if err != nil {
		return nil, fmt.Errorf("proxy media %s: read the copy: %w", m.MediaID, err)
	}
	return &ProxiedMedia{ContentType: e.ContentType, Length: info.Size, MaxAge: ttl, Body: rc}, nil
}

// keepCopy writes the content of r under a new key and records it as the cache entry of url for
// ttl. The key is reserved on the work list of blob deletions before the write and kept when
// the entry is recorded, so that a blob whose entry is never recorded is deleted (blobJournal).
// The entry it replaces is read locked, and its blob released, in the same transaction, so that
// of two concurrent refreshes neither leaves the other's blob behind.
func (p *MediaProxy) keepCopy(ctx context.Context, url string, r io.Reader, contentType string, ttl time.Duration) (content.RemoteMediaCacheEntry, error) {
	journal := p.journal()
	key := remoteMediaKey(url)
	if err := journal.reserve(ctx, key); err != nil {
		return content.RemoteMediaCacheEntry{}, err
	}
	info, err := p.d.Blobs.Put(ctx, key, r, blob.PutOptions{ContentType: contentType})
	if err != nil {
		journal.discard(ctx, key)
		return content.RemoteMediaCacheEntry{}, err
	}
	now := p.d.Clock.Now()
	e := content.RemoteMediaCacheEntry{
		URL: url, BlobKey: key, ContentType: contentType, FileSize: info.Size,
		FetchedAt: now, ExpiresAt: now.Add(ttl), LastAccessedAt: now,
	}
	err = p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := journal.keep(ctx, tx, key); err != nil {
			return err
		}
		// A concurrent first fetch of the URL may insert its entry between the lookup and the
		// insert; the lookup after that finds it.
		for range 3 {
			prev, err := p.d.Repository.LockRemoteMediaCache(ctx, tx, url)
			switch {
			case errors.Is(err, ErrNotFound):
				inserted, err := p.d.Repository.InsertRemoteMediaCache(ctx, tx, e)
				if err != nil || inserted {
					return err
				}
				continue
			case err != nil:
				return err
			}
			if err := p.d.Repository.UpsertRemoteMediaCache(ctx, tx, e); err != nil {
				return err
			}
			if prev.BlobKey == key {
				return nil
			}
			return journal.release(ctx, tx, prev.BlobKey)
		}
		return fmt.Errorf("content: the cache entry of %s keeps changing", url)
	})
	if err != nil {
		// A commit that failed to answer may have recorded the entry.
		if got, ferr := p.d.Repository.FindRemoteMediaCache(context.WithoutCancel(ctx), url); ferr == nil && got.BlobKey != key || errors.Is(ferr, ErrNotFound) {
			journal.discard(ctx, key)
		}
		return content.RemoteMediaCacheEntry{}, fmt.Errorf("record the copy: %w", err)
	}
	return e, nil
}

// journal returns the work list of blob deletions on the proxy's ports.
func (p *MediaProxy) journal() blobJournal {
	return blobJournal{uow: p.d.UnitOfWork, repo: p.d.Repository, blobs: p.d.Blobs, clock: p.d.Clock, logger: p.d.Logger}
}

// openCopy opens the copy of remoteURL, or returns nil when there is none to serve: no entry,
// an expired one, or one whose blob is gone (the cleanup deleted it, or a write was lost).
func (p *MediaProxy) openCopy(ctx context.Context, remoteURL string, ttl time.Duration) (*ProxiedMedia, error) {
	e, err := p.d.Repository.FindRemoteMediaCache(ctx, remoteURL)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}
	now := p.d.Clock.Now()
	if !now.Before(e.ExpiresAt) {
		return nil, nil
	}
	rc, info, err := p.d.Blobs.Get(ctx, e.BlobKey)
	switch {
	case errors.Is(err, blob.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}
	if now.Sub(e.LastAccessedAt) >= cacheTouchInterval {
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return p.d.Repository.TouchRemoteMediaCache(ctx, tx, e.URL, now) })
		if err != nil {
			// Serving the copy matters more than the order of the size limit's cleanup.
			p.d.Logger.WarnContext(ctx, "remote media cache access not recorded", slog.String("url", e.URL), slog.Any("error", err))
		}
	}
	return &ProxiedMedia{ContentType: e.ContentType, Length: info.Size, MaxAge: ttl, Body: rc}, nil
}

// errReader keeps the first error of r other than io.EOF, so that a failed blob write can be
// told apart from a failed read of the remote file whatever error the BlobStore returns.
type errReader struct {
	r   io.Reader
	err error
}

func (e *errReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && e.err == nil {
		e.err = err
	}
	return n, err
}

// CleanupCache deletes the copies of the remote media cache that have expired, then the least
// recently accessed ones while the cache is over its size limit, and returns how many it
// deleted (FR-803). It first deletes the blobs that are due on the work list of blob deletions
// (blobJournal), such as those of copies that a refresh replaced. Each copy's row goes in a
// transaction that records its blob key on that list, and then its blob; a failure leaves the
// key for the next run. A row fetched again since it was read is kept, and so is its blob,
// which has a key of its own. One run looks at a bounded number of copies; the next run
// continues.
func (p *MediaProxy) CleanupCache(ctx context.Context) (int, error) {
	if p.d.Blobs == nil || p.d.UnitOfWork == nil || p.d.Clock == nil {
		return 0, nil
	}
	if _, err := p.journal().sweep(ctx); err != nil {
		return 0, err
	}
	deleted := 0
	now := p.d.Clock.Now()
	for range cleanupMaxBatches {
		entries, err := p.d.Repository.ListExpiredRemoteMediaCache(ctx, now, cleanupBatch)
		if err != nil {
			return deleted, err
		}
		for _, e := range entries {
			ok, err := p.evict(ctx, e)
			if err != nil {
				return deleted, err
			}
			if ok {
				deleted++
			}
		}
		if len(entries) < cleanupBatch {
			break
		}
	}

	size, err := p.d.Repository.RemoteMediaCacheSize(ctx)
	if err != nil {
		return deleted, err
	}
	for batch := 0; size > p.d.MaxCacheSize && batch < cleanupMaxBatches; batch++ {
		entries, err := p.d.Repository.ListLeastRecentlyAccessedRemoteMediaCache(ctx, cleanupBatch)
		if err != nil {
			return deleted, err
		}
		if len(entries) == 0 {
			break
		}
		for _, e := range entries {
			if size <= p.d.MaxCacheSize {
				break
			}
			ok, err := p.evict(ctx, e)
			if err != nil {
				return deleted, err
			}
			if ok {
				size -= e.FileSize
				deleted++
			}
		}
	}
	return deleted, nil
}

// evict deletes the row of a cache entry unless it was fetched again since it was read, with
// its blob key recorded on the work list of blob deletions, then the blob, and reports whether
// it deleted the row.
func (p *MediaProxy) evict(ctx context.Context, e content.RemoteMediaCacheEntry) (bool, error) {
	journal := p.journal()
	var deleted bool
	err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if deleted, err = p.d.Repository.DeleteRemoteMediaCache(ctx, tx, e.URL, e.FetchedAt); err != nil || !deleted {
			return err
		}
		return journal.release(ctx, tx, e.BlobKey)
	})
	if err != nil || !deleted {
		return false, err
	}
	journal.deleteNow(ctx, e.BlobKey)
	return true, nil
}
