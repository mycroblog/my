// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeAccountSearch records the searches and returns ids.
type fakeAccountSearch struct {
	ids   []string
	calls []string
}

func (f *fakeAccountSearch) Search(_ context.Context, viewerID, q string, limit int) ([]string, error) {
	if _, err := appaccount.NormalizeSearchQuery(q); err != nil {
		return nil, err
	}
	f.calls = append(f.calls, fmt.Sprintf("%s %s %d", viewerID, q, limit))
	return f.ids, nil
}

// TestSearchAccounts checks the parameters, the token requirement, and the presentation of the
// prefix search of accounts; the contract tests of the database adapters check the search.
func TestSearchAccounts(t *testing.T) {
	search := &fakeAccountSearch{ids: []string{aliceAccountID, "01890a5d-ac96-774b-bcce-b302099a80ff"}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.AccountSearch = search })
	token := map[string]string{"Authorization": "Bearer read-accounts"}

	if res := e.get(t, "/accounts/search?q=al", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous search = %d, want 401", res.StatusCode)
	}
	for _, q := range []string{"q=", "q=%40", "q=" + strings.Repeat("a", 101), "q=al&limit=0", "q=al&limit=21"} {
		if res := e.get(t, "/accounts/search?"+q, token); res.StatusCode != http.StatusBadRequest {
			t.Errorf("search %s = %d, want 400", q, res.StatusCode)
		}
	}
	if res := e.get(t, "/accounts/search?q=%40"+strings.Repeat("a", 100), token); res.StatusCode != http.StatusOK {
		t.Errorf("search of 100 characters after @ = %d, want 200", res.StatusCode)
	}

	search.calls = nil
	res := e.get(t, "/accounts/search?q="+url.QueryEscape("@Ali"), token)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "private, max-age=30" {
		t.Fatalf("search = %d (Cache-Control %q)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	var list clientapi.AccountList
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	got := list.Items
	// The unknown account is left out.
	if len(got) != 1 || got[0].Id != aliceAccountID {
		t.Errorf("search = %+v, want alice", got)
	}
	if res := e.get(t, "/accounts/search?q=al&limit=20", token); res.StatusCode != http.StatusOK {
		t.Errorf("search with limit 20 = %d", res.StatusCode)
	}
	want := []string{aliceAccountID + " @Ali 8", aliceAccountID + " al 20"}
	if strings.Join(search.calls, "|") != strings.Join(want, "|") {
		t.Errorf("searches = %q, want %q", search.calls, want)
	}
}
