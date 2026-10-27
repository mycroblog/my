// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package db_test

import (
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/schema"
)

func TestSQLiteKeyValueStoreContract(t *testing.T) {
	testkit.KeyValueStoreContract(t, func(t *testing.T, clock *testkit.Clock) kernel.KeyValueStore {
		d, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "kv.db"), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = d.Close() })
		err = d.Tx(t.Context(), func(tx uow.Tx) error {
			sqlTx, err := d.SQLTx(tx)
			if err != nil {
				return err
			}
			_, err = sqlTx.ExecContext(t.Context(), schema.SQLite)
			return err
		})
		if err != nil {
			t.Fatalf("apply schema: %v", err)
		}
		return kvdb.NewSQLite(d, clock)
	})
}
