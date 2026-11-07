// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AppealContract verifies the appeals of a database adapter's moderation repository by its
// methods alone, the port contract of ADR-0153 §1: an action has one appeal and an appeal one
// decision, each read back as inserted inside and outside a transaction; the appeals of some
// actions; and the appeals listed newest first with their decisions, by state, in keyset pages,
// and counted by state. The use cases on top of it are AppealFlows, which
// internal/flowtest/moderation runs once on SQLite.
func AppealContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"RecordsOneAppealAndOneDecision", appealRepoOnce},
		{"ListsAndCounts", appealRepoLists},
	})
}

// appeal inserts a pending appeal of the action with its state.
func (r *modRepo) appeal(t *testing.T, a moderation.Action) moderation.Appeal {
	t.Helper()
	ap := moderation.Appeal{ID: r.ids.NewID(), ActionID: a.ID, AccountID: a.TargetAccountID, Text: "it was not spam", SubmittedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Moderation.InsertAppeal(ctx, tx, ap),
			r.h.Moderation.UpsertAppealStatus(ctx, tx, moderation.AppealStatus{AppealID: ap.ID, State: moderation.AppealPending, Since: ap.SubmittedAt}),
		)
	})
	r.clock.Advance(time.Second)
	return ap
}

// decide records a decision on the appeal and moves its state to it.
func (r *modRepo) decide(t *testing.T, appealID string, kind moderation.AppealDecisionKind) moderation.AppealDecision {
	t.Helper()
	d := moderation.AppealDecision{AppealID: appealID, Decision: kind, PublicReason: "we looked again", DecidedByUserID: NamedID("mod-1"), DecidedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Moderation.InsertAppealDecision(ctx, tx, d),
			r.h.Moderation.UpsertAppealStatus(ctx, tx, moderation.AppealStatus{AppealID: appealID, State: kind.State(), Since: d.DecidedAt}),
		)
	})
	r.clock.Advance(time.Second)
	return d
}

func appealRepoOnce(t *testing.T, r *modRepo) {
	ctx := t.Context()
	a := r.action(t, r.ids.NewID(), moderation.ActionSuspend, nil)
	ap := r.appeal(t, a)
	read := func(name string, want moderation.AppealRecord) {
		t.Helper()
		if got, err := r.h.Moderation.FindAppeal(ctx, ap.ID); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: FindAppeal = %+v, %v; want %+v", name, got, err, want)
		}
		var got moderation.AppealRecord
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.h.Moderation.Appeal(ctx, tx, ap.ID)
			return err
		})
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Appeal = %+v, want %+v", name, got, want)
		}
	}
	read("pending", moderation.AppealRecord{Appeal: ap})

	err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertAppeal(ctx, tx, moderation.Appeal{ID: r.ids.NewID(), ActionID: a.ID, AccountID: a.TargetAccountID, Text: "again", SubmittedAt: r.clock.Now()})
	})
	if !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertAppeal of an appealed action = %v, want ErrAlreadyRecorded", err)
	}

	d := r.decide(t, ap.ID, moderation.AppealDecisionUpheld)
	read("decided", moderation.AppealRecord{Appeal: ap, Decision: &d})
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertAppealDecision(ctx, tx, moderation.AppealDecision{AppealID: ap.ID, Decision: moderation.AppealDecisionOverturned, DecidedAt: r.clock.Now()})
	})
	if !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertAppealDecision of a decided appeal = %v, want ErrAlreadyRecorded", err)
	}
	read("after a second decision", moderation.AppealRecord{Appeal: ap, Decision: &d})

	unknown := r.ids.NewID()
	if _, err := r.h.Moderation.FindAppeal(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindAppeal(unknown) = %v, want ErrNotFound", err)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Moderation.Appeal(ctx, tx, unknown)
		return err
	})
	if !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("Appeal(unknown) = %v, want ErrNotFound", err)
	}
}

func appealRepoLists(t *testing.T, r *modRepo) {
	ctx := t.Context()
	account := r.ids.NewID()
	var actions []moderation.Action
	for range 4 {
		actions = append(actions, r.action(t, account, moderation.ActionLimit, nil))
	}
	unappealed := r.action(t, account, moderation.ActionWarn, nil)
	pending1, upheld, overturned, pending2 := r.appeal(t, actions[0]), r.appeal(t, actions[1]), r.appeal(t, actions[2]), r.appeal(t, actions[3])
	upheldDecision := r.decide(t, upheld.ID, moderation.AppealDecisionUpheld)
	r.decide(t, overturned.ID, moderation.AppealDecisionOverturned)

	ids := func(recs []moderation.AppealRecord) []string {
		var out []string
		for _, rec := range recs {
			out = append(out, rec.Appeal.ID)
		}
		return out
	}
	for _, c := range []struct {
		name  string
		q     appmoderation.AppealQuery
		limit int
		want  []string
	}{
		{"every appeal", appmoderation.AppealQuery{}, 10, []string{pending2.ID, overturned.ID, upheld.ID, pending1.ID}},
		{"pending", appmoderation.AppealQuery{State: moderation.AppealPending}, 10, []string{pending2.ID, pending1.ID}},
		{"upheld", appmoderation.AppealQuery{State: moderation.AppealUpheld}, 10, []string{upheld.ID}},
		{"a page", appmoderation.AppealQuery{}, 2, []string{pending2.ID, overturned.ID}},
		{"the next page", appmoderation.AppealQuery{Before: overturned.ID}, 2, []string{upheld.ID, pending1.ID}},
		{"pending, the next page", appmoderation.AppealQuery{State: moderation.AppealPending, Before: pending2.ID}, 2, []string{pending1.ID}},
	} {
		got, err := r.h.Moderation.ListAppeals(ctx, c.q, c.limit)
		if err != nil || !slices.Equal(ids(got), c.want) {
			t.Errorf("ListAppeals(%s) = %v, %v; want %v", c.name, ids(got), err, c.want)
		}
	}
	got, err := r.h.Moderation.ListAppeals(ctx, appmoderation.AppealQuery{State: moderation.AppealUpheld}, 10)
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], moderation.AppealRecord{Appeal: upheld, Decision: &upheldDecision}) {
		t.Errorf("ListAppeals(upheld) = %+v, %v; want the appeal with its decision", got, err)
	}

	for state, n := range map[moderation.AppealState]int64{moderation.AppealPending: 2, moderation.AppealUpheld: 1, moderation.AppealOverturned: 1} {
		if got, err := r.h.Moderation.CountAppeals(ctx, state); err != nil || got != n {
			t.Errorf("CountAppeals(%s) = %d, %v; want %d", state, got, err, n)
		}
	}

	of, err := r.h.Moderation.AppealsOfActions(ctx, []string{actions[1].ID, unappealed.ID, actions[0].ID})
	slices.SortFunc(of, func(a, b moderation.AppealRecord) int { return strings.Compare(a.Appeal.ID, b.Appeal.ID) })
	if err != nil || !reflect.DeepEqual(of, []moderation.AppealRecord{{Appeal: pending1}, {Appeal: upheld, Decision: &upheldDecision}}) {
		t.Errorf("AppealsOfActions = %+v, %v; want the appeals of the two appealed actions", of, err)
	}
	if of, err := r.h.Moderation.AppealsOfActions(ctx, nil); err != nil || len(of) != 0 {
		t.Errorf("AppealsOfActions(none) = %+v, %v; want none", of, err)
	}
}
