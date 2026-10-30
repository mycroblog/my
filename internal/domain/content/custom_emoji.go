// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The local custom emoji (ADR-0051): the rules of their shortcodes, aliases, categories, and
// images, and the scan of :shortcode: in the text of local posts and profiles.

// Bounds of the local custom emoji (ADR-0051 §1, §2).
const (
	// MinShortcodeLength and MaxShortcodeLength bound a local shortcode and alias.
	MinShortcodeLength = 2
	MaxShortcodeLength = 30
	// MaxCustomEmojiAliases is the most aliases of one emoji.
	MaxCustomEmojiAliases = 10
	// MaxCustomEmojiCategory is the most characters of a category.
	MaxCustomEmojiCategory = 64
	// MaxCustomEmojiSide is the most pixels of the longer side of the image of a local emoji.
	MaxCustomEmojiSide = 1024
	// DefaultCustomEmojiMaxBytes is the default limit of the size of the image of a local emoji
	// (the setting custom_emoji_max_bytes), Mastodon's limit.
	DefaultCustomEmojiMaxBytes = 256 << 10
	// DefaultRemoteCustomEmojiMaxBytes is the default limit of the size of the image of a remote
	// emoji that the media proxy serves (the setting remote_custom_emoji_max_bytes).
	DefaultRemoteCustomEmojiMaxBytes = 1 << 20
	// CustomEmojiFederationWarnBytes is the size above which GoToSocial does not fetch a remote
	// emoji by default; the admin screen warns about it.
	CustomEmojiFederationWarnBytes = 100 << 10
)

// CustomEmojiReason says why a custom emoji is refused.
type CustomEmojiReason string

// Reasons of a CustomEmojiError.
const (
	EmojiInvalidShortcode  CustomEmojiReason = "invalid_shortcode"
	EmojiShortcodeTaken    CustomEmojiReason = "shortcode_taken"
	EmojiInvalidAlias      CustomEmojiReason = "invalid_alias"
	EmojiInvalidCategory   CustomEmojiReason = "invalid_category"
	EmojiMediaNotFound     CustomEmojiReason = "media_not_found"
	EmojiMediaNotReady     CustomEmojiReason = "media_not_ready"
	EmojiUnsupportedFormat CustomEmojiReason = "unsupported_format"
	EmojiTooLarge          CustomEmojiReason = "too_large"
	EmojiTooBig            CustomEmojiReason = "too_big"
	EmojiRemote            CustomEmojiReason = "remote_emoji"
	// EmojiLocal refuses an operation for remote emoji only (the copy) on a local one.
	EmojiLocal CustomEmojiReason = "local_emoji"
	// EmojiFetchFailed reports the image of a remote emoji that could not be fetched: its
	// server did not return it, or its domain's media are rejected.
	EmojiFetchFailed CustomEmojiReason = "fetch_failed"
	// EmojiLimitReached refuses a request to create an emoji from a user who has submitted as
	// many approved and pending requests to create one this month as their limit (ADR-0124).
	EmojiLimitReached CustomEmojiReason = "limit_reached"
	// EmojiRequestPending refuses a request to replace the image of an emoji that has a
	// pending one (ADR-0070 §3).
	EmojiRequestPending CustomEmojiReason = "request_pending"
	// EmojiInvalidReason refuses the reason of a rejection: longer than
	// MaxCustomEmojiRejectReason characters or not text.
	EmojiInvalidReason CustomEmojiReason = "invalid_reason"
)

// ErrInvalidCustomEmoji is wrapped by every *CustomEmojiError.
var ErrInvalidCustomEmoji = errors.New("content: invalid custom emoji")

// CustomEmojiError reports a custom emoji that breaks a rule.
type CustomEmojiError struct {
	Reason CustomEmojiReason
	// Value is what broke the rule, for the logs.
	Value string
}

func (e *CustomEmojiError) Error() string {
	return fmt.Sprintf("content: custom emoji: %s %q", e.Reason, e.Value)
}

// Unwrap makes errors.Is(err, ErrInvalidCustomEmoji) hold.
func (e *CustomEmojiError) Unwrap() error { return ErrInvalidCustomEmoji }

func emojiError(r CustomEmojiReason, v string) error { return &CustomEmojiError{Reason: r, Value: v} }

// shortcodeByte reports whether b may be part of a shortcode: an ASCII letter, a digit, or "_".
func shortcodeByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// ValidLocalShortcode reports whether s is a shortcode that a local emoji or alias may have:
// MinShortcodeLength to MaxShortcodeLength ASCII letters, digits, and underscores (Mastodon's
// characters, GoToSocial's length). Case matters.
func ValidLocalShortcode(s string) bool {
	if len(s) < MinShortcodeLength || len(s) > MaxShortcodeLength {
		return false
	}
	for i := range len(s) {
		if !shortcodeByte(s[i]) {
			return false
		}
	}
	return true
}

// CustomEmojiSettings are the settings of a custom emoji (custom_emoji_settings, a current
// value). The zero row of an emoji without settings is DefaultCustomEmojiSettings.
type CustomEmojiSettings struct {
	EmojiID         string
	Enabled         bool
	VisibleInPicker bool
	// Category is "" for none.
	Category string
}

// DefaultCustomEmojiSettings returns the settings of an emoji without a settings row: enabled,
// in the picker, without a category.
func DefaultCustomEmojiSettings(emojiID string) CustomEmojiSettings {
	return CustomEmojiSettings{EmojiID: emojiID, Enabled: true, VisibleInPicker: true}
}

// IsDefault reports whether s are the default settings, which need no row.
func (s CustomEmojiSettings) IsDefault() bool {
	return s == DefaultCustomEmojiSettings(s.EmojiID)
}

// NormalizeCustomEmojiCategory trims a category and checks it: valid text of at most
// MaxCustomEmojiCategory characters on one line. "" is no category.
func NormalizeCustomEmojiCategory(c string) (string, error) {
	c = strings.TrimSpace(c)
	if !ValidText(c) || strings.ContainsAny(c, "\r\n") || utf8.RuneCountInString(c) > MaxCustomEmojiCategory {
		return "", emojiError(EmojiInvalidCategory, c)
	}
	return c, nil
}

// NormalizeCustomEmojiAliases checks aliases and returns them sorted without duplicates: at
// most MaxCustomEmojiAliases, each a valid local shortcode other than the emoji's own.
func NormalizeCustomEmojiAliases(shortcode string, aliases []string) ([]string, error) {
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if !ValidLocalShortcode(a) {
			return nil, emojiError(EmojiInvalidAlias, a)
		}
		if a != shortcode && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	if len(out) > MaxCustomEmojiAliases {
		return nil, emojiError(EmojiInvalidAlias, fmt.Sprint(len(out)))
	}
	slices.Sort(out)
	return out, nil
}

// CustomEmojiImageFacts are what the checks of the image of a local emoji read of an upload:
// the type detected from its content, its size as received, and its dimensions.
type CustomEmojiImageFacts struct {
	ContentType   string
	FileSize      int64
	Width, Height int
}

// CustomEmojiFormat reports whether contentType, detected from the content, is a format of the
// images of custom emoji, local or remote (ADR-0051 §2): PNG (APNG too), GIF, or WebP. JPEG,
// AVIF, and SVG are not, as Mastodon does not take them as emoji.
func CustomEmojiFormat(contentType string) bool {
	switch contentType {
	case "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// CheckCustomEmojiImage checks the image of a local emoji (ADR-0051 §2): a PNG (an APNG shows
// its first frame), a GIF, or a WebP, of at most maxBytes, at most MaxCustomEmojiSide pixels on
// its longer side.
func CheckCustomEmojiImage(f CustomEmojiImageFacts, maxBytes int64) error {
	if !CustomEmojiFormat(f.ContentType) {
		return emojiError(EmojiUnsupportedFormat, f.ContentType)
	}
	if maxBytes > 0 && f.FileSize > maxBytes {
		return emojiError(EmojiTooLarge, fmt.Sprint(f.FileSize))
	}
	if f.Width > MaxCustomEmojiSide || f.Height > MaxCustomEmojiSide {
		return emojiError(EmojiTooBig, fmt.Sprintf("%dx%d", f.Width, f.Height))
	}
	return nil
}

// ScanShortcodes returns the shortcodes that texts write as :shortcode:, in order of first use
// and without duplicates. A shortcode is found as Mastodon's CustomEmoji::SCAN_RE finds it: two
// or more letters, digits, and underscores between colons, with neither a letter, a digit, nor
// a colon right before the opening colon or after the closing one. The scan reads ASCII
// shortcodes only, which are the only ones that local emoji have.
func ScanShortcodes(texts ...string) []string {
	var out []string
	for _, t := range texts {
		for _, span := range shortcodeSpans(t) {
			if code := t[span[0]+1 : span[1]-1]; !slices.Contains(out, code) {
				out = append(out, code)
			}
		}
	}
	return out
}

// shortcodeSpans returns the byte ranges [start, end) of every :shortcode: in t, colons
// included, in order, under the rules of ScanShortcodes.
func shortcodeSpans(t string) [][2]int {
	var out [][2]int
	for i := 0; i < len(t); i++ {
		if t[i] != ':' || i > 0 && blocksShortcode(t, i, true) {
			continue
		}
		j := i + 1
		for j < len(t) && shortcodeByte(t[j]) {
			j++
		}
		if j >= len(t) || t[j] != ':' || j-i-1 < MinShortcodeLength {
			continue
		}
		if j+1 < len(t) && blocksShortcode(t, j+1, false) {
			continue
		}
		out = append(out, [2]int{i, j + 1})
		i = j // the closing colon cannot open another shortcode: it follows a letter
	}
	return out
}

// blocksShortcode reports whether the character before (before is true) the byte at i of t, or
// the one starting at i, is a letter, a digit, or a colon, which keeps a shortcode from
// starting or ending there (Mastodon's [[:alnum:]:], Unicode letters and digits included).
func blocksShortcode(t string, i int, before bool) bool {
	var r rune
	if before {
		r, _ = utf8.DecodeLastRuneInString(t[:i])
	} else {
		r, _ = utf8.DecodeRuneInString(t[i:])
	}
	return r == ':' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
