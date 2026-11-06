// SPDX-License-Identifier: AGPL-3.0-only

package main

import "testing"

func TestComment(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{" One line.\n", "One line."},
		// The lines stay, and the blank lines between paragraphs are kept once.
		{" First line,\n second line.\n\n\n Next paragraph.\n", "First line,\nsecond line.\n\nNext paragraph."},
		// A bare URL becomes an autolink, without the punctuation after it.
		{" The blurhash (https://blurha.sh) of it. See https://example.com/a.", "The blurhash (<https://blurha.sh>) of it. See <https://example.com/a>."},
	} {
		if got := comment(tc.in); got != tc.want {
			t.Errorf("comment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestItem(t *testing.T) {
	if got := item("", ""); got != "" {
		t.Errorf("item without a comment = %q", got)
	}
	// The later lines are indented to stay in the list item, and the trailing comment follows as a paragraph.
	if got, want := item(" One,\n two.\n\n Three.\n", " Four."), ": One,\n  two.\n\n  Three.\n\n  Four."; got != want {
		t.Errorf("item = %q, want %q", got, want)
	}
}

// A repeated heading gets the anchor that GitHub and the site give it.
func TestHeadingAnchors(t *testing.T) {
	p := &page{seen: map[string]int{}}
	for _, want := range []string{"outcome", "outcome-1", "outcome-2"} {
		if got := p.head(3, "`Outcome`"); got != want {
			t.Errorf("anchor = %q, want %q", got, want)
		}
	}
	if got := p.head(2, "`my.plugin.key_manager.v1`"); got != "mypluginkey_managerv1" {
		t.Errorf("anchor = %q", got)
	}
}
