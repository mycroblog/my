// SPDX-License-Identifier: AGPL-3.0-only

package shots

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSources(t *testing.T) {
	got := Sources([]string{
		"app/web/src/features/a/A.tsx", "app/ui/src/theme.css", "app/web/src/features/a/A.test.tsx",
		"app/ui/src/Button.stories.tsx", "app/ui/src/__screenshots__/Button.stories.tsx/a.png",
		"app/web/src/routeTree.gen.ts", "app/web/e2e/a.spec.ts", "internal/a.go", "app/web/package.json",
	})
	if want := []string{"app/ui/src/theme.css", "app/web/src/features/a/A.tsx"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadReadsTheManifest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(root); err != nil || got != nil {
		t.Fatalf("no manifest is no screenshots, got %v, %v", got, err)
	}
	for _, f := range []string{"a-390.png", "b-768.png", "c-390.png"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `{"shots":[
	  {"file":".tmp/agentkit/shots/a-390.png","story":"screens-a--normal","width":390,"theme":"light","locale":"en","takenAt":"2026-11-11T10:00:00.5Z"},
	  {"file":"b-768.png","story":"b","width":768,"takenAt":1794650400000},
	  {"file":"c-390.png","story":"c","width":390},
	  {"file":"gone.png","story":"d","width":390,"takenAt":"2026-11-11T10:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
	if want := time.Date(2026, 11, 11, 10, 0, 0, 500000000, time.UTC); got[0].Path != Dir+"/a-390.png" || !got[0].Taken.Equal(want) {
		t.Errorf("got %+v", got[0])
	}
	if !got[1].Taken.Equal(time.UnixMilli(1794650400000)) {
		t.Errorf("epoch milliseconds: got %v", got[1].Taken)
	}
	if time.Since(got[2].Taken) > time.Minute {
		t.Errorf("no takenAt is the file's time: got %v", got[2].Taken)
	}
}
