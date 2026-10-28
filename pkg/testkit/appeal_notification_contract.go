// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"strings"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// notificationAppeals checks the notifications of the appeals (FR-707, FR-712): an admin_appeal
// of a new appeal to the holders of manage_accounts other than the account that appealed, emailed
// to those with an address, and an appeal_decided of the decision to that account, each once.
func notificationAppeals(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	mod, reports, alice := e.user(t, "mod"), e.user(t, "reports"), e.user(t, "alice")
	e.staff.set(mod, notification.PermissionManageAccounts)
	e.staff.set(reports, notification.PermissionManageReports)
	e.staff.set(alice, notification.PermissionManageAccounts)
	e.staff.email(mod, "mod@example.com")

	e.clock.Advance(time.Second)
	appeal := moderation.Appeal{ID: e.ids.NewID(), ActionID: e.ids.NewID(), AccountID: alice, Text: "I did not.", SubmittedAt: e.clock.Now()}
	filed := appmoderation.AppealFiled{Appeal: appeal}
	e.project(t, filed, filed)
	check := func(accountID string, want ...notification.Type) {
		t.Helper()
		ns := e.list(t, accountID)
		got := make([]string, len(ns))
		for i, n := range ns {
			got[i] = string(n.Type)
			if n.AppealID != appeal.ID || n.SourceAccountID != "" {
				t.Errorf("notification %+v, want one of appeal %s from the instance", n, appeal.ID)
			}
		}
		w := make([]string, len(want))
		for i, typ := range want {
			w[i] = string(typ)
		}
		if strings.Join(got, ",") != strings.Join(w, ",") {
			t.Errorf("notifications of %s = %v, want %v", accountID, got, w)
		}
	}
	check(mod, notification.TypeAdminAppeal)
	check(reports) // manage_reports is not enough
	check(alice)   // nobody hears of their own appeal
	jobs := e.outbox.take()
	if len(jobs) != 1 || jobs[0].Type != appnotification.JobModeratorEmail {
		t.Fatalf("jobs = %+v, want the email of mod's notification", jobs)
	}

	// The email names neither the user nor the action.
	mailer := &Mailer{}
	reg := &queue.JobHandlerRegistry{}
	appnotification.RegisterModeratorJobs(reg, appnotification.MailDeps{
		Staff: e.staff, Settings: e.svc, Mailboxes: e.staff, Mailer: mailer, Host: contractHost, WebOrigin: "https://web.example",
	})
	handle, _, _ := reg.Lookup(appnotification.JobModeratorEmail)
	if res := handle(ctx, queue.Job{ID: e.ids.NewID(), Type: jobs[0].Type, Payload: jobs[0].Payload}); !res.IsDone() {
		t.Fatalf("moderator email = %+v, want done", res)
	}
	sent := mailer.Sent()
	if len(sent) != 1 || sent[0].To != "mod@example.com" || !strings.Contains(sent[0].Text, "https://web.example/notifications") ||
		strings.Contains(sent[0].Text, appeal.ID) || strings.Contains(sent[0].Text, appeal.ActionID) {
		t.Errorf("sent = %+v, want one email to mod linking to the notifications", sent)
	}

	// The decision goes to the account that appealed, once.
	e.clock.Advance(time.Second)
	decided := appmoderation.AppealDecided{Appeal: appeal, Decision: moderation.AppealDecision{
		AppealID: appeal.ID, Decision: moderation.AppealDecisionOverturned, DecidedByUserID: mod, DecidedAt: e.clock.Now(),
	}}
	e.project(t, decided, decided)
	check(alice, notification.TypeAppealDecided)
	check(mod, notification.TypeAdminAppeal)

	// Without manage_accounts, the admin_appeal leaves the list.
	e.staff.set(mod)
	check(mod)
}
