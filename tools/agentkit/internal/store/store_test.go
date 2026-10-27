// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

func TestIndexSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n\nnot part of the repository\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.md"), []byte("# Inside\n\ntext\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.md")); err != nil {
		t.Skip("symlinks are not available:", err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	r, err := repo.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if ns, err := s.FileNodes("inside.md"); err != nil || len(ns) == 0 {
		t.Errorf("inside.md should be indexed: %v %v", ns, err)
	}
	if ns, err := s.FileNodes("link.md"); err != nil || len(ns) != 0 {
		t.Errorf("a symlink must not be indexed: %v %v", ns, err)
	}
}
