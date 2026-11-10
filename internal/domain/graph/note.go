// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxNoteLength is the longest private note on an account, in characters (Unicode code points).
const MaxNoteLength = 2000

// Note is an account's private note on another account (FR-310; account_notes, a current value:
// only the account that wrote it reads it, it keeps no history, and nothing federates). An
// account has at most one note on each other account; writing another replaces it.
type Note struct {
	AccountID       string
	TargetAccountID string
	// Text is the note as its owner wrote it: 1 to MaxNoteLength characters of valid UTF-8.
	Text      string
	UpdatedAt time.Time
}

var (
	// ErrSelfNote reports a note whose account and target are the same.
	ErrSelfNote = errors.New("graph: an account cannot keep a note on itself")
	// ErrInvalidNote reports a note that is empty, longer than MaxNoteLength, not valid UTF-8, or
	// holds a NUL character.
	ErrInvalidNote = errors.New("graph: the note is empty, too long, or not valid text")
)

// Validate checks the invariants of a note.
func (n Note) Validate() error {
	if n.AccountID == "" || n.TargetAccountID == "" {
		return errors.New("graph: a note needs an account and a target")
	}
	if n.AccountID == n.TargetAccountID {
		return ErrSelfNote
	}
	return ValidateNoteText(n.Text)
}

// ValidateNoteText checks the text of a note: 1 to MaxNoteLength characters of valid UTF-8
// without a NUL character.
func ValidateNoteText(text string) error {
	if text == "" || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || utf8.RuneCountInString(text) > MaxNoteLength {
		return ErrInvalidNote
	}
	return nil
}
