// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"fmt"
	"slices"
	"time"

	"golang.org/x/text/language"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// PreferencesRepository is the persistence of user preferences (data-model §4.1,
// user_preferences and user_timeline_languages, current values). It is part of Repository.
type PreferencesRepository interface {
	// InsertPreferences stores the preferences of a user who has none yet.
	InsertPreferences(ctx context.Context, tx uow.Tx, p identity.Preferences) error
	// UpdatePreferences replaces the stored preferences of p.UserID, and stores p.TimeZone as
	// the user's choice when p.TimeZoneChosen. It fails with an error wrapping ErrNotFound when
	// the user has none.
	UpdatePreferences(ctx context.Context, tx uow.Tx, p identity.Preferences) error
	// ReplaceTimelineLanguages makes languages the user's timeline languages; an empty list
	// removes them all.
	ReplaceTimelineLanguages(ctx context.Context, tx uow.Tx, userID string, languages []string) error

	// LockPreferences reads the preferences and timeline languages of a user inside tx, which
	// changes them, and keeps other writers of them waiting until tx ends, so that a
	// read-modify-write of them loses no concurrent change. It fails with an error wrapping
	// ErrNotFound when the user has none.
	LockPreferences(ctx context.Context, tx uow.Tx, userID string) (UserPreferences, error)
	// FindPreferences returns the preferences of a user, with identity.DefaultTimeZone and
	// TimeZoneChosen false when the user has chosen no time zone.
	FindPreferences(ctx context.Context, userID string) (identity.Preferences, error)
	// ListTimelineLanguages returns the timeline languages of a user, sorted; none means all.
	ListTimelineLanguages(ctx context.Context, userID string) ([]string, error)
}

// UserPreferences are a user's preferences with the languages their timelines show.
type UserPreferences struct {
	identity.Preferences
	// TimelineLanguages are the languages timelines show, sorted; empty shows every language (FR-208).
	TimelineLanguages []string
}

// PreferencesInput are the settable preferences of SetPreferences; every field is required.
type PreferencesInput struct {
	Locale            string
	TimeZone          string
	DefaultVisibility identity.Visibility
	// DefaultQuotePolicy is who may quote new posts; "" keeps the current one.
	DefaultQuotePolicy      identity.QuotePolicy
	DefaultPostLanguage     string
	RequireMediaDescription bool
	// TimelineAutoInsert inserts new timeline posts without asking (ADR-0077).
	TimelineAutoInsert bool
}

// Preferences returns the preferences of a user. It fails with an error wrapping ErrNotFound
// when the user has none.
func (s *Service) Preferences(ctx context.Context, userID string) (UserPreferences, error) {
	p, err := s.d.Repository.FindPreferences(ctx, userID)
	if err != nil {
		return UserPreferences{}, err
	}
	langs, err := s.d.Repository.ListTimelineLanguages(ctx, userID)
	if err != nil {
		return UserPreferences{}, err
	}
	slices.Sort(langs) // byte order, whatever the database's collation
	return UserPreferences{Preferences: p, TimelineLanguages: langs}, nil
}

// SetPreferences validates in, canonicalizes its language tags, and replaces the preferences of
// a user. It fails with errors wrapping identity.ErrInvalidLanguageTag,
// identity.ErrInvalidTimeZone, or identity.ErrInvalidVisibility for invalid input, and with
// ErrNotFound when the user has no preferences.
func (s *Service) SetPreferences(ctx context.Context, userID string, in PreferencesInput) (identity.Preferences, error) {
	locale, err := CanonicalLocale(in.Locale)
	if err != nil {
		return identity.Preferences{}, err
	}
	postLang, err := CanonicalLanguage(in.DefaultPostLanguage)
	if err != nil {
		return identity.Preferences{}, err
	}
	if err := identity.ValidateTimeZone(in.TimeZone); err != nil {
		return identity.Preferences{}, err
	}
	if err := identity.ValidateVisibility(in.DefaultVisibility); err != nil {
		return identity.Preferences{}, err
	}
	quotePolicy := in.DefaultQuotePolicy
	if quotePolicy == "" {
		cur, err := s.d.Repository.FindPreferences(ctx, userID)
		if err != nil {
			return identity.Preferences{}, err
		}
		quotePolicy = cur.DefaultQuotePolicy
	}
	if err := identity.ValidateQuotePolicy(quotePolicy); err != nil {
		return identity.Preferences{}, err
	}
	p := identity.Preferences{
		UserID:                  userID,
		Locale:                  locale,
		TimeZone:                in.TimeZone,
		TimeZoneChosen:          true,
		DefaultVisibility:       in.DefaultVisibility,
		DefaultQuotePolicy:      quotePolicy,
		DefaultPostLanguage:     postLang,
		RequireMediaDescription: in.RequireMediaDescription,
		TimelineAutoInsert:      in.TimelineAutoInsert,
		UpdatedAt:               s.d.Clock.Now(),
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.UpdatePreferences(ctx, tx, p) })
	if err != nil {
		return identity.Preferences{}, err
	}
	return p, nil
}

// SetTimelineLanguages replaces the languages a user's timelines show with languages,
// canonicalized, deduplicated, and sorted, and returns them; an empty list shows every language
// (FR-208). It fails with errors wrapping identity.ErrInvalidLanguageTag or
// identity.ErrTooManyLanguages for invalid input, and with ErrNotFound when the user has no
// preferences.
func (s *Service) SetTimelineLanguages(ctx context.Context, userID string, languages []string) ([]string, error) {
	out, err := canonicalTimelineLanguages(languages)
	if err != nil {
		return nil, err
	}
	if _, err := s.d.Repository.FindPreferences(ctx, userID); err != nil {
		return nil, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.ReplaceTimelineLanguages(ctx, tx, userID, out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PreferencesUpdate is a change of a user's preferences; nil fields keep their current value.
type PreferencesUpdate struct {
	Locale                  *string
	TimeZone                *string
	DefaultVisibility       *identity.Visibility
	DefaultQuotePolicy      *identity.QuotePolicy
	DefaultPostLanguage     *string
	RequireMediaDescription *bool
	// TimelineAutoInsert is the preference of that name (ADR-0077).
	TimelineAutoInsert *bool
	// TimelineLanguages replace the languages timelines show; empty shows every language.
	TimelineLanguages *[]string
}

// PreferenceError is an invalid value of one preference in a PreferencesUpdate.
type PreferenceError struct {
	// Preference is the column of user_preferences, or timeline_languages.
	Preference string
	// Err wraps one of identity.ErrInvalidLanguageTag, identity.ErrInvalidTimeZone,
	// identity.ErrInvalidVisibility, identity.ErrInvalidQuotePolicy, and
	// identity.ErrTooManyLanguages.
	Err error
}

func (e *PreferenceError) Error() string { return e.Preference + ": " + e.Err.Error() }

// Unwrap returns Err.
func (e *PreferenceError) Unwrap() error { return e.Err }

// UpdatePreferences applies a change to the preferences of a user in one transaction and
// returns the result. Language tags are canonicalized and timeline languages deduplicated and
// sorted. The current preferences are read and locked in that transaction, so concurrent changes
// of different preferences (a time zone the client adopts while the user picks a locale) all
// stay. Invalid input fails with a *PreferenceError, and a user without preferences with
// ErrNotFound.
func (s *Service) UpdatePreferences(ctx context.Context, userID string, in PreferencesUpdate) (UserPreferences, error) {
	in, err := canonicalPreferencesUpdate(in)
	if err != nil {
		return UserPreferences{}, err
	}
	var out UserPreferences
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.LockPreferences(ctx, tx, userID)
		if err != nil {
			return err
		}
		out = in.applyTo(cur)
		out.UpdatedAt = s.d.Clock.Now()
		if err := s.d.Repository.UpdatePreferences(ctx, tx, out.Preferences); err != nil {
			return err
		}
		if in.TimelineLanguages == nil {
			return nil
		}
		return s.d.Repository.ReplaceTimelineLanguages(ctx, tx, userID, out.TimelineLanguages)
	})
	if err != nil {
		return UserPreferences{}, err
	}
	return out, nil
}

// canonicalPreferencesUpdate validates the values of in and returns them canonicalized, or a
// *PreferenceError naming the first invalid one.
func canonicalPreferencesUpdate(in PreferencesUpdate) (PreferencesUpdate, error) {
	if in.Locale != nil {
		locale, err := CanonicalLocale(*in.Locale)
		if err != nil {
			return PreferencesUpdate{}, &PreferenceError{Preference: "locale", Err: err}
		}
		in.Locale = &locale
	}
	if in.TimeZone != nil {
		if err := identity.ValidateTimeZone(*in.TimeZone); err != nil {
			return PreferencesUpdate{}, &PreferenceError{Preference: "time_zone", Err: err}
		}
	}
	if in.DefaultVisibility != nil {
		if err := identity.ValidateVisibility(*in.DefaultVisibility); err != nil {
			return PreferencesUpdate{}, &PreferenceError{Preference: "default_visibility", Err: err}
		}
	}
	if in.DefaultQuotePolicy != nil {
		if err := identity.ValidateQuotePolicy(*in.DefaultQuotePolicy); err != nil {
			return PreferencesUpdate{}, &PreferenceError{Preference: "default_quote_policy", Err: err}
		}
	}
	if in.DefaultPostLanguage != nil {
		lang, err := CanonicalLanguage(*in.DefaultPostLanguage)
		if err != nil {
			return PreferencesUpdate{}, &PreferenceError{Preference: "default_post_language", Err: err}
		}
		in.DefaultPostLanguage = &lang
	}
	if in.TimelineLanguages != nil {
		langs, err := canonicalTimelineLanguages(*in.TimelineLanguages)
		if err != nil {
			return PreferencesUpdate{}, &PreferenceError{Preference: "timeline_languages", Err: err}
		}
		in.TimelineLanguages = &langs
	}
	return in, nil
}

// applyTo returns cur with the values of the canonical update in in place of its own.
func (in PreferencesUpdate) applyTo(cur UserPreferences) UserPreferences {
	p := cur
	if in.Locale != nil {
		p.Locale = *in.Locale
	}
	if in.TimeZone != nil {
		p.TimeZone, p.TimeZoneChosen = *in.TimeZone, true
	}
	if in.DefaultVisibility != nil {
		p.DefaultVisibility = *in.DefaultVisibility
	}
	if in.DefaultQuotePolicy != nil {
		p.DefaultQuotePolicy = *in.DefaultQuotePolicy
	}
	if in.DefaultPostLanguage != nil {
		p.DefaultPostLanguage = *in.DefaultPostLanguage
	}
	if in.RequireMediaDescription != nil {
		p.RequireMediaDescription = *in.RequireMediaDescription
	}
	if in.TimelineAutoInsert != nil {
		p.TimelineAutoInsert = *in.TimelineAutoInsert
	}
	if in.TimelineLanguages != nil {
		p.TimelineLanguages = *in.TimelineLanguages
	}
	return p
}

// canonicalTimelineLanguages canonicalizes, deduplicates, and sorts timeline languages.
func canonicalTimelineLanguages(languages []string) ([]string, error) {
	if len(languages) > identity.MaxTimelineLanguageCount {
		return nil, fmt.Errorf("%w: at most %d", identity.ErrTooManyLanguages, identity.MaxTimelineLanguageCount)
	}
	out := make([]string, 0, len(languages))
	for _, l := range languages {
		c, err := CanonicalLanguage(l)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// CanonicalLanguage returns the canonical form of a BCP 47 language tag ("ja-jp" -> "ja-JP",
// "iw" -> "he"). It accepts identity.UndeterminedLanguage and rejects tags that are malformed,
// that use unknown subtags, or that name no language explicitly (such as "und-JP" or private-use "x-foo"), with
// an error wrapping identity.ErrInvalidLanguageTag.
func CanonicalLanguage(tag string) (string, error) {
	if len(tag) > identity.MaxLanguageTagLength {
		return "", fmt.Errorf("%w: longer than %d bytes", identity.ErrInvalidLanguageTag, identity.MaxLanguageTagLength)
	}
	t, err := language.Parse(tag)
	if err != nil {
		return "", fmt.Errorf("%w: %q", identity.ErrInvalidLanguageTag, tag)
	}
	c := t.String()
	// Base guesses a language for "und-JP" or "x-foo"; only an explicit one is Exact.
	if _, conf := t.Base(); conf != language.Exact && c != identity.UndeterminedLanguage {
		return "", fmt.Errorf("%w: %q has no language subtag", identity.ErrInvalidLanguageTag, tag)
	}
	return c, nil
}

// CanonicalLocale is CanonicalLanguage for a UI locale, which also rejects
// identity.UndeterminedLanguage.
func CanonicalLocale(tag string) (string, error) {
	c, err := CanonicalLanguage(tag)
	if err != nil {
		return "", err
	}
	if c == identity.UndeterminedLanguage {
		return "", fmt.Errorf("%w: a locale must name a language", identity.ErrInvalidLanguageTag)
	}
	return c, nil
}

// defaultPreferences returns the preferences of a new user: the registration's locale or else
// the instance default, no time zone chosen (UTC), and public posts in an undetermined language.
func (s *Service) defaultPreferences(ctx context.Context, userID, locale string, now time.Time) (identity.Preferences, error) {
	if locale == "" && s.d.DefaultLocale != nil {
		var err error
		if locale, err = s.d.DefaultLocale(ctx); err != nil {
			return identity.Preferences{}, fmt.Errorf("instance default locale: %w", err)
		}
	}
	if locale == "" {
		locale = i18n.SourceLocale
	}
	locale, err := CanonicalLocale(locale)
	if err != nil {
		return identity.Preferences{}, err
	}
	return identity.Preferences{
		UserID:              userID,
		Locale:              locale,
		TimeZone:            identity.DefaultTimeZone,
		DefaultVisibility:   identity.DefaultPostVisibility,
		DefaultQuotePolicy:  identity.DefaultPostQuotePolicy,
		DefaultPostLanguage: identity.UndeterminedLanguage,
		UpdatedAt:           now,
	}, nil
}
