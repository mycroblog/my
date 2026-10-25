// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Follows with remote accounts (federation.md §3.4, §7.1): the outbound projectors that turn the
// graph module's follow events between a local and a remote account into Follow, Accept,
// Reject, and Undo Follow, their fan-out to the other account, and the inbox handlers of the
// Follow, Accept, Reject, and Undo that remote servers send.

// LocalityReader tells local accounts from remote ones inside a transaction;
// *appaccount.Service implements it.
type LocalityReader interface {
	IsLocal(ctx context.Context, tx uow.Tx, accountID string) (bool, error)
}

// FollowReader reads a follow by ID; *appgraph.Service implements it.
type FollowReader interface {
	FindFollow(ctx context.Context, id string) (appgraph.FollowRecord, error)
}

// registerFollowProjectors records the activities of follows between a local and a remote
// account (federation.md §7.1):
//
//   - graph.followed and graph.follow_requested of a local follower of a remote account: Follow,
//     whose IRI is the follow's URI, so that the Accept answering it names it.
//   - graph.followed of a remote follower of a local account (accepted at once), and
//     graph.follow_accepted of one that the local followee accepted: Accept.
//   - graph.follow_resent of a remote follower whose server sent its Follow again for an active
//     follow: Accept of that Follow.
//   - graph.follow_rejected of a remote follower that the local followee or the system (a
//     block) rejected, and graph.follow_terminated of a remote follower that the local followee
//     removed: Reject.
//   - graph.follow_terminated of a local follower's follow of a remote account that the
//     follower ended (unfollowed, or by a block, moderation, or the move of the followee to
//     another account): Undo Follow.
//   - graph.follow_terminated of a remote follower's follow of a local account that moderation
//     ended (the suspension of the follower): Reject.
//
// What the remote side did itself (its Accept, Reject, Undo, or the deletion of either account)
// sends nothing back, and neither does a termination that is Unannounced (the suspension of a
// domain, ADR-0036).
func registerFollowProjectors(r *uow.Projectors, rec recorder) {
	sides := func(ctx context.Context, tx uow.Tx, f appgraph.FollowRecord) (followerLocal, followeeLocal bool, err error) {
		if followerLocal, err = rec.d.Accounts.IsLocal(ctx, tx, f.FollowerAccountID); err != nil {
			return false, false, err
		}
		followeeLocal, err = rec.d.Accounts.IsLocal(ctx, tx, f.FolloweeAccountID)
		return followerLocal, followeeLocal, err
	}
	// send records the activity t about f; followURI is the Follow that an Accept answers
	// when it is not f's own.
	send := func(ctx context.Context, tx uow.Tx, t federation.ActivityType, f appgraph.FollowRecord, at time.Time, followURI string) error {
		a := federation.OutboundActivity{
			Type: t, ActorAccountID: f.FolloweeAccountID, ObjectKind: federation.OutboundFollow, ObjectID: f.ID, CreatedAt: at,
		}
		switch t {
		case federation.ActivityFollow:
			a.ActivityURI, a.ActorAccountID = federation.IRI(f.URI), f.FollowerAccountID
		case federation.ActivityUndo:
			a.ActorAccountID = f.FollowerAccountID
		}
		return rec.recordWith(ctx, tx, a, fanOutPayload{FollowURI: followURI})
	}
	// requested sends the Follow of a local follower of a remote account.
	requested := func(ctx context.Context, tx uow.Tx, f appgraph.FollowRecord) error {
		followerLocal, followeeLocal, err := sides(ctx, tx, f)
		if err != nil || !followerLocal || followeeLocal {
			return err
		}
		return send(ctx, tx, federation.ActivityFollow, f, f.RequestedAt, "")
	}
	// answer sends the Accept or Reject of a local followee to a remote follower.
	answer := func(ctx context.Context, tx uow.Tx, t federation.ActivityType, f appgraph.FollowRecord, at time.Time, followURI string) error {
		followerLocal, followeeLocal, err := sides(ctx, tx, f)
		if err != nil || followerLocal || !followeeLocal {
			return err
		}
		return send(ctx, tx, t, f, at, followURI)
	}
	// answered is answer of the follow's own Follow.
	answered := func(ctx context.Context, tx uow.Tx, t federation.ActivityType, f appgraph.FollowRecord, at time.Time) error {
		return answer(ctx, tx, t, f, at, "")
	}

	r.Register(appgraph.EventFollowed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.Followed)
		if !ok {
			return nil
		}
		if err := requested(ctx, tx, ev.Follow); err != nil {
			return err
		}
		return answered(ctx, tx, federation.ActivityAccept, ev.Follow, ev.Acceptance.AcceptedAt)
	})
	r.Register(appgraph.EventFollowRequested, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.FollowRequested)
		if !ok {
			return nil
		}
		return requested(ctx, tx, ev.Follow)
	})
	r.Register(appgraph.EventFollowAccepted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.FollowAccepted)
		if !ok || ev.Acceptance.AcceptedBy == appgraph.AcceptedByRemote {
			return nil
		}
		return answered(ctx, tx, federation.ActivityAccept, ev.Follow, ev.Acceptance.AcceptedAt)
	})
	r.Register(appgraph.EventFollowRejected, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.FollowRejected)
		if !ok || ev.Rejection.RejectedBy == appgraph.RejectedByRemote {
			return nil
		}
		return answered(ctx, tx, federation.ActivityReject, ev.Follow, ev.Rejection.RejectedAt)
	})
	r.Register(appgraph.EventFollowResent, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.FollowResent)
		if !ok {
			return nil
		}
		return answer(ctx, tx, federation.ActivityAccept, ev.Follow, ev.Resend.ReceivedAt, ev.Resend.URI)
	})
	r.Register(appgraph.EventFollowTerminated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.FollowTerminated)
		if !ok || ev.Unannounced {
			return nil
		}
		at := ev.Termination.TerminatedAt
		switch ev.Termination.Reason {
		case appgraph.TerminatedRemovedByFollowee:
			return answered(ctx, tx, federation.ActivityReject, ev.Follow, at)
		case appgraph.TerminatedUnfollowed, appgraph.TerminatedBlocked, appgraph.TerminatedModeration, appgraph.TerminatedMoved:
			followerLocal, followeeLocal, err := sides(ctx, tx, ev.Follow)
			switch {
			case err != nil:
				return err
			case followerLocal && !followeeLocal:
				return send(ctx, tx, federation.ActivityUndo, ev.Follow, at, "")
			case !followerLocal && followeeLocal && ev.Termination.Reason == appgraph.TerminatedModeration:
				return send(ctx, tx, federation.ActivityReject, ev.Follow, at, "")
			}
		}
		return nil
	})
}

// followSubject loads the follow of a Follow, Accept, Reject, or Undo Follow and its other
// account. The actor of a Follow and its Undo is the follower; that of an Accept and a Reject
// is the followee.
func (f *FanOut) followSubject(ctx context.Context, s subject) (subject, error) {
	act := s.act
	if f.d.Follows == nil {
		return subject{}, fmt.Errorf("%w: %s of a follow without a follow reader", federation.ErrInvalidOutbound, act.Type)
	}
	fl, err := f.d.Follows.FindFollow(ctx, act.ObjectID)
	if errors.Is(err, appgraph.ErrNotFound) {
		return subject{}, fmt.Errorf("%w: follow %s", errGone, act.ObjectID)
	}
	if err != nil {
		return subject{}, err
	}
	s.follow = fl
	switch {
	case (act.Type == federation.ActivityFollow || act.Type == federation.ActivityUndo) && act.ActorAccountID == fl.FollowerAccountID:
		s.counterpart = fl.FolloweeAccountID
	case (act.Type == federation.ActivityAccept || act.Type == federation.ActivityReject) && act.ActorAccountID == fl.FolloweeAccountID:
		s.counterpart = fl.FollowerAccountID
	default:
		return subject{}, fmt.Errorf("%w: %s of follow %s by %s", federation.ErrInvalidOutbound, act.Type, fl.ID, act.ActorAccountID)
	}
	return s, nil
}

// counterpartTargets returns the inbox of the other account of a follow: its own inbox, which
// every implementation accepts follow activities at, or its shared inbox without one.
func (f *FanOut) counterpartTargets(ctx context.Context, s subject) ([]target, error) {
	inbox, err := f.d.Accounts.RemoteInbox(ctx, s.counterpart)
	if errors.Is(err, appaccount.ErrNotFound) {
		return nil, nil // a local account, or a remote one never fetched
	}
	if err != nil {
		return nil, err
	}
	iri := federation.IRI(inbox.InboxURL)
	if iri == "" {
		iri = federation.IRI(inbox.SharedInboxURL)
	}
	if t, ok := targetOf(iri); ok {
		return []target{t}, nil
	}
	return nil, nil
}

// followDocument builds a Follow, or the Accept, Reject, or Undo that embeds it, addressed to
// the other account of the follow (federation.md §7.4). The Follow's id is the follow's URI, so
// the Accept of a remote follower names the Follow that its server sent; the Accept of a Follow
// sent again names that one (subject.followURI).
func (f *FanOut) followDocument(ctx context.Context, s subject) ([]byte, error) {
	act, fl := s.act, s.follow
	follower, err := f.actorIRI(ctx, fl.FollowerAccountID)
	if err != nil {
		return nil, err
	}
	followee, err := f.actorIRI(ctx, fl.FolloweeAccountID)
	if err != nil {
		return nil, err
	}
	uri := federation.IRI(fl.URI)
	if s.followURI != "" {
		uri = s.followURI
	}
	follow := federation.NewActivityDocument(uri, federation.ActivityFollow, follower, fl.RequestedAt,
		federation.Audience{To: []federation.IRI{followee}}, followee)
	switch act.Type {
	case federation.ActivityFollow:
		return follow.Marshal()
	case federation.ActivityUndo:
		return federation.NewActivityDocument(act.ActivityURI, act.Type, follower, act.CreatedAt,
			federation.Audience{To: []federation.IRI{followee}}, follow.Embedded()).Marshal()
	}
	return federation.NewActivityDocument(act.ActivityURI, act.Type, followee, act.CreatedAt,
		federation.Audience{To: []federation.IRI{follower}}, follow.Embedded()).Marshal()
}

// RemoteFollows is the part of the graph module's public API that receives follow activities;
// *appgraph.Service implements it.
type RemoteFollows interface {
	ReceiveFollow(ctx context.Context, in appgraph.ReceiveFollowInput) error
	AcceptRemote(ctx context.Context, in appgraph.RemoteResponse) error
	RejectRemote(ctx context.Context, in appgraph.RemoteResponse) error
	TerminateRemote(ctx context.Context, in appgraph.RemoteUndo) error
}

// ActorsByURI finds stored accounts by their ActivityPub id, local or remote, without fetching;
// *appaccount.Service implements it.
type ActorsByURI interface {
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
}

// RelayFollows receives the follow activities about the relays (ADR-0061); *Relays implements
// it.
type RelayFollows interface {
	// ReceiveAnswer handles the Accept or Reject of a relay's Follow and reports whether it was
	// one.
	ReceiveAnswer(ctx context.Context, in *InboxActivity, accepted bool) (bool, error)
	// ReceiveFollow answers a Follow of the instance actor.
	ReceiveFollow(ctx context.Context, in *InboxActivity, instance appaccount.Actor) error
}

// FollowHandlerDeps are the ports of the follow inbox handlers.
type FollowHandlerDeps struct {
	Graph    RemoteFollows
	Accounts ActorsByURI
	// Relays receives the answers of the relays and the Follows of the instance actor; nil
	// drops the Follows of the instance actor and takes every answer as one about a follow.
	Relays RelayFollows
	// Quotes receives the answers to this server's QuoteRequests (ADR-0069 §3); nil takes them
	// as answers about follows, which drops them.
	Quotes QuoteAnswers
	// Refusals answers the Follows that the federation mode refuses with a Reject (ADR-0076),
	// and the Follows of the local accounts that moved (ADR-0089); nil drops them.
	Refusals FollowRefuser
	// Moves tells the local accounts that moved, which refuse new followers; nil means that none
	// moved.
	Moves MoveStates
}

// MoveStates reads the move in force of an account, appaccount.ErrNotFound when it has not
// moved; *appaccount.Service implements it.
type MoveStates interface {
	MoveState(ctx context.Context, accountID string) (appaccount.MoveState, error)
}

// moved reports whether the account accountID has a move in force; a nil moves moves none.
func moved(ctx context.Context, moves MoveStates, accountID string) (bool, error) {
	if moves == nil {
		return false, nil
	}
	_, err := moves.MoveState(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// FollowRefuser answers a Follow of a local actor that the federation mode refuses;
// *FollowRefusals implements it.
type FollowRefuser interface {
	Refuse(ctx context.Context, in *InboxActivity, followee appaccount.Actor) error
}

// QuoteAnswers receives the Accept and Reject of this server's QuoteRequests; *QuoteHandlers
// implements it.
type QuoteAnswers interface {
	ReceiveQuoteAnswer(ctx context.Context, in *InboxActivity, accepted bool) (bool, error)
}

// RegisterFollowHandlers sets the inbox handlers of the follow activities (federation.md §3.4):
//
//   - Follow of a local actor: graph.ReceiveFollow.
//   - Accept and Reject of a Follow a local account sent: graph.AcceptRemote and
//     graph.RejectRemote. The Follow is found by the object's id, and else by its embedded
//     actor (local) and object (the sender); an Accept or Reject of something other than a
//     Follow is dropped, and one of no known follow does nothing.
//   - Undo of a Follow: graph.TerminateRemote, found the same way; an unknown one leaves a
//     tombstone (federation.md §3.3).
//   - Undo of an Accept: graph.RejectRemote, the end of the follow it accepted.
//   - Accept and Reject of the Follow of a relay, and Follow of the instance actor: Relays
//     (ADR-0061).
//   - Accept and Reject of a QuoteRequest this server sent: Quotes (ADR-0069 §3).
//
// The federation mode decides the flows (ADR-0076): a Follow that it refuses (follow_in) gets a
// Reject from Refusals and records nothing, as does a Follow of a local account that moved
// (ADR-0089); the answers of the relays are about the instance
// actor's membership, which passes whenever the processor lets them through; the answers to
// QuoteRequests need publish, and the answers to the follows of local accounts follow_out.
//
// The embedded Follow is a hint only: the graph module matches it against the follows of the
// sender.
func RegisterFollowHandlers(p *InboxProcessor, d FollowHandlerDeps) {
	h := followHandlers{d: d}
	p.Handle(federation.ActivityFollow, h.follow)
	p.Handle(federation.ActivityAccept, h.respond(true, d.Graph.AcceptRemote))
	p.Handle(federation.ActivityReject, h.respond(false, d.Graph.RejectRemote))
	p.HandleUndo(federation.ActivityFollow, h.undoFollow)
	p.HandleUndo(federation.ActivityAccept, h.undoAccept)
}

type followHandlers struct{ d FollowHandlerDeps }

// local returns the ID of the local account with the ActivityPub id uri, "" when there is none.
func (h followHandlers) local(ctx context.Context, uri federation.IRI) (string, error) {
	if uri == "" {
		return "", nil
	}
	a, err := h.d.Accounts.FindByURI(ctx, string(uri))
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return "", nil
	case err != nil:
		return "", err
	case !a.Account.IsLocal():
		return "", nil
	}
	return a.Account.ID, nil
}

// embeddedFollow returns the Follow that ref embeds, nil when it embeds none. An embedded
// activity of another type fails with ErrDropActivity.
func embeddedFollow(ref federation.Ref) (*federation.Activity, error) {
	doc := ref.Untrusted()
	if doc == nil || doc.Activity == nil {
		return nil, nil
	}
	if doc.Activity.Type != federation.ActivityFollow {
		return nil, fmt.Errorf("%w: the object is a %s, not a Follow", ErrDropActivity, doc.Activity.RawType)
	}
	return doc.Activity, nil
}

// graphError turns the errors of the graph module's remote follow use cases into what the
// processor makes of them.
func graphError(err error) error {
	switch {
	case errors.Is(err, appgraph.ErrNotFound):
		return fmt.Errorf("%w: %w", ErrUnknownObject, err)
	case errors.Is(err, appgraph.ErrAccountUnavailable), errors.Is(err, appgraph.ErrRemoteAccount),
		errors.Is(err, appgraph.ErrSelfFollow):
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return err
}

func (h followHandlers) follow(ctx context.Context, in *InboxActivity) error {
	mode := in.mode()
	refused := !federation.Allows(mode, federation.FlowFollowIn)
	if refused && !federation.Refuses(mode, federation.FlowFollowIn) {
		return fmt.Errorf("%w: %w: Follow in mode %s", ErrDropActivity, ErrFederationRestricted, mode)
	}
	if object := in.Activity.Object().ID; object != "" {
		a, err := h.d.Accounts.FindByURI(ctx, string(object))
		switch {
		case err == nil && a.Account.IsLocal() && refused:
			if h.d.Refusals == nil {
				return fmt.Errorf("%w: %w: a Follow in mode %s", ErrDropActivity, ErrFederationRestricted, mode)
			}
			return h.d.Refusals.Refuse(ctx, in, a)
		case err == nil && a.IsInstanceActor():
			if h.d.Relays == nil {
				return fmt.Errorf("%w: a Follow of the instance actor", ErrDropActivity)
			}
			return h.d.Relays.ReceiveFollow(ctx, in, a)
		case err == nil && a.Account.IsLocal():
			switch m, err := moved(ctx, h.d.Moves, a.Account.ID); {
			case err != nil:
				return err
			case m && h.d.Refusals == nil:
				return fmt.Errorf("%w: a Follow of %s, which moved", ErrDropActivity, a.Account.URI)
			case m:
				return h.d.Refusals.Refuse(ctx, in, a)
			}
		case err != nil && !errors.Is(err, appaccount.ErrNotFound):
			return err
		}
	}
	followee, err := h.local(ctx, in.Activity.Object().ID)
	if err != nil {
		return err
	}
	if followee == "" || refused {
		return fmt.Errorf("%w: %s is not a local actor", ErrDropActivity, in.Activity.Object().ID)
	}
	uri := in.Activity.ID
	if uri == "" {
		uri = in.Received.ActivityURI
	}
	return graphError(h.d.Graph.ReceiveFollow(ctx, appgraph.ReceiveFollowInput{
		URI: string(uri), FollowerID: in.Actor.Account.ID, FolloweeID: followee,
	}))
}

// respond returns the handler of an Accept (accepted) or Reject: the answer of a relay to its
// Follow, or else one about a follow, which use applies.
func (h followHandlers) respond(accepted bool, use func(context.Context, appgraph.RemoteResponse) error) InboxHandler {
	return func(ctx context.Context, in *InboxActivity) error {
		if h.d.Relays != nil {
			if handled, err := h.d.Relays.ReceiveAnswer(ctx, in, accepted); handled {
				return err
			}
		}
		if h.d.Quotes != nil && federation.Allows(in.mode(), federation.FlowPublish) {
			if handled, err := h.d.Quotes.ReceiveQuoteAnswer(ctx, in, accepted); handled {
				return err
			}
		}
		if !federation.Allows(in.mode(), federation.FlowFollowOut) {
			return fmt.Errorf("%w: %w: %s in mode %s", ErrDropActivity, ErrFederationRestricted, in.Activity.RawType, in.mode())
		}
		ref := in.Activity.Object()
		follow, err := embeddedFollow(ref)
		if err != nil {
			return err
		}
		var follower string
		if follow != nil {
			if follow.Object().ID != in.Activity.Actor {
				return fmt.Errorf("%w: the Follow it answers is not of %s", ErrDropActivity, in.Activity.Actor)
			}
			if follower, err = h.local(ctx, follow.Actor); err != nil {
				return err
			}
		}
		return graphError(use(ctx, appgraph.RemoteResponse{
			FollowURI: string(ref.ID), FolloweeID: in.Actor.Account.ID, FollowerID: follower,
			ActivityURI: string(in.Received.ActivityURI),
		}))
	}
}

func (h followHandlers) undoFollow(ctx context.Context, in *InboxActivity) error {
	ref := in.Activity.Object()
	follow, err := embeddedFollow(ref)
	if err != nil {
		return err
	}
	var followee string
	if follow != nil {
		if follow.Actor != in.Activity.Actor {
			return fmt.Errorf("%w: the Follow it takes back is not of %s", ErrDropActivity, in.Activity.Actor)
		}
		if followee, err = h.local(ctx, follow.Object().ID); err != nil {
			return err
		}
	}
	return graphError(h.d.Graph.TerminateRemote(ctx, appgraph.RemoteUndo{
		FollowURI: string(ref.ID), FollowerID: in.Actor.Account.ID, FolloweeID: followee,
	}))
}

// undoAccept ends the follow whose Accept the followee takes back. Only an embedded Accept says
// which follow that is.
func (h followHandlers) undoAccept(ctx context.Context, in *InboxActivity) error {
	doc := in.Activity.Object().Untrusted()
	if doc == nil || doc.Activity == nil || doc.Activity.Type != federation.ActivityAccept {
		return ErrUnknownObject
	}
	accept := doc.Activity
	if accept.Actor != in.Activity.Actor {
		return fmt.Errorf("%w: the Accept it takes back is not of %s", ErrDropActivity, in.Activity.Actor)
	}
	ref := accept.Object()
	follow, err := embeddedFollow(ref)
	if err != nil {
		return err
	}
	var follower string
	if follow != nil {
		if follower, err = h.local(ctx, follow.Actor); err != nil {
			return err
		}
	}
	err = h.d.Graph.RejectRemote(ctx, appgraph.RemoteResponse{
		FollowURI: string(ref.ID), FolloweeID: in.Actor.Account.ID, FollowerID: follower,
		ActivityURI: string(in.Received.ActivityURI),
	})
	if errors.Is(err, appgraph.ErrNotFound) {
		// The follow is not known or has ended; a tombstone of the Accept would stop nothing.
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return graphError(err)
}
