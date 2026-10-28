// SPDX-License-Identifier: AGPL-3.0-only

package mailpreview

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

// goldenLocales are the locales of the golden files.
var goldenLocales = []string{"en", "ja"}

// goldenDir holds testdata/golden/<locale>/<name>.html and .txt, the HTML and the text version
// of every entry, so that a change to an email shows in the diff.
var goldenDir = filepath.Join("testdata", "golden")

const goldenHint = "run `scripts/task.sh mail:golden` and review the diff"

// TestGolden fails when an email differs from its golden files, or a golden file has no email.
func TestGolden(t *testing.T) {
	bundle, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.RemoveAll(goldenDir); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range goldenLocales {
		rendered, err := Render(bundle, locale)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(goldenDir, locale)
		var want []string
		for _, r := range rendered {
			for ext, body := range map[string]string{".html": r.Message.HTML, ".txt": r.Message.Text} {
				name := r.Entry.Name() + ext
				want = append(want, name)
				path := filepath.Join(dir, name)
				if *update {
					if err := os.MkdirAll(dir, 0o750); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
						t.Fatal(err)
					}
					continue
				}
				got, err := os.ReadFile(path) //nolint:gosec // G304: a golden file of this package
				if err != nil {
					t.Errorf("%v; %s", err, goldenHint)
					continue
				}
				if string(got) != body {
					t.Errorf("%s is stale; %s", path, goldenHint)
				}
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !slices.Contains(want, e.Name()) {
				t.Errorf("%s has no email; %s", filepath.Join(dir, e.Name()), goldenHint)
			}
		}
	}
}

// TestDark keeps the preview's dark mode working: the frame has dark styles for Dark to apply.
func TestDark(t *testing.T) {
	bundle, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(bundle, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := rendered[0].Message.HTML
	if !strings.Contains(html, darkMedia) {
		t.Fatalf("the frame has no %q; update Dark with the frame", darkMedia)
	}
	if strings.Contains(Dark(html), darkMedia) {
		t.Error("Dark left the dark styles conditional")
	}
}
