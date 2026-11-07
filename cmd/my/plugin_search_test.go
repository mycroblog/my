// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/search/fake"
)

func TestPluginTestSearch(t *testing.T) {
	h, rec := fakeplugin.Connect(fakeplugin.Options{Search: fake.New()})
	addr := fakeplugin.ServeUnix(t, h)

	code, stdout, stderr := runPluginTest("--kind", "search", addr)
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"PASS search_no_word\n", "PASS search_plain_text\n", "PASS search_japanese\n", "PASS search_invalid_cursor\n",
		"PASS replace_and_search\n", "PASS replace_replaces\n", "PASS remove\n", "PASS remove_by_author\n",
		"SKIP remove_by_visibility: ", "SKIP clear: ", "9 passed, 0 failed, 2 skipped\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}

	// --clear-index runs the cases that remove every document.
	code, stdout, _ = runPluginTest("--kind", "search", "--clear-index", "--protocol", "proto", addr)
	if code != 0 || !strings.Contains(stdout, "PASS remove_by_visibility\n") || !strings.Contains(stdout, "PASS clear\n") ||
		!strings.Contains(stdout, "11 passed, 0 failed, 0 skipped\n") {
		t.Errorf("--clear-index: code = %d\n%s", code, stdout)
	}

	// A plugin that answers nothing fails.
	broken, _ := fakeplugin.Connect(fakeplugin.Options{Search: fake.New(), Broken: true})
	if code, stdout, _ := runPluginTest("--kind", "search", fakeplugin.ServeUnix(t, broken)); code != 1 || !strings.Contains(stdout, "FAIL search_no_word: ") {
		t.Errorf("a broken plugin: code = %d\n%s", code, stdout)
	}

	// The rule of the server holds here too: plain TCP is refused, on loopback too.
	before := rec.Calls()
	tcp := fakeplugin.ServeTCP(t, h)
	if code, _, stderr := runPluginTest("--kind", "search", tcp); code != 1 || !strings.Contains(stderr, "must not be http://") {
		t.Errorf("plain TCP: code = %d, %s", code, stderr)
	}
	if n := rec.Calls() - before; n != 0 {
		t.Errorf("%d calls over plain TCP", n)
	}
}
