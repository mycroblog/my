// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// publicStreamQueries counts the database round trips that the local timeline streams of n
// connected accounts make for one new public post, from its signal until each of them got it.
func publicStreamQueries(t *testing.T, n int) int {
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
	}
	post := e.post(t, author, "hello", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	streams := make([]<-chan sseEvent, n)
	for i, v := range viewers {
		streams[i] = openTimelineStream(ctx, t, e, srv, v, "/streaming/timelines/local")
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
	app.Streams.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: post, OnLocal: true, OnFederated: true})
	for i, events := range streams {
		if got := nextUpdate(t, e, fmt.Sprintf("viewer%d", i), events, 5*time.Second); got != post {
			t.Fatalf("viewer%d got %s, want %s", i, got, post)
		}
	}
	return total() - before
}

// TestPublicStreamQueries checks that a new public post makes a number of round trips on the
// local timeline streams that does not grow with their connections.
func TestPublicStreamQueries(t *testing.T) {
	t.Parallel()
	small, large := publicStreamQueries(t, 2), publicStreamQueries(t, 8)
	t.Logf("%d queries for 2 connections, %d for 8", small, large)
	if large > small {
		t.Errorf("%d queries for 8 connections, more than the %d for 2", large, small)
	}
}
