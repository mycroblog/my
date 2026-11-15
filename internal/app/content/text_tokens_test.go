// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"unicode/utf16"

	"github.com/mycroblog/my/internal/app/content"
)

// textVectorsPath is the file of vectors that the web client's
// web/app-features/src/compose/text-tokens.ts reads too (ADR-0092 §4).
const textVectorsPath = "../../../pkg/testkit/fixtures/text/tokens.json"

type textTokenCase struct {
	Name   string `json:"name"`
	Text   string `json:"text"`
	Tokens []struct {
		Kind  string `json:"kind"`
		Text  string `json:"text"`
		UTF8  [2]int `json:"utf8"`
		UTF16 [2]int `json:"utf16"`
	} `json:"tokens"`
}

// textToken is a token as the test compares it: its kind, text, and UTF-8 start.
type textToken struct {
	Kind, Text string
	Start      int
}

// utf16Len returns the length of s in UTF-16 code units.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// TestParseTextVectors checks the mentions, hashtags, and URLs of the shared vectors; the
// shortcodes are checked by ScanShortcodes' test in internal/domain/content.
func TestParseTextVectors(t *testing.T) {
	b, err := os.ReadFile(textVectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []textTokenCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	kinds := map[content.SegmentKind]string{
		content.SegmentMention: "mention", content.SegmentHashtag: "hashtag", content.SegmentURL: "url",
	}
	for _, tc := range doc.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			var want []textToken
			for _, tok := range tc.Tokens {
				if tc.Text[tok.UTF8[0]:tok.UTF8[1]] != tok.Text {
					t.Errorf("utf8 %v of %q is not %q", tok.UTF8, tc.Text, tok.Text)
				}
				if s := utf16Len(tc.Text[:tok.UTF8[0]]); tok.UTF16 != [2]int{s, s + utf16Len(tok.Text)} {
					t.Errorf("utf16 of %q is %v, want %v", tok.Text, tok.UTF16, [2]int{s, s + utf16Len(tok.Text)})
				}
				if tok.Kind != "shortcode" {
					want = append(want, textToken{tok.Kind, tok.Text, tok.UTF8[0]})
				}
			}
			var got []textToken
			pos := 0
			for _, s := range content.ParseText(tc.Text).Segments {
				if s.Kind != content.SegmentText {
					got = append(got, textToken{kinds[s.Kind], s.Text, pos})
				}
				pos += len(s.Text)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ParseText(%q) tokens = %+v, want %+v", tc.Text, got, want)
			}
		})
	}
}
