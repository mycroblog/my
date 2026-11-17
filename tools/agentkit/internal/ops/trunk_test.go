// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"os"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// TestMain runs the tests with the default branch as the trunk, which their sandboxes build; the tests of an
// integration branch set repo.Trunk themselves (withTrunk).
func TestMain(m *testing.M) {
	repo.Trunk = repo.Default
	os.Exit(m.Run())
}

// withTrunk makes branch the trunk for the rest of the test.
func withTrunk(t *testing.T, branch string) {
	t.Helper()
	repo.Trunk = branch
	t.Cleanup(func() { repo.Trunk = repo.Default })
}
