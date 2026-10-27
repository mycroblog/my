// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Role badges and their display on profiles (ADR-0071).

// RoleBadge implements identity.RoleBadgeRepository.
func (r *IdentityRepository) RoleBadge(ctx context.Context, tx uow.Tx, roleID string) (identity.RoleBadge, bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.RoleBadge{}, false, err
	}
	row, err := q.GetRoleBadge(ctx, roleID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return identity.RoleBadge{}, false, nil
	case err != nil:
		return identity.RoleBadge{}, false, wrap(err, "get role badge")
	}
	return roleBadgeFrom(row), true, nil
}

// SetRoleBadge implements identity.RoleBadgeRepository.
func (r *IdentityRepository) SetRoleBadge(ctx context.Context, tx uow.Tx, roleID string, b identity.RoleBadge) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertRoleBadge(ctx, postgresgen.UpsertRoleBadgeParams{
		RoleID: roleID, Label: b.Label, Color: string(b.Color), UpdatedAt: b.UpdatedAt,
	}), "set role badge")
}

// DeleteRoleBadge implements identity.RoleBadgeRepository.
func (r *IdentityRepository) DeleteRoleBadge(ctx context.Context, tx uow.Tx, roleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteRoleBadgeDisplays(ctx, roleID); err != nil {
		return wrap(err, "delete role badge displays")
	}
	return wrap(q.DeleteRoleBadge(ctx, roleID), "delete role badge")
}

// RoleBadgeDisplayed implements identity.RoleBadgeRepository.
func (r *IdentityRepository) RoleBadgeDisplayed(ctx context.Context, tx uow.Tx, userID, roleID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.CountUserRoleBadgeDisplays(ctx, postgresgen.CountUserRoleBadgeDisplaysParams{UserID: userID, RoleID: roleID})
	return n > 0, wrap(err, "count role badge displays")
}

// SetRoleBadgeDisplayed implements identity.RoleBadgeRepository.
func (r *IdentityRepository) SetRoleBadgeDisplayed(ctx context.Context, tx uow.Tx, userID, roleID string, displayed bool, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if !displayed {
		return wrap(q.DeleteUserRoleBadgeDisplay(ctx, postgresgen.DeleteUserRoleBadgeDisplayParams{UserID: userID, RoleID: roleID}),
			"delete role badge display")
	}
	return wrap(q.InsertUserRoleBadgeDisplay(ctx, postgresgen.InsertUserRoleBadgeDisplayParams{
		UserID: userID, RoleID: roleID, CreatedAt: at,
	}), "insert role badge display")
}

// ListBadgeChoices implements identity.RoleBadgeRepository.
func (r *IdentityRepository) ListBadgeChoices(ctx context.Context, userID string) ([]identity.BadgeChoice, error) {
	rows, err := r.db.reader().ListUserRoleBadges(ctx, userID)
	if err != nil {
		return nil, wrap(err, "list role badges of user")
	}
	out := make([]identity.BadgeChoice, len(rows))
	for i, row := range rows {
		out[i] = identity.BadgeChoice{
			ProfileBadge: identity.ProfileBadge{RoleID: row.ID, Label: row.Label, Color: identity.BadgeColor(row.Color)},
			Displayed:    row.Displays > 0,
		}
	}
	return out, nil
}

// ListProfileBadges implements identity.RoleBadgeRepository.
func (r *IdentityRepository) ListProfileBadges(ctx context.Context, userID string) ([]identity.ProfileBadge, error) {
	rows, err := r.db.reader().ListDisplayedRoleBadges(ctx, userID)
	if err != nil {
		return nil, wrap(err, "list displayed role badges")
	}
	out := make([]identity.ProfileBadge, len(rows))
	for i, row := range rows {
		out[i] = identity.ProfileBadge{RoleID: row.ID, Label: row.Label, Color: identity.BadgeColor(row.Color)}
	}
	return out, nil
}

// roleBadges returns the badge of every role that has one, by role.
func roleBadges(ctx context.Context, q *postgresgen.Queries) (map[string]*identity.RoleBadge, error) {
	rows, err := q.ListRoleBadges(ctx)
	if err != nil {
		return nil, wrap(err, "list role badges")
	}
	out := make(map[string]*identity.RoleBadge, len(rows))
	for _, row := range rows {
		b := roleBadgeFrom(row)
		out[row.RoleID] = &b
	}
	return out, nil
}

func roleBadgeFrom(row postgresgen.RoleBadge) identity.RoleBadge {
	return identity.RoleBadge{Label: row.Label, Color: identity.BadgeColor(row.Color), UpdatedAt: row.UpdatedAt}
}
