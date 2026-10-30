// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	atlasschema "ariga.io/atlas/sql/schema"
	atlassqlite "ariga.io/atlas/sql/sqlite"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/schema"
)

// devDSN is the dev database of ApplySchema: in memory, a new one for each connection.
const devDSN = "file:my_schema_apply_dev?mode=memory"

// ApplySchema brings the SQLite database of dsn (the database_url setting, in the form Open
// takes) to the embedded schema (schema.SQLite) with the Atlas library: it executes the schema
// on an in-memory dev database, inspects both, plans the difference, and applies it in one
// transaction (ADR-0126 §1). The transaction runs with the foreign keys unenforced, so that
// Atlas can rebuild a table, and fails on commit when it adds a foreign key violation. A
// database that matches the schema is left unchanged. The file is created if it does not exist.
func ApplySchema(ctx context.Context, dsn string, opts schemaapply.Options) (schemaapply.Result, error) {
	return applySchema(ctx, dsn, schema.SQLite, opts)
}

// applySchema is ApplySchema with the desired schema ddl.
func applySchema(ctx context.Context, dsn, ddl string, opts schemaapply.Options) (schemaapply.Result, error) {
	desired, err := inspectDesired(ctx, ddl)
	if err != nil {
		return schemaapply.Result{}, fmt.Errorf("sqlite: schema apply: the embedded schema: %w", err)
	}
	name, err := normalizeDSN(dsn)
	if err != nil {
		return schemaapply.Result{}, err
	}
	db, err := sql.Open("sqlite", name)
	if err != nil {
		return schemaapply.Result{}, fmt.Errorf("sqlite: schema apply: %w", err)
	}
	defer func() { _ = db.Close() }()
	// OpenTx sets the foreign_keys pragma on a connection and then begins the transaction:
	// both must be the same connection.
	db.SetMaxOpenConns(1)
	drv, err := atlassqlite.Open(db)
	if err != nil {
		return schemaapply.Result{}, fmt.Errorf("sqlite: schema apply: %w", err)
	}
	current, err := drv.InspectSchema(ctx, "", nil)
	if err != nil {
		return schemaapply.Result{}, fmt.Errorf("sqlite: schema apply: inspect: %w", err)
	}
	r, err := schemaapply.Run(ctx, drv, current, desired, opts, func(changes []atlasschema.Change) error {
		tx, err := atlassqlite.OpenTx(ctx, db, nil)
		if err != nil {
			return err
		}
		txDrv, err := atlassqlite.Open(tx)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := txDrv.ApplyChanges(ctx, changes); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return r, fmt.Errorf("sqlite: schema apply: %w", err)
	}
	return r, nil
}

// inspectDesired executes ddl on a new in-memory database and inspects it.
func inspectDesired(ctx context.Context, ddl string) (*atlasschema.Schema, error) {
	dev, err := sql.Open("sqlite", devDSN)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dev.Close() }()
	// Each connection to devDSN is a database of its own.
	dev.SetMaxOpenConns(1)
	if _, err := dev.ExecContext(ctx, ddl); err != nil {
		return nil, err
	}
	drv, err := atlassqlite.Open(dev)
	if err != nil {
		return nil, err
	}
	return drv.InspectSchema(ctx, "", nil)
}
