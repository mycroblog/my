// SPDX-License-Identifier: AGPL-3.0-only

package gh

import "testing"

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
