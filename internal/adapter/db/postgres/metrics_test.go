// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/metricstest"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestDatabaseSizerContract(t *testing.T) {
	metricstest.DatabaseSizerContract(t, func(t *testing.T) metricstest.DatabaseSizerHarness {
		db := newTestDB(t, uow.Options{})
		return metricstest.DatabaseSizerHarness{Sizer: db, Grow: func(t *testing.T) {
			// Rows of 1 KiB stay below the size that PostgreSQL compresses.
			_, err := db.pool.Exec(t.Context(), `CREATE TABLE size_probe AS
				SELECT repeat(md5(i::text), 32) AS v FROM generate_series(1, 2048) AS i`)
			if err != nil {
				t.Fatal(err)
			}
		}}
	})
}

// Instances that share one database, each in its own schema of database_url's search_path, count
// the relations of their own schema only.
func TestDatabaseSizeCountsTheCurrentSchemaOnly(t *testing.T) {
	base := pgtest.NewDatabase(t)
	open := func(t *testing.T, searchPath string) *DB {
		t.Helper()
		u, err := url.Parse(base)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", searchPath)
		u.RawQuery = q.Encode()
		db, err := Open(t.Context(), u.String(), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		return db
	}
	public := open(t, "public")
	for _, s := range []string{"tenant_a", "Tenant-B"} {
		for _, sql := range []string{
			"CREATE SCHEMA " + pgx.Identifier{s}.Sanitize(),
			"CREATE TABLE " + pgx.Identifier{s, "notes"}.Sanitize() + " (id bigint PRIMARY KEY, body text)",
		} {
			if _, err := public.pool.Exec(t.Context(), sql); err != nil {
				t.Fatal(err)
			}
		}
	}
	a, b := open(t, "tenant_a"), open(t, `"Tenant-B"`)
	size := func(t *testing.T, db *DB) int64 {
		t.Helper()
		n, err := db.DatabaseSize(t.Context())
		if err != nil {
			t.Fatalf("DatabaseSize: %v", err)
		}
		return n
	}
	beforeA, beforeB, beforePublic := size(t, a), size(t, b), size(t, public)
	if beforeA <= 0 || beforeB <= 0 {
		t.Fatalf("the sizes of tenant_a and Tenant-B are %d and %d, want a table's index in each", beforeA, beforeB)
	}
	if beforePublic <= beforeA {
		t.Errorf("public, with the product schema, is %d bytes, not more than tenant_a's %d", beforePublic, beforeA)
	}

	// Rows of 1 KiB stay below the size that PostgreSQL compresses.
	if _, err := b.pool.Exec(t.Context(), `INSERT INTO notes
		SELECT i, repeat(md5(i::text), 32) FROM generate_series(1, 2048) AS i`); err != nil {
		t.Fatal(err)
	}
	if got := size(t, b); got <= beforeB {
		t.Errorf("Tenant-B's size went from %d to %d after it grew", beforeB, got)
	}
	if got := size(t, a); got != beforeA {
		t.Errorf("tenant_a's size went from %d to %d when Tenant-B grew", beforeA, got)
	}
	if got := size(t, public); got != beforePublic {
		t.Errorf("public's size went from %d to %d when Tenant-B grew", beforePublic, got)
	}
}

// A search_path none of whose schemas exists has no relations to count.
func TestDatabaseSizeWithoutASchemaIsZero(t *testing.T) {
	u, err := url.Parse(pgtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", "missing_schema")
	u.RawQuery = q.Encode()
	db, err := Open(t.Context(), u.String(), uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)
	n, err := db.DatabaseSize(t.Context())
	if err != nil || n != 0 {
		t.Errorf("DatabaseSize = %d, %v; want 0, nil", n, err)
	}
}
