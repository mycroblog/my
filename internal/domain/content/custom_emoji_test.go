// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/content"
)

func TestScanShortcodes(t *testing.T) {
	for _, tc := range []struct {
		texts []string
		want  []string
	}{
		{[]string{":blobcat:"}, []string{"blobcat"}},
		{[]string{"hi :blobcat: and :Blob_2:!"}, []string{"blobcat", "Blob_2"}},
		{[]string{":blobcat::blobfox:"}, nil},            // a colon right after or before
		{[]string{"a:blobcat: b"}, nil},                  // a letter before
		{[]string{":blobcat:b"}, nil},                    // a letter after
		{[]string{"あ:blobcat:"}, nil},                    // a Unicode letter before
		{[]string{"(:blobcat:)"}, []string{"blobcat"}},   // punctuation around
		{[]string{"\n:blobcat:\n"}, []string{"blobcat"}}, // line breaks around
		{[]string{":a:"}, nil},                           // too short
		{[]string{":blob cat:"}, nil},                    // a space inside
		{[]string{":blobcat: :blobcat:", ":blobfox:"}, []string{"blobcat", "blobfox"}},
		{[]string{"12:30:45"}, nil},
		{[]string{"::", ":", ""}, nil},
	} {
		if got := content.ScanShortcodes(tc.texts...); !slices.Equal(got, tc.want) {
			t.Errorf("ScanShortcodes(%q) = %q, want %q", tc.texts, got, tc.want)
		}
	}
}

func TestValidLocalShortcode(t *testing.T) {
	for s, want := range map[string]bool{
		"ab": true, "Blob_Cat_2": true, strings.Repeat("a", 30): true,
		"a": false, strings.Repeat("a", 31): false, "blob-cat": false, "ねこ": false, "": false, "a b": false,
	} {
		if got := content.ValidLocalShortcode(s); got != want {
			t.Errorf("ValidLocalShortcode(%q) = %v, want %v", s, got, want)
		}
	}
}

func reasonOf(err error) content.CustomEmojiReason {
	var ce *content.CustomEmojiError
	if errors.As(err, &ce) {
		return ce.Reason
	}
	return ""
}

func TestNormalizeCustomEmojiAliases(t *testing.T) {
	got, err := content.NormalizeCustomEmojiAliases("cat", []string{"neko", "cat", "kitty", "neko"})
	if err != nil || !slices.Equal(got, []string{"kitty", "neko"}) {
		t.Errorf("NormalizeCustomEmojiAliases = %q, %v", got, err)
	}
	if _, err := content.NormalizeCustomEmojiAliases("cat", []string{"ね"}); reasonOf(err) != content.EmojiInvalidAlias {
		t.Errorf("an invalid alias = %v", err)
	}
	many := []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11"}
	if _, err := content.NormalizeCustomEmojiAliases("cat", many); reasonOf(err) != content.EmojiInvalidAlias || !errors.Is(err, content.ErrInvalidCustomEmoji) {
		t.Errorf("11 aliases = %v", err)
	}
}

func TestNormalizeCustomEmojiCategory(t *testing.T) {
	if got, err := content.NormalizeCustomEmojiCategory("  Cats  "); err != nil || got != "Cats" {
		t.Errorf("NormalizeCustomEmojiCategory = %q, %v", got, err)
	}
	for _, c := range []string{strings.Repeat("あ", 65), "a\nb", "a\x00"} {
		if _, err := content.NormalizeCustomEmojiCategory(c); reasonOf(err) != content.EmojiInvalidCategory {
			t.Errorf("NormalizeCustomEmojiCategory(%q) = %v", c, err)
		}
	}
}

func TestCheckCustomEmojiImage(t *testing.T) {
	ok := content.CustomEmojiImageFacts{ContentType: "image/png", FileSize: 1000, Width: 128, Height: 128}
	if err := content.CheckCustomEmojiImage(ok, content.DefaultCustomEmojiMaxBytes); err != nil {
		t.Errorf("a small PNG = %v", err)
	}
	for _, tc := range []struct {
		f    content.CustomEmojiImageFacts
		want content.CustomEmojiReason
	}{
		{content.CustomEmojiImageFacts{ContentType: "image/jpeg", FileSize: 10, Width: 1, Height: 1}, content.EmojiUnsupportedFormat},
		{content.CustomEmojiImageFacts{ContentType: "image/gif", FileSize: content.DefaultCustomEmojiMaxBytes + 1}, content.EmojiTooLarge},
		{content.CustomEmojiImageFacts{ContentType: "image/webp", FileSize: 10, Width: 1025, Height: 10}, content.EmojiTooBig},
	} {
		if got := reasonOf(content.CheckCustomEmojiImage(tc.f, content.DefaultCustomEmojiMaxBytes)); got != tc.want {
			t.Errorf("CheckCustomEmojiImage(%+v) = %q, want %q", tc.f, got, tc.want)
		}
	}
}

func TestDefaultCustomEmojiSettings(t *testing.T) {
	s := content.DefaultCustomEmojiSettings("e")
	if !s.IsDefault() || !s.Enabled || !s.VisibleInPicker || s.Category != "" {
		t.Errorf("DefaultCustomEmojiSettings = %+v", s)
	}
	s.Enabled = false
	if s.IsDefault() {
		t.Error("a disabled emoji has the default settings")
	}
}

func TestCustomEmojiLimitPeriod(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	cases := []struct {
		now          time.Time
		since, until time.Time
	}{
		{time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 10, 31, 23, 59, 59, 999, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		// The month is the UTC one: 08:00 on the 1st in Japan is still the previous month.
		{time.Date(2026, 11, 1, 8, 0, 0, 0, jst), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		since, until := content.CustomEmojiLimitPeriod(c.now)
		if !since.Equal(c.since) || !until.Equal(c.until) {
			t.Errorf("CustomEmojiLimitPeriod(%v) = %v, %v; want %v, %v", c.now, since, until, c.since, c.until)
		}
	}
}
