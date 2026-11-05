// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
	"time"
)

func TestAdminMediaReprocess(t *testing.T) {
	newAdminEnv(t)
	code, stdout, stderr := runCmd(t, adminCmd, "media", "reprocess", "-before", "2026-01-31T09:00:00+09:00")
	if code != 0 || !strings.Contains(stdout, "before 2026-01-31T00:00:00Z") {
		t.Fatalf("reprocess = %d %q %q", code, stdout, stderr)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"media", "reprocess"}, "-before wants a time"},
		{[]string{"media", "reprocess", "-before", "yesterday"}, "-before wants a time"},
		{[]string{"media", "reprocess", "-before", time.Now().Add(time.Hour).Format(time.RFC3339)}, "a time that has passed"},
		{[]string{"media", "reprocess", "-before", "2026-01-31T09:00:00Z", "extra"}, "want 0 argument(s)"},
		{[]string{"media", "everything"}, `unknown subcommand "everything"`},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != 2 || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want 2 with %q", tc.args, code, stderr, tc.want)
		}
	}
}
