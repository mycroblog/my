// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestClientAPIFollowsVisibility checks follows_visibility (FR-311): a hidden list is a 403 for
// whoever may not read it, followers shows the lists to approved followers only, the owner always
// reads them, and the counts stay on the account.
func TestClientAPIFollowsVisibility(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, bob, carol, dave := e.user("alice"), e.user("bob"), e.user("carol"), e.user("dave")

	var me clientapi.CredentialAccount
	e.want(e.do(http.MethodGet, "/accounts/verify_credentials", alice.token, nil, &me), http.StatusOK, "verify_credentials")
	if me.FollowsVisibility != clientapi.FollowsVisibilityPublic {
		t.Errorf("new account's follows_visibility = %q, want public", me.FollowsVisibility)
	}
	followers := clientapi.FollowsVisibilityFollowers
	locked := true
	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", alice.token,
		clientapi.UpdateCredentialsRequest{Locked: &locked, FollowsVisibility: &followers}, &me), http.StatusOK, "hide the lists")
	if me.FollowsVisibility != clientapi.FollowsVisibilityFollowers {
		t.Fatalf("follows_visibility = %q, want followers", me.FollowsVisibility)
	}

	// Carol's follow is approved; Dave's request is not.
	e.relate("follow", carol, alice, nil)
	e.want(e.do(http.MethodPost, "/follow_requests/"+carol.id+"/accept", alice.token, nil, nil), http.StatusOK, "accept carol")
	e.relate("follow", dave, alice, nil)
	e.relate("follow", alice, bob, nil)

	lists := []string{"/accounts/" + alice.id + "/followers", "/accounts/" + alice.id + "/following"}
	hidden := func(who string, token string) {
		t.Helper()
		for _, path := range lists {
			if typ := e.want(e.do(http.MethodGet, path, token, nil, nil), http.StatusForbidden, who+" "+path); typ != clientapi.ProblemTypeFollowsHidden {
				t.Errorf("%s %s type = %s, want follows-hidden", who, path, typ)
			}
		}
	}
	readable := func(who string, token string) {
		t.Helper()
		if p := e.page(lists[0], token); !slices.Equal(ids(p.Items), []string{carol.id}) {
			t.Errorf("%s reads alice's followers %v, want carol", who, ids(p.Items))
		}
		if p := e.page(lists[1], token); !slices.Equal(ids(p.Items), []string{bob.id}) {
			t.Errorf("%s reads alice's follows %v, want bob", who, ids(p.Items))
		}
	}
	hidden("anonymous", "")
	hidden("bob, whom alice follows", bob.token)
	hidden("dave, whose request is pending", dave.token)
	readable("carol, an approved follower", carol.token)
	readable("alice", alice.token)

	// The counts stay visible.
	var got clientapi.Account
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, "", nil, &got), http.StatusOK, "alice")
	if got.FollowersCount == nil || *got.FollowersCount != 1 || got.FollowingCount == nil || *got.FollowingCount != 1 {
		t.Errorf("alice's counts = %v / %v, want 1 / 1", got.FollowersCount, got.FollowingCount)
	}

	private := clientapi.FollowsVisibilityPrivate
	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", alice.token,
		clientapi.UpdateCredentialsRequest{FollowsVisibility: &private}, nil), http.StatusOK, "keep the lists private")
	hidden("carol, once the lists are private", carol.token)
	readable("alice, whose lists are private", alice.token)

	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", alice.token,
		map[string]any{"follows_visibility": "nobody"}, nil), http.StatusUnprocessableEntity, "unknown follows_visibility")

	public := clientapi.FollowsVisibilityPublic
	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", alice.token,
		clientapi.UpdateCredentialsRequest{FollowsVisibility: &public}, nil), http.StatusOK, "show the lists")
	readable("anonymous, once the lists are public", "")
}
