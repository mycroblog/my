// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/shots"
)

func TestPostRecordsReadScreenshots(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	dir := filepath.Join(root, shots.Dir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := []string{"a-390.png", "b-390.png", "c-390.png", "manifest.json"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(root, "other.png")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Parallel reads run their hooks at the same time: none may lose another's record.
	var wg sync.WaitGroup
	for _, p := range []string{filepath.Join(dir, "a-390.png"), shots.Dir + "/b-390.png", filepath.Join(dir, "c-390.png"), filepath.Join(dir, "manifest.json"), other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if out := run(t, "post", map[string]any{"tool_name": "Read", "cwd": root, "tool_input": map[string]any{"file_path": p}}); out != nil {
				t.Errorf("a read prints nothing, got %v", out)
			}
		}()
	}
	wg.Wait()
	seen := shots.LoadSeen(root)
	if len(seen) != 3 {
		t.Fatalf("want the three screenshots recorded, got %v", seen)
	}
	for _, f := range files[:3] {
		if s, ok := seen[shots.Dir+"/"+f]; !ok || time.Since(s.At) > time.Minute {
			t.Errorf("%s: %v", f, s)
		}
	}
}
