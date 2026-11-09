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

// notificationQueries counts the database round trips of one request of bob's notifications, on a
// page of 4 notifications from each of n accounts: a mention, a follow, a reaction, and a
// favourite. Every query is one round trip (one call in pg_stat_statements).
func notificationQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"bob"}
	for i := range n {
		users = append(users, fmt.Sprintf("u%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	reacted := c.post("bob", map[string]any{"text": "react to this", "visibility": "public"})
	liked := c.post("bob", map[string]any{"text": "favourite this", "visibility": "public"})
	for _, u := range users[1:] {
		c.post(u, map[string]any{"text": "hello @bob #hi", "visibility": "public"})
		for _, req := range []struct {
			method, path string
			body         map[string]any
		}{
			{http.MethodPost, "/accounts/" + c.ids["bob"] + "/follow", nil},
			{http.MethodPut, "/statuses/" + reacted.Id + "/reaction", map[string]any{"emoji": "🎉"}},
			{http.MethodPost, "/statuses/" + liked.Id + "/favourite", nil},
		} {
			if res := c.do(u, req.method, req.path, req.body, nil); res.StatusCode/100 != 2 {
				t.Fatalf("%s %s %s = %d", u, req.method, req.path, res.StatusCode)
			}
		}
	}
	queries := func() int {
		count := 0
		for _, sp := range tracer.Spans() {
			if sp.Attrs["db.operation.name"] != "" {
				count++
			}
		}
		return count
	}
	before := queries()
	var page clientapi.NotificationPage
	if res := c.do("bob", http.MethodGet, fmt.Sprintf("/notifications?limit=%d", 4*n), nil, &page); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications = %d", res.StatusCode)
	}
	if len(page.Items) != 4*n {
		t.Fatalf("bob has %d notifications, want %d", len(page.Items), 4*n)
	}
	return queries() - before
}

// TestNotificationQueries checks that the round trips of a page of notifications do not grow with
// its notifications and their accounts.
func TestNotificationQueries(t *testing.T) {
	t.Parallel()
	small, large := notificationQueries(t, 3), notificationQueries(t, 10)
	t.Logf("%d queries for 12 notifications, %d for 40", small, large)
	if large > small {
		t.Errorf("%d queries for 40 notifications, more than the %d for 12", large, small)
	}
}
