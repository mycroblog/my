// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/notification"
)

// subscriptions returns the subscriptions' use cases on the harness of e.
func (e *notificationEnv) subscriptions() *appnotification.Subscriptions {
	return appnotification.NewSubscriptions(appnotification.SubscriptionDeps{
		UnitOfWork: e.nh.UnitOfWork, Repository: e.nh.Notifications, Accounts: e.accounts, Relationships: e.graph, Clock: e.clock,
	})
}

// remoteAccount inserts an active remote account.
func (e *notificationEnv) remoteAccount(t *testing.T, name string) string {
	t.Helper()
	a := account.Account{
		ID: e.ids.NewID(), URI: "https://remote.example/users/" + name, Domain: "remote.example",
		ActorType: account.ActorPerson, CreatedAt: e.clock.Now(),
	}
	err := e.nh.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return e.nh.Accounts.InsertAccount(t.Context(), tx, a) })
	if err != nil {
		t.Fatalf("InsertAccount(%s) = %v", name, err)
	}
	return a.ID
}

func checkSubscribed(t *testing.T, s *appnotification.Subscriptions, accountID, targetID string, want bool) {
	t.Helper()
	got, err := s.Subscribed(t.Context(), accountID, targetID)
	if err != nil || got != want {
		t.Errorf("Subscribed(%s, %s) = %v, %v; want %v", accountID, targetID, got, err, want)
	}
}

// notificationSubscriptions checks the subscriptions to new posts (ADR-0123 §1): a local account
// without a follow, a remote one only while it is actively followed, none to oneself or across a
// block; a subscription asked for with a pending follow of a remote account that the acceptance
// makes; the end of the follow of a remote account, which ends the subscription, and of a local
// one, which does not; a block, which ends both directions; and the list, newest first.
func notificationSubscriptions(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	s := e.subscriptions()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	remote := e.remoteAccount(t, "rem")

	if err := s.Subscribe(ctx, alice, bob); err != nil {
		t.Fatalf("Subscribe(local, without a follow) = %v", err)
	}
	if err := s.Subscribe(ctx, alice, bob); err != nil {
		t.Errorf("Subscribe again = %v, want nothing to change", err)
	}
	checkSubscribed(t, s, alice, bob, true)
	if err := s.Subscribe(ctx, alice, alice); !errors.Is(err, notification.ErrSelfSubscription) {
		t.Errorf("Subscribe(self) = %v, want ErrSelfSubscription", err)
	}
	if err := s.Subscribe(ctx, alice, remote); !errors.Is(err, appnotification.ErrRemoteNotFollowed) {
		t.Errorf("Subscribe(remote, without a follow) = %v, want ErrRemoteNotFollowed", err)
	}

	// A follow of a remote account awaits its server: Subscribe still refuses, the notify of the
	// follow waits for the acceptance.
	e.follow(t, alice, remote)
	if err := s.Subscribe(ctx, alice, remote); !errors.Is(err, appnotification.ErrRemoteNotFollowed) {
		t.Errorf("Subscribe(remote, pending) = %v, want ErrRemoteNotFollowed", err)
	}
	if err := s.SubscribeWithFollow(ctx, alice, remote); err != nil {
		t.Fatalf("SubscribeWithFollow(remote, pending) = %v", err)
	}
	checkSubscribed(t, s, alice, remote, false)
	e.clock.Advance(time.Second)
	if err := e.graph.AcceptRemote(ctx, appgraph.RemoteResponse{FollowURI: "https://remote.example/unknown", FolloweeID: remote, FollowerID: alice}); err != nil {
		t.Fatalf("AcceptRemote = %v", err)
	}
	checkSubscribed(t, s, alice, remote, true)

	// Unfollowing the remote account ends the subscription; unfollowing a local one does not.
	if _, err := e.graph.Unfollow(ctx, alice, remote); err != nil {
		t.Fatal(err)
	}
	checkSubscribed(t, s, alice, remote, false)
	e.follow(t, alice, bob)
	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Fatal(err)
	}
	checkSubscribed(t, s, alice, bob, true)

	// A pending request ends with the follow.
	e.follow(t, carol, remote)
	if err := s.SubscribeWithFollow(ctx, carol, remote); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Unfollow(ctx, carol, remote); err != nil {
		t.Fatal(err)
	}
	e.follow(t, carol, remote)
	if err := e.graph.AcceptRemote(ctx, appgraph.RemoteResponse{FollowURI: "https://remote.example/unknown", FolloweeID: remote, FollowerID: carol}); err != nil {
		t.Fatal(err)
	}
	checkSubscribed(t, s, carol, remote, false)

	// The list, newest first.
	e.clock.Advance(time.Second)
	if err := s.Subscribe(ctx, alice, carol); err != nil {
		t.Fatal(err)
	}
	subs, err := s.List(ctx, alice, appnotification.SubscriptionPage{Limit: 1})
	if err != nil || len(subs) != 1 || subs[0].TargetAccountID != carol {
		t.Fatalf("List(first) = %+v, %v; want carol", subs, err)
	}
	subs, err = s.List(ctx, alice, appnotification.SubscriptionPage{Before: appnotification.SubscriptionCursorOf(subs[0]), Limit: 5})
	if err != nil || len(subs) != 1 || subs[0].TargetAccountID != bob {
		t.Errorf("List(after carol) = %+v, %v; want bob", subs, err)
	}

	// A block ends the subscriptions both ways and refuses new ones.
	if err := s.Subscribe(ctx, bob, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Block(ctx, bob, alice); err != nil {
		t.Fatal(err)
	}
	checkSubscribed(t, s, alice, bob, false)
	checkSubscribed(t, s, bob, alice, false)
	if err := s.Subscribe(ctx, alice, bob); !errors.Is(err, appnotification.ErrSubscriptionBlocked) {
		t.Errorf("Subscribe(blocked) = %v, want ErrSubscriptionBlocked", err)
	}
	if err := s.Unsubscribe(ctx, alice, carol); err != nil {
		t.Fatal(err)
	}
	checkSubscribed(t, s, alice, carol, false)
}

// contractPosts implements notification.Posts with every status visible to everyone but those
// in hidden, and the authors that the content service finds.
type contractPosts struct {
	content *appcontent.Service
	hidden  map[string]bool
}

func (p contractPosts) StatusVisible(_ context.Context, accountID, _ string) (bool, error) {
	return !p.hidden[accountID], nil
}

func (p contractPosts) AuthorOf(ctx context.Context, uri string) (string, error) {
	st, err := p.content.FindStatusByURI(ctx, uri)
	if errors.Is(err, appcontent.ErrNotFound) {
		return "", nil
	}
	return st.AuthorAccountID, err
}

// notificationNewPosts checks the status notifications of new posts (ADR-0123 §2): a new post
// and a reply to the author's own post notify each subscriber once, from the job that the
// transaction of the post enqueues; a reply to someone else's post, an edit, a boost, and a
// fetched remote post notify nobody; a subscriber whom the post mentions gets the mention only,
// one who may not see the post nothing; and a status notification enqueues no email, also for a
// recipient who gets every other notification by email.
func notificationNewPosts(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	s := e.subscriptions()
	alice, bob, carol, dave, erin := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave"), e.user(t, "erin")
	reg := &queue.JobHandlerRegistry{}
	posts := contractPosts{content: e.content, hidden: map[string]bool{erin: true}}
	appnotification.RegisterPostNotificationJobs(reg, appnotification.PostNotificationDeps{UnitOfWork: e.nh.UnitOfWork, Posts: posts, Projectors: e.deps})
	handle, _, ok := reg.Lookup(appnotification.JobNotifyPostSubscribers)
	if !ok {
		t.Fatal("no handler of the post notifications")
	}
	// run runs the jobs that the transactions of the posts enqueued, and returns how many.
	run := func() int {
		t.Helper()
		taken := e.postJobs.take()
		for _, j := range taken {
			if j.Type != appnotification.JobNotifyPostSubscribers || j.IdempotencyKey == "" {
				t.Fatalf("job = %+v, want %s with an idempotency key", j, appnotification.JobNotifyPostSubscribers)
			}
			if res := handle(ctx, queue.Job{ID: e.ids.NewID(), Type: j.Type, Payload: j.Payload}); !res.IsDone() {
				t.Fatalf("%s = %+v, want done", j.Type, res)
			}
		}
		return len(taken)
	}
	for _, id := range []string{bob, carol, dave, erin} {
		if err := s.Subscribe(ctx, id, alice); err != nil {
			t.Fatal(err)
		}
	}
	// Bob gets every other notification by email.
	var all []notification.Setting
	for _, g := range notification.Groups {
		if g.Section == notification.SectionOrdinary && !g.Email.Absent {
			all = append(all, notification.Setting{Group: g.Group, Channel: notification.ChannelEmail, Enabled: true})
		}
	}
	if _, err := e.svc.UpdateSettings(ctx, bob, all); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateSettings(ctx, bob, []notification.Setting{{Group: notification.GroupStatus, Channel: notification.ChannelEmail, Enabled: true}}); !errors.Is(err, notification.ErrInvalidSetting) {
		t.Errorf("turning on the email of status = %v, want ErrInvalidSetting", err)
	}
	e.outbox.take()

	status := func(id string) summary { return summary{Type: notification.TypeStatus, Source: alice, StatusID: id} }
	if n := run(); n != 0 {
		t.Fatalf("jobs before any post = %d, want none", n)
	}
	p := e.post(t, alice, "hello @dave")
	if n := run(); n != 1 {
		t.Fatalf("jobs of a new post = %d, want 1", n)
	}
	checkSummaries(t, e, bob, status(p.Status.ID))
	checkSummaries(t, e, carol, status(p.Status.ID))
	checkSummaries(t, e, dave, summary{Type: notification.TypeMention, Source: alice, StatusID: p.Status.ID})
	checkSummaries(t, e, erin) // may not see the post
	for _, j := range e.outbox.take() {
		if j.Type == appnotification.JobNotificationEmail {
			t.Errorf("a status notification enqueued an email: %s", j.Payload)
		}
	}
	// Projecting the post again and running its job again notifies nobody twice.
	e.project(t, appcontent.StatusCreated(p))
	run()
	checkSummaries(t, e, carol, status(p.Status.ID))

	// A reply to her own post notifies; a reply to someone else's does not.
	e.clock.Advance(time.Second)
	own, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "and more"}, Visibility: content.VisibilityPublic, InReplyToID: p.Status.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	run()
	checkSummaries(t, e, carol, status(own.Status.ID), status(p.Status.ID))
	other := e.post(t, bob, "a post of bob")
	run()
	e.clock.Advance(time.Second)
	_, err = e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "a reply"}, Visibility: content.VisibilityPublic, InReplyToID: other.Status.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	run()
	checkSummaries(t, e, carol, status(own.Status.ID), status(p.Status.ID))

	// An edit and a boost notify nobody.
	e.clock.Advance(time.Second)
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "hello again"}}); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	if _, err := e.interactions.Boost(ctx, alice, other.Status.ID); err != nil {
		t.Fatal(err)
	}
	if n := run(); n != 0 {
		t.Errorf("jobs of an edit and a boost = %d, want none", n)
	}
	checkSummaries(t, e, carol, status(own.Status.ID), status(p.Status.ID))

	// A remote post that a delivered Create brought enqueues the job; a fetched one does not.
	remote := e.remoteAccount(t, "rem")
	err = e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.nh.Notifications.InsertSubscription(ctx, tx, notification.Subscription{AccountID: carol, TargetAccountID: remote, CreatedAt: e.clock.Now()})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	remotePost := func(activity string) appcontent.StatusCreated {
		return appcontent.StatusCreated{
			Status: content.Status{ID: e.ids.NewID(), URI: "https://remote.example/notes/" + e.ids.NewID(), AuthorAccountID: remote, Visibility: content.VisibilityPublic},
			Revision: content.Revision{
				RevisedAt: e.clock.Now(), Remote: &content.RemoteRevisionSource{ActivityURI: activity},
			},
		}
	}
	e.project(t, remotePost(""))
	if n := run(); n != 0 {
		t.Errorf("jobs of a fetched post = %d, want none", n)
	}
	delivered := remotePost("https://remote.example/notes/1/activity")
	e.project(t, delivered)
	if n := run(); n != 1 {
		t.Errorf("jobs of a delivered post = %d, want 1", n)
	}
	if got := e.list(t, carol, notification.TypeStatus); len(got) != 3 || got[0].StatusID != delivered.Status.ID || got[0].SourceAccountID != remote {
		t.Errorf("status notifications of carol = %+v, want the delivered post first", got)
	}
}
