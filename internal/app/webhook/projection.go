// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
)

// ProjectionRepository is the part of Repository that rebuilds the webhook projections.
// Each method derives one projection again by the rule of the projector that
// writes it.
type ProjectionRepository interface {
	// RebuildHeads empties webhook_endpoint_heads and derives it from the newest revision,
	// state change, and secret of each endpoint.
	RebuildHeads(ctx context.Context, tx uow.Tx) error
	// RebuildFailures derives webhook_endpoint_failures from the attempts: the failed attempts
	// of an endpoint after its newest succeeded one, from the first of them. The purge removes
	// the attempts after 14 days, so a run whose first failure it removed cannot be derived:
	// its row stays as it is unless a kept success ended the run.
	RebuildFailures(ctx context.Context, tx uow.Tx) error
}

// RebuildProjections derives webhook_endpoint_heads and webhook_endpoint_failures again from
// the revisions, state changes, secrets, and attempts of the endpoints, in one transaction.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.RebuildHeads(ctx, tx); err != nil {
			return err
		}
		return repo.RebuildFailures(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("rebuild the webhook projections: %w", err)
	}
	return nil
}
