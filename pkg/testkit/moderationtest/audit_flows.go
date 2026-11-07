// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// AuditLogFlows verifies the audit log of a database adapter's moderation repository (ADR-0058)
// through the moderation use cases and appmoderation.RegisterAuditProjector: the moderators'
// operations on domains and reports each record one entry in their transaction, with the actor (a
// user, or the CLI) and the target's label, and operations that change nothing record none; an
// entry rolls back with its transaction and an invalid one fails it; the schema accepts every
// action and kind of target; and the log lists newest first, filtered by actor, action, kind of
// target, and target, in keyset pages, with the labels it recorded whether the targets still exist
// or not; and it tells which accounts, reports, appeals, and sign-up blocks of its entries are
// gone. These are the flows of ADR-0153 §1, which internal/flowtest/moderation runs once on SQLite;
// AuditLogContract checks the repository's methods on every adapter.
func AuditLogFlows(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *auditEnv)
	}{
		{"RecordsTheModeratorsOperations", auditRecordsOperations},
		{"RollsBackWithItsTransaction", auditRollsBack},
		{"ListsWithFiltersAndPages", auditLists},
		{"TellsTheTargetsThatAreGone", auditTellsGoneTargets},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAuditEnv(t, newHarness)) })
	}
}

type auditEnv struct {
	h      ModerationHarness
	clock  *testkit.Clock
	ids    *testkit.IDGenerator
	svc    *appmoderation.Service
	remote string
	// others are the roles, custom emoji, and webhooks that exist, by kind.
	others stubAuditTargets
}

// stubAuditTargets is the appmoderation.AuditTargets of the other modules' targets: the IDs that
// exist, by kind.
type stubAuditTargets map[moderation.AuditTargetKind][]string

func (s stubAuditTargets) ExistingAuditTargets(_ context.Context, kind moderation.AuditTargetKind, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		if slices.Contains(s[kind], id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out, nil
}

func newAuditEnv(t *testing.T, newHarness ModerationFactory) *auditEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	clock := testkit.NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	ids := testkit.NewIDGenerator(clock)
	actors := &identitytest.StubActors{Actors: map[string]appaccount.Actor{}}
	e := &auditEnv{h: h, clock: clock, ids: ids, remote: ids.NewID(), others: stubAuditTargets{}}
	actors.Set(appaccount.Actor{
		Account: account.Account{ID: e.remote, Domain: "remote.example", URI: "https://remote.example/users/bob"},
		Handle:  &account.Handle{AccountID: e.remote, Username: "bob", Domain: "remote.example"},
	})
	e.svc = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: actors, Clock: clock, IDs: ids,
		AuditTargets: e.others,
	})
	return e
}

func (e *auditEnv) list(t *testing.T, q appmoderation.AuditQuery, limit int) []moderation.AuditEntry {
	t.Helper()
	got, err := e.svc.AuditLog(t.Context(), q, limit)
	if err != nil {
		t.Fatalf("AuditLog(%+v) = %v", q, err)
	}
	return got
}

// record writes an entry through the projector, in its own transaction.
func (e *auditEnv) record(t *testing.T, entry moderation.AuditEntry) moderation.AuditEntry {
	t.Helper()
	e.clock.Advance(time.Second)
	entry.ID, entry.OccurredAt = e.ids.NewID(), e.clock.Now()
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return tx.Emit(t.Context(), appmoderation.AuditRecorded{Entry: entry})
	})
	if err != nil {
		t.Fatalf("record %+v: %v", entry, err)
	}
	return entry
}

type auditWant struct {
	actor    moderation.AuditActorKind
	user     string
	action   moderation.AuditAction
	kind     moderation.AuditTargetKind
	target   string
	label    string
	source   string
	occurred time.Time
}

func checkAudit(t *testing.T, got []moderation.AuditEntry, want []auditWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("audit log = %d entries %+v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.ID == "" || g.ActorKind != w.actor || g.ActorUserID != w.user || g.Action != w.action || g.TargetKind != w.kind ||
			g.TargetID != w.target || g.TargetLabel != w.label || g.SourceEventID != w.source || !g.OccurredAt.Equal(w.occurred) {
			t.Errorf("entry %d = %+v, want %+v", i, g, w)
		}
	}
}

func auditRecordsOperations(t *testing.T, e *auditEnv) {
	ctx := t.Context()
	issued, err := e.svc.IssueDomainAction(ctx, appmoderation.DomainActionInput{
		Domain: "bad.example", Kind: moderation.DomainSuspend, PrivateNote: "not copied", IssuedByUserID: testkit.NamedID("mod-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	if err := e.svc.LiftDomainAction(ctx, issued.ID, ""); err != nil {
		t.Fatal(err)
	}
	liftedAt := e.clock.Now()
	// Lifting again changes nothing, so it records nothing.
	e.clock.Advance(time.Second)
	if err := e.svc.LiftDomainAction(ctx, issued.ID, testkit.NamedID("mod-1")); err != nil {
		t.Fatal(err)
	}
	report, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/1", ReporterAccountID: e.remote, TargetAccountID: e.remote,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	assignedAt := e.clock.Now()
	for _, by := range []string{testkit.NamedID("mod-1"), ""} { // the second assignment changes nothing
		if _, err := e.svc.AssignReport(ctx, report.ID, testkit.NamedID("mod-1"), by); err != nil {
			t.Fatal(err)
		}
	}
	e.clock.Advance(time.Second)
	unassignedAt := e.clock.Now()
	if _, err := e.svc.AssignReport(ctx, report.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	resolvedAt := e.clock.Now()
	for range 2 { // resolving a resolved report changes nothing
		if _, err := e.svc.ResolveReport(ctx, report.ID, moderation.OutcomeDismissed, testkit.NamedID("mod-1")); err != nil {
			t.Fatal(err)
		}
	}
	got := e.list(t, appmoderation.AuditQuery{}, 10)
	if len(got) != 5 {
		t.Fatalf("audit log = %+v, want 5 entries", got)
	}
	checkAudit(t, got, []auditWant{
		{moderation.AuditActorUser, testkit.NamedID("mod-1"), moderation.AuditReportResolved, moderation.AuditTargetReport, report.ID, "@bob@remote.example", got[0].SourceEventID, resolvedAt},
		{moderation.AuditActorCLI, "", moderation.AuditReportUnassigned, moderation.AuditTargetReport, report.ID, "@bob@remote.example", got[1].SourceEventID, unassignedAt},
		{moderation.AuditActorUser, testkit.NamedID("mod-1"), moderation.AuditReportAssigned, moderation.AuditTargetReport, report.ID, "@bob@remote.example", got[2].SourceEventID, assignedAt},
		{moderation.AuditActorCLI, "", moderation.AuditDomainActionLifted, moderation.AuditTargetDomain, "bad.example", "bad.example", issued.ID, liftedAt},
		{moderation.AuditActorUser, testkit.NamedID("mod-1"), moderation.AuditDomainActionIssued, moderation.AuditTargetDomain, "bad.example", "bad.example", issued.ID, issued.IssuedAt},
	})
	for i := range 3 {
		if got[i].SourceEventID == "" || got[i].SourceEventID == report.ID {
			t.Errorf("entry %d names no assignment or resolution: %+v", i, got[i])
		}
	}
	// An operation on an unknown report fails before it records anything.
	if _, err := e.svc.ResolveReport(ctx, e.ids.NewID(), moderation.OutcomeDismissed, testkit.NamedID("mod-1")); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ResolveReport(unknown) = %v, want ErrNotFound", err)
	}
	if n := len(e.list(t, appmoderation.AuditQuery{}, 10)); n != 5 {
		t.Errorf("audit log after the failure = %d entries, want 5", n)
	}

	// An action on an account and its lift, once.
	e.clock.Advance(time.Second)
	acted, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam, IssuedByUserID: testkit.NamedID("mod-2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	for _, by := range []string{testkit.NamedID("mod-1"), testkit.NamedID("mod-2")} {
		if err := e.svc.LiftAccountAction(ctx, acted.ID, by, ""); err != nil {
			t.Fatal(err)
		}
	}
	checkAudit(t, e.list(t, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetAccount, TargetID: e.remote}, 10), []auditWant{
		{moderation.AuditActorUser, testkit.NamedID("mod-1"), moderation.AuditAccountActionLifted, moderation.AuditTargetAccount, e.remote, "@bob@remote.example", acted.ID, e.clock.Now()},
		{moderation.AuditActorUser, testkit.NamedID("mod-2"), moderation.AuditAccountActionIssued, moderation.AuditTargetAccount, e.remote, "@bob@remote.example", acted.ID, acted.IssuedAt},
	})
}

func auditRollsBack(t *testing.T, e *auditEnv) {
	ctx := t.Context()
	errBoom := errors.New("boom")
	entry := moderation.AuditEntry{
		ID: e.ids.NewID(), ActorKind: moderation.AuditActorCLI, Action: moderation.AuditSettingChanged,
		TargetKind: moderation.AuditTargetSetting, TargetID: "registration_mode", TargetLabel: "registration_mode", OccurredAt: e.clock.Now(),
	}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := tx.Emit(ctx, appmoderation.AuditRecorded{Entry: entry}); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("Tx = %v, want the command's error", err)
	}
	if got := e.list(t, appmoderation.AuditQuery{}, 10); len(got) != 0 {
		t.Errorf("audit log after a rollback = %+v, want empty", got)
	}
	// An entry that the domain rejects fails the transaction that emits it.
	bad := entry
	bad.ActorUserID = testkit.NamedID("mod-1")
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: bad}) })
	if !errors.Is(err, moderation.ErrInvalidAuditEntry) {
		t.Errorf("Tx with an invalid entry = %v, want ErrInvalidAuditEntry", err)
	}
	// A label longer than the limit is cut, not refused.
	long := entry
	long.ID = e.ids.NewID()
	long.TargetLabel = fmt.Sprintf("%0*d", moderation.MaxAuditTargetLabel+10, 7)
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: long}) }); err != nil {
		t.Fatalf("Tx with a long label = %v", err)
	}
	if got := e.list(t, appmoderation.AuditQuery{}, 10); len(got) != 1 || len(got[0].TargetLabel) != moderation.MaxAuditTargetLabel {
		t.Errorf("audit log = %+v, want one entry with a cut label", got)
	}
}

func auditLists(t *testing.T, e *auditEnv) {
	// One entry per action, by two users and the CLI, on targets that do not exist in any table
	// (as after their erasure): the labels the entries recorded are what they list.
	var all []moderation.AuditEntry
	actors := []string{testkit.NamedID("mod-1"), testkit.NamedID("mod-2"), ""}
	for i, a := range moderation.AuditActions() {
		kind, _ := a.Target()
		actor, user := moderation.AuditActor(actors[i%len(actors)])
		all = append(all, e.record(t, moderation.AuditEntry{
			ActorKind: actor, ActorUserID: user, Action: a, TargetKind: kind,
			TargetID: fmt.Sprintf("target-%d", i%2), TargetLabel: fmt.Sprintf("label of %s", a), SourceEventID: e.ids.NewID(),
		}))
	}
	// Every kind of actor and target is accepted. These entries sort before the recorded ones.
	n := 0
	for _, k := range moderation.AuditTargetKinds() {
		for _, actor := range []moderation.AuditActorKind{moderation.AuditActorSystem, moderation.AuditActorRemote} {
			err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
				return e.h.Moderation.InsertAuditEntry(t.Context(), tx, moderation.AuditEntry{
					ID: fmt.Sprintf("00000000-0000-7000-8000-%012d", n), ActorKind: actor, Action: moderation.AuditRuleAdded, TargetKind: k,
					TargetID: "x", TargetLabel: "", OccurredAt: e.clock.Now(),
				})
			})
			if err != nil {
				t.Errorf("InsertAuditEntry(%s by %s) = %v", k, actor, err)
			}
			n++
		}
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
	check := func(name string, q appmoderation.AuditQuery, want []moderation.AuditEntry) {
		t.Helper()
		got := e.list(t, q, 100)
		if len(got) != len(want) {
			t.Errorf("%s: %d entries, want %d", name, len(got), len(want))
			return
		}
		for i := range want {
			if !reflect.DeepEqual(got[i], want[i]) {
				t.Errorf("%s: entry %d = %+v, want %+v", name, i, got[i], want[i])
			}
		}
	}
	check("by mod-1", appmoderation.AuditQuery{ActorUserID: testkit.NamedID("mod-1")}, newest(func(x moderation.AuditEntry) bool { return x.ActorUserID == testkit.NamedID("mod-1") }))
	check("by action", appmoderation.AuditQuery{Action: moderation.AuditRuleMoved}, newest(func(x moderation.AuditEntry) bool { return x.Action == moderation.AuditRuleMoved }))
	check("on rules", appmoderation.AuditQuery{TargetKind: moderation.AuditTargetRule, TargetID: "target-1"}, newest(func(x moderation.AuditEntry) bool {
		return x.TargetKind == moderation.AuditTargetRule && x.TargetID == "target-1"
	}))
	check("by mod-2 on settings", appmoderation.AuditQuery{ActorUserID: testkit.NamedID("mod-2"), TargetKind: moderation.AuditTargetSetting}, newest(func(x moderation.AuditEntry) bool {
		return x.ActorUserID == testkit.NamedID("mod-2") && x.TargetKind == moderation.AuditTargetSetting
	}))

	// Keyset pages, newest first.
	first := e.list(t, appmoderation.AuditQuery{}, 3)
	want := newest(func(moderation.AuditEntry) bool { return true })
	if !reflect.DeepEqual(first, want[:3]) {
		t.Fatalf("first page = %+v, want %+v", first, want[:3])
	}
	second := e.list(t, appmoderation.AuditQuery{Before: first[2].ID}, 3)
	if !reflect.DeepEqual(second, want[3:6]) {
		t.Errorf("second page = %+v, want %+v", second, want[3:6])
	}

	for _, q := range []appmoderation.AuditQuery{
		{Action: "domain_burned"}, {TargetKind: "planet"}, {TargetID: "target-1"},
	} {
		if _, err := e.svc.AuditLog(t.Context(), q, 10); !errors.Is(err, appmoderation.ErrInvalidQuery) {
			t.Errorf("AuditLog(%+v) = %v, want ErrInvalidQuery", q, err)
		}
	}
}

func auditTellsGoneTargets(t *testing.T, e *auditEnv) {
	ctx := t.Context()
	report, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/2", ReporterAccountID: e.remote, TargetAccountID: e.remote,
	})
	if err != nil {
		t.Fatal(err)
	}
	block, err := e.svc.AddSignupBlock(ctx, appmoderation.SignupBlockInput{
		Kind: moderation.SignupBlockEmailDomain, Value: "spam.example", Severity: moderation.SignupReject,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := func(kind moderation.AuditTargetKind, id string) moderation.AuditEntry {
		return moderation.AuditEntry{TargetKind: kind, TargetID: id}
	}
	missing := e.ids.NewID()
	role, emoji, hook, provider := e.ids.NewID(), e.ids.NewID(), e.ids.NewID(), e.ids.NewID()
	e.others[moderation.AuditTargetRole] = []string{role}
	e.others[moderation.AuditTargetCustomEmoji] = []string{emoji}
	e.others[moderation.AuditTargetWebhook] = []string{hook}
	e.others[moderation.AuditTargetLoginProvider] = []string{provider}
	e.others[moderation.AuditTargetLinkCardDomain] = []string{"ruled.example"}
	entries := []moderation.AuditEntry{
		entry(moderation.AuditTargetAccount, e.remote), entry(moderation.AuditTargetAccount, missing),
		entry(moderation.AuditTargetReport, report.ID), entry(moderation.AuditTargetReport, missing),
		entry(moderation.AuditTargetReport, missing), // a target of two entries is looked up once
		entry(moderation.AuditTargetAppeal, missing),
		entry(moderation.AuditTargetSignupBlock, block.ID), entry(moderation.AuditTargetSignupBlock, missing),
		// Kinds of targets that are never erased are never gone.
		entry(moderation.AuditTargetDomain, "gone.example"), entry(moderation.AuditTargetSetting, "registration_mode"),
		entry(moderation.AuditTargetUser, missing), entry(moderation.AuditTargetRule, missing),
		entry(moderation.AuditTargetInvite, missing),
		// The other modules' targets, deleted or not.
		entry(moderation.AuditTargetRole, role), entry(moderation.AuditTargetRole, missing),
		entry(moderation.AuditTargetCustomEmoji, emoji), entry(moderation.AuditTargetCustomEmoji, missing),
		entry(moderation.AuditTargetWebhook, hook), entry(moderation.AuditTargetWebhook, missing),
		entry(moderation.AuditTargetLoginProvider, provider), entry(moderation.AuditTargetLoginProvider, missing),
		entry(moderation.AuditTargetLinkCardDomain, "ruled.example"), entry(moderation.AuditTargetLinkCardDomain, "lifted.example"),
	}
	got, err := e.svc.GoneAuditTargets(ctx, entries)
	if err != nil {
		t.Fatal(err)
	}
	want := map[appmoderation.AuditTarget]bool{
		{Kind: moderation.AuditTargetAccount, ID: missing}:                 true,
		{Kind: moderation.AuditTargetReport, ID: missing}:                  true,
		{Kind: moderation.AuditTargetAppeal, ID: missing}:                  true,
		{Kind: moderation.AuditTargetSignupBlock, ID: missing}:             true,
		{Kind: moderation.AuditTargetRole, ID: missing}:                    true,
		{Kind: moderation.AuditTargetCustomEmoji, ID: missing}:             true,
		{Kind: moderation.AuditTargetWebhook, ID: missing}:                 true,
		{Kind: moderation.AuditTargetLoginProvider, ID: missing}:           true,
		{Kind: moderation.AuditTargetLinkCardDomain, ID: "lifted.example"}: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GoneAuditTargets = %v, want %v", got, want)
	}
	if got, err := e.svc.GoneAuditTargets(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("GoneAuditTargets(nil) = %v, %v; want none", got, err)
	}
}
