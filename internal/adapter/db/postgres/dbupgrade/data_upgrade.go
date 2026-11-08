// SPDX-License-Identifier: AGPL-3.0-only

package dbupgrade

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/dbupgradegen"
	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/uow"
)

// DataUpgradeRepository implements dbupgrade.Repository on data_upgrade_runs.
type DataUpgradeRepository struct{ db *moduleDB }

var _ dbupgrade.Repository = (*DataUpgradeRepository)(nil)

// NewDataUpgradeRepository returns the record of the data upgrade steps.
func NewDataUpgradeRepository(d *postgres.DB) *DataUpgradeRepository {
	return &DataUpgradeRepository{db: &moduleDB{d}}
}

// Runs implements dbupgrade.Repository.
func (r *DataUpgradeRepository) Runs(ctx context.Context) ([]dbupgrade.Run, error) {
	rows, err := r.db.reader().ListDataUpgradeRuns(ctx)
	if err != nil {
		if missing := r.missingRecord(ctx); missing != nil {
			return nil, missing
		}
		return nil, postgres.Wrap(err, "list data upgrade runs")
	}
	runs := make([]dbupgrade.Run, len(rows))
	for i, row := range rows {
		runs[i] = dbupgrade.Run{Key: row.StepKey, CompletedAt: postgres.UTC(row.CompletedAt)}
	}
	return runs, nil
}

// missingRecord returns dbupgrade.ErrEmptyDatabase or dbupgrade.ErrNoRecord when the current
// schema holds no table or lacks data_upgrade_runs, and nil otherwise.
func (r *DataUpgradeRepository) missingRecord(ctx context.Context) error {
	var tables, record int64
	err := r.db.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE table_name = 'data_upgrade_runs')
		FROM information_schema.tables WHERE table_schema = current_schema()`,
	).Scan(&tables, &record)
	switch {
	case err != nil:
		return nil
	case tables == 0:
		return dbupgrade.ErrEmptyDatabase
	case record == 0:
		return dbupgrade.ErrNoRecord
	default:
		return nil
	}
}

// RecordRun implements dbupgrade.Repository.
func (r *DataUpgradeRepository) RecordRun(ctx context.Context, tx uow.Tx, key string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDataUpgradeRun(ctx, dbupgradegen.InsertDataUpgradeRunParams{StepKey: key, CompletedAt: at})
	return postgres.Wrap(err, "record data upgrade run")
}
