// SPDX-License-Identifier: AGPL-3.0-only

package media_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/interface/media"
	"github.com/mycroblog/my/pkg/testkit"
)

type mux struct{ *http.ServeMux }

func (m mux) Handle(pattern string, h http.Handler) { m.ServeMux.Handle(pattern, h) }

func newServer(t *testing.T) (*testkit.MemoryBlobStore, http.Handler) {
	t.Helper()
	blobs := testkit.NewMemoryBlobStore(testkit.NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)))
	m := mux{http.NewServeMux()}
	media.New(media.Options{Blobs: blobs}).Register(m)
	return blobs, m
}

func put(t *testing.T, s blob.Store, key, content, ct string) {
	t.Helper()
	if _, err := s.Put(t.Context(), key, strings.NewReader(content), blob.PutOptions{ContentType: ct}); err != nil {
		t.Fatal(err)
	}
}

func TestServesPublicMediaWithSafeHeaders(t *testing.T) {
	blobs, h := newServer(t)
	put(t, blobs, "media/0192f0c1-0000-7000-8000-000000000001/full.jpg", "\xFF\xD8\xFFjpeg", "image/jpeg")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/0192f0c1-0000-7000-8000-000000000001/full.jpg", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "\xFF\xD8\xFFjpeg" {
		t.Fatalf("GET = %d %q", rec.Code, rec.Body.String())
	}
	for k, want := range map[string]string{
		"Content-Type":                 "image/jpeg",
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      "default-src 'none'; sandbox",
		"Cross-Origin-Resource-Policy": "cross-origin",
		"Content-Disposition":          `inline; filename="full.jpg"`,
		"Cache-Control":                "public, max-age=31536000, immutable",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/0192f0c1-0000-7000-8000-000000000001/full.jpg", nil)
	req.Header.Set("Range", "bytes=3-")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "jpeg" {
		t.Errorf("range GET = %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/media/0192f0c1-0000-7000-8000-000000000001/full.jpg", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("HEAD = %d, %d bytes, %q", rec.Code, rec.Body.Len(), rec.Header().Get("Content-Type"))
	}
}

// The video and audio variants that processing remuxes are served with their type, with range
// requests for seeking.
func TestServesVideoAndAudioVariants(t *testing.T) {
	blobs, h := newServer(t)
	for _, tc := range []struct{ name, ct string }{
		{"large-animated.mp4", "video/mp4"},
		{"large-animated.webm", "video/webm"},
		{"large-animated.m4a", "audio/mp4"},
		{"audio-large-animated.webm", "audio/webm"},
		{"large-animated.ogg", "audio/ogg"},
		{"large-animated.mp3", "audio/mpeg"},
	} {
		p := "/media/0192f0c1-0000-7000-8000-000000000001/0192f0c1-0000-7000-8000-000000000002/" + tc.name
		put(t, blobs, strings.TrimPrefix(p, "/"), "0123456789", tc.ct)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != tc.ct || rec.Body.String() != "0123456789" {
			t.Errorf("GET %s = %d %q", tc.name, rec.Code, rec.Header().Get("Content-Type"))
		}
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("Range", "bytes=4-")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent || rec.Body.String() != "456789" {
			t.Errorf("range GET %s = %d %q", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestServesOnlyPublicImageBlobs(t *testing.T) {
	blobs, h := newServer(t)
	put(t, blobs, "uploads/0192f0c1-0000-7000-8000-000000000001", "\xFF\xD8\xFFwith exif", "image/jpeg")
	put(t, blobs, "media/x/page.html", "<script>alert(1)</script>", "text/html")
	put(t, blobs, "media/x/image.svg", "<svg/>", "image/svg+xml")
	for _, p := range []string{
		"/media/x/page.html",
		"/media/x/image.svg",
		"/media/missing.jpg",
		"/media/../uploads/0192f0c1-0000-7000-8000-000000000001",
		"/media/%2e%2e/uploads/0192f0c1-0000-7000-8000-000000000001",
		"/media/x//y.jpg",
		"/media/",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s = 200 %q", p, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/0192f0c1-0000-7000-8000-000000000001", nil))
	if rec.Code == http.StatusOK {
		t.Errorf("an upload as received is served")
	}
}
