// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	domain "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// An upload goes through the database queue's outbox to the worker, which strips its EXIF
// (with the GPS position) before the instance serves it under /media/.
func TestUploadedMediaIsServedWithoutLocation(t *testing.T) {
	t.Parallel()
	for _, queueName := range []string{"db", "inline"} {
		t.Run(queueName, func(t *testing.T) {
			cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": queueName})
			s := start(t, cfg, platform.ServeOptions{}, nil)
			original := testkit.FixtureJPEG(640, 480, 6)
			m, err := s.app.Media.Upload(t.Context(), content.UploadInput{
				OwnerAccountID: "0192f0c1-0000-7000-8000-00000000a11c", Content: bytes.NewReader(original), DeclaredContentType: "image/jpeg",
			})
			if err != nil {
				t.Fatalf("Upload: %v", err)
			}
			testkit.Eventually(t, 10*time.Second, func() bool {
				if m.Ready() {
					return true
				}
				if m, err = s.app.Media.Media(t.Context(), m.File.ID); err != nil {
					t.Fatal(err)
				}
				return m.Ready()
			})
			if !m.Ready() {
				t.Fatalf("not processed: %+v", m.Result)
			}
			if m.Result.Width != 480 || m.Result.Height != 640 {
				t.Errorf("size %d×%d, want the orientation applied (480×640)", m.Result.Width, m.Result.Height)
			}
			large, _ := m.Result.Variant(domain.SizeLarge, domain.MotionStill)
			if got, want := s.app.Media.URL(large), "https://example.test/media/"+m.File.ID+"/"+m.Result.ID+"/large.jpg"; got != want {
				t.Errorf("URL = %q, want %q", got, want)
			}
			if len(m.Result.Variants) != 3 {
				t.Errorf("variants = %+v, want 3 still ones", m.Result.Variants)
			}
			for _, v := range m.Result.Variants {
				resp, body := s.do(t, http.MethodGet, "/"+v.BlobKey, nil)
				if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != v.ContentType ||
					resp.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
					t.Fatalf("GET %s = %d %s", v.BlobKey, resp.StatusCode, resp.Header.Get("Content-Type"))
				}
				for k, want := range map[string]string{
					"X-Content-Type-Options":       "nosniff",
					"Cross-Origin-Resource-Policy": "cross-origin",
					"Content-Security-Policy":      "default-src 'none'; sandbox",
				} {
					if got := resp.Header.Get(k); got != want {
						t.Errorf("%s: %s = %q, want %q", v.BlobKey, k, got, want)
					}
				}
				if strings.Contains(body, testkit.MetadataMarker) || strings.Contains(body, "Exif\x00") {
					t.Errorf("%s still has the EXIF and GPS data", v.BlobKey)
				}
			}
			// The job records the result before it deletes the upload as received, so the
			// deletion may still be under way when the media reads as ready.
			deleted := testkit.Eventually(t, 10*time.Second, func() bool {
				_, err := s.app.Blobs.Head(t.Context(), m.Upload.BlobKey)
				return err != nil
			})
			if !deleted {
				t.Errorf("the upload as received, with its location, is kept at %s", m.Upload.BlobKey)
			}
			if resp, _ := s.do(t, http.MethodGet, "/"+m.Upload.BlobKey, nil); resp.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s = %d, want 404", m.Upload.BlobKey, resp.StatusCode)
			}
		})
	}
}
