// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// moderationNotesAndReopenings checks the notes on a report, its reopening, the history of its
// target in its detail, and the resolution of a report by an action that follows from it, each
// with its audit log entry.
func moderationNotesAndReopenings(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	file := func(n string) moderation.Report {
		t.Helper()
		e.clock.Advance(time.Second)
		r, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
			URI: "https://remote.example/flags/" + n, ReporterAccountID: e.reports, TargetAccountID: e.local,
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	audited := func(want ...moderation.AuditAction) {
		t.Helper()
		got, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetReport}, 50)
		actions := []moderation.AuditAction{}
		for _, a := range got {
			actions = append(actions, a.Action)
		}
		if err != nil || !reflect.DeepEqual(actions, append([]moderation.AuditAction{}, want...)) {
			t.Errorf("audit log of reports = %v, %v; want %v (newest first)", actions, err, want)
		}
	}
	a, b := file("1"), file("2")

	// Notes list oldest first, each audited; an invalid note is refused.
	n1, err := e.svc.AddReportNote(ctx, a.ID, "first look", NamedID("mod-1"))
	if err != nil || n1.AuthorUserID != NamedID("mod-1") || n1.Body != "first look" || n1.ReportID != a.ID {
		t.Fatalf("AddReportNote = %+v, %v", n1, err)
	}
	e.clock.Advance(time.Second)
	n2, err := e.svc.AddReportNote(ctx, a.ID, "second look", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", "  \n", strings.Repeat("x", moderation.MaxReportNote+1), "a\x00b"} {
		if _, err := e.svc.AddReportNote(ctx, a.ID, body, NamedID("mod-1")); !errors.Is(err, moderation.ErrInvalidReportNote) {
			t.Errorf("AddReportNote(%.10q) = %v, want ErrInvalidReportNote", body, err)
		}
	}
	if _, err := e.svc.AddReportNote(ctx, e.ids.NewID(), "lost", NamedID("mod-1")); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("AddReportNote(unknown) = %v, want ErrNotFound", err)
	}

	// The detail shows the notes and the target's other reports and actions.
	if _, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.local, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategorySpam, IssuedByUserID: NamedID("mod-1"),
	}); err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.ReportDetail(ctx, a.ID)
	if err != nil || !reflect.DeepEqual(d.Notes, []moderation.ReportNote{n1, n2}) ||
		d.History != (moderation.TargetHistory{Reports: 1, Actions: 1}) || !reflect.DeepEqual(d.Report, a) {
		t.Errorf("ReportDetail = %+v, %v; want notes %+v, 1 other report, 1 action", d, err, []moderation.ReportNote{n1, n2})
	}
	if d, err := e.svc.ReportDetail(ctx, b.ID); err != nil || len(d.Notes) != 0 || d.History.Reports != 1 {
		t.Errorf("ReportDetail(b) = %+v, %v; want no notes", d, err)
	}
	if _, err := e.svc.ReportDetail(ctx, e.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReportDetail(unknown) = %v, want ErrNotFound", err)
	}

	// Only a resolved report reopens.
	if _, err := e.svc.ReopenReport(ctx, a.ID, NamedID("mod-1")); !errors.Is(err, appmoderation.ErrReportNotResolved) {
		t.Errorf("ReopenReport(open) = %v, want ErrReportNotResolved", err)
	}
	if _, err := e.svc.ReopenReport(ctx, e.ids.NewID(), NamedID("mod-1")); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReopenReport(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.AssignReport(ctx, a.ID, NamedID("mod-1"), NamedID("mod-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ResolveReport(ctx, a.ID, moderation.OutcomeDismissed, NamedID("mod-1")); err != nil {
		t.Fatal(err)
	}
	e.events.take()
	e.clock.Advance(time.Minute)
	reopenedAt := e.clock.Now()
	r, err := e.svc.ReopenReport(ctx, a.ID, NamedID("mod-2"))
	if err != nil || r.Status.State != moderation.ReportOpen || r.Status.Outcome != "" || r.Status.AssigneeUserID != NamedID("mod-1") ||
		!r.Status.Since.Equal(reopenedAt) {
		t.Errorf("ReopenReport = %+v, %v; want open since %v, still assigned to mod-1", r.Status, err, reopenedAt)
	}
	if n, err := e.svc.CountOpenReports(ctx); err != nil || n != 2 {
		t.Errorf("CountOpenReports = %d, %v; want 2", n, err)
	}

	// An action that follows from a report resolves it as actioned, once.
	e.clock.Advance(time.Minute)
	actedAt := e.clock.Now()
	act, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.local, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam, ReportID: a.ID, IssuedByUserID: NamedID("mod-2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.svc.ReportRecord(ctx, a.ID)
	if err != nil || r.Status.State != moderation.ReportResolved || r.Status.Outcome != moderation.OutcomeActioned || !r.Status.Since.Equal(actedAt) {
		t.Errorf("report after the action = %+v, %v; want resolved (actioned) at %v", r.Status, err, actedAt)
	}
	resolutions := 0
	for _, ev := range e.events.take() {
		if res, ok := ev.(appmoderation.ReportResolved); ok {
			resolutions++
			if res.Resolution.Outcome != moderation.OutcomeActioned || res.Resolution.ResolvedByUserID != NamedID("mod-2") || res.Report.ID != a.ID {
				t.Errorf("ReportResolved = %+v, want actioned by mod-2", res)
			}
		}
	}
	if resolutions != 1 {
		t.Errorf("the action emitted %d ReportResolved, want 1", resolutions)
	}
	e.clock.Advance(time.Minute)
	if _, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.local, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategorySpam, ReportID: a.ID, IssuedByUserID: NamedID("mod-2"),
	}); err != nil {
		t.Fatal(err)
	}
	if r, err := e.svc.ReportRecord(ctx, a.ID); err != nil || !r.Status.Since.Equal(actedAt) {
		t.Errorf("report after a second action = %+v, %v; want it unchanged", r.Status, err)
	}
	for _, ev := range e.events.take() {
		if _, ok := ev.(appmoderation.ReportResolved); ok {
			t.Errorf("a second action on a resolved report emitted %+v", ev)
		}
	}
	got, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditReportResolved, TargetKind: moderation.AuditTargetReport, TargetID: a.ID}, 10)
	if err != nil || len(got) != 2 || got[0].ActorUserID != NamedID("mod-2") || got[0].OccurredAt != actedAt {
		t.Errorf("audit log of the resolutions = %+v, %v; want the action's by mod-2 first", got, err)
	}
	if acts, err := e.svc.AccountActions(ctx, appmoderation.AccountActionFilter{AccountID: e.local}, 10); err != nil || len(acts) != 3 || acts[1].Action.ID != act.ID {
		t.Errorf("AccountActions = %+v, %v", acts, err)
	}

	audited(
		moderation.AuditReportResolved, moderation.AuditReportReopened, moderation.AuditReportResolved,
		moderation.AuditReportAssigned, moderation.AuditReportNoteAdded, moderation.AuditReportNoteAdded,
	)
}
