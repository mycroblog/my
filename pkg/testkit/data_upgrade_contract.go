// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/uow"
)

// DataUpgradeHarness is a dbupgrade.Repository on a fresh database with the product schema,
// with the UnitOfWork of the same database.
type DataUpgradeHarness struct {
	UnitOfWork uow.UnitOfWork
	Repository dbupgrade.Repository
	// DropRecord drops the table data_upgrade_runs from the database, as on a database whose
	// schema predates the record.
	DropRecord func(t *testing.T)
	// Empty is the repository on another database that holds no table.
	Empty dbupgrade.Repository
}

// DataUpgradeRepositoryContract is the contract of dbupgrade.Repository, and of
// dbupgrade.Service on it, that every database adapter must pass.
func DataUpgradeRepositoryContract(t *testing.T, newHarness func(t *testing.T) DataUpgradeHarness) {
	t.Helper()
	t.Run("records each step once", func(t *testing.T) {
		t.Parallel()
		dataUpgradeRecords(t, newHarness(t))
	})
	t.Run("tells a database without the record from an empty one", func(t *testing.T) {
		t.Parallel()
		dataUpgradeMissingRecord(t, newHarness(t))
	})
	t.Run("upgrade runs the pending steps once and resumes after a failure", func(t *testing.T) {
		t.Parallel()
		dataUpgradeService(t, newHarness(t))
	})
}

var dataUpgradeStart = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

func recordRun(t *testing.T, h DataUpgradeHarness, key string, at time.Time) {
	t.Helper()
	if err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return h.Repository.RecordRun(t.Context(), tx, key, at)
	}); err != nil {
		t.Fatalf("RecordRun(%q): %v", key, err)
	}
}

func dataUpgradeRecords(t *testing.T, h DataUpgradeHarness) {
	ctx := t.Context()
	runs, err := h.Repository.Runs(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatalf("Runs on a new database = %v, %v; want none", runs, err)
	}
	recordRun(t, h, "b.second", dataUpgradeStart.Add(time.Minute))
	recordRun(t, h, "a.first", dataUpgradeStart)
	// Recording again keeps the first record.
	recordRun(t, h, "a.first", dataUpgradeStart.Add(time.Hour))
	runs, err = h.Repository.Runs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []dbupgrade.Run{{Key: "a.first", CompletedAt: dataUpgradeStart}, {Key: "b.second", CompletedAt: dataUpgradeStart.Add(time.Minute)}}
	if !slices.EqualFunc(runs, want, func(a, b dbupgrade.Run) bool { return a.Key == b.Key && a.CompletedAt.Equal(b.CompletedAt) }) {
		t.Errorf("Runs = %v, want %v", runs, want)
	}
}

func dataUpgradeMissingRecord(t *testing.T, h DataUpgradeHarness) {
	ctx := t.Context()
	if _, err := h.Empty.Runs(ctx); !errors.Is(err, dbupgrade.ErrEmptyDatabase) {
		t.Errorf("Runs on a database without tables = %v, want ErrEmptyDatabase", err)
	}
	h.DropRecord(t)
	if _, err := h.Repository.Runs(ctx); !errors.Is(err, dbupgrade.ErrNoRecord) {
		t.Errorf("Runs on a database without data_upgrade_runs = %v, want ErrNoRecord", err)
	}

	prereqs := []dbupgrade.Prerequisite{{Key: "x.backfill", Via: "v0.2.0"}}
	steps := []dbupgrade.Step{{Key: "x.backfill"}}
	empty := dbupgrade.NewService(dbupgrade.Options{Repository: h.Empty, Steps: steps, Prerequisites: prereqs})
	if r, err := empty.Check(ctx); err != nil || !r.NewDatabase || len(r.Missing) != 0 {
		t.Errorf("Check on a database without tables = %+v, %v; want a new database", r, err)
	}
	old := dbupgrade.NewService(dbupgrade.Options{Repository: h.Repository, Steps: steps, Prerequisites: prereqs})
	if r, err := old.Check(ctx); err != nil || r.NewDatabase || !slices.Equal(r.Missing, prereqs) {
		t.Errorf("Check on a database without the record = %+v, %v; want %v missing", r, err, prereqs)
	}
}

// dataUpgradeService runs a test step that rewrites in three batches, each a transaction that
// records a marker key in place of the rows a real step rewrites, and skips the batches whose
// marker exists.
func dataUpgradeService(t *testing.T, h DataUpgradeHarness) {
	ctx := t.Context()
	clock := NewClock(dataUpgradeStart)
	failAfter := 2
	var batches, rebuilds []string
	backfill := func(ctx context.Context, u uow.UnitOfWork) error {
		runs, err := h.Repository.Runs(ctx)
		if err != nil {
			return err
		}
		done := map[string]bool{}
		for _, r := range runs {
			done[r.Key] = true
		}
		for i := 1; i <= 3; i++ {
			marker := fmt.Sprintf("test.backfill.batch%d", i)
			if done[marker] {
				continue
			}
			if i > failAfter {
				return errors.New("interrupted")
			}
			if err := u.Tx(ctx, func(tx uow.Tx) error { return h.Repository.RecordRun(ctx, tx, marker, clock.Now()) }); err != nil {
				return err
			}
			batches = append(batches, marker)
		}
		return nil
	}
	recordRun(t, h, "test.done", dataUpgradeStart)
	svc := dbupgrade.NewService(dbupgrade.Options{
		UnitOfWork: h.UnitOfWork, Repository: h.Repository, Clock: clock,
		Steps: []dbupgrade.Step{
			{Key: "test.done", Description: "recorded before", Run: func(context.Context, uow.UnitOfWork) error {
				return errors.New("a recorded step must not run")
			}},
			{Key: "test.backfill", Description: "backfill in batches", Run: backfill, Projections: []string{"p1", "p2"}},
			{Key: "test.retired", Description: "retired step"},
		},
		Prerequisites: []dbupgrade.Prerequisite{{Key: "test.done", Via: "v0.1.0"}},
		RebuildProjection: func(_ context.Context, name string) error {
			rebuilds = append(rebuilds, name)
			return nil
		},
	})

	if err := svc.Ready(ctx); !errors.Is(err, dbupgrade.ErrNotUpgraded) {
		t.Fatalf("Ready before the upgrade = %v, want ErrNotUpgraded", err)
	}
	if r, err := svc.Check(ctx); err != nil || len(r.Missing) != 0 {
		t.Errorf("Check = %+v, %v; want no prerequisite missing", r, err)
	}
	var started []string
	done, err := svc.Upgrade(ctx, func(s dbupgrade.Step) { started = append(started, s.Key) })
	if err == nil || len(done) != 0 || !slices.Equal(started, []string{"test.backfill"}) {
		t.Fatalf("Upgrade interrupted = %v, %v (started %v); want the backfill to fail", done, err, started)
	}
	if len(rebuilds) != 0 {
		t.Errorf("projections rebuilt after a failed run: %v", rebuilds)
	}
	if err := svc.Ready(ctx); !errors.Is(err, dbupgrade.ErrNotUpgraded) {
		t.Errorf("Ready after the failure = %v, want ErrNotUpgraded", err)
	}

	failAfter = 3
	clock.Advance(time.Minute)
	started = nil
	done, err = svc.Upgrade(ctx, func(s dbupgrade.Step) { started = append(started, s.Key) })
	if err != nil || len(done) != 2 || !slices.Equal(started, []string{"test.backfill", "test.retired"}) {
		t.Fatalf("Upgrade again = %v, %v (started %v); want the backfill and the retired step", done, err, started)
	}
	if want := []string{"test.backfill.batch1", "test.backfill.batch2", "test.backfill.batch3"}; !slices.Equal(batches, want) {
		t.Errorf("batches = %v, want %v, each once", batches, want)
	}
	if !slices.Equal(rebuilds, []string{"p1", "p2"}) {
		t.Errorf("rebuilds = %v, want p1 and p2", rebuilds)
	}
	if err := svc.Ready(ctx); err != nil {
		t.Errorf("Ready after the upgrade = %v", err)
	}

	// The second run finds nothing to do.
	started = nil
	if done, err := svc.Upgrade(ctx, func(s dbupgrade.Step) { started = append(started, s.Key) }); err != nil || len(done) != 0 || len(started) != 0 {
		t.Errorf("Upgrade a second time = %v, %v (started %v); want nothing", done, err, started)
	}
	runs, err := h.Repository.Runs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.Key == "test.backfill" && !r.CompletedAt.Equal(dataUpgradeStart.Add(time.Minute)) {
			t.Errorf("test.backfill completed at %v, want %v", r.CompletedAt, dataUpgradeStart.Add(time.Minute))
		}
	}
}
