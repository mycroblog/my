// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestReportedStatusesShowThePollQuoteAndCard reads the poll, the quote, and the link card of
// reported statuses as a moderator on the real database. The moderator's own vote and blocks do not
// color them, and neither do the restrictions on the authors. A quoted post that is not the
// report's evidence stays under the visibility rules, and a deleted status keeps its poll and quote.
func TestReportedStatusesShowThePollQuoteAndCard(t *testing.T) {
	t.Parallel()
	web := &pageWeb{hits: map[string]int{}, pages: map[string]webPage{
		"https://news.example/a": {http.StatusOK, "text/html; charset=utf-8", cardPage("Big news", "")},
	}}
	c := newStatusesClientWith(t, nil, platform.AppOptions{HTTPClient: web}, "carol")
	// Everyone but carol, the moderator, is a member: no moderation action reaches an administrator.
	for _, name := range []string{"alice", "bob", "dave", "erin"} {
		r, err := c.app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: name,
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		c.ids[name], c.tokens[name] = r.AccountID, issueTokens(t, c.app, r.User.ID, "").AccessToken
	}
	ok := func(res *http.Response, what string) {
		t.Helper()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d", what, res.StatusCode)
		}
	}

	ok(c.do("alice", http.MethodPost, "/accounts/"+c.ids["bob"]+"/follow", nil, nil), "alice follows bob")
	// carol, the moderator, follows bob only to vote in his poll.
	ok(c.do("carol", http.MethodPost, "/accounts/"+c.ids["bob"]+"/follow", nil, nil), "follow")
	poll := c.post("bob", map[string]any{"text": "pick", "visibility": "private", "poll": map[string]any{"options": []string{"yes", "no"}, "expires_in": 3600}})
	ok(c.do("carol", http.MethodPost, "/statuses/"+poll.Id+"/poll/votes", map[string]any{"poll_id": poll.Poll.Id, "choices": []int{1}}, nil), "vote")
	ok(c.do("carol", http.MethodDelete, "/accounts/"+c.ids["bob"]+"/follow", nil, nil), "unfollow")

	quotedByDave := c.post("dave", map[string]any{"text": "dave says", "visibility": "public"})
	quotedByErin := c.post("erin", map[string]any{"text": "erin says", "visibility": "public"})
	quotesDave := c.post("bob", map[string]any{"text": "look", "visibility": "public", "quoted_status_id": quotedByDave.Id})
	quotesErin := c.post("bob", map[string]any{"text": "look too", "visibility": "public", "quoted_status_id": quotedByErin.Id})
	secret := c.post("bob", map[string]any{"text": "bob's secret", "visibility": "private"})
	quotesSecret := c.post("bob", map[string]any{"text": "as I said", "visibility": "private", "quoted_status_id": secret.Id})
	card := c.post("bob", map[string]any{"text": "read https://news.example/a", "visibility": "public"})
	fetchLinkCard(t, c.app, card.Id)

	var report clientapi.Report
	if res := c.do("alice", http.MethodPost, "/reports", map[string]any{
		"account_id": c.ids["bob"], "category": "spam",
		"status_ids": []string{poll.Id, quotesDave.Id, quotesErin.Id, quotesSecret.Id, card.Id},
	}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	read := func() map[string]clientapi.Status {
		t.Helper()
		var detail clientapi.AdminReportDetail
		ok(c.do("carol", http.MethodGet, "/admin/reports/"+report.Id, nil, &detail), "GET report")
		got := map[string]clientapi.Status{}
		for _, s := range detail.Statuses {
			got[s.Status.Id] = s.Status
		}
		return got
	}
	wantPoll := func(p *clientapi.Poll) {
		t.Helper()
		if p == nil || len(p.Options) != 2 || p.VotesCount != 1 {
			t.Fatalf("poll = %+v, want its options and the vote", p)
		}
		if p.Voted != nil || p.OwnVotes != nil {
			t.Errorf("poll = %+v, want no vote of the moderator", p)
		}
	}
	wantQuoted := func(st clientapi.Status, id string) {
		t.Helper()
		if st.Quote == nil || st.Quote.State != clientapi.QuoteStateAccepted || st.Quote.QuotedStatus == nil || st.Quote.QuotedStatus.Id != id {
			t.Errorf("quote of %s = %+v, want the accepted quote of %s", st.Id, st.Quote, id)
		}
	}
	wantSecretHidden := func(st clientapi.Status) {
		t.Helper()
		if q := st.Quote; q == nil || q.State != clientapi.QuoteStateUnauthorized || q.QuotedStatus != nil {
			t.Errorf("quote of a followers-only post = %+v, want it unauthorized and not shown", q)
		}
	}
	checkAll := func() {
		t.Helper()
		got := read()
		wantPoll(got[poll.Id].Poll)
		wantQuoted(got[quotesDave.Id], quotedByDave.Id)
		wantQuoted(got[quotesErin.Id], quotedByErin.Id)
		wantSecretHidden(got[quotesSecret.Id])
		if c := got[card.Id].Card; c == nil || c.Url != "https://news.example/a" {
			t.Errorf("card = %+v", c)
		}
	}
	checkAll()

	// The moderator's own blocks, either way, hide nothing here, while they do elsewhere.
	ok(c.do("carol", http.MethodPost, "/accounts/"+c.ids["bob"]+"/block", nil, nil), "carol blocks bob")
	ok(c.do("carol", http.MethodPost, "/accounts/"+c.ids["dave"]+"/block", nil, nil), "carol blocks dave")
	ok(c.do("erin", http.MethodPost, "/accounts/"+c.ids["carol"]+"/block", nil, nil), "erin blocks carol")
	checkAll()
	if res := c.do("carol", http.MethodGet, "/statuses/"+quotedByDave.Id, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the post of a blocked account elsewhere = %d, want 404", res.StatusCode)
	}

	// A status its author deleted keeps its poll and its quote. Its card goes with the status's
	// projection, and the link stays in its content.
	for _, st := range []clientapi.Status{poll, quotesDave, card} {
		if res := c.do("bob", http.MethodDelete, "/statuses/"+st.Id, nil, nil); res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
			t.Fatalf("DELETE %s = %d", st.Id, res.StatusCode)
		}
	}
	got := read()
	wantPoll(got[poll.Id].Poll)
	wantQuoted(got[quotesDave.Id], quotedByDave.Id)
	if st := got[card.Id]; st.Card != nil || !strings.Contains(st.Content, "https://news.example/a") {
		t.Errorf("deleted status with a link = card %+v, content %q; want no card and the link", st.Card, st.Content)
	}

	// The restrictions on the quoted authors and on the reported author hide nothing here either,
	// while the suspended and the limited posts are hidden from anyone else.
	action := func(accountID, kind string) {
		t.Helper()
		if res := c.do("carol", http.MethodPost, "/admin/accounts/"+accountID+"/actions", map[string]any{"kind": kind, "public_reason": "rules"}, nil); res.StatusCode != http.StatusCreated {
			t.Fatalf("%s %s = %d", kind, accountID, res.StatusCode)
		}
	}
	action(c.ids["dave"], "suspend")
	action(c.ids["erin"], "limit")
	action(c.ids["bob"], "suspend")
	got = read()
	wantPoll(got[poll.Id].Poll)
	wantQuoted(got[quotesDave.Id], quotedByDave.Id)
	wantQuoted(got[quotesErin.Id], quotedByErin.Id)
	wantSecretHidden(got[quotesSecret.Id])
	if res := c.do("alice", http.MethodGet, "/statuses/"+quotedByDave.Id, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the post of a suspended author elsewhere = %d, want 404", res.StatusCode)
	}
	if res := c.do("alice", http.MethodGet, "/statuses/"+quotedByErin.Id, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the post of a limited author elsewhere = %d, want 404", res.StatusCode)
	}
}
