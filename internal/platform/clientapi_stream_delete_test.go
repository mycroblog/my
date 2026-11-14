// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// nextDeleted returns the status ID of the next event of events, which must be a delete within
// five seconds.
func nextDeleted(t *testing.T, name string, events <-chan sseEvent) string {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatalf("%s ended before a delete", name)
		}
		if ev.name != "delete" {
			t.Fatalf("%s: event %q (%s), want delete", name, ev.name, ev.data)
		}
		var data struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(ev.data, &data); err != nil {
			t.Fatal(err)
		}
		return data.ID
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: no delete within 5 seconds", name)
		return ""
	}
}

// TestStreamsSendTheDeletions opens user and timeline streams on the real database and checks
// that a deleted status reaches, as a delete event, the users who may see it and no one else: a
// private post its author's followers, a public post every user but those whom a block keeps from
// it, and a timeline stream only the statuses its timeline lists.
func TestStreamsSendTheDeletions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	e.follow(t, alice, bob)
	if _, err := e.app.Graph.Block(ctx, bob.accountID, dave.accountID); err != nil {
		t.Fatalf("Block: %v", err)
	}
	private := e.post(t, bob, "for my followers", domcontent.VisibilityPrivate, "en")
	unlisted := e.post(t, bob, "quietly", domcontent.VisibilityUnlisted, "en")
	public := e.post(t, bob, "for everyone", domcontent.VisibilityPublic, "en")
	davesPost := e.post(t, dave, "mine", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	streams := map[string]<-chan sseEvent{}
	for name, viewer := range map[string]timelineUser{"alice": alice, "bob": bob, "carol": carol, "dave": dave} {
		streams[name] = openTimelineStream(streamCtx, t, e, srv, viewer, "/streaming")
	}
	carolsLocal := openTimelineStream(streamCtx, t, e, srv, carol, "/streaming/timelines/local")
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.app.Streams.Len() >= 5 }) {
		t.Fatal("the streams did not subscribe")
	}

	for _, id := range []string{private, unlisted, public} {
		if _, err := e.app.Content.Delete(ctx, bob.accountID, id); err != nil {
			t.Fatalf("Delete(%s): %v", id, err)
		}
	}
	if _, err := e.app.Content.Delete(ctx, dave.accountID, davesPost); err != nil {
		t.Fatalf("Delete(%s): %v", davesPost, err)
	}

	// Each stream receives the deletions in order; the first one it does not receive would come
	// before the next one it does.
	for name, want := range map[string][]string{
		"alice": {private, unlisted, public, davesPost},
		"bob":   {private, unlisted, public},
		"carol": {unlisted, public, davesPost},
		"dave":  {davesPost},
	} {
		for _, id := range want {
			if got := nextDeleted(t, name, streams[name]); got != id {
				t.Errorf("%s: delete %s, want %s", name, got, id)
			}
		}
	}
	// The local timeline lists neither the private nor the unlisted post.
	for _, id := range []string{public, davesPost} {
		if got := nextDeleted(t, "carol's local timeline", carolsLocal); got != id {
			t.Errorf("carol's local timeline: delete %s, want %s", got, id)
		}
	}
}
