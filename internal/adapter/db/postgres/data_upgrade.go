// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/uow"
)

// DataUpgradeRepository implements dbupgrade.Repository on data_upgrade_runs.
type DataUpgradeRepository struct{ db *DB }

var _ dbupgrade.Repository = (*DataUpgradeRepository)(nil)

// DataUpgradeRepository returns the record of the data upgrade steps.
func (d *DB) DataUpgradeRepository() *DataUpgradeRepository { return &DataUpgradeRepository{db: d} }

// Runs implements dbupgrade.Repository.
func (r *DataUpgradeRepository) Runs(ctx context.Context) ([]dbupgrade.Run, error) {
	rows, err := r.db.reader().ListDataUpgradeRuns(ctx)
	if err != nil {
		if missing := r.missingRecord(ctx); missing != nil {
			return nil, missing
		}
		return nil, wrap(err, "list data upgrade runs")
	}
	runs := make([]dbupgrade.Run, len(rows))
	for i, row := range rows {
		runs[i] = dbupgrade.Run{Key: row.StepKey, CompletedAt: utc(row.CompletedAt)}
	}
	return runs, nil
}

// missingRecord returns dbupgrade.ErrEmptyDatabase or dbupgrade.ErrNoRecord when the current
// schema holds no table or lacks data_upgrade_runs, and nil otherwise.
func (r *DataUpgradeRepository) missingRecord(ctx context.Context) error {
	var tables, record int64
	err := r.db.pool.QueryRow(ctx,
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
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDataUpgradeRun(ctx, postgresgen.InsertDataUpgradeRunParams{StepKey: key, CompletedAt: at})
	return wrap(err, "record data upgrade run")
}
