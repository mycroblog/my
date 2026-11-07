// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// stubStaff is an in-memory notification.Staff and notification.Mailboxes: the permissions of
// local accounts and where their notifications are emailed.
type stubStaff struct {
	mu      sync.Mutex
	granted map[string][]notification.Permission
	mail    map[string]appnotification.Mailbox
}

func newStubStaff() *stubStaff {
	return &stubStaff{granted: map[string][]notification.Permission{}, mail: map[string]appnotification.Mailbox{}}
}

// set gives accountID the permissions perms, replacing the ones it had.
func (s *stubStaff) set(accountID string, perms ...notification.Permission) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.granted[accountID] = perms
}

// email gives accountID the usable address address for its notification emails, or none when
// address is "".
func (s *stubStaff) email(accountID, address string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if address == "" {
		delete(s.mail, accountID)
		return
	}
	s.mail[accountID] = appnotification.Mailbox{Address: address, Locale: "en"}
}

func (s *stubStaff) PermissionHolders(_ context.Context, _ uow.Tx, p notification.Permission) ([]appnotification.StaffMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []appnotification.StaffMember
	for id, perms := range s.granted {
		if slices.Contains(perms, p) {
			out = append(out, appnotification.StaffMember{AccountID: id})
		}
	}
	slices.SortFunc(out, func(a, b appnotification.StaffMember) int { return strings.Compare(a.AccountID, b.AccountID) })
	return out, nil
}

func (s *stubStaff) GrantedPermissions(_ context.Context, accountID string) ([]notification.Permission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.granted[accountID]), nil
}

func (s *stubStaff) Mailbox(_ context.Context, accountID string) (appnotification.Mailbox, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.mail[accountID]
	return m, ok, nil
}

// UnsubscribeLink returns a link that names accountID and the group.
func (s *stubStaff) UnsubscribeLink(_ context.Context, accountID string, g notification.Group) (string, error) {
	return "https://" + contractHost + "/mail/unsubscribe?token=my_eut_" + accountID + "_" + string(g), nil
}

// recordingOutbox is a queue.Outbox that keeps the jobs it is given.
type recordingOutbox struct {
	mu   sync.Mutex
	jobs []queue.NewJob
}

func (o *recordingOutbox) EnqueueTx(_ context.Context, _ uow.Tx, jobs ...queue.NewJob) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.jobs = append(o.jobs, jobs...)
	return nil
}

func (o *recordingOutbox) take() []queue.NewJob {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.jobs
	o.jobs = nil
	return out
}

// notificationModerators checks the moderator notifications (FR-712): an admin_report of the
// first open report of an account to the holders of manage_reports other than the reporter
// and the target, an admin_sign_up of a pending registration to the holders of
// manage_accounts, each once, an email job in the same transaction for each holder unless they
// turned the type's email off (it is on by default, per type), the email itself, none for a
// holder who turned the type off in the app, and the notifications leaving the list and the
// unread count once the permission is gone.
func notificationModerators(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	mod, helper, reporter, target := e.user(t, "mod"), e.user(t, "helper"), e.user(t, "reporter"), e.user(t, "target")
	both := []notification.Permission{notification.PermissionManageReports, notification.PermissionManageAccounts}
	e.staff.set(mod, both...)
	e.staff.set(helper, notification.PermissionManageReports)
	e.staff.set(reporter, both...)
	e.staff.set(target, both...)
	e.staff.email(mod, "mod@example.com")
	e.staff.email(target, "target@example.com")
	// The moderator emails are on by default and turned off per type: helper turns off the
	// emails of the reports, and reporter those of the sign-ups.
	e.setting(t, helper, notification.GroupAdminReport, notification.ChannelEmail, false)
	e.setting(t, reporter, notification.GroupAdminSignUp, notification.ChannelEmail, false)

	type moderatorSummary struct {
		Type          notification.Type
		ReportID      string
		SubjectUserID string
	}
	check := func(accountID string, want ...moderatorSummary) {
		t.Helper()
		got := []moderatorSummary{}
		for _, n := range e.list(t, accountID) {
			if n.SourceAccountID != "" {
				t.Errorf("notification %+v has a source account", n)
			}
			got = append(got, moderatorSummary{Type: n.Type, ReportID: n.ReportID, SubjectUserID: n.SubjectUserID})
		}
		if want == nil {
			want = []moderatorSummary{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("notifications of %s = %+v, want %+v", accountID, got, want)
		}
	}
	filed := func(first bool) appmoderation.ReportFiled {
		e.clock.Advance(time.Second)
		return appmoderation.ReportFiled{FirstOpen: first, Report: moderation.Report{
			ID: e.ids.NewID(), ReporterAccountID: reporter, TargetAccountID: target, Category: moderation.CategorySpam, FiledAt: e.clock.Now(),
		}}
	}
	emit := func(ev uow.Event) {
		t.Helper()
		if err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, ev) }); err != nil {
			t.Fatalf("emit %s = %v", ev.EventType(), err)
		}
	}

	first := filed(true)
	emit(first)
	report := moderatorSummary{Type: notification.TypeAdminReport, ReportID: first.Report.ID}
	check(mod, report)
	check(helper, report)
	check(reporter) // nobody hears of their own report
	check(target)   // nor of a report of their own account
	jobs := e.outbox.take()
	if len(jobs) != 1 || jobs[0].Type != appnotification.JobModeratorEmail || jobs[0].IdempotencyKey == "" {
		t.Fatalf("jobs = %+v, want the email of mod's notification", jobs)
	}
	emailJob := jobs[0]

	// Projecting the event again, or a report while another of the account is open, notifies
	// nobody and emails nobody.
	e.project(t, first)
	emit(filed(false))
	check(mod, report)
	if jobs := e.outbox.take(); len(jobs) != 0 {
		t.Errorf("jobs = %+v, want none", jobs)
	}

	// A registration that awaits approval goes to the holders of manage_accounts.
	r := e.registration(identity.RegistrationApproval, true)
	e.clock.Advance(time.Second)
	dave := r.signUp(t, signUpInput("dave"))
	signUp := moderatorSummary{Type: notification.TypeAdminSignUp, SubjectUserID: dave.User.ID}
	check(mod, signUp, report)
	check(helper, report)
	e.project(t, appidentity.RegistrationPending{UserID: dave.User.ID, AccountID: dave.AccountID, SubmittedAt: dave.User.CreatedAt})
	check(mod, signUp, report)
	check(target, signUp) // only the reports of their own account are kept from them
	if jobs := e.outbox.take(); len(jobs) != 2 {
		t.Errorf("jobs after the sign-up = %+v, want the emails of mod and target, once each (reporter turned them off)", jobs)
	}
	e.checkUnread(t, mod, 2)

	// The job emails the notification, without naming the report.
	mailer := &Mailer{}
	reg := &queue.JobHandlerRegistry{}
	appnotification.RegisterModeratorJobs(reg, appnotification.MailDeps{
		Staff: e.staff, Settings: e.svc, Mailboxes: e.staff, Mailer: mailer, Host: contractHost, WebOrigin: "https://web.example",
	})
	handle, _, ok := reg.Lookup(appnotification.JobModeratorEmail)
	if !ok {
		t.Fatal("no handler of the moderator email")
	}
	run := func() {
		t.Helper()
		if res := handle(ctx, queue.Job{ID: e.ids.NewID(), Type: emailJob.Type, Payload: emailJob.Payload}); !res.IsDone() {
			t.Fatalf("moderator email = %+v, want done", res)
		}
	}
	run()
	sent := mailer.Sent()
	if len(sent) != 1 || sent[0].To != "mod@example.com" || !strings.Contains(sent[0].Subject, contractHost) ||
		!strings.Contains(sent[0].Text, "https://web.example/notifications") || strings.Contains(sent[0].Text, first.Report.ID) {
		t.Errorf("sent = %+v, want one email to mod linking to the notifications", sent)
	}
	// It is a notification that its recipient can stop with one click (ADR-0086 §4).
	if len(sent) == 1 {
		link := "https://" + contractHost + "/mail/unsubscribe?token=my_eut_" + mod + "_admin_report"
		if sent[0].Category != kernel.MailNotification || sent[0].Headers[kernel.HeaderListUnsubscribe] != "<"+link+">" ||
			sent[0].Headers[kernel.HeaderListUnsubscribePost] != "List-Unsubscribe=One-Click" || !strings.Contains(sent[0].Text, link) {
			t.Errorf("sent = %+v, want a notification with the unsubscribe link in its headers and text", sent[0])
		}
	}
	// A moderator without a usable address, who turned the type's email or the type off since,
	// or who lost the permission, gets none.
	e.staff.email(mod, "")
	run()
	e.staff.email(mod, "mod@example.com")
	e.setting(t, mod, notification.GroupAdminReport, notification.ChannelEmail, false)
	run()
	e.setting(t, mod, notification.GroupAdminReport, notification.ChannelEmail, true)
	e.setting(t, mod, notification.GroupAdminReport, notification.ChannelInApp, false)
	run()
	e.setting(t, mod, notification.GroupAdminReport, notification.ChannelInApp, true)
	e.staff.set(mod, notification.PermissionManageAccounts)
	run()
	if got := len(mailer.Sent()); got != 1 {
		t.Errorf("sent %d emails, want still 1", got)
	}

	// Without manage_reports, the admin_report leaves the list and the unread count; given back,
	// it is listed again.
	check(mod, signUp)
	e.checkUnread(t, mod, 1)
	e.staff.set(helper)
	check(helper)
	e.checkUnread(t, helper, 0)
	e.staff.set(helper, notification.PermissionManageReports)
	check(helper, report)
}

// notificationLimitedMoves checks admin_account_moved (ADR-0089): the move of an account that
// moderation limits goes once to the holders of manage_accounts, naming the move, and the move of
// an account without a limit to nobody.
func notificationLimitedMoves(t *testing.T, e *notificationEnv) {
	mod, helper := e.user(t, "mod"), e.user(t, "helper")
	e.staff.set(mod, notification.PermissionManageAccounts)
	e.staff.set(helper, notification.PermissionManageReports)
	moved := func(limited bool) appaccount.Moved {
		e.clock.Advance(time.Second)
		return appaccount.Moved{SourceLimited: limited, Move: appaccount.Move{
			ID: e.ids.NewID(), AccountID: e.ids.NewID(), TargetAccountID: e.ids.NewID(), InitiatedBy: "remote", OccurredAt: e.clock.Now(),
		}}
	}
	plain := moved(false)
	e.project(t, plain)
	limited := moved(true)
	e.project(t, limited)
	e.project(t, limited)
	ns := e.list(t, mod)
	if len(ns) != 1 || ns[0].Type != notification.TypeAdminAccountMoved || ns[0].AccountMoveID != limited.Move.ID || ns[0].SourceAccountID != "" {
		t.Errorf("notifications of the moderator = %+v, want one admin_account_moved of %s", ns, limited.Move.ID)
	}
	if ns := e.list(t, helper); len(ns) != 0 {
		t.Errorf("notifications without manage_accounts = %+v, want none", ns)
	}
}
