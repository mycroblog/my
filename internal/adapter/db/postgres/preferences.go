// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertPreferences implements identity.PreferencesRepository.
func (r *IdentityRepository) InsertPreferences(ctx context.Context, tx uow.Tx, p identity.Preferences) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserPreferences(ctx, postgresgen.InsertUserPreferencesParams{
		UserID:                      p.UserID,
		Locale:                      p.Locale,
		DefaultVisibility:           string(p.DefaultVisibility),
		DefaultQuotePolicy:          string(p.DefaultQuotePolicy),
		DefaultPostLanguage:         p.DefaultPostLanguage,
		RequireMediaDescription:     p.RequireMediaDescription,
		EmailModeratorNotifications: p.EmailModeratorNotifications,
		TimelineAutoInsert:          p.TimelineAutoInsert,
		UpdatedAt:                   p.UpdatedAt,
	})
	if err != nil {
		return wrap(err, "insert preferences")
	}
	return setTimeZone(ctx, q, p)
}

// UpdatePreferences implements identity.PreferencesRepository.
func (r *IdentityRepository) UpdatePreferences(ctx context.Context, tx uow.Tx, p identity.Preferences) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.UpdateUserPreferences(ctx, postgresgen.UpdateUserPreferencesParams{
		UserID:                      p.UserID,
		Locale:                      p.Locale,
		DefaultVisibility:           string(p.DefaultVisibility),
		DefaultQuotePolicy:          string(p.DefaultQuotePolicy),
		DefaultPostLanguage:         p.DefaultPostLanguage,
		RequireMediaDescription:     p.RequireMediaDescription,
		EmailModeratorNotifications: p.EmailModeratorNotifications,
		TimelineAutoInsert:          p.TimelineAutoInsert,
		UpdatedAt:                   p.UpdatedAt,
	})
	if err != nil {
		return wrap(err, "update preferences")
	}
	if n == 0 {
		return fmt.Errorf("preferences of %s: %w", p.UserID, appidentity.ErrNotFound)
	}
	return setTimeZone(ctx, q, p)
}

// setTimeZone stores the time zone of p when the user chose one (user_time_zones).
func setTimeZone(ctx context.Context, q *postgresgen.Queries, p identity.Preferences) error {
	if !p.TimeZoneChosen {
		return nil
	}
	err := q.UpsertUserTimeZone(ctx, postgresgen.UpsertUserTimeZoneParams{UserID: p.UserID, TimeZone: p.TimeZone})
	return wrap(err, "set time zone")
}

// ReplaceTimelineLanguages implements identity.PreferencesRepository.
func (r *IdentityRepository) ReplaceTimelineLanguages(ctx context.Context, tx uow.Tx, userID string, languages []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteUserTimelineLanguages(ctx, userID); err != nil {
		return wrap(err, "delete timeline languages")
	}
	for _, l := range languages {
		err := q.InsertUserTimelineLanguage(ctx, postgresgen.InsertUserTimelineLanguageParams{UserID: userID, Language: l})
		if err != nil {
			return wrap(err, "insert timeline language")
		}
	}
	return nil
}

// LockPreferences implements identity.PreferencesRepository with SELECT ... FOR UPDATE on
// user_preferences: every writer of a user's preferences updates that row, so they wait for tx.
func (r *IdentityRepository) LockPreferences(ctx context.Context, tx uow.Tx, userID string) (appidentity.UserPreferences, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appidentity.UserPreferences{}, err
	}
	row, err := q.GetUserPreferencesForUpdate(ctx, userID)
	if err != nil {
		return appidentity.UserPreferences{}, notFound(err, appidentity.ErrNotFound, "preferences of "+userID)
	}
	p, err := userPreferencesFrom(ctx, q, row)
	if err != nil {
		return appidentity.UserPreferences{}, err
	}
	langs, err := q.ListUserTimelineLanguages(ctx, userID)
	if err != nil {
		return appidentity.UserPreferences{}, wrap(err, "list timeline languages")
	}
	return appidentity.UserPreferences{Preferences: p, TimelineLanguages: langs}, nil
}

// FindPreferences implements identity.PreferencesRepository.
func (r *IdentityRepository) FindPreferences(ctx context.Context, userID string) (identity.Preferences, error) {
	q := r.db.reader()
	row, err := q.GetUserPreferences(ctx, userID)
	if err != nil {
		return identity.Preferences{}, notFound(err, appidentity.ErrNotFound, "preferences of "+userID)
	}
	return userPreferencesFrom(ctx, q, row)
}

// userPreferencesFrom returns the preferences of row with the time zone the user chose, or
// identity.DefaultTimeZone and TimeZoneChosen false when they have chosen none.
func userPreferencesFrom(ctx context.Context, q *postgresgen.Queries, row postgresgen.UserPreference) (identity.Preferences, error) {
	tz, chosen := identity.DefaultTimeZone, false
	switch got, err := q.GetUserTimeZone(ctx, row.UserID); {
	case err == nil:
		tz, chosen = got, true
	case !errors.Is(err, pgx.ErrNoRows):
		return identity.Preferences{}, wrap(err, "get time zone")
	}
	return identity.Preferences{
		UserID:                      row.UserID,
		TimeZone:                    tz,
		TimeZoneChosen:              chosen,
		Locale:                      row.Locale,
		DefaultVisibility:           identity.Visibility(row.DefaultVisibility),
		DefaultQuotePolicy:          identity.QuotePolicy(row.DefaultQuotePolicy),
		DefaultPostLanguage:         row.DefaultPostLanguage,
		RequireMediaDescription:     row.RequireMediaDescription,
		EmailModeratorNotifications: row.EmailModeratorNotifications,
		TimelineAutoInsert:          row.TimelineAutoInsert,
		UpdatedAt:                   utc(row.UpdatedAt),
	}, nil
}

// ListTimelineLanguages implements identity.PreferencesRepository.
func (r *IdentityRepository) ListTimelineLanguages(ctx context.Context, userID string) ([]string, error) {
	langs, err := r.db.reader().ListUserTimelineLanguages(ctx, userID)
	return langs, wrap(err, "list timeline languages")
}
