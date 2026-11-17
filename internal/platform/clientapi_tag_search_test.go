// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestTagSearchAPI drives the search of hashtags on the real database (FR-306): a hashtag that
// posts were written with is found by the start of its name, as first written, with the count
// of its posts.
func TestTagSearchAPI(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	reader, writer := e.user(t, "alice"), e.user(t, "carol")
	e.post(t, writer, "Crumb shot of the rye. #Sourdough", domcontent.VisibilityPublic, "en")
	e.post(t, writer, "The second loaf of the week. #sourdough", domcontent.VisibilityPublic, "en")

	res := e.listCall(t, reader, http.MethodGet, "/tags/search?q=sourd", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /tags/search?q=sourd = %d", res.StatusCode)
	}
	got := decodeJSON[clientapi.TagSuggestionList](t, res).Items
	if want := []clientapi.TagSuggestion{{Name: "Sourdough", StatusesCount: 2}}; !slices.Equal(got, want) {
		t.Errorf("GET /tags/search?q=sourd = %+v, want %+v", got, want)
	}
}
