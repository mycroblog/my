// SPDX-License-Identifier: MIT

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/platform/platformtest"

	"github.com/mycroblog/my/pkg/mykit"
)

// favourites returns the PageFunc of the favourites, limit statuses a page.
func favourites(api *mykit.API, limit int) mykit.PageFunc[mykit.Status] {
	return func(ctx context.Context, cursor *mykit.Cursor) ([]mykit.Status, *mykit.Cursor, error) {
		resp, err := api.ListFavouritesWithResponse(ctx, &mykit.ListFavouritesParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return nil, nil, err
		}
		if resp.JSON200 == nil {
			return nil, nil, fmt.Errorf("favourites = %d %s, want 200 with a page", resp.StatusCode(), resp.Body)
		}
		return resp.JSON200.Items, resp.JSON200.NextCursor, nil
	}
}

func favouriteIDs(t *testing.T, api *mykit.API) []string {
	t.Helper()
	var ids []string
	for s, err := range mykit.All(t.Context(), favourites(api, 2)) {
		if err != nil {
			t.Fatalf("page the favourites: %v", err)
		}
		ids = append(ids, s.Id)
	}
	return ids
}

// TestFavourites favourites and unfavourites posts with mykit and pages the favourites, newest
// favourite first.
func TestFavourites(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})

	var posts []string
	for i := range 3 {
		posts = append(posts, post(t, bobAPI, fmt.Sprintf("post %d", i)).Id)
	}
	for _, i := range []int{0, 2, 1} {
		resp, err := aliceAPI.FavouriteStatusWithResponse(t.Context(), posts[i])
		if err != nil {
			t.Fatalf("favourite post %d: %v", i, err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Fatalf("favourite post %d = %d %s, want 200", i, resp.StatusCode(), resp.Body)
		}
	}
	if got, want := favouriteIDs(t, aliceAPI), []string{posts[1], posts[2], posts[0]}; !slices.Equal(got, want) {
		t.Errorf("favourites = %v, want newest favourite first %v", got, want)
	}

	resp, err := aliceAPI.UnfavouriteStatusWithResponse(t.Context(), posts[2])
	if err != nil {
		t.Fatalf("unfavourite: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("unfavourite = %d %s, want 200", resp.StatusCode(), resp.Body)
	}
	if got, want := favouriteIDs(t, aliceAPI), []string{posts[1], posts[0]}; !slices.Equal(got, want) {
		t.Errorf("favourites after unfavouriting = %v, want %v", got, want)
	}
}
