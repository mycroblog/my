// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/media/native"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
)

func TestPluginTestMedia(t *testing.T) {
	h, rec := fakeplugin.Connect(fakeplugin.Options{Media: native.New(native.Options{})})

	// Plain TCP works, as h2c, with a warning; the stream needs HTTP/2. Each kind runs its own cases.
	for _, tc := range []struct {
		kind string
		want []string
	}{
		{"media_image", []string{"PASS process_photo: ", "PASS process_emoji\n", "PASS process_animation\n", "4 passed, 0 failed, 0 skipped\n"}},
		{"media_audio", []string{"PASS remux_audio\n", "2 passed, 0 failed, 0 skipped\n"}},
		{"media_video", []string{"PASS remux_video: ", "2 passed, 0 failed, 0 skipped\n"}},
	} {
		code, stdout, stderr := runPluginTest("--kind", tc.kind, fakeplugin.ServeTCP(t, h))
		if code != 0 {
			t.Fatalf("%s: code = %d\n%s%s", tc.kind, code, stdout, stderr)
		}
		for _, want := range tc.want {
			if !strings.Contains(stdout, want) {
				t.Errorf("%s: stdout lacks %q:\n%s", tc.kind, want, stdout)
			}
		}
		if !strings.Contains(stderr, "warning: address is plain TCP") {
			t.Errorf("%s: stderr = %q", tc.kind, stderr)
		}
	}
	if rec.Proto() != "HTTP/2.0" {
		t.Errorf("the calls went over %s", rec.Proto())
	}

	// The stream cannot be JSON.
	if code, _, stderr := runPluginTest("--kind", "media_image", "--protocol", "json", fakeplugin.ServeUnix(t, h)); code != 1 || !strings.Contains(stderr, "must be proto or grpc") {
		t.Errorf("--protocol json: code = %d, %s", code, stderr)
	}
}
