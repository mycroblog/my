// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/media/native"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	domain "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// refusingMedia is a plugin's processor that fails every Process with err.
type refusingMedia struct {
	*native.Processor

	err error
}

func (r refusingMedia) Process(context.Context, []byte, media.Limits, media.Sizes, media.Request) (media.Output, error) {
	return media.Output{}, r.err
}

// With media_image = plugin, the plugin makes the variants of an upload, and what it does marks the job as the built-in
// processor does: an output is a ready file, a rejection a failed one, and a failure of the plugin leaves the file to the
// job's next try (ADR-0146 §13, §15).
func TestMediaOfAPlugin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		behind media.Processor
		broken bool
		want   domain.MediaOutcome // "" for no result yet
	}{
		{"ready", native.New(native.Options{}), false, domain.MediaReady},
		{"rejected", refusingMedia{Processor: native.New(native.Options{}), err: media.ErrCorrupt}, false, domain.MediaFailed},
		{"failed", native.New(native.Options{}), true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, rec := fakeplugin.Connect(fakeplugin.Options{Media: tc.behind, Broken: tc.broken})
			cfg := loadConfig(t, map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "db",
				"MY_MEDIA_IMAGE": "plugin", "MY_PLUGIN_MEDIA_IMAGE_ADDRESS": fakeplugin.ServeUnix(t, h),
			})
			s := start(t, cfg, platform.ServeOptions{}, nil)
			m, err := s.app.Media.Upload(t.Context(), content.UploadInput{
				OwnerAccountID: "0192f0c1-0000-7000-8000-00000000a11c", Content: bytes.NewReader(testkit.FixtureJPEG(640, 480, 6)), DeclaredContentType: "image/jpeg",
			})
			if err != nil {
				t.Fatalf("Upload: %v", err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for (m.Result == nil && tc.want != "" || tc.want == "" && rec.Calls() == 0) && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
				if m, err = s.app.Media.Media(t.Context(), m.File.ID); err != nil {
					t.Fatal(err)
				}
			}
			if rec.Calls() == 0 {
				t.Fatal("no call reached the plugin")
			}
			if tc.want == "" {
				if m.Result != nil {
					t.Fatalf("a failure of the plugin recorded %s", m.Result.Outcome)
				}
				if _, err := s.app.Blobs.Head(t.Context(), m.Upload.BlobKey); err != nil {
					t.Errorf("the upload as received is gone before a retry: %v", err)
				}
				return
			}
			if m.Result == nil || m.Result.Outcome != tc.want {
				t.Fatalf("result = %+v, want %s", m.Result, tc.want)
			}
			if tc.want == domain.MediaReady && (m.Result.Width != 480 || m.Result.Height != 640 || len(m.Result.Variants) != 3) {
				t.Errorf("result %d×%d with %d variants", m.Result.Width, m.Result.Height, len(m.Result.Variants))
			}
		})
	}
}

// A plugin that is down at start does not stop the server, and the processing waits for it.
func TestMediaOfAPluginAtStart(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_MEDIA_IMAGE": "plugin", "MY_PLUGIN_MEDIA_IMAGE_ADDRESS": "unix:///nowhere/media.sock",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatalf("a plugin that is down stopped the start: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.MediaProcessor.Process(t.Context(), testkit.FixtureJPEG(64, 48, 1), media.Limits{}, media.Sizes{}, media.Request{}); err == nil || media.Rejected(err) {
		t.Errorf("Process with the plugin down = %v, want a temporary failure", err)
	}
}
