// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
)

// ErrWrongPassword reports a password that does not match the user's current one, given to
// confirm an operation of a signed-in user.
var ErrWrongPassword = errors.New("identity: the password is wrong")

// DeleteOwnAccount deletes the local account that a user logs in as at the user's request
// (FR-008), once the user's current password or one of their passkeys confirms it: the account
// is deleted at once (account.Service.DeleteBySelf), which signs the user out everywhere, sends
// the Delete of the actor to the remote servers, and leaves the data to the erasure (data-model
// §8). A wrong password fails with ErrWrongPassword, a wrong passkey with the errors of
// Service.confirm, and too many of them (SelfDeletionLimit) with a *kernel.RateLimitedError; they
// change nothing.
func (s *Service) DeleteOwnAccount(ctx context.Context, userID string, c Confirmation) error {
	if s.d.RateLimiter == nil {
		return errors.New("identity: DeleteOwnAccount needs a RateLimiter")
	}
	if err := s.confirm(ctx, OperationAccountDeletion, userID, c); err != nil {
		return err
	}
	st, err := s.d.Accounts.UserLifecycle(ctx, userID)
	if err != nil {
		return err
	}
	return s.d.Accounts.DeleteBySelf(ctx, st.AccountID)
}
