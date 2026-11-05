// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/search"
)

// TestSearchCommandsWithNone checks that each command says what search = none means, and does nothing.
func TestSearchCommandsWithNone(t *testing.T) {
	newAdminEnv(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rebuild"}, "so nothing was done"},
		{[]string{"rebuild", "--keep", "--rate", "5"}, "so nothing was done"},
		{[]string{"clear"}, "Delete an index left in an engine in the engine itself"},
		{[]string{"status"}, "no full-text search of posts"},
	} {
		var out, errOut bytes.Buffer
		code := run(append([]string{"search"}, tc.args...), &out, &errOut)
		if code != 0 || !strings.Contains(out.String(), "search = none") || !strings.Contains(out.String(), tc.want) {
			t.Errorf("search %v = %d %q %q, want %q", tc.args, code, out.String(), errOut.String(), tc.want)
		}
	}
}

func TestSearchCommandUsage(t *testing.T) {
	for _, args := range [][]string{{"search"}, {"search", "nothing"}, {"search", "status", "extra"}, {"search", "rebuild", "--rate", "-1"}, {"search", "rebuild", "--rate", "1001"}} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 || errOut.Len() == 0 {
			t.Errorf("%v = %d %q", args, code, errOut.String())
		}
	}
}

func TestPrintSearchRebuild(t *testing.T) {
	at := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	var out bytes.Buffer
	printSearchRebuild(&out, search.Rebuild{
		Mode: search.RebuildClear, Phase: search.RebuildFilling, After: "s9", Rate: 20, Processed: 100, Failed: 1,
		LastError: "engine\nsaid no", StartedAt: at, UpdatedAt: at,
	}, []search.RebuildFailure{{StatusID: "s3", Message: "engine said no", FailedAt: at}})
	for _, want := range []string{
		"rebuild: filling (mode clear)\n", "started: 2026-11-06T01:02:03Z\n", "posts handled: 100\n", "posts failed: 1\n",
		"rate: 20 a second\n", "last post handled: s9\n", "last error: engine said no\n", "failed: s3 2026-11-06T01:02:03Z engine said no\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
