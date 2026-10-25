// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This file holds the emoji reactions of accounts to statuses (FR-116, ADR-0052): a Unicode
// emoji or a custom emoji, counted per emoji in the status_reaction_counts projection.

// MaxStatusReactions is the number of emoji whose counts a status presents, the most used first
// (ADR-0052 §5); the others are seen in the list of the accounts that reacted.
const MaxStatusReactions = 20

// ErrInvalidReaction reports an emoji that a reaction cannot have: neither one Unicode emoji nor
// a custom emoji.
var ErrInvalidReaction = errors.New("content: not an emoji that a reaction can have")

// ReactionEmoji is the emoji of a reaction: a Unicode emoji, fully qualified
// (NormalizeUnicodeEmoji), or a custom emoji (custom_emojis). Exactly one is set.
type ReactionEmoji struct {
	Unicode       string
	CustomEmojiID string
}

// Key returns the key that reactions are counted by: the Unicode emoji, or the ID of the
// custom emoji. Custom emoji IDs are UUIDs, which no emoji spells.
func (e ReactionEmoji) Key() string {
	if e.CustomEmojiID != "" {
		return e.CustomEmojiID
	}
	return e.Unicode
}

// Valid reports whether exactly one of the emoji is set.
func (e ReactionEmoji) Valid() bool { return (e.Unicode == "") != (e.CustomEmojiID == "") }

// Reaction is an account's emoji reaction to a status (reactions, ADR-0052 §1). An account
// reacts to a status at most once with each emoji; taking a reaction back deletes it, and
// reacting again makes a new one.
type Reaction struct {
	ID        string
	AccountID string
	StatusID  string
	Emoji     ReactionEmoji
	// URI is the IRI of the Like or EmojiReact.
	URI       string
	CreatedAt time.Time
}

// ReactionCount is the number of reactions to a status with one emoji (status_reaction_counts).
type ReactionCount struct {
	StatusID string
	Emoji    ReactionEmoji
	Count    int64
}

// NormalizeUnicodeEmoji returns s as one fully-qualified emoji of Unicode EmojiVersion, and
// whether it is one. The minimally-qualified and unqualified forms of an emoji, which differ
// from it only in the emoji presentation selectors (U+FE0F), become the fully-qualified form,
// so that the reactions of different servers with the same emoji count together.
func NormalizeUnicodeEmoji(s string) (string, bool) {
	if s == "" || len(s) > 64 {
		return "", false
	}
	fq, ok := emojiForms()[strings.ReplaceAll(s, "\uFE0F", "")]
	return fq, ok
}

var emojiForms = sync.OnceValue(func() map[string]string {
	lines := strings.Split(fullyQualifiedEmoji, "\n")
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		var b strings.Builder
		for _, f := range strings.Fields(line) {
			r, err := strconv.ParseUint(f, 16, 21) // a code point fits in 21 bits
			if err != nil {
				panic("content: malformed emoji table: " + line)
			}
			b.WriteRune(rune(r))
		}
		fq := b.String()
		out[strings.ReplaceAll(fq, "\uFE0F", "")] = fq
	}
	return out
})

// ParseReactionShortcode returns the shortcode of a custom emoji reaction, ":shortcode:", and
// whether s is one. The suffix "@host" that some implementations add to the shortcode of
// another server's emoji is dropped (ADR-0052 §1), as are colons around a bare shortcode
// (TrimShortcode).
func ParseReactionShortcode(s string) (string, bool) {
	if len(s) < 3 || s[0] != ':' || s[len(s)-1] != ':' {
		return "", false
	}
	code := TrimShortcode(s)
	return code, code != ""
}

// TrimShortcode returns the shortcode of a custom emoji's name: without the colons around it and
// the "@host" suffix, or "" when what is left is not a shortcode (letters, digits, "_", "-",
// "+", and ".").
func TrimShortcode(name string) string {
	name = strings.TrimSuffix(strings.TrimPrefix(name, ":"), ":")
	name, _, _ = strings.Cut(name, "@")
	if name == "" || len(name) > 128 {
		return ""
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '+', r == '.':
		default:
			return ""
		}
	}
	return name
}
