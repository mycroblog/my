// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// BackupTo writes a consistent copy of the database to the new file path with VACUUM INTO.
// It reads one snapshot of the database, so other connections and processes may keep writing
// while it runs: the copy holds every transaction committed before it started, including those
// still in the write-ahead log, and none after. The copy is one file without -wal or -shm
// files. BackupTo fails if path exists, and removes a partial copy when it fails.
func (d *DB) BackupTo(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("sqlite: backup: no file given")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("sqlite: backup: %s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sqlite: backup: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("sqlite: backup: %w", err)
	}
	return nil
}
