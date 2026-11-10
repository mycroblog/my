// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"slices"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
)

// RegisterCommitHooks signals to pub, once the command commits, the accounts that the events
// of RegisterProjectors may have notified (FR-207). The hooks name every candidate recipient of
// an event: the projectors may still have created nothing for it (a remote or blocking
// recipient, a repeated dedupe key), so a stream reads the notifications after the last one it
// sent and sends only those.
func RegisterCommitHooks(r *uow.Projectors, pub stream.Publisher) {
	signal := func(accountIDs ...string) {
		for _, id := range accountIDs {
			pub.Publish(stream.Event{Kind: stream.KindNotifications, AccountID: id})
		}
	}
	r.OnCommit(appgraph.EventFollowed, func(_ context.Context, e uow.Event) {
		signal(e.(appgraph.Followed).Follow.FolloweeAccountID)
	})
	r.OnCommit(appgraph.EventFollowRequested, func(_ context.Context, e uow.Event) {
		signal(e.(appgraph.FollowRequested).Follow.FolloweeAccountID)
	})
	r.OnCommit(appgraph.EventFollowsCreated, func(_ context.Context, e uow.Event) {
		ev := e.(appgraph.FollowsCreated)
		var ids []string
		for _, f := range ev.Followed {
			ids = append(ids, f.Follow.FolloweeAccountID)
		}
		for _, f := range ev.Requested {
			ids = append(ids, f.Follow.FolloweeAccountID)
		}
		slices.Sort(ids)
		signal(slices.Compact(ids)...)
	})
	r.OnCommit(appgraph.EventFollowAccepted, func(_ context.Context, e uow.Event) {
		signal(e.(appgraph.FollowAccepted).Follow.FollowerAccountID)
	})
	r.OnCommit(appcontent.EventStatusCreated, func(_ context.Context, e uow.Event) {
		if ev := e.(appcontent.StatusCreated); !ev.HeldReply {
			signal(ev.Revision.MentionedAccountIDs...)
		}
	})
	r.OnCommit(appcontent.EventStatusReleased, func(_ context.Context, e uow.Event) {
		signal(e.(appcontent.StatusReleased).Revision.MentionedAccountIDs...)
	})
	r.OnCommit(appcontent.EventInteractionRequested, func(_ context.Context, e uow.Event) {
		if ev := e.(appcontent.InteractionRequested); ev.State.State == appcontent.InteractionRequestPending && ev.Target != nil && ev.Target.IsLocal {
			signal(ev.Target.AuthorAccountID)
		}
	})
	r.OnCommit(appcontent.EventInteractionAnswered, func(_ context.Context, e uow.Event) {
		if ev := e.(appcontent.InteractionAnswered); ev.Request.Direction == appcontent.DirectionOutbound || ev.Request.Direction == appcontent.DirectionLocal && ev.Deferred {
			signal(ev.Request.InteractingAccountID)
		}
	})
	r.OnCommit(appcontent.EventStatusRevised, func(_ context.Context, e uow.Event) {
		if ev := e.(appcontent.StatusRevised); !ev.HeldReply {
			signal(ev.Revision.MentionedAccountIDs...)
		}
	})
	r.OnCommit(appcontent.EventBoosted, func(_ context.Context, e uow.Event) {
		signal(e.(appcontent.Boosted).Status.AuthorAccountID)
	})
	r.OnCommit(appcontent.EventFavourited, func(_ context.Context, e uow.Event) {
		signal(e.(appcontent.Favourited).Status.AuthorAccountID)
	})
	r.OnCommit(appcontent.EventReacted, func(_ context.Context, e uow.Event) {
		signal(e.(appcontent.Reacted).Status.AuthorAccountID)
	})
}
