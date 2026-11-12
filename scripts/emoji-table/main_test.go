// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `# group: Smileys & Emotion
263A FE0F ; fully-qualified # ☺️ E0.6 smiling face
263A ; unqualified # ☺ E0.6 smiling face
# group: People & Body
1F44B 1F3FB ; fully-qualified # 👋🏻 E1.0 waving hand: light skin tone
# group: Component
1F3FB ; component # 🏻 E1.0 light skin tone
`

func TestServerTable(t *testing.T) {
	entries, err := parseEmojiTest([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	src, err := serverTable(entries, "18.0.0")
	if err != nil {
		t.Fatal(err)
	}
	got := string(src)
	for _, want := range []string{`const EmojiVersion = "18.0.0"`, "fullyQualifiedEmoji = `263A FE0F\n1F44B 1F3FB`"} {
		if !strings.Contains(got, want) {
			t.Errorf("the table has no %s:\n%s", want, got)
		}
	}
}

// TestCommittedData checks that the committed server table lists the emoji of a whole emoji-test.txt as code points.
func TestCommittedData(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "domain", "content", "emoji_table.gen.go")
	src, err := os.ReadFile(path) //nolint:gosec // G304: a file of the repository the test names
	if err != nil {
		t.Fatal(err)
	}
	_, table, _ := strings.Cut(string(src), "fullyQualifiedEmoji = `")
	table, _, _ = strings.Cut(table, "`")
	n := 0
	for line := range strings.SplitSeq(table, "\n") {
		for p := range strings.FieldsSeq(line) {
			if _, err := parseCodePoint(p); err != nil {
				t.Fatal(err)
			}
		}
		n++
	}
	if n < 3000 {
		t.Fatalf("the server table has only %d emoji", n)
	}
}
