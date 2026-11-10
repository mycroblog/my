// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// accountListQueries counts the database round trips of one request of each account list of bob, by
// name,
// with n accounts on it: n accounts follow bob, react to his post, and are followed and muted by
// him, and bob reads his relationships with them. Every query is one round trip (one call in pg_stat_statements).
func accountListQueries(t *testing.T, n int) map[string]int {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"bob"}
	for i := range n {
		users = append(users, fmt.Sprintf("u%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	reacted := c.post("bob", map[string]any{"text": "react to this", "visibility": "public"})
	for _, u := range users[1:] {
		for _, req := range []struct {
			by, method, path string
			body             map[string]any
		}{
			{u, http.MethodPost, "/accounts/" + c.ids["bob"] + "/follow", nil},
			{u, http.MethodPut, "/statuses/" + reacted.Id + "/reaction", map[string]any{"emoji": "🎉"}},
			{"bob", http.MethodPost, "/accounts/" + c.ids[u] + "/follow", nil},
			{"bob", http.MethodPost, "/accounts/" + c.ids[u] + "/mute", nil},
		} {
			if res := c.do(req.by, req.method, req.path, req.body, nil); res.StatusCode/100 != 2 {
				t.Fatalf("%s %s %s = %d", req.by, req.method, req.path, res.StatusCode)
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
	out := map[string]int{}
	count := func(name, path string, page any, items func() int) {
		before := queries()
		if !strings.Contains(path, "?") {
			path += fmt.Sprintf("?limit=%d", n)
		}
		if res := c.do("bob", http.MethodGet, path, nil, page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", path, res.StatusCode)
		}
		out[name] = queries() - before
		if got := items(); got != n {
			t.Fatalf("%s has %d accounts, want %d", path, got, n)
		}
	}
	var followers, following clientapi.AccountPage
	var reactions clientapi.StatusReactionPage
	var mutes clientapi.MutePage
	count("followers", "/accounts/"+c.ids["bob"]+"/followers", &followers, func() int { return len(followers.Items) })
	count("following", "/accounts/"+c.ids["bob"]+"/following", &following, func() int { return len(following.Items) })
	count("reactions", "/statuses/"+reacted.Id+"/reactions", &reactions, func() int { return len(reactions.Items) })
	count("mutes", "/mutes", &mutes, func() int { return len(mutes.Items) })
	var rels clientapi.RelationshipList
	q := url.Values{}
	for _, u := range users[1:] {
		q.Add("id", c.ids[u])
	}
	count("relationships", "/accounts/relationships?"+q.Encode(), &rels, func() int { return len(rels.Items) })
	return out
}

// TestAccountListQueries checks that the round trips of a page of accounts do not grow with its
// accounts: the accounts, their restrictions, and the viewer's blocks are read together.
func TestAccountListQueries(t *testing.T) {
	t.Parallel()
	small, large := accountListQueries(t, 5), accountListQueries(t, 40)
	for name, n := range small {
		t.Logf("%s: %d queries for 5 accounts, %d for 40", name, n, large[name])
		if large[name] > n {
			t.Errorf("%s: %d queries for 40 accounts, more than the %d for 5", name, large[name], n)
		}
	}
}
