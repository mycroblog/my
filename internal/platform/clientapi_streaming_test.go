// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// sseEvent is one event of a Server-Sent Events stream.
type sseEvent struct {
	name string
	data []byte
}

// readEvents parses the events of an SSE stream into a channel, skipping comments, until the
// stream ends.
func readEvents(r *bufio.Reader) <-chan sseEvent {
	ch := make(chan sseEvent, 16)
	go func() {
		defer close(ch)
		var e sseEvent
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSuffix(line, "\n")
			switch {
			case line == "":
				if e.name != "" {
					ch <- e
				}
				e = sseEvent{}
			case strings.HasPrefix(line, "event: "):
				e.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				e.data = append(e.data, strings.TrimPrefix(line, "data: ")...)
			}
		}
	}()
	return ch
}

// TestStreamingDeliversHomeEntriesAndNotifications opens the streaming endpoint on the real
// database and checks that the posts and boosts of followed accounts and the new notifications
// arrive as the home timeline and the notification list show them, that the posts of other
// accounts do not, and that disconnecting releases the subscription.
func TestStreamingDeliversHomeEntriesAndNotifications(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, alice, bob)
	carolsPost := e.post(t, carol, "before the stream", domcontent.VisibilityPublic, "en")

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
	e.spec.ValidateResponse(t, req, res.StatusCode, res.Header, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /streaming = %d", res.StatusCode)
	}
	events := readEvents(bufio.NewReader(res.Body))
	next := func(name, schema string, v any) {
		t.Helper()
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("the stream ended before a %s event", name)
			}
			if ev.name != name {
				t.Fatalf("event %q (%s), want %q", ev.name, ev.data, name)
			}
			e.spec.ValidateSchema(t, schema, ev.data)
			if err := json.Unmarshal(ev.data, v); err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no %s event within 5 seconds", name)
		}
	}
	// Wait for the subscription, so that no signal is published before it.
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.app.Streams.Len() != 0 }) {
		t.Fatal("the stream did not subscribe")
	}

	// A post of an account alice does not follow is not in her home timeline; the next update
	// is bob's post.
	e.post(t, carol, "not followed", domcontent.VisibilityPublic, "en")
	bobsPost := e.post(t, bob, "hello alice", domcontent.VisibilityPublic, "en")
	var st clientapi.Status
	next("update", "Status", &st)
	if st.Id != bobsPost || st.Account.Id != bob.accountID {
		t.Errorf("update = %s by %s, want bob's post %s", st.Id, st.Account.Id, bobsPost)
	}

	// A cross-process broker may deliver a signal twice: the stream sends the entry once, so the
	// next update is bob's boost of carol's post, which reaches alice as the boost.
	e.app.Streams.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: bobsPost})
	boost := e.boost(t, bob, carolsPost)
	st = clientapi.Status{}
	next("update", "Status", &st)
	if st.Id != boost || st.Boost == nil || st.Boost.Id != carolsPost {
		t.Errorf("update = %+v, want bob's boost %s of %s", st, boost, carolsPost)
	}

	// carol follows alice: a follow notification.
	e.follow(t, carol, alice)
	var n clientapi.Notification
	next("notification", "Notification", &n)
	if n.Type != "follow" || n.Account == nil || n.Account.Id != carol.accountID {
		t.Errorf("notification = %+v, want carol's follow", n)
	}

	disconnect()
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.app.Streams.Len() == 0 }) {
		t.Fatalf("%d subscriptions still open after the client disconnected", e.app.Streams.Len())
	}
}
