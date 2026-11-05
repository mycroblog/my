// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/translator/fake"
)

func TestPluginTestTranslator(t *testing.T) {
	h, rec := fakeplugin.JSON(fakeplugin.Options{Translator: fake.New(fake.Options{Detected: "en", Unsupported: []string{"qaa"}})})
	addr := fakeplugin.ServeUnix(t, h)

	code, stdout, stderr := runPluginTest("--kind", "translator", addr)
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"PASS translate: ", "PASS translate_markers_only\n", "PASS translate_undetermined\n",
		"PASS translate_unsupported_language\n", "5 passed, 0 failed, 0 skipped\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}

	// The flags reach the cases: a language the service refuses fails them.
	code, stdout, _ = runPluginTest("--kind", "translator", "--translate-to", "qaa", addr)
	if code != 1 || !strings.Contains(stdout, "FAIL translate: ") {
		t.Errorf("--translate-to qaa: code = %d\n%s", code, stdout)
	}

	// The rule of the server holds here too: plain TCP is refused, on loopback too.
	before := rec.Calls()
	tcp := fakeplugin.ServeTCP(t, h)
	if code, _, stderr := runPluginTest("--kind", "translator", tcp); code != 1 || !strings.Contains(stderr, "must not be http://") {
		t.Errorf("plain TCP: code = %d, %s", code, stderr)
	}
	if n := rec.Calls() - before; n != 0 {
		t.Errorf("%d calls over plain TCP", n)
	}
}
