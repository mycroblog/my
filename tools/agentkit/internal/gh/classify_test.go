// SPDX-License-Identifier: AGPL-3.0-only

package gh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The seam of an issue's type, priority and ready mark: what the rest of agentkit asks, and what reaches gh.
func TestClassify(t *testing.T) {
	epic := &Issue{Labels: []Label{{Name: "type:epic"}, {Name: "area:harness"}}}
	ready := &Issue{Labels: []Label{{Name: "type:task"}, {Name: "status:ready"}}}
	if !epic.IsEpic() || ready.IsEpic() {
		t.Error("IsEpic: the epic is the issue of type epic")
	}
	if epic.IsReady() || !ready.IsReady() {
		t.Error("IsReady: the ready issue is the one marked ready")
	}

	dir := t.TempDir()
	log := filepath.Join(dir, "gh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\ncase \"$*\" in *p9*) echo no such label >&2; exit 1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGENTKIT_REPO", "")
	for _, err := range []error{SetType(5, "bug"), SetPriority(5, "p0"), MarkReady(5)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "issue edit 5 -R mycroblog/my --add-label type:bug\nissue edit 5 -R mycroblog/my --add-label priority:p0\nissue edit 5 -R mycroblog/my --add-label status:ready\n"
	if string(b) != want {
		t.Errorf("gh calls:\n%s\nwant:\n%s", b, want)
	}
	if err := SetPriority(5, "p9"); err == nil || !strings.Contains(err.Error(), "to do it by hand: gh issue edit 5") {
		t.Errorf("a failure must say how to do it by hand: %v", err)
	}
}
