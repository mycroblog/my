// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"os"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// TestMain runs the tests with the default branch as the trunk; TestMergeIntegrationBranch sets its own.
func TestMain(m *testing.M) {
	repo.Trunk = repo.Default
	os.Exit(m.Run())
}

func TestMergeIntegrationBranch(t *testing.T) {
	repo.Trunk = "redesign-web"
	t.Cleanup(func() { repo.Trunk = repo.Default })
	for _, tc := range []struct {
		ref    string
		denied bool
	}{
		{"origin/redesign-web", true},
		{"redesign-web", true},
		{"origin/main", false}, // the trunk keeps up with the default branch by merging it
		{"main", false},
	} {
		if got := isMain(tc.ref); got != tc.denied {
			t.Errorf("isMain(%q) = %v, want %v", tc.ref, got, tc.denied)
		}
	}
	if r := rebaseReason(); !contains(r, "git rebase origin/redesign-web") {
		t.Errorf("rebaseReason() = %q, want the rebase onto the trunk", r)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
