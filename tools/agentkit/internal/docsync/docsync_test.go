// SPDX-License-Identifier: AGPL-3.0-only

package docsync

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
)

func TestLines(t *testing.T) {
	got := Lines([]ctxmap.Impact{
		{Doc: "docs/operations/email.md", Paths: []string{"internal/adapter/mail/smtp/smtp.go"}, Changed: true},
		{Doc: "docs/operations/hidden.md", Paths: []string{"a.go", "b.go", "c.go", "d.go"}},
	})
	want := []string{
		"docs/operations/email.md (changed): describes internal/adapter/mail/smtp/smtp.go",
		"docs/operations/hidden.md (not changed): describes a.go, b.go, c.go and 1 more",
	}
	if len(got) != len(want) {
		t.Fatalf("Lines =\n%s", strings.Join(got, "\n"))
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}
