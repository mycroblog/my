// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// searchJobTypes are the job types of the search module (ADR-0149 §7).
var searchJobTypes = []string{"search.narrow", "search.rebuild", "search.sync_author", "search.sync_domain", "search.sync_status"}

// TestSearchNone checks the default of ADR-0149 §2: the server has no backend, says so to the
// clients, and a post enqueues no search job, while the job types stay registered and their
// handlers succeed doing nothing (§7).
func TestSearchNone(t *testing.T) {
	e := newAccountsEnv(t)
	if e.app.Search.Enabled() {
		t.Error("Search.Enabled() = true with search = none")
	}

	var inst clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	if inst.FullTextSearch.Available || inst.FullTextSearch.Visibilities == nil || len(inst.FullTextSearch.Visibilities) != 0 {
		t.Errorf("full_text_search = %+v, want not available and an empty list", inst.FullTextSearch)
	}

	alice := e.user("alice")
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "hello", "visibility": "public"}, nil), http.StatusCreated, "POST /statuses")
	jobs, err := e.app.JobInspector.ListJobs(t.Context(), time.Now().Add(time.Hour), queue.JobFilter{}, queue.JobPosition{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) == 0 {
		t.Error("the post enqueued no job, so this test sees nothing")
	}
	for _, j := range jobs {
		if strings.HasPrefix(j.Type, "search.") {
			t.Errorf("job %s enqueued with search = none", j.Type)
		}
	}

	for _, jobType := range searchJobTypes {
		if !slices.Contains(e.app.Handlers.Types(), jobType) {
			t.Errorf("no handler of %s", jobType)
			continue
		}
		h, _, _ := e.app.Handlers.Lookup(jobType)
		if res := h(t.Context(), queue.Job{Type: jobType}); !res.IsDone() {
			t.Errorf("%s: %+v, want done", jobType, res)
		}
	}
	for _, jobType := range searchJobTypes {
		if !slices.Contains(platform.JobTypes(), jobType) {
			t.Errorf("%s is not among the job types the settings can name", jobType)
		}
	}
}

// TestSearchWithABackend checks that the clients can tell a server with a backend, without a
// search that fails (ADR-0149 §9).
func TestSearchWithABackend(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), SearchIndex: fake.New()})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}

	var inst clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	if !inst.FullTextSearch.Available || len(inst.FullTextSearch.Visibilities) != 1 || inst.FullTextSearch.Visibilities[0] != "public" {
		t.Errorf("full_text_search = %+v, want available on public", inst.FullTextSearch)
	}
}
