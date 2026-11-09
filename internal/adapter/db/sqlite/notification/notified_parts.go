// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
)

// ErrorGroupNoticesOf implements notification.Repository.
func (r *Repository) ErrorGroupNoticesOf(ctx context.Context, hours []time.Time) (map[time.Time]int64, error) {
	out := map[time.Time]int64{}
	if len(hours) == 0 {
		return out, nil
	}
	millis := make([]int64, len(hours))
	for i, h := range hours {
		millis[i] = sqlite.ToMillis(h)
	}
	b, err := json.Marshal(millis)
	if err != nil {
		return nil, err
	}
	list := string(b)
	rows, err := r.db.reader().ListErrorGroupNoticesOf(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list error group notices")
	}
	for _, row := range rows {
		out[sqlite.FromMillis(row.HourStart)] = row.Notices
	}
	return out, nil
}
