// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/pkg/testkit"
)

// TimelineContract verifies the timeline repository of a database adapter by its methods alone,
// the port contract of ADR-0153 §1: the entries with their revisions, direct recipients, and tags,
// deleted alone or with their status; tag_usage, which counts the posts that count once each and
// reads their latest revision; the follow edges, exclusions, domain mutes, and restrictions, with
// their expiry and the subdomains of a domain; the home, local, federated, hashtag, profile, and
// list timelines with their filters, languages, read window, and keyset paging; the facts of a
// page and the muted domains; the lists with their members; and the projections emptied for a
// rebuild. The timeline use cases and projectors on top of it are TimelineFlows, which
// internal/flowtest/timeline runs once on SQLite.
func TimelineContract(t *testing.T, newHarness TimelineFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *timelineRepo)
	}{
		{"InsertsUpdatesAndDeletesEntries", timelineRepoEntries},
		{"CountsTagUsage", timelineRepoTagUsage},
		{"ListsTheHomeTimeline", timelineRepoHome},
		{"ListsTheLocalAndFederatedTimelines", timelineRepoPublic},
		{"ListsTheHashtagTimeline", timelineRepoTag},
		{"ListsTheProfileTimeline", timelineRepoProfile},
		{"FiltersLanguagesAndPages", timelineRepoLanguagesAndPages},
		{"AnswersFactsAndMutedDomains", timelineRepoFacts},
		{"KeepsListsAndMembers", timelineRepoLists},
		{"ListsTheListTimeline", timelineRepoListTimeline},
		{"EmptiesTheProjectionsForARebuild", timelineRepoRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &timelineRepo{uow: h.UnitOfWork, repo: h.Timeline, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

const timelineRepoRemote = "remote.example"

type timelineRepo struct {
	uow   uow.UnitOfWork
	repo  apptimeline.Repository
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *timelineRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.uow.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// author is an account with its domain ("" for a local one); the feed tables reference no account.
type timelineAuthor struct{ id, domain string }

func (r *timelineRepo) local() timelineAuthor { return timelineAuthor{id: r.ids.NewID()} }
func (r *timelineRepo) remote() timelineAuthor {
	return timelineAuthor{id: r.ids.NewID(), domain: timelineRepoRemote}
}

type entryOpts struct {
	language string
	reply    bool
	tags     []string
	direct   []string
}

// post inserts the post entry of a new status by a, with its tags and direct recipients.
func (r *timelineRepo) post(t *testing.T, a timelineAuthor, v appcontent.Visibility, o entryOpts) apptimeline.Entry {
	t.Helper()
	r.clock.Advance(time.Second)
	if o.language == "" {
		o.language = "en"
	}
	id := r.ids.NewID()
	e := apptimeline.Entry{
		ID: id, Kind: timeline.KindPost, StatusID: id, AuthorAccountID: a.id, AuthorDomain: a.domain,
		OriginalAuthorAccountID: a.id, OriginalAuthorDomain: a.domain, Visibility: v, IsLocal: a.domain == "",
		Language: o.language, RevisionSeq: 1, IsReply: o.reply,
	}
	r.insert(t, e, o)
	return e
}

// boost inserts the entry of a boost of of by a.
func (r *timelineRepo) boost(t *testing.T, a timelineAuthor, of apptimeline.Entry) apptimeline.Entry {
	t.Helper()
	r.clock.Advance(time.Second)
	e := of
	e.ID, e.Kind, e.AuthorAccountID, e.AuthorDomain, e.IsLocal, e.IsReply = r.ids.NewID(), timeline.KindBoost, a.id, a.domain, a.domain == "", false
	r.insert(t, e, entryOpts{})
	return e
}

func (r *timelineRepo) insert(t *testing.T, e apptimeline.Entry, o entryOpts) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		inserted, err := r.repo.InsertEntry(ctx, tx, e)
		if err != nil {
			return err
		}
		if !inserted {
			return errors.New("InsertEntry inserted nothing")
		}
		if len(o.direct) > 0 {
			if err := r.repo.ReplaceDirectRecipients(ctx, tx, e.ID, o.direct); err != nil {
				return err
			}
		}
		if len(o.tags) > 0 {
			return r.repo.ReplaceTags(ctx, tx, e, o.tags, r.clock.Now())
		}
		return nil
	})
}

func (r *timelineRepo) follow(t *testing.T, follower, followee timelineAuthor, showBoosts bool) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertFollowEdge(ctx, tx, apptimeline.FollowEdge{
			FollowerAccountID: follower.id, FolloweeAccountID: followee.id, ShowBoosts: showBoosts,
		})
	})
}

func (r *timelineRepo) exclude(t *testing.T, viewer, target timelineAuthor, reason timeline.ExclusionReason, expires *time.Time) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertExclusion(ctx, tx, apptimeline.Exclusion{
			ViewerAccountID: viewer.id, TargetAccountID: target.id, Reason: reason, ExpiresAt: expires,
		})
	})
}

func (r *timelineRepo) window(viewer timelineAuthor) apptimeline.Window {
	return apptimeline.Window{ViewerAccountID: viewer.id, Limit: 100, Now: r.clock.Now()}
}

// timelineIDsOf returns the IDs of entries.
func timelineIDsOf(entries []apptimeline.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

// timelineNewestFirst returns the IDs of entries in the order the timelines list them.
func timelineNewestFirst(entries ...apptimeline.Entry) []string {
	ids := timelineIDsOf(entries)
	slices.Sort(ids)
	slices.Reverse(ids)
	return ids
}

func checkTimeline(t *testing.T, name string, got []apptimeline.Entry, err error, want ...apptimeline.Entry) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s = %v", name, err)
	}
	if g, w := timelineIDsOf(got), timelineNewestFirst(want...); !slices.Equal(g, w) {
		t.Errorf("%s = %v, want %v", name, g, w)
	}
}

func timelineRepoEntries(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice, bob := r.local(), r.remote()
	p := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{})
	b := r.boost(t, bob, p)
	other := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if inserted, err := r.repo.InsertEntry(ctx, tx, p); err != nil || inserted {
			t.Errorf("InsertEntry(again) = %v, %v; want false", inserted, err)
		}
		if got, err := r.repo.Entry(ctx, tx, b.ID); err != nil || got != b {
			t.Errorf("Entry(boost) = %+v, %v; want %+v", got, err, b)
		}
		if _, err := r.repo.Entry(ctx, tx, r.ids.NewID()); !errors.Is(err, apptimeline.ErrNotFound) {
			t.Errorf("Entry(unknown) = %v, want ErrNotFound", err)
		}
		// A revision moves the post and its boosts, and an older one changes nothing.
		if err := r.repo.UpdateRevision(ctx, tx, p.StatusID, "ja", 3); err != nil {
			return err
		}
		return r.repo.UpdateRevision(ctx, tx, p.StatusID, "fr", 2)
	})
	snap, err := r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p.Language, p.RevisionSeq, b.Language, b.RevisionSeq = "ja", 3, "ja", 3
	if want := []apptimeline.Entry{p, b, other}; !reflect.DeepEqual(snap.Entries, want) {
		t.Errorf("Snapshot entries = %+v, want %+v", snap.Entries, want)
	}

	// Direct recipients are replaced as a whole.
	d := r.post(t, alice, appcontent.VisibilityDirect, entryOpts{direct: []string{bob.id}})
	carol := r.local()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.ReplaceDirectRecipients(ctx, tx, d.ID, []string{carol.id})
	})
	if snap, err := r.repo.Snapshot(ctx); err != nil ||
		!reflect.DeepEqual(snap.DirectRecipients, []apptimeline.DirectRecipient{{EntryID: d.ID, RecipientAccountID: carol.id}}) {
		t.Errorf("Snapshot direct recipients = %+v, %v; want only carol", snap.DirectRecipients, err)
	}

	// DeleteEntry deletes one entry with its recipients; DeleteStatusEntries the post and its boosts.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteEntry(ctx, tx, d.ID); err != nil {
			return err
		}
		return r.repo.DeleteStatusEntries(ctx, tx, p.StatusID)
	})
	snap, err = r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snap.Entries, []apptimeline.Entry{other}) || len(snap.DirectRecipients) != 0 {
		t.Errorf("Snapshot after the deletions = %+v, want only the other post", snap)
	}
}

func timelineRepoTagUsage(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice := r.local()
	tagA, tagB, tagC := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	first := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{tagA, tagB, tagA}})
	firstAt := r.clock.Now()
	second := r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{tags: []string{tagA}})
	secondAt := r.clock.Now()
	// The tags of private and direct posts do not count.
	r.post(t, alice, appcontent.VisibilityPrivate, entryOpts{tags: []string{tagC}})
	r.post(t, alice, appcontent.VisibilityDirect, entryOpts{tags: []string{tagC}})

	usage := func() []apptimeline.TagUsage {
		t.Helper()
		got, err := r.repo.TagUsageOf(ctx, []string{tagC, tagB, tagA})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	want := []apptimeline.TagUsage{
		{TagID: tagA, StatusesCount: 2, LastUsedAt: secondAt},
		{TagID: tagB, StatusesCount: 1, LastUsedAt: firstAt},
	}
	if got := usage(); !reflect.DeepEqual(got, want) {
		t.Errorf("TagUsageOf = %+v, want %+v", got, want)
	}

	// Revising the second post's tags moves its count from tagA to tagC and reads tagA's latest
	// use again.
	r.clock.Advance(time.Second)
	revisedAt := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.ReplaceTags(ctx, tx, second, []string{tagC}, revisedAt)
	})
	want = []apptimeline.TagUsage{
		{TagID: tagA, StatusesCount: 1, LastUsedAt: firstAt},
		{TagID: tagB, StatusesCount: 1, LastUsedAt: firstAt},
		{TagID: tagC, StatusesCount: 1, LastUsedAt: revisedAt},
	}
	if got := usage(); !reflect.DeepEqual(got, want) {
		t.Errorf("TagUsageOf after ReplaceTags = %+v, want %+v", got, want)
	}
	snap, err := r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var secondTags []string
	for _, te := range snap.TagEntries {
		if te.EntryID == second.ID {
			secondTags = append(secondTags, te.TagID)
			if te.AuthorAccountID != alice.id || te.Visibility != appcontent.VisibilityUnlisted || !te.IsLocal || !te.RevisedAt.Equal(revisedAt) {
				t.Errorf("tag entry = %+v", te)
			}
		}
	}
	if !slices.Equal(secondTags, []string{tagC}) {
		t.Errorf("the tags of the revised post = %v, want %v", secondTags, []string{tagC})
	}

	// Deleting the posts takes their tags out; a tag that drops to zero has no usage.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteEntry(ctx, tx, first.ID); err != nil {
			return err
		}
		return r.repo.DeleteStatusEntries(ctx, tx, second.StatusID)
	})
	if got := usage(); len(got) != 0 {
		t.Errorf("TagUsageOf after the deletions = %+v, want none", got)
	}
}

func timelineRepoHome(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, followed, quiet, stranger, far := r.local(), r.local(), r.local(), r.local(), r.remote()
	r.follow(t, viewer, followed, true)
	r.follow(t, viewer, quiet, false)
	r.follow(t, viewer, far, true)
	// The followers of followed among some accounts: the edges of the feed, whatever show_boosts.
	if got, err := r.repo.FeedFollowersAmong(ctx, followed.id, []string{viewer.id, stranger.id, quiet.id}); err != nil || !maps.Equal(got, map[string]bool{viewer.id: true}) {
		t.Errorf("FeedFollowersAmong(followed) = %v, %v; want the viewer alone", got, err)
	}
	if got, err := r.repo.FeedFollowersAmong(ctx, quiet.id, []string{viewer.id}); err != nil || !got[viewer.id] {
		t.Errorf("FeedFollowersAmong(quiet) = %v, %v; want the viewer", got, err)
	}

	own := r.post(t, viewer, appcontent.VisibilityPrivate, entryOpts{})
	fromFollowed := r.post(t, followed, appcontent.VisibilityUnlisted, entryOpts{})
	fromQuiet := r.post(t, quiet, appcontent.VisibilityPublic, entryOpts{})
	strangers := r.post(t, stranger, appcontent.VisibilityPublic, entryOpts{})
	ownBoost := r.boost(t, viewer, strangers)
	followedBoost := r.boost(t, followed, strangers)
	r.boost(t, quiet, strangers) // hidden: the follow does not show boosts
	r.boost(t, stranger, fromFollowed)
	toViewer := r.post(t, followed, appcontent.VisibilityDirect, entryOpts{direct: []string{viewer.id}})
	r.post(t, followed, appcontent.VisibilityDirect, entryOpts{direct: []string{stranger.id}})
	ownDirect := r.post(t, viewer, appcontent.VisibilityDirect, entryOpts{direct: []string{stranger.id}})
	farPost := r.post(t, far, appcontent.VisibilityPublic, entryOpts{})
	home := func() ([]apptimeline.Entry, error) {
		return r.repo.ListHome(ctx, apptimeline.HomeQuery{Window: r.window(viewer)})
	}
	all := []apptimeline.Entry{own, fromFollowed, fromQuiet, ownBoost, followedBoost, toViewer, ownDirect, farPost}
	got, err := home()
	checkTimeline(t, "ListHome", got, err, all...)

	// The read window keeps the entries at or above the floor.
	floored, err := r.repo.ListHome(ctx, apptimeline.HomeQuery{Window: r.window(viewer), Floor: toViewer.ID})
	checkTimeline(t, "ListHome(floor)", floored, err, toViewer, ownDirect, farPost)

	// A boost of a post on a muted domain leaves home, its posts stay.
	farBoost := r.boost(t, followed, farPost)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote})
	})
	got, err = home()
	checkTimeline(t, "ListHome(domain mute)", got, err, all...)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.DeleteDomainMute(ctx, tx, viewer.id, timelineRepoRemote)
	})
	got, err = home()
	checkTimeline(t, "ListHome(domain unmuted)", got, err, append(all, farBoost)...)
	all = append(all, farBoost)

	// An exclusion of the original author hides the boosts of its posts, until it expires.
	expires := r.clock.Now().Add(time.Minute)
	r.exclude(t, viewer, stranger, timeline.ExcludedMute, &expires)
	got, err = home()
	checkTimeline(t, "ListHome(mute)", got, err, own, fromFollowed, fromQuiet, toViewer, ownDirect, farPost, farBoost)
	r.clock.Advance(time.Minute)
	got, err = home()
	checkTimeline(t, "ListHome(expired mute)", got, err, all...)

	// A restriction of profile_view on a domain covers its subdomains.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertDomainRestriction(ctx, tx, apptimeline.DomainRestriction{Domain: "example", Restriction: timeline.RestrictProfileView})
	})
	got, err = home()
	checkTimeline(t, "ListHome(domain restriction)", got, err, own, fromFollowed, fromQuiet, ownBoost, followedBoost, toViewer, ownDirect)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteDomainRestriction(ctx, tx, "example", timeline.RestrictProfileView); err != nil {
			return err
		}
		return r.repo.UpsertAuthorRestriction(ctx, tx, apptimeline.AuthorRestriction{AccountID: followed.id, Restriction: timeline.RestrictProfileView})
	})
	got, err = home()
	checkTimeline(t, "ListHome(author restriction)", got, err, own, fromQuiet, ownBoost, ownDirect, farPost)

	// Without the follow edge, followed's posts leave home.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteAuthorRestriction(ctx, tx, followed.id, timeline.RestrictProfileView); err != nil {
			return err
		}
		return r.repo.DeleteFollowEdge(ctx, tx, viewer.id, followed.id)
	})
	got, err = home()
	checkTimeline(t, "ListHome(unfollowed)", got, err, own, fromQuiet, ownBoost, ownDirect, farPost)
}

func timelineRepoPublic(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, alice, bob, far, sub := r.local(), r.local(), r.local(), r.remote(), timelineAuthor{id: r.ids.NewID(), domain: "a." + timelineRepoRemote}
	pub := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{})
	loc := r.post(t, alice, appcontent.VisibilityLocal, entryOpts{})
	r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{})
	r.post(t, alice, appcontent.VisibilityPrivate, entryOpts{})
	r.boost(t, bob, pub)
	bobs := r.post(t, bob, appcontent.VisibilityPublic, entryOpts{})
	farPub := r.post(t, far, appcontent.VisibilityPublic, entryOpts{})
	subPub := r.post(t, sub, appcontent.VisibilityPublic, entryOpts{})

	local := func(includeLocal bool) ([]apptimeline.Entry, error) {
		return r.repo.ListLocal(ctx, apptimeline.LocalQuery{Window: r.window(viewer), IncludeLocal: includeLocal})
	}
	public := func() ([]apptimeline.Entry, error) {
		return r.repo.ListPublic(ctx, apptimeline.PublicQuery{Window: r.window(viewer)})
	}
	got, err := local(true)
	checkTimeline(t, "ListLocal", got, err, pub, loc, bobs)
	got, err = local(false)
	checkTimeline(t, "ListLocal(public only)", got, err, pub, bobs)
	got, err = r.repo.ListLocal(ctx, apptimeline.LocalQuery{Window: r.window(viewer), IncludeLocal: true, Floor: loc.ID})
	checkTimeline(t, "ListLocal(floor)", got, err, loc, bobs)
	got, err = public()
	checkTimeline(t, "ListPublic", got, err, pub, bobs, farPub, subPub)
	got, err = r.repo.ListPublic(ctx, apptimeline.PublicQuery{Window: r.window(viewer), Floor: farPub.ID})
	checkTimeline(t, "ListPublic(floor)", got, err, farPub, subPub)

	// The viewer's exclusions and domain mutes (with subdomains) and the discovery restrictions.
	r.exclude(t, viewer, bob, timeline.ExcludedBlockedBy, nil)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote}); err != nil {
			return err
		}
		return r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote})
	})
	got, err = local(true)
	checkTimeline(t, "ListLocal(excluded)", got, err, pub, loc)
	got, err = public()
	checkTimeline(t, "ListPublic(excluded and muted)", got, err, pub)
	// Another viewer has none of them.
	if got, err := r.repo.ListPublic(ctx, apptimeline.PublicQuery{Window: r.window(r.local())}); err != nil || len(got) != 4 {
		t.Errorf("ListPublic(another viewer) = %v, %v; want 4 entries", timelineIDsOf(got), err)
	}
	until := r.clock.Now().Add(time.Minute)
	for _, restriction := range []timeline.Restriction{timeline.RestrictPublicDiscovery, timeline.RestrictVisibilityCeilingPrivate, timeline.RestrictProfileView} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.repo.UpsertAuthorRestriction(ctx, tx, apptimeline.AuthorRestriction{AccountID: alice.id, Restriction: restriction, EffectiveUntil: &until})
		})
		got, err = local(true)
		checkTimeline(t, "ListLocal("+string(restriction)+")", got, err)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.repo.DeleteAuthorRestriction(ctx, tx, alice.id, restriction)
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertAuthorRestriction(ctx, tx, apptimeline.AuthorRestriction{AccountID: alice.id, Restriction: timeline.RestrictPublicDiscovery, EffectiveUntil: &until})
	})
	r.clock.Advance(time.Minute)
	got, err = local(true)
	checkTimeline(t, "ListLocal(expired restriction)", got, err, pub, loc)

	// A domain kept from discovery takes its subdomains off the federated timeline.
	other := r.local()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertDomainRestriction(ctx, tx, apptimeline.DomainRestriction{Domain: timelineRepoRemote, Restriction: timeline.RestrictPublicDiscovery})
	})
	got, err = r.repo.ListPublic(ctx, apptimeline.PublicQuery{Window: r.window(other)})
	checkTimeline(t, "ListPublic(domain restriction)", got, err, pub, bobs)
}

func timelineRepoTag(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, alice, far := r.local(), r.local(), r.remote()
	tag, other := r.ids.NewID(), r.ids.NewID()
	pub := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{tag}})
	loc := r.post(t, alice, appcontent.VisibilityLocal, entryOpts{tags: []string{tag}})
	r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{tags: []string{tag}})
	r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{other}})
	farPub := r.post(t, far, appcontent.VisibilityPublic, entryOpts{tags: []string{tag}})
	list := func(v timelineAuthor, includeLocal bool) ([]apptimeline.Entry, error) {
		return r.repo.ListTag(ctx, apptimeline.TagQuery{Window: r.window(v), TagID: tag, IncludeLocal: includeLocal})
	}
	got, err := list(viewer, true)
	checkTimeline(t, "ListTag", got, err, pub, loc, farPub)
	got, err = list(viewer, false)
	checkTimeline(t, "ListTag(public only)", got, err, pub, farPub)
	page, err := r.repo.ListTag(ctx, apptimeline.TagQuery{Window: apptimeline.Window{ViewerAccountID: viewer.id, Limit: 1, Before: farPub.ID, Now: r.clock.Now()}, TagID: tag, IncludeLocal: true})
	checkTimeline(t, "ListTag(page)", page, err, loc)

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote})
	})
	got, err = list(viewer, true)
	checkTimeline(t, "ListTag(domain mute)", got, err, pub, loc)
	r.exclude(t, viewer, alice, timeline.ExcludedBlock, nil)
	got, err = list(viewer, true)
	checkTimeline(t, "ListTag(block)", got, err)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertAuthorRestriction(ctx, tx, apptimeline.AuthorRestriction{AccountID: alice.id, Restriction: timeline.RestrictPublicDiscovery})
	})
	got, err = list(r.local(), true)
	checkTimeline(t, "ListTag(restricted author)", got, err, farPub)
}

func timelineRepoProfile(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice, viewer, stranger, far := r.local(), r.local(), r.local(), r.remote()
	pub := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{})
	loc := r.post(t, alice, appcontent.VisibilityLocal, entryOpts{})
	unl := r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{})
	priv := r.post(t, alice, appcontent.VisibilityPrivate, entryOpts{})
	toViewer := r.post(t, alice, appcontent.VisibilityDirect, entryOpts{direct: []string{viewer.id}})
	toStranger := r.post(t, alice, appcontent.VisibilityDirect, entryOpts{direct: []string{stranger.id}})
	reply := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{reply: true})
	boostOfFar := r.boost(t, alice, r.post(t, far, appcontent.VisibilityPublic, entryOpts{}))
	boostOfStranger := r.boost(t, alice, r.post(t, stranger, appcontent.VisibilityPublic, entryOpts{}))
	r.post(t, viewer, appcontent.VisibilityPublic, entryOpts{})

	everyVisibility := []appcontent.Visibility{
		appcontent.VisibilityPublic, appcontent.VisibilityLocal, appcontent.VisibilityUnlisted, appcontent.VisibilityPrivate,
	}
	profile := func(v timelineAuthor, q apptimeline.ProfileQuery) ([]apptimeline.Entry, error) {
		q.Window, q.AuthorAccountID = r.window(v), alice.id
		return r.repo.ListProfile(ctx, q)
	}
	got, err := profile(viewer, apptimeline.ProfileQuery{Visibilities: everyVisibility, IncludeDirect: true})
	checkTimeline(t, "ListProfile(every visibility)", got, err, pub, loc, unl, priv, toViewer, reply, boostOfFar, boostOfStranger)
	got, err = profile(alice, apptimeline.ProfileQuery{Visibilities: everyVisibility, IncludeDirect: true})
	checkTimeline(t, "ListProfile(by the author)", got, err, pub, loc, unl, priv, toViewer, toStranger, reply, boostOfFar, boostOfStranger)
	got, err = profile(viewer, apptimeline.ProfileQuery{Visibilities: []appcontent.Visibility{appcontent.VisibilityPublic, appcontent.VisibilityUnlisted}})
	checkTimeline(t, "ListProfile(public and unlisted)", got, err, pub, unl, reply, boostOfFar, boostOfStranger)
	got, err = profile(viewer, apptimeline.ProfileQuery{Visibilities: []appcontent.Visibility{appcontent.VisibilityPublic}, ExcludeReplies: true, ExcludeBoosts: true})
	checkTimeline(t, "ListProfile(no replies or boosts)", got, err, pub)

	// The boosts of statuses whose author the viewer excludes, or whose domain it mutes, are left out.
	r.exclude(t, viewer, stranger, timeline.ExcludedMute, nil)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote})
	})
	got, err = profile(viewer, apptimeline.ProfileQuery{Visibilities: []appcontent.Visibility{appcontent.VisibilityPublic}})
	checkTimeline(t, "ListProfile(excluded originals)", got, err, pub, reply)
}

func timelineRepoLanguagesAndPages(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, alice := r.local(), r.local()
	en := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{language: "en"})
	ja := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{language: "ja"})
	und := r.post(t, alice, appcontent.VisibilityPublic, entryOpts{language: timeline.UndeterminedLanguage})
	var more []apptimeline.Entry
	for range 3 {
		more = append(more, r.post(t, alice, appcontent.VisibilityPublic, entryOpts{language: "fr"}))
	}
	r.follow(t, viewer, alice, true)
	w := r.window(viewer)
	w.Languages = []string{"ja", "fr"}
	for _, tc := range []struct {
		name string
		list func(apptimeline.Window) ([]apptimeline.Entry, error)
	}{
		{"ListHome", func(w apptimeline.Window) ([]apptimeline.Entry, error) {
			return r.repo.ListHome(ctx, apptimeline.HomeQuery{Window: w})
		}},
		{"ListLocal", func(w apptimeline.Window) ([]apptimeline.Entry, error) {
			return r.repo.ListLocal(ctx, apptimeline.LocalQuery{Window: w, IncludeLocal: true})
		}},
		{"ListPublic", func(w apptimeline.Window) ([]apptimeline.Entry, error) {
			return r.repo.ListPublic(ctx, apptimeline.PublicQuery{Window: w})
		}},
		{"ListProfile", func(w apptimeline.Window) ([]apptimeline.Entry, error) {
			return r.repo.ListProfile(ctx, apptimeline.ProfileQuery{Window: w, AuthorAccountID: alice.id, Visibilities: []appcontent.Visibility{appcontent.VisibilityPublic}})
		}},
	} {
		got, err := tc.list(w)
		checkTimeline(t, tc.name+"(languages)", got, err, append([]apptimeline.Entry{ja, und}, more...)...)
		all := r.window(viewer)
		all.Languages = nil
		got, err = tc.list(all)
		checkTimeline(t, tc.name+"(every language)", got, err, append([]apptimeline.Entry{en, ja, und}, more...)...)

		// Pages of two, newest first, each before the last entry of the previous one.
		var paged []apptimeline.Entry
		page := r.window(viewer)
		page.Limit = 2
		for range 4 {
			got, err := tc.list(page)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) > 2 {
				t.Fatalf("%s page = %d entries, want at most 2", tc.name, len(got))
			}
			paged = append(paged, got...)
			if len(got) > 0 {
				page.Before = got[len(got)-1].ID
			}
		}
		checkTimeline(t, tc.name+"(pages)", paged, nil, append([]apptimeline.Entry{en, ja, und}, more...)...)
	}
}

func timelineRepoFacts(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, followed, muted, restricted, far := r.local(), r.local(), r.local(), r.local(), r.remote()
	r.follow(t, viewer, followed, true)
	expires := r.clock.Now().Add(time.Minute)
	r.exclude(t, viewer, muted, timeline.ExcludedMute, nil)
	r.exclude(t, viewer, muted, timeline.ExcludedBlock, &expires)
	// Upserting the same exclusion replaces its expiry.
	r.exclude(t, viewer, muted, timeline.ExcludedBlock, &expires)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.UpsertAuthorRestriction(ctx, tx, apptimeline.AuthorRestriction{AccountID: restricted.id, Restriction: timeline.RestrictProfileView, EffectiveUntil: &expires}),
			r.repo.UpsertDomainRestriction(ctx, tx, apptimeline.DomainRestriction{Domain: timelineRepoRemote, Restriction: timeline.RestrictPublicDiscovery}),
			r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote}),
		)
	})
	toViewer := r.post(t, followed, appcontent.VisibilityDirect, entryOpts{direct: []string{viewer.id}})
	toOther := r.post(t, followed, appcontent.VisibilityDirect, entryOpts{direct: []string{muted.id}})

	q := apptimeline.FactsQuery{
		ViewerAccountID: viewer.id, AccountIDs: []string{followed.id, muted.id, restricted.id, far.id},
		Domains: []string{timelineRepoRemote, "other.example"}, EntryIDs: []string{toViewer.ID, toOther.ID}, Now: r.clock.Now(),
	}
	f, err := r.repo.Facts(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(f.Exclusions[muted.id])
	if !reflect.DeepEqual(f.Followed, map[string]bool{followed.id: true}) ||
		!reflect.DeepEqual(f.Exclusions, map[string][]timeline.ExclusionReason{muted.id: {timeline.ExcludedBlock, timeline.ExcludedMute}}) ||
		!reflect.DeepEqual(f.AccountRestrictions, map[string][]timeline.Restriction{restricted.id: {timeline.RestrictProfileView}}) ||
		!reflect.DeepEqual(f.DomainRestrictions, map[string][]timeline.Restriction{timelineRepoRemote: {timeline.RestrictPublicDiscovery}}) ||
		!reflect.DeepEqual(f.Direct, map[string]bool{toViewer.ID: true}) {
		t.Errorf("Facts = %+v", f)
	}
	// Once they expire, the block and the restriction are left out.
	q.Now = expires
	f, err = r.repo.Facts(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Exclusions, map[string][]timeline.ExclusionReason{muted.id: {timeline.ExcludedMute}}) || len(f.AccountRestrictions) != 0 {
		t.Errorf("Facts at the expiry = %+v", f)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.DeleteExclusion(ctx, tx, viewer.id, muted.id, timeline.ExcludedMute)
	})
	if f, err := r.repo.Facts(ctx, q); err != nil || len(f.Exclusions) != 0 {
		t.Errorf("Facts after DeleteExclusion = %+v, %v", f.Exclusions, err)
	}

	got, err := r.repo.MutedDomains(ctx, viewer.id, []string{"a." + timelineRepoRemote, timelineRepoRemote, "example"})
	if err != nil || !slices.Equal(got, []string{timelineRepoRemote}) {
		t.Errorf("MutedDomains = %v, %v; want %v", got, err, []string{timelineRepoRemote})
	}
	if got, err := r.repo.MutedDomains(ctx, followed.id, []string{timelineRepoRemote}); err != nil || len(got) != 0 {
		t.Errorf("MutedDomains(another viewer) = %v, %v; want none", got, err)
	}
}

func timelineRepoLists(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	owner, a, b, c := r.local(), r.local(), r.local(), r.remote()
	r.follow(t, owner, a, true)
	r.follow(t, owner, b, true)
	first := apptimeline.List{ID: r.ids.NewID(), OwnerAccountID: owner.id, Name: "Friends", CreatedAt: r.clock.Now()}
	r.clock.Advance(time.Second)
	second := apptimeline.List{ID: r.ids.NewID(), OwnerAccountID: owner.id, Name: "Work", CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertList(ctx, tx, first); err != nil {
			return err
		}
		if err := r.repo.InsertList(ctx, tx, second); err != nil {
			return err
		}
		if err := r.repo.RenameList(ctx, tx, second.ID, "Office"); err != nil {
			return err
		}
		if n, err := r.repo.CountLists(ctx, tx, owner.id); err != nil || n != 2 {
			t.Errorf("CountLists = %d, %v; want 2", n, err)
		}
		if _, err := r.repo.FindList(ctx, tx, r.ids.NewID()); !errors.Is(err, apptimeline.ErrNotFound) {
			t.Errorf("FindList(unknown) = %v, want ErrNotFound", err)
		}
		for _, tc := range []struct {
			follower, followee timelineAuthor
			want               bool
		}{{owner, a, true}, {owner, c, false}, {a, owner, false}} {
			if got, err := r.repo.Following(ctx, tx, tc.follower.id, tc.followee.id); err != nil || got != tc.want {
				t.Errorf("Following = %v, %v; want %v", got, err, tc.want)
			}
		}
		for _, m := range []timelineAuthor{b, a} {
			if inserted, err := r.repo.InsertListMember(ctx, tx, apptimeline.ListMember{
				ListID: first.ID, MemberAccountID: m.id, OwnerAccountID: owner.id, AddedAt: r.clock.Now(),
			}); err != nil || !inserted {
				t.Errorf("InsertListMember = %v, %v; want true", inserted, err)
			}
		}
		if inserted, err := r.repo.InsertListMember(ctx, tx, apptimeline.ListMember{
			ListID: first.ID, MemberAccountID: a.id, OwnerAccountID: owner.id, AddedAt: r.clock.Now(),
		}); err != nil || inserted {
			t.Errorf("InsertListMember(again) = %v, %v; want false", inserted, err)
		}
		if _, err := r.repo.InsertListMember(ctx, tx, apptimeline.ListMember{
			ListID: second.ID, MemberAccountID: a.id, OwnerAccountID: owner.id, AddedAt: r.clock.Now(),
		}); err != nil {
			return err
		}
		if n, err := r.repo.CountListMembers(ctx, tx, first.ID); err != nil || n != 2 {
			t.Errorf("CountListMembers = %d, %v; want 2", n, err)
		}
		got, err := r.repo.FindList(ctx, tx, first.ID)
		if want := first; err != nil || got.ID != want.ID || got.Name != want.Name || got.OwnerAccountID != owner.id ||
			!got.CreatedAt.Equal(want.CreatedAt) || got.MembersCount != 2 {
			t.Errorf("FindList = %+v, %v; want %+v with 2 members", got, err, want)
		}
		return nil
	})

	second.Name, second.MembersCount, first.MembersCount = "Office", 1, 2
	if got, err := r.repo.List(ctx, second.ID); err != nil || !reflect.DeepEqual(got, second) {
		t.Errorf("List = %+v, %v; want %+v", got, err, second)
	}
	if _, err := r.repo.List(ctx, r.ids.NewID()); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("List(unknown) = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.ListsOf(ctx, owner.id); err != nil || !reflect.DeepEqual(got, []apptimeline.List{first, second}) {
		t.Errorf("ListsOf = %+v, %v; want %+v", got, err, []apptimeline.List{first, second})
	}
	if got, err := r.repo.ListsOf(ctx, a.id); err != nil || len(got) != 0 {
		t.Errorf("ListsOf(another account) = %+v, %v; want none", got, err)
	}
	members := []string{a.id, b.id}
	var paged []string
	after := ""
	for range 3 {
		page, err := r.repo.ListMembers(ctx, first.ID, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range page {
			paged = append(paged, id)
			after = id
		}
	}
	if !slices.Equal(paged, members) {
		t.Errorf("ListMembers pages = %v, want %v", paged, members)
	}
	if got, err := r.repo.ListIDsWithMember(ctx, owner.id, a.id); err != nil || !slices.Equal(got, []string{first.ID, second.ID}) {
		t.Errorf("ListIDsWithMember = %v, %v", got, err)
	}

	// An ended follow takes the member out of every list of the owner; a rebuild takes out the
	// members whose follow is gone.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteListMembersOfFollow(ctx, tx, owner.id, a.id); err != nil {
			return err
		}
		return r.repo.DeleteFollowEdge(ctx, tx, owner.id, b.id)
	})
	if got, err := r.repo.ListIDsWithMember(ctx, owner.id, a.id); err != nil || len(got) != 0 {
		t.Errorf("ListIDsWithMember after DeleteListMembersOfFollow = %v, %v", got, err)
	}
	if got, err := r.repo.AllListMembers(ctx); err != nil || len(got) != 1 || got[0].MemberAccountID != b.id {
		t.Errorf("AllListMembers = %+v, %v; want b's membership", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteListMembersWithoutFollow(ctx, tx) })
	if got, err := r.repo.AllListMembers(ctx); err != nil || len(got) != 0 {
		t.Errorf("AllListMembers after DeleteListMembersWithoutFollow = %+v, %v; want none", got, err)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.repo.InsertListMember(ctx, tx, apptimeline.ListMember{
			ListID: first.ID, MemberAccountID: a.id, OwnerAccountID: owner.id, AddedAt: r.clock.Now(),
		}); err != nil {
			return err
		}
		if err := r.repo.DeleteListMember(ctx, tx, first.ID, a.id); err != nil {
			return err
		}
		if n, err := r.repo.CountListMembers(ctx, tx, first.ID); err != nil || n != 0 {
			t.Errorf("CountListMembers after DeleteListMember = %d, %v", n, err)
		}
		return r.repo.DeleteList(ctx, tx, first.ID)
	})
	if _, err := r.repo.List(ctx, first.ID); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("List after DeleteList = %v, want ErrNotFound", err)
	}
}

func timelineRepoListTimeline(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	owner, member, quiet, outsider := r.local(), r.local(), r.local(), r.local()
	r.follow(t, owner, member, true)
	r.follow(t, owner, quiet, false)
	r.follow(t, owner, outsider, true)
	l := apptimeline.List{ID: r.ids.NewID(), OwnerAccountID: owner.id, Name: "List", CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertList(ctx, tx, l); err != nil {
			return err
		}
		for _, m := range []timelineAuthor{member, quiet} {
			if _, err := r.repo.InsertListMember(ctx, tx, apptimeline.ListMember{
				ListID: l.ID, MemberAccountID: m.id, OwnerAccountID: owner.id, AddedAt: r.clock.Now(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	outsiders := r.post(t, outsider, appcontent.VisibilityPublic, entryOpts{})
	members := r.post(t, member, appcontent.VisibilityPrivate, entryOpts{})
	boost := r.boost(t, member, outsiders)
	r.boost(t, quiet, outsiders) // hidden: the follow does not show boosts
	quiets := r.post(t, quiet, appcontent.VisibilityPublic, entryOpts{})
	toOwner := r.post(t, member, appcontent.VisibilityDirect, entryOpts{direct: []string{owner.id}})
	r.post(t, member, appcontent.VisibilityDirect, entryOpts{direct: []string{outsider.id}})
	r.post(t, owner, appcontent.VisibilityPublic, entryOpts{})

	list := func(v timelineAuthor, floor string) ([]apptimeline.Entry, error) {
		return r.repo.ListListEntries(ctx, apptimeline.ListTimelineQuery{Window: r.window(v), ListID: l.ID, Floor: floor})
	}
	got, err := list(owner, "")
	checkTimeline(t, "ListListEntries", got, err, members, boost, quiets, toOwner)
	got, err = list(owner, quiets.ID)
	checkTimeline(t, "ListListEntries(floor)", got, err, quiets, toOwner)
	got, err = list(outsider, "")
	checkTimeline(t, "ListListEntries(another viewer)", got, err)
	// A member whose follow is gone leaves the list's timeline.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.DeleteFollowEdge(ctx, tx, owner.id, member.id)
	})
	got, err = list(owner, "")
	checkTimeline(t, "ListListEntries(unfollowed)", got, err, quiets)
}

func timelineRepoRebuild(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, alice := r.local(), r.local()
	r.post(t, alice, appcontent.VisibilityDirect, entryOpts{direct: []string{viewer.id}, tags: []string{r.ids.NewID()}})
	r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{r.ids.NewID()}})
	r.follow(t, viewer, alice, false)
	r.exclude(t, viewer, alice, timeline.ExcludedMute, nil)
	until := r.clock.Now().Add(time.Hour)
	authorRestrictions := []apptimeline.AuthorRestriction{
		{AccountID: alice.id, Restriction: timeline.RestrictProfileView},
		{AccountID: alice.id, Restriction: timeline.RestrictPublicDiscovery, EffectiveUntil: &until},
	}
	domainRestrictions := []apptimeline.DomainRestriction{{Domain: timelineRepoRemote, Restriction: timeline.RestrictProfileView, EffectiveUntil: &until}}
	l := apptimeline.List{ID: r.ids.NewID(), OwnerAccountID: viewer.id, Name: "Kept", CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, ar := range authorRestrictions {
			if err := r.repo.UpsertAuthorRestriction(ctx, tx, ar); err != nil {
				return err
			}
		}
		for _, dr := range domainRestrictions {
			if err := r.repo.UpsertDomainRestriction(ctx, tx, dr); err != nil {
				return err
			}
		}
		if err := r.repo.InsertDomainMute(ctx, tx, apptimeline.DomainMute{ViewerAccountID: viewer.id, Domain: timelineRepoRemote}); err != nil {
			return err
		}
		return r.repo.InsertList(ctx, tx, l)
	})
	snap, err := r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Entries) != 2 || len(snap.DirectRecipients) != 1 || len(snap.TagEntries) != 2 || len(snap.TagUsage) != 1 ||
		!reflect.DeepEqual(snap.FollowEdges, []apptimeline.FollowEdge{{FollowerAccountID: viewer.id, FolloweeAccountID: alice.id}}) ||
		!reflect.DeepEqual(snap.Exclusions, []apptimeline.Exclusion{{ViewerAccountID: viewer.id, TargetAccountID: alice.id, Reason: timeline.ExcludedMute}}) ||
		!reflect.DeepEqual(snap.DomainMutes, []apptimeline.DomainMute{{ViewerAccountID: viewer.id, Domain: timelineRepoRemote}}) {
		t.Errorf("Snapshot = %+v", snap)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		authors, domains, err := r.repo.ListRestrictions(ctx, tx)
		if err != nil {
			return err
		}
		slices.SortFunc(authors, func(a, b apptimeline.AuthorRestriction) int { return cmp.Compare(a.Restriction, b.Restriction) })
		if !reflect.DeepEqual(authors, authorRestrictions) || !reflect.DeepEqual(domains, domainRestrictions) {
			t.Errorf("ListRestrictions = %+v, %+v; want %+v, %+v", authors, domains, authorRestrictions, domainRestrictions)
		}
		return r.repo.DeleteProjection(ctx, tx)
	})
	snap, err = r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Entries)+len(snap.DirectRecipients)+len(snap.TagEntries)+len(snap.TagUsage)+len(snap.FollowEdges)+len(snap.Exclusions)+len(snap.DomainMutes) != 0 {
		t.Errorf("Snapshot after DeleteProjection = %+v, want it empty", snap)
	}
	// The restrictions and the lists are not projections of content and graph events.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		authors, domains, err := r.repo.ListRestrictions(ctx, tx)
		if err != nil {
			return err
		}
		if len(authors) != 2 || len(domains) != 1 {
			t.Errorf("ListRestrictions after DeleteProjection = %+v, %+v; want them kept", authors, domains)
		}
		if err := r.repo.DeleteRestrictions(ctx, tx); err != nil {
			return err
		}
		authors, domains, err = r.repo.ListRestrictions(ctx, tx)
		if err != nil || len(authors)+len(domains) != 0 {
			t.Errorf("ListRestrictions after DeleteRestrictions = %+v, %+v, %v; want none", authors, domains, err)
		}
		return nil
	})
	if _, err := r.repo.List(ctx, l.ID); err != nil {
		t.Errorf("List after DeleteProjection = %v, want it kept", err)
	}
}
