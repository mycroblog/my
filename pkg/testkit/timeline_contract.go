// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// TimelineHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the repositories of the modules whose events feed the timelines, with the
// timeline repository itself.
type TimelineHarness struct {
	UnitOfWork   uow.UnitOfWork
	Statuses     appcontent.Repository
	Interactions appcontent.InteractionRepository
	Graph        appgraph.Repository
	Timeline     apptimeline.Repository
}

// TimelineFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type TimelineFactory func(t *testing.T, opts uow.Options) TimelineHarness

// TimelineContract verifies the timeline repository of a database adapter together with the
// timeline module's projectors and Service, driven by the content and graph modules' commands:
// the home timeline (follows, own posts and boosts, show_boosts, direct posts, the read window
// and its widening), the local and hashtag timelines (local and anonymous viewers, discovery
// restrictions of accounts and domains, projected from the moderation module's events), the
// profile timeline (the visibilities each relation
// allows), blocks and mutes (with expiry) everywhere, domain mutes (off the timelines other than
// home, with the boosts of their posts off home), the language filter, keyset paging,
// deletions of statuses and of accounts (local and remote), the tag_usage projection and the prefix
// search of hashtags over it (ADR-0092 §1), the property that incrementally updated projections equal a rebuild from the
// events the content and graph modules replay, and the lists with their timelines (ADR-0122, timeline_lists_contract.go).
func TimelineContract(t *testing.T, newHarness TimelineFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *timelineEnv)
	}{
		{"HomeShowsOwnAndFollowedPosts", timelineHome},
		{"HomeShowsBoostsUnlessHidden", timelineHomeBoosts},
		{"HomeWidensTheReadWindow", timelineHomeWindow},
		{"ExcludesBlocksAndMutes", timelineExclusions},
		{"ExcludesMutedDomains", timelineDomainMutes},
		{"LocalTimeline", timelineLocal},
		{"PublicTimeline", timelinePublic},
		{"PublicTimelineEntriesForStreams", timelinePublicEntries},
		{"HashtagTimeline", timelineHashtag},
		{"ProfileTimeline", timelineProfile},
		{"ProfileLeavesOutRepliesAndBoosts", timelineProfileFilters},
		{"FiltersLanguages", timelineLanguages},
		{"AppliesModerationRestrictions", timelineModeration},
		{"PagesByKeyset", timelinePaging},
		{"ForgetsDeletedStatuses", timelineDeletes},
		{"HidesDeletedAuthors", timelineHidesDeletedAuthors},
		{"CountsTagUsage", timelineTagUsage},
		{"SearchesTagsByPrefix", timelineTagSearch},
		{"IncrementalProjectionEqualsRebuild", timelineProjectionEqualsRebuild},
		{"ListCommands", timelineListCommands},
		{"ListMembersAreActiveFollows", timelineListMembers},
		{"ListLimits", timelineListLimits},
		{"ListTimeline", timelineListTimeline},
		{"RebuildKeepsListsOfFollows", timelineListRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newTimelineEnv(t, newHarness)) })
	}
}

// timelineEnv drives one harness with deterministic time and IDs: statuses through the
// statusEnv commands, boosts through the Interactions use cases, follows, blocks, and mutes
// through the graph Service, and the timelines through the timeline Service.
type timelineEnv struct {
	*statusEnv
	h            TimelineHarness
	actors       *stubActors
	graph        *appgraph.Service
	interactions *appcontent.Interactions
	svc          *apptimeline.Service
}

func newTimelineEnv(t *testing.T, newHarness TimelineFactory) *timelineEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appcontent.RegisterProjectors(projectors, h.Statuses)
	appcontent.RegisterInteractionProjectors(projectors, h.Interactions)
	appgraph.RegisterProjectors(projectors, h.Graph)
	apptimeline.RegisterProjectors(projectors, h.Timeline)
	clock := NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	ids := NewIDGenerator(clock)
	e := &timelineEnv{
		statusEnv: &statusEnv{h: StatusHarness{UnitOfWork: h.UnitOfWork, Statuses: h.Statuses}, clock: clock, ids: ids},
		h:         h,
		actors:    &stubActors{actors: map[string]appaccount.Actor{}},
	}
	e.graph = appgraph.NewService(appgraph.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Graph, Accounts: e.actors, Clock: clock, IDs: ids, Host: contractHost,
	})
	e.interactions = appcontent.NewInteractions(appcontent.InteractionDeps{
		UnitOfWork: h.UnitOfWork, Statuses: h.Statuses, Interactions: h.Interactions,
		Accounts: e.actors, Relationships: e.graph, Clock: clock, IDs: ids, Host: contractHost,
	})
	apptimeline.RegisterModerationProjectors(projectors, h.Timeline, e.actors)
	e.svc = apptimeline.NewService(apptimeline.Deps{UnitOfWork: h.UnitOfWork, Repository: h.Timeline, Clock: clock, IDs: ids})
	return e
}

const timelineRemoteDomain = "remote.example"

// account registers an active account with the stub lookup and returns its ID.
func (e *timelineEnv) account(local bool) string {
	id := e.ids.NewID()
	domain := account.LocalDomain
	if !local {
		domain = timelineRemoteDomain
	}
	e.actors.set(appaccount.Actor{
		Account:   account.Account{ID: id, Domain: domain},
		Lifecycle: account.LifecycleState{AccountID: id, State: account.StateActive},
	})
	return id
}

// postOpts are the optional parts of a post.
type postOpts struct {
	mentions []string
	tags     []string
	language string
	// inReplyTo is the URI of the status replied to, "" for none.
	inReplyTo string
}

// post creates a status of author, remote when the author is, and returns it.
func (e *timelineEnv) post(t *testing.T, author string, v content.Visibility, o postOpts) content.Status {
	t.Helper()
	s := e.newStatus(author, v)
	s.InReplyToURI = o.inReplyTo
	if !e.actors.local(author) {
		s.IsLocal = false
		s.URI = "https://" + timelineRemoteDomain + "/statuses/" + s.ID
	}
	rev := e.firstRevision(s, o.mentions...)
	rev.TagIDs = o.tags
	if o.language != "" {
		rev.Language = o.language
	}
	if err := e.create(t, s, nil, rev); err != nil {
		t.Fatalf("create status: %v", err)
	}
	return s
}

func (e *timelineEnv) public(t *testing.T, author string) content.Status {
	t.Helper()
	return e.post(t, author, content.VisibilityPublic, postOpts{})
}

// reviseWith inserts the next revision of a status with the given language, mentions, and tags.
func (e *timelineEnv) reviseWith(t *testing.T, s content.Status, o postOpts) {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	rev := content.Revision{
		ID: e.ids.NewID(), StatusID: s.ID, Language: o.language, RevisedAt: e.clock.Now(),
		Bodies:              []content.Body{{Language: o.language, ContentHTML: "<p>edited</p>"}},
		MentionedAccountIDs: o.mentions, TagIDs: o.tags,
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		head, err := e.h.Statuses.Head(t.Context(), tx, s.ID)
		if err != nil {
			return err
		}
		rev.Seq = head.Seq + 1
		if err := e.h.Statuses.InsertRevision(t.Context(), tx, rev); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.StatusRevised{Status: s, Revision: rev})
	})
	if err != nil {
		t.Fatalf("revise status %s: %v", s.ID, err)
	}
}

func (e *timelineEnv) tag(t *testing.T, name string) string {
	t.Helper()
	var tag content.Tag
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		tag, err = e.h.Statuses.EnsureTag(t.Context(), tx, content.Tag{ID: e.ids.NewID(), Name: name, NameNormalized: appcontent.NormalizeTag(name), CreatedAt: e.clock.Now()})
		return err
	})
	if err != nil {
		t.Fatalf("EnsureTag(%s): %v", name, err)
	}
	return tag.ID
}

func (e *timelineEnv) boost(t *testing.T, booster string, s content.Status) content.Boost {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	b, err := e.interactions.Boost(t.Context(), booster, s.ID)
	if err != nil {
		t.Fatalf("Boost(%s, %s): %v", booster, s.ID, err)
	}
	return b
}

func (e *timelineEnv) follow(t *testing.T, follower, followee string) {
	t.Helper()
	if _, err := e.graph.Follow(t.Context(), follower, followee); err != nil {
		t.Fatalf("Follow(%s, %s): %v", follower, followee, err)
	}
}

func (e *timelineEnv) restrict(t *testing.T, accountID string, r timeline.Restriction) {
	t.Helper()
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.h.Timeline.UpsertAuthorRestriction(t.Context(), tx, apptimeline.AuthorRestriction{AccountID: accountID, Restriction: r})
	})
	if err != nil {
		t.Fatalf("UpsertAuthorRestriction: %v", err)
	}
}

func viewerOf(accountID string) appcontent.Viewer {
	return appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: accountID}
}

var anonymous = appcontent.Viewer{Kind: appcontent.ViewerAnonymous}

// ids returns the entry IDs of a page.
func entryIDs(p apptimeline.Page) []string {
	out := []string{}
	for _, e := range p.Entries {
		out = append(out, e.ID)
	}
	return out
}

// newestFirst returns the IDs in descending order, as timelines list them.
func newestFirst(ids ...string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	slices.Reverse(out)
	return append([]string{}, out...)
}

func expectPage(t *testing.T, what string, p apptimeline.Page, err error, want ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if got := entryIDs(p); !slices.Equal(got, newestFirst(want...)) {
		t.Errorf("%s = %v, want %v", what, got, newestFirst(want...))
	}
}

func (e *timelineEnv) home(t *testing.T, viewer string, want ...string) {
	t.Helper()
	p, err := e.svc.Home(t.Context(), viewerOf(viewer), apptimeline.Query{Limit: apptimeline.MaxLimit})
	expectPage(t, "Home", p, err, want...)
}

func timelineHome(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	remote := e.account(false)
	e.follow(t, alice, bob)
	own := e.public(t, alice)
	var bobs []string
	for _, v := range []content.Visibility{content.VisibilityPublic, content.VisibilityLocal, content.VisibilityUnlisted, content.VisibilityPrivate} {
		bobs = append(bobs, e.post(t, bob, v, postOpts{}).ID)
	}
	toAlice := e.post(t, bob, content.VisibilityDirect, postOpts{mentions: []string{alice}})
	e.post(t, bob, content.VisibilityDirect, postOpts{mentions: []string{carol}})
	carols := e.public(t, carol)
	e.public(t, remote)
	ownDirect := e.post(t, alice, content.VisibilityDirect, postOpts{mentions: []string{carol}})

	e.home(t, alice, append(bobs, own.ID, toAlice.ID, ownDirect.ID)...)
	if _, err := e.svc.Home(t.Context(), anonymous, apptimeline.Query{}); !errors.Is(err, apptimeline.ErrLoginRequired) {
		t.Errorf("Home(anonymous) = %v, want ErrLoginRequired", err)
	}
	// A pending follow shows nothing; accepting it shows the followee's posts.
	locked := e.account(true)
	a, _ := e.actors.FindByID(t.Context(), locked)
	a.Profile = &account.ProfileRevision{AccountID: locked, Locked: true}
	e.actors.set(a)
	lockedPost := e.post(t, locked, content.VisibilityPrivate, postOpts{})
	e.follow(t, carol, locked)
	e.home(t, carol, carols.ID)
	if _, err := e.graph.AcceptFollowRequest(t.Context(), locked, carol); err != nil {
		t.Fatalf("AcceptFollowRequest: %v", err)
	}
	e.home(t, carol, carols.ID, lockedPost.ID)
	if _, err := e.graph.Unfollow(t.Context(), carol, locked); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	e.home(t, carol, carols.ID)
}

func timelineHomeBoosts(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	e.follow(t, alice, bob)
	s := e.public(t, carol)
	b := e.boost(t, bob, s)
	own := e.boost(t, alice, e.public(t, carol))
	e.home(t, alice, b.ID, own.ID)

	if _, err := e.graph.UpdatePreferences(t.Context(), alice, bob, appgraph.PreferencesInput{ShowBoosts: false}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	e.home(t, alice, own.ID)
	if _, err := e.graph.UpdatePreferences(t.Context(), alice, bob, appgraph.PreferencesInput{ShowBoosts: true}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	e.home(t, alice, b.ID, own.ID)
	if err := e.interactions.Unboost(t.Context(), bob, s.ID); err != nil {
		t.Fatalf("Unboost: %v", err)
	}
	e.home(t, alice, own.ID)
}

func timelineHomeWindow(t *testing.T, e *timelineEnv) {
	alice, bob := e.account(true), e.account(true)
	e.follow(t, alice, bob)
	start := e.clock.Now()
	var want []string
	// Posts far apart: a year ago, 100 days ago, 20 days ago, and now. Each widening of the
	// read window reaches further back.
	for _, ago := range []time.Duration{365 * 24 * time.Hour, 100 * 24 * time.Hour, 20 * 24 * time.Hour, 0} {
		e.clock.Set(start.Add(-ago))
		want = append(want, e.public(t, bob).ID)
	}
	e.clock.Set(start.Add(time.Minute))
	e.home(t, alice, want...)
	// Paging from an old cursor reads the window before the cursor, not before now.
	var got []string
	q := apptimeline.Query{Limit: 1}
	for range len(want) + 1 {
		p, err := e.svc.Home(t.Context(), viewerOf(alice), q)
		if err != nil {
			t.Fatalf("Home: %v", err)
		}
		got = append(got, entryIDs(p)...)
		if p.Next == "" {
			break
		}
		q.Before = p.Next
	}
	if !slices.Equal(got, newestFirst(want...)) {
		t.Errorf("Home pages = %v, want %v", got, newestFirst(want...))
	}
}

func timelineExclusions(t *testing.T, e *timelineEnv) {
	alice, bob, carol, dave := e.account(true), e.account(true), e.account(true), e.account(true)
	e.follow(t, alice, bob)
	e.follow(t, alice, dave)
	bobs := e.public(t, bob)
	carols := e.public(t, carol)
	daveBoostsBob := e.boost(t, dave, bobs)
	daveBoostsCarol := e.boost(t, dave, carols)
	davePost := e.public(t, dave)
	all := []string{bobs.ID, daveBoostsBob.ID, daveBoostsCarol.ID, davePost.ID}
	e.home(t, alice, all...)
	local := func(viewer string, want ...string) {
		t.Helper()
		p, err := e.svc.Local(t.Context(), viewerOf(viewer), apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Local", p, err, want...)
	}
	local(alice, bobs.ID, carols.ID, davePost.ID)

	// A mute hides the account's posts and boosts of its posts, until it expires.
	if _, err := e.graph.Mute(t.Context(), alice, bob, appgraph.MuteInput{Duration: time.Hour}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	e.home(t, alice, daveBoostsCarol.ID, davePost.ID)
	local(alice, carols.ID, davePost.ID)
	e.clock.Advance(2 * time.Hour)
	e.home(t, alice, all...)
	if _, err := e.graph.Mute(t.Context(), alice, carol, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	e.home(t, alice, bobs.ID, daveBoostsBob.ID, davePost.ID)
	if _, err := e.graph.Unmute(t.Context(), alice, carol); err != nil {
		t.Fatalf("Unmute: %v", err)
	}
	e.home(t, alice, all...)

	// A block either way hides the account, and the block ends the follows between them.
	if _, err := e.graph.Block(t.Context(), alice, carol); err != nil {
		t.Fatalf("Block: %v", err)
	}
	e.home(t, alice, bobs.ID, daveBoostsBob.ID, davePost.ID)
	local(alice, bobs.ID, davePost.ID)
	local(carol, carols.ID, davePost.ID, bobs.ID)
	if _, err := e.graph.Unblock(t.Context(), alice, carol); err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	e.home(t, alice, all...)
	if _, err := e.graph.Block(t.Context(), bob, alice); err != nil {
		t.Fatalf("Block: %v", err)
	}
	e.home(t, alice, daveBoostsCarol.ID, davePost.ID)
	local(alice, carols.ID, davePost.ID)
	local(dave, bobs.ID, carols.ID, davePost.ID)
	// An account restricted by profile_view leaves every timeline.
	e.restrict(t, dave, timeline.RestrictProfileView)
	e.home(t, alice)
	local(alice, carols.ID)
}

func timelineDomainMutes(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob := e.account(true), e.account(true)
	followed, other := e.account(false), e.account(false)
	e.follow(t, alice, bob)
	e.follow(t, alice, followed)
	// A follow of a remote account is active once its server accepts it.
	err := e.graph.AcceptRemote(ctx, appgraph.RemoteResponse{FolloweeID: followed, FollowerID: alice, ActivityURI: "https://remote.example/accept"})
	if err != nil {
		t.Fatalf("AcceptRemote: %v", err)
	}
	cats := e.tag(t, "cats")
	near := e.post(t, bob, content.VisibilityPublic, postOpts{tags: []string{cats}})
	farFollowed := e.public(t, followed)
	far := e.post(t, other, content.VisibilityPublic, postOpts{tags: []string{cats}})
	bobBoostsFar := e.boost(t, bob, far)
	q := apptimeline.Query{Limit: apptimeline.MaxLimit}
	var p apptimeline.Page
	check := func(want map[string][]string) {
		t.Helper()
		e.home(t, alice, want["home"]...)
		p, err := e.svc.Public(ctx, viewerOf(alice), q)
		expectPage(t, "Public", p, err, want["public"]...)
		p, err = e.svc.Hashtag(ctx, viewerOf(alice), cats, q)
		expectPage(t, "Hashtag", p, err, want["tag"]...)
		p, err = e.svc.Profile(ctx, viewerOf(alice), bob, apptimeline.ProfileFilter{}, q)
		expectPage(t, "Profile of bob", p, err, want["bob"]...)
		p, err = e.svc.Profile(ctx, viewerOf(alice), other, apptimeline.ProfileFilter{}, q)
		expectPage(t, "Profile of other", p, err, want["other"]...)
	}
	unmuted := map[string][]string{
		"home":   {near.ID, farFollowed.ID, bobBoostsFar.ID},
		"public": {near.ID, farFollowed.ID, far.ID},
		"tag":    {near.ID, far.ID},
		"bob":    {near.ID, bobBoostsFar.ID},
		"other":  {far.ID},
	}
	check(unmuted)

	// A mute of a parent domain covers the remote accounts' domain. The followed account's posts
	// stay on home, but no boost of a post of the domain does; the account's own profile still
	// lists its posts.
	if _, err := e.graph.MuteDomain(ctx, alice, "example"); err != nil {
		t.Fatalf("MuteDomain: %v", err)
	}
	check(map[string][]string{
		"home":   {near.ID, farFollowed.ID},
		"public": {near.ID},
		"tag":    {near.ID},
		"bob":    {near.ID},
		"other":  {far.ID},
	})
	// Other viewers are not affected.
	p, err = e.svc.Public(ctx, viewerOf(bob), q)
	expectPage(t, "Public of bob", p, err, near.ID, farFollowed.ID, far.ID)

	// A mute of an unrelated domain hides nothing more, and a domain only ends with a label.
	if _, err := e.graph.MuteDomain(ctx, alice, "ote.example"); err != nil {
		t.Fatalf("MuteDomain: %v", err)
	}
	if err := e.graph.UnmuteDomain(ctx, alice, "example"); err != nil {
		t.Fatalf("UnmuteDomain: %v", err)
	}
	check(unmuted)
}

func timelineLocal(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	remote := e.account(false)
	pub := e.public(t, alice)
	loc := e.post(t, alice, content.VisibilityLocal, postOpts{})
	for _, v := range []content.Visibility{content.VisibilityUnlisted, content.VisibilityPrivate, content.VisibilityDirect} {
		e.post(t, alice, v, postOpts{})
	}
	e.public(t, remote)
	e.boost(t, bob, pub)
	silenced := e.public(t, carol)
	hidden := e.public(t, carol)
	local := func(viewer appcontent.Viewer, want ...string) {
		t.Helper()
		p, err := e.svc.Local(t.Context(), viewer, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Local", p, err, want...)
	}
	local(viewerOf(bob), pub.ID, loc.ID, silenced.ID, hidden.ID)
	local(anonymous, pub.ID, silenced.ID, hidden.ID)
	for _, r := range []timeline.Restriction{timeline.RestrictPublicDiscovery, timeline.RestrictVisibilityCeilingPrivate} {
		e.restrict(t, carol, r)
		local(viewerOf(bob), pub.ID, loc.ID)
		local(anonymous, pub.ID)
		err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			return e.h.Timeline.DeleteAuthorRestriction(t.Context(), tx, carol, r)
		})
		if err != nil {
			t.Fatalf("DeleteAuthorRestriction: %v", err)
		}
	}
	// A restriction that has ended no longer applies.
	ended := e.clock.Now().Add(-time.Second)
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.h.Timeline.UpsertAuthorRestriction(t.Context(), tx, apptimeline.AuthorRestriction{
			AccountID: carol, Restriction: timeline.RestrictPublicDiscovery, EffectiveUntil: &ended,
		})
	})
	if err != nil {
		t.Fatalf("UpsertAuthorRestriction: %v", err)
	}
	local(anonymous, pub.ID, silenced.ID, hidden.ID)
}

func timelinePublic(t *testing.T, e *timelineEnv) {
	alice, bob := e.account(true), e.account(true)
	remote, other := e.account(false), e.account(false)
	pub := e.public(t, alice)
	e.post(t, alice, content.VisibilityLocal, postOpts{})
	far := e.public(t, remote)
	for _, v := range []content.Visibility{content.VisibilityUnlisted, content.VisibilityPrivate, content.VisibilityDirect} {
		e.post(t, alice, v, postOpts{})
		e.post(t, remote, v, postOpts{})
	}
	e.boost(t, bob, far)
	reply := e.post(t, other, content.VisibilityPublic, postOpts{inReplyTo: far.URI})
	public := func(viewer appcontent.Viewer, want ...string) {
		t.Helper()
		p, err := e.svc.Public(t.Context(), viewer, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Public", p, err, want...)
	}
	// Only public posts, local or remote; no local posts even for a logged-in viewer, no boosts.
	public(viewerOf(bob), pub.ID, far.ID, reply.ID)
	public(anonymous, pub.ID, far.ID, reply.ID)

	// The viewer's mutes and blocks apply.
	if _, err := e.graph.Mute(t.Context(), bob, other, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	public(viewerOf(bob), pub.ID, far.ID)
	public(anonymous, pub.ID, far.ID, reply.ID)

	// A domain restriction covers the domain and its subdomains; an author restriction the author.
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.h.Timeline.UpsertDomainRestriction(t.Context(), tx, apptimeline.DomainRestriction{
			Domain: "example", Restriction: timeline.RestrictPublicDiscovery,
		})
	})
	if err != nil {
		t.Fatalf("UpsertDomainRestriction: %v", err)
	}
	public(anonymous, pub.ID)
	e.restrict(t, alice, timeline.RestrictPublicDiscovery)
	public(anonymous)
}

func timelineHashtag(t *testing.T, e *timelineEnv) {
	alice, bob := e.account(true), e.account(true)
	remote := e.account(false)
	cats, dogs := e.tag(t, "cats"), e.tag(t, "dogs")
	pub := e.post(t, alice, content.VisibilityPublic, postOpts{tags: []string{cats}})
	loc := e.post(t, alice, content.VisibilityLocal, postOpts{tags: []string{cats, dogs}})
	e.post(t, alice, content.VisibilityUnlisted, postOpts{tags: []string{cats}})
	e.post(t, alice, content.VisibilityPrivate, postOpts{tags: []string{cats}})
	far := e.post(t, remote, content.VisibilityPublic, postOpts{tags: []string{cats}})
	untagged := e.public(t, alice)
	e.boost(t, bob, pub)
	tagged := func(viewer appcontent.Viewer, tagID string, want ...string) {
		t.Helper()
		p, err := e.svc.Hashtag(t.Context(), viewer, tagID, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Hashtag", p, err, want...)
	}
	tagged(viewerOf(bob), cats, pub.ID, loc.ID, far.ID)
	tagged(anonymous, cats, pub.ID, far.ID)
	tagged(viewerOf(bob), dogs, loc.ID)

	// Editing replaces the tags.
	e.reviseWith(t, loc, postOpts{language: "en", tags: []string{dogs}})
	e.reviseWith(t, untagged, postOpts{language: "en", tags: []string{cats}})
	tagged(viewerOf(bob), cats, pub.ID, untagged.ID, far.ID)

	// A domain restriction covers the domain and its subdomains.
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.h.Timeline.UpsertDomainRestriction(t.Context(), tx, apptimeline.DomainRestriction{
			Domain: "example", Restriction: timeline.RestrictPublicDiscovery,
		})
	})
	if err != nil {
		t.Fatalf("UpsertDomainRestriction: %v", err)
	}
	tagged(anonymous, cats, pub.ID, untagged.ID)
	e.restrict(t, alice, timeline.RestrictPublicDiscovery)
	tagged(viewerOf(bob), cats)
}

func timelineProfile(t *testing.T, e *timelineEnv) {
	author, follower, mentioned, stranger, blocker := e.account(true), e.account(true), e.account(true), e.account(true), e.account(true)
	e.follow(t, follower, author)
	pub := e.public(t, author)
	loc := e.post(t, author, content.VisibilityLocal, postOpts{})
	unl := e.post(t, author, content.VisibilityUnlisted, postOpts{})
	prv := e.post(t, author, content.VisibilityPrivate, postOpts{})
	dir := e.post(t, author, content.VisibilityDirect, postOpts{mentions: []string{mentioned}})
	other := e.public(t, stranger)
	boost := e.boost(t, author, other)
	profile := func(viewer appcontent.Viewer, want ...string) {
		t.Helper()
		p, err := e.svc.Profile(t.Context(), viewer, author, apptimeline.ProfileFilter{}, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Profile", p, err, want...)
	}
	profile(viewerOf(author), pub.ID, loc.ID, unl.ID, prv.ID, dir.ID, boost.ID)
	profile(viewerOf(follower), pub.ID, loc.ID, unl.ID, prv.ID, boost.ID)
	profile(viewerOf(mentioned), pub.ID, loc.ID, unl.ID, dir.ID, boost.ID)
	profile(viewerOf(stranger), pub.ID, loc.ID, unl.ID, boost.ID)
	profile(anonymous, pub.ID, unl.ID, boost.ID)

	// The viewer's exclusions of a boosted author hide the boost.
	if _, err := e.graph.Mute(t.Context(), follower, stranger, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	profile(viewerOf(follower), pub.ID, loc.ID, unl.ID, prv.ID)
	if _, err := e.graph.Block(t.Context(), blocker, author); err != nil {
		t.Fatalf("Block: %v", err)
	}
	profile(viewerOf(blocker))
	// Silencing makes every post followers-only; profile_view hides the profile from others.
	e.restrict(t, author, timeline.RestrictVisibilityCeilingPrivate)
	profile(viewerOf(stranger))
	profile(viewerOf(follower), pub.ID, loc.ID, unl.ID, prv.ID)
	e.restrict(t, author, timeline.RestrictProfileView)
	profile(viewerOf(follower))
	profile(viewerOf(author), pub.ID, loc.ID, unl.ID, prv.ID, dir.ID, boost.ID)
}

func timelineProfileFilters(t *testing.T, e *timelineEnv) {
	author, other := e.account(true), e.account(true)
	first := e.public(t, other)
	pub := e.public(t, author)
	reply := e.post(t, author, content.VisibilityPublic, postOpts{inReplyTo: first.URI})
	boost := e.boost(t, author, first)
	profile := func(f apptimeline.ProfileFilter, want ...string) {
		t.Helper()
		p, err := e.svc.Profile(t.Context(), anonymous, author, f, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, fmt.Sprintf("Profile(%+v)", f), p, err, want...)
	}
	profile(apptimeline.ProfileFilter{}, pub.ID, reply.ID, boost.ID)
	profile(apptimeline.ProfileFilter{ExcludeReplies: true}, pub.ID, boost.ID)
	profile(apptimeline.ProfileFilter{ExcludeBoosts: true}, pub.ID, reply.ID)
	profile(apptimeline.ProfileFilter{ExcludeReplies: true, ExcludeBoosts: true}, pub.ID)
}

func timelineLanguages(t *testing.T, e *timelineEnv) {
	alice := e.account(true)
	en := e.post(t, alice, content.VisibilityPublic, postOpts{language: "en"})
	ja := e.post(t, alice, content.VisibilityPublic, postOpts{language: "ja"})
	und := e.post(t, alice, content.VisibilityPublic, postOpts{language: timeline.UndeterminedLanguage})
	boost := e.boost(t, alice, en)
	local := func(langs []string, want ...string) {
		t.Helper()
		p, err := e.svc.Local(t.Context(), anonymous, apptimeline.Query{Limit: apptimeline.MaxLimit, Languages: langs})
		expectPage(t, "Local", p, err, want...)
	}
	local(nil, en.ID, ja.ID, und.ID)
	local([]string{"ja"}, ja.ID, und.ID)
	local([]string{"en", "ja"}, en.ID, ja.ID, und.ID)
	q := apptimeline.Query{Limit: apptimeline.MaxLimit, Languages: []string{"ja"}}
	p, err := e.svc.Home(t.Context(), viewerOf(alice), q)
	expectPage(t, "Home(ja)", p, err, ja.ID, und.ID)
	// An edit moves the post and its boosts to the new language.
	e.reviseWith(t, en, postOpts{language: "ja"})
	p, err = e.svc.Home(t.Context(), viewerOf(alice), q)
	expectPage(t, "Home(ja) after the edit", p, err, en.ID, ja.ID, und.ID, boost.ID)
}

func timelinePaging(t *testing.T, e *timelineEnv) {
	alice := e.account(true)
	var want []string
	for range 5 {
		want = append(want, e.public(t, alice).ID)
	}
	want = newestFirst(want...)
	var pages [][]string
	q := apptimeline.Query{Limit: 2}
	for range 5 {
		p, err := e.svc.Local(t.Context(), anonymous, q)
		if err != nil {
			t.Fatalf("Local: %v", err)
		}
		pages = append(pages, entryIDs(p))
		if p.Next == "" {
			break
		}
		q.Before = p.Next
	}
	if wantPages := [][]string{want[0:2], want[2:4], want[4:5]}; !reflect.DeepEqual(pages, wantPages) {
		t.Errorf("pages = %v, want %v", pages, wantPages)
	}
}

func timelineDeletes(t *testing.T, e *timelineEnv) {
	alice, bob := e.account(true), e.account(true)
	e.follow(t, bob, alice)
	cats := e.tag(t, "cats")
	s := e.post(t, alice, content.VisibilityPublic, postOpts{tags: []string{cats}})
	d := e.post(t, alice, content.VisibilityDirect, postOpts{mentions: []string{bob}})
	e.boost(t, bob, s)
	e.mustRemove(t, s)
	e.mustRemove(t, d)
	e.home(t, bob)
	snap, err := e.h.Timeline.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Entries)+len(snap.DirectRecipients)+len(snap.TagEntries)+len(snap.TagUsage) != 0 {
		t.Errorf("Snapshot after deleting every status = %+v, want no entries", snap)
	}
	// A late revision of a deleted status changes nothing.
	late := content.Revision{
		ID: e.ids.NewID(), StatusID: s.ID, Seq: 2, Language: "ja", RevisedAt: e.clock.Now(),
		Bodies: []content.Body{{Language: "ja", ContentHTML: "<p>late</p>"}}, TagIDs: []string{cats},
	}
	if err := e.insertRevision(t, late); err != nil {
		t.Fatalf("insert a late revision: %v", err)
	}
	if snap, err := e.h.Timeline.Snapshot(t.Context()); err != nil || len(snap.Entries)+len(snap.TagEntries) != 0 {
		t.Errorf("Snapshot after a late revision = %+v, %v; want no entries", snap, err)
	}
}

// deleteAccount records the deletion of an account, local or on its own server.
func (e *timelineEnv) deleteAccount(t *testing.T, accountID string, kind account.LifecycleKind) {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return tx.Emit(t.Context(), appaccount.LifecycleEventRecorded{Event: account.LifecycleEvent{
			ID: e.ids.NewID(), AccountID: accountID, Kind: kind, InitiatedBy: account.InitiatedByRemote, OccurredAt: e.clock.Now(),
		}})
	})
	if err != nil {
		t.Fatalf("delete account %s: %v", accountID, err)
	}
}

func timelineHidesDeletedAuthors(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	remote := e.account(false)
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	bobs, carols, remotes := e.public(t, bob), e.public(t, carol), e.public(t, remote)
	boost := e.boost(t, carol, remotes)
	profile := func(author string, want ...string) {
		t.Helper()
		p, err := e.svc.Profile(t.Context(), anonymous, author, apptimeline.ProfileFilter{}, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Profile", p, err, want...)
	}
	e.home(t, alice, bobs.ID, carols.ID, boost.ID)
	profile(remote, remotes.ID)

	// A remote actor's deletion hides its posts and the boosts of them at once.
	e.deleteAccount(t, remote, account.LifecycleRemoteDeleted)
	e.home(t, alice, bobs.ID, carols.ID)
	profile(remote)
	profile(carol, carols.ID)
	// So does a local account's deletion, and a deletion recorded again changes nothing.
	for range 2 {
		e.deleteAccount(t, bob, account.LifecycleDeleted)
		e.home(t, alice, carols.ID)
		p, err := e.svc.Local(t.Context(), anonymous, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Local", p, err, carols.ID)
	}
	// Other lifecycle events leave the timelines as they are.
	e.deleteAccount(t, carol, account.LifecycleDeactivated)
	e.home(t, alice, carols.ID)
}

func timelineProjectionEqualsRebuild(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(101, 6)) //nolint:gosec // a fixed seed keeps the test reproducible
	var accounts []string
	for i := range 6 {
		accounts = append(accounts, e.account(i < 5))
	}
	locals := accounts[:5]
	tags := []string{e.tag(t, "a"), e.tag(t, "b"), e.tag(t, "c")}
	var live []content.Status
	pick := func(s []string) string { return s[rng.IntN(len(s))] }
	subset := func(s []string) []string {
		var out []string
		for _, x := range s {
			if rng.IntN(3) == 0 {
				out = append(out, x)
			}
		}
		return out
	}
	visibilities := content.Visibilities()
	for range 200 {
		e.clock.Advance(time.Minute)
		a, b := pick(locals), pick(accounts)
		switch rng.IntN(12) {
		case 0, 1, 2:
			author := pick(accounts)
			v := visibilities[rng.IntN(len(visibilities))]
			if !e.actors.local(author) && v == content.VisibilityLocal {
				v = content.VisibilityPublic
			}
			o := postOpts{mentions: subset(locals), tags: subset(tags), language: pick([]string{"en", "ja", "und"})}
			if len(live) > 0 && rng.IntN(2) == 0 {
				o.inReplyTo = live[rng.IntN(len(live))].URI
			}
			live = append(live, e.post(t, author, v, o))
		case 3:
			if len(live) > 0 {
				s := live[rng.IntN(len(live))]
				e.reviseWith(t, s, postOpts{mentions: subset(locals), tags: subset(tags), language: pick([]string{"en", "ja"})})
			}
		case 4:
			if len(live) > 0 {
				i := rng.IntN(len(live))
				e.mustRemove(t, live[i])
				live = slices.Delete(live, i, i+1)
			}
		case 5, 6:
			if len(live) > 0 {
				_, _ = e.interactions.Boost(ctx, a, live[rng.IntN(len(live))].ID) // may be hidden or not boostable
			}
		case 7:
			if len(live) > 0 {
				_ = e.interactions.Unboost(ctx, a, live[rng.IntN(len(live))].ID)
			}
		case 8:
			if b2 := pick(locals); b2 != a {
				_, _ = e.graph.Follow(ctx, a, b2) // may be blocked
			}
		case 9:
			_, _ = e.graph.Unfollow(ctx, a, pick(locals))
			_, _ = e.graph.UpdatePreferences(ctx, a, pick(locals), appgraph.PreferencesInput{ShowBoosts: rng.IntN(2) == 0})
		case 10:
			if b2 := pick(locals); b2 != a {
				if rng.IntN(2) == 0 {
					_, _ = e.graph.Block(ctx, a, b2)
				} else {
					_, _ = e.graph.Unblock(ctx, a, b2)
				}
			}
		case 11:
			if b != a {
				switch rng.IntN(3) {
				case 0:
					_, _ = e.graph.Mute(ctx, a, b, appgraph.MuteInput{})
				case 1:
					_, _ = e.graph.Mute(ctx, a, b, appgraph.MuteInput{Duration: time.Duration(1+rng.IntN(120)) * time.Minute})
				default:
					_, _ = e.graph.Unmute(ctx, a, b)
				}
			}
			if rng.IntN(2) == 0 {
				_, _ = e.graph.MuteDomain(ctx, a, timelineRemoteDomain)
			} else {
				_ = e.graph.UnmuteDomain(ctx, a, timelineRemoteDomain)
			}
		}
	}
	incremental, err := e.h.Timeline.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(incremental.Entries) == 0 || len(incremental.FollowEdges) == 0 || len(incremental.Exclusions) == 0 ||
		len(incremental.TagEntries) == 0 || len(incremental.TagUsage) == 0 || len(incremental.DirectRecipients) == 0 {
		t.Fatalf("the random history left some projection empty: %+v", incremental)
	}
	sources := []apptimeline.Source{
		apptimeline.ContentSource(e.h.Statuses, e.h.Interactions), apptimeline.GraphSource(e.h.Graph),
	}
	for range 2 {
		if err := apptimeline.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Timeline, sources...); err != nil {
			t.Fatalf("RebuildProjections: %v", err)
		}
		rebuilt, err := e.h.Timeline.Snapshot(ctx)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if !reflect.DeepEqual(rebuilt, incremental) {
			t.Fatalf("rebuilt projection differs from the incremental one:\n rebuilt:     %+v\n incremental: %+v", rebuilt, incremental)
		}
	}
}

// moderate emits the restrictions in effect on an account or a domain, as the moderation module
// does after an action or a lift.
func (e *timelineEnv) moderate(t *testing.T, ev uow.Event) {
	t.Helper()
	if err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return tx.Emit(t.Context(), ev) }); err != nil {
		t.Fatalf("emit %s: %v", ev.EventType(), err)
	}
}

func timelineModeration(t *testing.T, e *timelineEnv) {
	alice, carol := e.account(true), e.account(true)
	remote := e.account(false)
	pub := e.public(t, carol)
	far := e.public(t, remote)
	local := func(want ...string) {
		t.Helper()
		p, err := e.svc.Local(t.Context(), anonymous, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Local", p, err, want...)
	}
	profile := func(want ...string) {
		t.Helper()
		p, err := e.svc.Profile(t.Context(), viewerOf(alice), remote, apptimeline.ProfileFilter{}, apptimeline.Query{Limit: apptimeline.MaxLimit})
		expectPage(t, "Profile", p, err, want...)
	}
	local(pub.ID)
	profile(far.ID)

	// A limit of carol keeps her posts off the local timeline until it is lifted; the
	// restrictions that do not bear on timelines are left out.
	until := e.clock.Now().Add(time.Hour)
	e.moderate(t, appmoderation.AccountRestrictionsChanged{AccountID: carol, Effects: []appmoderation.Effect{
		{Restriction: moderation.RestrictFederation},
		{Restriction: moderation.RestrictPublicDiscovery, Until: &until},
	}})
	local()
	e.moderate(t, appmoderation.AccountRestrictionsChanged{AccountID: carol})
	local(pub.ID)

	// A suspension of the remote account's parent domain hides its profile.
	e.moderate(t, appmoderation.DomainRestrictionsChanged{Domain: "example", Effects: []appmoderation.Effect{
		{Restriction: moderation.RestrictProfileView}, {Restriction: moderation.RestrictFederation},
	}})
	profile()
	e.moderate(t, appmoderation.DomainRestrictionsChanged{Domain: "example"})
	profile(far.ID)
}
