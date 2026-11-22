// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestNotificationRequestsShowTheirLastNotificationAndSources checks the requests of the
// notifications that alice's policy filters: a request whose newest notification a hide keyword
// filter leaves out has no last_notification, and the next request still has its own; and the
// request of a source that alice may not see, a suspended account, is left out of the list and
// answers 404 by its ID.
func TestNotificationRequestsShowTheirLastNotificationAndSources(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "bob", "carol", "eve")
	if res := e.send("alice", http.MethodPatch, "/preferences/notification_policy", map[string]any{"not_following": "filter"}, nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH the policy = %s", e.outcome(res))
	}
	filter := map[string]any{
		"title": "Sale", "action": "hide", "contexts": []string{"notifications"},
		"keywords": []map[string]any{{"keyword": "sale", "whole_word": false}},
	}
	if res := e.send("alice", http.MethodPost, "/filters", filter, nil, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /filters = %s", e.outcome(res))
	}
	e.post("eve", map[string]any{"text": "hi @alice", "visibility": "public"})
	e.drain()
	e.suspend("eve")
	carol := e.post("carol", map[string]any{"text": "hello @alice", "visibility": "public"})
	e.post("bob", map[string]any{"text": "a sale for @alice", "visibility": "public"})
	e.drain()

	var page clientapi.NotificationRequestPage
	if res := e.send("alice", http.MethodGet, "/notifications/requests", nil, nil, &page); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications/requests = %s", e.outcome(res))
	}
	byID := map[string]clientapi.NotificationRequest{}
	var order []string
	for _, r := range page.Items {
		byID[r.Id] = r
		order = append(order, r.Id)
	}
	if len(order) != 2 || order[0] != e.ids["bob"] || order[1] != e.ids["carol"] {
		t.Fatalf("requests = %v, want bob's and carol's, without the suspended eve's", order)
	}
	if r := byID[e.ids["bob"]]; r.LastNotification != nil {
		t.Errorf("bob's request shows the hidden notification %+v", r.LastNotification)
	}
	if r := byID[e.ids["carol"]]; r.LastNotification == nil || r.LastNotification.Status == nil || r.LastNotification.Status.Id != carol.Id {
		t.Errorf("carol's request = %+v, want her mention as its last notification", r.LastNotification)
	}
	if got := e.outcome(e.send("alice", http.MethodGet, "/notifications/requests/"+e.ids["eve"], nil, nil, nil)); got[:3] != "404" {
		t.Errorf("GET the request of the suspended eve = %s, want 404", got)
	}
}
