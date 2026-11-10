// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"cmp"
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// timelineRepoTagFollows checks the followed hashtags: inserted once, counted, listed the latest
// first, looked up, deleted, and kept by the rebuild of the projections.
func timelineRepoTagFollows(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice, bob := r.local(), r.local()
	cats, dogs := r.ids.NewID(), r.ids.NewID()
	first := r.clock.Now()
	second := first.Add(time.Minute)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, f := range []apptimeline.TagFollow{
			{AccountID: alice.id, TagID: cats, FollowedAt: first},
			{AccountID: alice.id, TagID: dogs, FollowedAt: second},
			{AccountID: bob.id, TagID: cats, FollowedAt: first},
		} {
			if inserted, err := r.repo.InsertTagFollow(ctx, tx, f); err != nil || !inserted {
				t.Errorf("InsertTagFollow(%+v) = %v, %v; want true", f, inserted, err)
			}
		}
		again := apptimeline.TagFollow{AccountID: alice.id, TagID: cats, FollowedAt: second}
		if inserted, err := r.repo.InsertTagFollow(ctx, tx, again); err != nil || inserted {
			t.Errorf("InsertTagFollow(again) = %v, %v; want false", inserted, err)
		}
		if n, err := r.repo.CountTagFollows(ctx, tx, alice.id); err != nil || n != 2 {
			t.Errorf("CountTagFollows = %d, %v; want 2", n, err)
		}
		return nil
	})
	got, err := r.repo.TagFollowsOf(ctx, alice.id)
	want := []apptimeline.TagFollow{
		{AccountID: alice.id, TagID: dogs, FollowedAt: second},
		{AccountID: alice.id, TagID: cats, FollowedAt: first},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("TagFollowsOf = %+v, %v; want %+v", got, err, want)
	}
	if ok, err := r.repo.FollowsTag(ctx, bob.id, cats); err != nil || !ok {
		t.Errorf("FollowsTag(bob, cats) = %v, %v; want true", ok, err)
	}
	if ok, err := r.repo.FollowsTag(ctx, bob.id, dogs); err != nil || ok {
		t.Errorf("FollowsTag(bob, dogs) = %v, %v; want false", ok, err)
	}

	// A rebuild keeps them: they record what the accounts did.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteProjection(ctx, tx) })
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteTagFollow(ctx, tx, alice.id, cats); err != nil {
			return err
		}
		return r.repo.DeleteTagFollow(ctx, tx, alice.id, r.ids.NewID())
	})
	all, err := r.repo.AllTagFollows(ctx)
	wantAll := []apptimeline.TagFollow{
		{AccountID: alice.id, TagID: dogs, FollowedAt: second},
		{AccountID: bob.id, TagID: cats, FollowedAt: first},
	}
	slices.SortFunc(wantAll, func(a, b apptimeline.TagFollow) int {
		if a.AccountID != b.AccountID {
			return cmp.Compare(a.AccountID, b.AccountID)
		}
		return cmp.Compare(a.TagID, b.TagID)
	})
	if err != nil || !reflect.DeepEqual(all, wantAll) {
		t.Errorf("AllTagFollows = %+v, %v; want %+v", all, err, wantAll)
	}
}

// timelineRepoHomeTags checks the hashtag branch of the home timeline (data-model §6.1): the
// public and local posts with a followed tag, each once with its followed tags, without boosts,
// the viewer's own posts, the posts of followed authors, and what the hashtag timeline leaves
// out; and the followers of an entry's tags among accounts.
func timelineRepoHomeTags(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, alice, followee, far, muted, silenced := r.local(), r.local(), r.local(), r.remote(), r.local(), r.local()
	cats, dogs, birds := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, tag := range []string{cats, dogs} {
			if _, err := r.repo.InsertTagFollow(ctx, tx, apptimeline.TagFollow{AccountID: viewer.id, TagID: tag, FollowedAt: r.clock.Now()}); err != nil {
				return err
			}
		}
		return nil
	})
	r.follow(t, viewer, followee, true)
	pub := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{cats}})
	both := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{dogs, cats, birds}})
	loc := r.post(t, alice, appcontent.VisibilityLocal, entryOpts{tags: []string{cats}, reply: true})
	r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{tags: []string{cats}})
	r.post(t, alice, appcontent.VisibilityPrivate, entryOpts{tags: []string{cats}})
	r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{birds}})
	r.post(t, viewer, appcontent.VisibilityPublic, entryOpts{tags: []string{cats}})
	r.post(t, followee, appcontent.VisibilityPublic, entryOpts{tags: []string{cats}})
	relayed := r.post(t, far, appcontent.VisibilityPublic, entryOpts{tags: []string{dogs}})
	r.boost(t, alice, relayed)
	mutedPost := r.post(t, muted, appcontent.VisibilityPublic, entryOpts{tags: []string{cats}})
	silencedPost := r.post(t, silenced, appcontent.VisibilityPublic, entryOpts{tags: []string{cats}})
	ja := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{cats}, language: "ja"})

	list := func(q apptimeline.HomeQuery) ([]apptimeline.Entry, error) {
		if q.ViewerAccountID == "" {
			q.Window = r.window(viewer)
		}
		return r.repo.ListHomeTags(ctx, q)
	}
	got, err := list(apptimeline.HomeQuery{})
	checkTimeline(t, "ListHomeTags", got, err, pub, both, loc, relayed, mutedPost, silencedPost, ja)
	for _, e := range got {
		want := []string{cats}
		switch e.ID {
		case both.ID:
			want = []string{cats, dogs}
			slices.Sort(want)
		case relayed.ID:
			want = []string{dogs}
		}
		if !slices.Equal(e.FollowedTagIDs, want) {
			t.Errorf("FollowedTagIDs of %s = %v, want %v", e.ID, e.FollowedTagIDs, want)
		}
	}
	if got, err := r.repo.ListHome(ctx, apptimeline.HomeQuery{Window: r.window(viewer)}); err != nil {
		t.Fatal(err)
	} else if slices.ContainsFunc(got, func(e apptimeline.Entry) bool { return len(e.FollowedTagIDs) > 0 }) {
		t.Errorf("ListHome has FollowedTagIDs: %+v", got)
	}

	// The read window, the cursor, the limit, and the languages, as on home.
	got, err = list(apptimeline.HomeQuery{Floor: loc.ID})
	checkTimeline(t, "ListHomeTags(floor)", got, err, loc, relayed, mutedPost, silencedPost, ja)
	w := r.window(viewer)
	w.Before, w.Limit = loc.ID, 1
	got, err = list(apptimeline.HomeQuery{Window: w})
	checkTimeline(t, "ListHomeTags(page)", got, err, both)
	w = r.window(viewer)
	w.Languages = []string{"en"}
	got, err = list(apptimeline.HomeQuery{Window: w})
	checkTimeline(t, "ListHomeTags(languages)", got, err, pub, both, loc, relayed, mutedPost, silencedPost)

	// Mutes, domain mutes, and the restrictions of the hashtag timeline.
	r.exclude(t, viewer, muted, timeline.ExcludedMute, nil)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote}); err != nil {
			return err
		}
		return r.repo.UpsertAuthorRestriction(ctx, tx, apptimeline.AuthorRestriction{AccountID: silenced.id, Restriction: timeline.RestrictPublicDiscovery})
	})
	got, err = list(apptimeline.HomeQuery{})
	checkTimeline(t, "ListHomeTags(muted and silenced)", got, err, pub, both, loc, ja)
	r.exclude(t, viewer, alice, timeline.ExcludedBlockedBy, nil)
	got, err = list(apptimeline.HomeQuery{})
	checkTimeline(t, "ListHomeTags(blocked by)", got, err)

	// Another viewer follows nothing.
	got, err = list(apptimeline.HomeQuery{Window: r.window(r.local())})
	checkTimeline(t, "ListHomeTags(no followed tags)", got, err)

	// The followers of the tags of an entry, for the streams.
	other := r.local()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.repo.InsertTagFollow(ctx, tx, apptimeline.TagFollow{AccountID: other.id, TagID: birds, FollowedAt: r.clock.Now()})
		return err
	})
	stranger := r.local()
	if got, err := r.repo.FeedTagFollowersAmong(ctx, both.ID, []string{viewer.id, other.id, stranger.id}); err != nil ||
		!reflect.DeepEqual(got, map[string]bool{viewer.id: true, other.id: true}) {
		t.Errorf("FeedTagFollowersAmong(both) = %v, %v; want viewer and other", got, err)
	}
	unlisted := r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{tags: []string{cats}})
	if got, err := r.repo.FeedTagFollowersAmong(ctx, unlisted.ID, []string{viewer.id}); err != nil || len(got) != 0 {
		t.Errorf("FeedTagFollowersAmong(unlisted) = %v, %v; want none", got, err)
	}
	if got, err := r.repo.FeedTagFollowersAmong(ctx, both.ID, nil); err != nil || len(got) != 0 {
		t.Errorf("FeedTagFollowersAmong(no accounts) = %v, %v; want none", got, err)
	}
}
