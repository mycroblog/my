// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AuditLogContract verifies the audit log of a database adapter's moderation repository by its
// methods alone, the port contract of ADR-0153 §1: an entry rolls back with its transaction; the
// schema accepts every action, kind of actor, and kind of target; the log lists newest first,
// filtered by actor, action, kind of target, and target, in keyset pages; and it tells which
// reports, appeals, and sign-up blocks among some IDs exist, in ID order. The audit of the use
// cases on top of it is AuditLogFlows, which internal/flowtest/moderation runs once on SQLite.
func AuditLogContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"RollsBackWithItsTransaction", auditRepoRollsBack},
		{"AcceptsEveryActionActorAndTarget", auditRepoAcceptsEverything},
		{"ListsWithFiltersAndPages", auditRepoLists},
		{"TellsTheTargetsThatExist", auditRepoExistingTargets},
	})
}

// auditEntry inserts an audit log entry, changed by edit, a second before the next.
func (r *modRepo) auditEntry(t *testing.T, edit func(*moderation.AuditEntry)) moderation.AuditEntry {
	t.Helper()
	e := moderation.AuditEntry{
		ID: r.ids.NewID(), ActorKind: moderation.AuditActorCLI, Action: moderation.AuditSettingChanged,
		TargetKind: moderation.AuditTargetSetting, TargetID: "registration_mode", TargetLabel: "registration_mode", OccurredAt: r.clock.Now(),
	}
	if edit != nil {
		edit(&e)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertAuditEntry(ctx, tx, e) })
	r.clock.Advance(time.Second)
	return e
}

func (r *modRepo) auditLog(t *testing.T, q appmoderation.AuditQuery, limit int) []moderation.AuditEntry {
	t.Helper()
	got, err := r.h.Moderation.ListAuditEntries(t.Context(), q, limit)
	if err != nil {
		t.Fatalf("ListAuditEntries(%+v) = %v", q, err)
	}
	return got
}

func auditRepoRollsBack(t *testing.T, r *modRepo) {
	errBoom := errors.New("boom")
	err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Moderation.InsertAuditEntry(ctx, tx, moderation.AuditEntry{
			ID: r.ids.NewID(), ActorKind: moderation.AuditActorCLI, Action: moderation.AuditSettingChanged,
			TargetKind: moderation.AuditTargetSetting, TargetID: "x", OccurredAt: r.clock.Now(),
		}); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("Tx = %v, want the command's error", err)
	}
	if got := r.auditLog(t, appmoderation.AuditQuery{}, 10); len(got) != 0 {
		t.Errorf("audit log after a rollback = %+v, want empty", got)
	}
	e := r.auditEntry(t, func(e *moderation.AuditEntry) {
		e.ActorKind, e.ActorUserID, e.SourceEventID = moderation.AuditActorUser, NamedID("mod-1"), r.ids.NewID()
	})
	if got := r.auditLog(t, appmoderation.AuditQuery{}, 10); !reflect.DeepEqual(got, []moderation.AuditEntry{e}) {
		t.Errorf("audit log = %+v, want %+v", got, e)
	}
}

func auditRepoAcceptsEverything(t *testing.T, r *modRepo) {
	n := 0
	for _, a := range moderation.AuditActions() {
		kind, _ := a.Target()
		r.auditEntry(t, func(e *moderation.AuditEntry) { e.Action, e.TargetKind, e.TargetID = a, kind, "x" })
		n++
	}
	for _, k := range moderation.AuditTargetKinds() {
		for _, actor := range []moderation.AuditActorKind{moderation.AuditActorSystem, moderation.AuditActorRemote, moderation.AuditActorCLI} {
			r.auditEntry(t, func(e *moderation.AuditEntry) {
				e.ActorKind, e.Action, e.TargetKind, e.TargetID, e.TargetLabel = actor, moderation.AuditRuleAdded, k, "x", ""
			})
			n++
		}
	}
	if got := r.auditLog(t, appmoderation.AuditQuery{}, n+1); len(got) != n {
		t.Errorf("audit log = %d entries, want %d", len(got), n)
	}
}

func auditRepoLists(t *testing.T, r *modRepo) {
	users := []string{NamedID("mod-1"), NamedID("mod-2"), ""}
	actions := []moderation.AuditAction{moderation.AuditRuleAdded, moderation.AuditRuleMoved, moderation.AuditSettingChanged}
	var all []moderation.AuditEntry
	for i := range 9 {
		a := actions[i%3]
		kind, _ := a.Target()
		actor, user := moderation.AuditActor(users[(i/3)%3])
		all = append(all, r.auditEntry(t, func(e *moderation.AuditEntry) {
			e.ActorKind, e.ActorUserID, e.Action, e.TargetKind = actor, user, a, kind
			e.TargetID, e.TargetLabel, e.SourceEventID = fmt.Sprintf("target-%d", i%2), fmt.Sprintf("label %d", i), r.ids.NewID()
		}))
	}
	newest := func(keep func(moderation.AuditEntry) bool) []moderation.AuditEntry {
		var out []moderation.AuditEntry
		for i := len(all) - 1; i >= 0; i-- {
			if keep(all[i]) {
				out = append(out, all[i])
			}
		}
		return out
	}
	for _, c := range []struct {
		name string
		q    appmoderation.AuditQuery
		keep func(moderation.AuditEntry) bool
	}{
		{"every entry", appmoderation.AuditQuery{}, func(moderation.AuditEntry) bool { return true }},
		{"by mod-1", appmoderation.AuditQuery{ActorUserID: NamedID("mod-1")}, func(x moderation.AuditEntry) bool { return x.ActorUserID == NamedID("mod-1") }},
		{"by action", appmoderation.AuditQuery{Action: moderation.AuditRuleMoved}, func(x moderation.AuditEntry) bool { return x.Action == moderation.AuditRuleMoved }},
		{"on rules", appmoderation.AuditQuery{TargetKind: moderation.AuditTargetRule}, func(x moderation.AuditEntry) bool { return x.TargetKind == moderation.AuditTargetRule }},
		{"on a rule", appmoderation.AuditQuery{TargetKind: moderation.AuditTargetRule, TargetID: "target-1"}, func(x moderation.AuditEntry) bool {
			return x.TargetKind == moderation.AuditTargetRule && x.TargetID == "target-1"
		}},
		{"by mod-2 on settings", appmoderation.AuditQuery{ActorUserID: NamedID("mod-2"), TargetKind: moderation.AuditTargetSetting}, func(x moderation.AuditEntry) bool {
			return x.ActorUserID == NamedID("mod-2") && x.TargetKind == moderation.AuditTargetSetting
		}},
	} {
		want := newest(c.keep)
		if got := r.auditLog(t, c.q, 100); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ListAuditEntries = %+v, want %+v", c.name, got, want)
		}
	}
	want := newest(func(moderation.AuditEntry) bool { return true })
	first := r.auditLog(t, appmoderation.AuditQuery{}, 4)
	if !reflect.DeepEqual(first, want[:4]) {
		t.Fatalf("first page = %+v, want %+v", first, want[:4])
	}
	if second := r.auditLog(t, appmoderation.AuditQuery{Before: first[3].ID}, 4); !reflect.DeepEqual(second, want[4:8]) {
		t.Errorf("second page = %+v, want %+v", second, want[4:8])
	}
	if last := r.auditLog(t, appmoderation.AuditQuery{Before: want[7].ID}, 4); !reflect.DeepEqual(last, want[8:]) {
		t.Errorf("last page = %+v, want %+v", last, want[8:])
	}
}

func auditRepoExistingTargets(t *testing.T, r *modRepo) {
	ctx := t.Context()
	a, b := r.report(t, r.ids.NewID(), r.ids.NewID(), nil), r.report(t, r.ids.NewID(), r.ids.NewID(), nil)
	action := r.action(t, r.ids.NewID(), moderation.ActionSuspend, nil)
	appeal := moderation.Appeal{ID: r.ids.NewID(), ActionID: action.ID, AccountID: action.TargetAccountID, Text: "no", SubmittedAt: r.clock.Now()}
	block := moderation.SignupBlock{
		ID: r.ids.NewID(), Kind: moderation.SignupBlockEmailDomain, Value: "spam.example", Severity: moderation.SignupReject, CreatedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Moderation.InsertAppeal(ctx, tx, appeal); err != nil {
			return err
		}
		return r.h.Moderation.InsertSignupBlock(ctx, tx, block)
	})
	missing := r.ids.NewID()
	sorted := func(ids ...string) []string { slices.Sort(ids); return ids }
	for _, c := range []struct {
		kind moderation.AuditTargetKind
		ids  []string
		want []string
	}{
		{moderation.AuditTargetReport, []string{b.ID, missing, a.ID}, sorted(a.ID, b.ID)},
		{moderation.AuditTargetAppeal, []string{missing, appeal.ID}, []string{appeal.ID}},
		{moderation.AuditTargetSignupBlock, []string{block.ID, missing}, []string{block.ID}},
		{moderation.AuditTargetReport, []string{appeal.ID, block.ID}, nil},
		{moderation.AuditTargetReport, nil, nil},
	} {
		got, err := r.h.Moderation.ExistingAuditTargets(ctx, c.kind, c.ids)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("ExistingAuditTargets(%s, %v) = %v, %v; want %v", c.kind, c.ids, got, err, c.want)
		}
	}
}
