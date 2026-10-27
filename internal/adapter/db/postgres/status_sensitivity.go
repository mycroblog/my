// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// InsertSensitivityOverride implements content.Repository.
func (r *StatusRepository) InsertSensitivityOverride(ctx context.Context, tx uow.Tx, statusID, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertStatusSensitivityOverride(ctx, postgresgen.InsertStatusSensitivityOverrideParams{
		StatusID: statusID, SourceActionID: actionID,
	}), "insert status sensitivity override")
}

// DeleteSensitivityOverridesOf implements content.Repository.
func (r *StatusRepository) DeleteSensitivityOverridesOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteStatusSensitivityOverridesOfAction(ctx, actionID), "delete status sensitivity overrides")
}

// SensitivityOverridden implements content.Repository.
func (r *StatusRepository) SensitivityOverridden(ctx context.Context, statusID string) (bool, error) {
	ok, err := r.db.reader().StatusSensitivityOverridden(ctx, statusID)
	return ok, wrap(err, "check status sensitivity override")
}
