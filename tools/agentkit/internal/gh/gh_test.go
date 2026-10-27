// SPDX-License-Identifier: AGPL-3.0-only

package gh

import "testing"

func TestNotTruncated(t *testing.T) {
	if err := notTruncated("issues", 4999, 5000); err != nil {
		t.Errorf("a list under the limit is complete: %v", err)
	}
	if err := notTruncated("issues", 5000, 5000); err == nil {
		t.Error("a list at the limit may be cut and must be an error")
	}
}

func TestMoreFilesReadsEveryPage(t *testing.T) {
	pages := map[string]struct {
		files []File
		next  string
	}{
		"c1": {[]File{{Path: "b"}}, "c2"},
		"c2": {[]File{{Path: "c"}}, ""},
	}
	got, err := moreFiles(7, "c1", func(_ int, cursor string) ([]File, pageInfo, error) {
		p := pages[cursor]
		return p.files, pageInfo{HasNextPage: p.next != "", EndCursor: p.next}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "b" || got[1].Path != "c" {
		t.Errorf("got %v", got)
	}
}
