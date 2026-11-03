// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// FollowRefusalsDeps are the ports and settings of FollowRefusals.
type FollowRefusalsDeps struct {
	UnitOfWork uow.UnitOfWork
	// Outbound records the Reject and its document, and Outbox enqueues its fan-out in the same
	// transaction.
	Outbound Repository
	Outbox   queue.Outbox
	// Inboxes reads the inbox of the remote follower.
	Inboxes RemoteInboxes
	Clock   kernel.Clock
	IDs     kernel.IDGenerator
	// Host is the serving domain of the activity URIs (ADR-0018).
	Host string
}

// FollowRefusals answers the remote Follows that the federation mode refuses (follow_in in
// inbound, ADR-0076) with a Reject, without recording a follow, so that none of them stays
// pending.
type FollowRefusals struct {
	d   FollowRefusalsDeps
	rec recorder
}

// NewFollowRefusals returns the refusals on d.
func NewFollowRefusals(d FollowRefusalsDeps) *FollowRefusals {
	return &FollowRefusals{d: d, rec: recorder{d: RecorderDeps{Repository: d.Outbound, Outbox: d.Outbox, IDs: d.IDs, Host: d.Host}}}
}

// Refuse sends the Reject of the Follow in of the local actor followee to the follower's inbox:
// an outbound activity of the kind refusal, whose document is stored when it is recorded. A
// follower without an inbox is dropped.
func (r *FollowRefusals) Refuse(ctx context.Context, in *InboxActivity, followee appaccount.Actor) error {
	inbox, err := r.d.Inboxes.RemoteInbox(ctx, in.Actor.Account.ID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return fmt.Errorf("%w: %s has no inbox", ErrDropActivity, in.Activity.Actor)
	}
	if err != nil {
		return err
	}
	target := federation.IRI(inbox.InboxURL)
	if target == "" {
		target = federation.IRI(inbox.SharedInboxURL)
	}
	follow := in.Activity.ID
	if follow == "" {
		follow = in.Received.ActivityURI
	}
	now := kernel.Normalize(r.d.Clock.Now())
	return r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		first, err := r.rec.firstAnswer(ctx, tx, in.Received.ActivityURI, now)
		if err != nil || !first {
			return err
		}
		act, err := r.rec.insert(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityReject, ActorAccountID: followee.Account.ID,
			ObjectKind: federation.OutboundRefusal, ObjectID: in.Actor.Account.ID, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		doc := federation.FollowAnswerDocument(act.ActivityURI, federation.ActivityReject, federation.IRI(followee.Account.URI), in.Activity.Actor, follow, now)
		b, err := doc.Marshal()
		if err != nil {
			return err
		}
		if _, err := r.d.Outbound.InsertOutboundDocument(ctx, tx, act.ID, b, act.CreatedAt); err != nil {
			return err
		}
		return r.rec.enqueue(ctx, tx, act, fanOutPayload{Inbox: string(target)})
	})
}
