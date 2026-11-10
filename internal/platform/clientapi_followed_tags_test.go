// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestFollowedTagsAPI drives the followed hashtags on the real database: following a hashtag
// that no post used yet, reading it, listing the followed ones, the limit, and the home timeline
// with the posts of the followed hashtags, each once, marked with the tags that brought them.
func TestFollowedTagsAPI(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice := e.userWithScope(t, "alice", "read write")
	bob, carol := e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, alice, bob)

	hashtag := func(viewer *timelineUser, method, path string) (clientapi.Hashtag, int) {
		t.Helper()
		var res *http.Response
		if viewer == nil {
			res = e.spec.Serve(t, e.h, httptest.NewRequest(method, clientapi.BasePath+path, nil))
		} else {
			res = e.listCall(t, *viewer, method, path, "")
		}
		if res.StatusCode != http.StatusOK {
			return clientapi.Hashtag{}, res.StatusCode
		}
		return decodeJSON[clientapi.Hashtag](t, res), res.StatusCode
	}
	if h, code := hashtag(&alice, http.MethodPost, "/tags/Cats/follow"); code != http.StatusOK || h.Name != "Cats" || !h.Following {
		t.Fatalf("POST /tags/Cats/follow = %d %+v", code, h)
	}
	if h, code := hashtag(&alice, http.MethodPost, "/tags/%23dogs/follow"); code != http.StatusOK || h.Name != "dogs" || !h.Following {
		t.Fatalf("POST /tags/%%23dogs/follow = %d %+v", code, h)
	}
	if h, code := hashtag(&alice, http.MethodGet, "/tags/CATS"); code != http.StatusOK || h.Name != "Cats" || !h.Following || h.Url == "" {
		t.Errorf("GET /tags/CATS = %d %+v, want Cats followed", code, h)
	}
	if h, code := hashtag(&carol, http.MethodGet, "/tags/cats"); code != http.StatusOK || h.Following {
		t.Errorf("GET /tags/cats by carol = %d %+v, want unfollowed", code, h)
	}
	if h, code := hashtag(nil, http.MethodGet, "/tags/birds"); code != http.StatusOK || h.Name != "birds" || h.Following {
		t.Errorf("GET /tags/birds by a visitor = %d %+v, want birds unfollowed", code, h)
	}
	if _, code := hashtag(&alice, http.MethodPost, "/tags/not-a-tag/follow"); code != http.StatusUnprocessableEntity {
		t.Errorf("following an invalid hashtag = %d, want 422", code)
	}
	res := e.listCall(t, alice, http.MethodGet, "/followed_tags", "")
	if c := decodeJSON[clientapi.FollowedHashtagCollection](t, res); len(c.Items) != 2 || c.Items[0].Name != "dogs" || c.Items[1].Name != "Cats" {
		t.Errorf("GET /followed_tags = %+v, want dogs then Cats", c.Items)
	}

	// Home lists carol's posts with a followed hashtag, bob's once without the mark, and not an
	// unlisted one.
	tagged := e.post(t, carol, "about #cats and #dogs", domcontent.VisibilityPublic, "en")
	e.post(t, carol, "quietly #cats", domcontent.VisibilityUnlisted, "en")
	bobs := e.post(t, bob, "my #cats", domcontent.VisibilityPublic, "en")
	page := e.page(t, &alice, "/timelines/home")
	if got := itemIDs(page); !slices.Equal(got, []string{bobs, tagged}) {
		t.Fatalf("home = %v, want bob's and carol's tagged post", got)
	}
	for _, st := range page.Items {
		var want []string
		if st.Id == tagged {
			want = []string{"Cats", "dogs"}
		}
		if got := st.FollowedTags; (got == nil) != (want == nil) || (got != nil && !slices.Equal(*got, want)) {
			t.Errorf("followed_tags of %s = %v, want %v", st.Id, got, want)
		}
	}

	// Unfollowing takes carol's post off home.
	if h, code := hashtag(&alice, http.MethodDelete, "/tags/cats/follow"); code != http.StatusOK || h.Following {
		t.Errorf("DELETE /tags/cats/follow = %d %+v", code, h)
	}
	if h, code := hashtag(&alice, http.MethodDelete, "/tags/dogs/follow"); code != http.StatusOK || h.Following {
		t.Errorf("DELETE /tags/dogs/follow = %d %+v", code, h)
	}
	if got := itemIDs(e.page(t, &alice, "/timelines/home")); !slices.Equal(got, []string{bobs}) {
		t.Errorf("home after unfollowing = %v, want bob's", got)
	}

	// At most timeline.MaxFollowedTags.
	for i := range timeline.MaxFollowedTags {
		if _, code := hashtag(&alice, http.MethodPost, fmt.Sprintf("/tags/tag%d/follow", i)); code != http.StatusOK {
			t.Fatalf("following tag%d = %d", i, code)
		}
	}
	res = e.listCall(t, alice, http.MethodPost, "/tags/onemore/follow", "")
	if p := decodeJSON[clientapi.Problem](t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeTooManyFollowedTags {
		t.Errorf("following beyond the limit = %d %+v, want 422 too-many-followed-tags", res.StatusCode, p)
	}
}

// TestStreamingDeliversFollowedTags checks that the home stream sends a post of a followed
// hashtag by an account the user does not follow, with its followed_tags.
func TestStreamingDeliversFollowedTags(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, carol := e.userWithScope(t, "alice", "read write"), e.user(t, "carol")
	if res := e.listCall(t, alice, http.MethodPost, "/tags/cats/follow", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("POST /tags/cats/follow = %d", res.StatusCode)
	}

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	ctx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+alice.token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /streaming: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	events := readEvents(bufio.NewReader(res.Body))
	deadline := time.Now().Add(5 * time.Second)
	for e.app.Streams.Len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the stream did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Neither an untagged post nor one of another hashtag reaches alice.
	e.post(t, carol, "untagged", domcontent.VisibilityPublic, "en")
	e.post(t, carol, "a #cat", domcontent.VisibilityPublic, "en")
	tagged := e.post(t, carol, "about #cats", domcontent.VisibilityPublic, "en")
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("the stream ended")
		}
		if ev.name != "update" {
			t.Fatalf("event %q (%s), want update", ev.name, ev.data)
		}
		var st clientapi.Status
		if err := json.Unmarshal(ev.data, &st); err != nil {
			t.Fatal(err)
		}
		e.spec.ValidateSchema(t, "Status", ev.data)
		if st.Id != tagged || st.FollowedTags == nil || !slices.Equal(*st.FollowedTags, []string{"cats"}) {
			t.Errorf("update = %s with followed_tags %v, want carol's #cats post with cats", st.Id, st.FollowedTags)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5 seconds")
	}
}
