// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"time"
)

// Preferences are a user's settings (data-model §4.1, user_preferences; FR-007). Language tags
// are canonical BCP 47 tags and TimeZone is an IANA time zone name.
type Preferences struct {
	UserID string
	// Locale is the UI language. It is never UndeterminedLanguage.
	Locale string
	// TimeZone is the zone dates are shown in, such as "Asia/Tokyo" or "UTC"; DefaultTimeZone
	// until the user chooses one.
	TimeZone string
	// TimeZoneChosen is whether the user (or a client for them) has chosen TimeZone. A new user
	// has not, and the web client then offers the browser's zone (data-model §4.1).
	TimeZoneChosen bool
	// DefaultVisibility is the visibility a new post starts with.
	DefaultVisibility Visibility
	// DefaultQuotePolicy is who may quote a new post (ADR-0069 §1).
	DefaultQuotePolicy QuotePolicy
	// DefaultPostLanguage is the language a new post starts with; UndeterminedLanguage leaves it
	// to the client.
	DefaultPostLanguage string
	// RequireMediaDescription makes clients ask for a description before attaching media.
	RequireMediaDescription bool
	// TimelineAutoInsert inserts new timeline posts without asking (ADR-0077);
	// DefaultTimelineAutoInsert for a new user (ADR-0127 §6).
	TimelineAutoInsert bool
	UpdatedAt          time.Time
}

// Defaults of a new user's preferences (data-model §4.1). The locale comes from the registration
// or the instance default.
const (
	DefaultTimeZone        = "UTC"
	DefaultPostVisibility  = VisibilityPublic
	DefaultPostQuotePolicy = QuotePolicyPublic
	// DefaultTimelineAutoInsert inserts new timeline posts without asking (ADR-0127 §6).
	DefaultTimelineAutoInsert = true
	UndeterminedLanguage      = "und" // BCP 47 "undetermined" (data-model §1.2, lang)
	MaxLanguageTagLength      = 64    // longer tags are rejected before parsing
	MaxTimeZoneLength         = 64
	MaxTimelineLanguageCount  = 64
)

// Visibility is the audience of a post (ADR-0017). Preferences use it for the default of new posts.
type Visibility string

// The visibility levels, widest first.
const (
	VisibilityPublic   Visibility = "public"
	VisibilityLocal    Visibility = "local"
	VisibilityUnlisted Visibility = "unlisted"
	VisibilityPrivate  Visibility = "private"
	VisibilityDirect   Visibility = "direct"
)

// QuotePolicy is who may quote a post (ADR-0069 §1). Preferences use it for the default of new
// posts.
type QuotePolicy string

// The quote policies.
const (
	QuotePolicyPublic    QuotePolicy = "public"
	QuotePolicyFollowers QuotePolicy = "followers"
	QuotePolicyNobody    QuotePolicy = "nobody"
)

// Errors that preference validation reports.
var (
	ErrInvalidLanguageTag = errors.New("invalid language tag")
	ErrInvalidTimeZone    = errors.New("invalid time zone")
	ErrInvalidVisibility  = errors.New("invalid visibility")
	ErrInvalidQuotePolicy = errors.New("invalid quote policy")
	ErrTooManyLanguages   = errors.New("too many timeline languages")
)

// ValidateVisibility accepts the five visibility levels.
func ValidateVisibility(v Visibility) error {
	switch v {
	case VisibilityPublic, VisibilityLocal, VisibilityUnlisted, VisibilityPrivate, VisibilityDirect:
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidVisibility, v)
}

// ValidateQuotePolicy accepts the three quote policies.
func ValidateQuotePolicy(p QuotePolicy) error {
	switch p {
	case QuotePolicyPublic, QuotePolicyFollowers, QuotePolicyNobody:
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidQuotePolicy, p)
}

// ValidateTimeZone accepts an IANA time zone name that the time zone database knows, such as
// "Asia/Tokyo" or "UTC". It rejects "" and "Local", which time.LoadLocation maps to UTC and to
// the host's zone. The server binary embeds the database (time/tzdata), so the result does not
// depend on the host.
func ValidateTimeZone(name string) error {
	if name == "" || name == "Local" || len(name) > MaxTimeZoneLength {
		return fmt.Errorf("%w: %q", ErrInvalidTimeZone, name)
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidTimeZone, name)
	}
	return nil
}
