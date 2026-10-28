// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeSuggester records the requests and returns suggestions.
type fakeSuggester struct {
	suggestions []appaccount.Suggestion
	calls       []string
}

func (f *fakeSuggester) Suggest(_ context.Context, viewerID string, limit int) ([]appaccount.Suggestion, error) {
	f.calls = append(f.calls, fmt.Sprintf("%s %d", viewerID, limit))
	return f.suggestions, nil
}

// TestListAccountSuggestions checks the parameters, the token requirement, and the presentation
// of the suggestions; the tests of internal/app/account and the platform check what is
// suggested.
func TestListAccountSuggestions(t *testing.T) {
	suggester := &fakeSuggester{suggestions: []appaccount.Suggestion{
		{AccountID: "01890a5d-ac96-774b-bcce-b302099a80ff", Source: appaccount.SuggestionFeatured},
		{AccountID: aliceAccountID, Source: appaccount.SuggestionRecentlyActive},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.AccountSuggestions = suggester })
	token := map[string]string{"Authorization": "Bearer read-accounts"}

	if res := e.get(t, "/accounts/suggestions", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous suggestions = %d, want 401", res.StatusCode)
	}
	for _, q := range []string{"limit=0", "limit=41"} {
		if res := e.get(t, "/accounts/suggestions?"+q, token); res.StatusCode != http.StatusBadRequest {
			t.Errorf("suggestions %s = %d, want 400", q, res.StatusCode)
		}
	}

	suggester.calls = nil
	res := e.get(t, "/accounts/suggestions", token)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("suggestions = %d (Cache-Control %q)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	var got []clientapi.AccountSuggestion
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	// The unknown account is left out.
	if len(got) != 1 || got[0].Account.Id != aliceAccountID || got[0].Source != clientapi.AccountSuggestionSourceRecentlyActive {
		t.Errorf("suggestions = %+v, want alice, recently active", got)
	}
	if res := e.get(t, "/accounts/suggestions?limit=40", token); res.StatusCode != http.StatusOK {
		t.Errorf("suggestions with limit 40 = %d", res.StatusCode)
	}
	want := []string{aliceAccountID + " 10", aliceAccountID + " 40"}
	if strings.Join(suggester.calls, "|") != strings.Join(want, "|") {
		t.Errorf("requests = %q, want %q", suggester.calls, want)
	}
}
