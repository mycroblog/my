// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestReportNotesReopenAndActionThroughTheAPI handles a local report through the admin API on
// the real database: a note stays with the moderators, a resolved report reopens, an action
// that follows from the report resolves it as actioned, and the reporter is notified of the
// first resolution only.
func TestReportNotesReopenAndActionThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	if _, err := c.app.Identity.RevokeRole(t.Context(), identity.RoleChangeInput{UserID: c.userIDs["bob"], RoleKey: domainidentity.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	const secret = "Probably the same person as before"
	var report clientapi.Report
	if res := c.do("alice", http.MethodPost, "/reports", map[string]any{"account_id": c.ids["bob"], "category": "spam"}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	var detail clientapi.AdminReportDetail
	if res := c.do("carol", http.MethodPost, "/admin/reports/"+report.Id+"/notes", map[string]any{"body": secret}, &detail); res.StatusCode != http.StatusOK ||
		len(detail.Notes) != 1 || detail.Notes[0].Body != secret || detail.Notes[0].AuthorAccountId == nil || *detail.Notes[0].AuthorAccountId != c.ids["carol"] {
		t.Fatalf("POST notes = %d %+v", res.StatusCode, detail.Notes)
	}
	if res := c.do("carol", http.MethodPost, "/admin/reports/"+report.Id+"/reopen", nil, nil); res.StatusCode != http.StatusConflict {
		t.Errorf("reopen an open report = %d, want 409", res.StatusCode)
	}
	if res := c.do("carol", http.MethodPost, "/admin/reports/"+report.Id+"/resolve", map[string]any{"outcome": "dismissed"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("resolve = %d", res.StatusCode)
	}
	if res := c.do("carol", http.MethodPost, "/admin/reports/"+report.Id+"/reopen", nil, &detail); res.StatusCode != http.StatusOK ||
		detail.State != clientapi.AdminReportStateOpen || detail.Outcome != nil {
		t.Fatalf("reopen = %d %+v, want open", res.StatusCode, detail)
	}
	if res := c.do("carol", http.MethodPost, "/admin/accounts/"+c.ids["bob"]+"/actions", map[string]any{
		"kind": "warn", "reason_category": "spam", "report_id": report.Id, "notify_target": false,
	}, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST actions = %d", res.StatusCode)
	}
	if res := c.do("carol", http.MethodGet, "/admin/reports/"+report.Id, nil, &detail); res.StatusCode != http.StatusOK ||
		detail.State != clientapi.AdminReportStateResolved || detail.Outcome == nil || *detail.Outcome != clientapi.AdminReportOutcomeActioned ||
		detail.TargetHistory != (clientapi.AdminReportTargetHistory{ReportsCount: 0, ActionsCount: 1}) {
		t.Errorf("report after the action = %d %+v, want resolved (actioned) with one action on bob", res.StatusCode, detail)
	}
	var audit clientapi.AdminAuditEntryPage
	if res := c.do("carol", http.MethodGet, "/admin/audit_log?target_kind=report&target_id="+report.Id, nil, &audit); res.StatusCode != http.StatusOK {
		t.Fatalf("GET audit_log = %d", res.StatusCode)
	}
	actions := []clientapi.AdminAuditAction{}
	for _, e := range audit.Items {
		actions = append(actions, e.Action)
	}
	want := []clientapi.AdminAuditAction{"report_resolved", "report_reopened", "report_resolved", "report_note_added"}
	if strings.Join(toStrings(actions), " ") != strings.Join(toStrings(want), " ") {
		t.Errorf("audit log of the report = %v, want %v", actions, want)
	}

	// The reporter is notified once and, like the target, never sees the note.
	var notifications clientapi.NotificationPage
	if res := c.do("alice", http.MethodGet, "/notifications", nil, &notifications); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications = %d", res.StatusCode)
	}
	resolved := 0
	for _, n := range notifications.Items {
		if n.Type == clientapi.NotificationTypeReportResolved {
			resolved++
		}
	}
	if resolved != 1 {
		t.Errorf("alice's report_resolved notifications = %d, want 1", resolved)
	}
	for _, read := range []struct{ user, path string }{
		{"alice", "/notifications"}, {"alice", "/reports"}, {"bob", "/notifications"}, {"bob", "/moderation/actions"},
	} {
		var raw json.RawMessage
		res := c.do(read.user, http.MethodGet, read.path, nil, &raw)
		if res.StatusCode == http.StatusOK && strings.Contains(string(raw), secret) {
			t.Errorf("GET %s by %s shows the note: %s", read.path, read.user, raw)
		}
	}
}

func toStrings[T ~string](xs []T) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = string(x)
	}
	return out
}
