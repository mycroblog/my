// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
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
		RequireMediaDescription: p.RequireMediaDescription,
		TimelineAutoInsert:      p.TimelineAutoInsert,
		LinkCardSize:            string(p.LinkCardSize),
		LocaleFollowsBrowser:    p.LocaleFollowsBrowser,
		TimeZoneFollowsBrowser:  p.TimeZoneFollowsBrowser,
		UpdatedAt:               p.UpdatedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert preferences")
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
		UserID:                  p.UserID,
		Locale:                  p.Locale,
		DefaultVisibility:       string(p.DefaultVisibility),
		DefaultQuotePolicy:      string(p.DefaultQuotePolicy),
		DefaultPostLanguage:     p.DefaultPostLanguage,
		RequireMediaDescription: p.RequireMediaDescription,
		TimelineAutoInsert:      p.TimelineAutoInsert,
		LinkCardSize:            string(p.LinkCardSize),
		LocaleFollowsBrowser:    p.LocaleFollowsBrowser,
		TimeZoneFollowsBrowser:  p.TimeZoneFollowsBrowser,
		UpdatedAt:               p.UpdatedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "update preferences")
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
	return postgres.Wrap(err, "set time zone")
}

// ReplaceTimelineLanguages implements identity.PreferencesRepository.
func (r *Repository) ReplaceTimelineLanguages(ctx context.Context, tx uow.Tx, userID string, languages []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteUserTimelineLanguages(ctx, userID); err != nil {
		return postgres.Wrap(err, "delete timeline languages")
	}
	for _, l := range languages {
		err := q.InsertUserTimelineLanguage(ctx, identitygen.InsertUserTimelineLanguageParams{UserID: userID, Language: l})
		if err != nil {
			return postgres.Wrap(err, "insert timeline language")
		}
	}
	return nil
}

// LockPreferences implements identity.PreferencesRepository with SELECT ... FOR UPDATE on
// user_preferences: every writer of a user's preferences updates that row, so they wait for tx.
func (r *Repository) LockPreferences(ctx context.Context, tx uow.Tx, userID string) (appidentity.UserPreferences, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appidentity.UserPreferences{}, err
	}
	row, err := q.GetUserPreferencesForUpdate(ctx, userID)
	if err != nil {
		return appidentity.UserPreferences{}, postgres.NotFound(err, appidentity.ErrNotFound, "preferences of "+userID)
	}
	p, err := userPreferencesFrom(ctx, q, row)
	if err != nil {
		return appidentity.UserPreferences{}, err
	}
	langs, err := q.ListUserTimelineLanguages(ctx, userID)
	if err != nil {
		return appidentity.UserPreferences{}, postgres.Wrap(err, "list timeline languages")
	}
	return appidentity.UserPreferences{Preferences: p, TimelineLanguages: langs}, nil
}

// FindPreferences implements identity.PreferencesRepository.
func (r *Repository) FindPreferences(ctx context.Context, userID string) (identity.Preferences, error) {
	q := r.db.reader()
	row, err := q.GetUserPreferences(ctx, userID)
	if err != nil {
		return identity.Preferences{}, postgres.NotFound(err, appidentity.ErrNotFound, "preferences of "+userID)
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
	case !errors.Is(err, pgx.ErrNoRows):
		return identity.Preferences{}, postgres.Wrap(err, "get time zone")
	}
	return identity.Preferences{
		UserID:                  row.UserID,
		TimeZone:                tz,
		TimeZoneChosen:          chosen,
		Locale:                  row.Locale,
		DefaultVisibility:       identity.Visibility(row.DefaultVisibility),
		DefaultQuotePolicy:      identity.QuotePolicy(row.DefaultQuotePolicy),
		DefaultPostLanguage:     row.DefaultPostLanguage,
		RequireMediaDescription: row.RequireMediaDescription,
		TimelineAutoInsert:      row.TimelineAutoInsert,
		LinkCardSize:            identity.LinkCardSize(row.LinkCardSize),
		LocaleFollowsBrowser:    row.LocaleFollowsBrowser,
		TimeZoneFollowsBrowser:  row.TimeZoneFollowsBrowser,
		UpdatedAt:               postgres.UTC(row.UpdatedAt),
	}, nil
}

// ListTimelineLanguages implements identity.PreferencesRepository.
func (r *Repository) ListTimelineLanguages(ctx context.Context, userID string) ([]string, error) {
	langs, err := r.db.reader().ListUserTimelineLanguages(ctx, userID)
	return langs, postgres.Wrap(err, "list timeline languages")
}
