// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The replies that the timelines show (ADR-0013 §31): home and the lists show a reply whose
// parent's author is the viewer, the reply's author, or an account the viewer follows; the local
// and federated timelines a reply to its own author; the hashtag timelines and the profiles every
// reply. The parent's author is known while content links the reply to its parent.

// release accepts the request that holds the reply s, as its parent's author does, and emits
// StatusReleased with the link that the release makes: to the parent while it is stored and live.
func (e *timelineEnv) release(t *testing.T, s content.Status) {
	t.Helper()
	e.Clock.Advance(time.Millisecond)
	ctx := t.Context()
	rev, err := e.h.Statuses.FindCurrentRevision(ctx, s.ID)
	if err != nil {
		t.Fatalf("FindCurrentRevision(%s): %v", s.ID, err)
	}
	approvals, ok := e.h.Statuses.(appcontent.ApprovalRepository)
	if !ok {
		t.Fatal("the status repository keeps no reply requests")
	}
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		req, err := approvals.FindInteractionRequestFor(ctx, tx, content.InteractionReply, s.URI, s.InReplyToURI)
		if err != nil {
			return err
		}
		if err := approvals.InsertInteractionEvent(ctx, tx, content.InteractionEvent{
			ID: e.IDs.NewID(), RequestID: req.ID, Kind: content.InteractionEventAccepted, OccurredAt: e.Clock.Now(),
		}); err != nil {
			return err
		}
		ev := appcontent.StatusReleased{Status: s, Revision: rev}
		switch parent, err := e.h.Statuses.FindLiveStatusByURI(ctx, tx, s.InReplyToURI); {
		case errors.Is(err, appcontent.ErrNotFound):
		case err != nil:
			return err
		default:
			ev.ReplyLink = &content.ReplyLink{StatusID: s.ID, ParentStatusID: parent.ID, ParentAuthorAccountID: parent.AuthorAccountID}
		}
		return tx.Emit(ctx, ev)
	})
	if err != nil {
		t.Fatalf("release %s: %v", s.ID, err)
	}
}

// removeMany deletes sts together and emits one StatusesDeleted, as a moderation action on
// statuses does.
func (e *timelineEnv) removeMany(t *testing.T, sts ...content.Status) {
	t.Helper()
	e.Clock.Advance(time.Millisecond)
	action := e.IDs.NewID()
	ev := appcontent.StatusesDeleted{}
	ds := make([]content.Deletion, len(sts))
	for i, s := range sts {
		ds[i] = content.Deletion{
			StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByModeration,
			ModerationActionID: action, DeletedAt: e.Clock.Now(),
		}
		ev.Deletions = append(ev.Deletions, appcontent.StatusDeleted{Deletion: ds[i], Status: s})
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.h.Statuses.InsertDeletions(t.Context(), tx, ds); err != nil {
			return err
		}
		return tx.Emit(t.Context(), ev)
	})
	if err != nil {
		t.Fatalf("delete %d statuses: %v", len(sts), err)
	}
}

func listedIn(p apptimeline.Page) map[string]apptimeline.Entry {
	out := map[string]apptimeline.Entry{}
	for _, en := range p.Entries {
		out[en.ID] = en
	}
	return out
}

// streamsAgree checks that the judgements of the streams agree with the REST timelines of viewer
// for each of ids: HomeEntry with Home (and with its followed hashtags), EntryLists of a home
// entry with the timelines of the viewer's lists (list_update), and LocalEntry, PublicEntry,
// LocalEntryFor, and PublicEntryFor with Local and Public.
func (e *timelineEnv) streamsAgree(t *testing.T, when, viewer string, listIDs []string, ids []string) {
	t.Helper()
	ctx := t.Context()
	v := viewerOf(viewer)
	q := apptimeline.Query{Limit: apptimeline.MaxLimit}
	page := func(p apptimeline.Page, err error) map[string]apptimeline.Entry {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: a timeline: %v", when, err)
		}
		return listedIn(p)
	}
	home := page(e.svc.Home(ctx, v, q))
	local := page(e.svc.Local(ctx, v, q))
	public := page(e.svc.Public(ctx, v, q))
	lists := map[string]map[string]apptimeline.Entry{}
	for _, id := range listIDs {
		lists[id] = page(e.svc.ListTimeline(ctx, v, id, q))
	}
	viewers := []apptimeline.StreamViewer{{Viewer: v}}
	for _, id := range ids {
		en, ok, err := e.svc.HomeEntry(ctx, v, id, apptimeline.Query{})
		listed, onHome := home[id]
		switch {
		case err != nil:
			t.Fatalf("%s: HomeEntry(%s): %v", when, id, err)
		case ok != onHome:
			t.Errorf("%s: HomeEntry(%s) = %v, Home lists it: %v", when, id, ok, onHome)
		case ok && !slices.Equal(en.FollowedTags, listed.FollowedTags):
			t.Errorf("%s: HomeEntry(%s) has the followed tags %v, Home %v", when, id, en.FollowedTags, listed.FollowedTags)
		}
		var wantLists []string
		for _, l := range listIDs {
			if _, in := lists[l][id]; in {
				wantLists = append(wantLists, l)
			}
		}
		if ok {
			got, err := e.svc.EntryLists(ctx, v, en)
			if err != nil {
				t.Fatalf("%s: EntryLists(%s): %v", when, id, err)
			}
			slices.Sort(got)
			slices.Sort(wantLists)
			if !slices.Equal(got, wantLists) {
				t.Errorf("%s: EntryLists(%s) = %v, the list timelines that list it %v", when, id, got, wantLists)
			}
		} else if len(wantLists) > 0 {
			t.Errorf("%s: the lists %v list %s, home does not", when, wantLists, id)
		}
		for _, tl := range []struct {
			name     string
			listed   map[string]apptimeline.Entry
			entry    func() (apptimeline.Entry, bool, error)
			entryFor func() (map[string]apptimeline.Entry, error)
		}{
			{"Local", local,
				func() (apptimeline.Entry, bool, error) { return e.svc.LocalEntry(ctx, v, id, apptimeline.Query{}) },
				func() (map[string]apptimeline.Entry, error) { return e.svc.LocalEntryFor(ctx, viewers, id) }},
			{"Public", public,
				func() (apptimeline.Entry, bool, error) { return e.svc.PublicEntry(ctx, v, id, apptimeline.Query{}) },
				func() (map[string]apptimeline.Entry, error) { return e.svc.PublicEntryFor(ctx, viewers, id) }},
		} {
			_, want := tl.listed[id]
			if _, ok, err := tl.entry(); err != nil || ok != want {
				t.Errorf("%s: %sEntry(%s) = %v, %v; %s lists it: %v", when, tl.name, id, ok, err, tl.name, want)
			}
			got, err := tl.entryFor()
			if _, ok := got[viewer]; err != nil || ok != want {
				t.Errorf("%s: %sEntryFor(%s) = %v, %v; %s lists it: %v", when, tl.name, id, got, err, tl.name, want)
			}
		}
	}
}

// expected is, by timeline, the entries it lists.
type expected map[string]map[string]bool

func (w expected) add(id string, timelines ...string) {
	for _, tl := range timelines {
		if w[tl] == nil {
			w[tl] = map[string]bool{}
		}
		w[tl][id] = true
	}
}

func (w expected) drop(id string, timelines ...string) {
	for _, tl := range timelines {
		delete(w[tl], id)
	}
}

func (w expected) ids(tl string) []string { return slices.Collect(maps.Keys(w[tl])) }

// timelineReplies posts replies of every kind and checks which timelines list them as posted, once
// a parent that a reply waited for is stored and a held reply is released, after their parents'
// deletions alone and together, and after a rebuild of the projections; and that the streams
// judge each of them as the REST timelines list it.
func timelineReplies(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	viewer, bob, carol, stranger := e.account(true), e.account(true), e.account(true), e.account(true)
	far, held := e.account(false), e.account(false)
	for _, followee := range []string{bob, carol, far, held} {
		e.follow(t, viewer, followee)
	}
	e.acceptRemote(t, viewer, far)
	e.acceptRemote(t, viewer, held)
	list := e.createList(t, viewer, "Replies")
	for _, member := range []string{bob, far, held} {
		e.addMember(t, viewer, list.ID, member)
	}
	cats := e.tag(t, "cats")
	missing := "https://" + timelineRemoteDomain + "/statuses/missing"
	lateURI := "https://" + timelineRemoteDomain + "/statuses/late"
	w := expected{}
	var all []string
	post := func(author string, o postOpts, timelines ...string) content.Status {
		t.Helper()
		s := e.post(t, author, content.VisibilityPublic, o)
		all = append(all, s.ID)
		w.add(s.ID, timelines...)
		return s
	}
	reply := func(author, to string, timelines ...string) content.Status {
		t.Helper()
		return post(author, postOpts{inReplyTo: to, tags: []string{cats}}, append(timelines, "tag")...)
	}
	boost := func(booster string, s content.Status, timelines ...string) content.Boost {
		t.Helper()
		b := e.boost(t, booster, s)
		all = append(all, b.ID)
		w.add(b.ID, timelines...)
		return b
	}

	bobs := post(bob, postOpts{}, "home", "list", "local", "public", "bob")
	viewers := post(viewer, postOpts{}, "home", "local", "public")
	carols := post(carol, postOpts{}, "home", "local", "public")
	strangers := post(stranger, postOpts{}, "local", "public")
	farRoot := post(far, postOpts{}, "home", "list", "public", "far")

	bobSelf := reply(bob, bobs.URI, "home", "list", "local", "public", "bob")
	reply(bob, viewers.URI, "home", "list", "bob")
	bobToCarol := reply(bob, carols.URI, "home", "list", "bob")
	bobToStranger := reply(bob, strangers.URI, "bob")
	bobToMissing := reply(bob, missing, "bob")
	reply(viewer, viewers.URI, "home", "local", "public")
	viewerToBob := reply(viewer, bobs.URI, "home")
	reply(viewer, strangers.URI)
	reply(stranger, strangers.URI, "local", "public")
	strangerToBob := reply(stranger, bobs.URI)
	farSelf := reply(far, farRoot.URI, "home", "list", "public", "far")
	farToBob := reply(far, bobs.URI, "home", "list", "far")
	reply(far, strangers.URI, "far")
	farLate := reply(far, lateURI, "far")
	// Boosts of replies are not replies: a followed account's boost shows whatever it boosts.
	carolBoost := boost(carol, bobToStranger, "home")
	boost(bob, strangerToBob, "home", "list", "bob")
	// A held reply has no link to its parent until it is released.
	heldReply := post(held, postOpts{inReplyTo: carols.URI, heldBy: carol})

	q := apptimeline.Query{Limit: apptimeline.MaxLimit}
	v := viewerOf(viewer)
	check := func(when string) {
		t.Helper()
		for _, tl := range []struct {
			name string
			list func() (apptimeline.Page, error)
		}{
			{"home", func() (apptimeline.Page, error) { return e.svc.Home(ctx, v, q) }},
			{"list", func() (apptimeline.Page, error) { return e.svc.ListTimeline(ctx, v, list.ID, q) }},
			{"local", func() (apptimeline.Page, error) { return e.svc.Local(ctx, v, q) }},
			{"public", func() (apptimeline.Page, error) { return e.svc.Public(ctx, v, q) }},
			{"tag", func() (apptimeline.Page, error) { return e.svc.Hashtag(ctx, v, cats, q) }},
			{"bob", func() (apptimeline.Page, error) { return e.svc.Profile(ctx, v, bob, apptimeline.ProfileFilter{}, q) }},
			{"far", func() (apptimeline.Page, error) { return e.svc.Profile(ctx, v, far, apptimeline.ProfileFilter{}, q) }},
		} {
			p, err := tl.list()
			expectPage(t, when+": "+tl.name, p, err, w.ids(tl.name)...)
		}
		e.streamsAgree(t, when, viewer, []string{list.ID}, all)
	}
	check("as posted")

	// The parent that a reply waited for arrives; the held reply is released.
	lateParent := post(far, postOpts{uri: lateURI}, "home", "list", "public", "far")
	w.add(farLate.ID, "home", "list", "public")
	e.release(t, heldReply)
	w.add(heldReply.ID, "home", "list")
	check("after the late parent and the release")

	// Deleting a parent, alone or with others, takes the replies to it off home, the lists, and
	// the local and federated timelines; a reply to a deleted parent is not shown there either.
	e.MustRemove(t, carols)
	w.drop(carols.ID, "home", "local", "public")
	w.drop(bobToCarol.ID, "home", "list")
	w.drop(heldReply.ID, "home", "list")
	e.removeMany(t, bobs, farRoot)
	w.drop(bobs.ID, "home", "list", "local", "public", "bob")
	w.drop(farRoot.ID, "home", "list", "public", "far")
	w.drop(bobSelf.ID, "home", "list", "local", "public")
	w.drop(viewerToBob.ID, "home")
	w.drop(farSelf.ID, "home", "list", "public")
	w.drop(farToBob.ID, "home", "list")
	reply(bob, carols.URI, "bob")
	check("after the deletions of the parents")

	// The projection keeps the parent and its author while the link holds, and a rebuild ends
	// where the incremental updates did.
	incremental, err := e.h.Timeline.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	targets := map[string][2]string{}
	for _, en := range incremental.Entries {
		targets[en.ID] = [2]string{en.InReplyToStatusID, en.InReplyToAccountID}
	}
	for id, want := range map[string][2]string{
		bobToStranger.ID: {strangers.ID, stranger},
		farLate.ID:       {lateParent.ID, far},
		bobToMissing.ID:  {},
		bobSelf.ID:       {},
		bobToCarol.ID:    {},
		heldReply.ID:     {},
		carolBoost.ID:    {},
	} {
		if got := targets[id]; got != want {
			t.Errorf("the reply target of %s = %v, want %v", id, got, want)
		}
	}
	sources := []apptimeline.Source{apptimeline.ContentSource(e.h.Statuses, e.h.Interactions), apptimeline.GraphSource(e.h.Graph)}
	if err := apptimeline.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Timeline, sources...); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	if rebuilt, err := e.h.Timeline.Snapshot(ctx); err != nil || !reflect.DeepEqual(rebuilt, incremental) {
		t.Fatalf("rebuilt projection differs from the incremental one (%v):\n rebuilt:     %+v\n incremental: %+v", err, rebuilt, incremental)
	}
	check("after a rebuild")
}

// timelineRepliesOfFollowedTags checks the hashtag branch of home next to the replies (ADR-0013
// §25, §28, §31): a reply with a followed hashtag that the follows do not show, a followed
// author's and the viewer's own included, appears once with the followed-hashtag mark; one that
// the follows show appears once without it; the lists, which have no hashtag branch, leave out
// what the follows do not show.
func timelineRepliesOfFollowedTags(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	viewer, bob, carol, stranger := e.account(true), e.account(true), e.account(true), e.account(true)
	e.follow(t, viewer, bob)
	e.follow(t, viewer, carol)
	dogs := e.followTag(t, viewer, "dogs")
	list := e.createList(t, viewer, "Bob")
	e.addMember(t, viewer, list.ID, bob)
	missing := "https://" + timelineRemoteDomain + "/statuses/missing"

	marked, unmarked, inList := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var all []string
	post := func(author string, v content.Visibility, o postOpts) content.Status {
		t.Helper()
		s := e.post(t, author, v, o)
		all = append(all, s.ID)
		return s
	}
	tagged := func(to string) postOpts { return postOpts{inReplyTo: to, tags: []string{dogs}} }
	bobs := post(bob, content.VisibilityPublic, postOpts{})
	carols := post(carol, content.VisibilityPublic, postOpts{})
	viewers := post(viewer, content.VisibilityPublic, postOpts{})
	strangers := post(stranger, content.VisibilityPublic, postOpts{})
	unmarked[bobs.ID], unmarked[carols.ID], unmarked[viewers.ID], inList[bobs.ID] = true, true, true, true

	bobToStranger := post(bob, content.VisibilityPublic, tagged(strangers.URI))
	bobLocalToStranger := post(bob, content.VisibilityLocal, tagged(strangers.URI))
	bobToMissing := post(bob, content.VisibilityPublic, tagged(missing))
	viewerToStranger := post(viewer, content.VisibilityPublic, tagged(strangers.URI))
	strangerToStranger := post(stranger, content.VisibilityPublic, tagged(strangers.URI))
	for _, s := range []content.Status{bobToStranger, bobLocalToStranger, bobToMissing, viewerToStranger, strangerToStranger} {
		marked[s.ID] = true
	}
	bobToCarol := post(bob, content.VisibilityPublic, tagged(carols.URI))
	bobSelf := post(bob, content.VisibilityPublic, tagged(bobs.URI))
	viewerSelf := post(viewer, content.VisibilityPublic, tagged(viewers.URI))
	unmarked[bobToCarol.ID], unmarked[bobSelf.ID], unmarked[viewerSelf.ID] = true, true, true
	inList[bobToCarol.ID], inList[bobSelf.ID] = true, true
	// Neither branch shows a reply to a stranger without a followed tag, nor an unlisted one.
	post(bob, content.VisibilityPublic, postOpts{inReplyTo: strangers.URI})
	post(bob, content.VisibilityUnlisted, tagged(strangers.URI))

	v := viewerOf(viewer)
	q := apptimeline.Query{Limit: apptimeline.MaxLimit}
	check := func(when string) {
		t.Helper()
		p, err := e.svc.Home(ctx, v, q)
		expectPage(t, when+": Home", p, err, append(slices.Collect(maps.Keys(marked)), slices.Collect(maps.Keys(unmarked))...)...)
		for _, en := range p.Entries {
			var want []string
			if marked[en.ID] {
				want = []string{"dogs"}
			}
			if !slices.Equal(en.FollowedTags, want) {
				t.Errorf("%s: the followed tags of %s = %v, want %v", when, en.ID, en.FollowedTags, want)
			}
		}
		p, err = e.svc.ListTimeline(ctx, v, list.ID, q)
		expectPage(t, when+": ListTimeline", p, err, slices.Collect(maps.Keys(inList))...)
		e.streamsAgree(t, when, viewer, []string{list.ID}, all)
	}
	check("as posted")

	// Once carol's post is deleted, bob's reply to it has no known parent: the follows leave it
	// out, the followed hashtag brings it in, marked, and the list leaves it out.
	e.MustRemove(t, carols)
	delete(unmarked, carols.ID)
	delete(unmarked, bobToCarol.ID)
	delete(inList, bobToCarol.ID)
	marked[bobToCarol.ID] = true
	check("after the deletion of a parent")
}
