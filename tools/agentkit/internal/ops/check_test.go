// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

func fakeRepo(t *testing.T, script string) *repo.Repo {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatal(err)
	}
	if script != "" {
		if err := os.WriteFile(filepath.Join(root, "scripts", workersScript), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return &repo.Repo{Root: root}
}

func TestWorkersCheck(t *testing.T) {
	if out, err := workersCheck(fakeRepo(t, "")); err != nil || out != nil {
		t.Errorf("a checkout without the script must pass, got %q, %v", out, err)
	}
	if _, err := workersCheck(fakeRepo(t, "#!/bin/sh\nexit 0\n")); err != nil {
		t.Errorf("a passing script failed: %v", err)
	}
	out, err := workersCheck(fakeRepo(t, "#!/bin/sh\necho drift >&2\nexit 1\n"))
	if err == nil || !strings.Contains(string(out), "drift") {
		t.Errorf("a stale list must fail with the script's output, got %q, %v", out, err)
	}
}

func TestGeneratorsWriteWorkersList(t *testing.T) {
	for _, g := range generators {
		if g.prefix == "internal/platform/job_types.go" {
			if g.script != workersScript || len(g.args) != 1 || g.args[0] != "--write" {
				t.Errorf("job_types.go must run the workers check with --write, got %+v", g)
			}
			return
		}
	}
	t.Error("no generator for internal/platform/job_types.go")
}
