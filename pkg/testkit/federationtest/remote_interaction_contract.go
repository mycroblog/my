// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// RemoteInteractionContract verifies the repository methods that boosts, favourites, and emoji
// reactions across servers use on a database adapter, by those methods alone, the port contract of
// ADR-0153 §1: a boost is stored once per URI, found by ID and URI, retracted once, and its edge,
// one per booster and status, inserted and deleted; the live boosts are listed without the
// retracted ones; the counters of a status move with the boosts and favourites; a favourite is
// recorded once per account and status, found by its URI, and taken back; and a reaction is
// recorded once per account, status, and emoji, found by its URI, deleted by ID or all but one
// emoji, and counted per emoji, the counts derived again from the reactions; and a transaction that
// locks a poll waits for another that holds the lock, which the votes in polls rely on (ADR-0153
// §5). The use cases on top of it are RemoteInteractionFlows, which internal/flowtest/federation
// runs once on SQLite.
func RemoteInteractionContract(t *testing.T, newHarness RemoteInteractionFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *interactionRepo)
	}{
		{"RecordsAndRetractsRemoteBoosts", interactionRepoBoosts},
		{"RecordsAndTakesBackRemoteFavourites", interactionRepoFavourites},
		{"RecordsAndCountsRemoteReactions", interactionRepoReactions},
		{"LocksAPollOneTransactionAtATime", interactionRepoPollLock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			r := &interactionRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)}
			r.post = r.status(t)
			tc.run(t, r)
		})
	}
}

type interactionRepo struct {
	h     RemoteInteractionHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
	// post is a local status with counters, the object of the interactions.
	post content.Status
}

func (r *interactionRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// status inserts a local status with zero counters.
func (r *interactionRepo) status(t *testing.T) content.Status {
	t.Helper()
	id := r.ids.NewID()
	s := content.Status{
		ID: id, URI: "https://" + testkit.ContractHost + "/statuses/" + id, AuthorAccountID: r.ids.NewID(), IsLocal: true,
		Visibility: content.VisibilityPublic, PublishedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.h.Statuses.InsertStatus(ctx, tx, s), r.h.Statuses.InsertCounters(ctx, tx, content.Counters{StatusID: id}))
	})
	return s
}

func (r *interactionRepo) counters(t *testing.T) content.Counters {
	t.Helper()
	c, err := r.h.Statuses.FindCounters(t.Context(), r.post.ID)
	if err != nil {
		t.Fatalf("FindCounters: %v", err)
	}
	return c
}

func interactionRepoBoosts(t *testing.T, r *interactionRepo) {
	ctx := t.Context()
	booster := r.ids.NewID()
	boost := func() content.Boost {
		id := r.ids.NewID()
		return content.Boost{
			ID: id, URI: "https://remote.example/announces/" + id, BoosterAccountID: booster, StatusID: r.post.ID,
			Visibility: content.VisibilityPublic, BoostedAt: r.clock.Now(),
		}
	}
	first, second := boost(), boost()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Interactions.InsertBoost(ctx, tx, first); err != nil {
			return err
		}
		if ok, err := r.h.Interactions.InsertBoostEdge(ctx, tx, content.BoostEdge{BoosterAccountID: booster, StatusID: r.post.ID, BoostID: first.ID}); err != nil || !ok {
			t.Errorf("InsertBoostEdge = %v, %v; want true", ok, err)
		}
		return r.h.Interactions.AddBoosts(ctx, tx, r.post.ID, 1)
	})
	dup := second
	dup.URI = first.URI
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Interactions.InsertBoost(ctx, tx, dup) }); !errors.Is(err, appcontent.ErrBoostExists) {
		t.Errorf("InsertBoost of a stored URI = %v, want ErrBoostExists", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Interactions.InsertBoost(ctx, tx, second); err != nil {
			return err
		}
		if ok, err := r.h.Interactions.InsertBoostEdge(ctx, tx, content.BoostEdge{BoosterAccountID: booster, StatusID: r.post.ID, BoostID: second.ID}); err != nil || ok {
			t.Errorf("InsertBoostEdge of a pair with a live boost = %v, %v; want false", ok, err)
		}
		if got, err := r.h.Interactions.Boost(ctx, tx, second.ID); err != nil || got != second {
			t.Errorf("Boost = %+v, %v; want %+v", got, err, second)
		}
		if got, err := r.h.Interactions.BoostEdge(ctx, tx, booster, r.post.ID); err != nil || got.BoostID != first.ID {
			t.Errorf("BoostEdge = %+v, %v; want the first boost", got, err)
		}
		return nil
	})
	if got, err := r.h.Interactions.FindBoostByURI(ctx, first.URI); err != nil || got != first {
		t.Errorf("FindBoostByURI = %+v, %v; want %+v", got, err, first)
	}
	if c := r.counters(t); c.BoostsCount != 1 {
		t.Errorf("BoostsCount = %d, want 1", c.BoostsCount)
	}

	retraction := content.BoostRetraction{BoostID: first.ID, Origin: content.RetractedByRemote, RetractedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Interactions.InsertBoostRetraction(ctx, tx, retraction); err != nil {
			return err
		}
		if ok, err := r.h.Interactions.DeleteBoostEdge(ctx, tx, content.BoostEdge{BoosterAccountID: booster, StatusID: r.post.ID, BoostID: first.ID}); err != nil || !ok {
			t.Errorf("DeleteBoostEdge = %v, %v; want true", ok, err)
		}
		if got, err := r.h.Interactions.FindBoostRetractionTx(ctx, tx, first.ID); err != nil || got != retraction {
			t.Errorf("FindBoostRetractionTx = %+v, %v; want %+v", got, err, retraction)
		}
		return r.h.Interactions.AddBoosts(ctx, tx, r.post.ID, -1)
	})
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Interactions.InsertBoostRetraction(ctx, tx, retraction) }); !errors.Is(err, appcontent.ErrBoostRetracted) {
		t.Errorf("a second retraction = %v, want ErrBoostRetracted", err)
	}
	if _, err := r.h.Interactions.FindBoostRetraction(ctx, second.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindBoostRetraction(live) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Interactions.FindBoostEdge(ctx, booster, r.post.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindBoostEdge after DeleteBoostEdge = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Interactions.RebuildBoostEdges(ctx, tx); err != nil {
			return err
		}
		live, err := r.h.Interactions.ListLiveBoosts(ctx, tx)
		if err != nil || len(live) != 1 || live[0] != second {
			t.Errorf("ListLiveBoosts after the rebuild = %+v, %v; want only %+v", live, err, second)
		}
		return nil
	})
	if c := r.counters(t); c.BoostsCount != 0 {
		t.Errorf("BoostsCount after the retraction = %d, want 0", c.BoostsCount)
	}
}

func interactionRepoFavourites(t *testing.T, r *interactionRepo) {
	ctx := t.Context()
	account := r.ids.NewID()
	f := content.Favourite{ID: r.ids.NewID(), AccountID: account, StatusID: r.post.ID, URI: "https://remote.example/likes/1", CreatedAt: r.clock.Now()}
	for i, want := range []bool{true, false} {
		again := f
		if i > 0 {
			again.ID, again.URI = r.ids.NewID(), "https://remote.example/likes/2"
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Interactions.InsertFavourite(ctx, tx, again)
			if err == nil && ok != want {
				t.Errorf("InsertFavourite #%d = %v, want %v", i+1, ok, want)
			}
			if err == nil && ok {
				err = r.h.Interactions.AddFavourites(ctx, tx, r.post.ID, 1)
			}
			return err
		})
	}
	if got, err := r.h.Interactions.FindFavouriteByURI(ctx, f.URI); err != nil || got != f {
		t.Errorf("FindFavouriteByURI = %+v, %v; want %+v", got, err, f)
	}
	if c := r.counters(t); c.FavouritesCount != 1 {
		t.Errorf("FavouritesCount = %d, want 1", c.FavouritesCount)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, ok, err := r.h.Interactions.DeleteFavourite(ctx, tx, account, r.post.ID)
		if err != nil || !ok || got != f {
			t.Errorf("DeleteFavourite = %+v, %v, %v; want %+v", got, ok, err, f)
		}
		if _, ok, err := r.h.Interactions.DeleteFavourite(ctx, tx, account, r.post.ID); err != nil || ok {
			t.Errorf("a second DeleteFavourite = %v, %v; want false", ok, err)
		}
		return r.h.Interactions.AddFavourites(ctx, tx, r.post.ID, -1)
	})
	if _, err := r.h.Interactions.FindFavouriteByURI(ctx, f.URI); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindFavouriteByURI after DeleteFavourite = %v, want ErrNotFound", err)
	}
	if c := r.counters(t); c.FavouritesCount != 0 {
		t.Errorf("FavouritesCount after DeleteFavourite = %d, want 0", c.FavouritesCount)
	}
}

func interactionRepoReactions(t *testing.T, r *interactionRepo) {
	ctx := t.Context()
	alice, bob := r.ids.NewID(), r.ids.NewID()
	thumbs := content.ReactionEmoji{Unicode: "👍"}
	custom := content.ReactionEmoji{CustomEmojiID: r.ids.NewID()}
	react := func(account string, emoji content.ReactionEmoji, want bool) content.Reaction {
		id := r.ids.NewID()
		x := content.Reaction{ID: id, AccountID: account, StatusID: r.post.ID, Emoji: emoji, URI: "https://remote.example/reactions/" + id, CreatedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Interactions.InsertReaction(ctx, tx, x)
			if err == nil && ok != want {
				t.Errorf("InsertReaction(%s, %s) = %v, want %v", account, emoji.Key(), ok, want)
			}
			if err == nil && ok {
				err = r.h.Interactions.AddReactions(ctx, tx, r.post.ID, emoji, 1)
			}
			return err
		})
		return x
	}
	aliceThumbs := react(alice, thumbs, true)
	react(alice, thumbs, false)
	aliceCustom := react(alice, custom, true)
	bobThumbs := react(bob, thumbs, true)

	if got, err := r.h.Interactions.FindReactionByURI(ctx, aliceCustom.URI); err != nil || got != aliceCustom {
		t.Errorf("FindReactionByURI = %+v, %v; want %+v", got, err, aliceCustom)
	}
	counts := func() map[string]int64 {
		got, err := r.h.Interactions.ListReactionCounts(ctx, r.post.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int64{}
		for _, c := range got {
			out[c.Emoji.Key()] = c.Count
		}
		return out
	}
	if got := counts(); got[thumbs.Key()] != 2 || got[custom.Key()] != 1 {
		t.Errorf("ListReactionCounts = %v, want 2 of %s and 1 of %s", got, thumbs.Key(), custom.Key())
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		removed, err := r.h.Interactions.DeleteAccountStatusReactions(ctx, tx, alice, r.post.ID, custom.Key())
		if err != nil || len(removed) != 1 || removed[0] != aliceThumbs {
			t.Errorf("DeleteAccountStatusReactions(all but the custom one) = %+v, %v; want %+v", removed, err, aliceThumbs)
		}
		if err := r.h.Interactions.AddReactions(ctx, tx, r.post.ID, thumbs, -1); err != nil {
			return err
		}
		got, ok, err := r.h.Interactions.DeleteReaction(ctx, tx, bobThumbs.ID)
		if err != nil || !ok || got != bobThumbs {
			t.Errorf("DeleteReaction = %+v, %v, %v; want %+v", got, ok, err, bobThumbs)
		}
		return r.h.Interactions.AddReactions(ctx, tx, r.post.ID, thumbs, -1)
	})
	if got := counts(); len(got) != 1 || got[custom.Key()] != 1 {
		t.Errorf("ListReactionCounts after the deletions = %v, want only 1 of %s", got, custom.Key())
	}
	if left, err := r.h.Interactions.ListAccountStatusReactions(ctx, alice, r.post.ID); err != nil || !slices.Equal(left, []content.Reaction{aliceCustom}) {
		t.Errorf("ListAccountStatusReactions = %+v, %v; want %+v", left, err, aliceCustom)
	}
	before, err := r.h.Interactions.ListAllReactionCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Interactions.RebuildReactionCounts(ctx, tx) })
	if after, err := r.h.Interactions.ListAllReactionCounts(ctx); err != nil || !slices.Equal(after, before) {
		t.Errorf("ListAllReactionCounts after RebuildReactionCounts = %+v, %v; want %+v", after, err, before)
	}
}

func interactionRepoPollLock(t *testing.T, r *interactionRepo) {
	ctx := t.Context()
	poll := &content.Poll{ID: r.ids.NewID(), Options: []content.PollOption{{Position: 0, Title: "yes"}, {Position: 1, Title: "no"}}}
	rev := content.Revision{ID: r.ids.NewID(), StatusID: r.post.ID, Seq: 1, RevisedAt: r.clock.Now(), Bodies: []content.Body{{ContentHTML: "<p>poll</p>"}}, Poll: poll}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Statuses.InsertRevision(ctx, tx, rev),
			r.h.Statuses.UpsertHead(ctx, tx, content.Head{StatusID: r.post.ID, RevisionID: rev.ID, Seq: 1, LastRevisedAt: rev.RevisedAt}),
		)
	})

	var (
		mu    sync.Mutex
		order []string
	)
	note := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}
	second := make(chan error, 1)
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := r.h.Statuses.LockPoll(ctx, tx, poll.ID); err != nil {
			return err
		}
		go func() {
			second <- r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				if err := r.h.Statuses.LockPoll(ctx, tx, poll.ID); err != nil {
					return err
				}
				note("second locked")
				return nil
			})
		}()
		// Give the second transaction the time to reach the lock while this one holds it.
		time.Sleep(100 * time.Millisecond)
		note("first commits")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatalf("the second transaction: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"first commits", "second locked"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v: the second lock waits for the first transaction", order, want)
	}
}
