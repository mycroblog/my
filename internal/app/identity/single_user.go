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

// The one-person server (FR-811, ADR-0152): the instance setting single_user, read through
// Deps.SingleUser and Deps.SingleUserTx, keeps every way of creating an account shut (§3) and
// reads the default visibility local as private (§5). The instance module asks this module to
// count the local accounts before the mode turns on (§1).

// ErrSingleUserMode reports the creation of an account on a one-person server (ADR-0152 §3).
// SignUp and SignUpExternal report ErrRegistrationClosed instead.
var ErrSingleUserMode = errors.New("identity: the server is a one-person server")

// singleUserPageSize is how many rejected registrations CountSingleUserAccountsTx reads at a time.
const singleUserPageSize = 500

// CountSingleUserAccountsTx counts, inside tx, the local accounts that a one-person server has only
// one of (ADR-0152 §1): every local account that is not deleted, the pending, deactivated,
// scheduled for deletion, and restricted ones included, without those whose registration was
// rejected. The instance actor has no local account and is not counted.
func (s *Service) CountSingleUserAccountsTx(ctx context.Context, tx uow.Tx) (int, error) {
	n, err := s.d.Accounts.CountLocalUsersTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	before := ""
	for {
		ids, err := s.d.Repository.ListUserIDsInRegistrationStateTx(ctx, tx, identity.RegistrationRejected, before, singleUserPageSize)
		if err != nil {
			return 0, fmt.Errorf("identity: list the rejected registrations: %w", err)
		}
		for _, id := range ids {
			counted, err := s.countedLocalAccountTx(ctx, tx, id)
			if err != nil {
				return 0, err
			}
			if counted {
				n--
			}
		}
		if len(ids) < singleUserPageSize {
			return n, nil
		}
		before = ids[len(ids)-1]
	}
}

// SingleUserAccountID returns the account of the operator of a one-person server (ADR-0152 §7): the
// first local account that CountSingleUserAccountsTx counts, which is the only one while the mode
// is on. It reports false when no account counts.
func (s *Service) SingleUserAccountID(ctx context.Context) (string, bool, error) {
	var id string
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		id = ""
		locals, err := s.d.Accounts.LocalAccountsTx(ctx, tx)
		if err != nil {
			return fmt.Errorf("identity: list the local accounts: %w", err)
		}
		for _, la := range locals {
			deleted, err := s.d.Accounts.DeletedTx(ctx, tx, la.AccountID)
			if err != nil {
				return err
			}
			if deleted {
				continue
			}
			st, err := s.d.Repository.FindRegistrationStateTx(ctx, tx, la.UserID)
			switch {
			case errors.Is(err, ErrNotFound):
			case err != nil:
				return fmt.Errorf("identity: the registration of %s: %w", la.UserID, err)
			case st.State == identity.RegistrationRejected:
				continue
			}
			id = la.AccountID
			return nil
		}
		return nil
	})
	return id, id != "", err
}

// countedLocalAccountTx reports whether CountLocalUsersTx counted the local account of userID: it
// has one, and it is not deleted.
func (s *Service) countedLocalAccountTx(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	la, err := s.d.Accounts.LocalAccountOfUserTx(ctx, tx, userID)
	if errors.Is(err, account.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	deleted, err := s.d.Accounts.DeletedTx(ctx, tx, la.AccountID)
	return err == nil && !deleted, err
}

// singleUser reports whether the server is a one-person server.
func (s *Service) singleUser(ctx context.Context) (bool, error) {
	if s.d.SingleUser == nil {
		return false, nil
	}
	on, err := s.d.SingleUser(ctx)
	if err != nil {
		return false, fmt.Errorf("instance single_user: %w", err)
	}
	return on, nil
}

// refuseSingleUserTx fails with ErrSingleUserMode on a one-person server, reading the setting in
// tx, so that an account the mode turned on against never gets written (ADR-0152 §3).
func (s *Service) refuseSingleUserTx(ctx context.Context, tx uow.Tx) error {
	if s.d.SingleUserTx == nil {
		return nil
	}
	on, err := s.d.SingleUserTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("instance single_user: %w", err)
	}
	if on {
		return ErrSingleUserMode
	}
	return nil
}
