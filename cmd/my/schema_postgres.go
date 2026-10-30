// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package main

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
)

// applyPostgresSchema applies the embedded schema to the PostgreSQL database of databaseURL,
// building the desired schema in the database of devURL when it is not empty.
func applyPostgresSchema(ctx context.Context, databaseURL, devURL string, opts schemaapply.Options) (schemaapply.Result, error) {
	return postgres.ApplySchema(ctx, databaseURL, devURL, opts)
}
