// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestReportedStatusesThroughTheAPI reads the statuses of a report as a moderator on the real
// database: followers-only and direct statuses that the moderator may not see elsewhere are in
// the report's detail, and so is a status that its author deleted afterwards, with when.
func TestReportedStatusesThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	if res := c.do("alice", http.MethodPost, "/accounts/"+c.ids["bob"]+"/follow", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("follow = %d", res.StatusCode)
	}
	private := c.post("bob", map[string]any{"text": "followers only", "visibility": "private"})
	direct := c.post("bob", map[string]any{"text": "@alice just you", "visibility": "direct"})
	var report clientapi.Report
	if res := c.do("alice", http.MethodPost, "/reports", map[string]any{
		"account_id": c.ids["bob"], "status_ids": []string{private.Id, direct.Id}, "category": "spam",
	}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	// The visibility policy still hides them from carol everywhere else.
	for _, id := range []string{private.Id, direct.Id} {
		if res := c.do("carol", http.MethodGet, "/statuses/"+id, nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET /statuses/%s by carol = %d, want 404", id, res.StatusCode)
		}
	}

	var detail clientapi.AdminReportDetail
	if res := c.do("carol", http.MethodGet, "/admin/reports/"+report.Id, nil, &detail); res.StatusCode != http.StatusOK {
		t.Fatalf("GET report = %d", res.StatusCode)
	}
	got := map[string]clientapi.AdminReportStatus{}
	for _, s := range detail.Statuses {
		got[s.Status.Id] = s
	}
	if len(detail.Statuses) != 2 || got[private.Id].Status.Content != private.Content || got[direct.Id].Status.Content != direct.Content ||
		got[private.Id].DeletedAt != nil || got[direct.Id].Status.Visibility != clientapi.VisibilityDirect {
		t.Fatalf("statuses of the report = %+v, want the private and the direct status", detail.Statuses)
	}

	if res := c.do("bob", http.MethodDelete, "/statuses/"+private.Id, nil, nil); res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE = %d", res.StatusCode)
	}
	if res := c.do("carol", http.MethodGet, "/admin/reports/"+report.Id, nil, &detail); res.StatusCode != http.StatusOK {
		t.Fatalf("GET report = %d", res.StatusCode)
	}
	got = map[string]clientapi.AdminReportStatus{}
	for _, s := range detail.Statuses {
		got[s.Status.Id] = s
	}
	if s, ok := got[private.Id]; !ok || s.DeletedAt == nil || s.Status.Content != private.Content {
		t.Errorf("deleted status in the report = %+v (found %v), want its content and when it was deleted", s, ok)
	}
	if got[direct.Id].DeletedAt != nil {
		t.Errorf("direct status = %+v, want it not deleted", got[direct.Id])
	}
}
