// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// ListCurrentSettingsTx implements the reads in a transaction.
func (r *Repository) ListCurrentSettingsTx(ctx context.Context, tx uow.Tx) ([]instance.CurrentSetting, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListCurrentSettings(ctx)
}
