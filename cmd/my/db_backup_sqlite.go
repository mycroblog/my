// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package main

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
)

// backupSQLite copies the SQLite database at databaseURL to the new file path in one
// consistent snapshot.
func backupSQLite(ctx context.Context, databaseURL, path string) error {
	db, err := sqlite.Open(ctx, databaseURL, uow.Options{})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return db.BackupTo(ctx, path)
}
