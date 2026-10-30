// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// Accounts is the part of the account module's public API that the projectors read: whether a
// recipient is a local account, and the domain of a source. Only local accounts receive
// notifications here; remote ones are notified by their own servers. *appaccount.Service
// implements it.
type Accounts interface {
	IsLocal(ctx context.Context, tx uow.Tx, accountID string) (bool, error)
	// DomainOf returns the domain of a remote account, "" for a local or unknown one.
	DomainOf(ctx context.Context, tx uow.Tx, accountID string) (string, error)
}

// Relationships is the part of the graph module's public API that the projectors read: whether
// a block between the recipient and the source, or the recipient's mute of the source with
// hide_notifications, keeps the notification from being created, and whether the recipient
// follows the source. *appgraph.Service implements it.
type Relationships interface {
	HidesNotifications(ctx context.Context, tx uow.Tx, recipientID, sourceID string) (bool, error)
	Follows(ctx context.Context, tx uow.Tx, followerID, followeeID string) (bool, error)
}

// ReachPolicy is the part of the moderation module's public API that the projectors read:
// whether the source's restrictions, its own and its domain's, include unsolicited_reach, so
// that it notifies only the accounts that follow it (data-model §5.2).
// *appmoderation.AccountPolicy implements it.
type ReachPolicy interface {
	UnsolicitedReachRestricted(ctx context.Context, accountID string) (bool, error)
}

// ProjectorDeps are the ports the notification projectors use.
type ProjectorDeps struct {
	Repository    Repository
	Accounts      Accounts
	Relationships Relationships
	// Reach keeps the notifications of a source with unsolicited_reach from the recipients
	// that do not follow it; nil restricts no source.
	Reach ReachPolicy
	IDs   kernel.IDGenerator
	// Push enqueues the Web Push messages of the new notifications; nil pushes none.
	Push *Pusher
	// Email enqueues the emails of the new notifications whose recipients get them by email;
	// nil emails none.
	Email *EmailScheduler
	// PostJobs enqueues JobNotifyPostSubscribers for the new posts of the accounts with
	// subscribers (ADR-0123 §2); nil notifies no subscriber.
	PostJobs queue.Outbox
}

// RegisterProjectors registers the synchronous projectors that create notifications from the
// graph, content, and moderation events, in the transaction of the command that emits them
// (data-model §2.1, §4.7):
//
//   - graph.followed: follow, to the followee.
//   - graph.follow_requested: follow_request, to the followee.
//   - graph.follow_accepted: follow_accepted, to the follower.
//   - content.status_created and content.status_revised: mention, to each account the revision
//     mentions; an edit notifies only the accounts it newly mentions.
//   - content.boosted: boost, to the status's author.
//   - content.favourited: favourite, to the status's author.
//   - content.reacted: reaction, to the status's author, with the reaction's emoji. Its dedupe
//     key is per emoji (notification.ReactionKey), so a reaction with another emoji notifies
//     again (ADR-0052 §5).
//   - content.poll_closed: poll_closed, to the author of the poll without a source, and to each
//     account that voted in it from the author, once per poll (notification.PollClosedKey).
//   - moderation.report_resolved: report_resolved, to the reporter when the reporter is a local
//     account. It comes from the instance and has no source account. Its dedupe key is the
//     report's (notification.ReportResolvedKey), so only the first resolution notifies: a
//     report reopened and resolved again does not notify again.
//   - moderation.account_action_issued: moderation_notice, to the account the action is on when
//     the action notifies it. It comes from the instance and has no source account.
//   - moderation.relationships_severed: relationships_severed, to the local account that lost
//     follows to a suspension, once per severance (notification.RelationshipsSeveredKey). It
//     comes from the instance and has no source account.
//   - graph.follow_migrated: account_moved, to the local follower of an account that moved, from
//     that account, with what became of the follow (ADR-0089), once per move
//     (notification.AccountMovedKey). The follower may no longer follow the moved account, so a
//     limit on it does not keep the notification back.
//   - moderation.appeal_decided: appeal_decided, to the account that appealed. It comes from
//     the instance and has no source account; its dedupe key is the appeal's
//     (notification.AppealDecidedKey).
//   - content.custom_emoji_request_recorded of an approval or a rejection:
//     custom_emoji_request_decided, to the requester's local account (ADR-0070 §9). It comes from
//     the instance and has no source account; its dedupe key is the request's
//     (notification.CustomEmojiRequestDecidedKey). A withdrawal and a lapse notify nobody.
//
// A notification goes only to a local recipient other than its source, and not when either
// blocks the other, the recipient mutes the source with hide_notifications or mutes its domain
// (FR-304), the source has unsolicited_reach (a limit on it or its domain) and the recipient
// does not follow it, or the recipient turned the type's group off in the app (FR-210). Each notification
// has a dedupe key unique per recipient (notification.FollowKey, MentionKey, InteractionKey,
// ModerationNoticeKey),
// so projecting an event again, or a repeated boost or favourite, creates nothing more. Each new
// notification enqueues its Web Push messages through d.Push, and its email through d.Email when
// the recipient gets its group by email, in the same transaction.
//
// The exclusion projectors keep notification_exclusions, which the list and the unread count
// apply at read time, so that a later block or mute also hides the notifications that came
// before it:
//
//   - graph.blocked: block from the blocker to the blocked account, and blocked_by back.
//   - graph.unblocked: deletes both.
//   - graph.muted: mute from the account to the target with the mute's expiry when the mute
//     hides notifications; deletes it otherwise.
//   - graph.unmuted: deletes the mute.
//   - graph.domain_muted: a domain exclusion of the domain, which hides the notifications whose
//     source_domain is the domain or one of its subdomains.
//   - graph.domain_unmuted: deletes it.
//
// The projector of content.status_deleted records the status in notification_deleted_statuses
// when a notification is about it, which the list and the unread count also apply, so that the
// mention, boost, and favourite notifications about a deleted status no longer appear. A
// deleted status gets no notifications later, so one without them needs no row. Notifications
// are events and stay until the erasure job deletes them.
//
// The subscription projectors (registerSubscriptionProjectors) keep the subscriptions to new
// posts in step with the follows and blocks, and enqueue the status notifications of a new post.
//
// Register them after the graph, content, and moderation projectors.
func RegisterProjectors(r *uow.Projectors, d ProjectorDeps) {
	p := &projector{d: d}
	registerSubscriptionProjectors(r, d)
	r.Register(appgraph.EventFollowed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.Followed)
		f := ev.Follow
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: f.FolloweeAccountID, SourceAccountID: f.FollowerAccountID, Type: notification.TypeFollow,
			DedupeKey: notification.FollowKey(notification.TypeFollow, f.ID), CreatedAt: ev.Acceptance.AcceptedAt,
		})
	})
	r.Register(appgraph.EventFollowRequested, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		f := e.(appgraph.FollowRequested).Follow
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: f.FolloweeAccountID, SourceAccountID: f.FollowerAccountID, Type: notification.TypeFollowRequest,
			DedupeKey: notification.FollowKey(notification.TypeFollowRequest, f.ID), CreatedAt: f.RequestedAt,
		})
	})
	r.Register(appgraph.EventFollowAccepted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.FollowAccepted)
		f := ev.Follow
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: f.FollowerAccountID, SourceAccountID: f.FolloweeAccountID, Type: notification.TypeFollowAccepted,
			DedupeKey: notification.FollowKey(notification.TypeFollowAccepted, f.ID), CreatedAt: ev.Acceptance.AcceptedAt,
		})
	})
	registerExclusionProjectors(r, d.Repository)
	r.Register(appcontent.EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.StatusCreated)
		return p.mentions(ctx, tx, ev.Status.ID, ev.Status.AuthorAccountID, ev.Revision.MentionedAccountIDs, ev.Revision.RevisedAt)
	})
	r.Register(appcontent.EventStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.StatusRevised)
		return p.mentions(ctx, tx, ev.Status.ID, ev.Status.AuthorAccountID, ev.Revision.MentionedAccountIDs, ev.Revision.RevisedAt)
	})
	registerDeletedStatusProjector(r, d.Repository)
	r.Register(appcontent.EventBoosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.Boosted)
		return p.interaction(ctx, tx, notification.TypeBoost, ev.Status, ev.Boost.BoosterAccountID, ev.Boost.BoostedAt)
	})
	r.Register(appcontent.EventFavourited, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.Favourited)
		return p.interaction(ctx, tx, notification.TypeFavourite, ev.Status, ev.Favourite.AccountID, ev.Favourite.CreatedAt)
	})
	r.Register(appcontent.EventReacted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.Reacted)
		re, st := ev.Reaction, ev.Status
		if !st.IsLocal {
			return nil // a remote author is notified by its own server
		}
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: st.AuthorAccountID, SourceAccountID: re.AccountID, Type: notification.TypeReaction, StatusID: st.ID,
			ReactionEmoji: re.Emoji.Unicode, ReactionCustomEmojiID: re.Emoji.CustomEmojiID,
			DedupeKey: notification.ReactionKey(st.ID, re.AccountID, re.Emoji.Key()), CreatedAt: re.CreatedAt,
		})
	})
	// A quote of a local status notifies its author once the quote is shown: a legacy one when
	// it is recorded, an FEP-044f one when it is approved (ADR-0069 §5).
	r.Register(appcontent.EventQuoteRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.QuoteRecorded)
		if ev.Quote.Form != appcontent.QuoteFormLegacy {
			return nil
		}
		return p.quote(ctx, tx, ev.Status, ev.Quoted, ev.RecordedAt)
	})
	r.Register(appcontent.EventQuoteChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.QuoteChanged)
		if ev.Event.Kind != appcontent.QuoteEventAccepted {
			return nil
		}
		return p.quote(ctx, tx, ev.Status, ev.Quoted, ev.Event.OccurredAt)
	})
	r.Register(appcontent.EventPollClosed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.PollClosed)
		return p.pollClosed(ctx, tx, ev)
	})
	r.Register(appmoderation.EventReportResolved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appmoderation.ReportResolved)
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: ev.Report.ReporterAccountID, Type: notification.TypeReportResolved, ReportID: ev.Report.ID,
			DedupeKey: notification.ReportResolvedKey(ev.Report.ID), CreatedAt: ev.Resolution.ResolvedAt,
		})
	})
	r.Register(appmoderation.EventAccountActionIssued, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		a := e.(appmoderation.AccountActionIssued).Action
		if !a.NotifyTarget {
			return nil
		}
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: a.TargetAccountID, Type: notification.TypeModerationNotice, ModerationActionID: a.ID,
			DedupeKey: notification.ModerationNoticeKey(a.ID), CreatedAt: a.IssuedAt,
		})
	})
	r.Register(appgraph.EventFollowMigrated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.FollowMigrated)
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: ev.AccountID, SourceAccountID: ev.SourceAccountID, Type: notification.TypeAccountMoved,
			AccountMoveID: ev.MoveID, AccountMoveOutcome: notification.MoveOutcome(ev.Outcome),
			DedupeKey: notification.AccountMovedKey(ev.MoveID), CreatedAt: ev.At,
		})
	})
	r.Register(appmoderation.EventRelationshipsSevered, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		sv := e.(appmoderation.RelationshipsSevered).Severance
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: sv.AccountID, Type: notification.TypeRelationshipsSevered, SeveranceID: sv.ID,
			DedupeKey: notification.RelationshipsSeveredKey(sv.ID), CreatedAt: sv.OccurredAt,
		})
	})
	r.Register(appmoderation.EventAppealDecided, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appmoderation.AppealDecided)
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: ev.Appeal.AccountID, Type: notification.TypeAppealDecided, AppealID: ev.Appeal.ID,
			DedupeKey: notification.AppealDecidedKey(ev.Appeal.ID), CreatedAt: ev.Decision.DecidedAt,
		})
	})
	r.Register(appcontent.EventCustomEmojiRequestRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.CustomEmojiRequestRecorded)
		if ev.Outcome == nil || ev.RequesterAccountID == "" ||
			ev.Action != appcontent.CustomEmojiRequestApproved && ev.Action != appcontent.CustomEmojiRequestRejected {
			return nil
		}
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: ev.RequesterAccountID, Type: notification.TypeCustomEmojiRequestDecided, CustomEmojiRequestID: ev.Request.ID,
			DedupeKey: notification.CustomEmojiRequestDecidedKey(ev.Request.ID), CreatedAt: ev.At,
		})
	})
}

// registerExclusionProjectors registers the projectors of notification_exclusions, for
// RegisterProjectors and RebuildExclusions.
func registerExclusionProjectors(r *uow.Projectors, repo Repository) {
	r.Register(appgraph.EventBlocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(appgraph.Blocked).Block
		for _, x := range blockExclusions(b.BlockerAccountID, b.BlockedAccountID) {
			if err := repo.UpsertExclusion(ctx, tx, x); err != nil {
				return err
			}
		}
		return nil
	})
	r.Register(appgraph.EventUnblocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(appgraph.Unblocked).Block
		for _, x := range blockExclusions(b.BlockerAccountID, b.BlockedAccountID) {
			if err := repo.DeleteExclusion(ctx, tx, x.ViewerAccountID, x.TargetAccountID, x.Reason); err != nil {
				return err
			}
		}
		return nil
	})
	r.Register(appgraph.EventMuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		m := e.(appgraph.Muted).Mute
		if !m.HideNotifications {
			return repo.DeleteExclusion(ctx, tx, m.AccountID, m.TargetAccountID, notification.ExcludedMute)
		}
		return repo.UpsertExclusion(ctx, tx, notification.Exclusion{
			ViewerAccountID: m.AccountID, TargetAccountID: m.TargetAccountID, Reason: notification.ExcludedMute, ExpiresAt: m.ExpiresAt,
		})
	})
	r.Register(appgraph.EventUnmuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.Unmuted)
		return repo.DeleteExclusion(ctx, tx, ev.AccountID, ev.TargetAccountID, notification.ExcludedMute)
	})
	r.Register(appgraph.EventDomainMuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		m := e.(appgraph.DomainMuted).Mute
		return repo.InsertDomainExclusion(ctx, tx, m.AccountID, m.Domain)
	})
	r.Register(appgraph.EventDomainUnmuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.DomainUnmuted)
		return repo.DeleteDomainExclusion(ctx, tx, ev.AccountID, ev.Domain)
	})
}

// registerDeletedStatusProjector registers the projector of notification_deleted_statuses, for
// RegisterProjectors.
func registerDeletedStatusProjector(r *uow.Projectors, repo Repository) {
	r.Register(appcontent.EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return recordDeletedStatus(ctx, tx, repo, e.(appcontent.StatusDeleted).Deletion.StatusID)
	})
}

// recordDeletedStatus records a deleted status in notification_deleted_statuses when a
// notification is about it. Most deletions by the retention purge (ADR-0034) are of remote
// statuses that nobody was notified of, and they need no row.
func recordDeletedStatus(ctx context.Context, tx uow.Tx, repo Repository, statusID string) error {
	notified, err := repo.StatusHasNotifications(ctx, tx, statusID)
	if err != nil || !notified {
		return err
	}
	return repo.InsertDeletedStatus(ctx, tx, statusID)
}

// blockExclusions are the exclusions of blocker's block of blocked: each hides the other's
// notifications.
func blockExclusions(blocker, blocked string) []notification.Exclusion {
	return []notification.Exclusion{
		{ViewerAccountID: blocker, TargetAccountID: blocked, Reason: notification.ExcludedBlock},
		{ViewerAccountID: blocked, TargetAccountID: blocker, Reason: notification.ExcludedBlockedBy},
	}
}

type projector struct {
	d ProjectorDeps
}

func (p *projector) mentions(ctx context.Context, tx uow.Tx, statusID, authorID string, mentioned []string, at time.Time) error {
	for _, id := range mentioned {
		err := p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: id, SourceAccountID: authorID, Type: notification.TypeMention, StatusID: statusID,
			DedupeKey: notification.MentionKey(statusID), CreatedAt: at,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// pollClosed notifies the author of a poll that has ended, without a source, and each voter,
// from the author; notify keeps the local recipients. The author who voted in the own poll is
// notified once (the dedupe key is the poll's).
func (p *projector) pollClosed(ctx context.Context, tx uow.Tx, ev appcontent.PollClosed) error {
	st, at := ev.Status, *ev.Poll.ClosesAt
	key := notification.PollClosedKey(ev.Poll.ID)
	err := p.notify(ctx, tx, notification.Notification{
		RecipientAccountID: st.AuthorAccountID, Type: notification.TypePollClosed, StatusID: st.ID, DedupeKey: key, CreatedAt: at,
	})
	if err != nil {
		return err
	}
	for _, id := range ev.VoterAccountIDs {
		if id == st.AuthorAccountID {
			continue
		}
		err := p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: id, SourceAccountID: st.AuthorAccountID, Type: notification.TypePollClosed, StatusID: st.ID,
			DedupeKey: key, CreatedAt: at,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// quote notifies the local author of the status quoted of the quote quoting.
func (p *projector) quote(ctx context.Context, tx uow.Tx, quoting appcontent.StatusRef, quoted *appcontent.StatusRef, at time.Time) error {
	if quoted == nil || !quoted.IsLocal {
		return nil // a remote author is notified by its own server
	}
	return p.notify(ctx, tx, notification.Notification{
		RecipientAccountID: quoted.AuthorAccountID, SourceAccountID: quoting.AuthorAccountID, Type: notification.TypeQuote,
		StatusID: quoting.ID, DedupeKey: notification.InteractionKey(notification.TypeQuote, quoting.ID, quoting.AuthorAccountID),
		CreatedAt: at,
	})
}

func (p *projector) interaction(ctx context.Context, tx uow.Tx, t notification.Type, st appcontent.StatusRef, sourceID string, at time.Time) error {
	if !st.IsLocal {
		return nil // a remote author is notified by its own server
	}
	return p.notify(ctx, tx, notification.Notification{
		RecipientAccountID: st.AuthorAccountID, SourceAccountID: sourceID, Type: t, StatusID: st.ID,
		DedupeKey: notification.InteractionKey(t, st.ID, sourceID), CreatedAt: at,
	})
}

// notify inserts n, with its source's domain, unless its recipient is its source or not local,
// the relationship between them hides it, the source may reach only its followers and the
// recipient is none, the recipient mutes the source's domain, or the recipient turned its group
// off in the app. A notification with the same dedupe key leaves n out.
func (p *projector) notify(ctx context.Context, tx uow.Tx, n notification.Notification) error {
	if n.RecipientAccountID == n.SourceAccountID {
		return nil
	}
	local, err := p.d.Accounts.IsLocal(ctx, tx, n.RecipientAccountID)
	if err != nil || !local {
		return err
	}
	if n.SourceAccountID != "" {
		hidden, err := p.d.Relationships.HidesNotifications(ctx, tx, n.RecipientAccountID, n.SourceAccountID)
		if err != nil || hidden {
			return err
		}
		if n.Type != notification.TypeAccountMoved {
			if reached, err := p.reaches(ctx, tx, n.RecipientAccountID, n.SourceAccountID); err != nil || !reached {
				return err
			}
		}
		if n.SourceDomain, err = p.d.Accounts.DomainOf(ctx, tx, n.SourceAccountID); err != nil {
			return err
		}
		if n.SourceDomain != "" {
			muted, err := p.d.Repository.DomainExcluded(ctx, tx, n.RecipientAccountID, n.SourceDomain)
			if err != nil || muted {
				return err
			}
		}
	}
	choices, err := choicesTx(ctx, p.d.Repository, tx, n.RecipientAccountID)
	if err != nil {
		return err
	}
	if !choices.TypeEnabled(n.Type, notification.ChannelInApp) {
		return nil // turned off in the app: not created, so delivered nowhere (ADR-0109 §3)
	}
	n.ID = p.d.IDs.NewID()
	if err := n.Validate(); err != nil {
		return err
	}
	created, err := p.d.Repository.InsertNotification(ctx, tx, n)
	if err != nil || !created {
		return err
	}
	if err := p.d.Push.enqueue(ctx, tx, n); err != nil {
		return err
	}
	return p.d.Email.enqueue(ctx, tx, n, choices)
}

// reaches reports whether sourceID may notify recipientID: always, unless the source has
// unsolicited_reach, and then only when the recipient follows it.
func (p *projector) reaches(ctx context.Context, tx uow.Tx, recipientID, sourceID string) (bool, error) {
	if p.d.Reach == nil {
		return true, nil
	}
	restricted, err := p.d.Reach.UnsolicitedReachRestricted(ctx, sourceID)
	if err != nil || !restricted {
		return err == nil, err
	}
	return p.d.Relationships.Follows(ctx, tx, recipientID, sourceID)
}
