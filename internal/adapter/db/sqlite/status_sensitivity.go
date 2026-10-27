// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
)

// InsertSensitivityOverride implements content.Repository.
func (r *StatusRepository) InsertSensitivityOverride(ctx context.Context, tx uow.Tx, statusID, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertStatusSensitivityOverride(ctx, sqlitegen.InsertStatusSensitivityOverrideParams{
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
