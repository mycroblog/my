// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/search"
)

// The accounts of the documents of SearchIndexContract.
const (
	searchAuthorA = "0190a000-0000-7000-8000-00000000000a"
	searchAuthorB = "0190a000-0000-7000-8000-00000000000b"
)

// searchStatusID returns the ID of the nth status of SearchIndexContract, in the order of time
// like a UUIDv7.
func searchStatusID(n int) string {
	return fmt.Sprintf("0190b000-0000-7000-8000-%012d", n)
}

// searchDoc returns a public English document of searchAuthorA.
func searchDoc(n int, text string) search.Document {
	return search.Document{
		StatusID: searchStatusID(n), AuthorID: searchAuthorA, Visibility: search.VisibilityPublic, Language: "en",
		PublishedAt: time.Date(2026, 1, 1, 0, 0, n, 0, time.UTC), Text: text,
	}
}

// searchContract holds the index of one case of SearchIndexContract.
type searchContract struct {
	t *testing.T
	x search.Index
}

func (c searchContract) replace(docs ...search.Document) {
	c.t.Helper()
	for _, d := range docs {
		if err := c.x.Replace(c.t.Context(), d); err != nil {
			c.t.Fatalf("Replace(%s): %v", d.StatusID, err)
		}
	}
}

// find returns the IDs of every page of a search, checking that each page keeps to the limit and
// that the pages end.
func (c searchContract) find(text string, limit int) []string {
	c.t.Helper()
	var ids []string
	cursor := ""
	for range 100 {
		page, err := c.x.Search(c.t.Context(), search.Query{Text: text, Cursor: cursor, Limit: limit})
		if err != nil {
			c.t.Fatalf("Search(%q, cursor %q): %v", text, cursor, err)
		}
		if len(page.StatusIDs) > limit {
			c.t.Fatalf("Search(%q) returned %d IDs, over the limit %d", text, len(page.StatusIDs), limit)
		}
		ids = append(ids, page.StatusIDs...)
		if page.Next == "" {
			return ids
		}
		cursor = page.Next
	}
	c.t.Fatalf("Search(%q): the pages do not end", text)
	return nil
}

// want checks that a search returns exactly these statuses, in this order.
func (c searchContract) want(text string, statuses ...int) {
	c.t.Helper()
	var ids []string
	for _, n := range statuses {
		ids = append(ids, searchStatusID(n))
	}
	if got := c.find(text, 10); !slices.Equal(got, ids) {
		c.t.Errorf("Search(%q) = %v, want %v", text, got, ids)
	}
}

// SearchIndexContract runs the contract of search.Index (ADR-0149 §4) on the indexes newIndex
// returns, each one empty. It is for backends that really search; search = none has no backend.
//
// A write must show in the next search. A backend whose writes show later, after a refresh of the
// engine, gives the contract a wrapper whose writes wait for that.
//
// Whether a Japanese document can be found is not part of it: each backend decides and documents
// that (ADR-0149 §13). The contract requires that Japanese documents can be replaced and removed
// intact, which it sees through the Latin words beside the Japanese text, and that Japanese
// queries do not fail.
func SearchIndexContract(t *testing.T, newIndex func(t *testing.T) search.Index) {
	t.Helper()
	cases := []struct {
		name string
		run  func(c searchContract)
	}{
		{"indexing adds documents, and a search returns only their IDs, newest first", searchContractIndexing},
		{"every word of a query must match", searchContractWords},
		{"a query with no word finds nothing", searchContractNoWord},
		{"documents of either visibility and with no language are found", searchContractKinds},
		{"replacing a document replaces its text and keeps one document", searchContractReplace},
		{"removing a document leaves the others and can be repeated", searchContractRemove},
		{"removing by author leaves the other authors and can be repeated", searchContractRemoveByAuthor},
		{"removing by visibility leaves the other visibility and can be repeated", searchContractRemoveByVisibility},
		{"clearing removes everything, can be repeated, and the index takes documents again", searchContractClear},
		{"pages hold each ID once, newest first, up to the limit", searchContractPages},
		{"a cursor stays valid while the index changes", searchContractCursorStays},
		{"a cursor the backend did not issue is ErrInvalidCursor", searchContractBadCursor},
		{"a query is plain text, never the query language of the engine", searchContractPlainText},
		{"Japanese documents are replaced and removed intact", searchContractJapaneseDocuments},
		{"Japanese queries do not fail", searchContractJapaneseQueries},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(searchContract{t: t, x: newIndex(t)})
		})
	}
}

func searchContractIndexing(c searchContract) {
	c.replace(searchDoc(1, "alpha one"), searchDoc(3, "alpha three"), searchDoc(2, "alpha two"), searchDoc(4, "bravo four"))
	c.want("alpha", 3, 2, 1)
	c.want("bravo", 4)
	c.want("missing")
}

func searchContractWords(c searchContract) {
	c.replace(searchDoc(1, "alpha bravo"), searchDoc(2, "bravo alpha"), searchDoc(3, "alpha"), searchDoc(4, "bravo"))
	c.want("alpha bravo", 2, 1)
	c.want("bravo   alpha", 2, 1)
}

func searchContractNoWord(c searchContract) {
	c.replace(searchDoc(1, "alpha"))
	c.want("")
	c.want("  \t ")
}

func searchContractKinds(c searchContract) {
	local := searchDoc(1, "alpha")
	local.Visibility, local.AuthorID = search.VisibilityLocal, searchAuthorB
	noLanguage := searchDoc(2, "alpha")
	noLanguage.Language = ""
	c.replace(local, noLanguage)
	c.want("alpha", 2, 1)
}

func searchContractReplace(c searchContract) {
	c.replace(searchDoc(1, "alpha"))
	c.replace(searchDoc(1, "bravo"))
	c.want("alpha")
	c.want("bravo", 1)
	c.replace(searchDoc(1, "bravo"))
	c.want("bravo", 1)
}

func searchContractRemove(c searchContract) {
	c.replace(searchDoc(1, "alpha"), searchDoc(2, "alpha"))
	for range 2 {
		if err := c.x.Remove(c.t.Context(), searchStatusID(1)); err != nil {
			c.t.Fatalf("Remove: %v", err)
		}
		c.want("alpha", 2)
	}
}

func searchContractRemoveByAuthor(c searchContract) {
	other := searchDoc(3, "alpha")
	other.AuthorID = searchAuthorB
	c.replace(searchDoc(1, "alpha"), searchDoc(2, "alpha"), other)
	for range 2 {
		if err := c.x.RemoveByAuthor(c.t.Context(), searchAuthorA); err != nil {
			c.t.Fatalf("RemoveByAuthor: %v", err)
		}
		c.want("alpha", 3)
	}
}

func searchContractRemoveByVisibility(c searchContract) {
	local := searchDoc(2, "alpha")
	local.Visibility = search.VisibilityLocal
	c.replace(searchDoc(1, "alpha"), local, searchDoc(3, "alpha"))
	for range 2 {
		if err := c.x.RemoveByVisibility(c.t.Context(), search.VisibilityLocal); err != nil {
			c.t.Fatalf("RemoveByVisibility: %v", err)
		}
		c.want("alpha", 3, 1)
	}
}

func searchContractClear(c searchContract) {
	other := searchDoc(2, "alpha")
	other.AuthorID, other.Visibility = searchAuthorB, search.VisibilityLocal
	c.replace(searchDoc(1, "alpha"), other)
	for range 2 {
		if err := c.x.Clear(c.t.Context()); err != nil {
			c.t.Fatalf("Clear: %v", err)
		}
		c.want("alpha")
	}
	c.replace(searchDoc(3, "alpha"))
	c.want("alpha", 3)
}

func searchContractPages(c searchContract) {
	var ids []string
	for n := 7; n >= 1; n-- {
		c.replace(searchDoc(n, "alpha"))
		ids = append(ids, searchStatusID(n))
	}
	for _, limit := range []int{1, 2, 3, 7, 8} {
		if got := c.find("alpha", limit); !slices.Equal(got, ids) {
			c.t.Errorf("the pages with the limit %d = %v, want %v", limit, got, ids)
		}
	}
}

func searchContractCursorStays(c searchContract) {
	for n := 1; n <= 4; n++ {
		c.replace(searchDoc(n, "alpha"))
	}
	first, err := c.x.Search(c.t.Context(), search.Query{Text: "alpha", Limit: 2})
	if err != nil {
		c.t.Fatalf("Search: %v", err)
	}
	if first.Next == "" {
		c.t.Fatalf("the first page %+v has no cursor", first)
	}
	// A newer status does not move the older ones into the next page, and a status removed from
	// the next page is simply not there.
	c.replace(searchDoc(5, "alpha"))
	if err := c.x.Remove(c.t.Context(), searchStatusID(2)); err != nil {
		c.t.Fatalf("Remove: %v", err)
	}
	next, err := c.x.Search(c.t.Context(), search.Query{Text: "alpha", Cursor: first.Next, Limit: 2})
	if err != nil {
		c.t.Fatalf("Search of the next page: %v", err)
	}
	if want := []string{searchStatusID(1)}; !slices.Equal(next.StatusIDs, want) {
		c.t.Errorf("the next page = %v, want %v", next.StatusIDs, want)
	}
}

func searchContractBadCursor(c searchContract) {
	c.replace(searchDoc(1, "alpha"))
	for _, cursor := range []string{"not a cursor", "%%%", "\x00"} {
		if _, err := c.x.Search(c.t.Context(), search.Query{Text: "alpha", Cursor: cursor, Limit: 2}); !errors.Is(err, search.ErrInvalidCursor) {
			c.t.Errorf("Search with the cursor %q: %v, want ErrInvalidCursor", cursor, err)
		}
	}
}

func searchContractPlainText(c searchContract) {
	alpha, bravo := searchStatusID(1), searchStatusID(2)
	c.replace(searchDoc(1, "alpha"), searchDoc(2, "bravo"))
	for _, tc := range []struct {
		query   string
		mustNot []string
	}{
		// Words are joined by AND: no document has both, whatever comes between them.
		{"alpha OR bravo", []string{alpha, bravo}},
		{"alpha || bravo", []string{alpha, bravo}},
		{"alpha | bravo", []string{alpha, bravo}},
		{"bravo -alpha", []string{alpha, bravo}},
		{"bravo NOT alpha", []string{alpha, bravo}},
		{"bravo !alpha", []string{alpha, bravo}},
		// A negation alone does not turn into every other document.
		{"-alpha", []string{bravo}},
		{"NOT alpha", []string{bravo}},
		// A wildcard or a field of the document does not match by itself.
		{"*", []string{alpha, bravo}},
		{"?", []string{alpha, bravo}},
		{"*:*", []string{alpha, bravo}},
		{"visibility:public", []string{alpha, bravo}},
		{"status_id:*", []string{alpha, bravo}},
		// Syntax that an engine's query language would refuse does not fail.
		{`"alpha`, []string{bravo}},
		{"(alpha", []string{bravo}},
		{"alpha)", []string{bravo}},
		{"[alpha TO", []string{bravo}},
		{"{alpha", []string{bravo}},
		{"alpha~", []string{bravo}},
		{"alpha^2", []string{bravo}},
		{"/alpha", []string{bravo}},
		{`alpha\`, []string{bravo}},
		{":", []string{alpha, bravo}},
		{"+", []string{alpha, bravo}},
		{"&&", []string{alpha, bravo}},
	} {
		got := c.find(tc.query, 10)
		for _, id := range tc.mustNot {
			if slices.Contains(got, id) {
				c.t.Errorf("Search(%q) = %v, which reads it as the engine's query language", tc.query, got)
			}
		}
	}
}

func searchContractJapaneseDocuments(c searchContract) {
	ja := func(n int, text string) search.Document {
		d := searchDoc(n, text)
		d.Language = "ja"
		return d
	}
	c.replace(ja(1, "全文検索の本文です alpha"), ja(2, "東京都の天気は晴れ、𠮷野家へ 👍 alpha"))
	c.want("alpha", 2, 1)

	c.replace(ja(1, "置き換えた日本語の本文 bravo"))
	c.want("alpha", 2)
	c.want("bravo", 1)

	if err := c.x.Remove(c.t.Context(), searchStatusID(1)); err != nil {
		c.t.Fatalf("Remove: %v", err)
	}
	c.want("bravo")
	c.want("alpha", 2)

	if err := c.x.RemoveByAuthor(c.t.Context(), searchAuthorA); err != nil {
		c.t.Fatalf("RemoveByAuthor: %v", err)
	}
	c.want("alpha")
}

func searchContractJapaneseQueries(c searchContract) {
	ja := searchDoc(1, "全文検索の本文です")
	ja.Language = "ja"
	c.replace(ja, searchDoc(2, "alpha"))
	for _, query := range []string{"全文検索", "検索", "本", "全文 検索", "全文　検索", "ぜんぶん", "カタカナ", "𠮷", "👍"} {
		for _, id := range c.find(query, 10) {
			if id != searchStatusID(1) {
				c.t.Errorf("Search(%q) returned %s, whose text has nothing Japanese", query, id)
			}
		}
	}
}
