// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ErrTargetOutranksActor is identity.ErrTargetOutranksActor.
var ErrTargetOutranksActor = identity.ErrTargetOutranksActor

// mayActOn reports whether the user byUserID may moderate or recover the account of the user
// targetUserID: the CLI ("") may do anything, and anyone else needs to rank above the target
// (identity.RanksAbove), so no one acts on their own account through it.
func mayActOn(ctx context.Context, repo Repository, byUserID, targetUserID string) (bool, error) {
	if byUserID == "" {
		return true, nil
	}
	actor, err := repo.ListUserRoles(ctx, byUserID)
	if err != nil {
		return false, err
	}
	target, err := repo.ListUserRoles(ctx, targetUserID)
	if err != nil {
		return false, err
	}
	return identity.RanksAbove(actor, target), nil
}

// checkActOn fails with ErrTargetOutranksActor unless mayActOn.
func checkActOn(ctx context.Context, repo Repository, byUserID, targetUserID string) error {
	ok, err := mayActOn(ctx, repo, byUserID, targetUserID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: user %s", ErrTargetOutranksActor, targetUserID)
	}
	return nil
}

// MayActOn reports whether the user byUserID ("" for the CLI) may moderate the account of the
// user targetUserID. The moderation module asks it before it acts on a local account.
func (s *Service) MayActOn(ctx context.Context, byUserID, targetUserID string) (bool, error) {
	return mayActOn(ctx, s.d.Repository, byUserID, targetUserID)
}

// refuseLastAdministrator fails with ErrLastAdmin when taking the role with roleKey from userID
// would leave no active administrator. The count inside the transaction of a revocation guards
// against a concurrent one; this adds the holders who have left, whom that count includes.
func (s *Service) refuseLastAdministrator(ctx context.Context, roleKey, userID string) error {
	if roleKey != identity.RoleAdmin {
		return nil
	}
	last, err := s.IsLastAdministrator(ctx, userID)
	if err != nil {
		return err
	}
	if last {
		return ErrLastAdmin
	}
	return nil
}

// IsLastAdministrator reports whether userID holds the admin role and no other holder has an
// active account, so that the instance has no administrator left without them.
func (s *Service) IsLastAdministrator(ctx context.Context, userID string) (bool, error) {
	holders, err := s.RoleHolders(ctx, identity.RoleAdmin)
	if errors.Is(err, ErrNotFound) {
		return false, nil // an instance without the built-in roles has no administrator to protect
	}
	if err != nil {
		return false, err
	}
	last := false
	for _, holder := range holders {
		if holder == userID {
			last = true
			continue
		}
		st, err := s.d.Accounts.UserLifecycle(ctx, holder)
		if err != nil && !errors.Is(err, account.ErrNotFound) {
			return false, err
		}
		if err == nil && st.Active() {
			return false, nil
		}
	}
	return last, nil
}
