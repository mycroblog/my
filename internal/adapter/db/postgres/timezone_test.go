// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// TestTimestampsComeBackInUTC checks that a timestamptz comes back in UTC whatever the process's
// local time zone is, as the SQLite adapter returns it (#2508): pgx would return it in time.Local.
// It changes time.Local, so it does not run in parallel.
func TestTimestampsComeBackInUTC(t *testing.T) {
	local := time.Local
	time.Local = time.FixedZone("UTC+9", 9*60*60)
	t.Cleanup(func() { time.Local = local })

	db := newTestDB(t, uow.Options{})
	want := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	var got time.Time
	if err := db.pool.QueryRow(t.Context(), "SELECT $1::timestamptz", want).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got.Location() != time.UTC || !got.Equal(want) {
		t.Errorf("timestamptz came back as %v (%v), want %v in UTC", got, got.Location(), want)
	}
}
