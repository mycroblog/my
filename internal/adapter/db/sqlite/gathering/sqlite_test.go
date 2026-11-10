// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/gatheringtest"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

func TestGatheringContract(t *testing.T) {
	t.Parallel()
	gatheringtest.GatheringContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := sqlite.Open(t.Context(), sqlitetest.NewFile(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}

func TestDueContract(t *testing.T) {
	t.Parallel()
	gatheringtest.DueContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := sqlite.Open(t.Context(), sqlitetest.NewFile(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}

func TestPollContract(t *testing.T) {
	t.Parallel()
	gatheringtest.PollContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := sqlite.Open(t.Context(), sqlitetest.NewFile(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}
