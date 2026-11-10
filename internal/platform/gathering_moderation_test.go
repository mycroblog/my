// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/moderation"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
)

// TestModeratorCancelsAReportedEvent checks ADR-0161 §14 through the app: a report of an event
// keeps the revision that the reporter saw after the organizer revises it, and a moderator's
// cancellation is in the audit log and notifies the attendees.
func TestModeratorCancelsAReportedEvent(t *testing.T) {
	t.Parallel()
	e := newEventEnv(t, "bob", "carol", "mod")
	ctx := t.Context()
	e.answer(t, "bob", gathering.AnswerGoing)
	target, err := e.app.Events.ReportTarget(ctx, e.ids["carol"], e.event.ID)
	if err != nil {
		t.Fatalf("ReportTarget = %v", err)
	}
	report, err := e.app.LocalReports.File(ctx, appmoderation.LocalReportInput{
		ReporterAccountID: e.ids["carol"], TargetAccountID: target.OrganizerAccountID, Category: moderation.CategorySpam,
		Gathering: &moderation.ReportGathering{GatheringID: target.GatheringID, RevisionID: target.RevisionID},
	})
	if err != nil {
		t.Fatalf("File = %v", err)
	}
	title := "Board game night (new title)"
	if _, err := e.app.Events.Update(ctx, e.ids["alice"], e.event.ID, appgathering.Patch{Title: &title}); err != nil {
		t.Fatalf("Update = %v", err)
	}
	rec, err := e.app.Moderation.ReportRecord(ctx, report.ID)
	if err != nil || rec.Report.Gathering == nil || rec.Report.Gathering.RevisionID != e.event.Revision.ID || rec.Report.TargetAccountID != e.ids["alice"] {
		t.Fatalf("ReportRecord = %+v, %v; want the event's first revision and its organizer", rec.Report, err)
	}
	records, err := e.app.Events.EventRecords(ctx, e.event.ID)
	if err != nil || len(records.Revisions) != 2 || records.Revisions[0].ID != rec.Report.Gathering.RevisionID ||
		records.Revisions[0].Settings.Title != "Board game night" {
		t.Fatalf("EventRecords = %+v, %v; want the revision that carol saw first", records.Revisions, err)
	}
	if reported, err := e.app.Moderation.ReportedGatherings(ctx, []string{e.event.ID}); err != nil || !reported[e.event.ID] {
		t.Errorf("ReportedGatherings = %v, %v; want the event", reported, err)
	}
	e.drain(t)

	mod, err := e.app.Accounts.FindByID(ctx, e.ids["mod"])
	if err != nil {
		t.Fatal(err)
	}
	err = e.app.Events.CancelByModerator(ctx, appgathering.ModeratorCancelInput{
		GatheringID: e.event.ID, ByUserID: mod.Local.UserID, ByAccountID: e.ids["mod"], Note: "Spam",
	})
	if err != nil {
		t.Fatalf("CancelByModerator = %v", err)
	}
	e.drain(t)
	entries, err := e.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditEventCancelled}, 10)
	if err != nil || len(entries) != 1 || entries[0].TargetKind != moderation.AuditTargetEvent || entries[0].TargetID != e.event.ID ||
		entries[0].ActorUserID != mod.Local.UserID {
		t.Errorf("audit log = %+v, %v; want the moderator's cancellation of the event", entries, err)
	}
	cancelled := eventNotice{domainnotification.TypeEventCancelled, domainnotification.EventCancelledKey(e.event.ID)}
	wantNotices(t, "bob", e.notices(t, "bob"), cancelled)
	wantNotices(t, "mod", e.notices(t, "mod"))
}

// TestSuspendingAnOrganizerCancelsTheirEvents checks ADR-0161 §14 through the app: suspending the
// organizer cancels its events that have not started, as organizer_suspended, and notifies their
// attendees; an event that started stays.
func TestSuspendingAnOrganizerCancelsTheirEvents(t *testing.T) {
	t.Parallel()
	e := newEventEnv(t, "bob")
	ctx := t.Context()
	e.answer(t, "bob", gathering.AnswerGoing)
	soon, err := e.app.Events.Create(ctx, e.ids["alice"], appgathering.CreateInput{
		Settings: gathering.Settings{Title: "Coffee", TimeZone: "Asia/Tokyo"},
		Schedule: gathering.Schedule{StartsAt: e.start.Add(time.Hour), EndsAt: e.start.Add(6 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	e.at(t, 90*time.Minute)
	if _, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.ids["alice"], Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	}); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	e.drain(t)
	rec, err := e.app.Events.EventRecords(ctx, e.event.ID)
	if err != nil || rec.Cancellation == nil || rec.Cancellation.Reason != gathering.CancelOrganizerSuspended || rec.Cancellation.CancelledByAccountID != "" {
		t.Errorf("the upcoming event = %+v, %v; want it cancelled as organizer_suspended", rec.Cancellation, err)
	}
	if rec, err := e.app.Events.EventRecords(ctx, soon.ID); err != nil || rec.Cancellation != nil {
		t.Errorf("the started event = %+v, %v; want it kept", rec.Cancellation, err)
	}
	wantNotices(t, "bob", e.notices(t, "bob"), eventNotice{domainnotification.TypeEventCancelled, domainnotification.EventCancelledKey(e.event.ID)})
}
