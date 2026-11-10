// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// failingIndex is the fake with writes that fail while a test says so.
type failingIndex struct {
	*fake.Index
	mu   sync.Mutex
	fail func(statusID string) error
}

func (x *failingIndex) setFail(fail func(string) error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.fail = fail
}

func (x *failingIndex) Replace(ctx context.Context, doc search.Document) error {
	x.mu.Lock()
	fail := x.fail
	x.mu.Unlock()
	if fail != nil {
		if err := fail(doc.StatusID); err != nil {
			return err
		}
	}
	return x.Index.Replace(ctx, doc)
}

// frozenClock tells the time of its start until a test moves it, so that a rebuild's rate, which
// spaces its jobs by the clock, never lets the next job fall due while a slow machine goes on.
type frozenClock struct{ at time.Time }

func (c frozenClock) Now() time.Time { return c.at }

// newRebuildEnv is a server with the fake backend whose rebuild covers batch statuses in a job,
// on a clock that does not move.
func newRebuildEnv(t *testing.T, batch int) (*searchEnv, *failingIndex) {
	t.Helper()
	index := &failingIndex{Index: fake.New()}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{
		Logger: slog.New(slog.DiscardHandler), SearchIndex: index, SearchRebuildBatch: batch, Clock: frozenClock{at: time.Now().Round(0)},
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &searchEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, index: index.Index}, index
}

func (e *searchEnv) rebuild(opts search.RebuildOptions) search.RebuildStart {
	e.t.Helper()
	out, err := e.app.Search.StartRebuild(e.t.Context(), opts)
	if err != nil {
		e.t.Fatalf("StartRebuild: %v", err)
	}
	return out
}

func (e *searchEnv) progress() search.Rebuild {
	e.t.Helper()
	r, _, ok, err := e.app.Search.RebuildProgress(e.t.Context(), 10)
	if err != nil || !ok {
		e.t.Fatalf("RebuildProgress = %v, %v", ok, err)
	}
	return r
}

// strayDoc writes a document the database knows nothing of, as a missed deletion leaves behind.
func (e *searchEnv) strayDoc(id, word string) {
	e.t.Helper()
	err := e.index.Replace(e.t.Context(), search.Document{
		StatusID: id, AuthorID: "0197c000-0000-7000-8000-000000000000", Visibility: search.VisibilityPublic,
		PublishedAt: time.Now(), Text: word,
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

const strayID = "0197c000-0000-7000-8000-000000000001"

// TestSearchRebuildFromTheDatabase checks the default: what a missed deletion left goes, what
// qualifies comes back, what does not stays out, and running it twice gives the same index.
func TestSearchRebuildFromTheDatabase(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	alice, bob := e.user("alice"), e.user("bob")
	e.setIndexable(alice, true)
	public := e.post(alice, "armadillo public", "public")
	e.post(alice, "armadillo local", "local")
	e.post(alice, "armadillo private", "private")
	e.post(alice, "armadillo unlisted", "unlisted")
	e.post(bob, "armadillo from someone not indexable", "public")
	e.drain()
	e.strayDoc(strayID, "armadillo")
	e.wantFound("armadillo", public, strayID)

	for range 2 {
		if out := e.rebuild(search.RebuildOptions{}); !out.Cleared || out.Resumed {
			t.Errorf("StartRebuild = %+v, want a new rebuild that cleared", out)
		}
		e.wantFound("armadillo", public)
		if r := e.progress(); r.Phase != search.RebuildFinished || r.Mode != search.RebuildClear || r.Processed != 3 || r.Failed != 0 {
			t.Errorf("progress = %+v; want finished, with the 3 public and local statuses handled", r)
		}
	}
}

// TestSearchRebuildKeep checks --keep: it adds older posts after widening the setting, and leaves
// the index otherwise as it was, the rest of what search finds included.
func TestSearchRebuildKeep(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	public := e.post(alice, "armadillo public", "public")
	local := e.post(alice, "armadillo local", "local")
	e.drain()
	e.strayDoc(strayID, "armadillo")
	e.wantFound("armadillo", public, strayID)

	e.setVisibilities("public,local")
	e.wantFound("armadillo", public, strayID)
	if out := e.rebuild(search.RebuildOptions{Keep: true}); out.Cleared || out.Resumed {
		t.Errorf("StartRebuild = %+v, want a new rebuild that did not clear", out)
	}
	e.wantFound("armadillo", public, local, strayID)
	if r := e.progress(); r.Phase != search.RebuildFinished || r.Mode != search.RebuildKeep {
		t.Errorf("progress = %+v", r)
	}
}

// TestSearchRebuildFollowsTheSetting checks that a rebuild reads the current setting.
func TestSearchRebuildFollowsTheSetting(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	e.post(alice, "armadillo public", "public")
	local := e.post(alice, "armadillo local", "local")
	e.drain()

	e.setVisibilities("local")
	e.rebuild(search.RebuildOptions{})
	e.wantFound("armadillo", local)
}

// TestSearchRebuildResumes interrupts a rebuild between batches: at the rate of 1 the next batch is
// seconds away. Resuming goes on from its place, and does not clear what the walk already wrote. The
// job left waiting finds that the rebuild moved on and does nothing.
func TestSearchRebuildResumes(t *testing.T) {
	t.Parallel()
	e, _ := newRebuildEnv(t, 2)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = e.post(alice, "armadillo", "public")
	}
	e.drain()

	if out := e.rebuild(search.RebuildOptions{Rate: 1}); !out.Cleared || out.Rebuild.Rate != 1 {
		t.Fatalf("StartRebuild = %+v", out)
	}
	e.wantFound("armadillo", ids[0], ids[1])
	r := e.progress()
	if r.Phase != search.RebuildFilling || r.After != ids[1] || r.Processed != 2 {
		t.Fatalf("after the first batch = %+v", r)
	}

	e.strayDoc(strayID, "armadillo")
	out := e.rebuild(search.RebuildOptions{Rate: 1})
	if !out.Resumed || out.Cleared {
		t.Errorf("StartRebuild again = %+v, want a resumption that does not clear", out)
	}
	e.wantFound("armadillo", ids[0], ids[1], ids[2], ids[3], strayID)

	out = e.rebuild(search.RebuildOptions{Rate: 900})
	if !out.Resumed || out.Rebuild.Rate != 900 {
		t.Errorf("StartRebuild with a new rate = %+v", out)
	}
	e.wantFound("armadillo", append(slices.Clone(ids), strayID)...)
	if r := e.progress(); r.Phase != search.RebuildFinished || r.Processed != 5 || r.Rate != 900 {
		t.Errorf("at the end = %+v; each status must count once", r)
	}
	// The next rebuild is a new one.
	if out := e.rebuild(search.RebuildOptions{}); out.Resumed || !out.Cleared {
		t.Errorf("after the end = %+v, want a new rebuild", out)
	}
}

// TestSearchRebuildRefusesTheOtherMode checks that asking for the mode the unfinished rebuild does
// not have changes nothing, and that my search clear makes room for it.
func TestSearchRebuildRefusesTheOtherMode(t *testing.T) {
	t.Parallel()
	e, _ := newRebuildEnv(t, 1)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	e.post(alice, "armadillo one", "public")
	e.post(alice, "armadillo two", "public")
	e.drain()

	e.rebuild(search.RebuildOptions{Keep: true, Rate: 1})
	e.drain()
	if _, err := e.app.Search.StartRebuild(t.Context(), search.RebuildOptions{}); !errors.Is(err, search.ErrRebuildModeDiffers) {
		t.Errorf("StartRebuild = %v, want ErrRebuildModeDiffers", err)
	}
	if r := e.progress(); r.Mode != search.RebuildKeep {
		t.Errorf("the refused call changed the rebuild: %+v", r)
	}
	if err := e.app.Search.ClearIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if out := e.rebuild(search.RebuildOptions{Rate: 1}); out.Resumed {
		t.Errorf("after clear = %+v, want a new rebuild", out)
	}
}

// TestSearchRebuildDuringPosts checks what happens between two batches: a post deleted after the walk
// passed it, a post deleted before the walk reaches it, and a new post all end up right.
func TestSearchRebuildDuringPosts(t *testing.T) {
	t.Parallel()
	e, _ := newRebuildEnv(t, 2)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = e.post(alice, "armadillo", "public")
	}
	e.drain()

	e.rebuild(search.RebuildOptions{Rate: 1})
	e.wantFound("armadillo", ids[0], ids[1])
	for _, id := range []string{ids[0], ids[3]} {
		e.want(e.do(http.MethodDelete, "/statuses/"+id, alice.token, nil, nil), http.StatusNoContent, "DELETE status")
	}
	added := e.post(alice, "armadillo", "public")
	e.wantFound("armadillo", ids[1], added)

	for range 5 {
		e.rebuild(search.RebuildOptions{Rate: 900})
		e.drain()
	}
	e.wantFound("armadillo", ids[1], ids[2], added)
	if r := e.progress(); r.Phase != search.RebuildFinished {
		t.Errorf("at the end = %+v", r)
	}
}

// TestSearchClearEndsTheRebuild checks that clearing empties the index and leaves nothing that fills
// it again: the job left waiting of the rebuild does nothing.
func TestSearchClearEndsTheRebuild(t *testing.T) {
	t.Parallel()
	e, _ := newRebuildEnv(t, 1)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	e.post(alice, "armadillo one", "public")
	e.post(alice, "armadillo two", "public")
	e.drain()

	e.rebuild(search.RebuildOptions{Rate: 1000})
	if err := e.app.Search.ClearIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := e.app.Search.RebuildProgress(t.Context(), 10); ok || err != nil {
		t.Errorf("RebuildProgress = %v, %v; want no rebuild", ok, err)
	}
	time.Sleep(5 * time.Millisecond)
	e.wantFound("armadillo")
}

// TestSearchRebuildFailures checks a status the engine refuses, which is skipped and recorded, and an
// outage, where the walk stays at its place until the engine is back.
func TestSearchRebuildFailures(t *testing.T) {
	t.Parallel()
	e, index := newRebuildEnv(t, 100)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = e.post(alice, "armadillo", "public")
	}
	e.drain()

	index.setFail(func(id string) error {
		if id == ids[1] {
			return errors.New("the engine refuses this one")
		}
		return nil
	})
	e.rebuild(search.RebuildOptions{})
	e.wantFound("armadillo", ids[0], ids[2], ids[3])
	r, failures, _, err := e.app.Search.RebuildProgress(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Phase != search.RebuildFinished || r.Processed != 4 || r.Failed != 1 || r.LastError != "the engine refuses this one" {
		t.Errorf("progress = %+v", r)
	}
	if len(failures) != 1 || failures[0].StatusID != ids[1] {
		t.Errorf("failures = %+v", failures)
	}

	// --keep retries what failed, and clears the failures.
	index.setFail(nil)
	e.rebuild(search.RebuildOptions{Keep: true})
	e.wantFound("armadillo", ids...)
	r, failures, _, err = e.app.Search.RebuildProgress(t.Context(), 10)
	if err != nil || r.Failed != 0 || r.LastError != "" || len(failures) != 0 {
		t.Errorf("after --keep = %+v, %+v, %v", r, failures, err)
	}

	// The engine goes down: the walk records the error and stays where it was.
	index.setFail(func(string) error { return errors.New("the engine is down") })
	e.rebuild(search.RebuildOptions{Keep: true})
	e.drain()
	r, failures, _, err = e.app.Search.RebuildProgress(t.Context(), 10)
	if err != nil || r.Phase != search.RebuildFilling || r.After != "" || r.Processed != 0 || r.LastError != "the engine is down" || len(failures) != 0 {
		t.Errorf("during the outage = %+v, %+v, %v", r, failures, err)
	}
	index.setFail(nil)
	if out := e.rebuild(search.RebuildOptions{Keep: true}); !out.Resumed {
		t.Errorf("after the outage = %+v, want a resumption", out)
	}
	e.drain()
	if r := e.progress(); r.Phase != search.RebuildFinished || r.Processed != 4 || r.LastError != "" {
		t.Errorf("after the outage = %+v", r)
	}
}

// TestSearchRebuildWithNone checks that search = none writes nothing and runs nothing.
func TestSearchRebuildWithNone(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.Search.StartRebuild(t.Context(), search.RebuildOptions{}); !errors.Is(err, search.ErrNoBackend) {
		t.Errorf("StartRebuild = %v, want ErrNoBackend", err)
	}
	if err := app.Search.ClearIndex(t.Context()); !errors.Is(err, search.ErrNoBackend) {
		t.Errorf("ClearIndex = %v, want ErrNoBackend", err)
	}
	if _, _, _, err := app.Search.RebuildProgress(t.Context(), 1); !errors.Is(err, search.ErrNoBackend) {
		t.Errorf("RebuildProgress = %v, want ErrNoBackend", err)
	}
}
