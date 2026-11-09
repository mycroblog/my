// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// searchQueries counts the database round trips of one status search with n hits, half by local
// and half by remote authors, two hits each, and of the JobSyncAuthor run that brings the n
// statuses of one author in line. Every query is one round trip (one call in pg_stat_statements).
func searchQueries(t *testing.T, n int) (searching, syncing int) {
	t.Helper()
	tracer := testkit.NewTracer()
	index := fake.New()
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), SearchIndex: index, Clock: clock, Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &searchEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, index: index, clock: clock}
	viewer := e.user("viewer")
	for i := range n / 4 {
		u := e.user(fmt.Sprintf("local%d", i))
		e.setIndexable(u, true)
		e.post(u, "needle one", "public")
		e.post(u, "needle two", "public")
		domain := fmt.Sprintf("r%d.example", i)
		uri := "https://" + domain + "/users/remote"
		id := e.remoteActor(uri, domain, "remote", true)
		e.remoteStatus(uri, "1", id, domain, "<p>needle three</p>")
		e.remoteStatus(uri, "2", id, domain, "<p>needle four</p>")
	}
	if got := e.found("needle"); len(got) != n {
		t.Fatalf("the index has %d needles, want %d", len(got), n)
	}
	total := func() int { return count(ops(tracer)) }
	before := total()
	var page clientapi.StatusPage
	e.want(e.do(http.MethodGet, fmt.Sprintf("/statuses/search?q=needle&limit=%d", n), viewer.token, nil, &page), http.StatusOK, "search")
	searching = total() - before
	if len(page.Items) != n {
		t.Fatalf("the search found %d statuses, want %d", len(page.Items), n)
	}

	// One author with n statuses, synced again by one job.
	writer := e.user("writer")
	e.setIndexable(writer, true)
	for i := range n {
		e.post(writer, fmt.Sprintf("hay %d", i), "public")
	}
	e.drain()
	payload, err := json.Marshal(map[string]any{"account_id": writer.id})
	if err != nil {
		t.Fatal(err)
	}
	h, _, _ := app.Handlers.Lookup(search.JobSyncAuthor)
	before = total()
	if res := h(t.Context(), queue.Job{Type: search.JobSyncAuthor, Payload: payload}); res.Err() != nil {
		t.Fatalf("%s: %v", search.JobSyncAuthor, res.Err())
	}
	syncing = total() - before
	if got := e.found("hay"); len(got) != n {
		t.Fatalf("the index has %d hays, want %d", len(got), n)
	}
	return searching, syncing
}

// count sums the queries of byOp.
func count(byOp map[string]int) int {
	n := 0
	for _, c := range byOp {
		n += c
	}
	return n
}

// TestSearchQueries checks that a status search and an author's sync make a number of round trips
// that does not grow with their hits and statuses.
func TestSearchQueries(t *testing.T) {
	t.Parallel()
	smallSearch, smallSync := searchQueries(t, 8)
	largeSearch, largeSync := searchQueries(t, 20)
	t.Logf("search: %d queries for 8 hits, %d for 20; author sync: %d for 8 statuses, %d for 20", smallSearch, largeSearch, smallSync, largeSync)
	if largeSearch > smallSearch {
		t.Errorf("%d queries to search 20 hits, more than the %d for 8", largeSearch, smallSearch)
	}
	if largeSync > smallSync {
		t.Errorf("%d queries to sync 20 statuses, more than the %d for 8", largeSync, smallSync)
	}
}
