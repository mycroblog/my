// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestModeratorNotificationsThroughTheAPI drives the moderator notifications on the real
// database (FR-712): the first open report of an account notifies the administrators other
// than the reporter and the target, a second one notifies nobody, a sign-up that awaits
// approval notifies every administrator with the new user's account, and the counts of what
// awaits the moderators follow.
func TestModeratorNotificationsThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	ctx := t.Context()
	types := func(user string) []clientapi.Notification {
		t.Helper()
		var page clientapi.NotificationPage
		if res := c.do(user, http.MethodGet, "/notifications", nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /notifications as %s = %d", user, res.StatusCode)
		}
		return page.Items
	}
	counts := func(want clientapi.AdminCounts) {
		t.Helper()
		var got clientapi.AdminCounts
		if res := c.do("carol", http.MethodGet, "/admin/counts", nil, &got); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /admin/counts = %d", res.StatusCode)
		}
		if got.OpenReports == nil || got.PendingRegistrations == nil || *got.OpenReports != *want.OpenReports ||
			*got.PendingRegistrations != *want.PendingRegistrations {
			t.Errorf("counts = %+v, want %+v", got, want)
		}
	}
	n := func(v int64) *int64 { return &v }
	counts(clientapi.AdminCounts{OpenReports: n(0), PendingRegistrations: n(0)})

	var report clientapi.Report
	if res := c.do("alice", http.MethodPost, "/reports", map[string]any{"account_id": c.ids["bob"], "category": "spam"}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	got := types("carol")
	if len(got) != 1 || got[0].Type != clientapi.NotificationTypeAdminReport || got[0].ReportId == nil || *got[0].ReportId != report.Id || got[0].Account != nil {
		t.Fatalf("carol's notifications = %+v, want the admin_report of %s", got, report.Id)
	}
	if got := types("alice"); len(got) != 0 {
		t.Errorf("the reporter's notifications = %+v, want none", got)
	}
	if got := types("bob"); len(got) != 0 {
		t.Errorf("the target's notifications = %+v, want none", got)
	}
	// Another report of bob while the first is open notifies nobody.
	if res := c.do("carol", http.MethodPost, "/reports", map[string]any{"account_id": c.ids["bob"], "category": "spam"}, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	if got := types("alice"); len(got) != 0 {
		t.Errorf("alice's notifications after a second report = %+v, want none", got)
	}

	if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationApproval),
	}); err != nil {
		t.Fatal(err)
	}
	dave, err := c.app.Identity.SignUp(ctx, appidentity.SignUpInput{
		Username: "dave", Email: "dave@example.com", Password: "correct horse battery staple", Reason: "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"alice", "bob", "carol"} {
		got := types(user)
		if len(got) == 0 || got[0].Type != clientapi.NotificationTypeAdminSignUp || got[0].Account == nil || got[0].Account.Id != dave.AccountID {
			t.Errorf("%s's notifications = %+v, want the admin_sign_up of dave first", user, got)
		}
	}
	counts(clientapi.AdminCounts{OpenReports: n(2), PendingRegistrations: n(1)})
}
