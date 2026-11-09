// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// timelineQueries counts the database round trips of one request of each timeline, on a page of
// size statuses: posts by two authors, each with a mention and a hashtag, with replies, quotes,
// polls, and pictures among them, and boosts, read by a signed-in viewer who follows both authors. Every query is one round trip, on
// SQLite as on PostgreSQL (one call in pg_stat_statements).
func timelineQueries(t *testing.T, size int) map[string]int {
	t.Helper()
	out, _ := timelineScene(t, size)
	return out
}

// timelineScene builds the scene of timelineQueries, counts the round trips of each timeline, and
// returns them with the app, the viewer bob, and the IDs of the statuses of bob's home page.
func timelineScene(t *testing.T, size int) (map[string]int, sceneOf) {
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
		in := appcontent.CreateInput{
			AuthorAccountID: authors[i%2].accountID, Visibility: domcontent.VisibilityPublic,
			TextInput: appcontent.TextInput{Text: fmt.Sprintf("post %d for @dave #tag%d", i, i), Language: "en"},
		}
		// Replies, quotes, polls, and pictures among the posts, each with its own parts to read.
		switch last := len(posts) - 1; {
		case last >= 0 && i%5 == 1:
			in.InReplyToID = posts[last]
		case last >= 0 && i%5 == 2:
			in.QuotedStatusID = posts[last]
		case i%7 == 4:
			in.Poll = &domcontent.PollInput{Options: []string{"yes", "no"}, Duration: time.Hour}
		case i%7 == 5:
			m, err := app.Media.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: in.AuthorAccountID, Content: bytes.NewReader(testkit.FixturePNG(64, 48, false))})
			if err != nil {
				t.Fatalf("Upload: %v", err)
			}
			app.Media.Process(t.Context(), m.File.ID, "")
			in.Media = []appcontent.MediaInput{{MediaID: m.File.ID}}
		}
		p, err := app.Content.Create(t.Context(), in)
		if err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
		posts = append(posts, p.Status.ID)
	}

	// A query's span is named after its operation (SELECT, BEGIN, ...) and carries it as an
	// attribute; a transaction's span does not.
	scene := sceneOf{app: app, viewer: bob}
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
		if len(page.Items) < size*3/4 {
			t.Fatalf("%s has %d statuses, want most of the %d", path, len(page.Items), size)
		}
		if path == "/timelines/home" {
			for _, st := range page.Items {
				id := st.Id
				if st.Boost != nil {
					id = st.Boost.Id
				}
				scene.ids = append(scene.ids, id)
			}
		}
	}
	return out, scene
}

// sceneOf is what timelineScene built: the app, the viewer, and the statuses of the viewer's
// home page.
type sceneOf struct {
	app    *platform.App
	viewer timelineUser
	ids    []string
}

// TestTimelineQueries checks that the round trips of a timeline do not grow with its page: a page
// reads its statuses, everything they refer to, and everything the viewer did with them together
// (content.Reader.page), so a page of 30 statuses makes as many as a page of 10.
func TestTimelineQueries(t *testing.T) {
	t.Parallel()
	small, large := timelineQueries(t, 10), timelineQueries(t, 30)
	for _, path := range []string{"/timelines/local", "/timelines/home", "/timelines/federated"} {
		t.Logf("%s: %d queries for 10 statuses, %d for 30", path, small[path], large[path])
		if large[path] > small[path] {
			t.Errorf("%s: %d queries for 30 statuses, more than the %d for 10", path, large[path], small[path])
		}
	}
}

// TestTimelineViewsAreThoseOfOneStatus checks that a page, which reads its statuses together,
// presents each of them exactly as View presents it alone, for a visitor and for a signed-in
// viewer: the replies, quotes, polls, pictures, mentions, hashtags, and the viewer's own state.
func TestTimelineViewsAreThoseOfOneStatus(t *testing.T) {
	t.Parallel()
	_, scene := timelineScene(t, 30)
	for name, viewer := range map[string]appcontent.Viewer{
		"visitor": {Kind: domcontent.ViewerAnonymous},
		"bob":     {Kind: domcontent.ViewerLocal, AccountID: scene.viewer.accountID},
	} {
		views, err := scene.app.Statuses.Views(t.Context(), viewer, scene.ids)
		if err != nil {
			t.Fatalf("%s: Views: %v", name, err)
		}
		if len(views) != len(scene.ids) {
			t.Fatalf("%s: Views = %d statuses, want %d", name, len(views), len(scene.ids))
		}
		for i, id := range scene.ids {
			one, err := scene.app.Statuses.View(t.Context(), viewer, id)
			if err != nil {
				t.Fatalf("%s: View(%s): %v", name, id, err)
			}
			if !reflect.DeepEqual(views[i], one) {
				t.Errorf("%s: the page's view of %s differs from View:\npage %+v\none  %+v", name, id, views[i], one)
			}
		}
	}
}
