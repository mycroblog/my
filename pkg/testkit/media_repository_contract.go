// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/blob"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// MediaHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the content module's media and status repositories on it.
type MediaHarness struct {
	UnitOfWork uow.UnitOfWork
	Media      appcontent.MediaRepository
	Statuses   appcontent.Repository
}

// MediaFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type MediaFactory func(t *testing.T, opts uow.Options) MediaHarness

// MediaRepositoryContract verifies the media repository of a database adapter through the
// use cases that drive it: uploads stored with the detected type and their processing job
// enqueued, rejected uploads leaving nothing behind, idempotent processing that records one
// result with its variants for the upload's purpose under keys of the result and deletes the
// file as received, processing again for another purpose, failures recorded as results,
// focal points and descriptions as current values that only the uploader reads and changes
// until a status attaches the file, the cleanup of old unattached uploads that keeps attached
// and referenced ones, the erasure of a deleted account's uploads with their blobs that keeps
// the attached and referenced ones, remote media files recorded once per owner and URL, and the media
// proxy's remote media cache with its cleanup, the work list of blob deletions that leaves no
// blob of a failed or losing write and retries failed deletions, and the bytes that the local
// uploads keep.
func MediaRepositoryContract(t *testing.T, newHarness MediaFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *mediaEnv)
	}{
		{"StoresUploads", mediaStoresUploads},
		{"RejectsUploadsWithoutWriting", mediaRejectsUploads},
		{"ProcessesIdempotently", mediaProcessesIdempotently},
		{"ProcessesAgainForAnotherPurpose", mediaReprocessesForAnotherPurpose},
		{"RecordsFailedProcessing", mediaRecordsFailures},
		{"RetriesOnTransientErrors", mediaRetriesTransient},
		{"KeepsTheFocusAsACurrentValue", mediaKeepsFocus},
		{"ChecksProfileImages", mediaChecksProfileImages},
		{"ReadsImagesTogether", mediaReadsImagesTogether},
		{"UpdatesOnlyOwnUnattachedUploads", mediaUpdates},
		{"CleansUpOldUnattachedUploads", mediaCleansUp},
		{"CleanupPagesPastReferencedUploads", mediaCleanupPages},
		{"ErasesDeletedAccountsUploads", mediaErasesDeletedAccounts},
		{"RecordsRemoteMediaOnce", mediaRecordsRemoteMedia},
		{"CachesRemoteMediaUnderThePolicy", mediaCachesRemoteMedia},
		{"CachesTheSmallVariantOfRemoteMedia", mediaCachesSmallRemoteVariants},
		{"CleansUpTheRemoteMediaCache", mediaCleansUpRemoteMediaCache},
		{"CountsStoredBytes", mediaCountsStoredBytes},
		{"LeavesNoUnrecordedUploads", mediaLeavesNoUnrecordedUploads},
		{"LeavesNoUnrecordedVariants", mediaLeavesNoUnrecordedVariants},
		{"RetriesFailedCleanupDeletions", mediaRetriesFailedCleanupDeletions},
		{"KeepsCopiesRefreshedDuringCleanup", mediaKeepsCopiesRefreshedDuringCleanup},
		{"ProcessesVideoAndAudio", mediaProcessesVideoAndAudio},
		{"RejectsVideoAndAudio", mediaRejectsVideoAndAudio},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newMediaEnv(t, newHarness)) })
	}
}

// recordingJobs is a queue.JobQueue that keeps what it is given.
type recordingJobs struct {
	mu   sync.Mutex
	jobs []queue.NewJob
}

func (r *recordingJobs) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append(r.jobs, jobs...)
	return nil
}

// referrerSet is a MediaReferrer that references a fixed set of media files.
type referrerSet map[string]bool

func (r referrerSet) IsMediaReferenced(_ context.Context, id string) (bool, error) { return r[id], nil }

type mediaEnv struct {
	h         MediaHarness
	clock     *Clock
	ids       *IDGenerator
	blobs     *MemoryBlobStore
	jobs      *recordingJobs
	referrers referrerSet
	service   *appcontent.MediaService
}

const mediaOwner = "0192f0c1-0000-7000-8000-00000000a11c"

func newMediaEnv(t *testing.T, newHarness MediaFactory) *mediaEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appcontent.RegisterProjectors(projectors, h.Statuses)
	clock := NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	e := &mediaEnv{h: h, clock: clock, ids: NewIDGenerator(clock), blobs: NewMemoryBlobStore(clock), jobs: &recordingJobs{}, referrers: referrerSet{}}
	e.service = e.serviceOn(e.blobs, h.Media)
	return e
}

// serviceOn returns a media service of the environment on other blob storage and repository,
// which wrap the environment's to fail on purpose.
func (e *mediaEnv) serviceOn(blobs blob.Store, repo appcontent.MediaRepository) *appcontent.MediaService {
	return appcontent.NewMediaService(appcontent.MediaDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: repo, Blobs: blobs, Processor: FakeMediaProcessor{},
		Jobs: e.jobs, Clock: e.clock, IDs: e.ids, Limits: media.Limits{MaxBytes: 1 << 20},
		Referrers: []appcontent.MediaReferrer{e.referrers},
	})
}

func (e *mediaEnv) upload(t *testing.T, data []byte) appcontent.Media {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	m, err := e.service.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(data)})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return m
}

func (e *mediaEnv) process(t *testing.T, id string) {
	t.Helper()
	e.processFor(t, id, "")
}

func (e *mediaEnv) processFor(t *testing.T, id string, purpose content.MediaPurpose) {
	t.Helper()
	if res := e.service.Process(t.Context(), id, purpose); !res.IsDone() {
		t.Fatalf("Process(%s) = %+v, want Done", id, res)
	}
}

func (e *mediaEnv) media(t *testing.T, id string) appcontent.Media {
	t.Helper()
	m, err := e.service.Media(t.Context(), id)
	if err != nil {
		t.Fatalf("Media(%s): %v", id, err)
	}
	return m
}

func mediaStoresUploads(t *testing.T, e *mediaEnv) {
	data := FixturePNG(4, 3, false)
	m := e.upload(t, data)
	if m.File.OwnerAccountID != mediaOwner || m.File.Origin != content.MediaOriginUpload || !m.File.CreatedAt.Equal(e.clock.Now()) {
		t.Errorf("File = %+v", m.File)
	}
	want := content.UploadedMedia{MediaID: m.File.ID, BlobKey: "uploads/" + m.File.ID, ContentType: "image/png", FileSize: int64(len(data))}
	if m.Upload == nil || *m.Upload != want {
		t.Errorf("Upload = %+v, want %+v", m.Upload, want)
	}
	if m.Result != nil || m.Focus != nil || m.Ready() {
		t.Errorf("a new upload has a result or focus: %+v", m)
	}
	if !bytes.Equal(e.blobs.Bytes(want.BlobKey), data) {
		t.Errorf("the received file is not in blob storage")
	}
	if strings.HasPrefix(want.BlobKey, blob.PublicPrefix) {
		t.Errorf("the received file is public: %s", want.BlobKey)
	}
	if len(e.jobs.jobs) != 1 {
		t.Fatalf("enqueued %d jobs, want 1", len(e.jobs.jobs))
	}
	j := e.jobs.jobs[0]
	var p struct {
		MediaID string `json:"media_id"`
	}
	if err := json.Unmarshal(j.Payload, &p); err != nil || j.Type != appcontent.JobProcessMedia || p.MediaID != m.File.ID || j.IdempotencyKey == "" {
		t.Errorf("job = %+v", j)
	}
	// A declared type that matches, or tells nothing, is accepted.
	for _, declared := range []string{"image/png", "image/png; charset=binary", "application/octet-stream", ""} {
		if _, err := e.service.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(data), DeclaredContentType: declared}); err != nil {
			t.Errorf("declared %q: %v", declared, err)
		}
	}
}

func mediaRejectsUploads(t *testing.T, e *mediaEnv) {
	png := FixturePNG(4, 3, false)
	for name, tc := range map[string]struct {
		data     []byte
		declared string
		want     error
	}{
		"svg":           {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml", media.ErrUnsupportedFormat},
		"html as jpeg":  {[]byte("<html><script>alert(1)</script></html>"), "image/jpeg", media.ErrUnsupportedFormat},
		"png as jpeg":   {png, "image/jpeg", appcontent.ErrContentTypeMismatch},
		"png as gif":    {png, "image/gif", appcontent.ErrContentTypeMismatch},
		"bad declared":  {png, "not a type", appcontent.ErrContentTypeMismatch},
		"too large":     {append(slices.Clone(png), make([]byte, 1<<20)...), "", media.ErrTooLarge},
		"corrupt image": {append(slices.Clone(png), "corrupt"...), "", media.ErrCorrupt},
	} {
		_, err := e.service.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(tc.data), DeclaredContentType: tc.declared})
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: Upload = %v, want %v", name, err, tc.want)
		}
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("rejected uploads left blobs: %v", keys)
	}
	if len(e.jobs.jobs) != 0 {
		t.Errorf("rejected uploads enqueued %d jobs", len(e.jobs.jobs))
	}
	ids, err := e.h.Media.ListUnattachedUploads(t.Context(), "", e.clock.Now().Add(time.Hour), 10)
	if err != nil || len(ids) != 0 {
		t.Errorf("rejected uploads left media files: %v, %v", ids, err)
	}
}

func mediaProcessesIdempotently(t *testing.T, e *mediaEnv) {
	m := e.upload(t, FixturePNG(4, 3, false))
	e.clock.Advance(time.Second)
	for range 3 {
		e.process(t, m.File.ID)
	}
	got := e.media(t, m.File.ID)
	if !got.Ready() {
		t.Fatalf("after processing: %+v", got.Result)
	}
	r := got.Result
	if r.MediaType != content.MediaTypeImage || r.Width != 4 || r.Height != 3 || r.Blurhash == "" || !r.ProcessedAt.Equal(e.clock.Now()) {
		t.Errorf("Result = %+v", r)
	}
	if r.Purpose != content.PurposeAttachment || r.Animated() || len(r.Variants) != 3 {
		t.Fatalf("Result = %+v, want 3 still variants of an attachment", r)
	}
	var keys []string
	for i, size := range content.MediaSizes {
		v, ok := r.Variant(size, content.MotionStill)
		w, h := []int{1, 2, 4}[i], []int{1, 1, 3}[i]
		key := "media/" + m.File.ID + "/" + r.ID + "/" + string(size) + ".png"
		if !ok || v.BlobKey != key || v.ContentType != "image/png" || v.Width != w || v.Height != h || v.ResultID != r.ID {
			t.Errorf("%s = %+v, want %s %d×%d", size, v, key, w, h)
		}
		if v.FileSize != int64(len(e.blobs.Bytes(v.BlobKey))) || v.FileSize == 0 {
			t.Errorf("%s FileSize = %d, blob has %d bytes", size, v.FileSize, len(e.blobs.Bytes(v.BlobKey)))
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if got := e.blobs.Keys(); !slices.Equal(got, keys) {
		t.Errorf("blobs = %v, want only the variants %v (the received file deleted)", got, keys)
	}
	results, err := e.h.Media.ListProcessingResults(t.Context(), m.File.ID)
	if err != nil || len(results) != 1 {
		t.Errorf("%d results after three runs (%v), want 1", len(results), err)
	}
	large, _ := r.Variant(content.SizeLarge, content.MotionStill)
	if u := e.service.URL(large); u != e.blobs.PublicURL(large.BlobKey) {
		t.Errorf("URL = %q", u)
	}
	// A result recorded concurrently makes the insert a no-op.
	err = e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		has, err := e.h.Media.HasProcessingResult(t.Context(), tx, m.File.ID, content.PurposeAttachment)
		if err == nil && !has {
			t.Error("HasProcessingResult(attachment) = false")
		}
		if other, err := e.h.Media.HasProcessingResult(t.Context(), tx, m.File.ID, content.PurposeAvatar); err != nil || other {
			t.Errorf("HasProcessingResult(avatar) = %v, %v; want false", other, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mediaReprocessesForAnotherPurpose checks that using a file for another purpose processes its
// large variant, the moving one of an animation, again into a new result with keys of its own,
// once, and that the upload's purpose picks the first one.
func mediaReprocessesForAnotherPurpose(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	m, err := e.service.Upload(ctx, appcontent.UploadInput{
		OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixtureGIF(4, 3, 2)), Purpose: content.PurposeHeader,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p := mediaJobPurpose(t, e.jobs.jobs[len(e.jobs.jobs)-1]); p != content.PurposeHeader {
		t.Errorf("the upload's job processes for %q, want header", p)
	}
	e.processFor(t, m.File.ID, content.PurposeHeader)
	first := e.media(t, m.File.ID).Result
	if first.Purpose != content.PurposeHeader || !first.Animated() || len(first.Variants) != 6 {
		t.Fatalf("first result = %+v, want 6 variants of a header", first)
	}
	if _, err := e.service.Upload(ctx, appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixturePNG(4, 3, false)), Purpose: "banner"}); !errors.Is(err, appcontent.ErrInvalidPurpose) {
		t.Errorf("an unknown purpose = %v, want ErrInvalidPurpose", err)
	}

	jobs := len(e.jobs.jobs)
	if err := e.service.CheckProfileImage(ctx, mediaOwner, m.File.ID, content.PurposeHeader); err != nil || len(e.jobs.jobs) != jobs {
		t.Errorf("the same purpose: %v, %d jobs enqueued", err, len(e.jobs.jobs)-jobs)
	}
	if err := e.service.CheckProfileImage(ctx, mediaOwner, m.File.ID, content.PurposeAvatar); err != nil {
		t.Fatal(err)
	}
	if len(e.jobs.jobs) != jobs+1 || mediaJobPurpose(t, e.jobs.jobs[jobs]) != content.PurposeAvatar {
		t.Fatalf("jobs = %+v, want one for the avatar", e.jobs.jobs[jobs:])
	}
	e.clock.Advance(time.Second)
	for range 2 {
		e.processFor(t, m.File.ID, content.PurposeAvatar)
	}
	results, err := e.h.Media.ListProcessingResults(ctx, m.File.ID)
	if err != nil || len(results) != 2 {
		t.Fatalf("%d results (%v), want 2", len(results), err)
	}
	second := results[1]
	if second.Purpose != content.PurposeAvatar || second.Outcome != content.MediaReady || !second.Animated() || len(second.Variants) != 6 {
		t.Fatalf("second result = %+v, want 6 variants of an avatar", second)
	}
	if second.Width != first.Width || second.Height != first.Height {
		t.Errorf("second result is %d×%d, want the original %d×%d", second.Width, second.Height, first.Width, first.Height)
	}
	for _, v := range second.Variants {
		if !strings.HasPrefix(v.BlobKey, "media/"+m.File.ID+"/"+second.ID+"/") || len(e.blobs.Bytes(v.BlobKey)) == 0 {
			t.Errorf("variant %+v is not under the new result", v)
		}
	}
	if got := e.media(t, m.File.ID).Result; got.ID != second.ID {
		t.Errorf("current result = %s, want %s", got.ID, second.ID)
	}
	// Back to the header: a job of its own, not deduplicated with the first one.
	if err := e.service.CheckProfileImage(ctx, mediaOwner, m.File.ID, content.PurposeHeader); err != nil {
		t.Fatal(err)
	}
	if last := e.jobs.jobs[len(e.jobs.jobs)-1]; last.IdempotencyKey == e.jobs.jobs[jobs].IdempotencyKey || mediaJobPurpose(t, last) != content.PurposeHeader {
		t.Errorf("job = %+v, want a new one for the header", last)
	}
}

func mediaJobPurpose(t *testing.T, j queue.NewJob) content.MediaPurpose {
	t.Helper()
	var p struct {
		MediaID string               `json:"media_id"`
		Purpose content.MediaPurpose `json:"purpose"`
	}
	if j.Type != appcontent.JobProcessMedia || json.Unmarshal(j.Payload, &p) != nil || p.MediaID == "" {
		t.Fatalf("job = %+v, want media.process", j)
	}
	return p.Purpose
}

func mediaRecordsFailures(t *testing.T, e *mediaEnv) {
	m := e.upload(t, FixturePNG(4, 3, false))
	// The stored file turned out not to decode.
	if _, err := e.blobs.Put(t.Context(), m.Upload.BlobKey, strings.NewReader("\x89PNG\r\n\x1A\ncorrupt"), blob.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	e.process(t, m.File.ID)
	got := e.media(t, m.File.ID)
	if got.Result == nil || got.Result.Outcome != content.MediaFailed || got.Ready() || len(got.Result.Variants) != 0 {
		t.Fatalf("Result = %+v, want a failure without variants", got.Result)
	}
	if got.Result.Width != 0 || got.Result.Blurhash != "" {
		t.Errorf("a failure has dimensions or a blurhash: %+v", got.Result)
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("blobs after a failure: %v", keys)
	}
	e.process(t, m.File.ID)

	// A received file that is gone is a failure too.
	m2 := e.upload(t, FixturePNG(4, 3, false))
	if err := e.blobs.Delete(t.Context(), m2.Upload.BlobKey); err != nil {
		t.Fatal(err)
	}
	e.process(t, m2.File.ID)
	if got := e.media(t, m2.File.ID); got.Result == nil || got.Result.Outcome != content.MediaFailed {
		t.Errorf("missing file: Result = %+v", got.Result)
	}
	// An unknown media file (cleaned up before its job ran) is done.
	e.process(t, "0192f0c1-0000-7000-8000-00000000dead")
}

func mediaRetriesTransient(t *testing.T, e *mediaEnv) {
	m := e.upload(t, FixturePNG(4, 3, false))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if res := e.service.Process(ctx, m.File.ID, ""); !res.IsRetry() {
		t.Errorf("Process with a cancelled context = %+v, want Retry", res)
	}
	if got := e.media(t, m.File.ID); got.Result != nil {
		t.Errorf("a cancelled run recorded %+v", got.Result)
	}
}

func mediaKeepsFocus(t *testing.T, e *mediaEnv) {
	m := e.upload(t, FixturePNG(4, 3, false))
	for _, f := range []content.MediaFocus{{MediaID: m.File.ID, X: 0.5, Y: -0.25}, {MediaID: m.File.ID, X: -1, Y: 1}} {
		if err := e.service.SetFocus(t.Context(), mediaOwner, f); err != nil {
			t.Fatalf("SetFocus(%+v): %v", f, err)
		}
		if got := e.media(t, m.File.ID).Focus; got == nil || *got != f {
			t.Errorf("Focus = %+v, want %+v", got, f)
		}
	}
	if err := e.service.SetFocus(t.Context(), mediaOwner, content.MediaFocus{MediaID: m.File.ID, X: 1.5}); !errors.Is(err, content.ErrInvalidFocus) {
		t.Errorf("out of range: %v", err)
	}
	if err := e.service.SetFocus(t.Context(), "someone-else", content.MediaFocus{MediaID: m.File.ID}); !errors.Is(err, appcontent.ErrNotOwner) {
		t.Errorf("other owner: %v", err)
	}
	if err := e.service.SetFocus(t.Context(), mediaOwner, content.MediaFocus{MediaID: "0192f0c1-0000-7000-8000-00000000dead"}); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("unknown file: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

func mediaUpdates(t *testing.T, e *mediaEnv) {
	e.clock.Advance(time.Millisecond)
	m, err := e.service.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixturePNG(4, 3, false)), Description: "a cat"})
	if err != nil {
		t.Fatalf("Upload with a description: %v", err)
	}
	if m.Description != "a cat" {
		t.Errorf("Description after upload = %q", m.Description)
	}
	id := m.File.ID
	if got, err := e.service.OwnMedia(t.Context(), mediaOwner, id); err != nil || got.File.ID != id || got.Description != "a cat" {
		t.Errorf("OwnMedia = %+v, %v", got, err)
	}
	if _, err := e.service.OwnMedia(t.Context(), "someone-else", id); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("OwnMedia of another account: %v, want ErrNotFound", err)
	}

	// Only the given fields change.
	got, err := e.service.Update(t.Context(), mediaOwner, appcontent.MediaUpdate{MediaID: id, Focus: &content.MediaFocus{X: 0.5, Y: -0.5}})
	if err != nil || got.Description != "a cat" || got.Focus == nil || *got.Focus != (content.MediaFocus{MediaID: id, X: 0.5, Y: -0.5}) {
		t.Errorf("Update focus = %+v, %v", got, err)
	}
	got, err = e.service.Update(t.Context(), mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: ptr("a black cat")})
	if err != nil || got.Description != "a black cat" || got.Focus == nil || got.Focus.X != 0.5 {
		t.Errorf("Update description = %+v, %v", got, err)
	}
	got, err = e.service.Update(t.Context(), mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: ptr("")})
	if err != nil || got.Description != "" {
		t.Errorf("removing the description = %+v, %v", got, err)
	}
	if _, err := e.h.Media.FindDescription(t.Context(), id); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("a removed description is still stored: %v", err)
	}

	long := strings.Repeat("あ", content.MaxMediaDescriptionLength+1)
	for name, tc := range map[string]struct {
		account string
		u       appcontent.MediaUpdate
		want    error
	}{
		"too long":      {mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: &long}, content.ErrDescriptionTooLong},
		"NUL":           {mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: ptr("a\x00b")}, content.ErrInvalidDescription},
		"invalid UTF-8": {mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: ptr("a\xffb")}, content.ErrInvalidDescription},
		"bad focus":     {mediaOwner, appcontent.MediaUpdate{MediaID: id, Focus: &content.MediaFocus{X: 2}}, content.ErrInvalidFocus},
		"other account": {"someone-else", appcontent.MediaUpdate{MediaID: id, Description: ptr("mine")}, appcontent.ErrNotFound},
		"unknown":       {mediaOwner, appcontent.MediaUpdate{MediaID: "0192f0c1-0000-7000-8000-00000000dead", Description: ptr("x")}, appcontent.ErrNotFound},
	} {
		if _, err := e.service.Update(t.Context(), tc.account, tc.u); !errors.Is(err, tc.want) {
			t.Errorf("%s: Update = %v, want %v", name, err, tc.want)
		}
	}
	if _, err := e.service.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixturePNG(4, 3, false)), Description: long}); !errors.Is(err, content.ErrDescriptionTooLong) {
		t.Errorf("Upload with a long description = %v", err)
	}
	if _, err := e.service.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: mediaOwner, Content: bytes.NewReader(FixturePNG(4, 3, false)), Description: "\x00"}); !errors.Is(err, content.ErrInvalidDescription) {
		t.Errorf("Upload with NUL in the description = %v", err)
	}
	if got := e.media(t, id); got.Description != "" || got.Focus.X != 0.5 {
		t.Errorf("rejected updates changed the file: %+v", got)
	}
	// Exactly the maximum is accepted.
	if _, err := e.service.Update(t.Context(), mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: ptr(long[len("あ"):])}); err != nil {
		t.Errorf("a description of the maximum length: %v", err)
	}

	// Once a status attaches the file, it no longer changes on its own.
	e.attach(t, id)
	if _, err := e.service.Update(t.Context(), mediaOwner, appcontent.MediaUpdate{MediaID: id, Description: ptr("changed")}); !errors.Is(err, appcontent.ErrMediaAttached) {
		t.Errorf("Update of an attached file = %v, want ErrMediaAttached", err)
	}
	err = e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		attached, err := e.h.Media.IsAttached(t.Context(), tx, id)
		if err == nil && !attached {
			t.Error("IsAttached = false for an attached file")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mediaChecksProfileImages checks CheckProfileImage: only the owner's processed uploads are
// usable as an avatar or header.
func mediaChecksProfileImages(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	pending := e.upload(t, FixturePNG(4, 3, false))
	if err := e.service.CheckProfileImage(ctx, mediaOwner, pending.File.ID, content.PurposeAvatar); !errors.Is(err, appaccount.ErrUnusableImage) {
		t.Errorf("unprocessed upload = %v, want ErrUnusableImage", err)
	}
	e.process(t, pending.File.ID)
	if err := e.service.CheckProfileImage(ctx, mediaOwner, pending.File.ID, content.PurposeAvatar); err != nil {
		t.Errorf("processed upload = %v, want usable", err)
	}
	if err := e.service.CheckProfileImage(ctx, "someone-else", pending.File.ID, content.PurposeAvatar); !errors.Is(err, appaccount.ErrUnusableImage) {
		t.Errorf("another account's upload = %v, want ErrUnusableImage", err)
	}
	if err := e.service.CheckProfileImage(ctx, mediaOwner, "0192f0c1-0000-7000-8000-00000000dead", content.PurposeAvatar); !errors.Is(err, appaccount.ErrUnusableImage) {
		t.Errorf("unknown file = %v, want ErrUnusableImage", err)
	}
}

// mediaReadsImagesTogether checks ListReadyVariants and Images: the variants of each listed
// file whose newest processing result is ready, and its image when the file's account owns it.
func mediaReadsImagesTogether(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	ready := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, ready.File.ID)
	other := e.upload(t, FixturePNG(5, 3, false))
	e.process(t, other.File.ID)
	pending := e.upload(t, FixturePNG(6, 3, false))
	reprocessed := e.upload(t, FixturePNG(7, 3, false))
	e.process(t, reprocessed.File.ID)
	// A newer result that failed makes the file's current state a failure.
	e.clock.Advance(time.Millisecond)
	failed := content.MediaProcessingResult{
		ID: e.ids.NewID(), MediaID: reprocessed.File.ID, Purpose: content.PurposeAttachment, Outcome: content.MediaFailed, MediaType: content.MediaTypeUnknown,
		ProcessedAt: e.clock.Now(),
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Media.InsertProcessingResult(ctx, tx, failed) }); err != nil {
		t.Fatal(err)
	}
	unknown := "0192f0c1-0000-7000-8000-00000000dead"

	ids := []string{other.File.ID, ready.File.ID, pending.File.ID, reprocessed.File.ID, unknown}
	variants, err := e.h.Media.ListReadyVariants(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range variants {
		if v.Variant.Motion != content.MotionStill || v.File.OwnerAccountID != mediaOwner || v.Variant.BlobKey == "" {
			t.Errorf("variant = %+v", v)
		}
		got = append(got, v.File.ID+" "+string(v.Variant.Size))
	}
	want := []string{
		ready.File.ID + " large", ready.File.ID + " medium", ready.File.ID + " small",
		other.File.ID + " large", other.File.ID + " medium", other.File.ID + " small",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ListReadyVariants = %v, want %v (ordered by ID, size)", got, want)
	}
	if none, err := e.h.Media.ListReadyVariants(ctx, nil); err != nil || len(none) != 0 {
		t.Errorf("ListReadyVariants(nil) = %v, %v", none, err)
	}

	r := e.media(t, ready.File.ID).Result
	large, _ := r.Variant(content.SizeLarge, content.MotionStill)
	small, _ := r.Variant(content.SizeSmall, content.MotionStill)
	refs := []appcontent.ImageRef{
		{AccountID: mediaOwner, MediaID: ready.File.ID},
		{AccountID: NamedID("someone-else"), MediaID: other.File.ID},
		{AccountID: mediaOwner, MediaID: pending.File.ID},
		{AccountID: mediaOwner, MediaID: reprocessed.File.ID},
		{AccountID: mediaOwner, MediaID: unknown},
		{AccountID: mediaOwner, MediaID: ""},
	}
	images, err := e.service.Images(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	img := images[refs[0]]
	if len(images) != 1 || img.Still.Large.URL != e.service.URL(large) || img.Still.Small.URL != e.service.URL(small) ||
		img.Still.Large.Width != large.Width || img.Still.Small.ContentType != small.ContentType || img.Animated != nil {
		t.Errorf("Images = %+v, want only %s: %s", images, refs[0].MediaID, e.service.URL(large))
	}
}

// attach inserts a local status whose first revision attaches mediaID.
func (e *mediaEnv) attach(t *testing.T, mediaID string) {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	id := e.ids.NewID()
	s := content.Status{
		ID: id, URI: "https://" + contractHost + "/ap/statuses/" + id, AuthorAccountID: mediaOwner, IsLocal: true,
		Visibility: content.VisibilityPublic, PublishedAt: e.clock.Now(),
	}
	rev := content.Revision{
		ID: e.ids.NewID(), StatusID: id, Seq: 1, Language: "en", RevisedAt: s.PublishedAt,
		Bodies:      []content.Body{{Language: "en", ContentHTML: "<p>look</p>"}},
		Attachments: []content.Attachment{{Position: 0, MediaID: mediaID, Description: "a picture"}},
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.h.Statuses.InsertStatus(t.Context(), tx, s); err != nil {
			return err
		}
		if err := e.h.Statuses.InsertRevision(t.Context(), tx, rev); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.StatusCreated{Status: s, Revision: rev})
	})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
}

func mediaCleansUp(t *testing.T, e *mediaEnv) {
	unattached := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, unattached.File.ID)
	if _, err := e.service.Update(t.Context(), mediaOwner, appcontent.MediaUpdate{MediaID: unattached.File.ID, Description: ptr("a cat")}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.SetFocus(t.Context(), mediaOwner, content.MediaFocus{MediaID: unattached.File.ID, X: 0.1}); err != nil {
		t.Fatal(err)
	}
	attached := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, attached.File.ID)
	e.attach(t, attached.File.ID)
	referenced := e.upload(t, FixturePNG(4, 3, false))
	e.referrers[referenced.File.ID] = true
	unprocessed := e.upload(t, FixturePNG(4, 3, false))

	e.clock.Advance(appcontent.DefaultUnattachedTTL - time.Minute)
	recent := e.upload(t, FixturePNG(4, 3, false))
	if n, err := e.service.CleanupUnattached(t.Context()); err != nil || n != 0 {
		t.Fatalf("cleanup before the TTL deleted %d (%v)", n, err)
	}

	e.clock.Advance(2 * time.Minute)
	n, err := e.service.CleanupUnattached(t.Context())
	if err != nil || n != 2 {
		t.Fatalf("CleanupUnattached = %d, %v; want 2 (the processed and the unprocessed unattached uploads)", n, err)
	}
	for _, gone := range []appcontent.Media{unattached, unprocessed} {
		if _, err := e.service.Media(t.Context(), gone.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("deleted upload: Media = %v, want ErrNotFound", err)
		}
		if _, err := e.h.Media.FindFocus(t.Context(), gone.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("deleted upload kept its focus: %v", err)
		}
		if _, err := e.h.Media.FindDescription(t.Context(), gone.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("deleted upload kept its description: %v", err)
		}
		if rs, err := e.h.Media.ListProcessingResults(t.Context(), gone.File.ID); err != nil || len(rs) != 0 {
			t.Errorf("deleted upload kept %d results (%v)", len(rs), err)
		}
		for _, k := range e.blobs.Keys() {
			if strings.Contains(k, gone.File.ID) {
				t.Errorf("deleted upload kept blob %s", k)
			}
		}
	}
	for _, kept := range []appcontent.Media{attached, referenced, recent} {
		if _, err := e.service.Media(t.Context(), kept.File.ID); err != nil {
			t.Errorf("kept upload: Media = %v", err)
		}
	}
	if got := e.media(t, attached.File.ID); !got.Ready() || len(e.blobs.Bytes(mediaLargeKey(got))) == 0 {
		t.Errorf("the attached upload lost its result or blobs")
	}
	if n, err := e.service.CleanupUnattached(t.Context()); err != nil || n != 0 {
		t.Errorf("second cleanup deleted %d (%v)", n, err)
	}
	// Processing a deleted upload, as a late job would, is done and writes nothing.
	e.process(t, unprocessed.File.ID)
	if _, err := e.service.Media(t.Context(), unprocessed.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("late processing brought it back: %v", err)
	}
	// The guarded delete keeps a file that is attached.
	err = e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		deleted, err := e.h.Media.DeleteUnattachedMedia(t.Context(), tx, attached.File.ID)
		if err == nil && deleted {
			t.Error("DeleteUnattachedMedia deleted an attached file")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mediaCleanupPages(t *testing.T, e *mediaEnv) {
	const n = 105
	var last appcontent.Media
	for i := range n {
		last = e.upload(t, FixturePNG(4, 3, false))
		if i < n-1 {
			e.referrers[last.File.ID] = true
		}
	}
	e.clock.Advance(appcontent.DefaultUnattachedTTL + time.Minute)
	deleted, err := e.service.CleanupUnattached(t.Context())
	if err != nil || deleted != 1 {
		t.Fatalf("CleanupUnattached = %d, %v; want 1", deleted, err)
	}
	if _, err := e.service.Media(t.Context(), last.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("the upload after %d referenced ones was kept: %v", n-1, err)
	}
}

// failingDeletes is a blob store whose deletions fail while fail is set.
type failingDeletes struct {
	blob.Store
	fail bool
}

var errBlobDelete = errors.New("testkit: blob deletion failed")

func (f *failingDeletes) Delete(ctx context.Context, key string) error {
	if f.fail {
		return errBlobDelete
	}
	return f.Store.Delete(ctx, key)
}

// eraseMedia runs the media eraser on the owner's uploads in batches of limit to the end of a
// run, and reports whether a batch kept something.
func (e *mediaEnv) eraseMedia(t *testing.T, er *appcontent.MediaEraser, owner string, limit int) bool {
	t.Helper()
	kept, cursor := false, ""
	for range 100 {
		step, err := er.EraseAccountData(t.Context(), appaccount.ErasureTarget{AccountID: owner}, cursor, limit)
		if err != nil {
			t.Fatalf("EraseAccountData(%q) = %v", cursor, err)
		}
		kept = kept || step.Kept
		if step.Done {
			return kept
		}
		cursor = step.Next
	}
	t.Fatal("the media eraser did not reach the end")
	return kept
}

func mediaErasesDeletedAccounts(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	plain := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, plain.File.ID)
	if _, err := e.service.Update(ctx, mediaOwner, appcontent.MediaUpdate{MediaID: plain.File.ID, Description: ptr("a cat")}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.SetFocus(ctx, mediaOwner, content.MediaFocus{MediaID: plain.File.ID, X: 0.1}); err != nil {
		t.Fatal(err)
	}
	unprocessed := e.upload(t, FixturePNG(4, 3, false))
	attached := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, attached.File.ID)
	e.attach(t, attached.File.ID)
	referenced := e.upload(t, FixturePNG(4, 3, false))
	e.process(t, referenced.File.ID)
	e.referrers[referenced.File.ID] = true
	other, err := e.service.Upload(ctx, appcontent.UploadInput{
		OwnerAccountID: "0192f0c1-0000-7000-8000-0000000000b0", Content: bytes.NewReader(FixturePNG(4, 3, false)),
	})
	if err != nil {
		t.Fatal(err)
	}
	blobsOf := func(id string) int {
		n := 0
		for _, k := range e.blobs.Keys() {
			if strings.Contains(k, id) {
				n++
			}
		}
		return n
	}

	store := &failingDeletes{Store: e.blobs, fail: true}
	er := appcontent.NewMediaEraser(appcontent.MediaErasureDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Media, Blobs: store, Referrers: []appcontent.MediaReferrer{e.referrers},
	})
	// A failed blob deletion keeps the rows, which the next run deletes again.
	if _, err := er.EraseAccountData(ctx, appaccount.ErasureTarget{AccountID: mediaOwner}, "", 10); !errors.Is(err, errBlobDelete) {
		t.Fatalf("EraseAccountData with failing deletions = %v, want errBlobDelete", err)
	}
	if _, err := e.service.Media(ctx, plain.File.ID); err != nil {
		t.Fatalf("a failed blob deletion lost the rows: %v", err)
	}
	store.fail = false
	// Batches of one file exercise the continuation.
	if kept := e.eraseMedia(t, er, mediaOwner, 1); !kept {
		t.Error("the run did not report the attached upload as kept")
	}
	for _, gone := range []appcontent.Media{plain, unprocessed} {
		if _, err := e.service.Media(ctx, gone.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("erased upload: Media = %v, want ErrNotFound", err)
		}
		if _, err := e.h.Media.FindFocus(ctx, gone.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("erased upload kept its focus: %v", err)
		}
		if _, err := e.h.Media.FindDescription(ctx, gone.File.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("erased upload kept its description: %v", err)
		}
		if rs, err := e.h.Media.ListProcessingResults(ctx, gone.File.ID); err != nil || len(rs) != 0 {
			t.Errorf("erased upload kept %d results (%v)", len(rs), err)
		}
		if n := blobsOf(gone.File.ID); n != 0 {
			t.Errorf("erased upload kept %d blobs", n)
		}
	}
	for _, kept := range []appcontent.Media{attached, referenced, other} {
		if _, err := e.service.Media(ctx, kept.File.ID); err != nil {
			t.Errorf("kept upload: Media = %v", err)
		}
	}
	for _, kept := range []appcontent.Media{attached, referenced} {
		if n := blobsOf(kept.File.ID); n != 3 {
			t.Errorf("kept upload has %d blobs, want its 3 variants", n)
		}
	}
	// A second run keeps the same files.
	if kept := e.eraseMedia(t, er, mediaOwner, 10); !kept {
		t.Error("the second run did not report the attached upload as kept")
	}
	if _, err := e.service.Media(ctx, attached.File.ID); err != nil {
		t.Errorf("the second run erased the attached upload: %v", err)
	}
}

// mediaLargeKey returns the blob key of the large still variant of a processed file.
func mediaLargeKey(m appcontent.Media) string {
	if m.Result == nil {
		return ""
	}
	v, _ := m.Result.Variant(content.SizeLarge, content.MotionStill)
	return v.BlobKey
}

// mediaCountsStoredBytes checks MediaService.StoredBytes: the local uploads count with the
// variants of their current result once processed, and with the file as received before.
func mediaCountsStoredBytes(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	local, remote, err := e.service.StoredBytes(ctx)
	if err != nil || local != 0 || remote != 0 {
		t.Fatalf("StoredBytes of an empty database = %d, %d, %v", local, remote, err)
	}
	processed := e.upload(t, FixturePNG(40, 30, false))
	pending := e.upload(t, FixtureGIF(4, 3, 2))
	e.process(t, processed.File.ID)
	want := pending.Upload.FileSize
	result := e.media(t, processed.File.ID).Result
	if result == nil || len(result.Variants) == 0 {
		t.Fatalf("Result = %+v, want variants", result)
	}
	for _, v := range result.Variants {
		want += v.FileSize
	}
	if local, _, err = e.service.StoredBytes(ctx); err != nil || local != want {
		t.Errorf("StoredBytes local = %d, %v, want %d", local, err, want)
	}

	// The variants of an earlier result no longer count.
	e.clock.Advance(time.Second)
	e.processFor(t, processed.File.ID, content.PurposeAvatar)
	want = pending.Upload.FileSize
	for _, v := range e.media(t, processed.File.ID).Result.Variants {
		want += v.FileSize
	}
	if local, _, err = e.service.StoredBytes(ctx); err != nil || local != want {
		t.Errorf("after processing again: StoredBytes local = %d, %v, want %d", local, err, want)
	}
}
