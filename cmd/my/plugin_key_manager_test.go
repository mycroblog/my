// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestPluginTestKeyManager(t *testing.T) {
	h, rec := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager("hsm/transit-1")})
	addr := fakeplugin.ServeUnix(t, h)

	code, stdout, stderr := runPluginTest("--kind", "key_manager", addr)
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"PASS describe\n", "PASS self_test\n", "PASS unwrap_changed_value\n", "PASS unwrap_unknown_key\n", "5 passed, 0 failed, 0 skipped\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}

	// A plugin that answers nothing fails the cases.
	broken, _ := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager("hsm/transit-1"), Broken: true})
	code, stdout, _ = runPluginTest("--kind", "key_manager", fakeplugin.ServeUnix(t, broken))
	if code != 1 || !strings.Contains(stdout, "FAIL describe: ") {
		t.Errorf("broken: code = %d\n%s", code, stdout)
	}

	// The rule of the server holds here too: plain TCP is refused, on loopback too.
	before := rec.Calls()
	tcp := fakeplugin.ServeTCP(t, h)
	if code, _, stderr := runPluginTest("--kind", "key_manager", tcp); code != 1 || !strings.Contains(stderr, "must not be http://") {
		t.Errorf("plain TCP: code = %d, %s", code, stderr)
	}
	if n := rec.Calls() - before; n != 0 {
		t.Errorf("%d calls over plain TCP", n)
	}
}
