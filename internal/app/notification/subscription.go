// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The subscriptions to the new posts of an account (ADR-0123): a local account subscribes to a
// local account whether it follows it or not, and to a remote account only while it actively
// follows it. Each new post of the target that the subscriber may see becomes a status
// notification, in the app and by Web Push, never by email.

// SubscriptionRepository is the persistence of the subscriptions (post_subscriptions and
// post_subscription_requests, data-model §4.7), part of Repository.
type SubscriptionRepository interface {
	// InsertSubscription inserts a subscription and reports false, writing nothing, when the
	// account already subscribes to the target.
	InsertSubscription(ctx context.Context, tx uow.Tx, s notification.Subscription) (bool, error)
	// DeleteSubscription deletes the subscription, if any, and reports whether there was one.
	DeleteSubscription(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error)
	// CountSubscriptions counts the account's subscriptions.
	CountSubscriptions(ctx context.Context, tx uow.Tx, accountID string) (int64, error)
	// Subscribed reports whether the account subscribes to the target.
	Subscribed(ctx context.Context, accountID, targetAccountID string) (bool, error)
	// HasSubscribers reports whether any account subscribes to the target.
	HasSubscribers(ctx context.Context, tx uow.Tx, targetAccountID string) (bool, error)
	// ListSubscriptions returns at most limit of the account's subscriptions, newest first,
	// after the cursor before (the zero cursor starts at the newest).
	ListSubscriptions(ctx context.Context, accountID string, before SubscriptionCursor, limit int) ([]notification.Subscription, error)
	// ListSubscribers returns at most limit of the accounts that subscribe to the target, in
	// order of their IDs, after the ID after ("" starts at the first).
	ListSubscribers(ctx context.Context, targetAccountID, after string, limit int) ([]string, error)
	// InsertSubscriptionRequest records a subscription asked for with a pending follow; recording
	// it again changes nothing.
	InsertSubscriptionRequest(ctx context.Context, tx uow.Tx, r notification.SubscriptionRequest) error
	// DeleteSubscriptionRequest deletes the request, if any, and reports whether there was one.
	DeleteSubscriptionRequest(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error)
	// SubscriptionRequested reports whether the account asked for a subscription to the target
	// that awaits the acceptance of its follow.
	SubscriptionRequested(ctx context.Context, accountID, targetAccountID string) (bool, error)
}

// SubscriptionCursor is the position of a subscription in the newest-first list.
type SubscriptionCursor struct {
	CreatedAt       time.Time
	TargetAccountID string
}

// SubscriptionCursorOf returns the cursor that pages on after s.
func SubscriptionCursorOf(s notification.Subscription) SubscriptionCursor {
	return SubscriptionCursor{CreatedAt: s.CreatedAt, TargetAccountID: s.TargetAccountID}
}

// SubscriptionRelationships is the part of the graph module's public API that the subscriptions
// read inside their transactions. *appgraph.Service implements it.
type SubscriptionRelationships interface {
	Follows(ctx context.Context, tx uow.Tx, followerID, followeeID string) (bool, error)
	FollowRequested(ctx context.Context, tx uow.Tx, followerID, followeeID string) (bool, error)
	BlockedEitherWay(ctx context.Context, tx uow.Tx, a, b string) (bool, error)
}

// ErrNotSubscribable reports a subscription that ADR-0123 §1 does not allow: to a remote account
// that the subscriber does not actively follow (ErrRemoteNotFollowed), or between accounts one of
// which blocks the other (ErrSubscriptionBlocked).
var ErrNotSubscribable = errors.New("notification: the account cannot be subscribed to")

// The reasons of ErrNotSubscribable; each wraps it.
var (
	ErrRemoteNotFollowed   = fmt.Errorf("%w: a remote account is subscribed to only while it is followed", ErrNotSubscribable)
	ErrSubscriptionBlocked = fmt.Errorf("%w: one of the accounts blocks the other", ErrNotSubscribable)
)

// Page sizes of the list of subscriptions.
const (
	DefaultSubscriptionPageLimit = 40
	MaxSubscriptionPageLimit     = 80
)

// SubscriptionPage selects a page of an account's subscriptions, newest first.
type SubscriptionPage struct {
	Before SubscriptionCursor
	// Limit is the page size; zero or less means DefaultSubscriptionPageLimit, and it is capped
	// at MaxSubscriptionPageLimit.
	Limit int
}

func (p SubscriptionPage) limit() int {
	switch {
	case p.Limit <= 0:
		return DefaultSubscriptionPageLimit
	case p.Limit > MaxSubscriptionPageLimit:
		return MaxSubscriptionPageLimit
	}
	return p.Limit
}

// SubscriptionDeps are the ports of the subscriptions' use cases.
type SubscriptionDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    SubscriptionRepository
	Accounts      Accounts
	Relationships SubscriptionRelationships
	Clock         kernel.Clock
}

// Subscriptions runs the use cases of a local account's subscriptions to new posts: subscribing,
// unsubscribing, reading whether it subscribes, and listing them.
type Subscriptions struct {
	d SubscriptionDeps
}

// NewSubscriptions returns Subscriptions on d.
func NewSubscriptions(d SubscriptionDeps) *Subscriptions { return &Subscriptions{d: d} }

// Subscribe makes accountID subscribe to targetID. Subscribing again changes nothing. Subscribing
// to itself fails with notification.ErrSelfSubscription, to a remote account that accountID does
// not actively follow with ErrRemoteNotFollowed, when either blocks the other with
// ErrSubscriptionBlocked, and beyond notification.MaxSubscriptions with
// notification.ErrSubscriptionLimit.
func (s *Subscriptions) Subscribe(ctx context.Context, accountID, targetID string) error {
	return s.subscribe(ctx, accountID, targetID, false)
}

// SubscribeWithFollow is Subscribe for the notify of a follow (ADR-0123 §1): when accountID's
// follow of the remote targetID awaits its server's Accept, it records the request, and the
// acceptance makes the subscription.
func (s *Subscriptions) SubscribeWithFollow(ctx context.Context, accountID, targetID string) error {
	return s.subscribe(ctx, accountID, targetID, true)
}

func (s *Subscriptions) subscribe(ctx context.Context, accountID, targetID string, request bool) error {
	sub := notification.Subscription{AccountID: accountID, TargetAccountID: targetID, CreatedAt: s.d.Clock.Now()}
	if err := sub.Validate(); err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		blocked, err := s.d.Relationships.BlockedEitherWay(ctx, tx, accountID, targetID)
		if err != nil {
			return err
		}
		if blocked {
			return ErrSubscriptionBlocked
		}
		local, err := s.d.Accounts.IsLocal(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if !local {
			follows, err := s.d.Relationships.Follows(ctx, tx, accountID, targetID)
			if err != nil {
				return err
			}
			if !follows {
				if !request {
					return ErrRemoteNotFollowed
				}
				requested, err := s.d.Relationships.FollowRequested(ctx, tx, accountID, targetID)
				if err != nil {
					return err
				}
				if !requested {
					return ErrRemoteNotFollowed
				}
				return s.d.Repository.InsertSubscriptionRequest(ctx, tx, notification.SubscriptionRequest(sub))
			}
		}
		return insertSubscription(ctx, s.d.Repository, tx, sub)
	})
}

// insertSubscription inserts sub inside tx unless the subscriber has as many as it may have.
func insertSubscription(ctx context.Context, repo SubscriptionRepository, tx uow.Tx, sub notification.Subscription) error {
	inserted, err := repo.InsertSubscription(ctx, tx, sub)
	if err != nil || !inserted {
		return err
	}
	n, err := repo.CountSubscriptions(ctx, tx, sub.AccountID)
	if err != nil {
		return err
	}
	if n > notification.MaxSubscriptions {
		return fmt.Errorf("%d subscriptions of %s: %w", n, sub.AccountID, notification.ErrSubscriptionLimit)
	}
	return nil
}

// Unsubscribe ends accountID's subscription to targetID, and the request of one with a pending
// follow. Without either it changes nothing.
func (s *Subscriptions) Unsubscribe(ctx context.Context, accountID, targetID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return deleteSubscription(ctx, s.d.Repository, tx, accountID, targetID)
	})
}

func deleteSubscription(ctx context.Context, repo SubscriptionRepository, tx uow.Tx, accountID, targetID string) error {
	if _, err := repo.DeleteSubscription(ctx, tx, accountID, targetID); err != nil {
		return err
	}
	_, err := repo.DeleteSubscriptionRequest(ctx, tx, accountID, targetID)
	return err
}

// Subscribed reports whether accountID subscribes to targetID. A request that awaits the
// acceptance of a follow does not count.
func (s *Subscriptions) Subscribed(ctx context.Context, accountID, targetID string) (bool, error) {
	return s.d.Repository.Subscribed(ctx, accountID, targetID)
}

// List returns a page of accountID's subscriptions, newest first.
func (s *Subscriptions) List(ctx context.Context, accountID string, p SubscriptionPage) ([]notification.Subscription, error) {
	return s.d.Repository.ListSubscriptions(ctx, accountID, p.Before, p.limit())
}

// registerSubscriptionProjectors registers the projectors that keep the subscriptions in step
// with the graph module (ADR-0123 §1), and the one that starts the notifications of a new post
// (§2):
//
//   - graph.follow_accepted: a request of the follower turns into a subscription, unless the
//     follower has as many as it may have.
//   - graph.follow_rejected: deletes the request.
//   - graph.follow_terminated: deletes the request, and the subscription when the followee is a
//     remote account; a subscription to a local account stays.
//   - graph.blocked: deletes the subscriptions and requests of the two accounts to each other.
//   - content.status_created: a post made here or that a delivered Create (or an Update taken
//     as one) brought, by an author with subscribers, enqueues JobNotifyPostSubscribers in the
//     transaction that records it. A fetched post enqueues nothing.
func registerSubscriptionProjectors(r *uow.Projectors, d ProjectorDeps) {
	repo := d.Repository
	r.Register(appgraph.EventFollowAccepted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		f := e.(appgraph.FollowAccepted).Follow
		requested, err := repo.DeleteSubscriptionRequest(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID)
		if err != nil || !requested {
			return err
		}
		n, err := repo.CountSubscriptions(ctx, tx, f.FollowerAccountID)
		if err != nil || n >= notification.MaxSubscriptions {
			return err
		}
		_, err = repo.InsertSubscription(ctx, tx, notification.Subscription{
			AccountID: f.FollowerAccountID, TargetAccountID: f.FolloweeAccountID, CreatedAt: e.(appgraph.FollowAccepted).Acceptance.AcceptedAt,
		})
		return err
	})
	r.Register(appgraph.EventFollowRejected, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		f := e.(appgraph.FollowRejected).Follow
		_, err := repo.DeleteSubscriptionRequest(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID)
		return err
	})
	r.Register(appgraph.EventFollowTerminated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		f := e.(appgraph.FollowTerminated).Follow
		if _, err := repo.DeleteSubscriptionRequest(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID); err != nil {
			return err
		}
		local, err := d.Accounts.IsLocal(ctx, tx, f.FolloweeAccountID)
		if err != nil || local {
			return err
		}
		_, err = repo.DeleteSubscription(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID)
		return err
	})
	r.Register(appgraph.EventBlocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(appgraph.Blocked).Block
		if err := deleteSubscription(ctx, repo, tx, b.BlockerAccountID, b.BlockedAccountID); err != nil {
			return err
		}
		return deleteSubscription(ctx, repo, tx, b.BlockedAccountID, b.BlockerAccountID)
	})
	r.Register(appcontent.EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.StatusCreated)
		if d.PostJobs == nil {
			return nil
		}
		fetched := !ev.Status.IsLocal && (ev.Revision.Remote == nil || ev.Revision.Remote.ActivityURI == "")
		if fetched {
			return nil // an old post fetched for a thread, a search, or a quote is no new post
		}
		has, err := repo.HasSubscribers(ctx, tx, ev.Status.AuthorAccountID)
		if err != nil || !has {
			return err
		}
		payload, err := json.Marshal(postJob{
			StatusID: ev.Status.ID, AuthorAccountID: ev.Status.AuthorAccountID, InReplyToURI: ev.Status.InReplyToURI,
			MentionedAccountIDs: ev.Revision.MentionedAccountIDs, At: ev.Revision.RevisedAt,
		})
		if err != nil {
			return err
		}
		return d.PostJobs.EnqueueTx(ctx, tx, queue.NewJob{
			Type: JobNotifyPostSubscribers, Payload: payload, IdempotencyKey: JobNotifyPostSubscribers + ":" + ev.Status.ID,
		})
	})
}

// JobNotifyPostSubscribers notifies the subscribers of the author of a new post (ADR-0123 §2).
const JobNotifyPostSubscribers = "notification.notify_post_subscribers"

// postJob is the payload of JobNotifyPostSubscribers.
type postJob struct {
	StatusID        string `json:"status_id"`
	AuthorAccountID string `json:"author_account_id"`
	// InReplyToURI is the post it replies to, "" for none.
	InReplyToURI string `json:"in_reply_to_uri,omitempty"`
	// MentionedAccountIDs are the accounts the post mentions, which get a mention instead.
	MentionedAccountIDs []string  `json:"mentioned_account_ids,omitempty"`
	At                  time.Time `json:"at"`
}

// Posts is what the notifications of new posts read of the content module. The platform
// implements it on the content module's single visibility policy.
type Posts interface {
	// StatusVisible reports whether the local account may see the status now; false for a
	// missing or deleted status.
	StatusVisible(ctx context.Context, accountID, statusID string) (bool, error)
	// AuthorOf returns the author of the stored status at uri, "" when none is stored.
	AuthorOf(ctx context.Context, uri string) (string, error)
}

// PostNotificationDeps are the ports of JobNotifyPostSubscribers.
type PostNotificationDeps struct {
	UnitOfWork uow.UnitOfWork
	Posts      Posts
	// Projectors are the ports that create the notifications, as for the projectors.
	Projectors ProjectorDeps
}

// RegisterPostNotificationJobs registers the handler of JobNotifyPostSubscribers. The job takes
// the subscribers of the author notification.SubscriberBatch at a time, each batch in its own
// transaction, and notifies each one that may see the post with a status notification
// (notification.StatusKey), unless the post replies to someone else's post, or the subscriber
// gets a mention of it. The exclusions of the other notifications apply too (notify): blocks,
// mutes with hide_notifications, domain mutes, unsolicited_reach, and the in-app setting. The
// dedupe key makes a retried job notify nobody twice.
func RegisterPostNotificationJobs(reg *queue.JobHandlerRegistry, d PostNotificationDeps) {
	n := &postNotifier{d: d, p: &projector{d: d.Projectors}}
	reg.Register(JobNotifyPostSubscribers, func(ctx context.Context, j queue.Job) queue.Result {
		var job postJob
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return queue.Fail(fmt.Errorf("decode %s: %w", JobNotifyPostSubscribers, err))
		}
		if err := n.run(ctx, job); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
}

type postNotifier struct {
	d PostNotificationDeps
	p *projector
}

func (n *postNotifier) run(ctx context.Context, job postJob) error {
	if job.InReplyToURI != "" {
		parent, err := n.d.Posts.AuthorOf(ctx, job.InReplyToURI)
		if err != nil {
			return err
		}
		if parent != job.AuthorAccountID {
			return nil // a reply to someone else, or to a post this server does not have
		}
	}
	repo := n.d.Projectors.Repository
	for after := ""; ; {
		ids, err := repo.ListSubscribers(ctx, job.AuthorAccountID, after, notification.SubscriberBatch)
		if err != nil {
			return err
		}
		var to []string
		for _, id := range ids {
			if slices.Contains(job.MentionedAccountIDs, id) {
				continue
			}
			visible, err := n.d.Posts.StatusVisible(ctx, id, job.StatusID)
			if err != nil {
				return err
			}
			if visible {
				to = append(to, id)
			}
		}
		if len(to) > 0 {
			err := n.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				for _, id := range to {
					err := n.p.notify(ctx, tx, notification.Notification{
						RecipientAccountID: id, SourceAccountID: job.AuthorAccountID, Type: notification.TypeStatus,
						StatusID: job.StatusID, DedupeKey: notification.StatusKey(job.StatusID), CreatedAt: job.At,
					})
					if err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		if len(ids) < notification.SubscriberBatch {
			return nil
		}
		after = ids[len(ids)-1]
	}
}
