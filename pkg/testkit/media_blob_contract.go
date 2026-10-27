// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// errBlobPut is the error of a blob write that fails on purpose.
var errBlobPut = errors.New("testkit: blob write failed")

// flakyBlobs is a blob store that, for the keys that putFails matches, stores the blob and then
// fails the write, as a write cancelled after the object was published does; deletions fail
// while failDeletes is set. beforePut, when set, runs once before the first write of a key it
// matches.
type flakyBlobs struct {
	blob.Store
	mu          sync.Mutex
	putFails    func(key string) bool
	failDeletes bool
	beforePut   func(key string) bool
	hook        func()
}

func (f *flakyBlobs) Put(ctx context.Context, key string, r io.Reader, opts blob.PutOptions) (blob.Info, error) {
	f.mu.Lock()
	var hook func()
	if f.hook != nil && f.beforePut != nil && f.beforePut(key) {
		hook, f.hook = f.hook, nil
	}
	fails := f.putFails != nil && f.putFails(key)
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	info, err := f.Store.Put(ctx, key, r, opts)
	if err == nil && fails {
		return blob.Info{}, errBlobPut
	}
	return info, err
}

func (f *flakyBlobs) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	fail := f.failDeletes
	f.mu.Unlock()
	if fail {
		return errBlobDelete
	}
	return f.Store.Delete(ctx, key)
}

// failingResults is a media repository whose processing results are never recorded.
type failingResults struct{ appcontent.MediaRepository }

var errResultInsert = errors.New("testkit: result insert failed")

func (failingResults) InsertProcessingResult(context.Context, uow.Tx, content.MediaProcessingResult) error {
	return errResultInsert
}

// dueBlobDeletions returns the keys on the work list of blob deletions due by at.
func (e *mediaEnv) dueBlobDeletions(t *testing.T, at time.Time) []string {
	t.Helper()
	keys, err := e.h.Media.ListDueBlobDeletions(t.Context(), at, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// mediaKeys returns the blob keys of the environment that contain s.
func (e *mediaEnv) mediaKeys(s string) []string {
	var keys []string
	for _, k := range e.blobs.Keys() {
		if strings.Contains(k, s) {
			keys = append(keys, k)
		}
	}
	return keys
}

// mediaLeavesNoUnrecordedUploads checks that an upload whose blob write fails after the blob
// was stored leaves no blob, and that one whose blob cannot even be deleted then is deleted by
// the cleanup once its grace period is over.
func mediaLeavesNoUnrecordedUploads(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	flaky := &flakyBlobs{Store: e.blobs, putFails: func(string) bool { return true }}
	svc := e.serviceOn(flaky, e.h.Media)
	if _, err := svc.Upload(ctx, appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixturePNG(4, 3, false))}); !errors.Is(err, errBlobPut) {
		t.Fatalf("Upload = %v, want the write's error", err)
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("a failed upload left blobs %v", keys)
	}
	if due := e.dueBlobDeletions(t, e.clock.Now().Add(100*24*time.Hour)); len(due) != 0 {
		t.Errorf("a failed upload left deletions %v", due)
	}

	flaky.failDeletes = true
	if _, err := svc.Upload(ctx, appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixturePNG(4, 3, false))}); !errors.Is(err, errBlobPut) {
		t.Fatalf("Upload = %v, want the write's error", err)
	}
	left := e.blobs.Keys()
	if len(left) != 1 || !strings.HasPrefix(left[0], "uploads/") {
		t.Fatalf("blobs = %v, want the upload whose deletion failed", left)
	}
	if n, err := e.service.CleanupUnattached(ctx); err != nil || n != 0 || len(e.blobs.Keys()) != 1 {
		t.Errorf("a cleanup within the grace period = %d, %v; blobs %v", n, err, e.blobs.Keys())
	}
	e.clock.Advance(appcontent.DefaultUnattachedTTL + time.Minute)
	if _, err := e.service.CleanupUnattached(ctx); err != nil {
		t.Fatal(err)
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("the cleanup left blobs %v of a failed upload", keys)
	}
	if due := e.dueBlobDeletions(t, e.clock.Now().Add(100*24*time.Hour)); len(due) != 0 {
		t.Errorf("deletions %v are left after the cleanup", due)
	}
}

// mediaLeavesNoUnrecordedVariants checks that a processing run that does not record its result
// deletes the variants it wrote: one whose later variant write fails, one whose result insert
// fails, and one that a concurrent run of the same job beats; a run afterwards records the
// result with its variants only.
func mediaLeavesNoUnrecordedVariants(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	m := e.upload(t, FixturePNG(4, 3, false))
	prefix := "media/" + m.File.ID + "/"

	flaky := &flakyBlobs{Store: e.blobs, putFails: func(k string) bool { return strings.Contains(k, "/medium.") }}
	if res := e.serviceOn(flaky, e.h.Media).Process(ctx, m.File.ID, ""); res.IsDone() {
		t.Fatal("Process with a failing variant write is done")
	}
	if keys := e.mediaKeys(prefix); len(keys) != 0 {
		t.Errorf("a failed variant write left variants %v", keys)
	}

	if res := e.serviceOn(e.blobs, failingResults{e.h.Media}).Process(ctx, m.File.ID, ""); res.IsDone() {
		t.Fatal("Process with a failing result insert is done")
	}
	if keys := e.mediaKeys(prefix); len(keys) != 0 {
		t.Errorf("a failed result insert left variants %v", keys)
	}

	// The concurrent run records its result while this one writes its large variant.
	flaky = &flakyBlobs{Store: e.blobs, beforePut: func(k string) bool { return strings.Contains(k, "/large.") }}
	flaky.hook = func() { e.process(t, m.File.ID) }
	if res := e.serviceOn(flaky, e.h.Media).Process(ctx, m.File.ID, ""); !res.IsDone() {
		t.Fatalf("the losing run = %+v, want Done", res)
	}
	got := e.media(t, m.File.ID)
	if !got.Ready() {
		t.Fatalf("Result = %+v", got.Result)
	}
	var want []string
	for _, v := range got.Result.Variants {
		want = append(want, v.BlobKey)
	}
	slices.Sort(want)
	if keys := e.mediaKeys(prefix); !slices.Equal(keys, want) {
		t.Errorf("variants = %v, want only those of the recorded result %v", keys, want)
	}
	if due := e.dueBlobDeletions(t, e.clock.Now().Add(100*24*time.Hour)); len(due) != 0 {
		t.Errorf("deletions %v are left", due)
	}
}

// mediaRetriesFailedCleanupDeletions checks that the cleanup of unattached uploads keeps the
// keys of blobs whose deletion fails after their rows are gone, and deletes them on the next run.
func mediaRetriesFailedCleanupDeletions(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	m := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, m.File.ID)
	if len(e.mediaKeys(m.File.ID)) == 0 {
		t.Fatal("no variants")
	}
	e.clock.Advance(appcontent.DefaultUnattachedTTL + time.Minute)
	flaky := &flakyBlobs{Store: e.blobs, failDeletes: true}
	if n, err := e.serviceOn(flaky, e.h.Media).CleanupUnattached(ctx); err != nil || n != 1 {
		t.Fatalf("CleanupUnattached with failing deletions = %d, %v; want 1", n, err)
	}
	if _, err := e.service.Media(ctx, m.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Media = %v, want ErrNotFound", err)
	}
	left := e.mediaKeys(m.File.ID)
	if len(left) == 0 {
		t.Fatal("the failing deletions deleted the blobs")
	}
	due := e.dueBlobDeletions(t, e.clock.Now())
	for _, k := range left {
		if !slices.Contains(due, k) {
			t.Errorf("blob %s is left without a deletion due", k)
		}
	}
	if _, err := e.service.CleanupUnattached(ctx); err != nil {
		t.Fatal(err)
	}
	if keys := e.mediaKeys(m.File.ID); len(keys) != 0 {
		t.Errorf("the next cleanup left blobs %v", keys)
	}
	if due := e.dueBlobDeletions(t, e.clock.Now()); len(due) != 0 {
		t.Errorf("deletions %v are left", due)
	}
}

// racingCleanup is a media repository on which the cleanup of the remote media cache lets
// refresh run once, after it listed the expired entries and before it deletes them.
type racingCleanup struct {
	appcontent.MediaRepository
	refresh func()
}

func (r *racingCleanup) ListExpiredRemoteMediaCache(ctx context.Context, now time.Time, limit int) ([]content.RemoteMediaCacheEntry, error) {
	entries, err := r.MediaRepository.ListExpiredRemoteMediaCache(ctx, now, limit)
	if len(entries) > 0 && r.refresh != nil {
		f := r.refresh
		r.refresh = nil
		f()
	}
	return entries, err
}

// mediaKeepsCopiesRefreshedDuringCleanup checks that a copy of the remote media cache that a
// request fetches again while the cleanup deletes its expired copy stays readable: the cleanup
// deletes neither its entry nor its blob, and later deletes the blob of the replaced copy.
func mediaKeepsCopiesRefreshedDuringCleanup(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	c := newCacheEnv(t, e, 0)
	c.set(appcontent.RemoteMediaCacheAll, time.Hour)
	m := c.record(t, "https://remote.example/race.png")
	c.open(t, m)
	old, _ := c.entry(t, m.RemoteURL)
	e.clock.Advance(2 * time.Hour)

	cleaner := c.proxyOn(t, &racingCleanup{MediaRepository: e.h.Media, refresh: func() { c.open(t, m) }}, 0)
	if _, err := cleaner.CleanupCache(ctx); err != nil {
		t.Fatal(err)
	}
	fresh, ok := c.entry(t, m.RemoteURL)
	if !ok || fresh.BlobKey == old.BlobKey || !bytes.Equal(e.blobs.Bytes(fresh.BlobKey), c.data) {
		t.Fatalf("after the cleanup, entry = %+v, %v; blob %d bytes", fresh, ok, len(e.blobs.Bytes(fresh.BlobKey)))
	}
	c.open(t, m)
	if n := c.net.count(m.RemoteURL); n != 2 {
		t.Errorf("the remote file was fetched %d times, want 2 (the refreshed copy served)", n)
	}
	if _, err := c.proxy.CleanupCache(ctx); err != nil {
		t.Fatal(err)
	}
	if blobs := c.remoteBlobs(); !slices.Equal(blobs, []string{fresh.BlobKey}) {
		t.Errorf("remote blobs = %v, want only the refreshed copy %s", blobs, fresh.BlobKey)
	}
}
