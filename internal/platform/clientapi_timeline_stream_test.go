// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// openTimelineStream opens the stream of path on srv as viewer and returns its events;
// canceling ctx disconnects.
func openTimelineStream(ctx context.Context, t *testing.T, e *timelineAPI, srv *httptest.Server, viewer timelineUser, path string) <-chan sseEvent {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+viewer.token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	e.spec.ValidateResponse(t, req, res.StatusCode, res.Header, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", path, res.StatusCode)
	}
	return readEvents(bufio.NewReader(res.Body))
}

// nextUpdate returns the ID of the Status of the next event of events, which must be an update
// within timeout.
func nextUpdate(t *testing.T, e *timelineAPI, name string, events <-chan sseEvent, timeout time.Duration) string {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatalf("%s ended before an update", name)
		}
		if ev.name != "update" {
			t.Fatalf("%s: event %q (%s), want update", name, ev.name, ev.data)
		}
		e.spec.ValidateSchema(t, "Status", ev.data)
		var st clientapi.Status
		if err := json.Unmarshal(ev.data, &st); err != nil {
			t.Fatal(err)
		}
		return st.Id
	case <-time.After(timeout):
		t.Fatalf("%s: no update within %s", name, timeout)
		return ""
	}
}

// TestTimelineStreamsDeliverWhatTheTimelinesList opens the streams of the local and federated
// timelines on the real database and checks that each delivers exactly the new posts that the
// REST timeline lists for its viewer, in order: local posts on the local timeline only, no
// unlisted posts and no boosts, and the viewer's mutes, blocks, and timeline languages applied.
func TestTimelineStreamsDeliverWhatTheTimelinesList(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, erin := e.user(t, "alice"), e.user(t, "erin")
	bob, carol, dave := e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	if _, err := e.app.Graph.Mute(ctx, alice.accountID, carol.accountID, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	if _, err := e.app.Graph.Block(ctx, dave.accountID, alice.accountID); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if _, err := e.app.Identity.SetTimelineLanguages(ctx, erin.userID, []string{"en"}); err != nil {
		t.Fatalf("SetTimelineLanguages: %v", err)
	}
	before := e.post(t, bob, "before the streams", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	streams := map[string]<-chan sseEvent{
		"alice's local":     openTimelineStream(streamCtx, t, e, srv, alice, "/streaming/timelines/local"),
		"alice's federated": openTimelineStream(streamCtx, t, e, srv, alice, "/streaming/timelines/federated"),
		"erin's local":      openTimelineStream(streamCtx, t, e, srv, erin, "/streaming/timelines/local"),
	}
	// Wait for the subscriptions, so that no signal is published before them.
	deadline := time.Now().Add(5 * time.Second)
	for e.app.Streams.Len() != len(streams) {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d streams subscribed", e.app.Streams.Len(), len(streams))
		}
		time.Sleep(5 * time.Millisecond)
	}

	muted := e.post(t, carol, "muted by alice", domcontent.VisibilityPublic, "en")
	blocker := e.post(t, dave, "blocks alice", domcontent.VisibilityPublic, "en")
	e.post(t, bob, "unlisted", domcontent.VisibilityUnlisted, "en")
	local := e.post(t, bob, "local", domcontent.VisibilityLocal, "en")
	ja := e.post(t, bob, "日本語", domcontent.VisibilityPublic, "ja")
	e.boost(t, bob, before)
	last := e.post(t, bob, "last", domcontent.VisibilityPublic, "en")

	for name, tc := range map[string]struct {
		viewer timelineUser
		path   string
		want   []string
	}{
		"alice's local":     {alice, "/timelines/local", []string{local, ja, last}},
		"alice's federated": {alice, "/timelines/public", []string{ja, last}},
		"erin's local":      {erin, "/timelines/local", []string{muted, blocker, local, last}},
	} {
		var got []string
		for range tc.want {
			got = append(got, nextUpdate(t, e, name, streams[name], 5*time.Second))
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s stream = %v, want %v", name, got, tc.want)
		}
		// The REST timeline lists the same new posts.
		var listed []string
		for _, id := range itemIDs(e.page(t, &tc.viewer, tc.path)) {
			if id != before {
				listed = append(listed, id)
			}
		}
		slices.Reverse(listed)
		if !slices.Equal(listed, tc.want) {
			t.Errorf("%s REST timeline lists %v, the stream sent %v", name, listed, tc.want)
		}
	}

	disconnect()
	deadline = time.Now().Add(5 * time.Second)
	for e.app.Streams.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d subscriptions still open after the clients disconnected", e.app.Streams.Len())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
