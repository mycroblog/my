// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// replyTo posts a public reply of author to the status to through the content service.
func (e *timelineAPI) replyTo(t *testing.T, author timelineUser, to, text string) string {
	t.Helper()
	p, err := e.app.Content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author.accountID, TextInput: appcontent.TextInput{Text: text, Language: "en"},
		Visibility: domcontent.VisibilityPublic, InReplyToID: to,
	})
	if err != nil {
		t.Fatalf("Create(%q): %v", text, err)
	}
	return p.Status.ID
}

// homeUpdates reads the events of the home stream until the update of last and returns the
// statuses of its update and list_update events, in order; other events are skipped.
func homeUpdates(t *testing.T, e *timelineAPI, events <-chan sseEvent, last string) (updates, listUpdates []string) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("the home stream ended before the update of %s", last)
			}
			switch ev.name {
			case "update":
				e.spec.ValidateSchema(t, "Status", ev.data)
				var st clientapi.Status
				if err := json.Unmarshal(ev.data, &st); err != nil {
					t.Fatal(err)
				}
				updates = append(updates, st.Id)
			case "list_update":
				var lu struct {
					Status clientapi.Status `json:"status"`
				}
				if err := json.Unmarshal(ev.data, &lu); err != nil {
					t.Fatal(err)
				}
				listUpdates = append(listUpdates, lu.Status.Id)
				if lu.Status.Id == last {
					return updates, listUpdates
				}
			}
		case <-timeout:
			t.Fatalf("no list_update of %s within 10 seconds; updates %v, list updates %v", last, updates, listUpdates)
		}
	}
}

// TestStreamsDeliverTheRepliesTheTimelinesList posts replies on the real database while the home
// stream and the streams of the local and federated timelines are open, and checks that each
// delivers exactly the replies that its REST timeline lists (ADR-0013 §31): home and a list the
// replies to the viewer, to the author itself, and to a followed account, and the hashtag branch
// of home a reply with a followed hashtag; local and federated the replies to their own author.
func TestStreamsDeliverTheRepliesTheTimelinesList(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	list, err := e.app.Timeline.CreateList(ctx, alice.accountID, "Bob")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := e.app.Timeline.AddListMember(ctx, alice.accountID, list.ID, bob.accountID); err != nil {
		t.Fatalf("AddListMember: %v", err)
	}
	if _, err := e.app.Timeline.FollowTag(ctx, alice.accountID, "dogs"); err != nil {
		t.Fatalf("FollowTag: %v", err)
	}
	bobs := e.post(t, bob, "bob's post", domcontent.VisibilityPublic, "en")
	alices := e.post(t, alice, "alice's post", domcontent.VisibilityPublic, "en")
	carols := e.post(t, carol, "carol's post", domcontent.VisibilityPublic, "en")
	daves := e.post(t, dave, "dave's post", domcontent.VisibilityPublic, "en")
	before := []string{bobs, alices, carols, daves}

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	home := openTimelineStream(streamCtx, t, e, srv, alice, "/streaming")
	streams := map[string]<-chan sseEvent{
		"/timelines/local":     openTimelineStream(streamCtx, t, e, srv, alice, "/streaming/timelines/local"),
		"/timelines/federated": openTimelineStream(streamCtx, t, e, srv, alice, "/streaming/timelines/federated"),
	}
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.app.Streams.Len() == 3 }) {
		t.Fatalf("%d of 3 streams subscribed", e.app.Streams.Len())
	}

	bobSelf := e.replyTo(t, bob, bobs, "a thread")
	bobToAlice := e.replyTo(t, bob, alices, "to alice")
	bobToCarol := e.replyTo(t, bob, carols, "to carol")
	e.replyTo(t, bob, daves, "to dave")
	bobTagged := e.replyTo(t, bob, daves, "to dave about #dogs")
	daveSelf := e.replyTo(t, dave, daves, "dave's thread")
	e.replyTo(t, dave, bobs, "dave to bob")
	last := e.post(t, bob, "last", domcontent.VisibilityPublic, "en")

	rest := func(path string) []string {
		t.Helper()
		var out []string
		for _, id := range reversed(itemIDs(e.page(t, &alice, path))) {
			if !slices.Contains(before, id) {
				out = append(out, id)
			}
		}
		return out
	}
	updates, listUpdates := homeUpdates(t, e, home, last)
	for _, c := range []struct {
		path string
		got  []string
		want []string
	}{
		{"/timelines/home", updates, []string{bobSelf, bobToAlice, bobToCarol, bobTagged, last}},
		{"/timelines/list/" + list.ID, listUpdates, []string{bobSelf, bobToAlice, bobToCarol, last}},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("the stream of %s sent %v, want %v", c.path, c.got, c.want)
		}
		if listed := rest(c.path); !slices.Equal(listed, c.want) {
			t.Errorf("%s lists %v, want %v", c.path, listed, c.want)
		}
	}
	for path, events := range streams {
		want := []string{bobSelf, daveSelf, last}
		var got []string
		for range want {
			got = append(got, nextUpdate(t, e, path, events, 5*time.Second))
		}
		if !slices.Equal(got, want) {
			t.Errorf("the stream of %s sent %v, want %v", path, got, want)
		}
		if listed := rest(path); !slices.Equal(listed, want) {
			t.Errorf("%s lists %v, want %v", path, listed, want)
		}
	}
}
