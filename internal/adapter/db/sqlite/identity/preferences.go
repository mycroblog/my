// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if err := insertInteractionDefaults(ctx, q, p.UserID, p.DefaultInteractionPolicy); err != nil {
		return err
	}
	return setTimeZone(ctx, q, p)
}

// ReplaceInteractionDefaults implements identity.PreferencesRepository.
func (r *Repository) ReplaceInteractionDefaults(ctx context.Context, tx uow.Tx, userID string, d identity.InteractionDefaults) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteUserInteractionPolicies(ctx, userID); err != nil {
		return sqlite.Wrap(err, "delete interaction defaults")
	}
	return insertInteractionDefaults(ctx, q, userID, d)
}

// interactionDefaultsFrom returns the defaults of the rows of user_interaction_policies; a kind
// without rows takes identity.DefaultInteractionDefaults.
func interactionDefaultsFrom(rows []identitygen.ListUserInteractionPoliciesRow) identity.InteractionDefaults {
	out := identity.DefaultInteractionDefaults
	for _, k := range identity.InteractionKinds {
		var set identity.InteractionSetting
		for _, row := range rows {
			if identity.InteractionKind(row.Kind) != k {
				continue
			}
			if row.Approval == "manual" {
				set.Manual = identity.InteractionAudience(row.Audience)
			} else {
				set.Automatic = identity.InteractionAudience(row.Audience)
			}
		}
		if set.Automatic != "" {
			out = out.With(k, set)
		}
	}
	return out
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
	policies, err := q.ListUserInteractionPolicies(ctx, row.UserID)
	if err != nil {
		return identity.Preferences{}, sqlite.Wrap(err, "list interaction defaults")
	}
	return identity.Preferences{
		UserID:                   row.UserID,
		DefaultInteractionPolicy: interactionDefaultsFrom(policies),
		TimeZone:                 tz,
		TimeZoneChosen:           chosen,
		Locale:                   row.Locale,
		DefaultVisibility:        identity.Visibility(row.DefaultVisibility),
		DefaultPostLanguage:      row.DefaultPostLanguage,
		RequireMediaDescription:  row.RequireMediaDescription != 0,
		TimelineAutoInsert:       row.TimelineAutoInsert != 0,
		LinkCardSize:             identity.LinkCardSize(row.LinkCardSize),
		LocaleFollowsBrowser:     row.LocaleFollowsBrowser != 0,
		TimeZoneFollowsBrowser:   row.TimeZoneFollowsBrowser != 0,
		UpdatedAt:                sqlite.FromMillis(row.UpdatedAt),
	}, nil
}

// ListTimelineLanguages implements identity.PreferencesRepository.
func (r *Repository) ListTimelineLanguages(ctx context.Context, userID string) ([]string, error) {
	langs, err := r.db.reader().ListUserTimelineLanguages(ctx, userID)
	return langs, sqlite.Wrap(err, "list timeline languages")
}

// insertInteractionDefaults inserts the rows of the defaults d of a user with one statement.
func insertInteractionDefaults(ctx context.Context, q *identitygen.Queries, userID string, d identity.InteractionDefaults) error {
	type row struct {
		Kind     string `json:"kind"`
		Approval string `json:"approval"`
		Audience string `json:"audience"`
	}
	var rows []row
	for _, k := range identity.InteractionKinds {
		set := d.Of(k)
		rows = append(rows, row{Kind: string(k), Approval: "automatic", Audience: string(set.Automatic)})
		if set.Manual != "" {
			rows = append(rows, row{Kind: string(k), Approval: "manual", Audience: string(set.Manual)})
		}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertUserInteractionPolicies(ctx, identitygen.InsertUserInteractionPoliciesParams{UserID: userID, Rules: string(list)}),
		"insert interaction defaults")
}
