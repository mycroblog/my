// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_sqlite

package main

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/platform/config"
)

// applySQLiteSchema reports that the build tag no_sqlite left the SQLite adapter out.
func applySQLiteSchema(context.Context, string, schemaapply.Options) (schemaapply.Result, error) {
	return schemaapply.Result{}, config.ExcludedError("db", "sqlite", "no_sqlite")
}
