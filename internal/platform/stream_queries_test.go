// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// homeStreamQueries counts the database round trips that the user streams of n connected
// accounts make for one new post of an author whom two of them follow, from its signal until the
// two followers got it. Every query is one round trip (one call in pg_stat_statements).
func homeStreamQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &timelineAPI{app: app, h: app.Handler(), spec: apitest.Load(t)}
	author := e.user(t, "author")
	viewers := make([]timelineUser, n)
	for i := range viewers {
		viewers[i] = e.user(t, fmt.Sprintf("viewer%d", i))
		if i < 2 {
			e.follow(t, viewers[i], author)
		}
	}
	// The post is made before the streams open, and its signal published once they are.
	post := e.post(t, author, "hello", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	followers := make([]<-chan sseEvent, 2)
	for i, v := range viewers {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+v.token)
		res, err := srv.Client().Do(req)
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("GET /streaming: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		events := readEvents(bufio.NewReader(res.Body))
		if i < 2 {
			followers[i] = events
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for app.Streams.Len() < n {
		if time.Now().After(deadline) {
			t.Fatal("the streams did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}
	total := func() int {
		count := 0
		for _, c := range ops(tracer) {
			count += c
		}
		return count
	}
	before := total()
	app.Streams.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: post, AuthorAccountID: author.accountID})
	for _, events := range followers {
		select {
		case ev := <-events:
			if ev.name != "update" {
				t.Fatalf("event %q, want update", ev.name)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a follower got no update")
		}
	}
	// The other streams handle the signal by then or soon after; let them.
	time.Sleep(200 * time.Millisecond)
	return total() - before
}

// TestHomeStreamQueries checks that a new post makes a number of round trips on the user streams
// that does not grow with the connected accounts that do not follow its author.
func TestHomeStreamQueries(t *testing.T) {
	t.Parallel()
	small, large := homeStreamQueries(t, 4), homeStreamQueries(t, 12)
	t.Logf("%d queries for 4 connected accounts (2 followers), %d for 12", small, large)
	if large > small {
		t.Errorf("%d queries for 12 connected accounts, more than the %d for 4", large, small)
	}
}
