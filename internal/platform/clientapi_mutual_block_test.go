// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestBlockedUserManagesTheirOwnEdges checks that a user whom an account blocks can still block it
// back and undo their own block and mute of it, while everything else answers as for an unknown
// account.
func TestBlockedUserManagesTheirOwnEdges(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, bob := e.user("alice"), e.user("bob")

	// Alice mutes bob, and then bob blocks alice.
	e.relate("mute", alice, bob, nil)
	e.relate("block", bob, alice, nil)

	if r := e.relate("block", alice, bob, nil); !r.Blocking {
		t.Errorf("blocking back = %+v, want blocking", r)
	}
	if page := e.page("/blocks", alice.token); len(page.Accounts) != 1 || page.Accounts[0].Id != bob.id {
		t.Errorf("blocks = %+v, want bob", page.Accounts)
	}
	if r := e.relate("unblock", alice, bob, nil); r.Blocking {
		t.Errorf("unblocking in a mutual block = %+v, want not blocking", r)
	}
	if r := e.relate("unmute", alice, bob, nil); r.Muting {
		t.Errorf("unmuting an account that blocks the user = %+v, want not muting", r)
	}

	// What shows or changes more than the user's own edge still answers 404.
	path := "/accounts/" + bob.id
	e.want(e.do(http.MethodGet, path, alice.token, nil, nil), http.StatusNotFound, "get the blocker")
	e.want(e.do(http.MethodPost, path+"/follow", alice.token, nil, nil), http.StatusNotFound, "follow the blocker")
	e.want(e.do(http.MethodPost, path+"/mute", alice.token, nil, nil), http.StatusNotFound, "mute the blocker")

	// An unknown account stays a 404 for the three.
	const unknown = "/accounts/01890a5d-ac96-774b-bcce-b302099a8057"
	for _, action := range []string{"/block", "/unblock", "/unmute"} {
		e.want(e.do(http.MethodPost, unknown+action, alice.token, nil, nil), http.StatusNotFound, action+" unknown")
	}
}

// TestListAccountStatusesAnswersLikeGetAccount checks that the posts of an account that blocks the
// viewer or that moderation hides are a 404, the same as the account itself.
func TestListAccountStatusesAnswersLikeGetAccount(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	mod, alice, bob, carol := e.user("mod"), e.user("alice"), e.user("bob"), e.user("carol")

	for _, u := range []testUser{alice, carol} {
		e.want(e.do(http.MethodPost, "/statuses", u.token, map[string]any{"text": "hello", "visibility": "public"}, nil), http.StatusOK, "post")
	}
	list := func(of testUser, token, query string, want int) {
		t.Helper()
		e.want(e.do(http.MethodGet, "/accounts/"+of.id+"/statuses"+query, token, nil, nil), want, "statuses of "+of.id+query)
		e.want(e.do(http.MethodGet, "/accounts/"+of.id, token, nil, nil), want, "account "+of.id)
	}

	list(alice, bob.token, "", http.StatusOK)
	e.relate("block", alice, bob, nil)
	list(alice, bob.token, "", http.StatusNotFound)
	list(alice, bob.token, "?pinned=true", http.StatusNotFound)
	list(alice, alice.token, "", http.StatusOK)
	list(alice, "", "", http.StatusOK)
	// The blocker still sees the account that it blocks.
	list(bob, alice.token, "", http.StatusOK)

	e.want(e.do(http.MethodPost, "/admin/accounts/"+carol.id+"/actions", mod.token, map[string]any{"kind": "suspend"}, nil), http.StatusOK, "suspend")
	for _, token := range []string{"", bob.token} {
		list(carol, token, "", http.StatusNotFound)
		list(carol, token, "?pinned=true", http.StatusNotFound)
	}
	e.want(e.do(http.MethodGet, "/accounts/01890a5d-ac96-774b-bcce-b302099a8057/statuses", bob.token, nil, nil), http.StatusNotFound, "statuses of an unknown account")
}

// TestReportsShowATargetThatIsSuspendedOrBlocksTheReporter checks that the reporter's list and the
// moderators' views keep the whole target, which a report is about, after the target blocked the
// reporter and was suspended.
func TestReportsShowATargetThatIsSuspendedOrBlocksTheReporter(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	mod, bob, carol := e.user("mod"), e.user("bob"), e.user("carol")

	var filed clientapi.Report
	e.want(e.do(http.MethodPost, "/reports", bob.token, map[string]any{"account_id": carol.id, "category": "spam"}, &filed), http.StatusOK, "file the report")
	e.relate("block", carol, bob, nil)
	e.want(e.do(http.MethodPost, "/admin/accounts/"+carol.id+"/actions", mod.token, map[string]any{"kind": "suspend"}, nil), http.StatusOK, "suspend")

	var own clientapi.ReportPage
	e.want(e.do(http.MethodGet, "/reports", bob.token, nil, &own), http.StatusOK, "own reports")
	if len(own.Items) != 1 || own.Items[0].TargetAccount.Id != carol.id || own.Items[0].TargetAccount.Username != "carol" {
		t.Errorf("own reports = %+v, want the target", own.Items)
	}

	var detail clientapi.AdminReportDetail
	e.want(e.do(http.MethodGet, "/admin/reports/"+filed.Id, mod.token, nil, &detail), http.StatusOK, "report detail")
	if detail.TargetAccount == nil || detail.TargetAccount.Id != carol.id || detail.TargetAccount.Username != "carol" {
		t.Errorf("report detail target = %+v, want the account", detail.TargetAccount)
	}
	if detail.ReporterAccount == nil || detail.ReporterAccount.Id != bob.id {
		t.Errorf("report detail reporter = %+v, want the account", detail.ReporterAccount)
	}

	res := e.do(http.MethodGet, "/admin/reports", mod.token, nil, nil)
	e.want(res, http.StatusOK, "reports")
	var page clientapi.AdminReportPage
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].TargetAccount == nil || page.Items[0].TargetAccount.Id != carol.id {
		t.Errorf("reports = %+v, want the target", page.Items)
	}
}
