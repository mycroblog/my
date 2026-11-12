// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// testMaxBytes and testMaxAVBytes are the upload limits of fakeMedia for images and for video
// and audio files. bobAccountID is in notifications_test.go.
const (
	testMaxBytes   = 4096
	testMaxAVBytes = 8192
)

// fakeMedia is a MediaFiles that keeps media files in memory. Upload rejects content as the
// real service does: by its sniffed format, its declared type, and its size; content
// containing "corrupt" or "huge" is corrupt or too large in dimensions.
type fakeMedia struct {
	mu       sync.Mutex
	files    map[string]appcontent.Media
	attached map[string]bool
	next     int
	// imageCalls counts the calls of Images.
	imageCalls int
	// purposes are the purposes of the uploads.
	purposes map[string]content.MediaPurpose
	// posters are the posters of the uploads.
	posters map[string][]byte
}

func newFakeMedia() *fakeMedia {
	return &fakeMedia{files: map[string]appcontent.Media{}, attached: map[string]bool{}, purposes: map[string]content.MediaPurpose{}, posters: map[string][]byte{}}
}

func (f *fakeMedia) Upload(_ context.Context, in appcontent.UploadInput) (appcontent.Media, error) {
	data, err := io.ReadAll(in.Content)
	if err != nil {
		return appcontent.Media{}, err
	}
	if err := content.ValidateMediaDescription(in.Description); err != nil {
		return appcontent.Media{}, err
	}
	var poster []byte
	if in.Poster != nil {
		if poster, err = io.ReadAll(in.Poster); err != nil {
			return appcontent.Media{}, err
		}
	}
	if av := media.SniffAV(data); av != "" {
		return f.uploadAV(in, data, av, poster)
	}
	format := media.Sniff(data)
	switch {
	case int64(len(data)) > testMaxBytes:
		return appcontent.Media{}, media.ErrTooLarge
	case format == "":
		return appcontent.Media{}, media.ErrUnsupportedFormat
	case in.DeclaredContentType != "" && in.DeclaredContentType != "application/octet-stream" && in.DeclaredContentType != format.ContentType():
		return appcontent.Media{}, appcontent.ErrContentTypeMismatch
	case bytes.Contains(data, []byte("corrupt")):
		return appcontent.Media{}, media.ErrCorrupt
	case bytes.Contains(data, []byte("huge")):
		return appcontent.Media{}, media.ErrDimensions
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := fmt.Sprintf("01890a5d-ac96-774b-bcce-%012d", f.next)
	m := appcontent.Media{
		File:        content.MediaFile{ID: id, OwnerAccountID: in.OwnerAccountID, Origin: content.MediaOriginUpload, CreatedAt: createdAt},
		Upload:      &content.UploadedMedia{MediaID: id, BlobKey: "uploads/" + id, ContentType: format.ContentType(), FileSize: int64(len(data))},
		Description: in.Description,
	}
	f.files[id] = m
	f.purposes[id] = in.Purpose
	return m, nil
}

// uploadAV accepts a video or audio upload as the real service does: an MP4 is a video and
// anything else audio; content containing "hevc" has an unsupported codec, and "too-long" is
// too long.
func (f *fakeMedia) uploadAV(in appcontent.UploadInput, data []byte, format media.Format, poster []byte) (appcontent.Media, error) {
	switch {
	case int64(len(data)) > testMaxAVBytes:
		return appcontent.Media{}, media.ErrTooLarge
	case in.Purpose != "" && in.Purpose != content.PurposeAttachment:
		return appcontent.Media{}, media.ErrUnsupportedFormat
	case in.DeclaredContentType != "" && !format.Declares(in.DeclaredContentType):
		return appcontent.Media{}, appcontent.ErrContentTypeMismatch
	case bytes.Contains(data, []byte("hevc")):
		return appcontent.Media{}, media.ErrUnsupportedCodec
	case bytes.Contains(data, []byte("too-long")):
		return appcontent.Media{}, media.ErrTooLong
	}
	contentType := "audio/mpeg"
	if format == media.FormatMP4 {
		contentType = "video/mp4"
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := fmt.Sprintf("01890a5d-ac96-774b-bcce-%012d", f.next)
	m := appcontent.Media{
		File:        content.MediaFile{ID: id, OwnerAccountID: in.OwnerAccountID, Origin: content.MediaOriginUpload, CreatedAt: createdAt},
		Upload:      &content.UploadedMedia{MediaID: id, BlobKey: "uploads/" + id, ContentType: contentType, FileSize: int64(len(data))},
		Description: in.Description,
	}
	f.files[id] = m
	f.purposes[id] = in.Purpose
	f.posters[id] = poster
	return m, nil
}

// processAV records a ready result for a video or audio upload, as the processing job would:
// the file, and the still variants of its poster when it has one.
func (f *fakeMedia) processAV(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := f.files[id]
	r := content.MediaProcessingResult{ID: testkit.NamedID("result-" + id), MediaID: id, Outcome: content.MediaReady, Purpose: content.PurposeAttachment, DurationMS: 12500}
	file := content.MediaVariant{ResultID: r.ID, Size: content.SizeLarge, Motion: content.MotionAnimated, ContentType: m.Upload.ContentType, FileSize: 5000}
	if m.Upload.ContentType == "video/mp4" {
		r.MediaType, r.Width, r.Height = content.MediaTypeVideo, 640, 360
		file.BlobKey, file.Width, file.Height = "media/"+id+"/r/large-animated.mp4", 640, 360
	} else {
		r.MediaType, file.BlobKey = content.MediaTypeAudio, "media/"+id+"/r/large-animated.mp3"
	}
	r.Variants = []content.MediaVariant{file}
	if f.posters[id] != nil {
		r.Blurhash = "LEHV6nWB2yk8pyo0adR*.7kCMdnj"
		r.Variants = append(r.Variants,
			content.MediaVariant{ResultID: r.ID, Size: content.SizeLarge, Motion: content.MotionStill, BlobKey: "media/" + id + "/r/large.png", ContentType: "image/png", FileSize: 1234, Width: 640, Height: 480},
			content.MediaVariant{ResultID: r.ID, Size: content.SizeMedium, Motion: content.MotionStill, BlobKey: "media/" + id + "/r/medium.webp", ContentType: "image/webp", FileSize: 456, Width: 640, Height: 480},
			content.MediaVariant{ResultID: r.ID, Size: content.SizeSmall, Motion: content.MotionStill, BlobKey: "media/" + id + "/r/small.webp", ContentType: "image/webp", FileSize: 99, Width: 415, Height: 311},
		)
	}
	m.Result = &r
	f.files[id] = m
}

// process records a result for a media file, as the processing job would.
func (f *fakeMedia) process(id string, outcome content.MediaOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := f.files[id]
	r := content.MediaProcessingResult{ID: testkit.NamedID("result-" + id), MediaID: id, Outcome: outcome, MediaType: content.MediaTypeUnknown}
	if outcome == content.MediaReady {
		r.MediaType, r.Width, r.Height, r.Blurhash = content.MediaTypeImage, 640, 480, "LEHV6nWB2yk8pyo0adR*.7kCMdnj"
		r.Purpose = content.PurposeAttachment
		r.Variants = []content.MediaVariant{
			{ResultID: r.ID, Size: content.SizeLarge, Motion: content.MotionStill, BlobKey: "media/" + id + "/r/large.png", ContentType: "image/png", FileSize: 1234, Width: 640, Height: 480},
			{ResultID: r.ID, Size: content.SizeMedium, Motion: content.MotionStill, BlobKey: "media/" + id + "/r/medium.webp", ContentType: "image/webp", FileSize: 456, Width: 640, Height: 480},
			{ResultID: r.ID, Size: content.SizeSmall, Motion: content.MotionStill, BlobKey: "media/" + id + "/r/small.webp", ContentType: "image/webp", FileSize: 99, Width: 415, Height: 311},
		}
	}
	m.Result = &r
	f.files[id] = m
}

func (f *fakeMedia) OwnMedia(_ context.Context, accountID, id string) (appcontent.Media, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.files[id]
	if !ok || m.File.OwnerAccountID != accountID {
		return appcontent.Media{}, appcontent.ErrNotFound
	}
	return m, nil
}

func (f *fakeMedia) Update(ctx context.Context, accountID string, u appcontent.MediaUpdate) (appcontent.Media, error) {
	if u.Description != nil {
		if err := content.ValidateMediaDescription(*u.Description); err != nil {
			return appcontent.Media{}, err
		}
	}
	if u.Focus != nil {
		if err := u.Focus.Validate(); err != nil {
			return appcontent.Media{}, err
		}
	}
	m, err := f.OwnMedia(ctx, accountID, u.MediaID)
	if err != nil {
		return appcontent.Media{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attached[u.MediaID] {
		return appcontent.Media{}, appcontent.ErrMediaAttached
	}
	if u.Description != nil {
		m.Description = *u.Description
	}
	if u.Focus != nil {
		m.Focus = &content.MediaFocus{MediaID: u.MediaID, X: u.Focus.X, Y: u.Focus.Y}
	}
	f.files[u.MediaID] = m
	return m, nil
}

// processedVariants returns the variants of a file that fakeMedia.process made ready.
func processedVariants(id string) clientapi.MediaVariants {
	variant := func(name, contentType string, w, h int, size int64) clientapi.MediaVariant {
		return clientapi.MediaVariant{Url: "https://social.example/media/" + id + "/r/" + name, ContentType: &contentType, Width: &w, Height: &h, Size: &size}
	}
	return clientapi.MediaVariants{
		Small:  variant("small.webp", "image/webp", 415, 311, 99),
		Medium: variant("medium.webp", "image/webp", 640, 480, 456),
		Large:  variant("large.png", "image/png", 640, 480, 1234),
	}
}

func (f *fakeMedia) url(v content.MediaVariant) string { return "https://social.example/" + v.BlobKey }

// CustomEmojis knows no custom emoji: the platform tests present them through the real media
// service.
func (f *fakeMedia) CustomEmojis(context.Context, []string) (map[string]appcontent.CustomEmojiView, error) {
	return map[string]appcontent.CustomEmojiView{}, nil
}

// Image presents the variants of a processed file as the real service does.
func (f *fakeMedia) Image(m appcontent.Media) (appcontent.Image, bool) {
	if !m.Ready() {
		return appcontent.Image{}, false
	}
	var file *appcontent.ImageVariant
	if v, ok := m.Result.Variant(content.SizeLarge, content.MotionAnimated); ok && m.Result.MediaType != content.MediaTypeImage {
		file = &appcontent.ImageVariant{URL: f.url(v), ContentType: v.ContentType, Width: v.Width, Height: v.Height, Size: v.FileSize}
	}
	set := func(motion content.MediaMotion) (appcontent.ImageVariants, bool) {
		var out appcontent.ImageVariants
		for i, size := range content.MediaSizes {
			v, ok := m.Result.Variant(size, motion)
			if !ok {
				return out, false
			}
			iv := appcontent.ImageVariant{URL: f.url(v), ContentType: v.ContentType, Width: v.Width, Height: v.Height, Size: v.FileSize}
			*[]*appcontent.ImageVariant{&out.Small, &out.Medium, &out.Large}[i] = iv
		}
		return out, true
	}
	still, ok := set(content.MotionStill)
	if file != nil {
		return appcontent.Image{Still: still, Media: file}, true
	}
	if !ok {
		return appcontent.Image{}, false
	}
	img := appcontent.Image{Still: still}
	if moving, ok := set(content.MotionAnimated); ok {
		img.Animated = &moving
	}
	return img, true
}

// Images answers as the real service does and counts its calls in imageCalls.
func (f *fakeMedia) Images(ctx context.Context, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error) {
	f.mu.Lock()
	f.imageCalls++
	f.mu.Unlock()
	out := map[appcontent.ImageRef]appcontent.Image{}
	for _, ref := range refs {
		m, err := f.OwnMedia(ctx, ref.AccountID, ref.MediaID)
		if err != nil {
			continue
		}
		if img, ok := f.Image(m); ok {
			out[ref] = img
		}
	}
	return out, nil
}

func (f *fakeMedia) Limits() media.Limits { return media.Limits{MaxBytes: testMaxBytes}.WithDefaults() }

func (f *fakeMedia) MaxUploadBytes() int64 { return testMaxAVBytes }

type mediaEnv struct {
	*authEnv
	media *fakeMedia
}

func newMediaEnv(t *testing.T) *mediaEnv {
	t.Helper()
	m := newFakeMedia()
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Media = m })
	e.auth.tokens["bob-write"] = bearer("write", bobAccountID)
	e.auth.tokens["write-media"] = bearer("write:media", bobAccountID)
	return &mediaEnv{authEnv: e, media: m}
}

// part is one part of a multipart upload.
type part struct {
	name, filename, contentType string
	data                        []byte
}

func multipartBody(t *testing.T, parts ...part) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		disposition := `form-data; name="` + p.name + `"`
		if p.filename != "" {
			disposition += `; filename="` + p.filename + `"`
		}
		h.Set("Content-Disposition", disposition)
		if p.contentType != "" {
			h.Set("Content-Type", p.contentType)
		}
		pw, err := w.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pw.Write(p.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func (e *mediaEnv) send(t *testing.T, method, path, token, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, clientapi.BasePath+path, body)
	req.RemoteAddr = "192.0.2.1:1234"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept-Language", "en")
	return e.spec.Serve(t, e.h, req)
}

func (e *mediaEnv) upload(t *testing.T, token string, parts ...part) *http.Response {
	t.Helper()
	body, contentType := multipartBody(t, parts...)
	return e.send(t, http.MethodPost, "/media", token, contentType, body)
}

func wantProblem(t *testing.T, res *http.Response, status int, typ string) clientapi.Problem {
	t.Helper()
	p := decode[clientapi.Problem](t, res)
	if res.StatusCode != status || p.Type != typ {
		t.Errorf("got %d %s (%s), want %d %s", res.StatusCode, p.Type, p.Title, status, typ)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("problem Cache-Control = %q", cc)
	}
	return p
}

func png() part {
	return part{name: "file", filename: "cat.png", contentType: "image/png", data: testkit.FixturePNG(4, 3, false)}
}

func TestUploadMedia(t *testing.T) {
	e := newMediaEnv(t)
	res := e.upload(t, "bob-write", png(), part{name: "description", data: []byte("a black cat")}, part{name: "ignored", data: []byte("x")})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("upload = %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	got := decode[clientapi.MediaAttachment](t, res)
	if got.State != "processing" || got.Type != "image" || got.Description == nil || *got.Description != "a black cat" ||
		got.Variants != nil || got.Blurhash != nil || got.Meta.Width != nil || got.Meta.Focus != (clientapi.MediaFocus{}) {
		t.Errorf("upload = %+v", got)
	}
	m, err := e.media.OwnMedia(t.Context(), bobAccountID, got.Id)
	if err != nil {
		t.Fatalf("the upload is not bob's: %v", err)
	}
	if m.Upload.FileSize != int64(len(png().data)) {
		t.Errorf("stored %d bytes, want %d", m.Upload.FileSize, len(png().data))
	}

	if p := e.media.purposes[got.Id]; p != "" {
		t.Errorf("purpose = %q, want none (the service's default)", p)
	}
	avatar := decode[clientapi.MediaAttachment](t, e.upload(t, "bob-write", png(), part{name: "purpose", data: []byte("avatar")}))
	if p := e.media.purposes[avatar.Id]; p != content.PurposeAvatar {
		t.Errorf("purpose = %q, want avatar", p)
	}

	// The fine-grained scope is enough; a declared octet-stream tells nothing.
	file := png()
	file.contentType = "application/octet-stream"
	if res := e.upload(t, "write-media", file); res.StatusCode != http.StatusAccepted {
		t.Errorf("upload with write:media = %d", res.StatusCode)
	}
}

func TestUploadMediaRejects(t *testing.T) {
	e := newMediaEnv(t)
	pngData := testkit.FixturePNG(4, 3, false)
	tests := []struct {
		name       string
		parts      []part
		wantStatus int
		wantType   string
	}{
		{"svg", []part{{name: "file", filename: "x.svg", contentType: "image/svg+xml", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)}},
			http.StatusUnsupportedMediaType, clientapi.ProblemTypeUnsupportedMediaFormat},
		{"declared type differs", []part{{name: "file", filename: "x.jpg", contentType: "image/jpeg", data: pngData}},
			http.StatusUnsupportedMediaType, clientapi.ProblemTypeUnsupportedMediaFormat},
		{"corrupt", []part{{name: "file", contentType: "image/png", data: append(bytes.Clone(pngData), "corrupt"...)}},
			http.StatusUnprocessableEntity, clientapi.ProblemTypeMediaCorrupt},
		{"dimensions", []part{{name: "file", contentType: "image/png", data: append(bytes.Clone(pngData), "huge"...)}},
			http.StatusUnprocessableEntity, clientapi.ProblemTypeMediaDimensions},
		{"file part over the limit", []part{{name: "file", contentType: "image/png", data: append(bytes.Clone(pngData), make([]byte, testMaxBytes)...)}},
			http.StatusRequestEntityTooLarge, clientapi.ProblemTypeMediaTooLarge},
		{"no file", []part{{name: "description", data: []byte("nothing")}}, http.StatusBadRequest, clientapi.ProblemTypeBlank},
		{"two files", []part{png(), png()}, http.StatusBadRequest, clientapi.ProblemTypeBlank},
		{"description too long", []part{png(), {name: "description", data: []byte(strings.Repeat("a", content.MaxMediaDescriptionLength+1))}},
			http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank},
		{"description part too long", []part{png(), {name: "description", data: make([]byte, 4*content.MaxMediaDescriptionLength+1)}},
			http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank},
		{"unknown purpose", []part{png(), {name: "purpose", data: []byte("banner")}}, http.StatusBadRequest, clientapi.ProblemTypeBlank},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantProblem(t, e.upload(t, "bob-write", tc.parts...), tc.wantStatus, tc.wantType)
		})
	}
	if len(e.media.files) != 0 {
		t.Errorf("rejected uploads stored %d files", len(e.media.files))
	}

	t.Run("not multipart", func(t *testing.T) {
		res := e.send(t, http.MethodPost, "/media", "bob-write", "application/json", strings.NewReader(`{}`))
		wantProblem(t, res, http.StatusBadRequest, clientapi.ProblemTypeBlank)
	})

	t.Run("localized", func(t *testing.T) {
		// Alice's locale is ja.
		res := e.upload(t, "write", part{name: "file", contentType: "image/svg+xml", data: []byte("<svg/>")})
		p := wantProblem(t, res, http.StatusUnsupportedMediaType, clientapi.ProblemTypeUnsupportedMediaFormat)
		if res.Header.Get("Content-Language") != "ja" || !strings.Contains(p.Title, "対応していません") {
			t.Errorf("problem in %s: %q", res.Header.Get("Content-Language"), p.Title)
		}
	})
}

func mp4() part {
	return part{name: "file", filename: "clip.mov", contentType: "video/quicktime", data: []byte("\x00\x00\x00\x18ftypqt  \x00\x00\x02\x00qt  video")}
}

func TestUploadVideoAndAudio(t *testing.T) {
	e := newMediaEnv(t)
	poster := testkit.FixturePNG(4, 3, false)
	res := e.upload(t, "bob-write", mp4(), part{name: "thumbnail", filename: "poster.png", data: poster}, part{name: "description", data: []byte("a cat jumps")})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("upload = %d", res.StatusCode)
	}
	got := decode[clientapi.MediaAttachment](t, res)
	if got.State != "processing" || got.Type != "video" || got.Url != nil || got.Meta.Duration != nil {
		t.Errorf("upload = %+v", got)
	}
	if !bytes.Equal(e.media.posters[got.Id], poster) {
		t.Error("the thumbnail part is not the poster")
	}
	e.media.processAV(got.Id)
	ready := decode[clientapi.MediaAttachment](t, e.send(t, http.MethodGet, "/media/"+got.Id, "bob-write", "", nil))
	if ready.State != "ready" || ready.Type != "video" || ready.Url == nil || *ready.Url != "https://social.example/media/"+got.Id+"/r/large-animated.mp4" ||
		ready.Meta.Duration == nil || *ready.Meta.Duration != 12.5 || ready.Meta.Width == nil || *ready.Meta.Width != 640 {
		t.Errorf("ready video = %+v", ready)
	}
	if want := processedVariants(got.Id); ready.Variants == nil || !reflect.DeepEqual(*ready.Variants, want) ||
		ready.PreviewUrl == nil || *ready.PreviewUrl != want.Small.Url || ready.Blurhash == nil {
		t.Errorf("the poster of the video is %+v (preview %v)", ready.Variants, ready.PreviewUrl)
	}

	// An audio file without a poster has no variants and no preview.
	audio := decode[clientapi.MediaAttachment](t, e.upload(t, "bob-write", part{name: "file", contentType: "audio/mpeg", data: []byte("ID3\x04\x00\x00song")}))
	if audio.Type != "audio" {
		t.Errorf("audio upload = %+v", audio)
	}
	e.media.processAV(audio.Id)
	ready = decode[clientapi.MediaAttachment](t, e.send(t, http.MethodGet, "/media/"+audio.Id, "bob-write", "", nil))
	if ready.Type != "audio" || ready.Url == nil || ready.Variants != nil || ready.PreviewUrl != nil || ready.Meta.Width != nil {
		t.Errorf("ready audio = %+v", ready)
	}

	for _, tc := range []struct {
		name       string
		parts      []part
		wantStatus int
		wantType   string
	}{
		{"hevc", []part{{name: "file", contentType: "video/mp4", data: append(mp4().data, "hevc"...)}},
			http.StatusUnsupportedMediaType, clientapi.ProblemTypeUnsupportedMediaFormat},
		{"too long", []part{{name: "file", contentType: "video/mp4", data: append(mp4().data, "too-long"...)}},
			http.StatusUnprocessableEntity, clientapi.ProblemTypeMediaTooLong},
		{"as an avatar", []part{mp4(), {name: "purpose", data: []byte("avatar")}}, http.StatusUnsupportedMediaType, clientapi.ProblemTypeUnsupportedMediaFormat},
		{"declared as an image", []part{{name: "file", contentType: "image/png", data: mp4().data}},
			http.StatusUnsupportedMediaType, clientapi.ProblemTypeUnsupportedMediaFormat},
		{"over the video limit", []part{{name: "file", contentType: "video/mp4", data: append(mp4().data, make([]byte, testMaxAVBytes)...)}},
			http.StatusRequestEntityTooLarge, clientapi.ProblemTypeMediaTooLarge},
		{"thumbnail over the image limit", []part{mp4(), {name: "thumbnail", data: make([]byte, testMaxBytes+1)}},
			http.StatusRequestEntityTooLarge, clientapi.ProblemTypeMediaTooLarge},
		{"two thumbnails", []part{mp4(), {name: "thumbnail", data: poster}, {name: "thumbnail", data: poster}},
			http.StatusBadRequest, clientapi.ProblemTypeBlank},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantProblem(t, e.upload(t, "bob-write", tc.parts...), tc.wantStatus, tc.wantType)
		})
	}
	// An image larger than the image limit is too large, even under the video limit.
	big := png()
	big.data = append(big.data, make([]byte, testMaxBytes)...)
	wantProblem(t, e.upload(t, "bob-write", big), http.StatusRequestEntityTooLarge, clientapi.ProblemTypeMediaTooLarge)
}

// unreadBody fails the test when the handler reads the body.
type unreadBody struct{ t *testing.T }

func (b unreadBody) Read([]byte) (int, error) {
	b.t.Error("the body of an oversized request was read")
	return 0, io.EOF
}

// TestUploadMediaBodyLimit checks that a body over the limit is refused before it is read
// when it declares its length, and cut off at the limit when it does not.
func TestUploadMediaBodyLimit(t *testing.T) {
	e := newMediaEnv(t)
	limit := int64(testMaxAVBytes + testMaxBytes + 64<<10)

	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/media", unreadBody{t})
	req.ContentLength = limit + 1
	req.Header.Set("Content-Length", strconv.FormatInt(req.ContentLength, 10))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("Authorization", "Bearer bob-write")
	wantProblem(t, e.spec.Serve(t, e.h, req), http.StatusRequestEntityTooLarge, clientapi.ProblemTypeMediaTooLarge)

	// Without a length the body is cut off at the limit, also inside a part that is skipped.
	body, contentType := multipartBody(t, part{name: "padding", data: make([]byte, limit)}, png())
	req = httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/media", io.MultiReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer bob-write")
	wantProblem(t, e.spec.Serve(t, e.h, req), http.StatusRequestEntityTooLarge, clientapi.ProblemTypeMediaTooLarge)

	// Other operations take JSON bodies of up to 1 MiB.
	res := e.send(t, http.MethodPatch, "/media/01890a5d-ac96-774b-bcce-000000000001", "bob-write", "application/json",
		strings.NewReader(`{"description": "`+strings.Repeat("a", 1<<20)+`"}`))
	wantProblem(t, res, http.StatusRequestEntityTooLarge, clientapi.ProblemTypeBlank)
}

func TestUploadMediaRateLimit(t *testing.T) {
	e := newMediaEnv(t)
	for i := range clientapi.UploadLimit.Max {
		if res := e.upload(t, "bob-write", png()); res.StatusCode != http.StatusAccepted {
			t.Fatalf("upload %d = %d", i+1, res.StatusCode)
		}
	}
	res := e.upload(t, "bob-write", png())
	wantProblem(t, res, http.StatusTooManyRequests, clientapi.ProblemTypeBlank)
	if res.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	// The limit is per account, and reading is not limited by it.
	if res := e.upload(t, "write", png()); res.StatusCode != http.StatusAccepted {
		t.Errorf("another account's upload = %d", res.StatusCode)
	}
	e.clock.Advance(2 * clientapi.UploadLimit.Window)
	if res := e.upload(t, "bob-write", png()); res.StatusCode != http.StatusAccepted {
		t.Errorf("upload after the window = %d", res.StatusCode)
	}
}

func TestGetMedia(t *testing.T) {
	e := newMediaEnv(t)
	up := decode[clientapi.MediaAttachment](t, e.upload(t, "bob-write", png()))
	path := "/media/" + up.Id

	res := e.send(t, http.MethodGet, path, "bob-write", "", nil)
	if got := decode[clientapi.MediaAttachment](t, res); res.StatusCode != http.StatusOK || got.State != "processing" || got.Description != nil {
		t.Errorf("processing = %d %+v", res.StatusCode, got)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	e.media.process(up.Id, content.MediaReady)
	got := decode[clientapi.MediaAttachment](t, e.send(t, http.MethodGet, path, "bob-write", "", nil))
	want := processedVariants(up.Id)
	if got.State != "ready" || got.Type != "image" || got.Variants == nil || !reflect.DeepEqual(*got.Variants, want) ||
		deref(got.Url) != want.Large.Url || deref(got.PreviewUrl) != want.Small.Url ||
		got.Blurhash == nil || *got.Blurhash == "" || got.Meta.Width == nil || *got.Meta.Width != 640 || *got.Meta.Height != 480 {
		t.Errorf("ready = %+v (variants %+v)", got, got.Variants)
	}

	failed := decode[clientapi.MediaAttachment](t, e.upload(t, "bob-write", png()))
	e.media.process(failed.Id, content.MediaFailed)
	got = decode[clientapi.MediaAttachment](t, e.send(t, http.MethodGet, "/media/"+failed.Id, "bob-write", "", nil))
	if got.State != "failed" || got.Type != "unknown" || got.Variants != nil || got.Meta.Width != nil {
		t.Errorf("failed = %+v", got)
	}

	// Only the uploader sees the file.
	wantProblem(t, e.send(t, http.MethodGet, path, "write", "", nil), http.StatusNotFound, clientapi.ProblemTypeBlank)
	wantProblem(t, e.send(t, http.MethodGet, "/media/01890a5d-ac96-774b-bcce-0000000000ff", "bob-write", "", nil), http.StatusNotFound, clientapi.ProblemTypeBlank)
	wantProblem(t, e.send(t, http.MethodGet, path, "bob", "", nil), http.StatusForbidden, clientapi.ProblemTypeInsufficientScope)
}

func TestUpdateMedia(t *testing.T) {
	e := newMediaEnv(t)
	up := decode[clientapi.MediaAttachment](t, e.upload(t, "bob-write", png(), part{name: "description", data: []byte("a cat")}))
	path := "/media/" + up.Id
	patch := func(token, body string) *http.Response {
		t.Helper()
		return e.send(t, http.MethodPatch, path, token, "application/json", strings.NewReader(body))
	}

	res := patch("bob-write", `{"focus": {"x": 0.5, "y": -0.25}}`)
	got := decode[clientapi.MediaAttachment](t, res)
	if res.StatusCode != http.StatusOK || got.Description == nil || *got.Description != "a cat" || got.Meta.Focus != (clientapi.MediaFocus{X: 0.5, Y: -0.25}) {
		t.Errorf("focus = %d %+v", res.StatusCode, got)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	got = decode[clientapi.MediaAttachment](t, patch("bob-write", `{"description": "a black cat"}`))
	if got.Description == nil || *got.Description != "a black cat" || got.Meta.Focus.X != 0.5 {
		t.Errorf("description = %+v", got)
	}
	got = decode[clientapi.MediaAttachment](t, patch("bob-write", `{"description": ""}`))
	if got.Description != nil {
		t.Errorf("removed description = %q", *got.Description)
	}

	wantProblem(t, patch("bob-write", `{"focus": {"x": 1.5, "y": 0}}`), http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank)
	p := wantProblem(t, patch("bob-write", `{"description": "`+strings.Repeat("あ", content.MaxMediaDescriptionLength+1)+`"}`),
		http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank)
	if p.Detail == nil || !strings.Contains(*p.Detail, "1,500") {
		t.Errorf("detail = %v", p.Detail)
	}
	// Text that a dialect cannot store as it is, sent as JSON or as a form part.
	for _, res := range []*http.Response{
		patch("bob-write", `{"description": "a\u0000cat"}`),
		e.upload(t, "bob-write", png(), part{name: "description", data: []byte("a\xffcat")}),
	} {
		p := wantProblem(t, res, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank)
		if p.Detail == nil || !strings.Contains(*p.Detail, "description") {
			t.Errorf("invalid description: detail = %v, want one naming description", p.Detail)
		}
	}
	wantProblem(t, patch("bob-write", `not json`), http.StatusBadRequest, clientapi.ProblemTypeBlank)
	wantProblem(t, patch("write", `{"description": "mine now"}`), http.StatusNotFound, clientapi.ProblemTypeBlank)

	e.media.attached[up.Id] = true
	p = wantProblem(t, patch("bob-write", `{"description": "too late"}`), http.StatusConflict, clientapi.ProblemTypeMediaAttached)
	if p.Detail == nil {
		t.Error("the media-attached problem has no detail")
	}
}
