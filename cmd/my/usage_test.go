// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestUsageNamesEveryCommand keeps the usage texts complete: an operator who reads them finds the
// commands that the documents describe.
func TestUsageNamesEveryCommand(t *testing.T) {
	text := func(f func(io.Writer)) string {
		var b bytes.Buffer
		f(&b)
		return b.String()
	}
	top := text(usage)
	for _, sub := range []string{
		"create", "account", "appeal", "audit", "closure", "domain", "emoji", "federation", "media", "refetch", "relay",
		"report", "retention", "role", "signup-block", "supporters", "theme", "brand", "types",
	} {
		if !strings.Contains(top, sub) {
			t.Errorf("my -help does not name %s", sub)
		}
	}
	admin := text(adminUsage)
	for _, want := range []string{
		"domain list|issue|lift|export|import", "report list|show|assign|unassign|resolve|note|reopen|forward",
		"emoji create|list|update|replace-image|enable|disable|copy|delete|requests", "signup-block create|import|list|lift",
	} {
		if !strings.Contains(admin, want) {
			t.Errorf("my admin -help does not name %s", want)
		}
	}
}

// TestJobTypesCommand prints the job types without any configuration.
func TestJobTypesCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runJob([]string{"types"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "federation.deliver\n") {
		t.Errorf("my job types = %d, %q, %q", code, stdout.String(), stderr.String())
	}
	if code := runJob([]string{"types", "extra"}, &stdout, &stderr); code != 2 {
		t.Errorf("my job types extra = %d, want 2", code)
	}
}
