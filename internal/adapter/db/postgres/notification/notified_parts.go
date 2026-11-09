// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
)

// ErrorGroupNoticesOf implements notification.Repository.
func (r *Repository) ErrorGroupNoticesOf(ctx context.Context, hours []time.Time) (map[time.Time]int64, error) {
	out := map[time.Time]int64{}
	if len(hours) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListErrorGroupNoticesOf(ctx, hours)
	if err != nil {
		return nil, postgres.Wrap(err, "list error group notices")
	}
	for _, row := range rows {
		out[row.HourStart.UTC()] = row.Notices
	}
	return out, nil
}
