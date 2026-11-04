// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"testing/fstest"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/web"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestInstancePreviewImage checks end to end that profiles and posts without images get the
// built-in preview image, then the image of the preview_image setting with its localized alt
// text as soon as it is set, without a restart (ADR-0028).
func TestInstancePreviewImage(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	build := fstest.MapFS{
		".vite/manifest.json":  {Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true}}`)},
		"assets/index-AAAA.js": {Data: []byte("export {}")},
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), WebBuild: build})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	reg, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
		Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice", Locale: "ja",
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser: %v", err)
	}
	post, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: reg.AccountID, Visibility: domcontent.VisibilityPublic, TextInput: appcontent.TextInput{Text: "こんにちは", Language: "ja"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	h := app.Handler()
	image := func(path string) (url, alt, width, height string) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.Host+path, nil)
		req.Header.Set("Accept", acceptBrowser)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		meta := map[string]string{}
		for _, m := range regexp.MustCompile(`<meta property="(og:image[^"]*)" content="([^"]*)">`).FindAllStringSubmatch(rec.Body.String(), -1) {
			meta[m[1]] = html.UnescapeString(m[2])
		}
		return meta["og:image"], meta["og:image:alt"], meta["og:image:width"], meta["og:image:height"]
	}
	profile, status := "/@alice", "/@alice/status/"+post.Status.ID
	builtIn := "https://" + cfg.Host + web.PathDefaultImage
	for path, wantAlt := range map[string]string{
		profile: "A speech bubble with three dots on a blue gradient",
		status:  "青いグラデーションの上に、点が 3 つ並んだ吹き出し",
	} {
		if url, alt, w, ht := image(path); url != builtIn || alt != wantAlt || w != "1200" || ht != "630" {
			t.Errorf("GET %s before the setting: image %q %q %sx%s, want the built-in one", path, url, alt, w, ht)
		}
	}

	m, err := app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: reg.AccountID, Content: bytes.NewReader(testkit.FixturePNG(1200, 630, false))})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// The setting accepts an upload that is still being processed; previews keep the built-in
	// image until it is.
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingPreviewImage, Value: m.File.ID}); err != nil {
		t.Fatalf("SetSetting(preview_image): %v", err)
	}
	if url, _, _, _ := image(profile); url != builtIn {
		t.Errorf("image before processing = %q, want the built-in one", url)
	}
	if r := app.Media.Process(ctx, m.File.ID, ""); r.Err() != nil {
		t.Fatalf("Process: %v", r.Err())
	}
	if _, err := app.Instance.SetText(ctx, appinstance.SetTextInput{Key: instance.TextPreviewImageAlt, Body: "マスコット"}); err != nil {
		t.Fatalf("SetText(preview_image_alt): %v", err)
	}
	processed, err := app.Media.Media(ctx, m.File.ID)
	if err != nil || !processed.Ready() {
		t.Fatalf("Media = %+v, %v", processed.Result, err)
	}
	// An opaque image's large variant is a JPEG (ADR-0005 §5).
	want := "https://" + cfg.Host + "/media/" + m.File.ID + "/" + processed.Result.ID + "/large.jpg"
	for path, wantAlt := range map[string]string{profile: "マスコット", status: "マスコット"} {
		if url, alt, w, ht := image(path); url != want || alt != wantAlt || w != "1200" || ht != "630" {
			t.Errorf("GET %s after the setting: image %q %q %sx%s, want %q %q", path, url, alt, w, ht, want, wantAlt)
		}
	}
	// The setting keeps its media file from the cleanup of unattached uploads.
	if referenced, err := app.Instance.IsMediaReferenced(ctx, m.File.ID); err != nil || !referenced {
		t.Errorf("IsMediaReferenced = %v, %v; want true", referenced, err)
	}

	// Clearing the setting goes back to the built-in image.
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingPreviewImage, Value: ""}); err != nil {
		t.Fatalf("SetSetting(preview_image, \"\"): %v", err)
	}
	if url, _, _, _ := image(status); url != builtIn {
		t.Errorf("image after clearing = %q, want the built-in one", url)
	}

	// Only media files that are images are accepted.
	for _, id := range []string{"0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f", "not-a-media-id"} {
		_, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingPreviewImage, Value: id})
		if !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("SetSetting(preview_image, %s) = %v, want ErrInvalidValue", id, err)
		}
	}
}
