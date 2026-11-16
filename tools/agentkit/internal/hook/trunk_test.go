// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"os"
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// TestMain runs the tests with the default branch as the trunk; TestTrunk sets its own.
func TestMain(m *testing.M) {
	repo.Trunk = repo.Default
	os.Exit(m.Run())
}

func TestTrunk(t *testing.T) {
	repo.Trunk = "redesign-web"
	t.Cleanup(func() { repo.Trunk = repo.Default })
	for _, tc := range []struct {
		ref   string
		trunk bool
	}{
		{"origin/redesign-web", true},
		{"redesign-web", true},
		{"origin/main", false},
		{"main", false},
	} {
		if got := isTrunk(tc.ref); got != tc.trunk {
			t.Errorf("isTrunk(%q) = %v, want %v", tc.ref, got, tc.trunk)
		}
	}
	if r := mergeReason(); !strings.Contains(r, "origin/redesign-web") {
		t.Errorf("mergeReason() = %q, want the trunk named", r)
	}
}
