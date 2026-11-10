// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Job types of outbound delivery (federation.md §7).
const (
	// JobFanOut computes the inboxes of one outbound activity, one batch at a time, and
	// enqueues a JobDeliver per new delivery.
	JobFanOut = "federation.fanout"
	// JobDeliver signs and POSTs one delivery. Its payload is deliveryPayload.
	JobDeliver = "federation.deliver"
)

// fanOutPayload is the payload of JobFanOut. Phase and After are the cursor: the source of
// recipients the job reads and the position in it, both "" for the first batch.
type fanOutPayload struct {
	OutboundActivityID string `json:"outbound_activity_id"`
	// RevisionID is the revision a Create or Update presents (federation.md §7.1), "" for other
	// activities.
	RevisionID string `json:"revision_id,omitempty"`
	// FollowURI is the Follow that an Accept answers when it is not the follow's own: one that
	// the follower sent again (graph.follow_resent). "" for other activities.
	FollowURI string `json:"follow_uri,omitempty"`
	// SharedInboxes sends the Update of an actor to every known shared inbox after its
	// followers, as the Delete of an actor goes: the Update of a suspension and of its end
	// (federation.md §11.1). false for other activities.
	SharedInboxes bool `json:"shared_inboxes,omitempty"`
	// Inbox is the only recipient of an activity of the relay membership (ADR-0006), "" for
	// other activities.
	Inbox string `json:"inbox,omitempty"`
	// Followers sends an activity of the approval of an interaction to the actor's followers
	// after Inbox: the Accept of a reply without a request (ADR-0155 §6). false for others.
	Followers bool   `json:"followers,omitempty"`
	Phase     string `json:"phase,omitempty"`
	After     string `json:"after,omitempty"`
}

// job returns the JobFanOut of p, a batch of a. Its ordering key is that of a (orderingKey), so
// that the fan-outs of the activities that must reach their inboxes in order run in the order
// they were recorded.
func (p fanOutPayload) job(a federation.OutboundActivity) (queue.NewJob, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{
		Type: JobFanOut, Payload: b,
		IdempotencyKey: "fanout:" + p.OutboundActivityID + ":" + p.Phase + ":" + p.After,
		OrderingKey:    orderingKey(a),
	}, nil
}

// orderingKey returns the ordering key (ADR-0004) of the jobs that fan out and deliver a, ""
// when a needs no order.
//
// The Like and Undo Like of one local account about one remote status share a key, whether they
// come from a favourite or a reaction (the Like slot, ADR-0051 §6): switching the slot sends the
// Undo of what was in it, then the new Like, and a server that keeps one reaction per account
// and status (Misskey) refuses a Like that overtakes the Undo. Their fan-outs share the key, so
// that the deliveries of the later activity are enqueued after those of the earlier one, and
// each delivery adds its inbox to it (deliverJob), so that a delivery waits for the earlier one
// to the same inbox while that one retries.
func orderingKey(a federation.OutboundActivity) string {
	if a.ObjectKind == federation.OutboundFavourite {
		return "like:" + a.ActorAccountID + ":" + a.ObjectID
	}
	return ""
}

// deliveryPayload is the payload of JobDeliver.
type deliveryPayload struct {
	DeliveryID string `json:"delivery_id"`
}

// deliverJob returns the JobDeliver of d, a delivery of a. When a has an ordering key, the job's
// is that key with the inbox of d: the deliveries to one inbox keep their order, and those to
// other inboxes do not wait for them.
func deliverJob(a federation.OutboundActivity, d federation.Delivery) (queue.NewJob, error) {
	b, err := json.Marshal(deliveryPayload{DeliveryID: d.ID})
	if err != nil {
		return queue.NewJob{}, err
	}
	j := queue.NewJob{Type: JobDeliver, Payload: b, IdempotencyKey: "deliver:" + d.ID}
	if k := orderingKey(a); k != "" {
		j.OrderingKey = k + ":" + string(d.InboxURL)
	}
	return j, nil
}

// RecorderDeps are the ports and settings of the outbound projectors.
type RecorderDeps struct {
	Repository Repository
	// Outbox enqueues the fan-out in the transaction of the local command.
	Outbox queue.Outbox
	IDs    kernel.IDGenerator
	// Host is the serving domain of the activity URIs (ADR-0018).
	Host string
	// Accounts tells local accounts from remote ones in the transaction, for the activities of
	// follows; nil records none of them.
	Accounts LocalityReader
	// Modes reads the federation mode, for the activities that are recorded only when it
	// publishes (the Update of an actor's aliases); nil is federation.DefaultMode.
	Modes ModeSettings
	// Hidden tells the actors that a suspension or their owners' deactivation hides from the remote
	// servers; nil hides none.
	Hidden ActorHiding
}

// ActorHiding reads in the transaction why a local actor is hidden from the remote servers: an
// actor stays hidden until neither holds, so a change of one while the other holds sends nothing
// (ADR-0089 §17).
type ActorHiding interface {
	SuspendedTx(ctx context.Context, tx uow.Tx, accountID string) (bool, error)
	DeactivatedTx(ctx context.Context, tx uow.Tx, accountID string) (bool, error)
}

// RegisterOutboundProjectors subscribes the federation module to the domain events of other
// modules that a local actor's activity follows from (federation.md §7.1). Each projector writes,
// in the transaction of the local command, an outbound_activities row and the JobFanOut that
// delivers it, so that an activity is recorded exactly when the command commits.
//
//   - content.status_created / status_revised / status_deleted of a local status: Create,
//     Update, and Delete; a held local reply has neither Create nor Update until
//     content.status_released, which records its Create (ADR-0155 §7).
//   - content.boosted / boost_retracted of a boost that federates: Announce and Undo Announce.
//     A boost of a local status (Boost.Federates is false) is not recorded (ADR-0017).
//   - account.profile_revised of a local profile (ProfileRevised.Federates): Update of the
//     actor.
//   - timeline.featured_tags_changed of a local account: Update of the actor, whose featuredTags
//     collection the remote servers fetch again (ADR-0006 §17).
//   - account.aliases_changed of a local account: Update of the actor, recorded only when the
//     federation mode publishes (ADR-0089).
//   - account.lifecycle_event_recorded of a local account's deletion: Delete of the actor. Of a
//     deactivation or a reactivation (ADR-0089 §17): Update of the actor to the followers and every
//     known shared inbox, unless a suspension hides the actor anyway.
//   - account.moved of a local account's own move: Update of the actor, which carries movedTo,
//     and Move (ADR-0089); account.move_canceled: Update of the actor, without movedTo.
//   - graph.followed / follow_requested / follow_accepted / follow_rejected / follow_terminated
//     between a local and a remote account: Follow, Accept, Reject, and Undo Follow
//     (registerFollowProjectors).
//   - content.favourited / unfavourited of a remote status: Like and Undo Like
//     (registerFavouriteProjectors).
//   - content.reacted / unreacted of a remote status: Like with the emoji and Undo Like
//     (registerReactionProjectors).
//   - content.poll_voted of a remote poll: a Create of a Note per choice; content.poll_voted
//     and content.poll_closed of a local poll: JobPollUpdate, which records the Update of the
//     poll's status (registerPollVoteProjectors).
//
// Other activities (Block) are added here, one projector each, with the use cases that send
// them.
func RegisterOutboundProjectors(r *uow.Projectors, d RecorderDeps) {
	rec := recorder{d: d}
	if d.Accounts != nil {
		registerFollowProjectors(r, rec)
	}
	registerFavouriteProjectors(r, rec)
	registerReactionProjectors(r, rec)
	registerPollVoteProjectors(r, rec)
	recordCreate := func(ctx context.Context, tx uow.Tx, st appcontent.Status, rev appcontent.Revision) error {
		return rec.record(ctx, tx, federation.OutboundActivity{
			ActivityURI: federation.IRI(st.URI + "/activity"), Type: federation.ActivityCreate,
			ActorAccountID: st.AuthorAccountID, ObjectKind: federation.OutboundStatus, ObjectID: st.ID,
			CreatedAt: rev.RevisedAt,
		}, rev.ID)
	}
	r.Register(appcontent.EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.StatusCreated)
		// A held local reply goes out once it is approved (ADR-0155 §7).
		if !ok || !ev.Status.IsLocal || ev.Unpublished() {
			return nil
		}
		return recordCreate(ctx, tx, ev.Status, ev.Revision)
	})
	r.Register(appcontent.EventStatusReleased, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.StatusReleased)
		if !ok || !ev.Published() {
			return nil
		}
		return recordCreate(ctx, tx, ev.Status, ev.Revision)
	})
	r.Register(appcontent.EventStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.StatusRevised)
		if !ok || !ev.Status.IsLocal || ev.Unpublished() {
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: ev.Status.AuthorAccountID,
			ObjectKind: federation.OutboundStatus, ObjectID: ev.Status.ID, CreatedAt: ev.Revision.RevisedAt,
		}, ev.Revision.ID)
	})
	r.Register(appcontent.EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.StatusDeleted)
		if !ok || !ev.Status.IsLocal || ev.ByAccountErasure() {
			return nil
		}
		if unsent, err := neverPublished(ctx, d.Repository, ev.Status); err != nil || unsent {
			// A reply that waited for its approval went out only in its ReplyRequest, whose
			// withdrawal takes it back (ADR-0155 §7).
			return err
		}
		act, err := rec.insert(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityDelete, ActorAccountID: ev.Status.AuthorAccountID,
			ObjectKind: federation.OutboundStatus, ObjectID: ev.Status.ID, CreatedAt: ev.Deletion.DeletedAt,
		})
		if err != nil {
			return err
		}
		// The document is stored with the deletion, so that every delivery of the Delete
		// sends it whether or not the status's rows are still there (federation.md §7.1).
		doc, err := deleteDocument(act, d.Host, ev.Status)
		if err != nil {
			return err
		}
		if _, err := d.Repository.InsertOutboundDocument(ctx, tx, act.ID, doc, act.CreatedAt); err != nil {
			return err
		}
		return rec.enqueue(ctx, tx, act, fanOutPayload{})
	})
	r.Register(appcontent.EventStatusesDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.StatusesDeleted)
		if !ok {
			return nil
		}
		// The Delete of each local status, as StatusDeleted records one, with one statement for
		// the activities, one for their documents, and one for their fan-outs.
		var as []federation.OutboundActivity
		var sts []appcontent.Status
		for _, d := range ev.Deletions {
			if d.Status.IsLocal && !d.ByAccountErasure() {
				as = append(as, federation.OutboundActivity{
					Type: federation.ActivityDelete, ActorAccountID: d.Status.AuthorAccountID,
					ObjectKind: federation.OutboundStatus, ObjectID: d.Status.ID, CreatedAt: d.Deletion.DeletedAt,
				})
				sts = append(sts, d.Status)
			}
		}
		if len(as) == 0 {
			return nil
		}
		docs := make(map[string][]byte, len(as))
		var builtAt time.Time
		for i := range as {
			a, err := rec.prepare(as[i])
			if err != nil {
				return err
			}
			as[i], builtAt = a, a.CreatedAt
			if docs[a.ID], err = deleteDocument(a, d.Host, sts[i]); err != nil {
				return err
			}
		}
		if err := d.Repository.InsertOutboundActivities(ctx, tx, as); err != nil {
			return fmt.Errorf("record %d Deletes: %w", len(as), err)
		}
		if err := d.Repository.InsertOutboundDocuments(ctx, tx, docs, builtAt); err != nil {
			return err
		}
		jobs := make([]queue.NewJob, len(as))
		for i, a := range as {
			job, err := (fanOutPayload{OutboundActivityID: a.ID}).job(a)
			if err != nil {
				return err
			}
			jobs[i] = job
		}
		return rec.d.Outbox.EnqueueTx(ctx, tx, jobs...)
	})
	r.Register(appcontent.EventBoosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.Boosted)
		if !ok || !ev.Boost.Federates() {
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			ActivityURI: federation.IRI(ev.Boost.URI), Type: federation.ActivityAnnounce,
			ActorAccountID: ev.Boost.BoosterAccountID, ObjectKind: federation.OutboundBoost, ObjectID: ev.Boost.ID,
			CreatedAt: ev.Boost.BoostedAt,
		}, "")
	})
	r.Register(appcontent.EventBoostRetracted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.BoostRetracted)
		if !ok || !ev.Boost.Federates() {
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUndo, ActorAccountID: ev.Boost.BoosterAccountID,
			ObjectKind: federation.OutboundBoost, ObjectID: ev.Boost.ID, CreatedAt: ev.Retraction.RetractedAt,
		}, "")
	})
	r.Register(appaccount.EventProfileRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appaccount.ProfileRevised)
		if !ok || !ev.Federates() {
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: ev.Revision.AccountID,
			ObjectKind: federation.OutboundAccount, ObjectID: ev.Revision.AccountID, CreatedAt: ev.Revision.RevisedAt,
		}, "")
	})
	r.Register(apptimeline.EventFeaturedTagsChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(apptimeline.FeaturedTagsChanged)
		if !ok {
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: ev.AccountID,
			ObjectKind: federation.OutboundAccount, ObjectID: ev.AccountID, CreatedAt: ev.ChangedAt,
		}, "")
	})
	r.Register(appaccount.EventAliasesChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appaccount.AliasesChanged)
		if !ok {
			return nil
		}
		switch publishes, err := modeAllowsTx(ctx, tx, d.Modes, federation.FlowPublish); {
		case err != nil:
			return err
		case !publishes:
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: ev.AccountID,
			ObjectKind: federation.OutboundAccount, ObjectID: ev.AccountID, CreatedAt: ev.ChangedAt,
		}, "")
	})
	r.Register(appaccount.EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appaccount.LifecycleEventRecorded)
		if !ok {
			return nil
		}
		switch {
		case ev.DeletesLocalAccount():
			return rec.record(ctx, tx, federation.OutboundActivity{
				Type: federation.ActivityDelete, ActorAccountID: ev.Event.AccountID,
				ObjectKind: federation.OutboundAccount, ObjectID: ev.Event.AccountID, CreatedAt: ev.Event.OccurredAt,
			}, "")
		case ev.Deactivates(), ev.Reactivates():
			if d.Hidden != nil {
				suspended, err := d.Hidden.SuspendedTx(ctx, tx, ev.Event.AccountID)
				if err != nil || suspended {
					return err
				}
			}
			return rec.recordWith(ctx, tx, federation.OutboundActivity{
				Type: federation.ActivityUpdate, ActorAccountID: ev.Event.AccountID,
				ObjectKind: federation.OutboundAccount, ObjectID: ev.Event.AccountID, CreatedAt: ev.Event.OccurredAt,
			}, fanOutPayload{SharedInboxes: true})
		}
		return nil
	})
	registerMoveProjectors(r, rec)
}

// registerMoveProjectors records what the move of a local account sends to its followers
// (ADR-0089): the Update of the actor, whose movedTo Misskey reads when the Move arrives, then
// the Move; and the Update of the actor when the move is canceled. The moves of remote accounts
// send nothing.
func registerMoveProjectors(r *uow.Projectors, rec recorder) {
	r.Register(appaccount.EventAccountMoved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appaccount.Moved)
		if !ok || ev.Move.InitiatedBy != appaccount.MoveInitiatedBySelf {
			return nil
		}
		m := ev.Move
		if err := rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: m.AccountID,
			ObjectKind: federation.OutboundAccount, ObjectID: m.AccountID, CreatedAt: m.OccurredAt,
		}, ""); err != nil {
			return err
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityMove, ActorAccountID: m.AccountID,
			ObjectKind: federation.OutboundMove, ObjectID: m.ID, CreatedAt: m.OccurredAt,
		}, "")
	})
	r.Register(appaccount.EventMoveCanceled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appaccount.MoveCanceled)
		if !ok {
			return nil
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: ev.Move.AccountID,
			ObjectKind: federation.OutboundAccount, ObjectID: ev.Move.AccountID, CreatedAt: ev.Cancellation.CanceledAt,
		}, "")
	})
}

type recorder struct{ d RecorderDeps }

// record inserts a and enqueues its first fan-out batch in tx. An activity without a URI gets
// /ap/activities/{id} (ADR-0018). The ID is generated here, in the transaction: a retried
// attempt starts over with a new one, and nothing of the rolled-back attempt remains.
func (r recorder) record(ctx context.Context, tx uow.Tx, a federation.OutboundActivity, revisionID string) error {
	return r.recordWith(ctx, tx, a, fanOutPayload{RevisionID: revisionID})
}

// recordWith is record with the fan-out payload p, whose OutboundActivityID it sets.
func (r recorder) recordWith(ctx context.Context, tx uow.Tx, a federation.OutboundActivity, p fanOutPayload) error {
	a, err := r.insert(ctx, tx, a)
	if err != nil {
		return err
	}
	return r.enqueue(ctx, tx, a, p)
}

// recordAll is recordWith of many activities, each with an empty payload, with one statement
// for the activities and one for their jobs.
func (r recorder) recordAll(ctx context.Context, tx uow.Tx, as []federation.OutboundActivity) error {
	if len(as) == 0 {
		return nil
	}
	jobs := make([]queue.NewJob, len(as))
	for i, a := range as {
		a, err := r.prepare(a)
		if err != nil {
			return err
		}
		as[i] = a
		if jobs[i], err = (fanOutPayload{OutboundActivityID: a.ID}).job(a); err != nil {
			return err
		}
	}
	if err := r.d.Repository.InsertOutboundActivities(ctx, tx, as); err != nil {
		return fmt.Errorf("record %d outbound activities: %w", len(as), err)
	}
	return r.d.Outbox.EnqueueTx(ctx, tx, jobs...)
}

// localAmong reports, inside tx, which of ids are local accounts: with one read when Accounts
// can, one by one otherwise.
func (r recorder) localAmong(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error) {
	if b, ok := r.d.Accounts.(localityBatch); ok {
		return b.LocalAmong(ctx, tx, ids)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		local, err := r.d.Accounts.IsLocal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out[id] = local
	}
	return out, nil
}

// prepare gives a its ID, unless it has one, and its URI, and validates it.
func (r recorder) prepare(a federation.OutboundActivity) (federation.OutboundActivity, error) {
	if a.ID == "" {
		a.ID = r.d.IDs.NewID()
	}
	if a.ActivityURI == "" {
		a.ActivityURI = federation.IRI(appcontent.ActivityURI(r.d.Host, a.ID))
	}
	a.CreatedAt = kernel.Normalize(a.CreatedAt)
	return a, a.Validate()
}

// insert gives a its ID, unless it has one, and its URI, and inserts it in tx.
func (r recorder) insert(ctx context.Context, tx uow.Tx, a federation.OutboundActivity) (federation.OutboundActivity, error) {
	a, err := r.prepare(a)
	if err != nil {
		return federation.OutboundActivity{}, err
	}
	if err := r.d.Repository.InsertOutboundActivity(ctx, tx, a); err != nil {
		return federation.OutboundActivity{}, fmt.Errorf("record %s %s: %w", a.Type, a.ObjectID, err)
	}
	return a, nil
}

// firstAnswer records in tx that this server answers the received activity activityURI, and
// reports whether this is the first answer. A job that processes the activity again, after the
// answer committed, sends nothing more.
func (r recorder) firstAnswer(ctx context.Context, tx uow.Tx, activityURI federation.IRI, at time.Time) (bool, error) {
	first, err := r.d.Repository.InsertReceivedActivityAnswer(ctx, tx, activityURI, kernel.Normalize(at))
	if err != nil {
		return false, fmt.Errorf("record the answer to %s: %w", activityURI, err)
	}
	return first, nil
}

// enqueue enqueues the first fan-out batch of a in tx, with the payload p of a.
func (r recorder) enqueue(ctx context.Context, tx uow.Tx, a federation.OutboundActivity, p fanOutPayload) error {
	p.OutboundActivityID = a.ID
	job, err := p.job(a)
	if err != nil {
		return err
	}
	return r.d.Outbox.EnqueueTx(ctx, tx, job)
}

// neverPublished reports whether the local reply st went out only in a ReplyRequest: it was
// held, and its Create was never recorded.
func neverPublished(ctx context.Context, repo Repository, st appcontent.Status) (bool, error) {
	if st.InReplyToURI == "" {
		return false, nil
	}
	acts, err := repo.ListOutboundActivitiesOf(ctx, federation.OutboundStatus, st.ID)
	if err != nil {
		return false, err
	}
	requested := false
	for _, a := range acts {
		switch a.Type {
		case federation.ActivityCreate:
			return false, nil
		case federation.ActivityReplyRequest:
			requested = true
		}
	}
	return requested, nil
}
