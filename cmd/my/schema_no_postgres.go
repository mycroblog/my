// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_postgres

package main

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/platform/config"
)

// applyPostgresSchema reports that the build tag no_postgres left the PostgreSQL adapter out.
func applyPostgresSchema(context.Context, string, string, schemaapply.Options) (schemaapply.Result, error) {
	return schemaapply.Result{}, config.ExcludedError("db", "postgres", "no_postgres")
}
