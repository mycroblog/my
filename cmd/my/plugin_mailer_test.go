// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
)

func TestPluginTestMailer(t *testing.T) {
	box := new(fakeplugin.Mailbox)
	h, _ := fakeplugin.JSON(fakeplugin.Options{Mailbox: box})
	addr := fakeplugin.ServeUnix(t, h)

	code, stdout, stderr := runPluginTest("--kind", "mailer", addr)
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{"SKIP send: ", "SKIP send_no_recipient: ", "1 passed, 0 failed, 2 skipped\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if n := len(box.Requests()); n != 0 {
		t.Errorf("%d requests without --mail-to", n)
	}

	code, stdout, stderr = runPluginTest("--kind", "mailer", "--mail-to", "me@example.com", "--mail-from", "noreply@social.example", addr)
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{"PASS send: ", "Auto-Submitted", "PASS send_no_recipient\n", "3 passed, 0 failed, 0 skipped\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if got := box.Delivered(); len(got) != 1 || got[0].GetTo() != "me@example.com" || got[0].GetEnvelopeFrom() != "noreply@social.example" {
		t.Errorf("delivered = %v", got)
	}

	// The rule of the server holds here too: plain TCP is refused, on loopback too.
	before := len(box.Requests())
	tcp := fakeplugin.ServeTCP(t, h)
	if code, _, stderr := runPluginTest("--kind", "mailer", "--mail-to", "me@example.com", tcp); code != 1 || !strings.Contains(stderr, "must not be http://") {
		t.Errorf("plain TCP: code = %d, %s", code, stderr)
	}
	if n := len(box.Requests()); n != before {
		t.Errorf("%d requests over plain TCP", n-before)
	}
}
