// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TextError reports a text that a moderation record cannot hold: one that is not valid UTF-8
// without NUL, or one longer than Max characters. The records' Validate
// methods return it wrapped together with their own sentinel error, such as ErrInvalidAction.
type TextError struct {
	// Field is the record's column, such as public_comment or reason.
	Field string
	// Max is the limit in characters that the text exceeds; 0 when the text is not valid text.
	Max int
}

func (e *TextError) Error() string {
	if e.Max > 0 {
		return fmt.Sprintf("the %s is longer than %d characters", e.Field, e.Max)
	}
	return fmt.Sprintf("the %s is not valid UTF-8 without NUL", e.Field)
}

// validText reports whether s is text that every database dialect stores as it is.
func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

// checkText checks the text of field: valid text of at most maxLen characters.
func checkText(field, s string, maxLen int) error {
	if !validText(s) {
		return &TextError{Field: field}
	}
	if utf8.RuneCountInString(s) > maxLen {
		return &TextError{Field: field, Max: maxLen}
	}
	return nil
}

// textField is one text of an action or a lift and its column.
type textField struct{ field, text string }

// checkTexts checks the texts of an action or a lift against MaxActionText, in order, and
// returns the first *TextError wrapped with ErrInvalidAction.
func checkTexts(fields ...textField) error {
	for _, f := range fields {
		if err := checkText(f.field, f.text, MaxActionText); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidAction, err)
		}
	}
	return nil
}
