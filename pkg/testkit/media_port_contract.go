// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"cmp"
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// MediaRepositoryContract verifies the media repository of a database adapter by its methods
// alone, the port contract of ADR-0153 §1, with statuses that the status repository stores for
// the attachments: media files with their uploads and processing results, the newest of which is
// the file's state; the variants of the ready files, also read in the writing transaction; the
// focus and description; the attachments that keep a file from deletion, the unattached uploads
// and the uploads of an owner; the results replaced before a time and the uploads to reprocess;
// remote media inserted once per owner and URL; the remote media cache with its expiry, accesses,
// size, deletion of the copy that was read, and LockRemoteMediaCache, which lets one of two
// refreshes of a URL replace a copy (ADR-0153 §5); the blob deletions due; and the bytes that the
// local uploads keep. The media use cases on top of it are MediaFlows, which
// internal/flowtest/content runs once on SQLite.
func MediaRepositoryContract(t *testing.T, newHarness MediaFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *mediaRepo)
	}{
		{"StoresUploadsAndTheirResults", mediaRepoResults},
		{"ListsTheReadyVariants", mediaRepoReadyVariants},
		{"KeepsFocusAndDescription", mediaRepoFocus},
		{"DeletesOnlyUnattachedMedia", mediaRepoUnattached},
		{"ListsResultsToPurgeAndUploadsToReprocess", mediaRepoReprocessing},
		{"InsertsRemoteMediaOnce", mediaRepoRemote},
		{"KeepsTheRemoteMediaCache", mediaRepoCache},
		{"LetsOneRefreshReplaceACopy", mediaRepoCacheLock},
		{"ListsTheBlobDeletionsDue", mediaRepoBlobDeletions},
		{"CountsTheBytesOfLocalUploads", mediaRepoLocalSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h MediaHarness
			se := newStatusEnv(t, func(t *testing.T, opts uow.Options) StatusHarness {
				h = newHarness(t, opts)
				return StatusHarness{UnitOfWork: h.UnitOfWork, Statuses: h.Statuses}
			})
			tc.run(t, &mediaRepo{se: se, h: h, repo: h.Media})
		})
	}
}

type mediaRepo struct {
	se   *statusEnv
	h    MediaHarness
	repo appcontent.MediaRepository
}

func (r *mediaRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func (r *mediaRepo) id() string { return r.se.ids.NewID() }

// upload stores an uploaded media file of owner, a second after the previous one.
func (r *mediaRepo) upload(t *testing.T, owner string, size int64) (content.MediaFile, content.UploadedMedia) {
	t.Helper()
	r.se.clock.Advance(time.Second)
	m := content.MediaFile{ID: r.id(), OwnerAccountID: owner, Origin: content.MediaOriginUpload, CreatedAt: r.se.clock.Now()}
	u := content.UploadedMedia{MediaID: m.ID, BlobKey: "media/" + m.ID, ContentType: "image/png", FileSize: size}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertMediaFile(ctx, tx, m); err != nil {
			return err
		}
		return r.repo.InsertUploadedMedia(ctx, tx, u)
	})
	return m, u
}

// result stores a processing result of a file, a second after the previous one, with a small and
// a large still variant of size bytes each when it is ready.
func (r *mediaRepo) result(t *testing.T, mediaID string, purpose content.MediaPurpose, outcome content.MediaOutcome, typ content.MediaType, size int64) content.MediaProcessingResult {
	t.Helper()
	r.se.clock.Advance(time.Second)
	res := content.MediaProcessingResult{
		ID: r.id(), MediaID: mediaID, Purpose: purpose, Outcome: outcome, MediaType: typ, ProcessedAt: r.se.clock.Now(),
	}
	if outcome == content.MediaReady {
		res.Width, res.Height, res.Blurhash = 640, 480, "LEHV6nWB2yk8"
		for _, s := range []content.MediaSize{content.SizeLarge, content.SizeSmall} {
			res.Variants = append(res.Variants, content.MediaVariant{
				ResultID: res.ID, Size: s, Motion: content.MotionStill, BlobKey: "media/" + res.ID + "/" + string(s),
				ContentType: "image/webp", FileSize: size, Width: 64, Height: 48,
			})
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.InsertProcessingResult(ctx, tx, res) })
	return res
}

func mediaRepoResults(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	owner := r.id()
	m, u := r.upload(t, owner, 1000)
	if got, err := r.repo.FindMediaFile(ctx, m.ID); err != nil || got != m {
		t.Errorf("FindMediaFile = %+v, %v; want %+v", got, err, m)
	}
	if got, err := r.repo.FindUploadedMedia(ctx, m.ID); err != nil || got != u {
		t.Errorf("FindUploadedMedia = %+v, %v; want %+v", got, err, u)
	}
	if _, err := r.repo.FindMediaFile(ctx, r.id()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindMediaFile(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.repo.FindLatestProcessingResult(ctx, m.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindLatestProcessingResult before processing = %v, want ErrNotFound", err)
	}
	failed := r.result(t, m.ID, content.PurposeAttachment, content.MediaFailed, content.MediaTypeUnknown, 0)
	ready := r.result(t, m.ID, content.PurposeAvatar, content.MediaReady, content.MediaTypeImage, 100)
	if got, err := r.repo.FindLatestProcessingResult(ctx, m.ID); err != nil || !sameResult(got, ready) {
		t.Errorf("FindLatestProcessingResult = %+v, %v; want %+v", got, err, ready)
	}
	if got, err := r.repo.ListProcessingResults(ctx, m.ID); err != nil || len(got) != 2 || !sameResult(got[0], failed) || !sameResult(got[1], ready) {
		t.Errorf("ListProcessingResults = %+v, %v; want the failed result, then the ready one", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, tc := range []struct {
			purpose content.MediaPurpose
			want    bool
		}{{content.PurposeAvatar, true}, {content.PurposeAttachment, false}} {
			if got, err := r.repo.HasProcessingResult(ctx, tx, m.ID, tc.purpose); err != nil || got != tc.want {
				t.Errorf("HasProcessingResult(%s) = %v, %v; want %v (only the newest counts)", tc.purpose, got, err, tc.want)
			}
		}
		for _, tc := range []struct {
			after string
			want  bool
		}{{failed.ID, true}, {ready.ID, false}} {
			if got, err := r.repo.HasNewerProcessingResult(ctx, tx, m.ID, tc.after); err != nil || got != tc.want {
				t.Errorf("HasNewerProcessingResult(%s) = %v, %v; want %v", tc.after, got, err, tc.want)
			}
		}
		keys, err := r.repo.DeleteProcessingResult(ctx, tx, ready.ID)
		slices.Sort(keys)
		if want := []string{ready.Variants[0].BlobKey, ready.Variants[1].BlobKey}; err != nil || !slices.Equal(keys, want) {
			t.Errorf("DeleteProcessingResult = %v, %v; want the keys of its variants %v", keys, err, want)
		}
		if keys, err := r.repo.DeleteProcessingResult(ctx, tx, ready.ID); err != nil || len(keys) != 0 {
			t.Errorf("DeleteProcessingResult(again) = %v, %v; want none", keys, err)
		}
		return nil
	})
	if got, err := r.repo.FindLatestProcessingResult(ctx, m.ID); err != nil || !sameResult(got, failed) {
		t.Errorf("FindLatestProcessingResult after the deletion = %+v, %v; want %+v", got, err, failed)
	}
}

// sameResult reports whether a and b are the same result with the same variants, in any order.
func sameResult(a, b content.MediaProcessingResult) bool {
	sortVariants := func(vs []content.MediaVariant) []content.MediaVariant {
		vs = slices.Clone(vs)
		slices.SortFunc(vs, func(x, y content.MediaVariant) int { return cmp.Compare(x.BlobKey, y.BlobKey) })
		if len(vs) == 0 {
			return nil
		}
		return vs
	}
	av, bv := sortVariants(a.Variants), sortVariants(b.Variants)
	a.Variants, b.Variants = nil, nil
	return reflect.DeepEqual(a, b) && reflect.DeepEqual(av, bv)
}

func mediaRepoReadyVariants(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	owner := r.id()
	ready, _ := r.upload(t, owner, 10)
	readyResult := r.result(t, ready.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)
	waiting, _ := r.upload(t, owner, 10)
	failing, _ := r.upload(t, owner, 10)
	r.result(t, failing.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)
	r.result(t, failing.ID, content.PurposeAttachment, content.MediaFailed, content.MediaTypeUnknown, 0)
	ids := []string{failing.ID, r.id(), waiting.ID, ready.ID}
	check := func(name string, got []appcontent.FileVariant, err error) {
		t.Helper()
		if err != nil || len(got) != 2 {
			t.Fatalf("%s = %+v, %v; want the 2 variants of the ready file", name, got, err)
		}
		var keys []string
		for _, v := range got {
			if v.File != ready {
				t.Errorf("%s file = %+v, want %+v", name, v.File, ready)
			}
			keys = append(keys, v.Variant.BlobKey)
		}
		slices.Sort(keys)
		if want := []string{readyResult.Variants[0].BlobKey, readyResult.Variants[1].BlobKey}; !slices.Equal(keys, want) {
			t.Errorf("%s keys = %v, want %v", name, keys, want)
		}
	}
	got, err := r.repo.ListReadyVariants(ctx, ids)
	check("ListReadyVariants", got, err)
	// In a transaction, the reads see what it wrote.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.repo.ListReadyVariantsTx(ctx, tx, ids)
		check("ListReadyVariantsTx", got, err)
		if _, err := r.repo.DeleteProcessingResult(ctx, tx, readyResult.ID); err != nil {
			return err
		}
		if got, err := r.repo.ListReadyVariantsTx(ctx, tx, ids); err != nil || len(got) != 0 {
			t.Errorf("ListReadyVariantsTx after deleting the result in tx = %+v, %v; want none", got, err)
		}
		return nil
	})
	if got, err := r.repo.ListReadyVariants(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("ListReadyVariants(no IDs) = %+v, %v; want none", got, err)
	}
}

func mediaRepoFocus(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	m, _ := r.upload(t, r.id(), 10)
	if _, err := r.repo.FindFocus(ctx, m.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindFocus(none) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.UpsertFocus(ctx, tx, content.MediaFocus{MediaID: m.ID, X: 0.5, Y: -0.5}),
			r.repo.UpsertFocus(ctx, tx, content.MediaFocus{MediaID: m.ID, X: -0.25, Y: 0.75}),
			r.repo.UpsertDescription(ctx, tx, content.MediaDescription{MediaID: m.ID, Text: "first"}),
			r.repo.UpsertDescription(ctx, tx, content.MediaDescription{MediaID: m.ID, Text: "A cat"}),
		)
	})
	if got, err := r.repo.FindFocus(ctx, m.ID); err != nil || got != (content.MediaFocus{MediaID: m.ID, X: -0.25, Y: 0.75}) {
		t.Errorf("FindFocus = %+v, %v; want the second focus", got, err)
	}
	if got, err := r.repo.FindDescription(ctx, m.ID); err != nil || got.Text != "A cat" {
		t.Errorf("FindDescription = %+v, %v; want the second description", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.repo.DeleteDescription(ctx, tx, m.ID), r.repo.DeleteDescription(ctx, tx, m.ID))
	})
	if _, err := r.repo.FindDescription(ctx, m.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindDescription after DeleteDescription = %v, want ErrNotFound", err)
	}
}

func mediaRepoUnattached(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	owner, other := r.id(), r.id()
	attached, _ := r.upload(t, owner, 10)
	loose, _ := r.upload(t, owner, 10)
	r.result(t, loose.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.UpsertFocus(ctx, tx, content.MediaFocus{MediaID: loose.ID, X: 0.1, Y: 0.1}),
			r.repo.UpsertDescription(ctx, tx, content.MediaDescription{MediaID: loose.ID, Text: "loose"}),
		)
	})
	others, _ := r.upload(t, other, 10)
	createdBefore := r.se.clock.Now().Add(time.Second)
	r.se.clock.Advance(time.Hour)
	newer, _ := r.upload(t, owner, 10)
	s := r.se.newStatus(owner, content.VisibilityPublic)
	rev := r.se.firstRevision(s)
	rev.Attachments = []content.Attachment{{Position: 0, MediaID: attached.ID, Description: "attached"}}
	if err := r.se.create(t, s, nil, rev); err != nil {
		t.Fatal(err)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, tc := range []struct {
			id   string
			want bool
		}{{attached.ID, true}, {loose.ID, false}} {
			if got, err := r.repo.IsAttached(ctx, tx, tc.id); err != nil || got != tc.want {
				t.Errorf("IsAttached(%s) = %v, %v; want %v", tc.id, got, err, tc.want)
			}
		}
		return nil
	})
	var unattached []string
	after := ""
	for range 3 {
		page, err := r.repo.ListUnattachedUploads(ctx, after, createdBefore, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range page {
			unattached, after = append(unattached, id), id
		}
	}
	if want := []string{loose.ID, others.ID}; !slices.Equal(unattached, want) {
		t.Errorf("ListUnattachedUploads pages = %v, want the unattached uploads created before the cutoff %v", unattached, want)
	}
	owned, err := r.repo.ListOwnerUploads(ctx, owner, "", 10)
	want := []appcontent.OwnedUpload{{MediaID: attached.ID, Attached: true}, {MediaID: loose.ID}, {MediaID: newer.ID}}
	if err != nil || !reflect.DeepEqual(owned, want) {
		t.Errorf("ListOwnerUploads = %+v, %v; want %+v", owned, err, want)
	}
	if owned, err := r.repo.ListOwnerUploads(ctx, owner, attached.ID, 1); err != nil || !reflect.DeepEqual(owned, want[1:2]) {
		t.Errorf("ListOwnerUploads(after the first, limit 1) = %+v, %v; want %+v", owned, err, want[1:2])
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, tc := range []struct {
			id   string
			want bool
		}{{attached.ID, false}, {loose.ID, true}, {loose.ID, false}, {r.id(), false}} {
			if got, err := r.repo.DeleteUnattachedMedia(ctx, tx, tc.id); err != nil || got != tc.want {
				t.Errorf("DeleteUnattachedMedia(%s) = %v, %v; want %v", tc.id, got, err, tc.want)
			}
		}
		return nil
	})
	if _, err := r.repo.FindMediaFile(ctx, loose.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindMediaFile of a deleted file = %v, want ErrNotFound", err)
	}
	for name, err := range map[string]error{
		"FindUploadedMedia":          second(r.repo.FindUploadedMedia(ctx, loose.ID)),
		"FindLatestProcessingResult": second(r.repo.FindLatestProcessingResult(ctx, loose.ID)),
		"FindFocus":                  second(r.repo.FindFocus(ctx, loose.ID)),
		"FindDescription":            second(r.repo.FindDescription(ctx, loose.ID)),
	} {
		if !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("%s of a deleted file = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := r.repo.FindMediaFile(ctx, attached.ID); err != nil {
		t.Errorf("FindMediaFile of an attached file = %v", err)
	}
}

func mediaRepoReprocessing(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	owner := r.id()
	image, _ := r.upload(t, owner, 10)
	old := r.result(t, image.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)
	current := r.result(t, image.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)
	video, _ := r.upload(t, owner, 10)
	r.result(t, video.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeVideo, 5)
	failed, _ := r.upload(t, owner, 10)
	r.result(t, failed.ID, content.PurposeAttachment, content.MediaFailed, content.MediaTypeUnknown, 0)
	cutoff := r.se.clock.Now().Add(time.Second)
	r.se.clock.Advance(time.Hour)
	recent, _ := r.upload(t, owner, 10)
	r.result(t, recent.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)
	again := r.result(t, recent.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 5)

	if got, err := r.repo.ListReplacedProcessingResults(ctx, "", cutoff, 10); err != nil || !slices.Equal(got, []string{old.ID}) {
		t.Errorf("ListReplacedProcessingResults(before the cutoff) = %v, %v; want %v", got, err, []string{old.ID})
	}
	all, err := r.repo.ListReplacedProcessingResults(ctx, "", again.ProcessedAt.Add(time.Second), 10)
	if err != nil || len(all) != 2 || all[0] != old.ID || slices.Contains(all, current.ID) || slices.Contains(all, again.ID) {
		t.Errorf("ListReplacedProcessingResults(now) = %v, %v; want the 2 replaced results, never the newest", all, err)
	}
	if got, err := r.repo.ListReplacedProcessingResults(ctx, old.ID, again.ProcessedAt.Add(time.Second), 10); err != nil || len(got) != 1 {
		t.Errorf("ListReplacedProcessingResults(after the first) = %v, %v; want 1", got, err)
	}
	if got, err := r.repo.ListReprocessableUploads(ctx, "", cutoff, 10); err != nil || !slices.Equal(got, []string{image.ID}) {
		t.Errorf("ListReprocessableUploads = %v, %v; want only the ready image processed before the cutoff %v", got, err, image.ID)
	}
	later := again.ProcessedAt.Add(time.Second)
	if got, err := r.repo.ListReprocessableUploads(ctx, image.ID, later, 10); err != nil || !slices.Equal(got, []string{recent.ID}) {
		t.Errorf("ListReprocessableUploads(after the first) = %v, %v; want %v", got, err, []string{recent.ID})
	}
}

func mediaRepoRemote(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	owner := r.id()
	upload, _ := r.upload(t, owner, 10)
	remote := func(url string) (content.RemoteMedia, bool) {
		t.Helper()
		r.se.clock.Advance(time.Second)
		m := content.MediaFile{ID: r.id(), OwnerAccountID: owner, Origin: content.MediaOriginRemote, CreatedAt: r.se.clock.Now()}
		rm := content.RemoteMedia{
			MediaID: m.ID, OwnerAccountID: owner, RemoteURL: url, DeclaredContentType: "image/jpeg",
			DeclaredMediaType: content.MediaTypeImage, Purpose: content.PurposeAttachment, Width: 800, Height: 600,
			Blurhash: "LEHV6nWB2yk8", Focus: &content.MediaFocus{MediaID: m.ID, X: 0.5, Y: 0.25}, Description: "A dog",
		}
		var inserted bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			if err := r.repo.InsertMediaFile(ctx, tx, m); err != nil {
				return err
			}
			inserted, err = r.repo.InsertRemoteMedia(ctx, tx, rm)
			return err
		})
		return rm, inserted
	}
	first, ok := remote("https://remote.example/a.jpg")
	if !ok {
		t.Fatal("InsertRemoteMedia = false, want true")
	}
	if _, ok := remote(first.RemoteURL); ok {
		t.Error("InsertRemoteMedia(the owner's URL again) = true, want false")
	}
	plain, _ := remote("https://remote.example/b.jpg")
	if got, err := r.repo.FindRemoteMedia(ctx, first.MediaID); err != nil || !reflect.DeepEqual(got, first) {
		t.Errorf("FindRemoteMedia = %+v, %v; want %+v", got, err, first)
	}
	if _, err := r.repo.FindRemoteMedia(ctx, upload.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindRemoteMedia(an upload) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.RemoteMediaByURL(ctx, tx, owner, first.RemoteURL); err != nil || !reflect.DeepEqual(got, first) {
			t.Errorf("RemoteMediaByURL = %+v, %v; want %+v", got, err, first)
		}
		if _, err := r.repo.RemoteMediaByURL(ctx, tx, r.id(), first.RemoteURL); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("RemoteMediaByURL(another owner) = %v, want ErrNotFound", err)
		}
		got, err := r.repo.ListRemoteMediaTx(ctx, tx, []string{plain.MediaID, upload.ID, first.MediaID})
		if want := []content.RemoteMedia{first, plain}; err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ListRemoteMediaTx = %+v, %v; want %+v", got, err, want)
		}
		return nil
	})
	got, err := r.repo.ListRemoteMedia(ctx, []string{plain.MediaID, r.id(), upload.ID, first.MediaID})
	if want := []content.RemoteMedia{first, plain}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ListRemoteMedia = %+v, %v; want the remote files in ID order %+v", got, err, want)
	}
}

func (r *mediaRepo) cacheEntry(url string, size int64, ttl time.Duration) content.RemoteMediaCacheEntry {
	r.se.clock.Advance(time.Second)
	now := r.se.clock.Now()
	return content.RemoteMediaCacheEntry{
		URL: url, BlobKey: "cache/" + r.id(), ContentType: "image/webp", FileSize: size,
		FetchedAt: now, ExpiresAt: now.Add(ttl), LastAccessedAt: now,
	}
}

func mediaRepoCache(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	a := r.cacheEntry("https://remote.example/a.jpg", 100, 3*time.Hour)
	b := r.cacheEntry("https://remote.example/b.jpg", 200, time.Hour)
	c := r.cacheEntry("https://remote.example/c.jpg", 300, 2*time.Hour)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, e := range []content.RemoteMediaCacheEntry{a, b, c} {
			if inserted, err := r.repo.InsertRemoteMediaCache(ctx, tx, e); err != nil || !inserted {
				t.Errorf("InsertRemoteMediaCache = %v, %v; want true", inserted, err)
			}
		}
		other := a
		other.BlobKey = "cache/other"
		if inserted, err := r.repo.InsertRemoteMediaCache(ctx, tx, other); err != nil || inserted {
			t.Errorf("InsertRemoteMediaCache(a URL with an entry) = %v, %v; want false", inserted, err)
		}
		return nil
	})
	if got, err := r.repo.FindRemoteMediaCache(ctx, a.URL); err != nil || got != a {
		t.Errorf("FindRemoteMediaCache = %+v, %v; want %+v", got, err, a)
	}
	if _, err := r.repo.FindRemoteMediaCache(ctx, "https://remote.example/none.jpg"); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindRemoteMediaCache(unknown) = %v, want ErrNotFound", err)
	}
	if n, err := r.repo.RemoteMediaCacheSize(ctx); err != nil || n != 600 {
		t.Errorf("RemoteMediaCacheSize = %d, %v; want 600", n, err)
	}

	// Accessing a makes b the least recently accessed; a touch of a missing entry is no error.
	r.se.clock.Advance(time.Second)
	touched := r.se.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.repo.TouchRemoteMediaCache(ctx, tx, a.URL, touched), r.repo.TouchRemoteMediaCache(ctx, tx, "https://remote.example/none.jpg", touched))
	})
	urls := func(es []content.RemoteMediaCacheEntry) []string {
		out := make([]string, len(es))
		for i, e := range es {
			out[i] = e.URL
		}
		return out
	}
	if got, err := r.repo.ListLeastRecentlyAccessedRemoteMediaCache(ctx, 2); err != nil || !slices.Equal(urls(got), []string{b.URL, c.URL}) {
		t.Errorf("ListLeastRecentlyAccessedRemoteMediaCache = %v, %v; want %v", urls(got), err, []string{b.URL, c.URL})
	}
	if got, err := r.repo.ListExpiredRemoteMediaCache(ctx, c.ExpiresAt, 10); err != nil || !slices.Equal(urls(got), []string{b.URL, c.URL}) {
		t.Errorf("ListExpiredRemoteMediaCache = %v, %v; want the earliest expiry first %v", urls(got), err, []string{b.URL, c.URL})
	}
	if got, err := r.repo.ListExpiredRemoteMediaCache(ctx, c.ExpiresAt, 1); err != nil || !slices.Equal(urls(got), []string{b.URL}) {
		t.Errorf("ListExpiredRemoteMediaCache(limit 1) = %v, %v", urls(got), err)
	}

	// Upserting replaces the entry; deleting the copy read before then deletes nothing.
	refreshed := r.cacheEntry(b.URL, 50, time.Hour)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.UpsertRemoteMediaCache(ctx, tx, refreshed); err != nil {
			return err
		}
		if deleted, err := r.repo.DeleteRemoteMediaCache(ctx, tx, b.URL, b.FetchedAt); err != nil || deleted {
			t.Errorf("DeleteRemoteMediaCache(an older copy) = %v, %v; want false", deleted, err)
		}
		if deleted, err := r.repo.DeleteRemoteMediaCache(ctx, tx, c.URL, c.FetchedAt); err != nil || !deleted {
			t.Errorf("DeleteRemoteMediaCache = %v, %v; want true", deleted, err)
		}
		return nil
	})
	if got, err := r.repo.FindRemoteMediaCache(ctx, b.URL); err != nil || got != refreshed {
		t.Errorf("FindRemoteMediaCache after the upsert = %+v, %v; want %+v", got, err, refreshed)
	}
	if n, err := r.repo.RemoteMediaCacheSize(ctx); err != nil || n != 150 {
		t.Errorf("RemoteMediaCacheSize = %d, %v; want 150", n, err)
	}
}

// mediaRepoCacheLock refreshes one URL from several transactions at once, each replacing the copy
// it read only when it is still the stored one: LockRemoteMediaCache makes the later ones wait and
// see the new copy, so exactly one replaces it.
func mediaRepoCacheLock(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	original := r.cacheEntry("https://remote.example/a.jpg", 100, time.Hour)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.UpsertRemoteMediaCache(ctx, tx, original) })
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		replaced int
	)
	for i := range 4 {
		next := original
		next.BlobKey, next.FetchedAt = "cache/refresh", original.FetchedAt.Add(time.Duration(i+1)*time.Second)
		wg.Go(func() {
			err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				cur, err := r.repo.LockRemoteMediaCache(ctx, tx, original.URL)
				if err != nil {
					return err
				}
				if !cur.FetchedAt.Equal(original.FetchedAt) {
					return nil
				}
				if err := r.repo.UpsertRemoteMediaCache(ctx, tx, next); err != nil {
					return err
				}
				mu.Lock()
				replaced++
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("refresh = %v", err)
			}
		})
	}
	wg.Wait()
	if replaced != 1 {
		t.Errorf("%d refreshes replaced the copy, want 1", replaced)
	}
	if _, err := r.repo.FindRemoteMediaCache(ctx, original.URL); err != nil {
		t.Errorf("FindRemoteMediaCache = %v", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.repo.LockRemoteMediaCache(ctx, tx, "https://remote.example/none.jpg"); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("LockRemoteMediaCache(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func mediaRepoBlobDeletions(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	now := r.se.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.RecordBlobDeletion(ctx, tx, "b", now.Add(2*time.Minute)),
			r.repo.RecordBlobDeletion(ctx, tx, "a", now.Add(time.Minute)),
			r.repo.RecordBlobDeletion(ctx, tx, "c", now.Add(time.Hour)),
			// Recording a key again replaces its time.
			r.repo.RecordBlobDeletion(ctx, tx, "c", now.Add(30*time.Second)),
		)
	})
	at := now.Add(2 * time.Minute)
	if got, err := r.repo.ListDueBlobDeletions(ctx, at, 10); err != nil || !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Errorf("ListDueBlobDeletions = %v, %v; want the earliest first [c a b]", got, err)
	}
	if got, err := r.repo.ListDueBlobDeletions(ctx, at, 2); err != nil || !slices.Equal(got, []string{"c", "a"}) {
		t.Errorf("ListDueBlobDeletions(limit 2) = %v, %v", got, err)
	}
	if got, err := r.repo.ListDueBlobDeletions(ctx, now, 10); err != nil || len(got) != 0 {
		t.Errorf("ListDueBlobDeletions(before any is due) = %v, %v; want none", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			if forgot, err := r.repo.ForgetBlobDeletion(ctx, tx, "a"); err != nil || forgot != want {
				t.Errorf("ForgetBlobDeletion #%d = %v, %v; want %v", i+1, forgot, err, want)
			}
		}
		return nil
	})
	if got, err := r.repo.ListDueBlobDeletions(ctx, at, 10); err != nil || !slices.Equal(got, []string{"c", "b"}) {
		t.Errorf("ListDueBlobDeletions after ForgetBlobDeletion = %v, %v", got, err)
	}
}

func mediaRepoLocalSize(t *testing.T, r *mediaRepo) {
	ctx := t.Context()
	owner := r.id()
	if n, err := r.repo.LocalMediaSize(ctx); err != nil || n != 0 {
		t.Errorf("LocalMediaSize(empty) = %d, %v; want 0", n, err)
	}
	r.upload(t, owner, 1000) // waits for its first processing: its original counts
	processed, _ := r.upload(t, owner, 2000)
	r.result(t, processed.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 30)
	r.result(t, processed.ID, content.PurposeAttachment, content.MediaReady, content.MediaTypeImage, 40) // the current one
	failed, _ := r.upload(t, owner, 3000)
	r.result(t, failed.ID, content.PurposeAttachment, content.MediaFailed, content.MediaTypeUnknown, 0)
	if n, err := r.repo.LocalMediaSize(ctx); err != nil || n != 1000+2*40 {
		t.Errorf("LocalMediaSize = %d, %v; want %d", n, err, 1000+2*40)
	}
}
