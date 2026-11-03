// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeTagSearch records the searches and returns tags.
type fakeTagSearch struct {
	tags  []apptimeline.TagSuggestion
	calls []string
}

func (f *fakeTagSearch) Search(_ context.Context, q string, limit int) ([]apptimeline.TagSuggestion, error) {
	if _, err := apptimeline.NormalizeTagSearchQuery(q); err != nil {
		return nil, err
	}
	f.calls = append(f.calls, fmt.Sprintf("%s %d", q, limit))
	return f.tags, nil
}

// TestSearchTags checks the parameters, the token and scope requirements, and the presentation
// of the prefix search of hashtags; the contract tests of the database adapters check the search.
func TestSearchTags(t *testing.T) {
	search := &fakeTagSearch{tags: []apptimeline.TagSuggestion{
		{ID: "01890a5d-ac96-774b-bcce-b302099a8001", Name: "Cat", StatusesCount: 1},
		{ID: "01890a5d-ac96-774b-bcce-b302099a8002", Name: "cats", StatusesCount: 3},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.TagSearch = search })
	token := map[string]string{"Authorization": "Bearer read"}

	if res := e.get(t, "/tags/search?q=ca", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous search = %d, want 401", res.StatusCode)
	}
	if res := e.get(t, "/tags/search?q=ca", map[string]string{"Authorization": "Bearer read-accounts"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("search without read:statuses = %d, want 403", res.StatusCode)
	}
	for _, q := range []string{"q=", "q=%23", "q=" + strings.Repeat("a", 101), "q=ca&limit=0", "q=ca&limit=21"} {
		if res := e.get(t, "/tags/search?"+q, token); res.StatusCode != http.StatusBadRequest {
			t.Errorf("search %s = %d, want 400", q, res.StatusCode)
		}
	}
	if res := e.get(t, "/tags/search?q=%23"+strings.Repeat("a", 100), token); res.StatusCode != http.StatusOK {
		t.Errorf("search of 100 characters after # = %d, want 200", res.StatusCode)
	}

	search.calls = nil
	res := e.get(t, "/tags/search?q="+url.QueryEscape("#Ca"), token)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "private, max-age=30" {
		t.Fatalf("search = %d (Cache-Control %q)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	var list clientapi.TagSuggestionList
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	got := list.Items
	want := []clientapi.TagSuggestion{{Name: "Cat", StatusesCount: 1}, {Name: "cats", StatusesCount: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("search = %+v, want %+v", got, want)
	}
	if res := e.get(t, "/tags/search?q=ca&limit=20", token); res.StatusCode != http.StatusOK {
		t.Errorf("search with limit 20 = %d", res.StatusCode)
	}
	if calls := []string{"#Ca 8", "ca 20"}; !reflect.DeepEqual(search.calls, calls) {
		t.Errorf("searches = %q, want %q", search.calls, calls)
	}

	// Without a search, nothing is found.
	e = newAuthEnv(t, nil)
	res = e.get(t, "/tags/search?q=ca", token)
	var none clientapi.TagSuggestionList
	if err := json.NewDecoder(res.Body).Decode(&none); err != nil || res.StatusCode != http.StatusOK || none.Items == nil || len(none.Items) != 0 {
		t.Errorf("search without a TagSearch = %d %v %v, want 200 []", res.StatusCode, none, err)
	}
}
