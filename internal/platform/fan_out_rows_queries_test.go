// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// newCountedFanOutEnv is newFanOutEnv with a tracer, and fan-outs that take every recipient in
// their first job.
func newCountedFanOutEnv(t *testing.T, tracer *testkit.Tracer) *fanOutEnv {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), FanOutSize: 1000, Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &fanOutEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, app: app}
}

// fanOutRowsQueries counts the round trips of the first job of the announcement fan-out and of
// the service closure fan-out over n local users, and of the webhook message of a post to n
// endpoints.
func fanOutRowsQueries(t *testing.T, n int) (announcement, closure, hooks int) {
	t.Helper()
	tracer := testkit.NewTracer()
	e := newCountedFanOutEnv(t, tracer)
	admin := e.user("admin")
	users := append(e.members("user", n), admin)
	run := func(jobType string) int {
		t.Helper()
		jobs := e.pending(jobType)
		if len(jobs) != 1 {
			t.Fatalf("%d jobs of %s, want 1", len(jobs), jobType)
		}
		was := ops(tracer)
		e.handle(jobs[0])
		count, names := opsSince(tracer, was)
		t.Logf("%s over %d users: %d queries: %v", jobType, len(users), count, names)
		return count
	}
	var a clientapi.AdminAnnouncement
	e.want(e.do(http.MethodPost, "/admin/announcements", admin.token, map[string]any{"title": "Hello", "text": "Hello"}, &a), http.StatusCreated, "create")
	e.want(e.do(http.MethodPost, "/admin/announcements/"+a.Id+"/publish", admin.token, nil, nil), http.StatusOK, "publish")
	announcement = run(appnotification.JobNotifyAnnouncement)
	closes := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Second)
	e.want(e.do(http.MethodPut, "/admin/closure/announcement", admin.token, map[string]any{"closes_at": closes}, &clientapi.AdminServiceClosure{}), http.StatusOK, "announce")
	closure = run(appnotification.JobServiceClosureFanout)
	for _, u := range users {
		if got := e.count(u, clientapi.NotificationTypeAnnouncement); got != 1 {
			t.Errorf("announcement notifications = %d, want 1", got)
		}
		if got := e.count(u, clientapi.NotificationTypeServiceClosure); got != 1 {
			t.Errorf("service closure notifications = %d, want 1", got)
		}
	}

	for i := range n {
		e.want(e.do(http.MethodPost, "/admin/webhooks", admin.token, map[string]any{
			"url": fmt.Sprintf("https://hooks.example/%d", i), "events": []string{"status.created"},
		}, nil), http.StatusCreated, "create a webhook")
	}
	was := ops(tracer)
	e.want(e.do(http.MethodPost, "/statuses", users[0].token, map[string]any{"text": "hello", "visibility": "public"}, nil), http.StatusCreated, "post")
	hooks, names := opsSince(tracer, was)
	t.Logf("a post with %d webhooks: %d queries: %v", n, hooks, names)
	return announcement, closure, hooks
}

// TestFanOutRowsQueries checks that a job of the announcement and the service closure fan-outs,
// and a webhook message, write their rows with a number of round trips that does not grow with
// the recipients or the endpoints.
func TestFanOutRowsQueries(t *testing.T) {
	t.Parallel()
	smallA, smallC, smallH := fanOutRowsQueries(t, 2)
	largeA, largeC, largeH := fanOutRowsQueries(t, 6)
	for _, c := range []struct {
		what         string
		small, large int
	}{{"the announcement fan-out", smallA, largeA}, {"the closure fan-out", smallC, largeC}, {"a webhook message", smallH, largeH}} {
		if c.large > c.small {
			t.Errorf("%s makes %d queries for 6, more than the %d for 2", c.what, c.large, c.small)
		}
	}
}
