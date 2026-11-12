// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// EmailAddress is an email address a user added (user_email_addresses).
type EmailAddress struct {
	ID                string
	UserID            string
	Address           string
	AddressNormalized string
	AddedAt           time.Time
}

// MaxEmailLength is the longest email address accepted, in bytes (string(320)).
const MaxEmailLength = 320

// ErrInvalidEmail reports an email address that ValidateEmail rejects.
var ErrInvalidEmail = errors.New("invalid email address")

// ValidateEmail accepts a bare addr-spec (local@domain) of at most MaxEmailLength bytes of valid
// UTF-8 without NUL, without a display name, angle brackets, comments, or
// surrounding spaces.
func ValidateEmail(address string) error {
	if address == "" || len(address) > MaxEmailLength || !validText(address, MaxEmailLength) {
		return fmt.Errorf("%w: must be 1 to %d bytes of UTF-8 without NUL", ErrInvalidEmail, MaxEmailLength)
	}
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Name != "" || parsed.Address != address {
		return fmt.Errorf("%w: must be a plain address such as alice@example.com", ErrInvalidEmail)
	}
	return nil
}

// NormalizeEmail returns the form of an address that uniqueness and login lookups compare:
// the whole address in lower case. Providers that treat the local part case-sensitively are
// rare, and two users who differ only in case would be indistinguishable to people anyway.
func NormalizeEmail(address string) string {
	return strings.ToLower(address)
}
