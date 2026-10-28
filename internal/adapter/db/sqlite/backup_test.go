// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestBackupTo(t *testing.T) {
	ctx := t.Context()
	db, _ := newTestDB(t, "", uow.Options{})
	want := []string{testkit.NamedID("a"), testkit.NamedID("b")}
	slices.Sort(want)
	for _, k := range want {
		if err := insertProbe(ctx, sqlitegen.New(db.db), k); err != nil {
			t.Fatalf("insert %s: %v", k, err)
		}
	}

	// Another connection holds a read snapshot open meanwhile, as the server's connections do.
	reader, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Rollback() }()
	var n int
	if err := reader.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "backup.db")
	if err := db.BackupTo(ctx, path); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); err == nil {
			t.Errorf("the copy has a %s file", suffix)
		}
	}
	restored, err := Open(ctx, path, uow.Options{})
	if err != nil {
		t.Fatalf("open the copy: %v", err)
	}
	defer func() { _ = restored.Close() }()
	if err := restored.Ready(ctx); err != nil {
		t.Errorf("the copy is not ready: %v", err)
	}
	if keys, err := probeKeys(ctx, restored); err != nil || !slices.Equal(keys, want) {
		t.Errorf("rows of the copy = %q (%v), want %q", keys, err, want)
	}

	if err := db.BackupTo(ctx, path); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("BackupTo over an existing file = %v, want an error that it already exists", err)
	}
	if err := db.BackupTo(ctx, filepath.Join(t.TempDir(), "missing", "backup.db")); err == nil {
		t.Error("BackupTo into a missing directory succeeded")
	}
	if err := db.BackupTo(ctx, ""); err == nil {
		t.Error("BackupTo without a file succeeded")
	}
}
