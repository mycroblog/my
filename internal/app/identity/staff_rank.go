// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
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

// IsLastAdministratorTx locks the admin role and reports, in tx, whether userID holds it and no other
// holder can sign in: their account is deleted, or moderation keeps them from signing in. Every path
// that removes an administrator (a moderation action, a role change, deleting the account) calls it
// in its transaction before the removal, so such paths run one at a time and each sees what the one
// before it committed. An instance without the built-in roles has no administrator to protect.
func (s *Service) IsLastAdministratorTx(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	role, err := s.d.Repository.LockRole(ctx, tx, identity.RoleAdmin)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	holders, err := s.d.Repository.ListRoleHolderIDsTx(ctx, tx, role.ID)
	if err != nil {
		return false, err
	}
	if !slices.Contains(holders, userID) {
		return false, nil
	}
	for _, holder := range holders {
		if holder == userID {
			continue
		}
		can, err := s.canSignInTx(ctx, tx, holder)
		if err != nil {
			return false, err
		}
		if can {
			return false, nil
		}
	}
	return true, nil
}

// canSignInTx reports, in tx, whether the account of userID is active and moderation does not keep it
// from signing in.
func (s *Service) canSignInTx(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	st, err := s.d.Accounts.UserLifecycleTx(ctx, tx, userID)
	if errors.Is(err, account.ErrNotFound) {
		return false, nil
	}
	if err != nil || !st.Active() {
		return false, err
	}
	if s.d.LoginsTx == nil {
		return true, nil
	}
	restricted, err := s.d.LoginsTx.LoginRestrictedTx(ctx, tx, st.AccountID)
	return !restricted, err
}

// refuseLastAdministratorTx fails with ErrLastAdmin when IsLastAdministratorTx.
func (s *Service) refuseLastAdministratorTx(ctx context.Context, tx uow.Tx, userID string) error {
	last, err := s.IsLastAdministratorTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if last {
		return ErrLastAdmin
	}
	return nil
}
