// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"regexp"
	"strings"
	"testing"
)

var addedRelay = regexp.MustCompile(`^created mastodon relay https://relay\.example/inbox \(relay (\S+), inbox https://relay\.example/inbox\); its Follow is sent\n$`)

func TestAdminRelayCommands(t *testing.T) {
	newAdminEnv(t)
	code, stdout, stderr := runCmd(t, adminCmd, "relay", "create", "-protocol", "mastodon", "https://relay.example/inbox")
	m := addedRelay.FindStringSubmatch(stdout)
	if code != 0 || m == nil {
		t.Fatalf("relay create = %d %q %q", code, stdout, stderr)
	}
	id := m[1]
	if code, stdout, _ = runCmd(t, adminCmd, "relay", "list"); code != 0 ||
		!regexp.MustCompile(id+` +mastodon +https://relay\.example/inbox +pending +\S+ +true +0 +0 +false`).MatchString(stdout) {
		t.Errorf("relay list = %d\n%s", code, stdout)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "relay", "unfollow", id); code != 0 || stdout != "left relay "+id+"\n" {
		t.Errorf("relay unfollow = %d %q", code, stdout)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "relay", "follow", id); code != 0 || stdout != "sent a new Follow to relay "+id+"\n" {
		t.Errorf("relay follow = %d %q", code, stdout)
	}
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"relay", "remove", id}, 1, "leave it before removing it"},
		{[]string{"relay", "follow", id}, 1, "leave it before joining it again"},
		{[]string{"relay", "create", "-protocol", "mastodon", "https://relay.example/inbox"}, 1, "already registered"},
		{[]string{"relay", "create", "-protocol", "mastodon", "http://relay.example/inbox"}, 1, "https"},
		{[]string{"relay", "create", "-protocol", "pleroma", "https://relay.example/inbox"}, 2, "-protocol must be"},
		{[]string{"relay", "unfollow", "01890a5d-ac96-774b-bcce-b302099a0000"}, 1, "no relay has the ID"},
		{[]string{"relay", "remove"}, 2, "want 1 argument(s)"},
		{[]string{"relay", "burn"}, 2, `unknown subcommand "burn"`},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
	if code, _, _ = runCmd(t, adminCmd, "relay", "unfollow", id); code != 0 {
		t.Fatalf("relay unfollow = %d", code)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "relay", "remove", id); code != 0 || stdout != "removed relay "+id+"\n" {
		t.Errorf("relay remove = %d %q", code, stdout)
	}
	if _, stdout, _ = runCmd(t, adminCmd, "relay", "list"); strings.Contains(stdout, id) {
		t.Errorf("relay list after remove:\n%s", stdout)
	}
	if _, stdout, _ = runCmd(t, adminCmd, "audit", "list", "-target-kind", "relay"); strings.Count(stdout, "relay_") < 6 {
		t.Errorf("audit list of the relays:\n%s", stdout)
	}
}
