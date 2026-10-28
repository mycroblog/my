// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// NotificationHarness is one database under test: an empty database with the product schema,
// its UnitOfWork, and the identity, account, graph, content, and notification repositories on it.
type NotificationHarness struct {
	AccountHarness
	Graph         appgraph.Repository
	Statuses      appcontent.Repository
	Interactions  appcontent.InteractionRepository
	Notifications appnotification.Repository
}

// NotificationFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type NotificationFactory func(t *testing.T, opts uow.Options) NotificationHarness

// NotificationContract verifies the notification repository of a database adapter together
// with the notification module's projectors and use cases, driven by the real follow, posting,
// and interaction use cases: follow, follow request, and follow acceptance notifications;
// mentions, also those an edit adds; boosts and favourites; never notifying an account of its
// own actions or a remote recipient; the first resolution of a local reporter's report only;
// projecting an event again, also concurrently, without
// duplicates; hiding notifications across blocks and from accounts muted with
// hide_notifications, also those notified before the block or mute, which a lifted block or
// mute shows again; notifying of a source with unsolicited_reach only the accounts that follow
// it; leaving out notifications about deleted statuses; listing with a type filter and keyset paging; the read marker,
// unread count, and dismissals; and the moderator notifications of the first open report of an
// account, of a pending registration, and of an appeal, to the holders of the permission they
// need, emailed by a job to those who get them by email, and left out once the permission is
// gone; the decision on an appeal to the account that appealed; the notification settings, their
// defaults and fixed rows, and the types turned off in the app, which create nothing; and the
// emails of the notifications that stay unread.
func NotificationContract(t *testing.T, newHarness NotificationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *notificationEnv)
	}{
		{"NotifiesFollows", notificationFollows},
		{"NotifiesMentions", notificationMentions},
		{"NotifiesBoostsAndFavouritesOnce", notificationInteractions},
		{"NotifiesLocalRecipientsOnly", notificationLocalOnly},
		{"NotifiesAResolvedReportOnce", notificationReportResolved},
		{"ProjectsAnEventOnce", notificationProjectsOnce},
		{"AgreesOnConcurrentProjections", notificationConcurrent},
		{"HidesBlockedAndMutedSources", notificationHides},
		{"HidesLaterBlocksAndMutes", notificationHidesLater},
		{"HidesMutedDomains", notificationHidesMutedDomains},
		{"LimitsUnsolicitedReach", notificationUnsolicitedReach},
		{"LeavesOutDeletedStatuses", notificationDeletedStatuses},
		{"ListsWithTypesAndPages", notificationLists},
		{"TracksReadMarkersAndDismissals", notificationReads},
		{"NotifiesModerators", notificationModerators},
		{"NotifiesModeratorsOfLimitedMoves", notificationLimitedMoves},
		{"NotifiesAppeals", notificationAppeals},
		{"NotifiesTheServiceClosure", notificationServiceClosure},
		{"NotifiesVoiceRooms", notificationVoiceRooms},
		{"NotifiesErrorGroups", notificationErrorGroups},
		{"AppliesNotificationSettings", notificationSettings},
		{"EmailsUnreadNotifications", notificationEmails},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newNotificationEnv(t, newHarness)) })
	}
}

type notificationEnv struct {
	*accountEnv
	nh           NotificationHarness
	graph        *appgraph.Service
	content      *appcontent.Service
	interactions *appcontent.Interactions
	svc          *appnotification.Service
	reach        *contractReach
	// only holds just the notification projectors, for projecting an event again.
	only *uow.Projectors
	// staff stands in for the roles and addresses of the moderators and users; outbox keeps the
	// email jobs that the projectors enqueue.
	staff  *stubStaff
	outbox *recordingOutbox
}

func newNotificationEnv(t *testing.T, newHarness NotificationFactory) *notificationEnv {
	t.Helper()
	var (
		nh       NotificationHarness
		captured uow.Options
	)
	ae := newAccountEnv(t, func(t *testing.T, opts uow.Options) AccountHarness {
		captured = opts
		nh = newHarness(t, opts)
		return nh.AccountHarness
	})
	e := &notificationEnv{
		accountEnv: ae, nh: nh, only: &uow.Projectors{}, reach: &contractReach{}, staff: newStubStaff(), outbox: &recordingOutbox{},
	}
	e.graph = appgraph.NewService(appgraph.Deps{
		UnitOfWork: nh.UnitOfWork, Repository: nh.Graph, Accounts: ae.accounts, Clock: ae.clock, IDs: ae.ids, Host: contractHost,
	})
	appgraph.RegisterProjectors(captured.Projectors, nh.Graph)
	appcontent.RegisterProjectors(captured.Projectors, nh.Statuses)
	appcontent.RegisterInteractionProjectors(captured.Projectors, nh.Interactions)
	deps := appnotification.ProjectorDeps{
		Repository: nh.Notifications, Accounts: ae.accounts, Relationships: e.graph, Reach: e.reach, IDs: ae.ids,
		Email: &appnotification.EmailScheduler{Outbox: e.outbox, Clock: ae.clock},
	}
	appnotification.RegisterProjectors(captured.Projectors, deps)
	appnotification.RegisterProjectors(e.only, deps)
	modDeps := appnotification.ModeratorDeps{Repository: nh.Notifications, Staff: e.staff, IDs: ae.ids, Outbox: e.outbox}
	appnotification.RegisterModeratorProjectors(captured.Projectors, modDeps)
	appnotification.RegisterModeratorProjectors(e.only, modDeps)
	e.content = appcontent.NewService(appcontent.Deps{
		UnitOfWork: nh.UnitOfWork, Repository: nh.Statuses, Accounts: ae.accounts, Relationships: e.graph,
		MaxStatusLength: func(context.Context) (int, error) { return 500, nil },
		Clock:           ae.clock, IDs: ae.ids, Host: contractHost,
	})
	e.interactions = appcontent.NewInteractions(appcontent.InteractionDeps{
		UnitOfWork: nh.UnitOfWork, Statuses: nh.Statuses, Interactions: nh.Interactions,
		Accounts: ae.accounts, Relationships: e.graph, Clock: ae.clock, IDs: ae.ids, Host: contractHost,
	})
	e.svc = appnotification.NewService(appnotification.Deps{
		UnitOfWork: nh.UnitOfWork, Repository: nh.Notifications, Clock: ae.clock, Staff: e.staff,
	})
	return e
}

func (e *notificationEnv) user(t *testing.T, username string) string {
	t.Helper()
	return e.register(t, username).AccountID
}

func (e *notificationEnv) lock(t *testing.T, id string) {
	t.Helper()
	if _, err := e.accounts.ReviseProfile(t.Context(), id, appaccount.ProfileInput{Locked: true}); err != nil {
		t.Fatalf("ReviseProfile(%s, locked) = %v", id, err)
	}
}

func (e *notificationEnv) follow(t *testing.T, follower, followee string) {
	t.Helper()
	e.clock.Advance(time.Second)
	if _, err := e.graph.Follow(t.Context(), follower, followee); err != nil {
		t.Fatalf("Follow(%s, %s) = %v", follower, followee, err)
	}
}

func (e *notificationEnv) post(t *testing.T, author, text string) appcontent.Post {
	t.Helper()
	e.clock.Advance(time.Second)
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: text}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create(%q) = %v", text, err)
	}
	return p
}

func (e *notificationEnv) favourite(t *testing.T, accountID, statusID string) {
	t.Helper()
	e.clock.Advance(time.Second)
	if _, err := e.interactions.Favourite(t.Context(), accountID, statusID); err != nil {
		t.Fatalf("Favourite(%s, %s) = %v", accountID, statusID, err)
	}
}

// list returns all of accountID's notifications of the given types, newest first.
func (e *notificationEnv) list(t *testing.T, accountID string, types ...notification.Type) []notification.Notification {
	t.Helper()
	ns, err := e.svc.List(t.Context(), accountID, appnotification.Query{Types: types, Limit: appnotification.MaxPageLimit})
	if err != nil {
		t.Fatalf("List(%s, %v) = %v", accountID, types, err)
	}
	return ns
}

// summary is a notification without its generated ID and time, for comparing.
type summary struct {
	Type     notification.Type
	Source   string
	StatusID string
}

func (e *notificationEnv) summaries(t *testing.T, accountID string) []summary {
	t.Helper()
	out := []summary{}
	for _, n := range e.list(t, accountID) {
		out = append(out, summary{Type: n.Type, Source: n.SourceAccountID, StatusID: n.StatusID})
	}
	return out
}

func checkSummaries(t *testing.T, e *notificationEnv, accountID string, want ...summary) {
	t.Helper()
	if want == nil {
		want = []summary{}
	}
	if got := e.summaries(t, accountID); !reflect.DeepEqual(got, want) {
		t.Errorf("notifications of %s = %+v, want %+v", accountID, got, want)
	}
}

// project runs only the notification projectors on events, in one transaction.
func (e *notificationEnv) project(t *testing.T, events ...uow.Event) {
	t.Helper()
	err := e.nh.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		for _, ev := range events {
			if err := e.only.Project(t.Context(), tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("project %v = %v", eventTypes(events), err)
	}
}

// favourited returns a favourite event of accountID on a local status of author.
func (e *notificationEnv) favourited(accountID, author, statusID string) appcontent.Favourited {
	return appcontent.Favourited{
		Favourite: content.Favourite{ID: e.ids.NewID(), AccountID: accountID, StatusID: statusID, URI: "https://" + contractHost + "/ap/activities/" + e.ids.NewID(), CreatedAt: e.clock.Now()},
		Status:    appcontent.StatusRef{ID: statusID, AuthorAccountID: author, IsLocal: true, Visibility: content.VisibilityPublic},
	}
}

func notificationFollows(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.lock(t, carol)

	e.follow(t, alice, bob)
	checkSummaries(t, e, bob, summary{Type: notification.TypeFollow, Source: alice})
	e.follow(t, alice, carol)
	checkSummaries(t, e, carol, summary{Type: notification.TypeFollowRequest, Source: alice})
	checkSummaries(t, e, alice)

	if _, err := e.graph.AcceptFollowRequest(ctx, carol, alice); err != nil {
		t.Fatalf("AcceptFollowRequest = %v", err)
	}
	checkSummaries(t, e, alice, summary{Type: notification.TypeFollowAccepted, Source: carol})
	checkSummaries(t, e, carol, summary{Type: notification.TypeFollowRequest, Source: alice})

	// Following again after unfollowing is a new follow and notifies again.
	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Fatalf("Unfollow = %v", err)
	}
	e.follow(t, alice, bob)
	checkSummaries(t, e, bob, summary{Type: notification.TypeFollow, Source: alice}, summary{Type: notification.TypeFollow, Source: alice})
	ns := e.list(t, bob)
	if ns[0].CreatedAt.Before(ns[1].CreatedAt) || ns[0].ID <= ns[1].ID || ns[0].RecipientAccountID != bob {
		t.Errorf("notifications of bob = %+v, want newest first", ns)
	}
}

func notificationMentions(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	p := e.post(t, alice, "hi @bob @carol and @alice")
	mention := func(source string) summary {
		return summary{Type: notification.TypeMention, Source: source, StatusID: p.Status.ID}
	}
	checkSummaries(t, e, bob, mention(alice))
	checkSummaries(t, e, carol, mention(alice))
	checkSummaries(t, e, alice) // never of one's own actions

	// An edit notifies only the accounts it newly mentions.
	e.clock.Advance(time.Second)
	_, err := e.content.Edit(ctx, appcontent.EditInput{
		AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "hi @bob and @dave"},
	})
	if err != nil {
		t.Fatalf("Edit = %v", err)
	}
	checkSummaries(t, e, bob, mention(alice))
	checkSummaries(t, e, dave, mention(alice))
	checkSummaries(t, e, carol, mention(alice))
}

func notificationInteractions(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	p := e.post(t, alice, "hello")
	id := p.Status.ID

	e.favourite(t, bob, id)
	e.favourite(t, alice, id) // one's own
	if err := e.interactions.Unfavourite(ctx, bob, id); err != nil {
		t.Fatalf("Unfavourite = %v", err)
	}
	e.favourite(t, bob, id) // undoing and repeating does not notify again
	for range 2 {
		e.clock.Advance(time.Second)
		if _, err := e.interactions.Boost(ctx, bob, id); err != nil {
			t.Fatalf("Boost = %v", err)
		}
		if err := e.interactions.Unboost(ctx, bob, id); err != nil {
			t.Fatalf("Unboost = %v", err)
		}
	}
	if _, err := e.interactions.Boost(ctx, alice, id); err != nil {
		t.Fatalf("Boost(own) = %v", err)
	}
	checkSummaries(t, e, alice,
		summary{Type: notification.TypeBoost, Source: bob, StatusID: id},
		summary{Type: notification.TypeFavourite, Source: bob, StatusID: id},
	)
	checkSummaries(t, e, bob)
}

func notificationLocalOnly(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	remote := account.Account{
		ID: e.ids.NewID(), URI: "https://remote.example/users/rem", Domain: "remote.example",
		ActorType: account.ActorPerson, CreatedAt: e.clock.Now(),
	}
	err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.nh.Accounts.InsertAccount(ctx, tx, remote) })
	if err != nil {
		t.Fatalf("InsertAccount(remote) = %v", err)
	}
	missing := e.ids.NewID()
	follow := graph.Follow{ID: e.ids.NewID(), FollowerAccountID: alice, FolloweeAccountID: remote.ID, RequestedAt: e.clock.Now()}
	remoteStatus := e.favourited(alice, remote.ID, e.ids.NewID())
	remoteStatus.Status.IsLocal = false
	e.project(t,
		appgraph.Followed{Follow: follow, Acceptance: graph.Acceptance{FollowID: follow.ID, AcceptedAt: e.clock.Now()}},
		appcontent.StatusCreated{
			Status:   content.Status{ID: e.ids.NewID(), AuthorAccountID: alice, IsLocal: true, Visibility: content.VisibilityPublic},
			Revision: content.Revision{MentionedAccountIDs: []string{remote.ID, missing}, RevisedAt: e.clock.Now()},
		},
		remoteStatus,
	)
	for _, id := range []string{remote.ID, missing} {
		checkSummaries(t, e, id)
	}
	checkSummaries(t, e, alice)
}

func notificationProjectsOnce(t *testing.T, e *notificationEnv) {
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	p := e.post(t, alice, "hi @bob")
	follow := graph.Follow{ID: e.ids.NewID(), FollowerAccountID: alice, FolloweeAccountID: bob, RequestedAt: e.clock.Now()}
	events := []uow.Event{
		appcontent.StatusCreated(p),
		appgraph.FollowRequested{Follow: follow},
		e.favourited(bob, alice, p.Status.ID),
	}
	e.project(t, events...)
	e.project(t, events...)
	e.project(t, e.favourited(bob, alice, p.Status.ID)) // another favourite of the same pair
	checkSummaries(t, e, bob,
		summary{Type: notification.TypeFollowRequest, Source: alice},
		summary{Type: notification.TypeMention, Source: alice, StatusID: p.Status.ID},
	)
	checkSummaries(t, e, alice, summary{Type: notification.TypeFavourite, Source: bob, StatusID: p.Status.ID})
}

func notificationConcurrent(t *testing.T, e *notificationEnv) {
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	p := e.post(t, alice, "hello")
	ev := e.favourited(bob, alice, p.Status.ID)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			errs[i] = e.nh.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return e.only.Project(t.Context(), tx, ev) })
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("project %d = %v", i, err)
		}
	}
	checkSummaries(t, e, alice, summary{Type: notification.TypeFavourite, Source: bob, StatusID: p.Status.ID})
}

func notificationHides(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave, erin := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave"), e.user(t, "erin")

	// A mute with hide_notifications hides them; one without does not.
	if _, err := e.graph.Mute(ctx, bob, alice, appgraph.MuteInput{HideNotifications: true}); err != nil {
		t.Fatalf("Mute = %v", err)
	}
	if _, err := e.graph.Mute(ctx, carol, alice, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute = %v", err)
	}
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	p := e.post(t, alice, "hi @bob @carol")
	checkSummaries(t, e, bob)
	checkSummaries(t, e, carol,
		summary{Type: notification.TypeMention, Source: alice, StatusID: p.Status.ID},
		summary{Type: notification.TypeFollow, Source: alice},
	)

	// An expired mute hides nothing.
	if _, err := e.graph.Mute(ctx, dave, alice, appgraph.MuteInput{HideNotifications: true, Duration: time.Hour}); err != nil {
		t.Fatalf("Mute = %v", err)
	}
	e.follow(t, alice, dave)
	checkSummaries(t, e, dave)
	e.clock.Advance(2 * time.Hour)
	q := e.post(t, alice, "hi @dave")
	checkSummaries(t, e, dave, summary{Type: notification.TypeMention, Source: alice, StatusID: q.Status.ID})

	// A block in either direction hides them.
	mine := e.post(t, erin, "mine")
	if _, err := e.graph.Block(ctx, erin, alice); err != nil {
		t.Fatalf("Block = %v", err)
	}
	if _, err := e.graph.Block(ctx, dave, erin); err != nil {
		t.Fatalf("Block = %v", err)
	}
	e.project(t, e.favourited(alice, erin, mine.Status.ID), e.favourited(dave, erin, mine.Status.ID))
	checkSummaries(t, e, erin)
}

func notificationLists(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	var followers []string
	for _, name := range []string{"u1", "u2", "u3"} {
		id := e.user(t, name)
		followers = append(followers, id)
		e.follow(t, id, alice)
	}
	p := e.post(t, alice, "hello")
	e.favourite(t, followers[0], p.Status.ID)
	e.favourite(t, followers[1], p.Status.ID)

	all := e.list(t, alice)
	if len(all) != 5 {
		t.Fatalf("notifications = %d, want 5", len(all))
	}
	follows := e.list(t, alice, notification.TypeFollow)
	if len(follows) != 3 || follows[0].SourceAccountID != followers[2] || follows[2].SourceAccountID != followers[0] {
		t.Errorf("follow notifications = %+v, want the three follows newest first", follows)
	}
	if got := e.list(t, alice, notification.TypeFollow, notification.TypeFavourite); len(got) != 5 {
		t.Errorf("follow and favourite notifications = %d, want 5", len(got))
	}
	if got := e.list(t, alice, notification.TypeMention); len(got) != 0 {
		t.Errorf("mention notifications = %+v, want none", got)
	}
	if _, err := e.svc.List(ctx, alice, appnotification.Query{Types: []notification.Type{"poke"}}); !errors.Is(err, notification.ErrInvalidType) {
		t.Errorf("List(poke) = %v, want ErrInvalidType", err)
	}

	var paged []notification.Notification
	before := ""
	for {
		page, err := e.svc.List(ctx, alice, appnotification.Query{Before: before, Limit: 2})
		if err != nil {
			t.Fatalf("List = %v", err)
		}
		paged = append(paged, page...)
		if len(page) < 2 {
			break
		}
		before = page[len(page)-1].ID
	}
	if !reflect.DeepEqual(paged, all) {
		t.Errorf("pages = %+v, want %+v", paged, all)
	}
	// After pages forward, oldest first.
	var forward []notification.Notification
	after := "00000000-0000-0000-0000-000000000000"
	for {
		page, err := e.svc.List(ctx, alice, appnotification.Query{After: after, Limit: 2})
		if err != nil {
			t.Fatalf("List(after) = %v", err)
		}
		forward = append(forward, page...)
		if len(page) < 2 {
			break
		}
		after = page[len(page)-1].ID
	}
	oldestFirst := slices.Clone(all)
	slices.Reverse(oldestFirst)
	if !reflect.DeepEqual(forward, oldestFirst) {
		t.Errorf("forward pages = %+v, want %+v", forward, oldestFirst)
	}
	if page, err := e.svc.List(ctx, alice, appnotification.Query{Types: []notification.Type{notification.TypeFollow}, After: follows[2].ID}); err != nil ||
		len(page) != 2 || page[0].ID != follows[1].ID || page[1].ID != follows[0].ID {
		t.Errorf("List(follow, after the oldest) = %+v, %v; want the two newer follows, oldest first", page, err)
	}
	page, err := e.svc.List(ctx, alice, appnotification.Query{Types: []notification.Type{notification.TypeFollow}, Before: follows[0].ID, Limit: 1})
	if err != nil || len(page) != 1 || page[0].ID != follows[1].ID {
		t.Errorf("List(follow, before the newest, 1) = %+v, %v; want the second follow", page, err)
	}
}

func notificationReads(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	e.follow(t, bob, alice)
	e.follow(t, carol, alice)
	e.follow(t, dave, alice)
	e.follow(t, alice, bob)
	ns := e.list(t, alice) // dave, carol, bob
	other := e.list(t, bob)[0]

	checkUnread := func(want int64) {
		t.Helper()
		if n, err := e.svc.UnreadCount(ctx, alice); err != nil || n != want {
			t.Errorf("UnreadCount = %d, %v; want %d", n, err, want)
		}
	}
	if m, err := e.svc.ReadMarker(ctx, alice); err != nil || m != (notification.ReadMarker{AccountID: alice}) {
		t.Errorf("ReadMarker = %+v, %v; want none", m, err)
	}
	checkUnread(3)

	m, err := e.svc.MarkRead(ctx, alice, ns[1].ID)
	if err != nil || m != (notification.ReadMarker{AccountID: alice, LastReadID: ns[1].ID}) {
		t.Errorf("MarkRead = %+v, %v", m, err)
	}
	checkUnread(1)
	if m, err := e.svc.MarkRead(ctx, alice, ns[2].ID); err != nil || m.LastReadID != ns[1].ID {
		t.Errorf("MarkRead(older) = %+v, %v; want the marker to stay", m, err)
	}
	if _, err := e.svc.MarkRead(ctx, alice, other.ID); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("MarkRead(bob's) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.MarkRead(ctx, alice, e.ids.NewID()); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("MarkRead(missing) = %v, want ErrNotFound", err)
	}
	checkUnread(1)

	e.checkListed(t, alice, ns[0].ID, true)
	e.checkListed(t, alice, other.ID, false) // bob's
	e.checkListed(t, alice, e.ids.NewID(), false)

	// Dismissing leaves the list and the unread count, once.
	for range 2 {
		if err := e.svc.Dismiss(ctx, alice, ns[0].ID); err != nil {
			t.Fatalf("Dismiss = %v", err)
		}
	}
	checkUnread(0)
	e.checkListed(t, alice, ns[0].ID, false)
	if err := e.svc.Dismiss(ctx, alice, other.ID); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("Dismiss(bob's) = %v, want ErrNotFound", err)
	}
	if got := e.list(t, alice); !reflect.DeepEqual(got, ns[1:]) {
		t.Errorf("notifications = %+v, want %+v", got, ns[1:])
	}
	if n, err := e.svc.UnreadCount(ctx, bob); err != nil || n != 1 {
		t.Errorf("UnreadCount(bob) = %d, %v; want 1", n, err)
	}
}

func (e *notificationEnv) checkListed(t *testing.T, accountID, id string, want bool) {
	t.Helper()
	if ok, err := e.svc.Listed(t.Context(), accountID, id); err != nil || ok != want {
		t.Errorf("Listed(%s, %s) = %v, %v; want %v", accountID, id, ok, err, want)
	}
}

func (e *notificationEnv) checkUnread(t *testing.T, accountID string, want int64) {
	t.Helper()
	if n, err := e.svc.UnreadCount(t.Context(), accountID); err != nil || n != want {
		t.Errorf("UnreadCount(%s) = %d, %v; want %d", accountID, n, err, want)
	}
}

func notificationHidesLater(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	e.follow(t, alice, bob)
	p := e.post(t, alice, "hi @bob")
	fromAlice := []summary{
		{Type: notification.TypeMention, Source: alice, StatusID: p.Status.ID},
		{Type: notification.TypeFollow, Source: alice},
	}
	// A notification from the instance has no source and stays through every block and mute.
	notice := notification.Notification{
		ID: e.ids.NewID(), RecipientAccountID: bob, Type: notification.TypeModerationNotice,
		ModerationActionID: e.ids.NewID(), DedupeKey: "notice:" + e.ids.NewID(), CreatedAt: e.clock.Now(),
	}
	err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.nh.Notifications.InsertNotification(ctx, tx, notice)
		return err
	})
	if err != nil {
		t.Fatalf("InsertNotification(notice) = %v", err)
	}
	noticeOnly := summary{Type: notification.TypeModerationNotice}
	all := append([]summary{noticeOnly}, fromAlice...)
	checkSummaries(t, e, bob, all...)
	e.checkUnread(t, bob, 3)

	mute := func(in appgraph.MuteInput) {
		t.Helper()
		if _, err := e.graph.Mute(ctx, bob, alice, in); err != nil {
			t.Fatalf("Mute(%+v) = %v", in, err)
		}
	}
	// A later mute without hide_notifications hides nothing; with it, it hides what came before.
	mute(appgraph.MuteInput{})
	checkSummaries(t, e, bob, all...)
	listed := e.list(t, bob)
	mute(appgraph.MuteInput{HideNotifications: true})
	checkSummaries(t, e, bob, noticeOnly)
	e.checkUnread(t, bob, 1)
	// Listed agrees with the list.
	for _, n := range listed {
		e.checkListed(t, bob, n.ID, n.Type == notification.TypeModerationNotice)
	}
	// Replacing it with a mute without hide_notifications, or unmuting, shows them again.
	mute(appgraph.MuteInput{})
	checkSummaries(t, e, bob, all...)
	mute(appgraph.MuteInput{HideNotifications: true})
	if _, err := e.graph.Unmute(ctx, bob, alice); err != nil {
		t.Fatalf("Unmute = %v", err)
	}
	checkSummaries(t, e, bob, all...)
	e.checkUnread(t, bob, 3)
	// A mute with a duration hides them until it expires.
	mute(appgraph.MuteInput{HideNotifications: true, Duration: time.Hour})
	checkSummaries(t, e, bob, noticeOnly)
	e.clock.Advance(2 * time.Hour)
	checkSummaries(t, e, bob, all...)

	// A later block in either direction hides them, until it is lifted.
	e.follow(t, carol, dave)
	e.follow(t, dave, carol)
	fromDave := summary{Type: notification.TypeFollow, Source: dave}
	fromCarol := summary{Type: notification.TypeFollow, Source: carol}
	if _, err := e.graph.Block(ctx, carol, dave); err != nil {
		t.Fatalf("Block = %v", err)
	}
	checkSummaries(t, e, carol)
	checkSummaries(t, e, dave)
	e.checkUnread(t, carol, 0)
	e.checkUnread(t, dave, 0)
	// With blocks both ways, lifting one leaves the other in force.
	if _, err := e.graph.Block(ctx, dave, carol); err != nil {
		t.Fatalf("Block = %v", err)
	}
	if _, err := e.graph.Unblock(ctx, carol, dave); err != nil {
		t.Fatalf("Unblock = %v", err)
	}
	checkSummaries(t, e, carol)
	checkSummaries(t, e, dave)
	if _, err := e.graph.Unblock(ctx, dave, carol); err != nil {
		t.Fatalf("Unblock = %v", err)
	}
	checkSummaries(t, e, carol, fromDave)
	checkSummaries(t, e, dave, fromCarol)
	e.checkUnread(t, carol, 1)
}

func notificationHidesMutedDomains(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	remote := func(domain string) string {
		t.Helper()
		a := account.Account{
			ID: e.ids.NewID(), URI: "https://" + domain + "/users/" + e.ids.NewID(), Domain: domain,
			ActorType: account.ActorPerson, CreatedAt: e.clock.Now(),
		}
		if err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.nh.Accounts.InsertAccount(ctx, tx, a) }); err != nil {
			t.Fatalf("InsertAccount(%s) = %v", domain, err)
		}
		return a.ID
	}
	sub, other := remote("social.remote.example"), remote("other.example")
	statusID := e.post(t, alice, "hello").Status.ID
	e.project(t, e.favourited(sub, alice, statusID), e.favourited(other, alice, statusID))
	fromSub := summary{Type: notification.TypeFavourite, Source: sub, StatusID: statusID}
	fromOther := summary{Type: notification.TypeFavourite, Source: other, StatusID: statusID}
	checkSummaries(t, e, alice, fromOther, fromSub)
	e.checkUnread(t, alice, 2)

	// A mute of a parent domain hides what came before from its subdomains, and keeps new
	// notifications from them from being created at all.
	if _, err := e.graph.MuteDomain(ctx, alice, "remote.example"); err != nil {
		t.Fatalf("MuteDomain = %v", err)
	}
	checkSummaries(t, e, alice, fromOther)
	e.checkUnread(t, alice, 1)
	e.clock.Advance(time.Second)
	again := e.favourited(sub, alice, e.post(t, alice, "again").Status.ID)
	e.project(t, again)
	checkSummaries(t, e, alice, fromOther)

	// Rebuilding the exclusions keeps the domain exclusion.
	if err := appnotification.RebuildExclusions(ctx, e.nh.UnitOfWork, e.nh.Notifications, appnotification.GraphSource(e.nh.Graph)); err != nil {
		t.Fatalf("RebuildExclusions = %v", err)
	}
	checkSummaries(t, e, alice, fromOther)

	if err := e.graph.UnmuteDomain(ctx, alice, "remote.example"); err != nil {
		t.Fatalf("UnmuteDomain = %v", err)
	}
	checkSummaries(t, e, alice, fromOther, fromSub)
	e.checkUnread(t, alice, 2)
}

func notificationDeletedStatuses(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, carol, alice)
	mention := e.post(t, alice, "hi @bob")
	kept := e.post(t, alice, "kept")
	gone := e.post(t, alice, "gone")
	for _, id := range []string{kept.Status.ID, gone.Status.ID} {
		e.favourite(t, bob, id)
		e.clock.Advance(time.Second)
		if _, err := e.interactions.Boost(ctx, bob, id); err != nil {
			t.Fatalf("Boost = %v", err)
		}
	}
	checkSummaries(t, e, bob, summary{Type: notification.TypeMention, Source: alice, StatusID: mention.Status.ID})
	e.checkUnread(t, alice, 5)

	for _, id := range []string{mention.Status.ID, gone.Status.ID} {
		e.clock.Advance(time.Second)
		if _, err := e.content.Delete(ctx, alice, id); err != nil {
			t.Fatalf("Delete(%s) = %v", id, err)
		}
	}
	// Projecting a deletion again changes nothing.
	e.project(t, appcontent.StatusDeleted{
		Status:   gone.Status,
		Deletion: content.Deletion{StatusID: gone.Status.ID, URI: gone.Status.URI, AuthorAccountID: alice, Origin: content.DeletedByAuthor, DeletedAt: e.clock.Now()},
	})
	checkSummaries(t, e, bob)
	e.checkUnread(t, bob, 0)
	checkSummaries(t, e, alice,
		summary{Type: notification.TypeBoost, Source: bob, StatusID: kept.Status.ID},
		summary{Type: notification.TypeFavourite, Source: bob, StatusID: kept.Status.ID},
		summary{Type: notification.TypeFollow, Source: carol},
	)
	e.checkUnread(t, alice, 3)
}

func notificationReportResolved(t *testing.T, e *notificationEnv) {
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	resolved := func(reporter, reportID string, outcome moderation.ReportOutcome) appmoderation.ReportResolved {
		e.clock.Advance(time.Second)
		return appmoderation.ReportResolved{
			Report: moderation.Report{ID: reportID, ReporterAccountID: reporter, TargetAccountID: bob},
			Resolution: moderation.ReportResolution{
				ID: e.ids.NewID(), ReportID: reportID, Outcome: outcome, ResolvedAt: e.clock.Now(),
			},
		}
	}
	report, missing := e.ids.NewID(), e.ids.NewID()
	first := resolved(alice, report, moderation.OutcomeActioned)
	// Resolving it again after a reopening, and a report of a reporter that is not a local
	// account, notify nobody.
	e.project(t, first, resolved(alice, report, moderation.OutcomeDismissed), resolved(missing, e.ids.NewID(), moderation.OutcomeActioned))
	ns := e.list(t, alice)
	if len(ns) != 1 {
		t.Fatalf("notifications of alice = %+v, want one report_resolved", ns)
	}
	if n := ns[0]; n.Type != notification.TypeReportResolved || n.ReportID != report || n.SourceAccountID != "" || n.StatusID != "" ||
		n.ModerationActionID != "" || !n.CreatedAt.Equal(first.Resolution.ResolvedAt) || n.DedupeKey != notification.ReportResolvedKey(report) {
		t.Errorf("notification = %+v, want report_resolved of %s at %v", n, report, first.Resolution.ResolvedAt)
	}
	checkSummaries(t, e, bob)
	checkSummaries(t, e, missing)
}

// contractReach is an appnotification.ReachPolicy that restricts the accounts it is told to.
type contractReach struct {
	mu         sync.Mutex
	restricted map[string]bool
}

func (r *contractReach) set(accountID string, restricted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.restricted == nil {
		r.restricted = map[string]bool{}
	}
	r.restricted[accountID] = restricted
}

// UnsolicitedReachRestricted implements appnotification.ReachPolicy.
func (r *contractReach) UnsolicitedReachRestricted(_ context.Context, accountID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.restricted[accountID], nil
}

// notificationUnsolicitedReach checks that a source with unsolicited_reach notifies only the
// accounts that follow it with an accepted follow, of every type, and everyone again once the
// restriction ends.
func notificationUnsolicitedReach(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	e.lock(t, alice)
	e.lock(t, dave)
	e.follow(t, bob, alice)
	if _, err := e.graph.AcceptFollowRequest(ctx, alice, bob); err != nil {
		t.Fatalf("AcceptFollowRequest = %v", err)
	}
	e.follow(t, carol, alice) // pending: carol does not follow alice yet
	bobPost, carolPost := e.post(t, bob, "from bob"), e.post(t, carol, "from carol")
	e.reach.set(alice, true)

	p := e.post(t, alice, "hi @bob @carol")
	e.favourite(t, alice, bobPost.Status.ID)
	e.favourite(t, alice, carolPost.Status.ID)
	e.follow(t, alice, carol)
	e.follow(t, alice, dave)
	checkSummaries(t, e, bob,
		summary{Type: notification.TypeFavourite, Source: alice, StatusID: bobPost.Status.ID},
		summary{Type: notification.TypeMention, Source: alice, StatusID: p.Status.ID},
		summary{Type: notification.TypeFollowAccepted, Source: alice},
	)
	checkSummaries(t, e, carol)
	checkSummaries(t, e, dave)
	// The restriction is of the source: alice is still notified of the others.
	if got := e.list(t, alice, notification.TypeFollowRequest); len(got) != 2 {
		t.Errorf("follow requests of alice = %+v, want bob's and carol's", got)
	}

	e.reach.set(alice, false)
	q := e.post(t, alice, "again @carol")
	checkSummaries(t, e, carol, summary{Type: notification.TypeMention, Source: alice, StatusID: q.Status.ID})
}
