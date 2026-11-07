// SPDX-License-Identifier: AGPL-3.0-only

package contract_test

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/app/search"
)

func runSearch(t *testing.T, o fakeplugin.Options, clearIndex bool) map[string]contract.Result {
	t.Helper()
	k, _ := kind.ByName(kind.Search)
	h, _ := fakeplugin.Connect(o)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	out := map[string]contract.Result{}
	contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: k.Protocol, ClearIndex: clearIndex}, func(r contract.Result) { out[r.Case] = r })
	return out
}

var (
	searchCaseNames  = []string{"search_no_word", "search_plain_text", "search_japanese", "search_invalid_cursor", "replace_and_search", "replace_replaces", "remove", "remove_by_author"}
	searchClearNames = []string{"remove_by_visibility", "clear"}
)

func TestSearchCases(t *testing.T) {
	results := runSearch(t, fakeplugin.Options{Search: fake.New()}, true)
	for _, name := range append(append([]string{"reachable"}, searchCaseNames...), searchClearNames...) {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// Without --clear-index, the cases touch only documents of their own and leave none: a document of a real post stays,
// and nothing else is found.
func TestSearchCasesKeepARealIndex(t *testing.T) {
	index := fake.New()
	const real = "0190b000-0000-7000-8000-000000000001"
	post := search.Document{StatusID: real, AuthorID: "0190a000-0000-7000-8000-00000000000a", Visibility: search.VisibilityLocal, PublishedAt: time.Now(), Text: "my plugin test of a real post"}
	if err := index.Replace(t.Context(), post); err != nil {
		t.Fatal(err)
	}
	results := runSearch(t, fakeplugin.Options{Search: index}, false)
	for _, name := range searchCaseNames {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
	for _, name := range searchClearNames {
		if r := results[name]; r.Outcome != contract.Skip {
			t.Errorf("%s without --clear-index: %+v", name, r)
		}
	}
	page, err := index.Search(t.Context(), search.Query{Text: "my plugin test", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.StatusIDs) != 1 || page.StatusIDs[0] != real {
		t.Errorf("the index holds %v after the cases, want only %s", page.StatusIDs, real)
	}
}

// laxCursors is an index that takes any cursor as the start, as a plugin that forgot to check them does.
type laxCursors struct{ *fake.Index }

func (x laxCursors) Search(ctx context.Context, q search.Query) (search.Page, error) {
	q.Cursor = ""
	return x.Index.Search(ctx, q)
}

func TestSearchCasesFailAPluginThatBreaksTheContract(t *testing.T) {
	results := runSearch(t, fakeplugin.Options{Search: laxCursors{fake.New()}}, false)
	if r := results["search_invalid_cursor"]; r.Outcome != contract.Fail {
		t.Errorf("a plugin that takes any cursor: %+v", r)
	}

	results = runSearch(t, fakeplugin.Options{Search: fake.New(), Broken: true}, true)
	for _, name := range append(searchCaseNames, searchClearNames...) {
		if r := results[name]; r.Outcome != contract.Fail {
			t.Errorf("a plugin that answers nothing, %s: %+v", name, r)
		}
	}
}
