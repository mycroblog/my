// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// TestScanShortcodesVectors checks the shortcodes of the vectors that internal/app/content's
// ParseText test and the web client's lib/textTokens.ts read too (ADR-0092 §4): where each one
// is, and what ScanShortcodes returns.
func TestScanShortcodesVectors(t *testing.T) {
	b, err := os.ReadFile("../../../pkg/testkit/fixtures/text/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []struct {
			Name   string `json:"name"`
			Text   string `json:"text"`
			Tokens []struct {
				Kind string `json:"kind"`
				UTF8 [2]int `json:"utf8"`
			} `json:"tokens"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, tc := range doc.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			var want [][2]int
			var codes []string
			for _, tok := range tc.Tokens {
				if tok.Kind != "shortcode" {
					continue
				}
				want = append(want, tok.UTF8)
				if code := tc.Text[tok.UTF8[0]+1 : tok.UTF8[1]-1]; !slices.Contains(codes, code) {
					codes = append(codes, code)
				}
			}
			if got := ShortcodeSpans(tc.Text); !slices.Equal(got, want) {
				t.Errorf("ShortcodeSpans(%q) = %v, want %v", tc.Text, got, want)
			}
			if got := ScanShortcodes(tc.Text); !slices.Equal(got, codes) {
				t.Errorf("ScanShortcodes(%q) = %q, want %q", tc.Text, got, codes)
			}
		})
	}
}
