// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidText reports whether s is text that every database dialect stores as it is: valid UTF-8
// without NUL. Every local text of the content module passes it before it is
// stored: a status's text and content warning, an alt text, and an Idempotency-Key.
func ValidText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

// MaxIdempotencyKeyLength is the most characters of the Idempotency-Key of a new local status.
const MaxIdempotencyKeyLength = 256

// ErrInvalidIdempotencyKey reports an Idempotency-Key that ValidateIdempotencyKey rejects.
var ErrInvalidIdempotencyKey = errors.New("content: the idempotency key is not valid")

// ValidateIdempotencyKey accepts "" (no key) and valid text of at most MaxIdempotencyKeyLength
// characters.
func ValidateIdempotencyKey(key string) error {
	if !ValidText(key) || utf8.RuneCountInString(key) > MaxIdempotencyKeyLength {
		return fmt.Errorf("%w: must be at most %d characters of text", ErrInvalidIdempotencyKey, MaxIdempotencyKeyLength)
	}
	return nil
}
