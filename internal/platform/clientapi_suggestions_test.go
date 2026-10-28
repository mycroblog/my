// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestClientAPIAccountSuggestionsAndSearch covers the suggestions of accounts on the real
// database (FR-308): the featured accounts first, then the local accounts that posted publicly
// of late, without the viewer, the accounts it follows, and those that are not discoverable;
// and the search by the middle of a name, which an account that is not discoverable leaves
// (FR-307).
func TestClientAPIAccountSuggestionsAndSearch(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, bob, carol, dave, erin, frank := e.user("alice"), e.user("bob"), e.user("carol"), e.user("dave"), e.user("erin"), e.user("frank")

	if _, err := e.app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{
		Key: instance.SettingFeaturedAccounts, Value: "Frank, @erin",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{
		Key: instance.SettingFeaturedAccounts, Value: "nobody",
	}); err == nil {
		t.Error("featuring an unknown account = nil error")
	}
	var me clientapi.CredentialAccount
	e.want(e.do(http.MethodGet, "/accounts/verify_credentials", frank.token, nil, &me), http.StatusOK, "verify credentials")
	if !me.Discoverable {
		t.Error("a new account is not discoverable")
	}
	no := false
	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", frank.token, clientapi.UpdateCredentialsRequest{Discoverable: &no}, nil), http.StatusOK, "leave discovery")

	post := func(u testUser, visibility string) {
		t.Helper()
		e.want(e.do(http.MethodPost, "/statuses", u.token, map[string]string{"text": "hello", "visibility": visibility}, nil), http.StatusOK, "post")
	}
	post(alice, "public")
	post(dave, "private")
	post(carol, "public")
	post(bob, "local")
	e.relate("follow", alice, carol, nil)

	var got []clientapi.AccountSuggestion
	e.want(e.do(http.MethodGet, "/accounts/suggestions", alice.token, nil, &got), http.StatusOK, "suggestions")
	var gotIDs []string
	for _, s := range got {
		gotIDs = append(gotIDs, string(s.Source)+":"+s.Account.Id)
	}
	if want := []string{"featured:" + erin.id, "recently_active:" + bob.id}; !slices.Equal(gotIDs, want) {
		t.Errorf("suggestions = %v, want %v", gotIDs, want)
	}

	search := func(q string) []string {
		t.Helper()
		var accounts []clientapi.Account
		e.want(e.do(http.MethodGet, "/accounts/search?q="+q, alice.token, nil, &accounts), http.StatusOK, "search "+q)
		return ids(accounts)
	}
	if got := search("ri"); !slices.Equal(got, []string{erin.id}) {
		t.Errorf("search ri = %v, want erin", got)
	}
	if got := search("ran"); len(got) != 0 {
		t.Errorf("search ran = %v, want none: frank is not discoverable", got)
	}
	if got := search("frank"); !slices.Equal(got, []string{frank.id}) {
		t.Errorf("search frank = %v, want frank", got)
	}
}
