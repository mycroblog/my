// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestClientAPIAccountActions checks the actions on a local account through the composition
// root and the client API (FR-703, data-model §5.2): each takes away its capabilities at once,
// and lifting it gives them back; a user without manage_accounts may not issue one.
func TestClientAPIAccountActions(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	mod := e.user("mod")
	member := func(name string) testUser {
		t.Helper()
		r, err := e.app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: name,
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		return testUser{id: r.AccountID, token: issueTokens(t, e.app, r.User.ID, "").AccessToken}
	}
	alice, bob := member("alice"), member("bob")
	issue := func(kind string) clientapi.AdminAccountAction {
		t.Helper()
		var a clientapi.AdminAccountAction
		e.want(e.do(http.MethodPost, "/admin/accounts/"+alice.id+"/actions", mod.token, map[string]any{"kind": kind, "public_reason": "rules"}, &a), http.StatusOK, "issue "+kind)
		if a.Kind != clientapi.AdminAccountActionKind(kind) || a.State != clientapi.ModerationActionStateActive || a.AccountId != alice.id {
			t.Fatalf("issued %s = %+v", kind, a)
		}
		return a
	}
	lift := func(a clientapi.AdminAccountAction) {
		t.Helper()
		var got clientapi.AdminAccountAction
		e.want(e.do(http.MethodPost, "/admin/account_actions/"+a.Id+"/lift", mod.token, map[string]any{"reason": "appealed"}, &got), http.StatusOK, "lift "+string(a.Kind))
		if got.State != clientapi.ModerationActionStateLifted {
			t.Fatalf("lifted %s = %+v", a.Kind, got)
		}
	}
	post := func(who testUser, visibility string) *http.Response {
		t.Helper()
		return e.do(http.MethodPost, "/statuses", who.token, map[string]any{"text": "hello", "visibility": visibility}, nil)
	}
	var bobs clientapi.Status
	e.want(e.do(http.MethodPost, "/statuses", bob.token, map[string]any{"text": "hi", "visibility": "public"}, &bobs), http.StatusOK, "bob posts")

	// A user whose roles lack manage_accounts may not act on accounts.
	if typ := e.want(e.do(http.MethodPost, "/admin/accounts/"+bob.id+"/actions", alice.token, map[string]any{"kind": "suspend"}, nil), http.StatusForbidden, "issue by alice"); typ != clientapi.ProblemTypeInsufficientScope && typ != clientapi.ProblemTypeMissingPermission {
		t.Errorf("issue by alice = %s, want a 403 for the permission", typ)
	}

	// restrict_posting: no posting until lifted.
	a := issue("restrict_posting")
	e.want(post(alice, "public"), http.StatusForbidden, "post while restricted")
	lift(a)
	e.want(post(alice, "public"), http.StatusOK, "post after the lift")

	// restrict_interaction: no following, favouriting, or replying until lifted.
	a = issue("restrict_interaction")
	e.want(e.do(http.MethodPost, "/accounts/"+bob.id+"/follow", alice.token, nil, nil), http.StatusForbidden, "follow while restricted")
	e.want(e.do(http.MethodPost, "/statuses/"+bobs.Id+"/favourite", alice.token, nil, nil), http.StatusForbidden, "favourite while restricted")
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "re", "visibility": "public", "in_reply_to_id": bobs.Id}, nil),
		http.StatusForbidden, "reply while restricted")
	e.want(post(alice, "public"), http.StatusOK, "post while interaction is restricted")
	lift(a)
	e.relate("follow", alice, bob, nil)
	e.want(e.do(http.MethodPost, "/statuses/"+bobs.Id+"/favourite", alice.token, nil, nil), http.StatusOK, "favourite after the lift")

	// limit: new posts are followers-only.
	a = issue("limit")
	var limited clientapi.Status
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "hello", "visibility": "public"}, &limited), http.StatusOK, "post while limited")
	if limited.Visibility != clientapi.VisibilityPrivate {
		t.Errorf("visibility while limited = %s, want private", limited.Visibility)
	}
	lift(a)

	// suspend: the token stops working and the profile is hidden; the lift brings the
	// profile back, while the ended token stays ended.
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, bob.token, nil, nil), http.StatusOK, "alice's profile")
	a = issue("suspend")
	if typ := e.want(e.do(http.MethodGet, "/accounts/verify_credentials", alice.token, nil, nil), http.StatusUnauthorized, "alice while suspended"); typ != clientapi.ProblemTypeInvalidToken {
		t.Errorf("alice's token while suspended = %s, want invalid-token", typ)
	}
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, bob.token, nil, nil), http.StatusNotFound, "alice's profile while suspended")
	lift(a)
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, bob.token, nil, nil), http.StatusOK, "alice's profile after the lift")
	e.want(e.do(http.MethodGet, "/accounts/verify_credentials", alice.token, nil, nil), http.StatusUnauthorized, "alice's ended token after the lift")

	var page clientapi.AdminAccountActionPage
	e.want(e.do(http.MethodGet, "/admin/accounts/"+alice.id+"/actions", mod.token, nil, &page), http.StatusOK, "list")
	if len(page.Items) != 4 || page.Items[0].Kind != clientapi.AdminAccountActionKindSuspend || page.Items[0].LiftReason == nil {
		t.Errorf("actions on alice = %+v", page.Items)
	}
}
