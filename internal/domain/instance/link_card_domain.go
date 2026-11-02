// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// LinkCardDomainMode is what the link card crawler does not do on a domain and its subdomains.
type LinkCardDomainMode string

const (
	// LinkCardNoFetch fetches neither pages nor images, and hides the cards of its pages.
	LinkCardNoFetch LinkCardDomainMode = "no_fetch"
	// LinkCardNoImages fetches pages but no images, and shows its cards without their images.
	LinkCardNoImages LinkCardDomainMode = "no_images"
)

// Valid reports whether m is a known mode.
func (m LinkCardDomainMode) Valid() bool { return m == LinkCardNoFetch || m == LinkCardNoImages }

// MaxLinkCardDomainNote is the longest note of a rule, in characters.
const MaxLinkCardDomainNote = 5000

// MaxLinkCardDomainRules is the number of rules a server keeps; the list is not paginated.
const MaxLinkCardDomainRules = 1000

// ErrTooManyLinkCardDomainRules reports a new rule beyond MaxLinkCardDomainRules.
var ErrTooManyLinkCardDomainRules = errors.New("instance: too many link card domain rules")

// LinkCardDomainRule is the rule of the link card crawler for a domain and its subdomains. It
// bears only on link cards: federation and other remote media follow the domain actions.
type LinkCardDomainRule struct {
	// Domain is the canonical host, the key of the rule.
	Domain string
	Mode   LinkCardDomainMode
	// Note is what the administrators keep about the rule; "" for none. It is never shown to
	// anyone else.
	Note      string
	UpdatedAt time.Time
}

// LinkCardDomainError reports a rule that cannot be kept. It wraps ErrInvalidValue.
type LinkCardDomainError struct {
	// Field is "domain", "mode", or "note".
	Field string
	// Max is the limit that the value exceeds, or 0 when it is malformed.
	Max int
}

func (e *LinkCardDomainError) Error() string {
	return fmt.Sprintf("link card domain rule: invalid %s", e.Field)
}

// Unwrap makes errors.Is(err, ErrInvalidValue) hold.
func (e *LinkCardDomainError) Unwrap() error { return ErrInvalidValue }

// Validate checks the mode and the note of the rule; callers canonicalize and check the domain.
func (r LinkCardDomainRule) Validate() error {
	if !r.Mode.Valid() {
		return &LinkCardDomainError{Field: "mode"}
	}
	if !utf8.ValidString(r.Note) || strings.ContainsRune(r.Note, 0) {
		return &LinkCardDomainError{Field: "note"}
	}
	if utf8.RuneCountInString(r.Note) > MaxLinkCardDomainNote {
		return &LinkCardDomainError{Field: "note", Max: MaxLinkCardDomainNote}
	}
	return nil
}

// LinkCardCrawl is what the rules that cover a host stop.
type LinkCardCrawl struct {
	// NoFetch stops fetching from the host at all.
	NoFetch bool
	// NoImages stops fetching images from the host; NoFetch implies it.
	NoImages bool
}

// LinkCardCrawlOf returns what the modes of the rules that cover a host stop: the rule of the
// host and those of its parents all apply, as the domain actions do, so that a rule on a
// subdomain never loosens a rule on its parent.
func LinkCardCrawlOf(modes []LinkCardDomainMode) LinkCardCrawl {
	noFetch := slices.Contains(modes, LinkCardNoFetch)
	return LinkCardCrawl{NoFetch: noFetch, NoImages: noFetch || slices.Contains(modes, LinkCardNoImages)}
}
