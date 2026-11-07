// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertPreferences implements identity.PreferencesRepository.
func (r *Repository) InsertPreferences(ctx context.Context, tx uow.Tx, p identity.Preferences) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserPreferences(ctx, identitygen.InsertUserPreferencesParams{
		UserID:                  p.UserID,
		Locale:                  p.Locale,
		DefaultVisibility:       string(p.DefaultVisibility),
		DefaultQuotePolicy:      string(p.DefaultQuotePolicy),
		DefaultPostLanguage:     p.DefaultPostLanguage,
		RequireMediaDescription: sqlite.BoolToInt(p.RequireMediaDescription),
		TimelineAutoInsert:      sqlite.BoolToInt(p.TimelineAutoInsert),
		LinkCardSize:            string(p.LinkCardSize),
		LocaleFollowsBrowser:    sqlite.BoolToInt(p.LocaleFollowsBrowser),
		TimeZoneFollowsBrowser:  sqlite.BoolToInt(p.TimeZoneFollowsBrowser),
		UpdatedAt:               sqlite.ToMillis(p.UpdatedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert preferences")
	}
	return setTimeZone(ctx, q, p)
}

// UpdatePreferences implements identity.PreferencesRepository.
func (r *Repository) UpdatePreferences(ctx context.Context, tx uow.Tx, p identity.Preferences) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.UpdateUserPreferences(ctx, identitygen.UpdateUserPreferencesParams{
		Locale:                  p.Locale,
		DefaultVisibility:       string(p.DefaultVisibility),
		DefaultQuotePolicy:      string(p.DefaultQuotePolicy),
		DefaultPostLanguage:     p.DefaultPostLanguage,
		RequireMediaDescription: sqlite.BoolToInt(p.RequireMediaDescription),
		TimelineAutoInsert:      sqlite.BoolToInt(p.TimelineAutoInsert),
		LinkCardSize:            string(p.LinkCardSize),
		LocaleFollowsBrowser:    sqlite.BoolToInt(p.LocaleFollowsBrowser),
		TimeZoneFollowsBrowser:  sqlite.BoolToInt(p.TimeZoneFollowsBrowser),
		UpdatedAt:               sqlite.ToMillis(p.UpdatedAt),
		UserID:                  p.UserID,
	})
	if err != nil {
		return sqlite.Wrap(err, "update preferences")
	}
	if n == 0 {
		return fmt.Errorf("preferences of %s: %w", p.UserID, appidentity.ErrNotFound)
	}
	return setTimeZone(ctx, q, p)
}

// setTimeZone stores the time zone of p when the user chose one (user_time_zones).
func setTimeZone(ctx context.Context, q *identitygen.Queries, p identity.Preferences) error {
	if !p.TimeZoneChosen {
		return nil
	}
	err := q.UpsertUserTimeZone(ctx, identitygen.UpsertUserTimeZoneParams{UserID: p.UserID, TimeZone: p.TimeZone})
	return sqlite.Wrap(err, "set time zone")
}

// ReplaceTimelineLanguages implements identity.PreferencesRepository.
func (r *Repository) ReplaceTimelineLanguages(ctx context.Context, tx uow.Tx, userID string, languages []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteUserTimelineLanguages(ctx, userID); err != nil {
		return sqlite.Wrap(err, "delete timeline languages")
	}
	for _, l := range languages {
		err := q.InsertUserTimelineLanguage(ctx, identitygen.InsertUserTimelineLanguageParams{UserID: userID, Language: l})
		if err != nil {
			return sqlite.Wrap(err, "insert timeline language")
		}
	}
	return nil
}

// LockPreferences implements identity.PreferencesRepository. SQLite serializes write
// transactions (BEGIN IMMEDIATE), so reading inside tx is enough.
func (r *Repository) LockPreferences(ctx context.Context, tx uow.Tx, userID string) (appidentity.UserPreferences, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appidentity.UserPreferences{}, err
	}
	row, err := q.GetUserPreferences(ctx, userID)
	if err != nil {
		return appidentity.UserPreferences{}, sqlite.NotFound(err, appidentity.ErrNotFound, "preferences of "+userID)
	}
	p, err := userPreferencesFrom(ctx, q, row)
	if err != nil {
		return appidentity.UserPreferences{}, err
	}
	langs, err := q.ListUserTimelineLanguages(ctx, userID)
	if err != nil {
		return appidentity.UserPreferences{}, sqlite.Wrap(err, "list timeline languages")
	}
	return appidentity.UserPreferences{Preferences: p, TimelineLanguages: langs}, nil
}

// FindPreferences implements identity.PreferencesRepository.
func (r *Repository) FindPreferences(ctx context.Context, userID string) (identity.Preferences, error) {
	q := r.db.reader()
	row, err := q.GetUserPreferences(ctx, userID)
	if err != nil {
		return identity.Preferences{}, sqlite.NotFound(err, appidentity.ErrNotFound, "preferences of "+userID)
	}
	return userPreferencesFrom(ctx, q, row)
}

// userPreferencesFrom returns the preferences of row with the time zone the user chose, or
// identity.DefaultTimeZone and TimeZoneChosen false when they have chosen none.
func userPreferencesFrom(ctx context.Context, q *identitygen.Queries, row identitygen.UserPreference) (identity.Preferences, error) {
	tz, chosen := identity.DefaultTimeZone, false
	switch got, err := q.GetUserTimeZone(ctx, row.UserID); {
	case err == nil:
		tz, chosen = got, true
	case !errors.Is(err, sql.ErrNoRows):
		return identity.Preferences{}, sqlite.Wrap(err, "get time zone")
	}
	return identity.Preferences{
		UserID:                  row.UserID,
		TimeZone:                tz,
		TimeZoneChosen:          chosen,
		Locale:                  row.Locale,
		DefaultVisibility:       identity.Visibility(row.DefaultVisibility),
		DefaultQuotePolicy:      identity.QuotePolicy(row.DefaultQuotePolicy),
		DefaultPostLanguage:     row.DefaultPostLanguage,
		RequireMediaDescription: row.RequireMediaDescription != 0,
		TimelineAutoInsert:      row.TimelineAutoInsert != 0,
		LinkCardSize:            identity.LinkCardSize(row.LinkCardSize),
		LocaleFollowsBrowser:    row.LocaleFollowsBrowser != 0,
		TimeZoneFollowsBrowser:  row.TimeZoneFollowsBrowser != 0,
		UpdatedAt:               sqlite.FromMillis(row.UpdatedAt),
	}, nil
}

// ListTimelineLanguages implements identity.PreferencesRepository.
func (r *Repository) ListTimelineLanguages(ctx context.Context, userID string) ([]string, error) {
	langs, err := r.db.reader().ListUserTimelineLanguages(ctx, userID)
	return langs, sqlite.Wrap(err, "list timeline languages")
}
