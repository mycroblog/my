// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ListUserIDsWithPermission implements identity.Repository.
func (r *Repository) ListUserIDsWithPermission(ctx context.Context, tx uow.Tx, p identity.Permission) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListUserIDsWithPermission(ctx, string(p))
	return ids, postgres.Wrap(err, "list users with a permission")
}

// CountRegistrations implements identity.RegistrationRepository.
func (r *Repository) CountRegistrations(ctx context.Context, state identity.RegistrationState) (int64, error) {
	n, err := r.db.reader().CountRegistrationsInState(ctx, string(state))
	return n, postgres.Wrap(err, "count registrations")
}
