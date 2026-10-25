// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
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
		runs[i] = dbupgrade.Run{Key: row.StepKey, CompletedAt: fromMillis(row.CompletedAt)}
	}
	return runs, nil
}

// missingRecord returns dbupgrade.ErrEmptyDatabase or dbupgrade.ErrNoRecord when the database
// holds no table or lacks data_upgrade_runs, and nil otherwise.
func (r *DataUpgradeRepository) missingRecord(ctx context.Context) error {
	var tables, record int64
	err := r.db.db.QueryRowContext(ctx,
		`SELECT count(*), coalesce(sum(name = 'data_upgrade_runs'), 0) FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`,
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
	err = q.InsertDataUpgradeRun(ctx, sqlitegen.InsertDataUpgradeRunParams{StepKey: key, CompletedAt: toMillis(at)})
	return wrap(err, "record data upgrade run")
}
