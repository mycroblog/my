// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"slices"
	"testing"
	"time"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// HookHarness is what HookRepositoryContract needs from a database adapter.
type HookHarness struct {
	UnitOfWork uow.UnitOfWork
	Repository apphook.Repository
}

// HookRepositoryContract verifies the hook repository of a database adapter (ADR-0147 §6).
func HookRepositoryContract(t *testing.T, newHarness func(t *testing.T) HookHarness) {
	t.Helper()
	base := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	ids := NewIDGenerator(NewClock(base))
	decision := func(n hook.Name, key string, at time.Time) hook.Decision {
		return hook.Decision{
			ID: ids.NewID(), Hook: n, SubjectKey: key, Mode: hook.ModeEnforce, Outcome: hook.Allow,
			Latency: 25 * time.Millisecond, DecidedAt: at,
		}
	}
	insert := func(t *testing.T, h HookHarness, d hook.Decision) bool {
		t.Helper()
		var inserted bool
		err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			var err error
			inserted, err = h.Repository.InsertDecision(t.Context(), tx, d)
			return err
		})
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
		return inserted
	}

	t.Run("one decision per hook and subject", func(t *testing.T) {
		h := newHarness(t)
		d := decision(hook.InboundStatus, "https://remote.example/a", base)
		d.AccountID, d.Outcome, d.Reason, d.Note = ids.NewID(), hook.Drop, hook.ReasonSpam, "a note"
		if !insert(t, h, d) {
			t.Fatal("the first decision was not inserted")
		}
		if insert(t, h, decision(hook.InboundStatus, d.SubjectKey, base)) {
			t.Error("a second decision on the subject was inserted")
		}
		if !insert(t, h, decision(hook.InboundFollow, d.SubjectKey, base)) {
			t.Error("another hook's decision on the subject was refused")
		}
		got, err := h.Repository.RecordBySubject(t.Context(), hook.InboundStatus, d.SubjectKey)
		if err != nil {
			t.Fatal(err)
		}
		if got.Override != nil || got.Decision.ID != d.ID || got.Decision.AccountID != d.AccountID || got.Decision.Outcome != hook.Drop ||
			got.Decision.Reason != hook.ReasonSpam || got.Decision.Note != "a note" || got.Decision.Latency != d.Latency || !got.Decision.DecidedAt.Equal(base) {
			t.Errorf("read back %+v, want %+v", got, d)
		}
		if _, err := h.Repository.RecordBySubject(t.Context(), hook.InboundStatus, "none"); !errors.Is(err, apphook.ErrNotFound) {
			t.Errorf("an unknown subject: %v", err)
		}
	})

	t.Run("a decision is cancelled once", func(t *testing.T) {
		h := newHarness(t)
		d := decision(hook.InboundStatus, "k", base)
		insert(t, h, d)
		by := ids.NewID()
		for i, want := range []bool{true, false} {
			err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
				got, err := h.Repository.InsertOverride(t.Context(), tx, hook.Override{DecisionID: d.ID, ByUserID: by, At: base})
				if got != want {
					t.Errorf("override %d: inserted = %v", i, got)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			rec, err := h.Repository.RecordTx(t.Context(), tx, d.ID)
			if err != nil {
				return err
			}
			if rec.Override == nil || rec.Override.ByUserID != by || !rec.Override.At.Equal(base) {
				t.Errorf("override %+v", rec.Override)
			}
			if _, err := h.Repository.RecordTx(t.Context(), tx, ids.NewID()); !errors.Is(err, apphook.ErrNotFound) {
				t.Errorf("an unknown decision: %v", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the list is newest first, filtered, and leaves out allow", func(t *testing.T) {
		h := newHarness(t)
		allow := decision(hook.InboundStatus, "allow", base)
		first := decision(hook.InboundStatus, "first", base)
		first.Outcome = hook.Drop
		second := decision(hook.InboundFollow, "second", base.Add(time.Second))
		second.Outcome, second.Mode = hook.FailedTimeout, hook.ModeObserve
		for _, d := range []hook.Decision{allow, first, second} {
			insert(t, h, d)
		}
		list := func(q apphook.ListQuery) []string {
			t.Helper()
			q.Limit = 10
			records, err := h.Repository.ListRecords(t.Context(), q)
			if err != nil {
				t.Fatal(err)
			}
			keys := []string{}
			for _, r := range records {
				keys = append(keys, r.Decision.SubjectKey)
			}
			return keys
		}
		newestFirst := []string{"second", "first"}
		if second.ID < first.ID {
			newestFirst = []string{"first", "second"}
		}
		if got := list(apphook.ListQuery{}); !slices.Equal(got, newestFirst) {
			t.Errorf("list = %v, want %v", got, newestFirst)
		}
		if got := list(apphook.ListQuery{Outcome: hook.Allow}); !slices.Equal(got, []string{"allow"}) {
			t.Errorf("allow = %v", got)
		}
		if got := list(apphook.ListQuery{Hook: hook.InboundFollow}); !slices.Equal(got, []string{"second"}) {
			t.Errorf("hook = %v", got)
		}
		if got := list(apphook.ListQuery{Mode: hook.ModeObserve}); !slices.Equal(got, []string{"second"}) {
			t.Errorf("mode = %v", got)
		}
		before := first.ID
		if second.ID > before {
			before = second.ID
		}
		if got := list(apphook.ListQuery{Before: before}); !slices.Equal(got, newestFirst[1:]) {
			t.Errorf("before = %v, want %v", got, newestFirst[1:])
		}
	})

	t.Run("the summary counts by hook, mode, and outcome since a time", func(t *testing.T) {
		h := newHarness(t)
		a := decision(hook.InboundStatus, "a", base)
		a.Latency = 10 * time.Millisecond
		b := decision(hook.InboundStatus, "b", base)
		b.Latency = 30 * time.Millisecond
		old := decision(hook.InboundStatus, "old", base.Add(-time.Hour))
		for _, d := range []hook.Decision{a, b, old} {
			insert(t, h, d)
		}
		rows, err := h.Repository.Summarize(t.Context(), base)
		if err != nil {
			t.Fatal(err)
		}
		want := apphook.SummaryRow{
			Hook: hook.InboundStatus, Mode: hook.ModeEnforce, Outcome: hook.Allow, Decisions: 2,
			TotalLatency: 40 * time.Millisecond, MaxLatency: 30 * time.Millisecond,
		}
		if len(rows) != 1 || rows[0] != want {
			t.Errorf("summary = %+v, want %+v", rows, want)
		}
	})

	t.Run("the expired decisions come in batches with their statuses, and go with their children", func(t *testing.T) {
		h := newHarness(t)
		old := make([]hook.Decision, 3)
		for i := range old {
			old[i] = decision(hook.InboundStatus, string(rune('a'+i)), base.Add(time.Duration(i)*time.Minute))
			old[i].AccountID = ids.NewID()
			insert(t, h, old[i])
		}
		recent := decision(hook.InboundStatus, "recent", base.Add(time.Hour))
		insert(t, h, recent)
		statusA, statusB := ids.NewID(), ids.NewID()
		err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			for _, s := range []string{statusA, statusB, statusA} {
				if err := h.Repository.InsertDecisionStatus(t.Context(), tx, old[0].ID, s); err != nil {
					return err
				}
			}
			_, err := h.Repository.InsertOverride(t.Context(), tx, hook.Override{DecisionID: old[1].ID, At: base})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		q := apphook.ExpiredQuery{Before: base.Add(30 * time.Minute), Limit: 2}
		page, err := h.Repository.ListExpired(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 2 || page[0].ID != old[0].ID || page[1].ID != old[1].ID {
			t.Fatalf("first batch %+v", page)
		}
		want := []string{statusA, statusB}
		slices.Sort(want)
		got := slices.Clone(page[0].StatusIDs)
		slices.Sort(got)
		if !slices.Equal(got, want) || page[0].AccountID != old[0].AccountID || page[0].Overridden {
			t.Errorf("first decision %+v", page[0])
		}
		if !page[1].Overridden || len(page[1].StatusIDs) != 0 {
			t.Errorf("second decision %+v", page[1])
		}
		q.AfterAt, q.AfterID = page[1].DecidedAt, page[1].ID
		page, err = h.Repository.ListExpired(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 1 || page[0].ID != old[2].ID {
			t.Fatalf("second batch %+v", page)
		}
		err = h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			return h.Repository.DeleteDecisions(t.Context(), tx, []string{old[0].ID, old[1].ID})
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Repository.RecordBySubject(t.Context(), hook.InboundStatus, old[0].SubjectKey); !errors.Is(err, apphook.ErrNotFound) {
			t.Errorf("after the delete: %v", err)
		}
		// The rows of a deleted decision's children are gone: the same IDs can be written again.
		err = h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			ok, err := h.Repository.InsertOverride(t.Context(), tx, hook.Override{DecisionID: old[1].ID, At: base})
			if !ok {
				t.Error("the override of the deleted decision stayed")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the decisions of an account", func(t *testing.T) {
		h := newHarness(t)
		account := ids.NewID()
		for i := range 3 {
			d := decision(hook.InboundStatus, string(rune('a'+i)), base)
			d.AccountID = account
			insert(t, h, d)
		}
		insert(t, h, decision(hook.InboundStatus, "none", base))
		err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			found, err := h.Repository.DecisionIDsOfAccount(t.Context(), tx, account, 2)
			if len(found) != 2 {
				t.Errorf("found %v", found)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// HookPluginHarness is what HookPluginContract needs: an adapter of the port and a way to make the
// plugin behind it answer or fail.
type HookPluginHarness struct {
	Plugin apphook.Plugin
	// Answer makes the plugin return the verdict, reason, and note.
	Answer func(a apphook.Answer)
	// Broken makes the plugin return an empty answer, nil when the adapter cannot get one.
	Broken func()
	// Fail makes the plugin fail in a way that is no timeout.
	Fail func()
	// Block makes the plugin hold every call until the caller's deadline, nil when it cannot.
	Block func()
}

// HookPluginContract verifies an adapter of the hook Plugin port (ADR-0147, ADR-0146 §10).
func HookPluginContract(t *testing.T, newHarness func(t *testing.T) HookPluginHarness) {
	t.Helper()
	request := apphook.Request{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/a"}

	t.Run("returns the verdict, reason, and note", func(t *testing.T) {
		h := newHarness(t)
		for _, verdict := range []hook.Outcome{hook.Allow, hook.Drop, hook.Flag, hook.MarkSensitive, hook.RequireApproval, hook.Reject} {
			want := apphook.Answer{Verdict: verdict, Reason: hook.ReasonSpam, Note: "a note"}
			h.Answer(want)
			got, err := h.Plugin.Check(t.Context(), request)
			if err != nil {
				t.Fatalf("%s: %v", verdict, err)
			}
			if got != want {
				t.Errorf("%s: got %+v", verdict, got)
			}
		}
	})

	t.Run("an empty answer is invalid, never a verdict", func(t *testing.T) {
		h := newHarness(t)
		if h.Broken == nil {
			t.Skip("this adapter cannot get an empty answer")
		}
		h.Broken()
		if _, err := h.Plugin.Check(t.Context(), request); !errors.Is(err, apphook.ErrPluginInvalid) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a failure is a failure of the port", func(t *testing.T) {
		h := newHarness(t)
		h.Fail()
		if _, err := h.Plugin.Check(t.Context(), request); !errors.Is(err, apphook.ErrPluginFailed) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a call past its deadline is a timeout", func(t *testing.T) {
		h := newHarness(t)
		if h.Block == nil {
			t.Skip("this adapter cannot be held")
		}
		h.Block()
		if _, err := h.Plugin.Check(t.Context(), request); !errors.Is(err, apphook.ErrPluginTimeout) {
			t.Errorf("err = %v", err)
		}
	})
}
