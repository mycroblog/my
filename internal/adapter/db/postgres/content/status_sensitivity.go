// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// InsertSensitivityOverride implements content.Repository.
func (r *StatusRepository) InsertSensitivityOverride(ctx context.Context, tx uow.Tx, statusID, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertStatusSensitivityOverride(ctx, contentgen.InsertStatusSensitivityOverrideParams{
		StatusID: statusID, SourceID: actionID,
	}), "insert status sensitivity override")
}

// DeleteSensitivityOverridesOf implements content.Repository.
func (r *StatusRepository) DeleteSensitivityOverridesOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteStatusSensitivityOverridesOfAction(ctx, actionID), "delete status sensitivity overrides")
}

// InsertSensitivityOverrideOfHookDecision implements content.Repository.
func (r *StatusRepository) InsertSensitivityOverrideOfHookDecision(ctx context.Context, tx uow.Tx, statusID, decisionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertStatusSensitivityOverrideOfHookDecision(ctx, contentgen.InsertStatusSensitivityOverrideOfHookDecisionParams{
		StatusID: statusID, SourceID: decisionID,
	}), "insert status sensitivity override of a hook decision")
}

// DeleteSensitivityOverridesOfHookDecision implements content.Repository.
func (r *StatusRepository) DeleteSensitivityOverridesOfHookDecision(ctx context.Context, tx uow.Tx, decisionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteStatusSensitivityOverridesOfHookDecision(ctx, decisionID), "delete status sensitivity overrides of a hook decision")
}

// SensitivityOverridden implements content.Repository.
func (r *StatusRepository) SensitivityOverridden(ctx context.Context, statusID string) (bool, error) {
	ok, err := r.db.reader().StatusSensitivityOverridden(ctx, statusID)
	return ok, postgres.Wrap(err, "check status sensitivity override")
}
