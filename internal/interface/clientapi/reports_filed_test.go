// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeFiledReports keeps the reports of every reporter, ascending by ID.
type fakeFiledReports struct{ recs []moderation.ReportRecord }

func (f *fakeFiledReports) ReportsFiledBy(_ context.Context, reporterAccountID, before string, limit int) ([]moderation.ReportRecord, error) {
	var out []moderation.ReportRecord
	for _, r := range slices.Backward(f.recs) {
		if r.Report.ReporterAccountID == reporterAccountID && (before == "" || r.Report.ID < before) && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeFiledReports) ReportFiledBy(_ context.Context, reporterAccountID, id string) (moderation.ReportRecord, error) {
	for _, r := range f.recs {
		if r.Report.ID == id && r.Report.ReporterAccountID == reporterAccountID {
			return r, nil
		}
	}
	return moderation.ReportRecord{}, appmoderation.ErrNotFound
}

func filedReportID(i int) string { return fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a7%03d", i) }

// newFiledReportsEnv serves three reports of alice, the second resolved as actioned with an
// assignee, and one of bob, all of reportTargetID, with notifications ns.
func newFiledReportsEnv(t *testing.T, ns ...notification.Notification) *authEnv {
	t.Helper()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	rec := func(i int, reporter string) moderation.ReportRecord {
		return moderation.ReportRecord{
			Report: moderation.Report{
				ID: filedReportID(i), ReporterAccountID: reporter, TargetAccountID: reportTargetID, Category: moderation.CategorySpam,
				Comment: "spam", FiledAt: at.Add(time.Duration(i) * time.Minute),
			},
			Status: moderation.ReportStatus{ReportID: filedReportID(i), State: moderation.ReportOpen, Since: at},
		}
	}
	filed := &fakeFiledReports{recs: []moderation.ReportRecord{rec(1, aliceAccountID), rec(2, aliceAccountID), rec(3, aliceAccountID), rec(4, bobAccountID)}}
	filed.recs[1].Status = moderation.ReportStatus{
		ReportID: filedReportID(2), State: moderation.ReportResolved, Outcome: moderation.OutcomeActioned,
		AssigneeUserID: testkit.NamedID("moderator-user"), Since: at.Add(time.Hour),
	}
	return newAuthEnv(t, func(o *clientapi.Options) {
		accounts := fakeAccounts{}
		for id, a := range o.Accounts.(fakeAccounts) {
			accounts[id] = a
		}
		accounts[reportTargetID] = appaccount.Actor{
			Account:   account.Account{ID: reportTargetID, Domain: "remote.example", URI: "https://remote.example/users/bob", CreatedAt: createdAt},
			Handle:    &account.Handle{AccountID: reportTargetID, Username: "bob", Domain: "remote.example"},
			Lifecycle: account.InitialLifecycleState(reportTargetID),
		}
		o.Accounts = accounts
		o.FiledReports = filed
		o.Notifications = &fakeNotifications{ns: ns, dismissed: map[string]bool{}, markers: map[string]string{}}
	})
}

// readResolutionFree returns the body of res, checking that it tells nothing of how a report was resolved or who
// handled it.
func readResolutionFree(t *testing.T, res *http.Response) []byte {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"actioned", testkit.NamedID("moderator-user"), "outcome", "assignee"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the response has %q: %s", leak, raw)
		}
	}
	return raw
}

func TestListOwnReports(t *testing.T) {
	e := newFiledReportsEnv(t)
	res := e.send(t, http.MethodGet, "/reports?limit=2", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var page clientapi.ReportPage
	if err := json.Unmarshal(readResolutionFree(t, res), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Id != filedReportID(3) || page.Items[1].Id != filedReportID(2) || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	if page.Items[0].State != clientapi.ReportStateOpen || page.Items[1].State != clientapi.ReportStateResolved ||
		page.Items[1].TargetAccount.Acct != "bob@remote.example" {
		t.Errorf("states = %q, %q; target %+v", page.Items[0].State, page.Items[1].State, page.Items[1].TargetAccount)
	}
	res = e.send(t, http.MethodGet, "/reports?limit=2&cursor="+*page.NextCursor, "read", "")
	last := decode[clientapi.ReportPage](t, res)
	if len(last.Items) != 1 || last.Items[0].Id != filedReportID(1) || last.NextCursor != nil {
		t.Errorf("last page = %+v, want alice's first report only", last)
	}

	if res := e.send(t, http.MethodGet, "/reports", "write", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("without read:reports = %d, want 403", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, "/reports?limit=41", "read", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a limit out of range = %d, want 400", res.StatusCode)
	}
}

func TestReportResolvedNotification(t *testing.T) {
	at := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	e := newFiledReportsEnv(t, notification.Notification{
		ID: notificationID(1), RecipientAccountID: aliceAccountID, Type: notification.TypeReportResolved, ReportID: filedReportID(2), CreatedAt: at,
	}, notification.Notification{
		// A report that is gone, or bob's, leaves the notification without it.
		ID: notificationID(2), RecipientAccountID: aliceAccountID, Type: notification.TypeReportResolved, ReportID: filedReportID(4), CreatedAt: at,
	})
	res := e.send(t, http.MethodGet, "/notifications?types=report_resolved", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var page clientapi.NotificationPage
	if err := json.Unmarshal(readResolutionFree(t, res), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %+v, want both report_resolved", page.Items)
	}
	other, resolved := page.Items[0], page.Items[1]
	if other.Report != nil || other.Account != nil {
		t.Errorf("notification of another's report = %+v, want no report", other)
	}
	if resolved.Type != clientapi.NotificationTypeReportResolved || resolved.Account != nil || resolved.Report == nil ||
		resolved.Report.Id != filedReportID(2) || resolved.Report.State != clientapi.ReportStateResolved {
		t.Errorf("report_resolved = %+v (report %+v)", resolved, resolved.Report)
	}
}
