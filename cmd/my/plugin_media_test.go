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

	// Plain TCP works, as h2c, with a warning; the stream needs HTTP/2.
	code, stdout, stderr := runPluginTest("--kind", "media", fakeplugin.ServeTCP(t, h))
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"PASS process_photo: ", "PASS process_emoji\n", "PASS process_animation\n", "PASS remux_audio\n", "5 passed, 0 failed, 0 skipped\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "warning: address is plain TCP") {
		t.Errorf("stderr = %q", stderr)
	}
	if rec.Proto() != "HTTP/2.0" {
		t.Errorf("the calls went over %s", rec.Proto())
	}

	// The stream cannot be JSON.
	if code, _, stderr := runPluginTest("--kind", "media", "--protocol", "json", fakeplugin.ServeUnix(t, h)); code != 1 || !strings.Contains(stderr, "must be proto or grpc") {
		t.Errorf("--protocol json: code = %d, %s", code, stderr)
	}
}
