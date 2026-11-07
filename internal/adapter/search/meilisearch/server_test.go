// SPDX-License-Identifier: AGPL-3.0-only

package meilisearch_test

import (
	"encoding/json"
	"net/http"
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
	return newServerIndexWith(t, meilisearchtest.New(t), nil)
}

func newServerIndexWith(t *testing.T, s meilisearchtest.Server, settings []byte) *meilisearch.Index {
	t.Helper()
	x, err := meilisearch.New(meilisearch.Options{URL: s.URL, APIKey: s.Key, Index: s.Index, Settings: settings, HTTPClient: newClient()})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// tunedSettings is the example settings file of the operator documentation
// (docs/operations/search-engines.md#tuning-meilisearch).
const tunedSettings = `{
  "localizedAttributes": [
    {"attributePatterns": ["text_ja"], "locales": ["jpn"]}
  ],
  "separatorTokens": ["・"],
  "proximityPrecision": "byAttribute",
  "typoTolerance": {"enabled": true, "minWordSizeForTypos": {"oneTypo": 8, "twoTypos": 12}},
  "synonyms": {"ご飯": ["ごはん"], "ごはん": ["ご飯"]}
}`

// TestSearchServerContractWithSettingsFile runs the contract on an index tuned by a settings file.
func TestSearchServerContractWithSettingsFile(t *testing.T) {
	testkit.SearchIndexContract(t, func(t *testing.T) search.Index {
		return newServerIndexWith(t, meilisearchtest.New(t), []byte(tunedSettings))
	})
}

// TestSearchServerSettingsAreSentOnce checks that a process whose settings the index already has
// sends none, and that one with other settings does.
func TestSearchServerSettingsAreSentOnce(t *testing.T) {
	s := meilisearchtest.New(t)
	updates := func() int {
		t.Helper()
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.URL+"/tasks?types=settingsUpdate&indexUids="+s.Index, nil)
		req.Header.Set("Authorization", "Bearer "+s.Key)
		res, err := newClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var body struct {
			Total int `json:"total"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Total
	}
	for i, settings := range []string{tunedSettings, tunedSettings, `{"stopWords": ["the"]}`} {
		if err := newServerIndexWith(t, s, []byte(settings)).Replace(t.Context(), doc(1, "", "alpha")); err != nil {
			t.Fatal(err)
		}
		if got, want := updates(), []int{1, 1, 2}[i]; got != want {
			t.Errorf("after process %d, %d settings updates, want %d", i+1, got, want)
		}
	}
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
