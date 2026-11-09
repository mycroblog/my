// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// mentionQueries counts, by operation, the database round trips of one post by alice that
// mentions n local accounts. Every query is one round trip (one call in pg_stat_statements).
func mentionQueries(t *testing.T, n int) map[string]int {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"alice"}
	var mentions []string
	for i := range n {
		users = append(users, fmt.Sprintf("u%d", i))
		mentions = append(mentions, fmt.Sprintf("@u%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	was := ops(tracer)
	c.post("alice", map[string]any{"text": "hello " + strings.Join(mentions, " "), "visibility": "public"})
	out := map[string]int{}
	for k, v := range ops(tracer) {
		if d := v - was[k]; d > 0 {
			out[k] = d
		}
	}
	for _, u := range users[1:] {
		var page clientapi.NotificationPage
		if res := c.do(u, http.MethodGet, "/notifications?types[]=mention", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("%s has %d mention notifications (%d), want 1", u, len(page.Items), res.StatusCode)
		}
	}
	return out
}

// ops counts the queries a tracer recorded by operation.
func ops(tracer *testkit.Tracer) map[string]int {
	out := map[string]int{}
	for _, sp := range tracer.Spans() {
		if op := sp.Attrs["db.operation.name"]; op != "" {
			out[op]++
		}
	}
	return out
}

// notificationOps are the operations that notifying the recipients of a status makes: reading the
// recipients, their relations with the author, their settings and domain exclusions, and writing
// the notifications and their messages. Resolving the mentions of the text is #2695.
var notificationOps = []string{
	"ListAccountsByIDs", "ListLifecycleStatesByIDs", "GetBlockEdge", "GetActiveMute", "GetFollowEdge",
	"ListBlockEdgesFrom", "ListBlockEdgesTo", "ListActiveMutesTo", "ListFollowEdgesTo",
	"DomainExcluded", "DomainsExcludedAmong", "ListNotificationSettings", "ListNotificationSettingsOf",
	"InsertNotification", "InsertNotificationsFor", "ListPushSubscriptionIDsForAlert", "ListPushSubscriptionIDsForAlertOf",
}

// TestMentionNotificationQueries checks that notifying the accounts that a status mentions makes a
// number of round trips that does not grow with them, and notifies each of them.
func TestMentionNotificationQueries(t *testing.T) {
	t.Parallel()
	count := func(byOp map[string]int) int {
		n := 0
		for _, op := range notificationOps {
			n += byOp[op]
		}
		return n
	}
	small, large := count(mentionQueries(t, 5)), count(mentionQueries(t, 15))
	t.Logf("%d queries to notify 5 mentioned accounts, %d to notify 15", small, large)
	if large > small {
		t.Errorf("%d queries to notify 15 mentioned accounts, more than the %d for 5", large, small)
	}
}
