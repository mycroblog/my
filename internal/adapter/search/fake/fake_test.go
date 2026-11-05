// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestContract(t *testing.T) {
	testkit.SearchIndexContract(t, func(*testing.T) search.Index { return fake.New() })
}

// TestReplaceRefusesAVisibilityNeverIndexed makes a test fail when its code sends a status that
// the common rule of ADR-0149 §5 keeps out of every index.
func TestReplaceRefusesAVisibilityNeverIndexed(t *testing.T) {
	for _, v := range []search.Visibility{"unlisted", "private", "direct", ""} {
		x := fake.New()
		if err := x.Replace(t.Context(), search.Document{StatusID: "a", AuthorID: "b", Visibility: v, Text: "word"}); err == nil {
			t.Errorf("Replace with the visibility %q = nil, want an error", v)
		}
	}
}

// TestFindsJapanese is the fake's own choice, which the contract does not require: tests of what
// follows a search can look for Japanese text.
func TestFindsJapanese(t *testing.T) {
	x := fake.New()
	if err := x.Replace(t.Context(), search.Document{StatusID: "a", AuthorID: "b", Visibility: search.VisibilityPublic, Text: "全文検索の本文"}); err != nil {
		t.Fatal(err)
	}
	page, err := x.Search(t.Context(), search.Query{Text: "検索", Limit: 10})
	if err != nil || len(page.StatusIDs) != 1 {
		t.Errorf("Search = %+v, %v, want the document", page, err)
	}
}
