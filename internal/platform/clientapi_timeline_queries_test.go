// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// timelineQueries counts the database round trips of one request of each timeline, on a page of
// size statuses: posts by two authors, each with a mention and a hashtag, with replies, quotes,
// polls, pictures, and link cards to sites of their own among them, and boosts, read by a signed-in viewer who follows both authors. Every query is one round trip, on
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
	// Each post with a link links to a site of its own, with an image of its own.
	png := testkit.FixturePNG(64, 48, false)
	web := &pageWeb{hits: map[string]int{}, pages: map[string]webPage{}}
	for i := range size {
		site := fmt.Sprintf("https://site%d.example", i)
		web.pages[site+"/p"] = webPage{http.StatusOK, "text/html", cardPage(fmt.Sprintf("Site %d", i), "/i.png")}
		web.pages[site+"/i.png"] = webPage{http.StatusOK, "image/png", png}
	}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Tracer: tracer, HTTPClient: web})
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
		case i%2 == 0:
			// A link card, on a host of its own.
			in.Text += fmt.Sprintf(" https://site%d.example/p", i)
		}
		p, err := app.Content.Create(t.Context(), in)
		if err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
		posts = append(posts, p.Status.ID)
		fetchLinkCard(t, app, p.Status.ID)
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
		if cards := countCards(page.Items); cards == 0 {
			t.Fatalf("%s shows no link card", path)
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

// countCards counts the statuses of items that show a link card, boosted ones included.
func countCards(items []clientapi.Status) int {
	n := 0
	for _, st := range items {
		if st.Boost != nil {
			st = *st.Boost
		}
		if st.Card != nil && st.Card.Image != nil {
			n++
		}
	}
	return n
}

// TestViewsForAreThoseOfEachViewer checks that ViewsFor, which presents a page to many viewers
// with the shared parts read once, presents it to each of them exactly as Views does: bob, who
// follows the authors and interacted with nothing, and a stranger.
func TestViewsForAreThoseOfEachViewer(t *testing.T) {
	t.Parallel()
	_, scene := timelineScene(t, 30)
	stranger, err := scene.app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: "stranger", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	viewers := []appcontent.Viewer{
		{Kind: domcontent.ViewerLocal, AccountID: scene.viewer.accountID},
		{Kind: domcontent.ViewerLocal, AccountID: stranger.AccountID},
	}
	// bob favourites and bookmarks a status, and votes in no poll: his parts differ from the stranger's.
	if len(scene.ids) > 0 {
		if _, err := scene.app.Interactions.Favourite(t.Context(), scene.viewer.accountID, scene.ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scene.app.Statuses.ViewsFor(t.Context(), viewers, scene.ids)
	if err != nil {
		t.Fatalf("ViewsFor: %v", err)
	}
	for _, v := range viewers {
		want, err := scene.app.Statuses.Views(t.Context(), v, scene.ids)
		if err != nil {
			t.Fatalf("Views(%s): %v", v.AccountID, err)
		}
		if !reflect.DeepEqual(got[v.AccountID], want) {
			t.Errorf("ViewsFor(%s) differs from Views:\nfor  %+v\nview %+v", v.AccountID, got[v.AccountID], want)
		}
	}
}
