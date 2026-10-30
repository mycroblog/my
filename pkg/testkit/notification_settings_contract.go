// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"strings"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The notification settings cases of NotificationContract (FR-210, ADR-0109).

// setting turns channel ch of group g on or off for accountID.
func (e *notificationEnv) setting(t *testing.T, accountID string, g notification.Group, ch notification.Channel, on bool) {
	t.Helper()
	if _, err := e.svc.UpdateSettings(t.Context(), accountID, []notification.Setting{{Group: g, Channel: ch, Enabled: on}}); err != nil {
		t.Fatalf("UpdateSettings(%s, %s %s = %v) = %v", accountID, g, ch, on, err)
	}
}

// settingsOf returns accountID's settings by group.
func (e *notificationEnv) settingsOf(t *testing.T, accountID string) map[notification.Group]appnotification.GroupSettings {
	t.Helper()
	rows, err := e.svc.Settings(t.Context(), accountID)
	if err != nil {
		t.Fatalf("Settings(%s) = %v", accountID, err)
	}
	out := map[notification.Group]appnotification.GroupSettings{}
	for _, r := range rows {
		out[r.Spec.Group] = r
	}
	return out
}

// notificationSettings checks the settings: the rows each user gets with their defaults and
// fixed channels, the changes that fail and change nothing, a type turned off in the app that
// creates no notification and no email, and the one-click unsubscribe that turns off one row's
// email.
func notificationSettings(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, mod, helper := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "mod"), e.user(t, "helper")
	e.staff.set(mod, notification.PermissionManageReports)
	e.staff.set(helper, notification.PermissionManageReports)

	// The defaults: ordinary notifications in the app and not by email, the moderator ones both,
	// the instance's notices always in the app, and only the moderator rows of the permissions held.
	type cells struct{ inApp, inAppFixed, email, emailFixed bool }
	wantRow := func(accountID string, g notification.Group, want cells) {
		t.Helper()
		r, ok := e.settingsOf(t, accountID)[g]
		got := cells{r.InApp.Enabled, r.InApp.Fixed, r.Email.Enabled, r.Email.Fixed}
		if !ok || got != want {
			t.Errorf("row %s of %s = %+v (listed %v), want %+v", g, accountID, got, ok, want)
		}
	}
	rows, err := e.svc.Settings(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	var groups []string
	for _, r := range rows {
		groups = append(groups, string(r.Spec.Group))
	}
	if got, want := strings.Join(groups, ","),
		"mention,follow,follow_request,follow_accepted,boost,favourite,reaction,quote,poll_closed,account_moved,voice,status,moderation,service_closure"; got != want {
		t.Errorf("rows of a user = %s, want %s (no moderator rows, no supporter row while the program is off)", got, want)
	}
	wantRow(alice, notification.GroupMention, cells{inApp: true})
	wantRow(alice, notification.GroupVoice, cells{inApp: true})
	wantRow(alice, notification.GroupModeration, cells{inApp: true, inAppFixed: true})
	wantRow(alice, notification.GroupServiceClosure, cells{inApp: true, inAppFixed: true, email: true, emailFixed: true})
	wantRow(mod, notification.GroupAdminReport, cells{inApp: true, email: true})
	if _, ok := e.settingsOf(t, mod)[notification.GroupAdminSignUp]; ok {
		t.Error("admin_sign_up is offered without manage_accounts")
	}

	// A fixed channel, a row the user does not get, and an unknown group or channel fail, and a
	// change among them changes nothing.
	for _, bad := range []struct {
		s    notification.Setting
		want error
	}{
		{notification.Setting{Group: notification.GroupModeration, Channel: notification.ChannelInApp}, notification.ErrFixedSetting},
		{notification.Setting{Group: notification.GroupServiceClosure, Channel: notification.ChannelEmail}, notification.ErrFixedSetting},
		{notification.Setting{Group: notification.GroupAdminReport, Channel: notification.ChannelEmail}, notification.ErrInvalidSetting},
		{notification.Setting{Group: "nothing", Channel: notification.ChannelEmail}, notification.ErrInvalidSetting},
		{notification.Setting{Group: notification.GroupMention, Channel: "push"}, notification.ErrInvalidSetting},
	} {
		change := []notification.Setting{{Group: notification.GroupMention, Channel: notification.ChannelEmail, Enabled: true}, bad.s}
		if _, err := e.svc.UpdateSettings(ctx, alice, change); !errors.Is(err, bad.want) {
			t.Errorf("UpdateSettings(%+v) = %v, want %v", bad.s, err, bad.want)
		}
	}
	wantRow(alice, notification.GroupMention, cells{inApp: true})

	// A type turned off in the app creates no notification, so no email either; turned on
	// again, the next one comes with its email.
	e.setting(t, alice, notification.GroupFollow, notification.ChannelEmail, true)
	e.setting(t, alice, notification.GroupFollow, notification.ChannelInApp, false)
	wantRow(alice, notification.GroupFollow, cells{inApp: false, email: true})
	e.follow(t, bob, alice)
	checkSummaries(t, e, alice)
	if jobs := e.outbox.take(); len(jobs) != 0 {
		t.Errorf("jobs of a type off in the app = %+v, want none", jobs)
	}
	e.setting(t, alice, notification.GroupFollow, notification.ChannelInApp, true)
	e.follow(t, carol, alice)
	checkSummaries(t, e, alice, summary{Type: notification.TypeFollow, Source: carol})
	if jobs := e.outbox.take(); len(jobs) != 1 || jobs[0].Type != appnotification.JobNotificationEmail {
		t.Errorf("jobs = %+v, want the email of the follow", jobs)
	}

	// The unsubscribe link turns off the email of its row only.
	e.setting(t, alice, notification.GroupMention, notification.ChannelEmail, true)
	if err := e.svc.TurnOffEmail(ctx, alice, notification.GroupFollow); err != nil {
		t.Fatal(err)
	}
	wantRow(alice, notification.GroupFollow, cells{inApp: true})
	wantRow(alice, notification.GroupMention, cells{inApp: true, email: true})
	if err := e.svc.TurnOffEmail(ctx, alice, notification.GroupServiceClosure); !errors.Is(err, notification.ErrFixedSetting) {
		t.Errorf("TurnOffEmail(service_closure) = %v, want ErrFixedSetting", err)
	}

	// A moderator who turned a type off in the app gets none of it; the others still do.
	e.setting(t, mod, notification.GroupAdminReport, notification.ChannelInApp, false)
	e.clock.Advance(time.Second)
	e.project(t, appmoderation.ReportFiled{FirstOpen: true, Report: moderation.Report{
		ID: e.ids.NewID(), ReporterAccountID: alice, TargetAccountID: bob, Category: moderation.CategorySpam, FiledAt: e.clock.Now(),
	}})
	if ns := e.list(t, mod); len(ns) != 0 {
		t.Errorf("notifications of a moderator who turned admin_report off = %+v, want none", ns)
	}
	if ns := e.list(t, helper); len(ns) != 1 || ns[0].Type != notification.TypeAdminReport {
		t.Errorf("notifications of helper = %+v, want the admin_report", ns)
	}
	if jobs := e.outbox.take(); len(jobs) != 1 || jobs[0].Type != appnotification.JobModeratorEmail {
		t.Errorf("jobs = %+v, want the email of helper's admin_report only", jobs)
	}
}

// notificationEmails checks the email of an ordinary notification: enqueued with the
// notification to run 15 minutes later, and sent only when the notification is still unread, not
// dismissed, still shown, its row's email still on, and the recipient has a usable address.
func notificationEmails(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.setting(t, alice, notification.GroupMention, notification.ChannelEmail, true)
	e.staff.email(alice, "alice@example.com")

	mailer := &Mailer{}
	reg := &queue.JobHandlerRegistry{}
	appnotification.RegisterEmailJobs(reg, appnotification.EmailDeps{
		Notifications: e.svc, Audience: &stubPushAudience{ended: map[string]bool{}}, Mailboxes: e.staff, Mailer: mailer,
		Host: contractHost, WebOrigin: "https://web.example",
	})
	handle, _, ok := reg.Lookup(appnotification.JobNotificationEmail)
	if !ok {
		t.Fatal("no handler of the notification email")
	}
	// mention posts a mention of alice and returns its notification and email job.
	mention := func() (notification.Notification, queue.NewJob) {
		t.Helper()
		e.post(t, bob, "hi @alice")
		jobs := e.outbox.take()
		if len(jobs) != 1 || jobs[0].Type != appnotification.JobNotificationEmail ||
			!jobs[0].RunAt.Equal(e.clock.Now().Add(notification.EmailDelay)) {
			t.Fatalf("jobs = %+v, want the email of the mention in %v", jobs, notification.EmailDelay)
		}
		return e.list(t, alice, notification.TypeMention)[0], jobs[0]
	}
	run := func(j queue.NewJob, wantSent int, why string) {
		t.Helper()
		e.clock.Advance(notification.EmailDelay)
		if res := handle(ctx, queue.Job{ID: e.ids.NewID(), Type: j.Type, Payload: j.Payload}); !res.IsDone() {
			t.Fatalf("notification email = %+v, want done", res)
		}
		if got := len(mailer.Sent()); got != wantSent {
			t.Errorf("sent %d emails %s, want %d", got, why, wantSent)
		}
	}

	// Unread, it is sent: the subject says what happened, and the email links to the post and
	// carries the unsubscribe link of its row.
	n, j := mention()
	run(j, 1, "of an unread mention")
	if sent := mailer.Sent(); len(sent) == 1 {
		m := sent[0]
		if m.To != "alice@example.com" || !strings.Contains(m.Subject, "Carol") ||
			!strings.Contains(m.Text, "https://web.example/@carol@remote.example/status/"+n.StatusID) ||
			!strings.Contains(m.Text, "my_eut_"+alice+"_mention") {
			t.Errorf("sent = %+v, want the mention to alice with its link and unsubscribe link", m)
		}
	}

	// Read, dismissed, its row's email turned off, or without a usable address, it is not.
	n, j = mention()
	if _, err := e.svc.MarkRead(ctx, alice, n.ID); err != nil {
		t.Fatal(err)
	}
	run(j, 1, "of a read mention")
	n, j = mention()
	if err := e.svc.Dismiss(ctx, alice, n.ID); err != nil {
		t.Fatal(err)
	}
	run(j, 1, "of a dismissed mention")
	_, j = mention()
	e.setting(t, alice, notification.GroupMention, notification.ChannelEmail, false)
	run(j, 1, "after the email was turned off")
	e.setting(t, alice, notification.GroupMention, notification.ChannelEmail, true)
	_, j = mention()
	e.staff.email(alice, "")
	run(j, 1, "without a usable address")
	e.staff.email(alice, "alice@example.com")

	// No longer shown (alice blocked bob since), it is not.
	_, j = mention()
	if _, err := e.graph.Block(ctx, alice, bob); err != nil {
		t.Fatal(err)
	}
	run(j, 1, "of a hidden mention")
}
