// SPDX-License-Identifier: AGPL-3.0-only

package meilisearch_test

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/meilisearch"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/meilisearchtest"
)

func TestMain(m *testing.M) { os.Exit(meilisearchtest.Main(m)) }

func newServerIndex(t *testing.T) *meilisearch.Index {
	t.Helper()
	s := meilisearchtest.New(t)
	x, err := meilisearch.New(meilisearch.Options{URL: s.URL, APIKey: s.Key, Index: s.Index, HTTPClient: newClient()})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// TestSearchServerContract runs the contract on a real Meilisearch (scripts/task.sh go:test-search).
// Every write waits for its task, so it shows in the next search without a wrapper.
func TestSearchServerContract(t *testing.T) {
	testkit.SearchIndexContract(t, func(t *testing.T) search.Index { return newServerIndex(t) })
}

// TestSearchServerJapanese checks what the operator documentation says of Japanese: the text of
// a Japanese document is segmented into words as Japanese, kanji only or not, and the
// compatibility forms are folded.
func TestSearchServerJapanese(t *testing.T) {
	x := newServerIndex(t)
	docs := []struct {
		lang, text string
	}{
		{"ja", "全文検索の本文です"},
		{"ja", "東京都庁舎見学"},
		{"ja-JP", "ｶﾀｶﾅのテキスト"},
		{"", "ＡＢＣ full width"},
	}
	for i, d := range docs {
		doc := search.Document{
			StatusID: statusID(i + 1), AuthorID: "0190a000-0000-7000-8000-00000000000a", Visibility: search.VisibilityPublic,
			Language: d.lang, PublishedAt: time.Unix(int64(i), 0), Text: d.text,
		}
		if err := x.Replace(t.Context(), doc); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"検索", []int{1}},
		{"全文検索", []int{1}},
		{"全文　検索", []int{1}},
		{"本文", []int{1}},
		{"東京", []int{2}},
		// 東京都 is 東京 and 都, not 東 and 京都.
		{"京都", nil},
		{"カタカナ", []int{3}},
		{"かたかな", []int{3}},
		{"abc", []int{4}},
	} {
		page, err := x.Search(t.Context(), search.Query{Text: tc.query, Limit: 10})
		if err != nil {
			t.Fatalf("Search(%q): %v", tc.query, err)
		}
		var want []string
		for _, n := range tc.want {
			want = append(want, statusID(n))
		}
		if !slices.Equal(page.StatusIDs, want) {
			t.Errorf("Search(%q) = %v, want %v", tc.query, page.StatusIDs, want)
		}
	}
}
