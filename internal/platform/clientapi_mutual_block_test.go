// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
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

	if r := e.relate("block", alice, bob, nil); !set(r.Blocking) {
		t.Errorf("blocking back = %+v, want blocking", r)
	}
	if page := e.page("/blocks", alice.token); len(page.Items) != 1 || page.Items[0].Id != bob.id {
		t.Errorf("blocks = %+v, want bob", page.Items)
	}
	if r := e.relate("unblock", alice, bob, nil); set(r.Blocking) {
		t.Errorf("unblocking in a mutual block = %+v, want not blocking", r)
	}
	if r := e.relate("unmute", alice, bob, nil); set(r.Muting) {
		t.Errorf("unmuting an account that blocks the user = %+v, want not muting", r)
	}

	// What shows or changes more than the user's own edge still answers 404.
	path := "/accounts/" + bob.id
	e.want(e.do(http.MethodGet, path, alice.token, nil, nil), http.StatusNotFound, "get the blocker")
	e.want(e.do(http.MethodPost, path+"/follow", alice.token, nil, nil), http.StatusNotFound, "follow the blocker")
	e.want(e.do(http.MethodPost, path+"/mute", alice.token, nil, nil), http.StatusNotFound, "mute the blocker")

	// An unknown account stays a 404 for the three.
	const unknown = "/accounts/01890a5d-ac96-774b-bcce-b302099a8057"
	e.want(e.do(http.MethodPost, unknown+"/block", alice.token, nil, nil), http.StatusNotFound, "block unknown")
	for _, relation := range []string{"/block", "/mute"} {
		e.want(e.do(http.MethodDelete, unknown+relation, alice.token, nil, nil), http.StatusNotFound, "end "+relation+" of unknown")
	}
}

// TestListAccountStatusesAnswersLikeGetAccount checks that the posts of an account that blocks the
// viewer or that moderation hides are a 404, the same as the account itself.
func TestListAccountStatusesAnswersLikeGetAccount(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	mod, alice, bob, carol := e.user("mod"), e.user("alice"), e.user("bob"), e.user("carol")
	e.setRole(carol, "") // the moderator suspends her below, which needs a lower rank

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

// TestReportTargetAsTheReporterAndTheModerators checks that a reporter sees of the target what
// getAccount shows them, with the report still listed, in each way that a report reaches them,
// while the moderators keep the whole target.
func TestReportTargetAsTheReporterAndTheModerators(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	mod := e.user("mod")
	bob := e.user("bob")
	file := func(of testUser) clientapi.Report {
		t.Helper()
		var r clientapi.Report
		e.want(e.do(http.MethodPost, "/reports", bob.token, map[string]any{"account_id": of.id, "category": "spam"}, &r), http.StatusOK, "file a report")
		return r
	}
	// own returns the report as the reporter's list shows it, failing when it is not listed.
	own := func(id string) clientapi.Report {
		t.Helper()
		var page clientapi.ReportPage
		e.want(e.do(http.MethodGet, "/reports", bob.token, nil, &page), http.StatusOK, "own reports")
		for _, r := range page.Items {
			if r.Id == id {
				return r
			}
		}
		t.Fatalf("report %s is not listed: %+v", id, page.Items)
		return clientapi.Report{}
	}
	wantShown := func(r clientapi.Report, of testUser, what string) {
		t.Helper()
		if r.TargetAccount == nil || r.TargetAccount.Id != of.id {
			t.Errorf("%s: target = %+v, want %s", what, r.TargetAccount, of.id)
		}
		e.want(e.do(http.MethodGet, "/accounts/"+of.id, bob.token, nil, nil), http.StatusOK, what+": the normal view")
	}
	wantHidden := func(r clientapi.Report, of testUser, what string) {
		t.Helper()
		if r.TargetAccount != nil {
			t.Errorf("%s: target = %+v, want none", what, r.TargetAccount)
		}
		e.want(e.do(http.MethodGet, "/accounts/"+of.id, bob.token, nil, nil), http.StatusNotFound, what+": the normal view")
	}
	// moderatorSees checks both moderators' views.
	moderatorSees := func(id string, of testUser, what string) {
		t.Helper()
		var detail clientapi.AdminReportDetail
		e.want(e.do(http.MethodGet, "/admin/reports/"+id, mod.token, nil, &detail), http.StatusOK, what+": report detail")
		if detail.TargetAccount == nil || detail.TargetAccount.Id != of.id {
			t.Errorf("%s: moderator's detail target = %+v, want %s", what, detail.TargetAccount, of.id)
		}
		if detail.ReporterAccount == nil || detail.ReporterAccount.Id != bob.id {
			t.Errorf("%s: moderator's detail reporter = %+v, want %s", what, detail.ReporterAccount, bob.id)
		}
		var page clientapi.AdminReportPage
		e.want(e.do(http.MethodGet, "/admin/reports", mod.token, nil, &page), http.StatusOK, what+": reports")
		for _, r := range page.Items {
			if r.Id == id && (r.TargetAccount == nil || r.TargetAccount.Id != of.id) {
				t.Errorf("%s: moderator's list target = %+v, want %s", what, r.TargetAccount, of.id)
			}
		}
	}

	// A target that blocks the reporter, for as long as it does.
	blocker := e.user("blocker")
	filed := file(blocker)
	wantShown(filed, blocker, "before the block")
	wantShown(own(filed.Id), blocker, "listed before the block")
	e.relate("block", blocker, bob, nil)
	wantHidden(own(filed.Id), blocker, "blocks the reporter")
	moderatorSees(filed.Id, blocker, "blocks the reporter")
	e.relate("unblock", blocker, bob, nil)
	wantShown(own(filed.Id), blocker, "after the unblock")

	// A report filed on an account that already blocks the reporter.
	e.relate("block", blocker, bob, nil)
	wantHidden(file(blocker), blocker, "filed on a blocker")

	// The reporter's own block hides nothing, as in the normal view.
	blocked := e.user("blocked")
	ownBlock := file(blocked)
	e.relate("block", bob, blocked, nil)
	wantShown(own(ownBlock.Id), blocked, "blocked by the reporter")
	e.relate("unblock", bob, blocked, nil)

	// A suspended target, until the suspension is lifted, and the resolution's notification.
	suspended := e.user("suspended")
	e.setRole(suspended, "") // the moderator suspends the account below, which needs a lower rank
	report := file(suspended)
	var action clientapi.AdminAccountAction
	e.want(e.do(http.MethodPost, "/admin/accounts/"+suspended.id+"/actions", mod.token, map[string]any{"kind": "suspend"}, &action), http.StatusOK, "suspend")
	wantHidden(own(report.Id), suspended, "suspended")
	moderatorSees(report.Id, suspended, "suspended")
	e.want(e.do(http.MethodPost, "/admin/reports/"+report.Id+"/resolve", mod.token, map[string]any{"outcome": "dismissed"}, nil), http.StatusOK, "resolve")
	var notifications clientapi.NotificationPage
	e.want(e.do(http.MethodGet, "/notifications", bob.token, nil, &notifications), http.StatusOK, "notifications")
	found := false
	for _, n := range notifications.Items {
		if n.Report != nil && n.Report.Id == report.Id {
			found = true
			if n.Report.TargetAccount != nil {
				t.Errorf("the notification's report shows the suspended target: %+v", n.Report.TargetAccount)
			}
		}
	}
	if !found {
		t.Errorf("no notification tells of report %s: %+v", report.Id, notifications.Items)
	}
	e.want(e.do(http.MethodPost, "/admin/account_actions/"+action.Id+"/lift", mod.token, map[string]any{}, nil), http.StatusOK, "lift")
	wantShown(own(report.Id), suspended, "after the suspension")

	// A target that deleted its account stays listed, without the account.
	leaving := e.user("leaving")
	left := file(leaving)
	e.want(e.do(http.MethodPost, "/accounts/delete", leaving.token, map[string]any{"password": "correct horse battery staple"}, nil), http.StatusNoContent, "delete")
	wantHidden(own(left.Id), leaving, "deleted")
}
