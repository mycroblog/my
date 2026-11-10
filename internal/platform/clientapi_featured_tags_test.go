// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestFeaturedTagsAPI drives the featured hashtags on the real database: choosing them, reading
// one's own and another account's with the counts of their public and unlisted posts, the
// limits, and the featured-tags collection of the actor that other servers read (ADR-0006 §17).
func TestFeaturedTagsAPI(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice := e.userWithScope(t, "alice", "read write")
	bob := e.user(t, "bob")

	set := func(body string) (clientapi.FeaturedHashtagCollection, int) {
		t.Helper()
		res := e.listCall(t, alice, http.MethodPut, "/featured_tags", body)
		if res.StatusCode != http.StatusOK {
			return clientapi.FeaturedHashtagCollection{}, res.StatusCode
		}
		return decodeJSON[clientapi.FeaturedHashtagCollection](t, res), res.StatusCode
	}
	names := func(c clientapi.FeaturedHashtagCollection) []string {
		out := []string{}
		for _, it := range c.Items {
			out = append(out, it.Name)
		}
		return out
	}
	if c, code := set(`{"names":["#Cats","dogs","cats"]}`); code != http.StatusOK || !slices.Equal(names(c), []string{"Cats", "dogs"}) {
		t.Fatalf("PUT /featured_tags = %d %+v, want Cats and dogs", code, c)
	}
	if _, code := set(`{"names":["not-a-tag"]}`); code != http.StatusUnprocessableEntity {
		t.Errorf("an invalid hashtag = %d, want 422", code)
	}
	res := e.listCall(t, alice, http.MethodPut, "/featured_tags", `{"names":["a","b","c","d","e","f","g","h","i","j","k"]}`)
	if p := decodeJSON[clientapi.Problem](t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeTooManyFeaturedTags {
		t.Errorf("11 hashtags = %d %+v, want 422 too-many-featured-tags", res.StatusCode, p)
	}
	if _, code := set(`{"names":["dogs","Cats"]}`); code != http.StatusOK {
		t.Fatalf("reordering = %d", code)
	}

	// The counts read alice's public and unlisted posts alone.
	e.post(t, alice, "about #cats", domcontent.VisibilityPublic, "en")
	e.post(t, alice, "quietly #cats", domcontent.VisibilityUnlisted, "en")
	e.post(t, alice, "for friends #cats #dogs", domcontent.VisibilityPrivate, "en")
	e.post(t, bob, "also #cats", domcontent.VisibilityPublic, "en")
	own := decodeJSON[clientapi.FeaturedHashtagCollection](t, e.listCall(t, alice, http.MethodGet, "/featured_tags", ""))
	if !slices.Equal(names(own), []string{"dogs", "Cats"}) || own.Items[0].StatusesCount != 0 || own.Items[0].LastStatusAt != nil ||
		own.Items[1].StatusesCount != 2 || own.Items[1].LastStatusAt == nil || own.Items[1].Url == "" {
		t.Errorf("GET /featured_tags = %+v, want dogs unused, then Cats with 2 posts", own.Items)
	}
	for _, viewer := range []*timelineUser{&bob, nil} {
		req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/accounts/"+alice.accountID+"/featured_tags", nil)
		if viewer != nil {
			req.Header.Set("Authorization", "Bearer "+viewer.token)
		}
		res := e.spec.Serve(t, e.h, req)
		if c := decodeJSON[clientapi.FeaturedHashtagCollection](t, res); res.StatusCode != http.StatusOK || !slices.Equal(names(c), []string{"dogs", "Cats"}) {
			t.Errorf("GET /accounts/alice/featured_tags (viewer %v) = %d %+v", viewer != nil, res.StatusCode, c)
		}
	}
	if res := e.listCall(t, bob, http.MethodGet, "/accounts/0192f3a4-0000-7000-8000-00000000ffff/featured_tags", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown account = %d, want 404", res.StatusCode)
	}

	// The actor's featured-tags collection lists them as Hashtags, in alice's order.
	req := httptest.NewRequest(http.MethodGet, "https://"+e.app.Config.Host+"/ap/actors/"+alice.accountID+"/featured-tags", nil)
	req.Header.Set("Accept", "application/activity+json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	var doc struct {
		Type       string `json:"type"`
		TotalItems int    `json:"totalItems"`
		Items      []struct{ Type, Href, Name string }
	}
	if err := json.Unmarshal(body, &doc); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET featured-tags = %d %s", rec.Code, body)
	}
	if doc.Type != "Collection" || doc.TotalItems != 2 || len(doc.Items) != 2 || doc.Items[0].Name != "#dogs" || doc.Items[1].Name != "#Cats" ||
		doc.Items[1].Href != "https://"+e.app.Config.Host+"/@alice/tagged/cats" || doc.Items[0].Type != "Hashtag" {
		t.Errorf("featured-tags = %s, want #dogs and #Cats", body)
	}
}
