// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// AccountErasureContract verifies the erasure repositories of the modules on a database adapter, by
// their methods alone, the port contract of ADR-0153 §1: the deleted accounts to erase are listed
// in ID order and in pages until their erasure is recorded, once; an account loses its profile
// revisions and head; a user loses its email addresses and password credentials; the statuses of
// an author are listed in pages with whether they are deleted, and erased with their interactions,
// keeping their deletions; an account's favourites and bookmarks go in bounded batches; its follows,
// blocks, mutes, and counters go in bounded batches; its feed entries go; and its notifications go,
// as do the notifications about erased statuses. Each method leaves the rows of other accounts.
// The erasure on top of it is AccountErasureFlows, which internal/flowtest/federation runs once on
// SQLite.
func AccountErasureContract(t *testing.T, newHarness AccountErasureFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *erasureRepo)
	}{
		{"ListsAndRecordsTheAccountsToErase", erasureRepoTargets},
		{"ErasesProfilesAndUserData", erasureRepoProfiles},
		{"ErasesStatusesAndInteractions", erasureRepoContent},
		{"ErasesTheGraph", erasureRepoGraph},
		{"ErasesFeedsAndNotifications", erasureRepoFeeds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &erasureRepo{h: h, clock: clock, ids: NewIDGenerator(clock)})
		})
	}
}

type erasureRepo struct {
	h     AccountErasureHarness
	clock *Clock
	ids   *IDGenerator
}

func (r *erasureRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// account inserts a remote account and returns its ID.
func (r *erasureRepo) account(t *testing.T) string {
	t.Helper()
	id := r.ids.NewID()
	a := account.Account{ID: id, URI: "https://remote.example/users/" + id, Domain: "remote.example", ActorType: account.ActorPerson, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertAccount(ctx, tx, a) })
	return id
}

// rows requires that table has want rows whose column is value.
func (r *erasureRepo) rows(t *testing.T, table, column, value string, want int) {
	t.Helper()
	if got := r.h.Rows(t, table, column, value); got != want {
		t.Errorf("%s rows with %s = %s: %d, want %d", table, column, value, got, want)
	}
}

func erasureRepoTargets(t *testing.T, r *erasureRepo) {
	ctx := t.Context()
	state := func(id string, name account.LifecycleStateName, since time.Time) {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Accounts.UpsertLifecycleState(ctx, tx, account.LifecycleState{AccountID: id, State: name, Since: since})
		})
	}
	now := r.clock.Now()
	first, second, recent, active := r.account(t), r.account(t), r.account(t), r.account(t)
	state(first, account.StateDeleted, now.Add(-48*time.Hour))
	state(second, account.StateDeleted, now.Add(-24*time.Hour))
	state(recent, account.StateDeleted, now)
	state(active, account.StateActive, now.Add(-48*time.Hour))

	cutoff := now.Add(-time.Hour)
	targets, err := r.h.AccountErasure.ListAccountsToErase(ctx, cutoff, "", 10)
	want := []appaccount.ErasureTarget{{AccountID: first, DeletedAt: now.Add(-48 * time.Hour)}, {AccountID: second, DeletedAt: now.Add(-24 * time.Hour)}}
	if err != nil || !slices.EqualFunc(targets, want, func(a, b appaccount.ErasureTarget) bool {
		return a.AccountID == b.AccountID && a.UserID == b.UserID && a.DeletedAt.Equal(b.DeletedAt)
	}) {
		t.Errorf("ListAccountsToErase = %+v, %v; want %+v", targets, err, want)
	}
	if page, err := r.h.AccountErasure.ListAccountsToErase(ctx, cutoff, first, 1); err != nil || len(page) != 1 || page[0].AccountID != second {
		t.Errorf("the page after the first = %+v, %v; want %s", page, err, second)
	}

	for range 2 {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.AccountErasure.InsertErasure(ctx, tx, first, now)
		})
	}
	if erased, err := r.h.AccountErasure.AccountErased(ctx, first); err != nil || !erased {
		t.Errorf("AccountErased(erased) = %v, %v; want true", erased, err)
	}
	if erased, err := r.h.AccountErasure.AccountErased(ctx, second); err != nil || erased {
		t.Errorf("AccountErased(not erased) = %v, %v; want false", erased, err)
	}
	targets, err = r.h.AccountErasure.ListAccountsToErase(ctx, cutoff, "", 10)
	if err != nil || len(targets) != 1 || targets[0].AccountID != second {
		t.Errorf("ListAccountsToErase after an erasure = %+v, %v; want only %s", targets, err, second)
	}
}

func erasureRepoProfiles(t *testing.T, r *erasureRepo) {
	profile := func(id string) {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var errs []error
			for seq := 1; seq <= 2; seq++ {
				rev := account.ProfileRevision{ID: r.ids.NewID(), AccountID: id, Seq: seq, DisplayName: "name", RevisedAt: r.clock.Now()}
				errs = append(errs, r.h.Accounts.InsertProfileRevision(ctx, tx, rev))
				if seq == 2 {
					errs = append(errs, r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: id, RevisionID: rev.ID, Seq: seq}))
				}
			}
			return errors.Join(errs...)
		})
	}
	erased, kept := r.account(t), r.account(t)
	profile(erased)
	profile(kept)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.AccountErasure.EraseProfilesAndKeys(ctx, tx, erased)
	})
	r.rows(t, "account_profile_revisions", "account_id", erased, 0)
	r.rows(t, "account_profile_heads", "account_id", erased, 0)
	r.rows(t, "account_profile_revisions", "account_id", kept, 2)
	r.rows(t, "account_profile_heads", "account_id", kept, 1)

	user := func() string {
		u := identity.User{ID: r.ids.NewID(), CreatedAt: r.clock.Now()}
		email := identity.EmailAddress{ID: r.ids.NewID(), UserID: u.ID, Address: u.ID + "@example.com", AddressNormalized: u.ID + "@example.com", AddedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(
				r.h.Identity.InsertUser(ctx, tx, u),
				r.h.Identity.InsertEmailAddress(ctx, tx, email),
				r.h.Identity.InsertPrimaryEmail(ctx, tx, email),
				r.h.Identity.InsertPasswordCredential(ctx, tx, identity.PasswordCredential{ID: r.ids.NewID(), UserID: u.ID, Hash: "$argon2id$hash", SetAt: r.clock.Now()}),
			)
		})
		return u.ID
	}
	erasedUser, keptUser := user(), user()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.IdentityErasure.EraseUserData(ctx, tx, erasedUser); err != nil {
			return err
		}
		if ids, err := r.h.IdentityErasure.ListOwnedOAuthClientIDs(ctx, tx, erasedUser); err != nil || len(ids) != 0 {
			t.Errorf("ListOwnedOAuthClientIDs(a user without clients) = %v, %v; want none", ids, err)
		}
		return r.h.IdentityErasure.BlankOwnedOAuthClientDetails(ctx, tx, erasedUser)
	})
	for _, table := range []string{"user_email_addresses", "user_primary_emails", "password_credentials"} {
		r.rows(t, table, "user_id", erasedUser, 0)
		r.rows(t, table, "user_id", keptUser, 1)
	}
	r.rows(t, "users", "id", erasedUser, 1)
}

func erasureRepoContent(t *testing.T, r *erasureRepo) {
	ctx := t.Context()
	author, other := r.account(t), r.account(t)
	post := func(by string) content.Status {
		id := r.ids.NewID()
		s := content.Status{ID: id, URI: "https://remote.example/notes/" + id, AuthorAccountID: by, Visibility: content.VisibilityPublic, PublishedAt: r.clock.Now()}
		rev := content.Revision{ID: r.ids.NewID(), StatusID: id, Seq: 1, RevisedAt: s.PublishedAt, Bodies: []content.Body{{ContentHTML: "<p>hi</p>"}}}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(
				r.h.Statuses.InsertStatus(ctx, tx, s),
				r.h.Statuses.InsertRevision(ctx, tx, rev),
				r.h.Statuses.UpsertHead(ctx, tx, content.Head{StatusID: id, RevisionID: rev.ID, Seq: 1, LastRevisedAt: rev.RevisedAt}),
			)
		})
		return s
	}
	first, second, third := post(author), post(author), post(author)
	others := post(other)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.InsertDeletion(ctx, tx, content.Deletion{
			StatusID: second.ID, URI: second.URI, AuthorAccountID: author, Origin: content.DeletedByAccountDeletion, DeletedAt: r.clock.Now(),
		})
	})

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		page, err := r.h.ContentErasure.ListAuthorStatusesToErase(ctx, tx, author, "", 2)
		want := []appcontent.ErasableStatus{{Status: first}, {Status: second, Deleted: true}}
		if err != nil || !slices.EqualFunc(page, want, equalErasable) {
			t.Errorf("ListAuthorStatusesToErase = %+v, %v; want %+v", page, err, want)
		}
		page, err = r.h.ContentErasure.ListAuthorStatusesToErase(ctx, tx, author, second.ID, 2)
		if want := []appcontent.ErasableStatus{{Status: third}}; err != nil || !slices.EqualFunc(page, want, equalErasable) {
			t.Errorf("the page after the second = %+v, %v; want %+v", page, err, want)
		}
		return nil
	})

	// The other account favourites a post of the author and one of its own.
	for _, s := range []content.Status{first, others} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			id := r.ids.NewID()
			_, err := r.h.Interactions.InsertFavourite(ctx, tx, content.Favourite{
				ID: id, AccountID: other, StatusID: s.ID, URI: "https://remote.example/likes/" + id, CreatedAt: r.clock.Now(),
			})
			return err
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.ContentErasure.EraseStatuses(ctx, tx, []string{second.ID})
	})
	if _, err := r.h.Statuses.FindStatus(ctx, second.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindStatus(erased) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Statuses.FindDeletion(ctx, second.ID); err != nil {
		t.Errorf("FindDeletion(erased) = %v, want the deletion kept", err)
	}

	for i, want := range []int64{1, 1, 0} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			n, err := r.h.ContentErasure.EraseInteractionsBy(ctx, tx, other, 1)
			if err == nil && n != want {
				t.Errorf("EraseInteractionsBy #%d = %d, want %d", i+1, n, want)
			}
			return err
		})
	}
	r.rows(t, "favourites", "account_id", other, 0)
	if _, err := r.h.Statuses.FindStatus(ctx, others.ID); err != nil {
		t.Errorf("FindStatus(the other account's) = %v, want it kept", err)
	}
}

func equalErasable(a, b appcontent.ErasableStatus) bool {
	return a.Deleted == b.Deleted && a.Status.ID == b.Status.ID && a.Status.URI == b.Status.URI &&
		a.Status.AuthorAccountID == b.Status.AuthorAccountID && a.Status.PublishedAt.Equal(b.Status.PublishedAt)
}

func erasureRepoGraph(t *testing.T, r *erasureRepo) {
	ctx := t.Context()
	erased, a, b, c := r.account(t), r.account(t), r.account(t), r.account(t)
	follow := func(follower, followee string) {
		f := graph.Follow{ID: r.ids.NewID(), URI: "https://remote.example/follows/" + r.ids.NewID(), FollowerAccountID: follower, FolloweeAccountID: followee, RequestedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Graph.InsertFollow(ctx, tx, f); err != nil {
				return err
			}
			_, err := r.h.Graph.InsertEdge(ctx, tx, graph.Edge{FollowerAccountID: follower, FolloweeAccountID: followee, FollowID: f.ID, State: graph.StateActive, Since: r.clock.Now()})
			return err
		})
	}
	follow(erased, a)
	follow(b, erased)
	follow(a, b)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var errs []error
		for _, pair := range [][2]string{{erased, a}, {c, erased}, {a, c}} {
			blk := graph.Block{ID: r.ids.NewID(), BlockerAccountID: pair[0], BlockedAccountID: pair[1], CreatedAt: r.clock.Now()}
			_, err := r.h.Graph.InsertBlockEdge(ctx, tx, graph.BlockEdge{BlockerAccountID: pair[0], BlockedAccountID: pair[1], BlockID: blk.ID})
			errs = append(errs, r.h.Graph.InsertBlock(ctx, tx, blk), err)
		}
		for _, pair := range [][2]string{{erased, a}, {erased, b}, {c, erased}, {a, b}} {
			errs = append(errs, r.h.Graph.UpsertMute(ctx, tx, graph.Mute{AccountID: pair[0], TargetAccountID: pair[1], CreatedAt: r.clock.Now()}))
		}
		errs = append(errs,
			r.h.Graph.UpsertCounters(ctx, tx, graph.Counters{AccountID: erased, FollowersCount: 1, FollowingCount: 1}),
			r.h.Graph.UpsertCounters(ctx, tx, graph.Counters{AccountID: a, FollowersCount: 1, FollowingCount: 1}))
		return errors.Join(errs...)
	})

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if edges, err := r.h.GraphErasure.ListEdgesToErase(ctx, tx, erased, 10); err != nil || len(edges) != 2 {
			t.Errorf("ListEdgesToErase = %+v, %v; want the 2 follows into and out of the account", edges, err)
		}
		if edges, err := r.h.GraphErasure.ListEdgesToErase(ctx, tx, erased, 1); err != nil || len(edges) != 1 {
			t.Errorf("ListEdgesToErase(limit 1) = %+v, %v; want 1", edges, err)
		}
		return nil
	})
	type erase struct {
		name string
		fn   func(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error)
		want []int64
	}
	for _, e := range []erase{
		{"EraseFollowsOf", r.h.GraphErasure.EraseFollowsOf, []int64{1, 1, 0}},
		{"EraseBlocksOf", r.h.GraphErasure.EraseBlocksOf, []int64{1, 1, 0}},
		// Two mutes by the account and one of it: the more of the two counts per call.
		{"EraseMutesOf", r.h.GraphErasure.EraseMutesOf, []int64{1, 1, 0}},
	} {
		for i, want := range e.want {
			r.tx(t, func(ctx context.Context, tx uow.Tx) error {
				n, err := e.fn(ctx, tx, erased, 1)
				if err == nil && n != want {
					t.Errorf("%s #%d = %d, want %d", e.name, i+1, n, want)
				}
				return err
			})
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.GraphErasure.EraseCountersOf(ctx, tx, erased) })

	edges, err := r.h.Graph.ListEdges(ctx)
	if err != nil || len(edges) != 1 || edges[0].FollowerAccountID != a || edges[0].FolloweeAccountID != b {
		t.Errorf("ListEdges after the erasure = %+v, %v; want only %s -> %s", edges, err, a, b)
	}
	blocks, err := r.h.Graph.ListBlockEdges(ctx)
	if err != nil || len(blocks) != 1 || blocks[0].BlockerAccountID != a {
		t.Errorf("ListBlockEdges after the erasure = %+v, %v; want only the block by %s", blocks, err, a)
	}
	var mutes []graph.Mute
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		mutes, err = r.h.Graph.ListAllMutes(ctx, tx)
		return err
	})
	if len(mutes) != 1 || mutes[0].AccountID != a {
		t.Errorf("ListAllMutes after the erasure = %+v; want only the mute by %s", mutes, a)
	}
	counters, err := r.h.Graph.ListCounters(ctx)
	if err != nil || len(counters) != 1 || counters[0].AccountID != a {
		t.Errorf("ListCounters after the erasure = %+v, %v; want only %s", counters, err, a)
	}
}

func erasureRepoFeeds(t *testing.T, r *erasureRepo) {
	erased, other := r.account(t), r.account(t)
	entry := func(author string) apptimeline.Entry {
		e := apptimeline.Entry{
			ID: r.ids.NewID(), Kind: timeline.KindPost, StatusID: r.ids.NewID(), AuthorAccountID: author, AuthorDomain: "remote.example",
			OriginalAuthorAccountID: author, OriginalAuthorDomain: "remote.example", Visibility: content.VisibilityPublic, RevisionSeq: 1,
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			_, err := r.h.Timeline.InsertEntry(ctx, tx, e)
			return err
		})
		return e
	}
	entry(erased)
	entry(erased)
	entry(other)
	for i, want := range []int64{1, 1, 0} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			n, err := r.h.TimelineErasure.EraseFeedsOf(ctx, tx, erased, 1)
			if err == nil && n != want {
				t.Errorf("EraseFeedsOf #%d = %d, want %d", i+1, n, want)
			}
			return err
		})
	}
	r.rows(t, "feed_entries", "author_account_id", erased, 0)
	r.rows(t, "feed_entries", "author_account_id", other, 1)

	notify := func(recipient, source, statusID string) {
		n := notification.Notification{ID: r.ids.NewID(), RecipientAccountID: recipient, Type: notification.TypeFavourite, SourceAccountID: source, StatusID: statusID, CreatedAt: r.clock.Now()}
		if statusID == "" {
			n.Type = notification.TypeFollow
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			_, err := r.h.Notifications.InsertNotification(ctx, tx, n)
			return err
		})
	}
	third := r.account(t)
	erasedStatus := r.ids.NewID()
	notify(erased, other, "")
	notify(other, erased, "")
	notify(other, third, erasedStatus)
	notify(third, other, "")
	for i, want := range []int64{1, 1, 0} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			n, err := r.h.NotificationErasure.EraseNotificationsOf(ctx, tx, erased, "", 1)
			if err == nil && n != want {
				t.Errorf("EraseNotificationsOf #%d = %d, want %d", i+1, n, want)
			}
			return err
		})
	}
	r.rows(t, "notifications", "recipient_account_id", erased, 0)
	r.rows(t, "notifications", "source_account_id", erased, 0)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.NotificationErasure.EraseStatusNotifications(ctx, tx, []string{erasedStatus})
	})
	r.rows(t, "notifications", "status_id", erasedStatus, 0)
	r.rows(t, "notifications", "recipient_account_id", third, 1)
}
