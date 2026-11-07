// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform_test

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// newSearchPluginEnv is a server with search = plugin, whose plugin holds the fake index.
func newSearchPluginEnv(t *testing.T, o fakeplugin.Options) (*searchEnv, *fakeplugin.Recorder) {
	t.Helper()
	index := fake.New()
	o.Search = index
	h, rec := fakeplugin.Connect(o)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin,
		"MY_SEARCH": "plugin", "MY_PLUGIN_SEARCH_ADDRESS": fakeplugin.ServeUnix(t, h),
	})
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Clock: clock})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &searchEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, index: index, clock: clock}, rec
}

// TestSearchByAPlugin checks search = plugin end to end: the jobs write to the plugin, the search API asks it, and the
// server applies the view policy to the IDs it returns. Starting makes no call.
func TestSearchByAPlugin(t *testing.T) {
	e, rec := newSearchPluginEnv(t, fakeplugin.Options{})
	if n := rec.Calls(); n != 0 {
		t.Errorf("the start made %d calls", n)
	}
	viewer, alice := e.user("viewer"), e.user("alice")
	e.setIndexable(alice, true)

	public := e.post(alice, "plugged needle", "public")
	e.drain()
	e.wantSearch(viewer.token, "needle", public)

	// What the plugin holds but the viewer may not see stays out.
	private := e.post(alice, "planted", "private")
	e.drain()
	e.plant("planted", private)
	e.wantSearch(viewer.token, "planted")

	e.want(e.do(http.MethodDelete, "/statuses/"+public, alice.token, nil, nil), http.StatusNoContent, "DELETE")
	e.drain()
	if got := e.found("needle"); len(got) != 0 {
		t.Errorf("the plugin still holds %v", got)
	}
}

// TestSearchWithAPluginThatFails checks that a plugin that fails stops nothing but the search, which answers 503.
func TestSearchWithAPluginThatFails(t *testing.T) {
	e, _ := newSearchPluginEnv(t, fakeplugin.Options{Broken: true})
	viewer := e.user("viewer")
	e.want(e.do(http.MethodGet, "/statuses/search?q=anything", viewer.token, nil, nil), http.StatusServiceUnavailable, "a failed search")
}
