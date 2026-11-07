// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
)

// The logical type date is TEXT that date() gives back unchanged (data-model §1.2): the CHECK
// takes a real YYYY-MM-DD only.
func TestDateCheck(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	for _, tc := range []struct {
		date string
		ok   bool
	}{
		{"2026-04-10", true},
		{"2024-02-29", true},
		{"0001-01-01", true},
		{"9999-12-31", true},
		{"2026-02-29", false},
		{"2026-04-31", false},
		{"2026-4-10", false},
		{"2026-04-10 00:00:00", false},
		{"20260410", false},
		{"", false},
	} {
		_, err := db.SQLDB().ExecContext(t.Context(),
			"INSERT INTO cost_items (id, schedule, interval, starts_on, created_at) VALUES (?, 'once', 1, ?, 0)", "item-"+tc.date, tc.date)
		if (err == nil) != tc.ok {
			t.Errorf("starts_on %q: err = %v, want ok %v", tc.date, err, tc.ok)
		}
	}
}
