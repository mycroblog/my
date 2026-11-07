// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// DeleteAllSensitivityOverrides implements content.Repository.
func (r *StatusRepository) DeleteAllSensitivityOverrides(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty status sensitivity overrides", (*contentgen.Queries).DeleteAllStatusSensitivityOverrides)
}

// DeleteSensitivityOverridesWithoutStatus implements content.Repository.
func (r *StatusRepository) DeleteSensitivityOverridesWithoutStatus(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "drop status sensitivity overrides of gone statuses", (*contentgen.Queries).DeleteStatusSensitivityOverridesWithoutStatus)
}

// RebuildPollTallies implements content.Repository.
func (r *StatusRepository) RebuildPollTallies(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild poll tallies", (*contentgen.Queries).DeleteAllPollTallies, (*contentgen.Queries).RebuildPollTallies)
}
