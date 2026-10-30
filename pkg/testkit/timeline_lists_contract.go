// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// The lists of the timeline module (ADR-0122), verified with TimelineContract: their commands,
// the members (only active follows, taken out when the follow ends), the limits, that another
// account sees none of them, the list timeline, and the lists of a home entry for the streams.

// acceptRemote makes the pending follow of a remote account active, as its server's Accept does.
func (e *timelineEnv) acceptRemote(t *testing.T, follower, followee string) {
	t.Helper()
	err := e.graph.AcceptRemote(t.Context(), appgraph.RemoteResponse{FolloweeID: followee, FollowerID: follower, ActivityURI: "https://remote.example/accept/" + follower})
	if err != nil {
		t.Fatalf("AcceptRemote: %v", err)
	}
}

func (e *timelineEnv) createList(t *testing.T, owner, name string) apptimeline.List {
	t.Helper()
	l, err := e.svc.CreateList(t.Context(), owner, name)
	if err != nil {
		t.Fatalf("CreateList(%s): %v", name, err)
	}
	return l
}

func (e *timelineEnv) addMember(t *testing.T, owner, listID, member string) {
	t.Helper()
	if err := e.svc.AddListMember(t.Context(), owner, listID, member); err != nil {
		t.Fatalf("AddListMember(%s): %v", member, err)
	}
}

func (e *timelineEnv) members(t *testing.T, owner, listID string, want ...string) {
	t.Helper()
	got, err := e.svc.ListMembers(t.Context(), owner, listID, "", timeline.MaxListMembers)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ListMembers = %v, want %v", got, want)
	}
}

func (e *timelineEnv) listTimeline(t *testing.T, viewer, listID string, want ...string) {
	t.Helper()
	p, err := e.svc.ListTimeline(t.Context(), viewerOf(viewer), listID, apptimeline.Query{Limit: apptimeline.MaxLimit})
	expectPage(t, "ListTimeline", p, err, want...)
}

func listIDs(ls []apptimeline.List) []string {
	out := []string{}
	for _, l := range ls {
		out = append(out, l.ID)
	}
	return out
}

func timelineListCommands(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob := e.account(true), e.account(true)
	if _, err := e.svc.CreateList(ctx, alice, "  "); !errors.Is(err, timeline.ErrInvalidListName) {
		t.Errorf("CreateList(blank) = %v, want ErrInvalidListName", err)
	}
	if _, err := e.svc.CreateList(ctx, alice, strings.Repeat("あ", timeline.MaxListNameGraphemes+1)); !errors.Is(err, timeline.ErrInvalidListName) {
		t.Errorf("CreateList(too long) = %v, want ErrInvalidListName", err)
	}
	friends := e.createList(t, alice, " friends ")
	if friends.Name != "friends" || friends.OwnerAccountID != alice {
		t.Errorf("CreateList = %+v, want the trimmed name and the owner", friends)
	}
	e.clock.Advance(time.Millisecond)
	work := e.createList(t, alice, strings.Repeat("🇯🇵", timeline.MaxListNameGraphemes))
	renamed, err := e.svc.RenameList(ctx, alice, friends.ID, "close friends")
	if err != nil || renamed.Name != "close friends" {
		t.Fatalf("RenameList = %+v, %v", renamed, err)
	}
	lists, err := e.svc.Lists(ctx, alice)
	if err != nil || !slices.Equal(listIDs(lists), []string{friends.ID, work.ID}) || lists[0].Name != "close friends" {
		t.Errorf("Lists = %+v, %v; want both, oldest first, with the new name", lists, err)
	}
	if got, err := e.svc.List(ctx, alice, work.ID); err != nil || got.ID != work.ID {
		t.Errorf("List = %+v, %v", got, err)
	}

	// Another account sees none of alice's lists, as if they did not exist.
	notFound := func(what string, err error) {
		t.Helper()
		if !errors.Is(err, apptimeline.ErrNotFound) {
			t.Errorf("%s by another account = %v, want ErrNotFound", what, err)
		}
	}
	_, err = e.svc.List(ctx, bob, friends.ID)
	notFound("List", err)
	_, err = e.svc.RenameList(ctx, bob, friends.ID, "mine")
	notFound("RenameList", err)
	notFound("DeleteList", e.svc.DeleteList(ctx, bob, friends.ID))
	notFound("AddListMember", e.svc.AddListMember(ctx, bob, friends.ID, alice))
	notFound("RemoveListMember", e.svc.RemoveListMember(ctx, bob, friends.ID, alice))
	_, err = e.svc.ListMembers(ctx, bob, friends.ID, "", 10)
	notFound("ListMembers", err)
	_, err = e.svc.ListTimeline(ctx, viewerOf(bob), friends.ID, apptimeline.Query{})
	notFound("ListTimeline", err)
	if lists, err := e.svc.Lists(ctx, bob); err != nil || len(lists) != 0 {
		t.Errorf("Lists(bob) = %+v, %v; want none", lists, err)
	}
	_, err = e.svc.List(ctx, alice, e.ids.NewID())
	notFound("List of an unknown ID", err)

	// Deleting a list deletes its members with it.
	e.follow(t, alice, bob)
	e.addMember(t, alice, work.ID, bob)
	if err := e.svc.DeleteList(ctx, alice, work.ID); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}
	_, err = e.svc.List(ctx, alice, work.ID)
	notFound("List after DeleteList", err)
	if ls, err := e.svc.ListsWithMember(ctx, alice, bob); err != nil || len(ls) != 0 {
		t.Errorf("ListsWithMember after DeleteList = %+v, %v; want none", ls, err)
	}
}

func timelineListMembers(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob, carol, stranger := e.account(true), e.account(true), e.account(false), e.account(true)
	locked := e.account(true)
	a, _ := e.actors.FindByID(ctx, locked)
	a.Profile = &account.ProfileRevision{AccountID: locked, Locked: true}
	e.actors.set(a)
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	e.follow(t, alice, locked) // pending until locked accepts
	friends, others := e.createList(t, alice, "friends"), e.createList(t, alice, "others")

	// Only the accounts that alice actively follows, local or remote, can join: not one she does
	// not follow, nor one whose follow is pending (a locked account, a remote one that has not
	// accepted yet), nor herself.
	for _, who := range []string{stranger, locked, carol, alice} {
		if err := e.svc.AddListMember(ctx, alice, friends.ID, who); !errors.Is(err, timeline.ErrNotFollowed) {
			t.Errorf("AddListMember(not actively followed) = %v, want ErrNotFollowed", err)
		}
	}
	e.acceptRemote(t, alice, carol)
	e.addMember(t, alice, friends.ID, bob)
	e.addMember(t, alice, friends.ID, bob) // again: nothing changes
	e.addMember(t, alice, friends.ID, carol)
	e.addMember(t, alice, others.ID, bob)
	e.members(t, alice, friends.ID, bob, carol)
	if l, err := e.svc.List(ctx, alice, friends.ID); err != nil || l.MembersCount != 2 {
		t.Errorf("List = %+v, %v; want 2 members", l, err)
	}
	if ls, err := e.svc.Lists(ctx, alice); err != nil || len(ls) != 2 || ls[0].MembersCount != 2 || ls[1].MembersCount != 1 {
		t.Errorf("Lists = %+v, %v; want 2 and 1 members", ls, err)
	}
	if _, err := e.graph.AcceptFollowRequest(ctx, locked, alice); err != nil {
		t.Fatalf("AcceptFollowRequest: %v", err)
	}
	e.addMember(t, alice, others.ID, locked)
	e.members(t, alice, others.ID, bob, locked)
	if ls, err := e.svc.ListsWithMember(ctx, alice, bob); err != nil || !slices.Equal(listIDs(ls), []string{friends.ID, others.ID}) {
		t.Errorf("ListsWithMember(bob) = %+v, %v; want both lists", ls, err)
	}
	// Pages of members, in the order of their IDs.
	first, err := e.svc.ListMembers(ctx, alice, friends.ID, "", 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("ListMembers(limit 1) = %v, %v", first, err)
	}
	rest, err := e.svc.ListMembers(ctx, alice, friends.ID, first[0], 10)
	if err != nil || len(rest) != 1 || rest[0] <= first[0] {
		t.Errorf("ListMembers(after %s) = %v, %v; want the other member", first[0], rest, err)
	}

	if err := e.svc.RemoveListMember(ctx, alice, friends.ID, carol); err != nil {
		t.Fatalf("RemoveListMember: %v", err)
	}
	if err := e.svc.RemoveListMember(ctx, alice, friends.ID, carol); err != nil {
		t.Fatalf("RemoveListMember again: %v", err)
	}
	e.members(t, alice, friends.ID, bob)

	// The end of a follow takes the followee out of every list of the follower, and following
	// again does not bring it back.
	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	e.members(t, alice, friends.ID)
	e.members(t, alice, others.ID, locked)
	e.follow(t, alice, bob)
	e.members(t, alice, friends.ID)
	// A block ends the follow too.
	if _, err := e.graph.Block(ctx, locked, alice); err != nil {
		t.Fatalf("Block: %v", err)
	}
	e.members(t, alice, others.ID)
}

func timelineListLimits(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	var lists []apptimeline.List
	for i := range timeline.MaxLists {
		lists = append(lists, e.createList(t, alice, fmt.Sprintf("list %d", i)))
	}
	if _, err := e.svc.CreateList(ctx, alice, "one more"); !errors.Is(err, timeline.ErrTooManyLists) {
		t.Errorf("CreateList beyond the limit = %v, want ErrTooManyLists", err)
	}
	e.createList(t, bob, "bob's") // the limit is per owner

	// A full list refuses one more member and still accepts one it has.
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	full := lists[0].ID
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for i := range timeline.MaxListMembers - 1 {
			_, err := e.h.Timeline.InsertListMember(ctx, tx, apptimeline.ListMember{
				ListID: full, MemberAccountID: e.ids.NewID(), OwnerAccountID: alice, AddedAt: e.clock.Now().Add(time.Duration(i)),
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fill the list: %v", err)
	}
	e.addMember(t, alice, full, bob)
	if err := e.svc.AddListMember(ctx, alice, full, carol); !errors.Is(err, timeline.ErrTooManyListMembers) {
		t.Errorf("AddListMember beyond the limit = %v, want ErrTooManyListMembers", err)
	}
	e.addMember(t, alice, full, bob)
	if ls, err := e.svc.ListsWithMember(ctx, alice, carol); err != nil || len(ls) != 0 {
		t.Errorf("ListsWithMember(carol) = %+v, %v; want none", ls, err)
	}
}

func timelineListTimeline(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob, carol, dave, eve := e.account(true), e.account(true), e.account(true), e.account(false), e.account(true)
	for _, who := range []string{bob, carol, dave, eve} {
		e.follow(t, alice, who)
	}
	e.acceptRemote(t, alice, dave)
	l := e.createList(t, alice, "friends")
	for _, who := range []string{bob, carol, dave} {
		e.addMember(t, alice, l.ID, who)
	}
	other := e.createList(t, alice, "other")
	e.addMember(t, alice, other.ID, bob)

	var want []string
	for _, v := range []content.Visibility{content.VisibilityPublic, content.VisibilityLocal, content.VisibilityUnlisted, content.VisibilityPrivate} {
		want = append(want, e.post(t, bob, v, postOpts{}).ID)
	}
	toAlice := e.post(t, bob, content.VisibilityDirect, postOpts{mentions: []string{alice}})
	e.post(t, bob, content.VisibilityDirect, postOpts{mentions: []string{eve}}) // not to alice
	carols := e.public(t, carol)
	daves := e.public(t, dave)
	eves := e.public(t, eve) // eve is no member
	own := e.public(t, alice)
	bobBoostsEve := e.boost(t, bob, eves)
	e.boost(t, eve, carols) // eve is no member
	e.boost(t, carol, own)  // found in the page below
	want = append(want, toAlice.ID, carols.ID, daves.ID, bobBoostsEve.ID)
	page, err := e.svc.ListTimeline(ctx, viewerOf(alice), l.ID, apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	// The list never shows alice's own posts, but a member's boost of one is the member's entry.
	var boostOfOwn string
	for _, en := range page.Entries {
		if en.Kind == timeline.KindBoost && en.StatusID == own.ID {
			boostOfOwn = en.ID
		}
		if en.ID == own.ID {
			t.Errorf("ListTimeline lists the owner's own post")
		}
	}
	if boostOfOwn == "" {
		t.Fatalf("ListTimeline lacks carol's boost of alice's post")
	}
	want = append(want, boostOfOwn)
	e.listTimeline(t, alice, l.ID, want...)

	// The follow's show_boosts applies.
	if _, err := e.graph.UpdatePreferences(ctx, alice, bob, appgraph.PreferencesInput{ShowBoosts: false}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	without := slices.DeleteFunc(slices.Clone(want), func(id string) bool { return id == bobBoostsEve.ID })
	e.listTimeline(t, alice, l.ID, without...)
	if _, err := e.graph.UpdatePreferences(ctx, alice, bob, appgraph.PreferencesInput{ShowBoosts: true}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}

	// A mute hides the member, and a block hides the boosts of the blocked account's posts.
	if _, err := e.graph.Mute(ctx, alice, carol, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	unmuted := slices.DeleteFunc(slices.Clone(want), func(id string) bool { return id == carols.ID || id == boostOfOwn })
	e.listTimeline(t, alice, l.ID, unmuted...)
	if _, err := e.graph.Unmute(ctx, alice, carol); err != nil {
		t.Fatalf("Unmute: %v", err)
	}
	if _, err := e.graph.Block(ctx, alice, eve); err != nil {
		t.Fatalf("Block: %v", err)
	}
	unblocked := slices.DeleteFunc(slices.Clone(want), func(id string) bool { return id == bobBoostsEve.ID })
	e.listTimeline(t, alice, l.ID, unblocked...)
	// A domain mute hides the boosts of the domain's posts, not the posts of the members there.
	if _, err := e.graph.MuteDomain(ctx, alice, timelineRemoteDomain); err != nil {
		t.Fatalf("MuteDomain: %v", err)
	}
	e.listTimeline(t, alice, l.ID, unblocked...)

	// The lists of a home entry for the streams: those that have its author.
	for _, tc := range []struct {
		entry string
		want  []string
	}{{carols.ID, []string{l.ID}}, {want[0], []string{l.ID, other.ID}}, {own.ID, nil}} {
		en, ok, err := e.svc.HomeEntry(ctx, viewerOf(alice), tc.entry, apptimeline.Query{})
		if err != nil || !ok {
			t.Fatalf("HomeEntry(%s) = %v, %v", tc.entry, ok, err)
		}
		got, err := e.svc.EntryLists(ctx, viewerOf(alice), en)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("EntryLists(%s) = %v, %v; want %v", tc.entry, got, err, want)
		}
	}
	if _, err := e.svc.ListTimeline(ctx, anonymous, l.ID, apptimeline.Query{}); !errors.Is(err, apptimeline.ErrLoginRequired) {
		t.Errorf("ListTimeline(anonymous) = %v, want ErrLoginRequired", err)
	}
}

// timelineListRebuild checks that a rebuild of the projections keeps the lists and their members
// whose follows stay, and takes out a member whose follow the events no longer hold.
func timelineListRebuild(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	l := e.createList(t, alice, "friends")
	e.addMember(t, alice, l.ID, bob)
	e.addMember(t, alice, l.ID, carol)
	// A member row without a follow, as a projection that drifted would leave.
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.h.Timeline.InsertListMember(ctx, tx, apptimeline.ListMember{
			ListID: l.ID, MemberAccountID: e.ids.NewID(), OwnerAccountID: alice, AddedAt: e.clock.Now(),
		})
		return err
	})
	if err != nil {
		t.Fatalf("InsertListMember: %v", err)
	}
	if err := apptimeline.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Timeline,
		apptimeline.ContentSource(e.h.Statuses, e.h.Interactions), apptimeline.GraphSource(e.h.Graph)); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	e.members(t, alice, l.ID, bob, carol)
	all, err := e.h.Timeline.AllListMembers(ctx)
	if err != nil || len(all) != 2 {
		t.Errorf("AllListMembers = %+v, %v; want bob and carol", all, err)
	}
}
