// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// adminListQueries counts the database round trips of one page of the reports and one page of the
// accounts of the administration, with n rows each: n accounts, each of which reports the next.
// Every query is one round trip (one call in pg_stat_statements).
func adminListQueries(t *testing.T, n int) (reports, accounts int) {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"admin"}
	for i := range n {
		users = append(users, fmt.Sprintf("u%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	for i, u := range users[1:] {
		target := users[1+(i+1)%n]
		if res := c.do(u, http.MethodPost, "/reports", map[string]any{"account_id": c.ids[target], "category": "spam"}, nil); res.StatusCode != http.StatusCreated {
			t.Fatalf("%s reports %s = %d", u, target, res.StatusCode)
		}
	}
	total := func() int {
		count := 0
		for _, c := range ops(tracer) {
			count += c
		}
		return count
	}
	before := total()
	var rp clientapi.AdminReportPage
	if res := c.do("admin", http.MethodGet, fmt.Sprintf("/admin/reports?limit=%d", n), nil, &rp); res.StatusCode != http.StatusOK || len(rp.Items) != n {
		t.Fatalf("GET /admin/reports = %d with %d reports, want %d", res.StatusCode, len(rp.Items), n)
	}
	reports = total() - before
	before = total()
	var ap clientapi.AdminAccountPage
	if res := c.do("admin", http.MethodGet, fmt.Sprintf("/admin/accounts?limit=%d", n), nil, &ap); res.StatusCode != http.StatusOK || len(ap.Items) != n {
		t.Fatalf("GET /admin/accounts = %d with %d accounts, want %d", res.StatusCode, len(ap.Items), n)
	}
	accounts = total() - before
	return reports, accounts
}

// TestAdminListQueries checks that a page of the administration's reports and accounts makes a
// number of round trips that does not grow with its rows.
func TestAdminListQueries(t *testing.T) {
	t.Parallel()
	smallReports, smallAccounts := adminListQueries(t, 5)
	largeReports, largeAccounts := adminListQueries(t, 20)
	t.Logf("reports: %d queries for 5, %d for 20; accounts: %d for 5, %d for 20", smallReports, largeReports, smallAccounts, largeAccounts)
	if largeReports > smallReports {
		t.Errorf("%d queries for 20 reports, more than the %d for 5", largeReports, smallReports)
	}
	if largeAccounts > smallAccounts {
		t.Errorf("%d queries for 20 accounts, more than the %d for 5", largeAccounts, smallAccounts)
	}
}
