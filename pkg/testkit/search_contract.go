// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/app/uow"
)

// SearchHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the search module's repository on it.
type SearchHarness struct {
	UnitOfWork uow.UnitOfWork
	Repository search.Repository
}

func searchRebuildID(n int) string {
	return fmt.Sprintf("0197b000-0000-7000-8000-%012d", n)
}

// SearchRepositoryContract verifies the search repository of a database adapter (ADR-0149 §10): the
// one rebuild starts only when none is unfinished; a step applies only from where the rebuild was
// read; the failures go with the next rebuild; and deleting takes everything.
func SearchRepositoryContract(t *testing.T, newHarness func(t *testing.T) SearchHarness) {
	t.Helper()
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	first := search.Rebuild{ID: searchRebuildID(1), Mode: search.RebuildClear, Phase: search.RebuildClearing, Rate: 20, StartedAt: at}

	tx := func(t *testing.T, h SearchHarness, fn func(ctx context.Context, tx uow.Tx) error) {
		t.Helper()
		if err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
			t.Fatal(err)
		}
	}
	start := func(t *testing.T, h SearchHarness, r search.Rebuild) bool {
		t.Helper()
		var started bool
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			var err error
			started, err = h.Repository.StartRebuild(ctx, tx, r)
			return err
		})
		return started
	}
	step := func(t *testing.T, h SearchHarness, s search.RebuildStep) bool {
		t.Helper()
		var moved bool
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			var err error
			moved, err = h.Repository.StepRebuild(ctx, tx, s)
			return err
		})
		return moved
	}
	find := func(t *testing.T, h SearchHarness) search.Rebuild {
		t.Helper()
		r, ok, err := h.Repository.FindRebuild(t.Context())
		if err != nil || !ok {
			t.Fatalf("FindRebuild = %v, %v", ok, err)
		}
		return r
	}

	t.Run("NoRebuild", func(t *testing.T) {
		h := newHarness(t)
		if _, ok, err := h.Repository.FindRebuild(t.Context()); ok || err != nil {
			t.Errorf("FindRebuild = %v, %v; want none", ok, err)
		}
	})

	t.Run("OneUnfinishedRebuild", func(t *testing.T) {
		h := newHarness(t)
		if !start(t, h, first) {
			t.Fatal("the first rebuild did not start")
		}
		second := first
		second.ID, second.Mode, second.Phase = searchRebuildID(2), search.RebuildKeep, search.RebuildFilling
		if start(t, h, second) {
			t.Error("a second rebuild started beside an unfinished one")
		}
		got := find(t, h)
		if got.ID != first.ID || got.Mode != search.RebuildClear || got.Phase != search.RebuildClearing || got.After != "" ||
			got.Rate != 20 || got.Processed != 0 || got.Failed != 0 || got.LastError != "" || !got.StartedAt.Equal(at) || !got.UpdatedAt.Equal(at) {
			t.Errorf("FindRebuild = %+v", got)
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error { return h.Repository.SetRebuildRate(ctx, tx, first.ID, 50) })
		if got := find(t, h); got.Rate != 50 {
			t.Errorf("Rate = %d, want 50", got.Rate)
		}
	})

	t.Run("StepsOnlyFromWhereItWasRead", func(t *testing.T) {
		h := newHarness(t)
		start(t, h, first)
		later := at.Add(time.Minute)
		s := search.RebuildStep{
			ID: first.ID, FromPhase: search.RebuildClearing, Phase: search.RebuildFilling, After: searchRebuildID(10),
			Processed: 2, Failed: 1, LastError: "boom", UpdatedAt: later,
		}
		if !step(t, h, s) {
			t.Fatal("the step did not apply")
		}
		if step(t, h, s) {
			t.Error("the same step applied twice")
		}
		other := s
		other.ID, other.FromPhase, other.FromAfter = searchRebuildID(2), search.RebuildFilling, searchRebuildID(10)
		if step(t, h, other) {
			t.Error("the step of another rebuild applied")
		}
		got := find(t, h)
		if got.Phase != search.RebuildFilling || got.After != searchRebuildID(10) || got.Processed != 2 || got.Failed != 1 ||
			got.LastError != "boom" || !got.UpdatedAt.Equal(later) {
			t.Errorf("after the step = %+v", got)
		}
		s = search.RebuildStep{
			ID: first.ID, FromPhase: search.RebuildFilling, FromAfter: searchRebuildID(10), Phase: search.RebuildFilling,
			After: searchRebuildID(11), Processed: 3, UpdatedAt: later,
		}
		if !step(t, h, s) {
			t.Fatal("the second step did not apply")
		}
		if got := find(t, h); got.Processed != 5 || got.Failed != 1 || got.LastError != "" {
			t.Errorf("the counters do not add up: %+v", got)
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.SetRebuildError(ctx, tx, first.ID, "down", later.Add(time.Minute))
		})
		if got := find(t, h); got.LastError != "down" || !got.UpdatedAt.Equal(later.Add(time.Minute)) || got.After != searchRebuildID(11) {
			t.Errorf("after SetRebuildError = %+v", got)
		}
	})

	t.Run("FailuresGoWithTheNextRebuild", func(t *testing.T) {
		h := newHarness(t)
		start(t, h, first)
		failures := []search.RebuildFailure{
			{StatusID: searchRebuildID(21), Message: "two", FailedAt: at},
			{StatusID: searchRebuildID(20), Message: "one", FailedAt: at},
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.AddRebuildFailures(ctx, tx, first.ID, failures)
		})
		failures[0].Message = "again"
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.AddRebuildFailures(ctx, tx, first.ID, failures)
		})
		got, err := h.Repository.ListRebuildFailures(t.Context(), first.ID, 10)
		if err != nil || len(got) != 2 || got[0].StatusID != searchRebuildID(20) || got[1].Message != "again" || !got[1].FailedAt.Equal(at) {
			t.Errorf("ListRebuildFailures = %+v, %v", got, err)
		}
		if got, err := h.Repository.ListRebuildFailures(t.Context(), first.ID, 1); err != nil || len(got) != 1 {
			t.Errorf("the limit gives %+v, %v", got, err)
		}
		step(t, h, search.RebuildStep{ID: first.ID, FromPhase: search.RebuildClearing, Phase: search.RebuildFinished, UpdatedAt: at})
		second := first
		second.ID = searchRebuildID(2)
		if !start(t, h, second) {
			t.Fatal("a new rebuild did not start after the last finished")
		}
		if got := find(t, h); got.ID != second.ID || got.Phase != search.RebuildClearing {
			t.Errorf("FindRebuild = %+v", got)
		}
		if got, err := h.Repository.ListRebuildFailures(t.Context(), first.ID, 10); err != nil || len(got) != 0 {
			t.Errorf("the failures of the finished rebuild stayed: %+v, %v", got, err)
		}
	})

	t.Run("DeleteTakesEverything", func(t *testing.T) {
		h := newHarness(t)
		start(t, h, first)
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.AddRebuildFailures(ctx, tx, first.ID, []search.RebuildFailure{{StatusID: searchRebuildID(20), Message: "one", FailedAt: at}})
		})
		tx(t, h, func(ctx context.Context, tx uow.Tx) error { return h.Repository.DeleteRebuild(ctx, tx) })
		if _, ok, err := h.Repository.FindRebuild(t.Context()); ok || err != nil {
			t.Errorf("FindRebuild = %v, %v; want none", ok, err)
		}
		if got, err := h.Repository.ListRebuildFailures(t.Context(), first.ID, 10); err != nil || len(got) != 0 {
			t.Errorf("ListRebuildFailures = %+v, %v", got, err)
		}
		if !start(t, h, first) {
			t.Error("a rebuild did not start after the delete")
		}
	})
}
