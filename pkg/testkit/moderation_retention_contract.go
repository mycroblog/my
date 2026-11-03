// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// ModerationRetentionContract verifies the retention of the moderation records of a database
// adapter's moderation repository through the moderation use cases (ADR-0038, data-model §4.3,
// §8): an operator places holds on an account, each released once; the purge erases the actions
// with their appeals once they ended longer ago than the period (an action without lasting
// effect when it was issued, a lifted or expired one at its end, an appealed one not before the
// decision; an indefinite one and one with a pending appeal stay), the resolved reports longer
// ago than the period that no action follows from (with their notes and history), and the
// audit log entries older than the period, all in bounded batches and safe to repeat; a hold
// keeps every record of its account until it is released; the purge of statuses keeps those
// that a report or an action points at and those of a held author; and the export of an
// account's records holds the private notes and never the reporter. The accounts and statuses
// are in-memory stand-ins.
func ModerationRetentionContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *retentionModEnv)
	}{
		{"HoldsAreReleasedOnce", retentionHolds},
		{"PurgesReleasedHolds", retentionPurgesReleasedHolds},
		{"ErasesSeverances", retentionErasesSeverances},
		{"PurgesActionsAfterTheyEnd", retentionPurgesActions},
		{"PurgesReportsAfterTheirResolution", retentionPurgesReports},
		{"HoldKeepsTheRecordsAndTheAuditLog", retentionHoldKeepsRecords},
		{"KeepsTheEvidenceOfStatuses", retentionKeepsEvidence},
		{"ExportsWithoutTheReporter", retentionExports},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRetentionModEnv(t, newHarness)) })
	}
}

// stubRetentionSettings stands in for the instance's retention settings.
type stubRetentionSettings struct {
	mu sync.Mutex
	p  appmoderation.RetentionPeriods
}

// RetentionPeriods implements appmoderation.RetentionSettings.
func (s *stubRetentionSettings) RetentionPeriods(context.Context) (appmoderation.RetentionPeriods, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p, nil
}

type retentionModEnv struct {
	*appealEnv
	settings *stubRetentionSettings
	// reporter is a remote actor that files reports.
	reporter string
}

func newRetentionModEnv(t *testing.T, newHarness ModerationFactory) *retentionModEnv {
	t.Helper()
	e := &retentionModEnv{
		appealEnv: newAppealEnv(t, newHarness),
		settings:  &stubRetentionSettings{p: appmoderation.DefaultRetentionPeriods},
	}
	e.reporter = e.ids.NewID()
	e.actors.set(appaccount.Actor{Account: account.Account{ID: e.reporter, Domain: "remote.example", URI: "https://remote.example/actor"}})
	e.svc = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Moderation, Accounts: e.actors, Clock: e.clock, IDs: e.ids,
		Statuses: e.statuses, Lifecycles: e.lifecycles, AppealWindow: e.window, Retention: e.settings,
	})
	return e
}

func (e *retentionModEnv) purge(t *testing.T) appmoderation.PurgedRecords {
	t.Helper()
	n, err := e.svc.PurgeRecords(t.Context())
	if err != nil {
		t.Fatalf("PurgeRecords = %v", err)
	}
	return n
}

func (e *retentionModEnv) issueFor(t *testing.T, in appmoderation.AccountActionInput) moderation.Action {
	t.Helper()
	if in.ReasonCategory == "" {
		in.ReasonCategory = moderation.CategorySpam
	}
	a, err := e.svc.IssueAccountAction(t.Context(), in)
	if err != nil {
		t.Fatalf("IssueAccountAction(%s) = %v", in.Kind, err)
	}
	return a
}

func (e *retentionModEnv) report(t *testing.T, n string, statusIDs ...string) moderation.Report {
	t.Helper()
	r, _, err := e.svc.FileRemoteReport(t.Context(), appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/" + n, ReporterAccountID: e.reporter, TargetAccountID: e.local,
		Comment: "please look", StatusIDs: statusIDs,
	})
	if err != nil {
		t.Fatalf("FileRemoteReport = %v", err)
	}
	return r
}

// wantActions checks which of the actions remain.
func (e *retentionModEnv) wantActions(t *testing.T, kept, purged []moderation.Action) {
	t.Helper()
	for _, a := range kept {
		if _, err := e.h.Moderation.FindActionRecord(t.Context(), a.ID); err != nil {
			t.Errorf("FindActionRecord(%s %s) = %v; want it kept", a.Kind, a.ID, err)
		}
	}
	for _, a := range purged {
		if _, err := e.h.Moderation.FindActionRecord(t.Context(), a.ID); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("FindActionRecord(%s %s) = %v; want ErrNotFound", a.Kind, a.ID, err)
		}
	}
}

// wantReports checks which of the reports remain.
func (e *retentionModEnv) wantReports(t *testing.T, kept, purged []moderation.Report) {
	t.Helper()
	for _, r := range kept {
		if _, err := e.h.Moderation.FindReport(t.Context(), r.ID); err != nil {
			t.Errorf("FindReport(%s) = %v; want it kept", r.ID, err)
		}
	}
	for _, r := range purged {
		if _, err := e.h.Moderation.FindReport(t.Context(), r.ID); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("FindReport(%s) = %v; want ErrNotFound", r.ID, err)
		}
		if _, err := e.h.Moderation.FindReportStatus(t.Context(), r.ID); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("FindReportStatus(%s) = %v; want ErrNotFound", r.ID, err)
		}
		if notes, err := e.h.Moderation.ListReportNotes(t.Context(), r.ID); err != nil || len(notes) != 0 {
			t.Errorf("ListReportNotes(%s) = %v, %v; want none", r.ID, notes, err)
		}
		if h, err := e.h.Moderation.ReportHistory(t.Context(), r.ID); err != nil || len(h.Assignments)+len(h.Resolutions)+len(h.Reopenings) != 0 {
			t.Errorf("ReportHistory(%s) = %+v, %v; want none", r.ID, h, err)
		}
	}
}

func retentionHolds(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	for _, reason := range []string{"", " \n", strings.Repeat("x", moderation.MaxRetentionHoldReason+1), "a\x00b"} {
		if _, err := e.svc.PlaceRetentionHold(ctx, e.local, reason, ""); !errors.Is(err, moderation.ErrInvalidRetentionHold) {
			t.Errorf("PlaceRetentionHold(%.10q) = %v, want ErrInvalidRetentionHold", reason, err)
		}
	}
	if _, err := e.svc.PlaceRetentionHold(ctx, e.ids.NewID(), "court order", ""); !errors.Is(err, appmoderation.ErrAccountNotFound) {
		t.Errorf("PlaceRetentionHold(unknown account) = %v, want ErrAccountNotFound", err)
	}
	first, err := e.svc.PlaceRetentionHold(ctx, e.local, "court order", e.moderator)
	if err != nil || first.AccountID != e.local || first.CreatedByUserID != e.moderator {
		t.Fatalf("PlaceRetentionHold = %+v, %v", first, err)
	}
	e.clock.Advance(time.Second)
	second, err := e.svc.PlaceRetentionHold(ctx, e.local, "disclosure request", "")
	if err != nil {
		t.Fatal(err)
	}
	held := func(want ...string) {
		t.Helper()
		got, err := e.svc.HeldAccountIDs(ctx)
		if err != nil || !reflect.DeepEqual(append([]string{}, got...), append([]string{}, want...)) {
			t.Errorf("HeldAccountIDs = %v, %v; want %v", got, err, want)
		}
	}
	held(e.local)
	holds, err := e.svc.RetentionHolds(ctx, e.local)
	if err != nil || len(holds) != 2 || holds[0].Hold != second || holds[1].Hold != first || !holds[0].Active() {
		t.Errorf("RetentionHolds = %+v, %v; want the two holds newest first", holds, err)
	}
	if all, err := e.svc.RetentionHolds(ctx, ""); err != nil || len(all) != 2 {
		t.Errorf("RetentionHolds(every account) = %+v, %v; want 2", all, err)
	}
	if other, err := e.svc.RetentionHolds(ctx, e.other); err != nil || len(other) != 0 {
		t.Errorf("RetentionHolds(other) = %+v, %v; want none", other, err)
	}

	// Each hold is released once; the account stays held until its last hold is released.
	rec, released, err := e.svc.ReleaseRetentionHold(ctx, first.ID, e.moderator)
	if err != nil || !released || rec.Active() || rec.Release.ReleasedByUserID != e.moderator {
		t.Errorf("ReleaseRetentionHold = %+v, %t, %v; want released by the moderator", rec, released, err)
	}
	if rec2, released, err := e.svc.ReleaseRetentionHold(ctx, first.ID, ""); err != nil || released || !reflect.DeepEqual(rec2, rec) {
		t.Errorf("ReleaseRetentionHold again = %+v, %t, %v; want it unchanged", rec2, released, err)
	}
	held(e.local)
	if _, _, err := e.svc.ReleaseRetentionHold(ctx, second.ID, ""); err != nil {
		t.Fatal(err)
	}
	held()
	if _, _, err := e.svc.ReleaseRetentionHold(ctx, e.ids.NewID(), ""); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReleaseRetentionHold(unknown) = %v, want ErrNotFound", err)
	}
}

func retentionPurgesActions(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	period := e.settings.p.Actions
	warn := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionWarn})
	expiring := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionLimit, Duration: 24 * time.Hour})
	lifted := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionForceSensitive})
	if err := e.svc.LiftAccountAction(ctx, lifted.ID, e.moderator, "sorry"); err != nil {
		t.Fatal(err)
	}
	indefinite := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionRestrictPosting})
	pending := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionWarn})
	e.appeal(t, e.local, pending)
	decided := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionWarn})
	decidedAppeal := e.appeal(t, e.local, decided).Appeal
	e.clock.Advance(10 * 24 * time.Hour)
	if _, err := e.svc.DecideAppeal(ctx, decidedAppeal.ID, moderation.AppealDecisionUpheld, "", e.moderator); err != nil {
		t.Fatal(err)
	}

	// Within the period of the earliest end, nothing goes.
	e.clock.Advance(period - 10*24*time.Hour)
	if n := e.purge(t); n.Actions != 0 {
		t.Fatalf("purge within the period = %+v, want no action", n)
	}
	// Past it, the actions that ended then go; the one decided later, the indefinite one, and the
	// one with a pending appeal stay.
	e.clock.Advance(25 * time.Hour)
	if n := e.purge(t); n.Actions != 3 {
		t.Errorf("purge past the period = %+v, want 3 actions", n)
	}
	e.wantActions(t, []moderation.Action{indefinite, pending, decided}, []moderation.Action{warn, expiring, lifted})
	// The decided appeal's action goes with its appeal once the decision is past the period.
	e.clock.Advance(10 * 24 * time.Hour)
	if n := e.purge(t); n.Actions != 1 {
		t.Errorf("purge past the decision = %+v, want 1 action", n)
	}
	e.wantActions(t, []moderation.Action{indefinite, pending}, []moderation.Action{decided})
	if _, err := e.h.Moderation.FindAppeal(ctx, decidedAppeal.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindAppeal(purged) = %v, want ErrNotFound", err)
	}
	if n, err := e.h.Moderation.CountAppeals(ctx, moderation.AppealUpheld); err != nil || n != 0 {
		t.Errorf("CountAppeals(upheld) = %d, %v; want 0", n, err)
	}
	if n := e.purge(t); n != (appmoderation.PurgedRecords{}) {
		t.Errorf("repeated purge = %+v, want nothing", n)
	}
	// A shorter period applies at the next run; the indefinite action still stays.
	if err := e.svc.LiftAccountAction(ctx, indefinite.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	e.settings.mu.Lock()
	e.settings.p.Actions = 180 * 24 * time.Hour
	e.settings.mu.Unlock()
	e.clock.Advance(180*24*time.Hour + time.Hour)
	if n := e.purge(t); n.Actions != 1 {
		t.Errorf("purge under a shorter period = %+v, want 1 action", n)
	}
	e.wantActions(t, []moderation.Action{pending}, []moderation.Action{indefinite})
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rows, err := e.h.Moderation.AccountRestrictionRows(ctx, tx, e.local)
		if err == nil && len(rows) != 0 {
			t.Errorf("AccountRestrictionRows after the purge = %+v; want none", rows)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func retentionPurgesReports(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	period := e.settings.p.Reports
	open := e.report(t, "open")
	dismissed := e.report(t, "dismissed", NamedID("s1"))
	if _, err := e.svc.AssignReport(ctx, dismissed.ID, e.moderator, e.moderator); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddReportNote(ctx, dismissed.ID, "nothing to see", e.moderator); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ResolveReport(ctx, dismissed.ID, moderation.OutcomeDismissed, e.moderator); err != nil {
		t.Fatal(err)
	}
	// A report that an action follows from stays as long as the action.
	actioned := e.report(t, "actioned")
	action := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionRestrictPosting, ReportID: actioned.ID})

	e.clock.Advance(period + time.Hour)
	if n := e.purge(t); n.Reports != 1 {
		t.Errorf("purge past the period = %+v, want 1 report", n)
	}
	e.wantReports(t, []moderation.Report{open, actioned}, []moderation.Report{dismissed})
	// A report reopened and resolved again counts from its last resolution.
	if _, err := e.svc.ResolveReport(ctx, open.ID, moderation.OutcomeDismissed, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.LiftAccountAction(ctx, action.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(period - time.Hour)
	if n := e.purge(t); n.Reports != 0 || n.Actions != 0 {
		t.Errorf("purge within the period of the resolution = %+v, want nothing", n)
	}
	e.clock.Advance(2 * time.Hour)
	if n := e.purge(t); n.Reports != 2 || n.Actions != 1 {
		t.Errorf("purge past the period of the resolution = %+v, want 2 reports and the action", n)
	}
	e.wantReports(t, nil, []moderation.Report{open, actioned})
}

func retentionHoldKeepsRecords(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	heldAction := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionWarn})
	otherAction := e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.other, Kind: moderation.ActionWarn})
	heldReport := e.report(t, "held")
	if _, err := e.svc.ResolveReport(ctx, heldReport.ID, moderation.OutcomeDismissed, e.moderator); err != nil {
		t.Fatal(err)
	}
	hold, err := e.svc.PlaceRetentionHold(ctx, e.local, "court order", "")
	if err != nil {
		t.Fatal(err)
	}
	audited := func(accountID string) int {
		t.Helper()
		entries, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetAccount, TargetID: accountID}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	reportAudited := func() int {
		t.Helper()
		entries, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetReport, TargetID: heldReport.ID}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	if audited(e.local) != 1 || audited(e.other) != 1 || reportAudited() != 1 {
		t.Fatalf("audit log before the purge = %d, %d, %d; want one entry each", audited(e.local), audited(e.other), reportAudited())
	}

	// Past every period, only the held account's records stay.
	e.clock.Advance(e.settings.p.AuditLog + time.Hour)
	n := e.purge(t)
	if n.Actions != 1 || n.Reports != 0 || n.AuditEntries != 1 {
		t.Errorf("purge with a hold = %+v, want the other account's action and entry", n)
	}
	e.wantActions(t, []moderation.Action{heldAction}, []moderation.Action{otherAction})
	e.wantReports(t, []moderation.Report{heldReport}, nil)
	if audited(e.local) != 1 || audited(e.other) != 0 || reportAudited() != 1 {
		t.Errorf("audit log after the purge = %d, %d, %d; want the held account's entries only", audited(e.local), audited(e.other), reportAudited())
	}

	// Released, they go at the next run.
	if _, _, err := e.svc.ReleaseRetentionHold(ctx, hold.ID, ""); err != nil {
		t.Fatal(err)
	}
	n = e.purge(t)
	if n.Actions != 1 || n.Reports != 1 || n.AuditEntries != 2 {
		t.Errorf("purge after the release = %+v, want the held records", n)
	}
	e.wantActions(t, nil, []moderation.Action{heldAction})
	e.wantReports(t, nil, []moderation.Report{heldReport})
	if audited(e.local) != 0 || reportAudited() != 0 {
		t.Errorf("audit log after the release = %d, %d; want none", audited(e.local), reportAudited())
	}
}

func retentionKeepsEvidence(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	reported, acted, plain := e.ids.NewID(), e.ids.NewID(), e.ids.NewID()
	e.report(t, "evidence", reported)
	e.statuses.add(content.Status{ID: acted, URI: "https://social.example/statuses/" + acted, AuthorAccountID: e.local, IsLocal: true})
	e.issueFor(t, appmoderation.AccountActionInput{TargetAccountID: e.local, Kind: moderation.ActionMarkStatusesSensitive, StatusIDs: []string{acted}})
	retained := func(statusID, author string) bool {
		t.Helper()
		var got bool
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			got, err = e.svc.StatusRetained(ctx, tx, statusID, author)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !retained(reported, e.other) || !retained(acted, e.other) {
		t.Errorf("StatusRetained(reported, acted) = %t, %t; want both", retained(reported, e.other), retained(acted, e.other))
	}
	if retained(plain, e.other) {
		t.Error("StatusRetained(unreferenced) = true, want false")
	}
	hold, err := e.svc.PlaceRetentionHold(ctx, e.other, "court order", "")
	if err != nil {
		t.Fatal(err)
	}
	if !retained(plain, e.other) {
		t.Error("StatusRetained(held author) = false, want true")
	}
	if _, _, err := e.svc.ReleaseRetentionHold(ctx, hold.ID, ""); err != nil {
		t.Fatal(err)
	}
	if retained(plain, e.other) {
		t.Error("StatusRetained(released author) = true, want false")
	}
}

func retentionExports(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	r := e.report(t, "export", NamedID("s1"))
	if _, err := e.svc.AssignReport(ctx, r.ID, e.moderator, e.moderator); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddReportNote(ctx, r.ID, "the moderators' note on the report", e.moderator); err != nil {
		t.Fatal(err)
	}
	a := e.issueFor(t, appmoderation.AccountActionInput{
		TargetAccountID: e.local, Kind: moderation.ActionWarn, ReportID: r.ID, PublicReason: "spam",
		PrivateNote: "the moderators' note on the action", IssuedByUserID: e.moderator,
	})
	appeal := e.appeal(t, e.local, a).Appeal
	if _, err := e.svc.DecideAppeal(ctx, appeal.ID, moderation.AppealDecisionUpheld, "it was spam", e.moderator); err != nil {
		t.Fatal(err)
	}
	// The account's own report of another account.
	filed, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/filed", ReporterAccountID: e.local, TargetAccountID: e.other, Comment: "mine",
	})
	if err != nil {
		t.Fatal(err)
	}

	x, err := e.svc.ExportRecords(ctx, e.local)
	if err != nil {
		t.Fatalf("ExportRecords = %v", err)
	}
	if len(x.Actions) != 1 || x.Actions[0].PrivateNote != a.PrivateNote || x.Actions[0].Appeal == nil ||
		x.Actions[0].Appeal.Decision == nil || x.Actions[0].Appeal.Decision.PublicReason != "it was spam" {
		t.Errorf("exported actions = %+v; want the action with its note and appeal", x.Actions)
	}
	if len(x.ReportsAbout) != 1 || len(x.ReportsAbout[0].Notes) != 1 || x.ReportsAbout[0].Notes[0].Body != "the moderators' note on the report" ||
		len(x.ReportsAbout[0].Assignments) != 1 || len(x.ReportsAbout[0].Resolutions) != 1 || x.ReportsAbout[0].Outcome != string(moderation.OutcomeActioned) {
		t.Errorf("exported reports about the account = %+v; want the report with its note and history", x.ReportsAbout)
	}
	if len(x.ReportsFiled) != 1 || x.ReportsFiled[0].ID != filed.ID || x.ReportsFiled[0].TargetAccountID != e.other {
		t.Errorf("exported reports filed = %+v; want the account's own report", x.ReportsFiled)
	}
	// The issue of the action, the resolution of the report, its assignment, its note, and the
	// decision on the appeal.
	if len(x.AuditLog) != 5 {
		t.Errorf("exported audit log = %+v; want 5 entries", x.AuditLog)
	}
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{e.reporter, r.URI, "remote.example"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("export contains the reporter's identity %q: %s", secret, b)
		}
	}
	if _, err := e.svc.ExportRecords(ctx, e.ids.NewID()); !errors.Is(err, appmoderation.ErrAccountNotFound) {
		t.Errorf("ExportRecords(unknown) = %v, want ErrAccountNotFound", err)
	}

	// The account's own data export has the action as the account sees it, its appeal, and its
	// own report, without the private notes or the reports about it (ADR-0038).
	var own bytes.Buffer
	if err := e.svc.WriteOwnRecords(ctx, e.local, &own); err != nil {
		t.Fatalf("WriteOwnRecords = %v", err)
	}
	var ox appmoderation.OwnRecordExport
	if err := json.Unmarshal(own.Bytes(), &ox); err != nil {
		t.Fatal(err)
	}
	if ox.AccountID != e.local || len(ox.Actions) != 1 || ox.Actions[0].ID != a.ID || ox.Actions[0].PublicReason != "spam" ||
		ox.Actions[0].Appeal == nil || ox.Actions[0].Appeal.DecisionReason != "it was spam" || ox.Actions[0].Appeal.DecidedAt == nil {
		t.Errorf("own actions = %+v; want the action with its decided appeal", ox.Actions)
	}
	if len(ox.ReportsFiled) != 1 || ox.ReportsFiled[0].ID != filed.ID {
		t.Errorf("own reports filed = %+v; want the account's own report", ox.ReportsFiled)
	}
	for _, secret := range []string{a.PrivateNote, "the moderators' note on the report", e.reporter, e.moderator, r.ID} {
		if strings.Contains(own.String(), secret) {
			t.Errorf("own export contains %q: %s", secret, own.String())
		}
	}
}

// retentionPurgesReleasedHolds checks that a released hold goes with the actions' records, and that
// a hold in effect never does.
func retentionPurgesReleasedHolds(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	released, err := e.svc.PlaceRetentionHold(ctx, e.local, "court order", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PlaceRetentionHold(ctx, e.other, "disclosure request", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.ReleaseRetentionHold(ctx, released.ID, ""); err != nil {
		t.Fatal(err)
	}
	wantHolds := func(accountID string, want int, when string) {
		t.Helper()
		got, err := e.svc.RetentionHolds(ctx, accountID)
		if err != nil || len(got) != want {
			t.Errorf("%s: holds of %s = %+v, %v; want %d", when, accountID, got, err, want)
		}
	}
	if _, err := e.svc.PurgeRecords(ctx); err != nil {
		t.Fatal(err)
	}
	wantHolds(e.local, 1, "right after the release")

	e.clock.Advance(appmoderation.DefaultRetentionPeriods.Actions + time.Hour)
	out, err := e.svc.PurgeRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out.RetentionHolds != 1 {
		t.Errorf("PurgeRecords = %+v, want one retention hold", out)
	}
	wantHolds(e.local, 0, "past the retention")
	wantHolds(e.other, 1, "a hold in effect")
}

// retentionErasesSeverances checks that the erasure of an account deletes its severances only.
func retentionErasesSeverances(t *testing.T, e *retentionModEnv) {
	ctx := t.Context()
	severance := func(accountID string) string {
		t.Helper()
		sv := moderation.Severance{ID: e.ids.NewID(), ActionID: e.ids.NewID(), AccountID: accountID, FollowersLost: 1, OccurredAt: e.clock.Now()}
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := e.h.Moderation.InsertSeverance(ctx, tx, sv)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return sv.ID
	}
	erased, kept := severance(e.local), severance(e.other)
	for range 2 { // safe to repeat
		if _, err := e.svc.EraseAccountData(ctx, appaccount.ErasureTarget{AccountID: e.local}, "", 0); err != nil {
			t.Fatalf("EraseAccountData = %v", err)
		}
	}
	if _, err := e.svc.SeveranceOf(ctx, e.local, erased); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("SeveranceOf(erased) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.SeveranceOf(ctx, e.other, kept); err != nil {
		t.Errorf("SeveranceOf(another account's) = %v", err)
	}
}
