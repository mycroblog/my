// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestGetAccount checks an anonymous profile and the localized 404 of an unknown account; the
// platform tests drive the account endpoints on the real database.
func TestGetAccount(t *testing.T) {
	e := newAuthEnv(t, nil)
	res := e.get(t, "/accounts/lookup?acct=%40alice%40social.example", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("lookup = %d, want 200", res.StatusCode)
	}
	var got clientapi.Account
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Id != aliceAccountID || got.FollowersCount == nil || *got.FollowersCount != 2 || res.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("lookup = %+v (Cache-Control %q)", got, res.Header.Get("Cache-Control"))
	}

	res = e.get(t, "/accounts/01890a5d-ac96-774b-bcce-b302099a80ff", map[string]string{"Accept-Language": "ja"})
	if res.StatusCode != http.StatusNotFound || res.Header.Get("Content-Language") != "ja" || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unknown account = %d (Content-Language %q)", res.StatusCode, res.Header.Get("Content-Language"))
	}
	if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || p.Title != "見つかりませんでした。削除されたか、リンクが間違っている可能性があります。" {
		t.Errorf("problem = %+v", p)
	}
}
