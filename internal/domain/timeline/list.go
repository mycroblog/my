// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The limits of lists (ADR-0122 §5). They bound the IN of a list timeline's fan-out-on-read to
// about the size of a home timeline's follows, and what a screen can handle; they are constants,
// not settings.
const (
	// MaxLists is the most lists an account owns.
	MaxLists = 50
	// MaxListMembers is the most members a list has.
	MaxListMembers = 1000
	// MaxListNameGraphemes is the longest name of a list, in grapheme clusters.
	MaxListNameGraphemes = 50
	// MaxListNameLength bounds the code points of a name, whatever its grapheme clusters.
	MaxListNameLength = 200
)

// Errors of the list rules.
var (
	// ErrInvalidListName reports a name that ValidListName rejects.
	ErrInvalidListName = errors.New("timeline: invalid list name")
	// ErrTooManyLists reports a list beyond MaxLists.
	ErrTooManyLists = errors.New("timeline: too many lists")
	// ErrTooManyListMembers reports a member beyond MaxListMembers.
	ErrTooManyListMembers = errors.New("timeline: too many list members")
	// ErrNotFollowed reports a member that the owner does not actively follow: only accounts
	// whose follow the owner has and they accepted can join the owner's lists (ADR-0122 §2).
	ErrNotFollowed = errors.New("timeline: the owner does not follow the account")
)

// GraphemeCounter returns how many grapheme clusters s has. The application layer gives one
// that follows Unicode text segmentation; this package uses only the standard library.
type GraphemeCounter func(s string) int

// ValidListName reports whether name is a valid list name: 1 to MaxListNameGraphemes grapheme
// clusters (and at most MaxListNameLength code points) of valid UTF-8 on one line, without
// surrounding white space or control characters.
func ValidListName(name string, graphemes GraphemeCounter) bool {
	if name == "" || strings.TrimSpace(name) != name || !utf8.ValidString(name) ||
		strings.ContainsFunc(name, unicode.IsControl) || utf8.RuneCountInString(name) > MaxListNameLength {
		return false
	}
	return graphemes(name) <= MaxListNameGraphemes
}
