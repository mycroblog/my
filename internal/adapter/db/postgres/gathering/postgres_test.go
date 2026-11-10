// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"os"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/gatheringtest"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}

func TestGatheringContract(t *testing.T) {
	gatheringtest.GatheringContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := postgres.Open(t.Context(), pgtest.NewDatabase(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}

func TestRoomContract(t *testing.T) {
	gatheringtest.RoomContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := postgres.Open(t.Context(), pgtest.NewDatabase(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}

func TestDueContract(t *testing.T) {
	gatheringtest.DueContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := postgres.Open(t.Context(), pgtest.NewDatabase(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}

func TestPollContract(t *testing.T) {
	gatheringtest.PollContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := postgres.Open(t.Context(), pgtest.NewDatabase(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}

func TestModerationContract(t *testing.T) {
	gatheringtest.ModerationContract(t, func(t *testing.T, opts uow.Options) gatheringtest.Harness {
		db, err := postgres.Open(t.Context(), pgtest.NewDatabase(t), opts)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		return gatheringtest.Harness{UnitOfWork: db, Gathering: NewRepository(db)}
	})
}
