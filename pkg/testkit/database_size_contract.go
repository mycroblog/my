// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"testing"

	"github.com/mycroblog/my/internal/app/metrics"
)

// DatabaseSizerHarness is a metrics.DatabaseSizer on a database with the product schema.
type DatabaseSizerHarness struct {
	Sizer metrics.DatabaseSizer
	// Grow writes at least 1 MiB of rows that do not compress into the database.
	Grow func(t *testing.T)
}

// DatabaseSizerContract verifies that a database adapter satisfies metrics.DatabaseSizer: the size
// of a database with the schema is positive, and it grows with the data written.
func DatabaseSizerContract(t *testing.T, newHarness func(t *testing.T) DatabaseSizerHarness) {
	t.Helper()
	t.Run("measures the database and its growth", func(t *testing.T) {
		h := newHarness(t)
		before, err := h.Sizer.DatabaseSize(t.Context())
		if err != nil || before <= 0 {
			t.Fatalf("DatabaseSize = %d, %v; want a positive size", before, err)
		}
		h.Grow(t)
		after, err := h.Sizer.DatabaseSize(t.Context())
		if err != nil {
			t.Fatalf("DatabaseSize after writing: %v", err)
		}
		if after-before < 1<<20 {
			t.Errorf("DatabaseSize = %d after writing 1 MiB, want at least %d", after, before+1<<20)
		}
	})
}
