// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
)

// ExistingStatusIDs implements apphook.Statuses on the content module's table.
func (r *StatusRepository) ExistingStatusIDs(ctx context.Context, ids []string) ([]string, error) {
	out, err := r.db.reader().ListExistingStatusIDs(ctx, ids)
	return out, postgres.Wrap(err, "list existing statuses")
}
