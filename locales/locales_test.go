// SPDX-License-Identifier: AGPL-3.0-only

package locales

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFS checks that FS holds every catalog.
func TestFS(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		// <locale>/<namespace>/<area>.json
		segments := strings.Split(filepath.ToSlash(path), "/")
		if len(segments) < 3 {
			t.Errorf("%s is outside a namespace directory: catalogs are <locale>/<namespace>/<area>.json "+
				"(scripts/task.sh i18n:gen removes such files from the pseudo-locales)", path)
			return nil
		}
		if _, statErr := fs.Stat(FS, filepath.ToSlash(path)); statErr != nil {
			t.Errorf("FS does not embed %s: add its namespace to the go:embed directive (%v)", path, statErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("en/server"); err != nil {
		t.Fatalf("the source catalog is missing: %v", err)
	}
}
