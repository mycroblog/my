// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package main

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/schema"
)

// applySQLiteSchema applies the embedded schema to the SQLite database of databaseURL.
func applySQLiteSchema(ctx context.Context, databaseURL string, opts schemaapply.Options) (schemaapply.Result, error) {
	return sqlite.ApplySchema(ctx, databaseURL, schema.SQLite, opts)
}
