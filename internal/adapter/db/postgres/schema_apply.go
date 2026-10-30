// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	atlaspostgres "ariga.io/atlas/sql/postgres"
	atlasschema "ariga.io/atlas/sql/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/schema"
)

// schemaApplyLock is the key of the session-scoped advisory lock that ApplySchema holds, so
// that runs against one database take turns.
const schemaApplyLock = "my schema apply"

// devSchemaPrefix starts the name of the temporary schema of a run.
const devSchemaPrefix = "my_schema_apply_"

// ApplySchema brings the PostgreSQL database of url (the database_url setting) to the embedded
// schema (schema.Postgres) with the Atlas library (ADR-0126 §1). Holding a session-scoped
// advisory lock, it runs the bootstrap SQL (schema.PostgresBootstrap), executes the schema in
// a temporary schema of its own (in the database of devURL when it is not empty, else in the
// target database) and inspects it, inspects the target's current schema, plans the
// difference, and applies it in one transaction. The temporary schema is dropped whether the
// run succeeds or not. A database that matches the schema is left unchanged.
func ApplySchema(ctx context.Context, url, devURL string, opts schemaapply.Options) (schemaapply.Result, error) {
	r, err := applySchema(ctx, url, devURL, schema.Postgres, opts)
	if err != nil {
		return r, fmt.Errorf("postgres: schema apply: %w", err)
	}
	return r, nil
}

// applySchema is ApplySchema with the desired schema ddl.
func applySchema(ctx context.Context, url, devURL, ddl string, opts schemaapply.Options) (schemaapply.Result, error) {
	db, err := openSQL(url, "database_url")
	if err != nil {
		return schemaapply.Result{}, err
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return schemaapply.Result{}, err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1, 0))", schemaApplyLock); err != nil {
		return schemaapply.Result{}, fmt.Errorf("lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock(hashtextextended($1, 0))", schemaApplyLock)
	}()
	if _, err := conn.ExecContext(ctx, schema.PostgresBootstrap); err != nil {
		return schemaapply.Result{}, fmt.Errorf("bootstrap: %w", err)
	}
	var target string
	if err := conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&target); err != nil {
		return schemaapply.Result{}, fmt.Errorf("current schema: %w", err)
	}

	dev := conn
	if devURL != "" {
		devDB, err := openSQL(devURL, "database_dev_url")
		if err != nil {
			return schemaapply.Result{}, err
		}
		defer func() { _ = devDB.Close() }()
		if dev, err = devDB.Conn(ctx); err != nil {
			return schemaapply.Result{}, fmt.Errorf("dev database: %w", err)
		}
		defer func() { _ = dev.Close() }()
		if _, err := dev.ExecContext(ctx, schema.PostgresBootstrap); err != nil {
			return schemaapply.Result{}, fmt.Errorf("bootstrap the dev database: %w", err)
		}
	}
	desired, err := inspectDesired(ctx, dev, ddl)
	if err != nil {
		return schemaapply.Result{}, fmt.Errorf("the embedded schema: %w", err)
	}
	desired.Name = target

	drv, err := atlaspostgres.Open(conn)
	if err != nil {
		return schemaapply.Result{}, err
	}
	current, err := drv.InspectSchema(ctx, target, nil)
	if err != nil {
		return schemaapply.Result{}, fmt.Errorf("inspect: %w", err)
	}
	return schemaapply.Run(ctx, drv, current, desired, opts, func(changes []atlasschema.Change) error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txDrv, err := atlaspostgres.Open(tx)
		if err == nil {
			err = txDrv.ApplyChanges(ctx, changes)
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	})
}

// inspectDesired executes ddl in a new temporary schema on conn, inspects it,
// and drops it. The search path is the temporary schema, for the tables, then the connection's
// current schema, which holds the extensions of the bootstrap SQL. The inspected schema keeps
// the temporary name; the caller renames it to the target's.
func inspectDesired(ctx context.Context, conn *sql.Conn, ddl string) (_ *atlasschema.Schema, err error) {
	var current string
	if err := conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&current); err != nil {
		return nil, err
	}
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	name := devSchemaPrefix + hex.EncodeToString(suffix)
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA "+ident); err != nil {
		return nil, err
	}
	defer func() {
		// The connection may be in a failed state only inside a transaction, and none is open.
		_, dropErr := conn.ExecContext(context.WithoutCancel(ctx), "DROP SCHEMA IF EXISTS "+ident+" CASCADE; RESET search_path")
		if dropErr != nil {
			err = errors.Join(err, fmt.Errorf("drop the temporary schema %s: %w", name, dropErr))
		}
	}()
	if _, err := conn.ExecContext(ctx, "SET search_path TO "+ident+", "+pgx.Identifier{current}.Sanitize()); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, ddl); err != nil {
		return nil, err
	}
	drv, err := atlaspostgres.Open(conn)
	if err != nil {
		return nil, err
	}
	return drv.InspectSchema(ctx, name, nil)
}

// openSQL opens a database/sql pool on the connection string of the setting key.
func openSQL(url, key string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		// The error may quote the URL, which can hold a password; keep only its kind.
		return nil, fmt.Errorf("%s is not a valid connection string", key)
	}
	return stdlib.OpenDB(*cfg), nil
}
