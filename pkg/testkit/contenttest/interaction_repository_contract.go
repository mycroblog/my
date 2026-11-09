// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// InteractionRepositoryContract verifies the interaction repository of a database adapter by its
// methods alone, the port contract of ADR-0153 §1, on statuses that the status repository stores
// with the content module's own projectors (StatusEnv): boosts and their retractions, each
// recorded once and found by ID and URI; boost_edges, one live boost per pair, listed, deleted
// with their status, and rebuilt without the retracted boosts and the deleted statuses; the
// counters of boosts, favourites, and local posts; the mention index; favourites, bookmarks, and
// pins as current values inserted once, paged newest first, and deleted; the pin slots, which keep
// concurrent pins within content.MaxPins, and concurrent favourites of one pair, which insert
// once (ADR-0153 §5); and emoji reactions with their counts per emoji, rebuilt from the reactions
// to statuses that are not deleted. The interaction use cases on top of it are InteractionFlows,
// which internal/flowtest/content runs once on SQLite.
func InteractionRepositoryContract(t *testing.T, newHarness InteractionFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *contentInteractionRepo)
	}{
		{"RecordsBoostsAndRetractions", contentInteractionRepoBoosts},
		{"KeepsOneBoostEdgePerPair", contentInteractionRepoBoostEdges},
		{"AddsToTheCounters", contentInteractionRepoCounters},
		{"KeepsFavouritesAndBookmarks", contentInteractionRepoFavourites},
		{"KeepsPinsInSlots", contentInteractionRepoPins},
		{"KeepsConcurrentPinsWithinTheSlots", contentInteractionRepoConcurrentPins},
		{"InsertsConcurrentFavouritesOnce", contentInteractionRepoConcurrentFavourites},
		{"KeepsReactionsAndTheirCounts", contentInteractionRepoReactions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h InteractionHarness
			se := newStatusEnv(t, func(t *testing.T, opts uow.Options) StatusHarness {
				h = newHarness(t, opts)
				return StatusHarness{UnitOfWork: h.UnitOfWork, Statuses: h.Statuses}
			})
			tc.run(t, &contentInteractionRepo{se: se, h: h, repo: h.Interactions})
		})
	}
}

type contentInteractionRepo struct {
	se   *StatusEnv
	h    InteractionHarness
	repo appcontent.InteractionRepository
}

func (r *contentInteractionRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func (r *contentInteractionRepo) id() string { return r.se.IDs.NewID() }

// status stores a public local status with its first revision, mentioning mentions.
func (r *contentInteractionRepo) status(t *testing.T, mentions ...string) content.Status {
	t.Helper()
	s := r.se.NewStatus(r.id(), content.VisibilityPublic)
	r.se.mustCreate(t, s, mentions...)
	return s
}

func (r *contentInteractionRepo) boost(t *testing.T, booster string, s content.Status, local bool) content.Boost {
	t.Helper()
	r.se.Clock.Advance(time.Second)
	id := r.id()
	b := content.Boost{
		ID: id, URI: "https://" + testkit.ContractHost + "/ap/activities/" + id, BoosterAccountID: booster, StatusID: s.ID,
		Visibility: s.Visibility, IsLocal: local, BoostedAt: r.se.Clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertBoost(ctx, tx, b); err != nil {
			return err
		}
		_, err := r.repo.InsertBoostEdge(ctx, tx, content.BoostEdge{BoosterAccountID: booster, StatusID: s.ID, BoostID: b.ID})
		return err
	})
	return b
}

func contentInteractionRepoBoosts(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	s := r.status(t)
	b := r.boost(t, r.id(), s, true)
	if got, err := r.repo.FindBoost(ctx, b.ID); err != nil || got != b {
		t.Errorf("FindBoost = %+v, %v; want %+v", got, err, b)
	}
	if got, err := r.repo.FindBoostByURI(ctx, b.URI); err != nil || got != b {
		t.Errorf("FindBoostByURI = %+v, %v; want %+v", got, err, b)
	}
	if _, err := r.repo.FindBoost(ctx, r.id()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindBoost(unknown) = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.FindBoosts(ctx, []string{b.ID, r.id()}); err != nil || len(got) != 1 || got[b.ID] != b {
		t.Errorf("FindBoosts = %+v, %v; want %+v alone, as FindBoost", got, err, b)
	}
	if _, err := r.repo.FindBoostRetraction(ctx, b.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindBoostRetraction before the retraction = %v, want ErrNotFound", err)
	}
	other := b
	other.ID, other.BoosterAccountID = r.id(), r.id()
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.repo.InsertBoost(ctx, tx, other) }); !errors.Is(err, appcontent.ErrBoostExists) {
		t.Errorf("InsertBoost(a taken URI) = %v, want ErrBoostExists", err)
	}

	ret := content.BoostRetraction{BoostID: b.ID, Origin: content.RetractedByBooster, RetractedAt: r.se.Clock.Now()}
	for i := range 2 {
		err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.repo.InsertBoostRetraction(ctx, tx, ret) })
		if dup := i == 1; dup != errors.Is(err, appcontent.ErrBoostRetracted) || !dup && err != nil {
			t.Errorf("InsertBoostRetraction #%d = %v; want ErrBoostRetracted only for the second", i+1, err)
		}
	}
	if got, err := r.repo.FindBoostRetraction(ctx, b.ID); err != nil || got != ret {
		t.Errorf("FindBoostRetraction = %+v, %v; want %+v", got, err, ret)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.FindBoostRetractionTx(ctx, tx, b.ID); err != nil || got != ret {
			t.Errorf("FindBoostRetractionTx = %+v, %v; want %+v", got, err, ret)
		}
		if got, err := r.repo.Boost(ctx, tx, b.ID); err != nil || got != b {
			t.Errorf("Boost = %+v, %v; want %+v", got, err, b)
		}
		if _, err := r.repo.Boost(ctx, tx, r.id()); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("Boost(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func contentInteractionRepoBoostEdges(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	s, gone := r.status(t), r.status(t)
	local, remote := r.id(), r.id()
	lb := r.boost(t, local, s, true)
	rb := r.boost(t, remote, s, false)
	edge := content.BoostEdge{BoosterAccountID: local, StatusID: s.ID, BoostID: lb.ID}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if inserted, err := r.repo.InsertBoostEdge(ctx, tx, content.BoostEdge{BoosterAccountID: local, StatusID: s.ID, BoostID: r.id()}); err != nil || inserted {
			t.Errorf("InsertBoostEdge(a second live boost of the pair) = %v, %v; want false", inserted, err)
		}
		if got, err := r.repo.BoostEdge(ctx, tx, local, s.ID); err != nil || got != edge {
			t.Errorf("BoostEdge = %+v, %v; want %+v", got, err, edge)
		}
		if got, err := r.repo.ListLocalStatusBoosters(ctx, tx, s.ID); err != nil || !slices.Equal(got, []string{local}) {
			t.Errorf("ListLocalStatusBoosters = %v, %v; want only the local booster", got, err)
		}
		live, err := r.repo.ListLiveBoosts(ctx, tx)
		if want := []content.Boost{lb, rb}; err != nil || !reflect.DeepEqual(live, want) {
			t.Errorf("ListLiveBoosts = %+v, %v; want %+v", live, err, want)
		}
		return nil
	})
	if got, err := r.repo.FindBoostEdge(ctx, local, s.ID); err != nil || got != edge {
		t.Errorf("FindBoostEdge = %+v, %v; want %+v", got, err, edge)
	}
	if _, err := r.repo.FindBoostEdge(ctx, r.id(), s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindBoostEdge(no boost) = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.FindAccountInteractions(ctx, local, []string{s.ID, r.id()}); err != nil || len(got.BoostEdges) != 1 || got.BoostEdges[s.ID] != edge {
		t.Errorf("FindAccountInteractions: boost edges = %+v, %v; want %+v alone, as FindBoostEdge", got.BoostEdges, err, edge)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			if deleted, err := r.repo.DeleteBoostEdge(ctx, tx, edge); err != nil || deleted != want {
				t.Errorf("DeleteBoostEdge #%d = %v, %v; want %v", i+1, deleted, err, want)
			}
		}
		return nil
	})

	// A rebuild derives the edges of the boosts that are not retracted, of statuses that are not
	// deleted: lb is live again, gb's status is deleted, and the retracted rb is gone.
	gb := r.boost(t, local, gone, true)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertBoostRetraction(ctx, tx, content.BoostRetraction{BoostID: rb.ID, Origin: content.RetractedByRemote, RetractedAt: r.se.Clock.Now()})
	})
	r.se.MustRemove(t, gone)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.RebuildBoostEdges(ctx, tx) })
	edges, err := r.repo.ListBoostEdges(ctx)
	if want := []content.BoostEdge{edge}; err != nil || !reflect.DeepEqual(edges, want) {
		t.Errorf("ListBoostEdges after the rebuild = %+v, %v; want %+v (not %s)", edges, err, want, gb.ID)
	}
	// The local boosters of many statuses, a booster of several once for each, and the edges of
	// many statuses deleted together.
	other := r.status(t)
	r.boost(t, local, other, true)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.ListLocalStatusBoostersOf(ctx, tx, []string{s.ID, other.ID, r.id()}); err != nil || !slices.Equal(got, []string{local, local}) {
			t.Errorf("ListLocalStatusBoostersOf = %v, %v; want the local booster once for each status", got, err)
		}
		return errors.Join(r.repo.DeleteStatusBoostEdgesOf(ctx, tx, []string{s.ID, other.ID}), r.repo.DeleteStatusBoostEdgesOf(ctx, tx, nil))
	})
	if edges, err := r.repo.ListBoostEdges(ctx); err != nil || len(edges) != 0 {
		t.Errorf("ListBoostEdges after DeleteStatusBoostEdgesOf = %+v, %v; want none", edges, err)
	}
}

func contentInteractionRepoCounters(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	mentioned := r.id()
	s, gone := r.status(t, mentioned), r.status(t)
	r.se.MustRemove(t, gone)
	account := r.id()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.AddBoosts(ctx, tx, s.ID, 2), r.repo.AddBoosts(ctx, tx, s.ID, -1), r.repo.AddFavourites(ctx, tx, s.ID, 3),
			// A deleted status has no counters and gains nothing.
			r.repo.AddBoosts(ctx, tx, gone.ID, 1), r.repo.AddFavourites(ctx, tx, gone.ID, 1),
			r.repo.AddLocalPosts(ctx, tx, account, 2),
		)
	})
	if c, err := r.h.Statuses.FindCounters(ctx, s.ID); err != nil || c.BoostsCount != 1 || c.FavouritesCount != 3 {
		t.Errorf("FindCounters = %+v, %v; want 1 boost and 3 favourites", c, err)
	}
	if _, err := r.h.Statuses.FindCounters(ctx, gone.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCounters(deleted) = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Statuses.FindCountersOf(ctx, []string{s.ID, gone.ID}); err != nil || len(got) != 1 ||
		got[s.ID].BoostsCount != 1 || got[s.ID].FavouritesCount != 3 {
		t.Errorf("FindCountersOf = %+v, %v; want the counters of the live status alone, as FindCounters", got, err)
	}
	localPosts := func() int64 {
		t.Helper()
		all, err := r.h.Statuses.ListLocalPostCounters(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range all {
			if c.AccountID == account {
				return c.PostsCount
			}
		}
		return -1
	}
	if n := localPosts(); n != 2 {
		t.Errorf("local posts = %d, want 2", n)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.AddLocalPosts(ctx, tx, account, -2) })
	if n := localPosts(); n != -1 {
		t.Errorf("local posts at zero = %d, want no counter", n)
	}
	// Many accounts at once, as AddLocalPosts of a negative delta does for each.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.AddLocalPosts(ctx, tx, account, 3) })
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.repo.SubtractLocalPostsOf(ctx, tx, map[string]int64{account: 1, r.id(): 1}), r.repo.SubtractLocalPostsOf(ctx, tx, nil))
	})
	if n := localPosts(); n != 2 {
		t.Errorf("local posts after SubtractLocalPostsOf = %d, want 2", n)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.SubtractLocalPostsOf(ctx, tx, map[string]int64{account: 2})
	})
	if n := localPosts(); n != -1 {
		t.Errorf("local posts after SubtractLocalPostsOf to zero = %d, want no counter", n)
	}

	for _, tc := range []struct {
		account string
		want    bool
	}{{mentioned, true}, {account, false}} {
		if got, err := r.repo.Mentioned(ctx, s.ID, tc.account); err != nil || got != tc.want {
			t.Errorf("Mentioned = %v, %v; want %v", got, err, tc.want)
		}
		if got, err := r.repo.MentionedIn(ctx, []string{s.ID, gone.ID}, tc.account); err != nil || got[s.ID] != tc.want || got[gone.ID] {
			t.Errorf("MentionedIn = %v, %v; want %v of the status alone, as Mentioned", got, err, tc.want)
		}
	}
	if got, err := r.repo.MentionedAmong(ctx, s.ID, []string{mentioned, account}); err != nil || !maps.Equal(got, map[string]bool{mentioned: true}) {
		t.Errorf("MentionedAmong = %v, %v; want the mentioned account alone, as Mentioned", got, err)
	}
}

func contentInteractionRepoFavourites(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	account := r.id()
	var statuses []content.Status
	for range 3 {
		statuses = append(statuses, r.status(t))
	}
	var favs []content.Favourite
	var marks []content.Bookmark
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, s := range statuses {
			f := content.Favourite{ID: r.id(), AccountID: account, StatusID: s.ID, URI: "https://remote.example/likes/" + s.ID, CreatedAt: r.se.Clock.Now()}
			if inserted, err := r.repo.InsertFavourite(ctx, tx, f); err != nil || !inserted {
				t.Errorf("InsertFavourite = %v, %v; want true", inserted, err)
			}
			b := content.Bookmark{ID: r.id(), AccountID: account, StatusID: s.ID, CreatedAt: r.se.Clock.Now()}
			if inserted, err := r.repo.InsertBookmark(ctx, tx, b); err != nil || !inserted {
				t.Errorf("InsertBookmark = %v, %v; want true", inserted, err)
			}
			favs, marks = append(favs, f), append(marks, b)
		}
		again := favs[0]
		again.ID, again.URI = r.id(), ""
		if inserted, err := r.repo.InsertFavourite(ctx, tx, again); err != nil || inserted {
			t.Errorf("InsertFavourite(again) = %v, %v; want false", inserted, err)
		}
		if inserted, err := r.repo.InsertBookmark(ctx, tx, content.Bookmark{ID: r.id(), AccountID: account, StatusID: statuses[0].ID, CreatedAt: r.se.Clock.Now()}); err != nil || inserted {
			t.Errorf("InsertBookmark(again) = %v, %v; want false", inserted, err)
		}
		return nil
	})
	taken := content.Favourite{ID: r.id(), AccountID: r.id(), StatusID: statuses[0].ID, URI: favs[0].URI, CreatedAt: r.se.Clock.Now()}
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := r.repo.InsertFavourite(ctx, tx, taken)
		return err
	}); !errors.Is(err, appcontent.ErrFavouriteURIUsed) {
		t.Errorf("InsertFavourite(a taken URI) = %v, want ErrFavouriteURIUsed", err)
	}

	if got, err := r.repo.FindFavourite(ctx, account, statuses[1].ID); err != nil || got != favs[1] {
		t.Errorf("FindFavourite = %+v, %v; want %+v", got, err, favs[1])
	}
	if got, err := r.repo.FindFavouriteByURI(ctx, favs[2].URI); err != nil || got != favs[2] {
		t.Errorf("FindFavouriteByURI = %+v, %v; want %+v", got, err, favs[2])
	}
	if got, err := r.repo.FindBookmark(ctx, account, statuses[1].ID); err != nil || got != marks[1] {
		t.Errorf("FindBookmark = %+v, %v; want %+v", got, err, marks[1])
	}
	if _, err := r.repo.FindBookmark(ctx, r.id(), statuses[1].ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindBookmark(another account) = %v, want ErrNotFound", err)
	}
	ids := []string{r.id()}
	for _, st := range statuses {
		ids = append(ids, st.ID)
	}
	if got, err := r.repo.FindAccountInteractions(ctx, account, ids); err != nil {
		t.Errorf("FindAccountInteractions = %v", err)
	} else {
		for _, st := range statuses {
			f, ferr := r.repo.FindFavourite(ctx, account, st.ID)
			if g, ok := got.Favourites[st.ID]; ok != (ferr == nil) || ok && g != f {
				t.Errorf("FindAccountInteractions: favourite on %s = %+v (%t), want %+v (%v) as FindFavourite", st.ID, g, ok, f, ferr)
			}
			m, merr := r.repo.FindBookmark(ctx, account, st.ID)
			if g, ok := got.Bookmarks[st.ID]; ok != (merr == nil) || ok && g != m {
				t.Errorf("FindAccountInteractions: bookmark on %s = %+v (%t), want %+v (%v) as FindBookmark", st.ID, g, ok, m, merr)
			}
		}
	}
	var pagedFavs []content.Favourite
	var pagedMarks []content.Bookmark
	before, beforeMark := "", ""
	for range 3 {
		fp, err := r.repo.ListFavourites(ctx, account, before, 2)
		if err != nil {
			t.Fatal(err)
		}
		bp, err := r.repo.ListBookmarks(ctx, account, beforeMark, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(fp) > 2 || len(bp) > 2 {
			t.Fatalf("pages of %d favourites and %d bookmarks, want at most 2", len(fp), len(bp))
		}
		for _, f := range fp {
			pagedFavs, before = append(pagedFavs, f), f.ID
		}
		for _, b := range bp {
			pagedMarks, beforeMark = append(pagedMarks, b), b.ID
		}
	}
	slices.Reverse(favs)
	slices.Reverse(marks)
	if !reflect.DeepEqual(pagedFavs, favs) || !reflect.DeepEqual(pagedMarks, marks) {
		t.Errorf("ListFavourites, ListBookmarks pages = %+v, %+v; want newest first %+v, %+v", pagedFavs, pagedMarks, favs, marks)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, deleted, err := r.repo.DeleteFavourite(ctx, tx, account, statuses[0].ID); err != nil || !deleted || got != favs[2] {
			t.Errorf("DeleteFavourite = %+v, %v, %v; want %+v", got, deleted, err, favs[2])
		}
		if _, deleted, err := r.repo.DeleteFavourite(ctx, tx, account, statuses[0].ID); err != nil || deleted {
			t.Errorf("DeleteFavourite(again) = %v, %v; want false", deleted, err)
		}
		for i, want := range []bool{true, false} {
			if deleted, err := r.repo.DeleteBookmark(ctx, tx, account, statuses[0].ID); err != nil || deleted != want {
				t.Errorf("DeleteBookmark #%d = %v, %v; want %v", i+1, deleted, err, want)
			}
		}
		return nil
	})
	if _, err := r.repo.FindFavourite(ctx, account, statuses[0].ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindFavourite after DeleteFavourite = %v, want ErrNotFound", err)
	}
}

func contentInteractionRepoPins(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	account := r.id()
	var pins []content.Pin
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i := range content.MaxPins + 1 {
			r.se.Clock.Advance(time.Second)
			p := content.Pin{ID: r.id(), AccountID: account, StatusID: r.id(), CreatedAt: r.se.Clock.Now()}
			inserted, err := r.repo.InsertPin(ctx, tx, p)
			if err != nil {
				return err
			}
			if want := i < content.MaxPins; inserted != want {
				t.Errorf("InsertPin #%d = %v, want %v", i+1, inserted, want)
			}
			if inserted {
				pins = append(pins, p)
			}
		}
		again := pins[0]
		again.ID = r.id()
		if inserted, err := r.repo.InsertPin(ctx, tx, again); err != nil || inserted {
			t.Errorf("InsertPin(again) = %v, %v; want false", inserted, err)
		}
		if n, err := r.repo.CountPins(ctx, tx, account); err != nil || n != content.MaxPins {
			t.Errorf("CountPins = %d, %v; want %d", n, err, content.MaxPins)
		}
		if has, err := r.repo.HasPin(ctx, tx, account, pins[1].StatusID); err != nil || !has {
			t.Errorf("HasPin = %v, %v; want true", has, err)
		}
		// Freeing a slot lets one more pin in.
		for i, want := range []bool{true, false} {
			if deleted, err := r.repo.DeletePin(ctx, tx, account, pins[0].StatusID); err != nil || deleted != want {
				t.Errorf("DeletePin #%d = %v, %v; want %v", i+1, deleted, err, want)
			}
		}
		if has, err := r.repo.HasPin(ctx, tx, account, pins[0].StatusID); err != nil || has {
			t.Errorf("HasPin after DeletePin = %v, %v; want false", has, err)
		}
		p := content.Pin{ID: r.id(), AccountID: account, StatusID: r.id(), CreatedAt: r.se.Clock.Now()}
		if inserted, err := r.repo.InsertPin(ctx, tx, p); err != nil || !inserted {
			t.Errorf("InsertPin into a freed slot = %v, %v; want true", inserted, err)
		}
		pins = append(pins[1:], p)
		return nil
	})
	if got, err := r.repo.FindPin(ctx, account, pins[0].StatusID); err != nil || got != pins[0] {
		t.Errorf("FindPin = %+v, %v; want %+v", got, err, pins[0])
	}
	if _, err := r.repo.FindPin(ctx, r.id(), pins[0].StatusID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindPin(another account) = %v, want ErrNotFound", err)
	}
	pinned := make([]string, 0, len(pins)+1)
	for _, p := range pins {
		pinned = append(pinned, p.StatusID)
	}
	if got, err := r.repo.FindAccountInteractions(ctx, account, append(pinned, r.id())); err != nil || len(got.Pins) != len(pins) || got.Pins[pins[0].StatusID] != pins[0] {
		t.Errorf("FindAccountInteractions: pins = %+v, %v; want the %d pins, as FindPin", got.Pins, err, len(pins))
	}
	listed, err := r.repo.ListPins(ctx, account)
	slices.Reverse(pins)
	if err != nil || !reflect.DeepEqual(listed, pins) {
		t.Errorf("ListPins = %+v, %v; want newest first %+v", listed, err, pins)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if n, err := r.repo.DeletePinsOf(ctx, tx, account); err != nil || n != content.MaxPins {
			t.Errorf("DeletePinsOf = %d, %v; want %d", n, err, content.MaxPins)
		}
		return nil
	})
	if listed, err := r.repo.ListPins(ctx, account); err != nil || len(listed) != 0 {
		t.Errorf("ListPins after DeletePinsOf = %+v, %v; want none", listed, err)
	}
}

// contentInteractionRepoConcurrentPins pins more statuses than there are slots, concurrently: the unique
// slot per account keeps the pins within content.MaxPins on every adapter.
func contentInteractionRepoConcurrentPins(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	account := r.id()
	var wg sync.WaitGroup
	for range 2 * content.MaxPins {
		p := content.Pin{ID: r.id(), AccountID: account, StatusID: r.id(), CreatedAt: r.se.Clock.Now()}
		wg.Go(func() {
			if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				_, err := r.repo.InsertPin(ctx, tx, p)
				return err
			}); err != nil {
				t.Errorf("InsertPin = %v", err)
			}
		})
	}
	wg.Wait()
	// A pin that loses the free slot to a concurrent one stores nothing, so fewer may be stored,
	// never more; pinning one by one then fills the slots that are left.
	if listed, err := r.repo.ListPins(ctx, account); err != nil || len(listed) == 0 || len(listed) > content.MaxPins {
		t.Errorf("ListPins = %d pins, %v; want 1 to %d", len(listed), err, content.MaxPins)
	}
	for range content.MaxPins {
		p := content.Pin{ID: r.id(), AccountID: account, StatusID: r.id(), CreatedAt: r.se.Clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			_, err := r.repo.InsertPin(ctx, tx, p)
			return err
		})
	}
	if listed, err := r.repo.ListPins(ctx, account); err != nil || len(listed) != content.MaxPins {
		t.Errorf("ListPins after filling the slots = %d pins, %v; want %d", len(listed), err, content.MaxPins)
	}
}

// contentInteractionRepoConcurrentFavourites favourites one status by one account from several
// transactions at once: exactly one of them inserts the row, so the counter moves once.
func contentInteractionRepoConcurrentFavourites(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	s := r.status(t)
	account := r.id()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		inserted int
	)
	for range 6 {
		f := content.Favourite{ID: r.id(), AccountID: account, StatusID: s.ID, CreatedAt: r.se.Clock.Now()}
		wg.Go(func() {
			err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				ok, err := r.repo.InsertFavourite(ctx, tx, f)
				if err == nil && ok {
					err = r.repo.AddFavourites(ctx, tx, s.ID, 1)
				}
				if err == nil && ok {
					mu.Lock()
					inserted++
					mu.Unlock()
				}
				return err
			})
			if err != nil {
				t.Errorf("InsertFavourite = %v", err)
			}
		})
	}
	wg.Wait()
	if c, err := r.h.Statuses.FindCounters(ctx, s.ID); err != nil || c.FavouritesCount != 1 {
		t.Errorf("FindCounters = %+v, %v; want 1 favourite (%d transactions reported an insert)", c, err, inserted)
	}
}

func contentInteractionRepoReactions(t *testing.T, r *contentInteractionRepo) {
	ctx := t.Context()
	s, gone := r.status(t), r.status(t)
	alice, bob := r.id(), r.id()
	heart, star := content.ReactionEmoji{Unicode: "❤"}, content.ReactionEmoji{Unicode: "⭐"}
	react := func(account string, of content.Status, e content.ReactionEmoji) content.Reaction {
		t.Helper()
		r.se.Clock.Advance(time.Second)
		id := r.id()
		x := content.Reaction{ID: id, AccountID: account, StatusID: of.ID, Emoji: e, URI: "https://remote.example/reactions/" + id, CreatedAt: r.se.Clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			inserted, err := r.repo.InsertReaction(ctx, tx, x)
			if err != nil {
				return err
			}
			if !inserted {
				return errors.New("InsertReaction inserted nothing")
			}
			return r.repo.AddReactions(ctx, tx, of.ID, e, 1)
		})
		return x
	}
	aliceHeart := react(alice, s, heart)
	aliceStar := react(alice, s, star)
	bobHeart := react(bob, s, heart)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		again := aliceHeart
		again.ID, again.URI = r.id(), ""
		if inserted, err := r.repo.InsertReaction(ctx, tx, again); err != nil || inserted {
			t.Errorf("InsertReaction(again) = %v, %v; want false", inserted, err)
		}
		return nil
	})
	taken := content.Reaction{ID: r.id(), AccountID: bob, StatusID: s.ID, Emoji: star, URI: aliceHeart.URI, CreatedAt: r.se.Clock.Now()}
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := r.repo.InsertReaction(ctx, tx, taken)
		return err
	}); !errors.Is(err, appcontent.ErrReactionURIUsed) {
		t.Errorf("InsertReaction(a taken URI) = %v, want ErrReactionURIUsed", err)
	}

	if got, err := r.repo.FindReaction(ctx, alice, s.ID, star.Key()); err != nil || got != aliceStar {
		t.Errorf("FindReaction = %+v, %v; want %+v", got, err, aliceStar)
	}
	if got, err := r.repo.FindReactionByURI(ctx, bobHeart.URI); err != nil || got != bobHeart {
		t.Errorf("FindReactionByURI = %+v, %v; want %+v", got, err, bobHeart)
	}
	if _, err := r.repo.FindReaction(ctx, bob, s.ID, star.Key()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindReaction(none) = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.ListAccountStatusReactions(ctx, alice, s.ID); err != nil || !reflect.DeepEqual(got, []content.Reaction{aliceHeart, aliceStar}) {
		t.Errorf("ListAccountStatusReactions = %+v, %v; want oldest first", got, err)
	}
	if got, err := r.repo.ListStatusReactions(ctx, s.ID, "", "", 2); err != nil || !reflect.DeepEqual(got, []content.Reaction{bobHeart, aliceStar}) {
		t.Errorf("ListStatusReactions = %+v, %v; want the newest 2", got, err)
	}
	if got, err := r.repo.ListStatusReactions(ctx, s.ID, heart.Key(), bobHeart.ID, 10); err != nil || !reflect.DeepEqual(got, []content.Reaction{aliceHeart}) {
		t.Errorf("ListStatusReactions(heart, before bob's) = %+v, %v; want alice's heart", got, err)
	}
	counts, err := r.repo.ListReactionCounts(ctx, s.ID, 10)
	want := []content.ReactionCount{{StatusID: s.ID, Emoji: heart, Count: 2}, {StatusID: s.ID, Emoji: star, Count: 1}}
	if err != nil || !reflect.DeepEqual(counts, want) {
		t.Errorf("ListReactionCounts = %+v, %v; want %+v", counts, err, want)
	}
	if counts, err := r.repo.ListReactionCounts(ctx, s.ID, 1); err != nil || len(counts) != 1 {
		t.Errorf("ListReactionCounts(limit 1) = %+v, %v", counts, err)
	}
	other := r.status(t)
	for _, limit := range []int{10, 1} {
		single, err := r.repo.ListReactionCounts(ctx, s.ID, limit)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.repo.ListReactionCountsOf(ctx, []string{s.ID, other.ID}, limit)
		if err != nil || !reflect.DeepEqual(got[s.ID], single) || len(got[other.ID]) != 0 {
			t.Errorf("ListReactionCountsOf(limit %d) = %+v, %v; want %+v of the status alone, as ListReactionCounts", limit, got, err, single)
		}
	}
	if got, err := r.repo.FindAccountInteractions(ctx, alice, []string{s.ID, other.ID}); err != nil ||
		!reflect.DeepEqual(got.Reactions[s.ID], []content.Reaction{aliceHeart, aliceStar}) || len(got.Reactions[other.ID]) != 0 {
		t.Errorf("FindAccountInteractions: reactions = %+v, %v; want alice's two, oldest first, as ListAccountStatusReactions", got.Reactions, err)
	}

	// A deleted status gains no count; a rebuild leaves it out too.
	r.se.MustRemove(t, gone)
	goneHeart := react(alice, gone, heart)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		removed, err := r.repo.DeleteAccountStatusReactions(ctx, tx, alice, s.ID, heart.Key())
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(removed, []content.Reaction{aliceStar}) {
			t.Errorf("DeleteAccountStatusReactions(except heart) = %+v, want alice's star", removed)
		}
		if err := r.repo.AddReactions(ctx, tx, s.ID, star, -1); err != nil {
			return err
		}
		got, deleted, err := r.repo.DeleteReaction(ctx, tx, bobHeart.ID)
		if err != nil || !deleted || got != bobHeart {
			t.Errorf("DeleteReaction = %+v, %v, %v; want %+v", got, deleted, err, bobHeart)
		}
		if _, deleted, err := r.repo.DeleteReaction(ctx, tx, bobHeart.ID); err != nil || deleted {
			t.Errorf("DeleteReaction(again) = %v, %v; want false", deleted, err)
		}
		return r.repo.AddReactions(ctx, tx, s.ID, heart, -1)
	})
	want = []content.ReactionCount{{StatusID: s.ID, Emoji: heart, Count: 1}}
	if all, err := r.repo.ListAllReactionCounts(ctx); err != nil || !reflect.DeepEqual(all, want) {
		t.Errorf("ListAllReactionCounts = %+v, %v; want %+v (none of %s)", all, err, want, goneHeart.StatusID)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.DeleteStatusReactionCountsOf(ctx, tx, []string{s.ID})
	})
	if all, err := r.repo.ListAllReactionCounts(ctx); err != nil || len(all) != 0 {
		t.Errorf("reaction counts after DeleteStatusReactionCountsOf = %+v, %v; want none", all, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteStatusReactionCounts(ctx, tx, s.ID); err != nil {
			return err
		}
		return r.repo.RebuildReactionCounts(ctx, tx)
	})
	if all, err := r.repo.ListAllReactionCounts(ctx); err != nil || !reflect.DeepEqual(all, want) {
		t.Errorf("ListAllReactionCounts after the rebuild = %+v, %v; want %+v", all, err, want)
	}
}
