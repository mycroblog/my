// SPDX-License-Identifier: MIT

package integration_test

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/platform/platformtest"

	"github.com/mycroblog/my/pkg/mykit"
)

// TestFeaturedTags chooses the hashtags of a profile with mykit and reads them back as the owner
// and as another user, with the count of the owner's public posts of each.
func TestFeaturedTags(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})

	names := func(c *mykit.FeaturedHashtagCollection) []string {
		var out []string
		for _, it := range c.Items {
			out = append(out, it.Name)
		}
		return out
	}
	set, err := aliceAPI.SetFeaturedTagsWithResponse(t.Context(), mykit.SetFeaturedTagsRequest{Names: []string{"#Cats", "dogs"}})
	if err != nil {
		t.Fatal(err)
	}
	if set.JSON200 == nil || !slices.Equal(names(set.JSON200), []string{"Cats", "dogs"}) {
		t.Fatalf("setFeaturedTags = %d %s, want Cats and dogs", set.StatusCode(), set.Body)
	}
	// A problem is an error of type *mykit.Problem.
	_, err = aliceAPI.SetFeaturedTagsWithResponse(t.Context(), mykit.SetFeaturedTagsRequest{
		Names: []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11"},
	})
	var p *mykit.Problem
	if !errors.As(err, &p) || p.Status != http.StatusUnprocessableEntity || p.Type != "/api/v1/problems/too-many-featured-tags" {
		t.Errorf("11 hashtags = %v, want the problem 422 too-many-featured-tags", err)
	}

	post(t, aliceAPI, "about #cats")
	got, err := bobAPI.ListAccountFeaturedTagsWithResponse(t.Context(), alice.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if got.JSON200 == nil || !slices.Equal(names(got.JSON200), []string{"Cats", "dogs"}) || got.JSON200.Items[0].StatusesCount != 1 {
		t.Errorf("listAccountFeaturedTags = %d %s, want Cats with one post, then dogs", got.StatusCode(), got.Body)
	}
	own, err := aliceAPI.ListFeaturedTagsWithResponse(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if own.JSON200 == nil || !slices.Equal(names(own.JSON200), []string{"Cats", "dogs"}) {
		t.Errorf("listFeaturedTags = %d %s", own.StatusCode(), own.Body)
	}
}
