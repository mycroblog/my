// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
)

// ProjectionRepository is the part of Repository that rebuilds the identity projections
// (data-model §9). Each method empties one projection and derives it again by the rule of the
// commands that write it.
type ProjectionRepository interface {
	// RebuildUserRoles derives user_roles from the role grants without a revocation.
	RebuildUserRoles(ctx context.Context, tx uow.Tx) error
	// RebuildRegistrationStates derives user_registration_states from the registration
	// applications and their latest decision: pending since the application without one.
	RebuildRegistrationStates(ctx context.Context, tx uow.Tx) error
	// RebuildPrimaryEmails derives user_primary_emails from the email addresses: the first
	// address of each user, the one that registering or creating the user adds.
	RebuildPrimaryEmails(ctx context.Context, tx uow.Tx) error
	// RebuildEmailSuppressions derives email_suppressions from the delivery failures and the
	// lifts: for each address, the earliest hard bounce after its last lift, or the earliest
	// complaint after it when there is no such hard bounce (ADR-0086 §5).
	RebuildEmailSuppressions(ctx context.Context, tx uow.Tx) error
}

// RebuildProjections empties user_roles, user_registration_states, user_primary_emails, and
// email_suppressions and derives them again from the role grants and revocations, the
// registration applications and decisions, the email addresses, and the delivery failures and
// suppression lifts, in one transaction (data-model §9).
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.RebuildUserRoles(ctx, tx); err != nil {
			return err
		}
		if err := repo.RebuildRegistrationStates(ctx, tx); err != nil {
			return err
		}
		if err := repo.RebuildPrimaryEmails(ctx, tx); err != nil {
			return err
		}
		return repo.RebuildEmailSuppressions(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("rebuild the identity projections: %w", err)
	}
	return nil
}
