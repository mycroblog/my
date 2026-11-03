// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"

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

// otherAdministrators returns the holders of the admin role other than userID whom moderation does
// not keep from signing in, and whether userID holds the role. An instance without the built-in
// roles has no administrator to protect.
func (s *Service) otherAdministrators(ctx context.Context, userID string) (others []string, holds bool, err error) {
	holders, err := s.RoleHolders(ctx, identity.RoleAdmin)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	for _, holder := range holders {
		if holder == userID {
			holds = true
			continue
		}
		la, err := s.d.Accounts.LocalAccountOfUser(ctx, holder)
		if errors.Is(err, account.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		restricted, err := s.loginRestricted(ctx, la.AccountID)
		if err != nil {
			return nil, false, err
		}
		if !restricted {
			others = append(others, holder)
		}
	}
	return others, holds, nil
}

// IsLastAdministrator reports whether userID holds the admin role and no other holder can sign in:
// their account is deleted or moderation keeps them from signing in, so that the instance has no
// administrator left without userID.
func (s *Service) IsLastAdministrator(ctx context.Context, userID string) (bool, error) {
	others, holds, err := s.otherAdministrators(ctx, userID)
	if err != nil || !holds {
		return false, err
	}
	for _, holder := range others {
		st, err := s.d.Accounts.UserLifecycle(ctx, holder)
		if err != nil && !errors.Is(err, account.ErrNotFound) {
			return false, err
		}
		if err == nil && st.Active() {
			return false, nil
		}
	}
	return true, nil
}

// guardLastAdministrator returns the guard of the deletion of userID's account: it locks the admin
// role, so that two administrators who delete their accounts at once take turns and the second one
// sees the first one's deletion, and fails with ErrLastAdmin when no other administrator is left.
func (s *Service) guardLastAdministrator(ctx context.Context, userID string) (func(tx uow.Tx) error, error) {
	others, holds, err := s.otherAdministrators(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !holds {
		return nil, nil
	}
	return func(tx uow.Tx) error {
		if _, err := s.d.Repository.LockRole(ctx, tx, identity.RoleAdmin); err != nil {
			return err
		}
		for _, holder := range others {
			st, err := s.d.Accounts.UserLifecycleTx(ctx, tx, holder)
			if err != nil && !errors.Is(err, account.ErrNotFound) {
				return err
			}
			if err == nil && st.Active() {
				return nil
			}
		}
		return ErrLastAdmin
	}, nil
}
