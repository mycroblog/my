// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/diagnosticsgen"
	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

var _ appdiagnostics.UXRepository = (*Repository)(nil)

// AddUXCount implements appdiagnostics.UXRepository.
func (r *Repository) AddUXCount(ctx context.Context, c appdiagnostics.UXCount) error {
	return sqlite.Wrap(r.db.reader().AddUXCount(ctx, diagnosticsgen.AddUXCountParams{
		HourStart: sqlite.ToMillis(c.Hour), Metric: string(c.Metric), Name: c.Name, Bucket: int64(c.Bucket), Samples: c.Samples,
	}), "add ux count")
}

// DeleteUXCountsBefore implements appdiagnostics.UXRepository.
func (r *Repository) DeleteUXCountsBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.db.reader().DeleteUXCountsBefore(ctx, sqlite.ToMillis(before))
	return n, sqlite.Wrap(err, "delete ux counts")
}

// UXCountsSince implements appdiagnostics.UXRepository.
func (r *Repository) UXCountsSince(ctx context.Context, since time.Time) ([]appdiagnostics.UXCount, error) {
	rows, err := r.db.reader().ListUXCountsSince(ctx, sqlite.ToMillis(since))
	if err != nil {
		return nil, sqlite.Wrap(err, "list ux counts since")
	}
	out := make([]appdiagnostics.UXCount, len(rows))
	for i, row := range rows {
		out[i] = appdiagnostics.UXCount{Metric: diagnostics.UXMetric(row.Metric), Name: row.Name, Bucket: int(row.Bucket), Samples: row.Samples}
	}
	return out, nil
}
