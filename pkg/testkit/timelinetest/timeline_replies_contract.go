// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"context"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// timelineRepoReplies checks the reply targets of the entries (ADR-0013 §31): InsertEntry,
// SetReplyTargets, and ClearReplyTargetsTo keep them, boosts never get one; ListHome and
// ListListEntries list a reply whose parent's author is the viewer, its author, or followed;
// ListLocal and ListPublic one to its own author; ListHomeTags brings in a tagged reply that
// ListHome leaves out, a followed author's or the viewer's own too; ListTag and ListProfile list
// every reply.
func timelineRepoReplies(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	viewer, followed, stranger, far := r.local(), r.local(), r.local(), r.remote()
	r.follow(t, viewer, followed, true)
	r.follow(t, viewer, far, true)
	l := apptimeline.List{ID: r.ids.NewID(), OwnerAccountID: viewer.id, Name: "List", CreatedAt: r.clock.Now()}
	tag := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertList(ctx, tx, l); err != nil {
			return err
		}
		for _, m := range []timelineAuthor{followed, far} {
			if _, err := r.repo.InsertListMember(ctx, tx, apptimeline.ListMember{
				ListID: l.ID, MemberAccountID: m.id, OwnerAccountID: viewer.id, AddedAt: r.clock.Now(),
			}); err != nil {
				return err
			}
		}
		_, err := r.repo.InsertTagFollow(ctx, tx, apptimeline.TagFollow{AccountID: viewer.id, TagID: tag, FollowedAt: r.clock.Now()})
		return err
	})
	public := appcontent.VisibilityPublic
	viewers := r.post(t, viewer, public, entryOpts{})
	followeds := r.post(t, followed, public, entryOpts{})
	strangers := r.post(t, stranger, public, entryOpts{})
	fars := r.post(t, far, public, entryOpts{})
	to := func(p apptimeline.Entry, tags ...string) entryOpts { return entryOpts{parent: &p, tags: tags} }

	selfReply := r.post(t, followed, public, to(followeds))
	toViewer := r.post(t, followed, public, to(viewers))
	toFar := r.post(t, followed, public, to(fars))
	toStranger := r.post(t, followed, public, to(strangers))
	unknown := r.post(t, followed, public, entryOpts{reply: true})
	ownSelf := r.post(t, viewer, public, to(viewers))
	r.post(t, viewer, public, to(strangers)) // the viewer's own reply to a stranger
	farSelf := r.post(t, far, public, to(fars))
	strangerSelf := r.post(t, stranger, public, to(strangers))
	boostOfToStranger := r.boost(t, followed, toStranger)
	boostOfUnknown := r.boost(t, followed, unknown)
	taggedToStranger := r.post(t, followed, public, to(strangers, tag))
	taggedSelf := r.post(t, followed, public, to(followeds, tag))
	taggedOwnToStranger := r.post(t, viewer, public, to(strangers, tag))
	taggedUnknown := r.post(t, followed, public, entryOpts{reply: true, tags: []string{tag}})

	type lists struct{ home, list, local, public, tags, tag, profile []apptimeline.Entry }
	check := func(when string, want lists) {
		t.Helper()
		w := r.window(viewer)
		got, err := r.repo.ListHome(ctx, apptimeline.HomeQuery{Window: w})
		checkTimeline(t, when+": ListHome", got, err, want.home...)
		got, err = r.repo.ListListEntries(ctx, apptimeline.ListTimelineQuery{Window: w, ListID: l.ID})
		checkTimeline(t, when+": ListListEntries", got, err, want.list...)
		got, err = r.repo.ListLocal(ctx, apptimeline.LocalQuery{Window: w, IncludeLocal: true})
		checkTimeline(t, when+": ListLocal", got, err, want.local...)
		got, err = r.repo.ListPublic(ctx, apptimeline.PublicQuery{Window: w})
		checkTimeline(t, when+": ListPublic", got, err, want.public...)
		got, err = r.repo.ListHomeTags(ctx, apptimeline.HomeQuery{Window: w})
		checkTimeline(t, when+": ListHomeTags", got, err, want.tags...)
		got, err = r.repo.ListTag(ctx, apptimeline.TagQuery{Window: w, TagID: tag, IncludeLocal: true})
		checkTimeline(t, when+": ListTag", got, err, want.tag...)
		got, err = r.repo.ListProfile(ctx, apptimeline.ProfileQuery{Window: w, AuthorAccountID: followed.id, Visibilities: []appcontent.Visibility{public}})
		checkTimeline(t, when+": ListProfile", got, err, want.profile...)
	}
	tagged := []apptimeline.Entry{taggedToStranger, taggedSelf, taggedOwnToStranger, taggedUnknown}
	profile := []apptimeline.Entry{
		followeds, selfReply, toViewer, toFar, toStranger, unknown, boostOfToStranger, boostOfUnknown,
		taggedToStranger, taggedSelf, taggedUnknown,
	}
	check("as inserted", lists{
		home: []apptimeline.Entry{
			viewers, followeds, fars, selfReply, toViewer, toFar, ownSelf, farSelf, boostOfToStranger, boostOfUnknown, taggedSelf,
		},
		list:    []apptimeline.Entry{followeds, fars, selfReply, toViewer, toFar, farSelf, boostOfToStranger, boostOfUnknown, taggedSelf},
		local:   []apptimeline.Entry{viewers, followeds, strangers, selfReply, ownSelf, strangerSelf, taggedSelf},
		public:  []apptimeline.Entry{viewers, followeds, strangers, fars, selfReply, ownSelf, farSelf, strangerSelf, taggedSelf},
		tags:    []apptimeline.Entry{taggedToStranger, taggedOwnToStranger, taggedUnknown},
		tag:     tagged,
		profile: profile,
	})

	// SetReplyTargets gives the post entries of replies their parent; boosts keep none.
	// ClearReplyTargetsTo takes the deleted parents away from the replies to them.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.SetReplyTargets(ctx, tx, viewers.StatusID, viewer.id, []string{unknown.StatusID, taggedUnknown.StatusID}); err != nil {
			return err
		}
		if err := r.repo.ClearReplyTargetsTo(ctx, tx, nil); err != nil {
			return err
		}
		return r.repo.ClearReplyTargetsTo(ctx, tx, []string{followeds.StatusID, fars.StatusID, r.ids.NewID()})
	})
	snap, err := r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string][2]string{}
	for _, e := range snap.Entries {
		targets[e.ID] = [2]string{e.InReplyToStatusID, e.InReplyToAccountID}
	}
	for _, c := range []struct {
		e    apptimeline.Entry
		want [2]string
	}{
		{unknown, [2]string{viewers.StatusID, viewer.id}},
		{taggedUnknown, [2]string{viewers.StatusID, viewer.id}},
		{toStranger, [2]string{strangers.StatusID, stranger.id}},
		{selfReply, [2]string{}},
		{toFar, [2]string{}},
		{farSelf, [2]string{}},
		{boostOfUnknown, [2]string{}},
		{boostOfToStranger, [2]string{}},
		{viewers, [2]string{}},
	} {
		if got := targets[c.e.ID]; got != c.want {
			t.Errorf("the reply target of %s = %v, want %v", c.e.ID, got, c.want)
		}
	}
	check("after setting and clearing reply targets", lists{
		home: []apptimeline.Entry{
			viewers, followeds, fars, toViewer, unknown, ownSelf, boostOfToStranger, boostOfUnknown, taggedUnknown,
		},
		list:    []apptimeline.Entry{followeds, fars, toViewer, unknown, boostOfToStranger, boostOfUnknown, taggedUnknown},
		local:   []apptimeline.Entry{viewers, followeds, strangers, ownSelf, strangerSelf},
		public:  []apptimeline.Entry{viewers, followeds, strangers, fars, ownSelf, strangerSelf},
		tags:    []apptimeline.Entry{taggedToStranger, taggedSelf, taggedOwnToStranger},
		tag:     tagged,
		profile: profile,
	})
}
