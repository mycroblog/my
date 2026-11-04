// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestStatusReactionsEverywhere reads a post right after a reaction, with no wait, from every
// endpoint that returns a Status: the counts are a synchronous projection, so each already shows
// the reaction, with `me` for a viewer who is signed in and without it otherwise.
func TestStatusReactionsEverywhere(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	follow := func(user, target string) {
		t.Helper()
		if res := c.do(user, http.MethodPost, "/accounts/"+c.ids[target]+"/follow", nil, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("%s follows %s = %d", user, target, res.StatusCode)
		}
	}
	follow("alice", "bob")
	follow("alice", "carol")
	var list clientapi.List
	if res := c.do("alice", http.MethodPost, "/lists", map[string]any{"name": "friends"}, &list); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /lists = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodPost, "/lists/"+list.Id+"/accounts/"+c.ids["bob"], nil, nil); res.StatusCode/100 != 2 {
		t.Fatalf("add to the list = %d", res.StatusCode)
	}

	post := c.post("bob", map[string]any{"text": "reacted #reacted", "visibility": "public"})
	reply := c.post("carol", map[string]any{"text": "a reply", "visibility": "public", "in_reply_to_id": post.Id})
	quote := c.post("carol", map[string]any{"text": "a quote", "visibility": "public", "quoted_status_id": post.Id})
	if res := c.do("carol", http.MethodPost, "/statuses/"+post.Id+"/boost", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("boost = %d", res.StatusCode)
	}
	if res := c.do("bob", http.MethodPost, "/statuses/"+post.Id+"/pin", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("pin = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodPost, "/statuses/"+post.Id+"/bookmark", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("bookmark = %d", res.StatusCode)
	}
	var report clientapi.Report
	if res := c.do("carol", http.MethodPost, "/reports", map[string]any{
		"account_id": c.ids["bob"], "status_ids": []string{post.Id}, "category": "spam",
	}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}

	want := func(where, viewer string, st *clientapi.Status) {
		t.Helper()
		if st == nil {
			t.Errorf("%s as %q: the post is not there", where, viewer)
			return
		}
		if len(st.Reactions) != 1 || st.Reactions[0].Name != "🎉" || st.Reactions[0].Count != 1 {
			t.Errorf("%s as %q: reactions = %+v, want one 🎉", where, viewer, st.Reactions)
			return
		}
		me := st.Reactions[0].Me
		switch viewer {
		case "":
			if me != nil {
				t.Errorf("%s anonymously: me = %v, want it omitted", where, *me)
			}
		case "alice":
			if me == nil || !*me {
				t.Errorf("%s as alice: me = %v, want true", where, me)
			}
		default:
			if me == nil || *me {
				t.Errorf("%s as %s: me = %v, want false", where, viewer, me)
			}
		}
	}

	// The answer to the action itself, also with a token that only has the write scope.
	var acted clientapi.Status
	if res := c.do(c.appToken("alice", "write:favourites"), http.MethodPut, "/statuses/"+post.Id+"/reaction", map[string]any{"emoji": "🎉"}, &acted); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT reaction = %d", res.StatusCode)
	}
	want("PUT reaction", "alice", &acted)

	single := func(user, path string) {
		t.Helper()
		var st clientapi.Status
		if res := c.do(user, http.MethodGet, path, nil, &st); res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s as %q = %d", path, user, res.StatusCode)
		}
		want(path, user, &st)
	}
	for _, user := range []string{"alice", "carol", ""} {
		single(user, "/statuses/"+post.Id)
	}

	page := func(user, path string) {
		t.Helper()
		var found *clientapi.Status
		for _, st := range c.statusPage(user, path).Items {
			if st.Id == post.Id {
				found = &st
			}
		}
		want(path, user, found)
	}
	for _, r := range []struct{ user, path string }{
		{"alice", "/timelines/local"},
		{"", "/timelines/local"},
		{"alice", "/timelines/federated"},
		{"alice", "/timelines/tag/reacted"},
		{"alice", "/timelines/list/" + list.Id},
		{"alice", "/accounts/" + c.ids["bob"] + "/statuses"},
		{"", "/accounts/" + c.ids["bob"] + "/statuses"},
		{"alice", "/accounts/" + c.ids["bob"] + "/statuses?pinned=true"},
		{"alice", "/bookmarks"},
	} {
		page(r.user, r.path)
	}

	// A boost in a timeline: the status of the booster has no reactions of its own, as the
	// schema says, and the boosted status has them.
	var boosted *clientapi.Status
	for _, st := range c.statusPage("alice", "/timelines/home").Items {
		if st.Boost != nil && st.Boost.Id == post.Id {
			boosted = st.Boost
			if len(st.Reactions) != 0 {
				t.Errorf("the status of the booster has reactions %+v", st.Reactions)
			}
		}
	}
	want("the boost in the home timeline", "alice", boosted)

	// In the thread of a reply.
	var thread clientapi.StatusContext
	if res := c.do("alice", http.MethodGet, "/statuses/"+reply.Id+"/context", nil, &thread); res.StatusCode != http.StatusOK || len(thread.Ancestors) != 1 {
		t.Fatalf("GET context = %d, %d ancestors", res.StatusCode, len(thread.Ancestors))
	}
	want("the ancestor in a thread", "alice", &thread.Ancestors[0])

	// Quoted by another post.
	var quoting clientapi.Status
	if res := c.do("alice", http.MethodGet, "/statuses/"+quote.Id, nil, &quoting); res.StatusCode != http.StatusOK || quoting.Quote == nil {
		t.Fatalf("GET quote = %d, %+v", res.StatusCode, quoting.Quote)
	}
	want("the quoted post", "alice", quoting.Quote.QuotedStatus)

	// In a notification of its author.
	var notifications clientapi.NotificationPage
	if res := c.do("bob", http.MethodGet, "/notifications", nil, &notifications); res.StatusCode != http.StatusOK {
		t.Fatalf("GET notifications = %d", res.StatusCode)
	}
	var notified *clientapi.Status
	for _, n := range notifications.Items {
		if n.Type == clientapi.NotificationTypeReaction {
			notified = n.Status
		}
	}
	want("the notification of the reaction", "bob", notified)

	// As a moderator reviews it, without the moderator's relation to it.
	reported := func() clientapi.Status {
		t.Helper()
		var detail clientapi.AdminReportDetail
		if res := c.do("carol", http.MethodGet, "/admin/reports/"+report.Id, nil, &detail); res.StatusCode != http.StatusOK || len(detail.Statuses) != 1 {
			t.Fatalf("GET report = %d, %d statuses", res.StatusCode, len(detail.Statuses))
		}
		return detail.Statuses[0].Status
	}
	st := reported()
	want("the report", "", &st)

	// The list of the accounts that reacted says the same.
	var reactors clientapi.StatusReactionPage
	if res := c.do("carol", http.MethodGet, "/statuses/"+post.Id+"/reactions", nil, &reactors); res.StatusCode != http.StatusOK || len(reactors.Items) != 1 {
		t.Errorf("GET reactions = %d, %d items", res.StatusCode, len(reactors.Items))
	}

	// A deleted post leaves the counts, as it leaves its other counters.
	if res := c.do("bob", http.MethodDelete, "/statuses/"+post.Id, nil, nil); res.StatusCode/100 != 2 {
		t.Fatalf("DELETE = %d", res.StatusCode)
	}
	if st := reported(); len(st.Reactions) != 0 || st.Content != post.Content {
		t.Errorf("the deleted post in the report: reactions %+v, content %q", st.Reactions, st.Content)
	}
}
