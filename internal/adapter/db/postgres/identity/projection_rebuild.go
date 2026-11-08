// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
)

// RebuildUserRoles implements identity.Repository.
func (r *Repository) RebuildUserRoles(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild user roles", (*identitygen.Queries).DeleteAllUserRoles, (*identitygen.Queries).RebuildUserRoles)
}

// RebuildRegistrationStates implements identity.Repository.
func (r *Repository) RebuildRegistrationStates(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild registration states",
		(*identitygen.Queries).DeleteAllUserRegistrationStates, (*identitygen.Queries).RebuildUserRegistrationStates)
}

// RebuildPrimaryEmails implements identity.Repository.
func (r *Repository) RebuildPrimaryEmails(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild primary emails", (*identitygen.Queries).DeleteAllUserPrimaryEmails, (*identitygen.Queries).RebuildUserPrimaryEmails)
}
