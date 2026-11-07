// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// Video and audio files for the FakeMediaProcessor: an MP4 and an MP3 by their leading bytes,
// with "location" for the metadata that a remux drops.
var (
	fakeMP4 = []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isom location video")
	fakeMP3 = []byte("ID3\x04\x00\x00\x00\x00\x00\x00 location audio")
)

func (e *mediaEnv) uploadAV(t *testing.T, in appcontent.UploadInput) appcontent.Media {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	in.OwnerAccountID = mediaOwner
	m, err := e.service.Upload(t.Context(), in)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return m
}

// mediaProcessesVideoAndAudio checks video and audio uploads (ADR-0005 §9): the poster kept
// beside the file as received, the remuxed file as the large animated variant with its media
// type, size, and duration, the poster (or the cover in the file) as the still variants, the
// received file and poster deleted, and the image that clients load.
func mediaProcessesVideoAndAudio(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	poster := testkit.FixturePNG(4, 3, false)
	video := e.uploadAV(t, appcontent.UploadInput{Content: bytes.NewReader(fakeMP4), DeclaredContentType: "video/quicktime", Poster: bytes.NewReader(poster)})
	if video.Upload.ContentType != "video/mp4" {
		t.Errorf("Upload.ContentType = %q, want video/mp4", video.Upload.ContentType)
	}
	posterKey := "uploads/" + video.File.ID + ".poster"
	if !bytes.Equal(e.blobs.Bytes(posterKey), poster) {
		t.Fatal("the poster is not kept beside the file as received")
	}
	e.clock.Advance(time.Second)
	e.process(t, video.File.ID)
	e.process(t, video.File.ID)
	got := e.media(t, video.File.ID)
	if !got.Ready() {
		t.Fatalf("after processing: %+v", got.Result)
	}
	r := got.Result
	if r.MediaType != content.MediaTypeVideo || r.Width != 640 || r.Height != 360 || r.DurationMS != 3000 || r.Blurhash == "" {
		t.Errorf("result = %+v", r)
	}
	file, ok := r.Variant(content.SizeLarge, content.MotionAnimated)
	if !ok || file.ContentType != "video/mp4" || !strings.HasSuffix(file.BlobKey, "/large-animated.mp4") || file.Width != 640 {
		t.Errorf("the video variant = %+v", file)
	}
	if data := e.blobs.Bytes(file.BlobKey); data == nil || bytes.Contains(data, []byte("location")) {
		t.Errorf("the stored video is %q, want it remuxed", data)
	}
	for _, size := range content.MediaSizes {
		if v, ok := r.Variant(size, content.MotionStill); !ok || !strings.HasPrefix(v.ContentType, "image/") {
			t.Errorf("the %s poster variant = %+v", size, v)
		}
	}
	for _, key := range []string{video.Upload.BlobKey, posterKey} {
		if e.blobs.Bytes(key) != nil {
			t.Errorf("%s is kept after processing", key)
		}
	}
	img, ok := e.service.Image(got)
	if !ok || img.Media == nil || img.Primary().URL != e.service.URL(file) || !img.HasStill() || img.Animated != nil {
		t.Errorf("Image = %+v, %v", img, ok)
	}
	if err := e.service.CheckProfileImage(ctx, mediaOwner, video.File.ID, content.PurposeAvatar); !errors.Is(err, appaccount.ErrUnusableImage) {
		t.Errorf("CheckProfileImage of a video = %v, want ErrUnusableImage", err)
	}
	if err := e.service.CheckImage(ctx, video.File.ID); !errors.Is(err, appcontent.ErrNotAnImage) {
		t.Errorf("CheckImage of a video = %v, want ErrNotAnImage", err)
	}

	// An audio file takes the cover in it as its poster.
	cover := testkit.FixturePNG(3, 3, false)
	withCover := e.uploadAV(t, appcontent.UploadInput{Content: bytes.NewReader(append(bytes.Clone(fakeMP3), append([]byte("cover:"), cover...)...))})
	e.process(t, withCover.File.ID)
	got = e.media(t, withCover.File.ID)
	if !got.Ready() || got.Result.MediaType != content.MediaTypeAudio || got.Result.DurationMS != 3000 || got.Result.Width != 0 {
		t.Errorf("audio result = %+v", got.Result)
	}
	if _, ok := got.Result.Variant(content.SizeSmall, content.MotionStill); !ok {
		t.Error("the cover of the audio file is not its poster")
	}

	// Without a poster, an audio file is still shown.
	plain := e.uploadAV(t, appcontent.UploadInput{Content: bytes.NewReader(fakeMP3), DeclaredContentType: "audio/mpeg"})
	e.process(t, plain.File.ID)
	got = e.media(t, plain.File.ID)
	img, ok = e.service.Image(got)
	if !got.Ready() || len(got.Result.Variants) != 1 || !ok || img.Media == nil || img.HasStill() || img.Primary().ContentType != "audio/mpeg" {
		t.Errorf("audio without a poster: %+v, image %+v", got.Result, img)
	}
	images, err := e.service.Images(ctx, []appcontent.ImageRef{{AccountID: mediaOwner, MediaID: plain.File.ID}})
	if err != nil || images[appcontent.ImageRef{AccountID: mediaOwner, MediaID: plain.File.ID}].Media == nil {
		t.Errorf("Images of the audio file = %+v, %v", images, err)
	}
}

// mediaRejectsVideoAndAudio checks the uploads of video and audio files that are refused: by
// their codec, length, size, purpose, declared type, or poster.
func mediaRejectsVideoAndAudio(t *testing.T, e *mediaEnv) {
	e.service = appcontent.NewMediaService(appcontent.MediaDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Media, Blobs: e.blobs, Processor: testkit.FakeMediaProcessor{},
		Jobs: e.jobs, Clock: e.clock, IDs: e.ids, Limits: media.Limits{MaxBytes: 1 << 10},
		AVLimits: media.AVLimits{MaxBytes: 1 << 12},
	})
	if n := e.service.MaxUploadBytes(); n != 1<<12 {
		t.Errorf("MaxUploadBytes = %d, want the video and audio limit", n)
	}
	for name, tc := range map[string]struct {
		in   appcontent.UploadInput
		want error
	}{
		"hevc":              {appcontent.UploadInput{Content: bytes.NewReader(append(bytes.Clone(fakeMP4), "hevc"...))}, media.ErrUnsupportedCodec},
		"too long":          {appcontent.UploadInput{Content: bytes.NewReader(append(bytes.Clone(fakeMP4), "too-long"...))}, media.ErrTooLong},
		"too large":         {appcontent.UploadInput{Content: bytes.NewReader(append(bytes.Clone(fakeMP4), make([]byte, 1<<12)...))}, media.ErrTooLarge},
		"as an avatar":      {appcontent.UploadInput{Content: bytes.NewReader(fakeMP4), Purpose: content.PurposeAvatar}, media.ErrUnsupportedFormat},
		"declared as image": {appcontent.UploadInput{Content: bytes.NewReader(fakeMP4), DeclaredContentType: "image/png"}, appcontent.ErrContentTypeMismatch},
		"mp3 as mp4":        {appcontent.UploadInput{Content: bytes.NewReader(fakeMP3), DeclaredContentType: "video/mp4"}, appcontent.ErrContentTypeMismatch},
		"svg poster":        {appcontent.UploadInput{Content: bytes.NewReader(fakeMP4), Poster: strings.NewReader("<svg/>")}, appcontent.ErrInvalidPoster},
		"large poster":      {appcontent.UploadInput{Content: bytes.NewReader(fakeMP4), Poster: bytes.NewReader(append(testkit.FixturePNG(4, 3, false), make([]byte, 1<<10)...))}, media.ErrTooLarge},
		"image over limit":  {appcontent.UploadInput{Content: bytes.NewReader(append(testkit.FixturePNG(4, 3, false), make([]byte, 1<<10)...))}, media.ErrTooLarge},
	} {
		tc.in.OwnerAccountID = mediaOwner
		if _, err := e.service.Upload(t.Context(), tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: Upload = %v, want %v", name, err, tc.want)
		}
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("rejected uploads left blobs: %v", keys)
	}
	if len(e.jobs.jobs) != 0 {
		t.Errorf("rejected uploads enqueued %d jobs", len(e.jobs.jobs))
	}
}
