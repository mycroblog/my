// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func write(t *testing.T, root, p, content string) {
	t.Helper()
	full := filepath.Join(root, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCheckEvidenceIsBoundToTheBase: a pass recorded against one commit of the base does not count after the
// base moves, and a pass for one set of changed files does not count for another.
func TestCheckEvidenceIsBoundToTheBase(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q", "-b", "main")
	write(t, root, ".gitignore", ".tmp/\n")
	write(t, root, "a.txt", "a\n")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "init")
	gitIn(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitIn(t, root, "switch", "-qc", "work")
	write(t, root, "b.txt", "b\n")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "work")
	r, err := repo.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	const base = "origin/main"
	record := func() {
		t.Helper()
		if _, err := r.TmpDir("check"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(passedFile(r), []byte(evidenceKey(r, base)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if CheckPassed(r, base) {
		t.Fatal("no evidence recorded yet")
	}
	record()
	if !CheckPassed(r, base) {
		t.Fatal("evidence for this tree and base must count")
	}
	// Committing the checked changes keeps the evidence.
	write(t, root, "b.txt", "b\n")
	if !CheckPassed(r, base) {
		t.Fatal("an unchanged tree keeps the evidence")
	}
	// The same tree content compared with a different base commit.
	gitIn(t, root, "switch", "-q", "main")
	write(t, root, "c.txt", "c\n")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "main moves")
	gitIn(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitIn(t, root, "switch", "-q", "work")
	gitIn(t, root, "merge", "-q", "--no-edit", "main")
	if CheckPassed(r, base) {
		t.Fatal("evidence computed against another base must not count")
	}
	// A base that does not resolve never counts.
	if CheckPassed(r, "origin/missing") {
		t.Fatal("an unresolvable base must not count")
	}
}
