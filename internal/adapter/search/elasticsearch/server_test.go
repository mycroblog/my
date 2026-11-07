// SPDX-License-Identifier: AGPL-3.0-only

package elasticsearch

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/elasticsearchtest"
)

// refreshing is the Index of the contract: each write is followed by a refresh, since the contract
// wants a write in the next search and Elasticsearch shows it after the next refresh (every second
// by default).
type refreshing struct{ *Index }

func (r refreshing) after(ctx context.Context, err error) error {
	if err != nil {
		return err
	}
	return r.refresh(ctx)
}

func (r refreshing) Replace(ctx context.Context, doc search.Document) error {
	return r.after(ctx, r.Index.Replace(ctx, doc))
}

func (r refreshing) Remove(ctx context.Context, statusID string) error {
	return r.after(ctx, r.Index.Remove(ctx, statusID))
}

func (r refreshing) Clear(ctx context.Context) error {
	return r.after(ctx, r.Index.Clear(ctx))
}

var serverIndexes atomic.Int64

// openServerIndex opens an index of its own on the node at url.
func openServerIndex(t *testing.T, url string) *Index {
	t.Helper()
	x, err := Open(t.Context(), Options{
		URL: url, Username: elasticsearchtest.Username, Password: elasticsearchtest.Password,
		Index: fmt.Sprintf("test-%d", serverIndexes.Add(1)), Replicas: 0,
		HTTPClient: &http.Client{Timeout: time.Minute},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return x
}

// TestSearchServerElasticsearch runs the contract, and the creation and upgrade of the index, on
// an Elasticsearch node (scripts/task.sh go:test-search).
func TestSearchServerElasticsearch(t *testing.T) {
	url := elasticsearchtest.New(t)
	t.Run("contract", func(t *testing.T) {
		testkit.SearchIndexContract(t, func(t *testing.T) search.Index {
			return refreshing{openServerIndex(t, url)}
		})
	})
	t.Run("Japanese is found where the characters of a word stand together", func(t *testing.T) {
		x := refreshing{openServerIndex(t, url)}
		docs := []string{"全文検索の本文です", "東京都の天気は晴れ", "ｶﾀｶﾅとＡＢＣ", "猫がかわいい"}
		for i, text := range docs {
			id := fmt.Sprintf("0190b000-0000-7000-8000-%012d", i+1)
			if err := x.Replace(t.Context(), search.Document{StatusID: id, AuthorID: "a", Visibility: search.VisibilityPublic, Language: "ja", Text: text}); err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			query string
			want  []int
		}{
			{"全文検索", []int{1}}, {"検索", []int{1}}, {"本", []int{1}}, {"本文", []int{1}}, {"文検", []int{1}},
			{"全検", nil}, {"東京", []int{2}}, {"晴れ", []int{2}}, {"天気 東京", []int{2}}, {"の", []int{2, 1}},
			{"カタカナ", []int{3}}, {"カタ", nil}, {"ぜんぶん", nil}, {"abc", []int{3}}, {"猫", []int{4}}, {"かわいい", []int{4}}, {"かわ", []int{4}},
		} {
			page, err := x.Search(t.Context(), search.Query{Text: tc.query, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			for _, n := range tc.want {
				want = append(want, fmt.Sprintf("0190b000-0000-7000-8000-%012d", n))
			}
			slices.Sort(want)
			slices.Reverse(want)
			if !slices.Equal(page.StatusIDs, want) {
				t.Errorf("Search(%q) = %v, want %v", tc.query, page.StatusIDs, want)
			}
		}
	})
	t.Run("an older mapping is upgraded in place or onto a new index", func(t *testing.T) {
		x := openServerIndex(t, url)
		ctx := t.Context()
		if err := x.Replace(ctx, search.Document{StatusID: "s1", AuthorID: "a", Visibility: search.VisibilityPublic, Text: "alpha"}); err != nil {
			t.Fatal(err)
		}
		before, err := x.aliasIndex(ctx)
		if err != nil {
			t.Fatal(err)
		}

		inPlace := currentMapping
		inPlace.Version, inPlace.InPlaceSince = currentMapping.Version+1, currentMapping.Version
		inPlace.Properties = map[string]any{"extra": map[string]any{"type": "keyword"}}
		for k, v := range currentMapping.Properties {
			inPlace.Properties[k] = v
		}
		if err := x.ensure(ctx, inPlace); err != nil {
			t.Fatalf("ensure in place: %v", err)
		}
		if now, _ := x.aliasIndex(ctx); now != before {
			t.Errorf("an upgrade in place moved the alias from %s to %s", before, now)
		}
		if v, _ := x.mappingVersion(ctx, before); v != inPlace.Version {
			t.Errorf("the mapping version is %d after an upgrade in place, want %d", v, inPlace.Version)
		}

		rebuild := inPlace
		rebuild.Version, rebuild.InPlaceSince = inPlace.Version+1, inPlace.Version+1
		if err := x.ensure(ctx, rebuild); err != nil {
			t.Fatalf("ensure onto a new index: %v", err)
		}
		after, _ := x.aliasIndex(ctx)
		if after != x.indexName(rebuild) {
			t.Errorf("the alias points at %s after an upgrade onto a new index, want %s", after, x.indexName(rebuild))
		}
		if err := x.refresh(ctx); err != nil {
			t.Fatal(err)
		}
		if page, err := x.Search(ctx, search.Query{Text: "alpha", Limit: 10}); err != nil || len(page.StatusIDs) != 0 {
			t.Errorf("the new index holds %v (%v), want nothing until a rebuild", page.StatusIDs, err)
		}
		if err := x.ensure(ctx, rebuild); err != nil {
			t.Errorf("ensure of the same version again: %v", err)
		}
		if err := x.ensure(ctx, currentMapping); err == nil {
			t.Error("ensure of an older version than the index's succeeded, want an error")
		}
	})
}
