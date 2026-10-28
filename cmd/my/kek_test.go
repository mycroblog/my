// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/keymanager/local"
	"github.com/mycroblog/my/internal/platform/config"
)

func kekCmd(args []string, out, errOut *bytes.Buffer) int { return runKEK(args, out, errOut) }

func TestKEKRotation(t *testing.T) {
	newAdminEnv(t) // its kek is 32 bytes of 7
	oldKEK, newKEK, otherKEK := bytes.Repeat([]byte{7}, 32), bytes.Repeat([]byte{8}, 32), bytes.Repeat([]byte{9}, 32)
	b64 := base64.StdEncoding.EncodeToString
	// The actor keys of the instance actor and the administrator are sealed under the old KEK.
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "root@example.com"); code != 0 {
		t.Fatalf("admin create = %d %q", code, stderr)
	}
	column := "local_account_private_keys.private_key_enc"
	oldID, newID := local.KeyID(oldKEK), local.KeyID(newKEK)

	code, stdout, stderr := runCmd(t, kekCmd, "status")
	if code != 0 || !strings.Contains(stdout, oldID+" (current)") || !strings.Contains(stdout, "every secret is wrapped with the current KEK") {
		t.Fatalf("status before the rotation = %d %q %q", code, stdout, stderr)
	}

	// The rotation: the new KEK is current, and a wrong key is in kek_previous.
	t.Setenv(config.EnvPrefix+"KEK", b64(newKEK))
	t.Setenv(config.EnvPrefix+"KEK_PREVIOUS", b64(otherKEK))
	code, stdout, stderr = runCmd(t, kekCmd, "status")
	if code != 0 || !strings.Contains(stdout, column) || !strings.Contains(stdout, oldID) || !strings.Contains(stdout, "run my kek rewrap") {
		t.Fatalf("status during the rotation = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, kekCmd, "rewrap")
	if code != 1 || !strings.Contains(stdout, column+": 0 rewrapped, 0 already current, 2 failed") ||
		!strings.Contains(stderr, "wrapped with the KEK "+oldID+", which is neither kek nor in kek_previous") ||
		!strings.Contains(stderr, "keep it in kek_previous") {
		t.Fatalf("rewrap without the old KEK = %d %q %q", code, stdout, stderr)
	}

	t.Setenv(config.EnvPrefix+"KEK_PREVIOUS", b64(oldKEK))
	code, stdout, stderr = runCmd(t, kekCmd, "rewrap")
	if code != 0 || !strings.Contains(stdout, column+": 2 rewrapped, 0 already current") || !strings.Contains(stdout, "every secret is wrapped with the current KEK ("+newID+")") {
		t.Fatalf("rewrap = %d %q %q", code, stdout, stderr)
	}
	// Running it again changes nothing.
	code, stdout, stderr = runCmd(t, kekCmd, "rewrap")
	if code != 0 || !strings.Contains(stdout, column+": 0 rewrapped, 2 already current") {
		t.Fatalf("rewrap again = %d %q %q", code, stdout, stderr)
	}

	// The old KEK can go.
	t.Setenv(config.EnvPrefix+"KEK_PREVIOUS", "")
	code, stdout, stderr = runCmd(t, kekCmd, "status")
	if code != 0 || strings.Contains(stdout, oldID) || !strings.Contains(stdout, newID+" (current)") {
		t.Fatalf("status after the rotation = %d %q %q", code, stdout, stderr)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rotate"}, `unknown subcommand "rotate"`},
		{[]string{"status", "extra"}, "want 0 argument(s)"},
	} {
		if code, _, stderr := runCmd(t, kekCmd, tc.args...); code != 2 || !strings.Contains(stderr, tc.want) {
			t.Errorf("kek %q = %d %q; want 2 with %q", tc.args, code, stderr, tc.want)
		}
	}
}
