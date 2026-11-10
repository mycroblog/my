// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The approvals of replies and boosts between this server and remote ones (GoToSocial's
// interaction policies, ADR-0155 §6, §7): the ReplyRequest and AnnounceRequest of a local
// account's interaction with a remote status and the answer to them, the answer to a remote
// ReplyRequest or AnnounceRequest of an interaction with a local status, and the answers that
// the author of a local status gives later, which the quotes share.

// interactionRequestsPath is the path of the IRIs of the ReplyRequests and AnnounceRequests of
// this server before the request's ID (appcontent.InteractionRequestURI).
const interactionRequestsPath = "/ap/interaction_requests/"

// InteractionApprovals is the part of the content module's public API that asks and answers the
// approvals of replies and boosts; *appcontent.Interactions implements it.
type InteractionApprovals interface {
	ReceiveInteractionRequest(ctx context.Context, in appcontent.RemoteInteractionRequest) (appcontent.RequestAnswer, error)
	ReceiveInteractionAnswer(ctx context.Context, requestID string, accepted bool, authorizationURI string) (appcontent.InteractionState, bool, error)
	AcceptRemoteInteraction(ctx context.Context, kind appcontent.InteractionKind, interactingURI, targetURI, uri string) (bool, error)
	FindRequest(ctx context.Context, id string) (appcontent.InteractionRequest, appcontent.InteractionState, error)
	FindOutboundRequestOf(ctx context.Context, interactingURI string) (appcontent.InteractionRequest, bool, error)
	WithdrawRemoteInteraction(ctx context.Context, kind appcontent.InteractionKind, interactingURI, accountID string) (bool, error)
}

// requestTypes are the activities that ask for the approval of each kind.
var requestTypes = map[appcontent.InteractionKind]federation.ActivityType{
	appcontent.InteractionReply:    federation.ActivityReplyRequest,
	appcontent.InteractionAnnounce: federation.ActivityAnnounceRequest,
	appcontent.InteractionQuote:    federation.ActivityQuoteRequest,
}

// interactionRequest returns the handler of a remote request of kind (ReplyRequest or
// AnnounceRequest) of an interaction with a local status (ADR-0155 §6): the content module
// decides it, and an approval or a refusal goes to the actor's inbox as an Accept, with this
// server's authorization as its result, or a Reject; a pending request waits for the author's
// answer. The interaction (instrument) must be of the actor's origin. When the federation mode
// does not take the interaction in (ingest, ADR-0006), the request is refused without asking.
func (h *QuoteHandlers) interactionRequest(kind appcontent.InteractionKind) InboxHandler {
	return func(ctx context.Context, in *InboxActivity) error {
		act := in.Activity
		target, interacting := act.Object().ID, act.Instrument.ID
		switch {
		case target == "" || !h.isLocal(target):
			return fmt.Errorf("%w: %s of %q, which is not a local status", ErrDropActivity, act.Type, target)
		case interacting == "" || !federation.SameOrigin(interacting, act.Actor):
			return fmt.Errorf("%w: %s of %q by %s", ErrDropActivity, act.Type, interacting, act.Actor)
		}
		ans, err := h.receiveRequest(ctx, in, kind, target, interacting)
		switch {
		case errors.Is(err, appcontent.ErrNotFound), errors.Is(err, appcontent.ErrInteractionRefused):
			return fmt.Errorf("%w: %s of %s: %w", ErrDropActivity, act.Type, target, err)
		case err != nil:
			return err
		case ans.State.State == appcontent.InteractionRequestPending:
			return nil // the author answers it
		}
		inbox, requester, ok, err := h.remoteInbox(ctx, nil, in.Actor.Account.ID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: no inbox for %s", ErrDropActivity, act.Actor)
		}
		object := answerObject(kind, act.ID, act.Type, act.Actor, target, interacting)
		accepted := ans.State.State == appcontent.InteractionRequestAccepted
		return h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			first, err := h.rec.firstAnswer(ctx, tx, in.Received.ActivityURI, h.d.Clock.Now())
			if err != nil || !first {
				return err
			}
			return h.answerTx(ctx, tx, answerOf{
				kind: kind, accepted: accepted, authorization: federation.IRI(ans.AuthorizationURI), target: ans.Target,
				requester: requester, inbox: inbox, object: object,
			})
		})
	}
}

// receiveRequest asks the content module to decide a remote request, or refuses it when the
// federation mode does not take the interaction in.
func (h *QuoteHandlers) receiveRequest(ctx context.Context, in *InboxActivity, kind appcontent.InteractionKind, target, interacting federation.IRI) (appcontent.RequestAnswer, error) {
	if federation.Allows(in.mode(), federation.FlowIngest) {
		return h.d.Interactions.ReceiveInteractionRequest(ctx, appcontent.RemoteInteractionRequest{
			Kind: kind, TargetURI: string(target), InteractingURI: string(interacting), ActorAccountID: in.Actor.Account.ID,
			ActivityURI: string(in.Activity.ID),
		})
	}
	st, err := h.d.Statuses.FindStatusByURI(ctx, string(target))
	switch {
	case err != nil:
		return appcontent.RequestAnswer{}, err
	case !st.IsLocal:
		return appcontent.RequestAnswer{}, fmt.Errorf("status %s: %w", st.ID, appcontent.ErrNotFound)
	}
	return appcontent.RequestAnswer{Target: st, State: appcontent.InteractionState{State: appcontent.InteractionRequestRejected}}, nil
}

// answerObject is the object of the Accept or Reject of the request id of type by actor, of the
// interaction interacting with the status target: the request embedded, and for a ReplyRequest or
// an AnnounceRequest the interaction's IRI after it (ADR-0155 §6). GoToSocial takes the answer of
// its AnnounceRequest, and the Reject of its ReplyRequest, only from the interaction's IRI; the
// answer of a QuoteRequest embeds the request alone (ADR-0069 §2).
func answerObject(kind appcontent.InteractionKind, id federation.IRI, typ federation.ActivityType, actor, target, interacting federation.IRI) any {
	request := federation.NewActivityDocument(id, typ, actor, time.Time{}, federation.Audience{}, target).Embedded()
	request.Instrument = interacting
	if kind == appcontent.InteractionQuote {
		return request
	}
	return []any{request, interacting}
}

// answerOf is an Accept or a Reject of a remote account's interaction with the local status
// target, to requester's inbox: object is the request that it answers, or the interaction that
// came without one, and interactionTarget the target then.
type answerOf struct {
	kind              appcontent.InteractionKind
	accepted          bool
	authorization     federation.IRI
	target            appcontent.Status
	requester, inbox  federation.IRI
	object            any
	interactionTarget federation.IRI
}

// answerTx records in tx the Accept or Reject a by the author of its target, of the outbound kind
// of a's interaction, and stores its document for its one inbox. The Accept of a reply without a
// request to a public or unlisted status is addressed to the public and goes to the author's
// followers too, so that third servers learn of the approval (ADR-0155 §6).
func (h *QuoteHandlers) answerTx(ctx context.Context, tx uow.Tx, a answerOf) error {
	typ, result := federation.ActivityReject, federation.IRI("")
	if a.accepted {
		typ, result = federation.ActivityAccept, a.authorization
	}
	out, err := h.rec.insert(ctx, tx, federation.OutboundActivity{
		Type: typ, ActorAccountID: a.target.AuthorAccountID, ObjectKind: federation.ApprovalKindOf(string(a.kind)), ObjectID: a.target.ID,
		CreatedAt: h.d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	actor := federation.IRI(appaccount.ActorURI(h.d.Recorder.Host, a.target.AuthorAccountID))
	aud := federation.Audience{To: []federation.IRI{a.requester}}
	followers := a.interactionTarget != "" && a.accepted &&
		(a.target.Visibility == appcontent.VisibilityPublic || a.target.Visibility == appcontent.VisibilityUnlisted)
	if followers {
		aud.Cc = []federation.IRI{federation.PublicIRI, federation.FollowersOf(actor)}
	}
	doc := federation.NewActivityDocument(out.ActivityURI, typ, actor, out.CreatedAt, aud, a.object)
	doc.Result, doc.Target = result, a.interactionTarget
	return h.storeTo(ctx, tx, out, doc, fanOutPayload{Inbox: string(a.inbox), Followers: followers})
}

// recordDeferredAnswer records the Accept or Reject of the answer ev to a remote account's
// interaction with a local status that no handler answered at once (InteractionAnswered.Deferred):
// the author's answer to a pending request, which embeds the request, and the answer to an
// interaction that came without a request, which names it and its target.
func (h *QuoteHandlers) recordDeferredAnswer(ctx context.Context, tx uow.Tx, ev appcontent.InteractionAnswered) error {
	r := ev.Request
	if ev.Target == nil {
		return nil
	}
	target, err := h.d.Statuses.FindStatusByURITx(ctx, tx, r.TargetURI)
	if errors.Is(err, appcontent.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	inbox, requester, ok, err := h.remoteInbox(ctx, tx, r.InteractingAccountID)
	if err != nil || !ok {
		return err
	}
	a := answerOf{
		kind: r.Kind, accepted: ev.State.State == appcontent.InteractionRequestAccepted, authorization: federation.IRI(ev.State.AuthorizationURI),
		target: target, requester: requester, inbox: inbox, object: federation.IRI(r.InteractingURI),
	}
	if r.ActivityURI != "" {
		a.object = answerObject(r.Kind, federation.IRI(r.ActivityURI), requestTypes[r.Kind], requester, federation.IRI(r.TargetURI), federation.IRI(r.InteractingURI))
	} else {
		a.interactionTarget = federation.IRI(r.TargetURI)
	}
	return h.answerTx(ctx, tx, a)
}

// recordReplyRequest records the ReplyRequest of the local reply that r asks the approval of, to
// the inbox of the author of the remote status it replies to (ADR-0155 §7). Its document, which
// embeds the reply's Note, is built when it goes out (OutboundStatus).
func (h *QuoteHandlers) recordReplyRequest(ctx context.Context, tx uow.Tx, r appcontent.InteractionRequest) error {
	reply, err := h.d.Statuses.FindStatusByURITx(ctx, tx, r.InteractingURI)
	if err != nil {
		return err
	}
	inbox, _, ok, err := h.remoteInbox(ctx, tx, r.TargetAccountID)
	if err != nil || !ok {
		return err
	}
	return h.rec.recordWith(ctx, tx, federation.OutboundActivity{
		ActivityURI: federation.IRI(appcontent.InteractionRequestURI(h.d.Recorder.Host, r.ID)), Type: federation.ActivityReplyRequest,
		ActorAccountID: reply.AuthorAccountID, ObjectKind: federation.OutboundStatus, ObjectID: reply.ID, CreatedAt: h.d.Clock.Now(),
	}, fanOutPayload{Inbox: string(inbox)})
}

// recordAnnounceRequest records the AnnounceRequest of the local boost that r asks the approval
// of, to the inbox of the author of the remote status target (ADR-0155 §7): object is the
// status, instrument the Announce that goes out once it is approved.
func (h *QuoteHandlers) recordAnnounceRequest(ctx context.Context, tx uow.Tx, r appcontent.InteractionRequest, target *appcontent.StatusRef) error {
	if target == nil {
		return nil
	}
	inbox, author, ok, err := h.remoteInbox(ctx, tx, r.TargetAccountID)
	if err != nil || !ok {
		return err
	}
	actor := federation.IRI(appaccount.ActorURI(h.d.Recorder.Host, r.InteractingAccountID))
	vis := r.Visibility
	if !vis.Valid() {
		vis = target.Visibility
	}
	boost := appcontent.Boost{URI: r.InteractingURI, BoosterAccountID: r.InteractingAccountID, Visibility: vis, BoostedAt: r.CreatedAt}
	announce := announceDocument(boost, appcontent.Status{URI: target.URI}, actor, vis, []federation.IRI{author}).Embedded()
	act, err := h.rec.insert(ctx, tx, federation.OutboundActivity{
		ActivityURI: federation.IRI(appcontent.InteractionRequestURI(h.d.Recorder.Host, r.ID)), Type: federation.ActivityAnnounceRequest,
		ActorAccountID: r.InteractingAccountID, ObjectKind: federation.OutboundAnnounce, ObjectID: r.ID, CreatedAt: h.d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	doc := federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, federation.Audience{To: []federation.IRI{author}}, federation.IRI(target.URI))
	doc.Instrument = announce
	return h.store(ctx, tx, act, doc, inbox)
}

// recordWithdrawal records the withdrawal of the local account's pending request r, which the
// author of the remote status answered not yet (ADR-0155 §7), to the inbox that its request went
// to: the Delete of the reply, a Tombstone, or the Undo of the Announce that its AnnounceRequest
// embedded. Nobody else was told of the interaction, so nobody else is told of its end.
func (h *QuoteHandlers) recordWithdrawal(ctx context.Context, tx uow.Tx, r appcontent.InteractionRequest) error {
	if r.Kind == appcontent.InteractionQuote || r.TargetAccountID == "" {
		return nil
	}
	inbox, author, ok, err := h.remoteInbox(ctx, tx, r.TargetAccountID)
	if err != nil || !ok {
		return err
	}
	actor := federation.IRI(appaccount.ActorURI(h.d.Recorder.Host, r.InteractingAccountID))
	typ, kind := federation.ActivityDelete, federation.OutboundReply
	var object any = federation.TombstoneOf(federation.IRI(r.InteractingURI))
	if r.Kind == appcontent.InteractionAnnounce {
		vis := r.Visibility
		if !vis.Valid() {
			vis = appcontent.VisibilityPublic
		}
		boost := appcontent.Boost{URI: r.InteractingURI, BoosterAccountID: r.InteractingAccountID, Visibility: vis, BoostedAt: r.CreatedAt}
		typ, kind = federation.ActivityUndo, federation.OutboundAnnounce
		object = announceDocument(boost, appcontent.Status{URI: r.TargetURI}, actor, vis, []federation.IRI{author}).Embedded()
	}
	act, err := h.rec.insert(ctx, tx, federation.OutboundActivity{
		Type: typ, ActorAccountID: r.InteractingAccountID, ObjectKind: kind, ObjectID: r.ID, CreatedAt: h.d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	doc := federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, federation.Audience{To: []federation.IRI{author}}, object)
	return h.store(ctx, tx, act, doc, inbox)
}

// receiveInteractionAnswer takes an Accept or Reject whose object is this server's ReplyRequest
// or AnnounceRequest (ADR-0155 §7), and reports whether it was one. The answer must come from the
// author of the status interacted with. An Accept must name an authorization of that author's
// host that approves the interaction (checkAuthorization); the reply then goes out with its
// Create, or the boost with its Announce. A Reject refuses it.
func (h *QuoteHandlers) receiveInteractionAnswer(ctx context.Context, in *InboxActivity, accepted bool) (bool, error) {
	if h.d.Interactions == nil {
		return false, nil
	}
	id, ok, err := h.answeredRequest(ctx, in.Activity.Objects)
	if err != nil || !ok {
		return ok, err
	}
	r, _, err := h.d.Interactions.FindRequest(ctx, id)
	if errors.Is(err, appcontent.ErrNotFound) {
		return true, fmt.Errorf("%w: answer to the missing interaction request %s", ErrDropActivity, id)
	}
	if err != nil {
		return true, err
	}
	if r.Direction != appcontent.DirectionOutbound || r.TargetAccountID != in.Actor.Account.ID {
		return true, fmt.Errorf("%w: %s answered the interaction request %s of another's status", ErrDropActivity, in.Activity.Actor, id)
	}
	result := ""
	if accepted {
		typ := federation.ObjectReplyAuthorization
		if r.Kind == appcontent.InteractionAnnounce {
			typ = federation.ObjectAnnounceAuthorization
		}
		err := checkAuthorization(ctx, h.d.Fetcher, typ, in.Activity.Result, federation.IRI(in.Actor.Account.URI),
			federation.IRI(r.InteractingURI), federation.IRI(r.TargetURI))
		switch {
		case isFetchError(err), errors.Is(err, ErrQuoteUnauthorized):
			return true, fmt.Errorf("%w: %w", ErrDropActivity, err)
		case err != nil:
			return true, err
		}
		result = string(in.Activity.Result)
	}
	_, _, err = h.d.Interactions.ReceiveInteractionAnswer(ctx, id, accepted, result)
	return true, quoteError(err)
}

// answeredRequest returns the ID of this server's request that an Accept or Reject with the
// objects answers, as liberally as ADR-0155 §6 allows: an object that is the request (its IRI,
// or the request embedded) or, failing that, one that is the IRI of the local reply or Announce
// whose approval a pending request asks for.
func (h *QuoteHandlers) answeredRequest(ctx context.Context, objects []federation.Ref) (string, bool, error) {
	for _, o := range objects {
		if id, ok := h.interactionRequestID(o.ID); ok {
			return id, true, nil
		}
	}
	for _, o := range objects {
		if o.ID == "" || !h.isLocal(o.ID) || o.Untrusted() != nil {
			continue
		}
		r, ok, err := h.d.Interactions.FindOutboundRequestOf(ctx, string(o.ID))
		if err != nil || ok {
			return r.ID, ok, err
		}
	}
	return "", false, nil
}

// interactionRequestID returns the request whose ReplyRequest or AnnounceRequest is the IRI id.
func (h *QuoteHandlers) interactionRequestID(id federation.IRI) (string, bool) {
	rest, ok := strings.CutPrefix(string(id), h.origin+interactionRequestsPath)
	if !ok || !kernel.ValidID(rest) {
		return "", false
	}
	return rest, true
}

// verifyThirdPartyReply approves the stored remote reply obj to a remote status that its
// author's server approved (ADR-0155 §6): its replyAuthorization must be an authorization of the
// parent's author, of the author's host, that names the reply and its parent
// (checkAuthorization). A reply that names none, or one that does not verify, stays held; a
// reply to a local status is as its request was answered, and needs nothing here.
func (h *StatusHandlers) verifyThirdPartyReply(ctx context.Context, obj *federation.Object) {
	if h.d.Interactions == nil || h.d.Profiles == nil || obj.ReplyAuthorization == "" || obj.InReplyTo == "" || h.isLocal(obj.InReplyTo) {
		return
	}
	parent, err := h.d.Statuses.FindStatusByURI(ctx, string(obj.InReplyTo))
	if err != nil {
		return
	}
	author, err := h.d.Profiles.FindByID(ctx, parent.AuthorAccountID)
	if err == nil {
		err = checkAuthorization(ctx, h.d.Fetcher, federation.ObjectReplyAuthorization, obj.ReplyAuthorization,
			federation.IRI(author.Account.URI), obj.ID, federation.IRI(parent.URI))
	}
	if err == nil {
		_, err = h.d.Interactions.AcceptRemoteInteraction(ctx, appcontent.InteractionReply, string(obj.ID), parent.URI, string(obj.ReplyAuthorization))
	}
	if err != nil {
		h.d.Logger.InfoContext(ctx, "reply authorization not verified", "uri", string(obj.ID), "reason", err)
	}
}

// verifyAnnounce checks the announceAuthorization of the Announce in of the remote status st
// (ADR-0155 §6): an authorization of st's author, of the author's host, that names the Announce
// and st. A fetch that fails, or one that does not verify, fails with ErrQuoteUnauthorized.
func (h *StatusHandlers) verifyAnnounce(ctx context.Context, in *InboxActivity, st appcontent.Status) error {
	if h.d.Profiles == nil {
		return fmt.Errorf("%w: no profiles to read %s", ErrQuoteUnauthorized, st.AuthorAccountID)
	}
	author, err := h.d.Profiles.FindByID(ctx, st.AuthorAccountID)
	if err != nil {
		return err
	}
	err = checkAuthorization(ctx, h.d.Fetcher, federation.ObjectAnnounceAuthorization, in.Activity.Authorization,
		federation.IRI(author.Account.URI), federation.IRI(activityURI(in)), federation.IRI(st.URI))
	if isFetchError(err) {
		return fmt.Errorf("%w: %w", ErrQuoteUnauthorized, err)
	}
	return err
}
