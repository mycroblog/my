// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"log/slog"
	"testing"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// timelineQueries counts the database round trips of one request of each timeline, on a page of
// size statuses: posts by two authors, each with a mention and a hashtag, and boosts among them,
// read by a signed-in viewer who follows both authors. Every query is one round trip, on
// SQLite as on PostgreSQL (one call in pg_stat_statements).
func timelineQueries(t *testing.T, size int) map[string]int {
	t.Helper()
	tracer := testkit.NewTracer()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &timelineAPI{app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.user(t, "dave") // the account the posts mention
	e.follow(t, bob, alice)
	e.follow(t, bob, carol)
	authors := []timelineUser{alice, carol}
	var posts []string
	for i := range size {
		if i%4 == 3 && len(posts) > 0 {
			e.boost(t, carol, posts[len(posts)-1])
			continue
		}
		posts = append(posts, e.post(t, authors[i%2], fmt.Sprintf("post %d for @dave #tag%d", i, i), domcontent.VisibilityPublic, "en"))
	}

	// A query's span is named after its operation (SELECT, BEGIN, ...) and carries it as an
	// attribute; a transaction's span does not.
	queries := func() int {
		n := 0
		for _, sp := range tracer.Spans() {
			if sp.Attrs["db.operation.name"] != "" {
				n++
			}
		}
		return n
	}
	out := map[string]int{}
	for _, path := range []string{"/timelines/local", "/timelines/home", "/timelines/federated"} {
		before := queries()
		page := e.page(t, &bob, path+fmt.Sprintf("?limit=%d", size))
		out[path] = queries() - before
		if len(page.Items) == 0 {
			t.Fatalf("%s is empty", path)
		}
	}
	return out
}

// maxQueriesPerStatus bounds the round trips that one more status of a page adds. The page reads
// its statuses and their authors together; what each status reads of its own (its revision, its
// relations to the viewer, its counters, ...) is still read status by status, and is what this
// allows.
const maxQueriesPerStatus = 40

// TestTimelineQueries checks how the round trips of a timeline grow with its page.
func TestTimelineQueries(t *testing.T) {
	t.Parallel()
	small, large := timelineQueries(t, 5), timelineQueries(t, 20)
	for _, path := range []string{"/timelines/local", "/timelines/home", "/timelines/federated"} {
		t.Logf("%s: %d queries for 5 statuses, %d for 20", path, small[path], large[path])
		if per := float64(large[path]-small[path]) / 15; per > maxQueriesPerStatus {
			t.Errorf("%s: %.1f queries per status, want at most %d", path, per, maxQueriesPerStatus)
		}
	}
}
