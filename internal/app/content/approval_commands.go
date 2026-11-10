// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The commands of the approvals of interactions (ADR-0155 §6–§8): a remote ReplyRequest or
// AnnounceRequest of a local status, the author's answer to a pending request, the answer of a
// remote server to a local account's request, and the reads of the requests and of this
// server's authorizations.

var (
	// ErrInteractionAwaitsApproval reports an interaction that waits for the answer of the
	// author of its target: it is recorded as a request, and the interaction itself only once
	// it is approved.
	ErrInteractionAwaitsApproval = errors.New("content: the interaction awaits the approval of the author")
	// ErrInteractionRefused reports an interaction that the rules of its target refuse.
	ErrInteractionRefused = errors.New("content: the interaction is refused")
	// ErrRequestAnswered reports an answer to a request that is not pending.
	ErrRequestAnswered = errors.New("content: the interaction request is answered already")
)

// RemoteInteractionRequest is a remote ReplyRequest or AnnounceRequest of an interaction with a
// local status (ADR-0155 §6).
type RemoteInteractionRequest struct {
	Kind content.InteractionKind
	// TargetURI is the local status; InteractingURI the reply or the Announce.
	TargetURI      string
	InteractingURI string
	// ActorAccountID is the remote account that asks: the actor of the request.
	ActorAccountID string
	// ActivityURI is the id of the request.
	ActivityURI string
}

// RequestAnswer is how a request is answered: its stored request, its state, and the local
// status interacted with.
type RequestAnswer struct {
	Request content.InteractionRequest
	State   content.InteractionState
	Target  content.Status
	// AuthorizationURI is the authorization of an accepted request, "" for the others.
	AuthorizationURI string
}

// ReceiveInteractionRequest decides a remote ReplyRequest or AnnounceRequest (ADR-0155 §6) and
// records it: approved at once, with this server's authorization, when the rules of the status
// admit the actor automatically; pending, for its author to answer, when only a manual rule does;
// refused otherwise, as for an actor that may not see the status (content.CanView in
// ContextObjectFetch), that is under the interaction restriction (FR-708), or that blocks or is
// blocked by the author. A request received again gets the answer given before. A target that
// is not a live local status fails with ErrNotFound; a kind other than reply or announce with
// ErrInteractionRefused.
func (s *Interactions) ReceiveInteractionRequest(ctx context.Context, in RemoteInteractionRequest) (RequestAnswer, error) {
	if s.d.Quotes == nil {
		return RequestAnswer{}, fmt.Errorf("interaction requests: %w", ErrNotFound)
	}
	if in.Kind != content.InteractionReply && in.Kind != content.InteractionAnnounce {
		return RequestAnswer{}, fmt.Errorf("%s request: %w", in.Kind, ErrInteractionRefused)
	}
	target, err := s.localTarget(ctx, in.TargetURI)
	if err != nil {
		return RequestAnswer{}, err
	}
	d, err := s.remoteVerdict(ctx, in.Kind, in.ActorAccountID, target)
	if err != nil {
		return RequestAnswer{}, err
	}
	r := content.InteractionRequest{
		Kind: in.Kind, Direction: content.DirectionInbound, TargetURI: target.URI, TargetAccountID: target.AuthorAccountID,
		InteractingURI: in.InteractingURI, InteractingAccountID: in.ActorAccountID, ActivityURI: in.ActivityURI,
	}
	var out RequestAnswer
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		out = RequestAnswer{Target: target}
		a, err := s.firstAnswer(ctx, tx, d, r, target, false)
		if err != nil {
			return err
		}
		ref := RefOf(target)
		req, st, _, err := s.approvals().request(ctx, tx, r, a, &ref)
		out.Request, out.State, out.AuthorizationURI = req, st, st.AuthorizationURI
		return err
	})
	return out, err
}

// firstAnswer returns the answer that the decision d gives r at once, with the authorization of
// an approved interaction recorded in tx; deferred is whether its answer must still be
// delivered (InteractionAnswered.Deferred).
func (s *Interactions) firstAnswer(ctx context.Context, tx uow.Tx, d content.InteractionDecision, r content.InteractionRequest, target content.Status, deferred bool) (answer, error) {
	switch d {
	case content.InteractionPending:
		return answer{}, nil
	case content.InteractionDenied:
		return answer{kind: content.InteractionEventRejected, deferred: deferred}, nil
	}
	uri, err := s.approvals().authorize(ctx, tx, r.Kind, r.InteractingURI, r.InteractingAccountID, target)
	if err != nil {
		return answer{}, err
	}
	return answer{kind: content.InteractionEventAccepted, authorization: uri, deferred: deferred}, nil
}

// localTarget returns the live local status uri; any other fails with ErrNotFound.
func (s *Interactions) localTarget(ctx context.Context, uri string) (content.Status, error) {
	st, err := s.d.Statuses.FindStatusByURI(ctx, uri)
	if err != nil {
		return content.Status{}, err
	}
	if st, err = s.audience().live(ctx, st.ID); err != nil {
		return content.Status{}, err
	}
	if !st.IsLocal {
		return content.Status{}, fmt.Errorf("status %s is remote: %w", st.ID, ErrNotFound)
	}
	return st, nil
}

// remoteVerdict decides the interaction by kind of the remote account accountID with the local
// status st (content.MayInteract): refused for an account that is not remote, may not see st as
// an ActivityPub fetch by it would, is under the interaction restriction, or blocks or is
// blocked by the author.
func (s *Interactions) remoteVerdict(ctx context.Context, kind content.InteractionKind, accountID string, st content.Status) (content.InteractionDecision, error) {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if err != nil || actor.Account.IsLocal() {
		return content.InteractionDenied, nil //nolint:nilerr // an unknown actor is refused
	}
	r, err := s.d.Restrictions.Restrictions(ctx, accountID)
	if err != nil {
		return content.InteractionDenied, err
	}
	if !r.CanInteract() {
		return content.InteractionDenied, nil
	}
	a := s.audience()
	visible, err := a.remoteVisible(ctx, accountID, st)
	if err != nil || !visible {
		return content.InteractionDenied, err
	}
	rel, err := s.d.Relationships.Relationship(ctx, accountID, st.AuthorAccountID)
	if err != nil {
		return content.InteractionDenied, err
	}
	if rel.Blocking || rel.BlockedBy {
		return content.InteractionDenied, nil
	}
	mentioned, err := s.d.Interactions.Mentioned(ctx, st.ID, accountID)
	if err != nil {
		return content.InteractionDenied, err
	}
	in := interactor{follows: rel.ActivelyFollowing(), followedBy: rel.ActivelyFollowedBy(), mentioned: mentioned}
	return s.interactionReaders().mayInteract(ctx, kind, accountID, false, &in, st)
}

// AnswerInteractionRequest records the answer of accountID, the author of a local status, to the
// pending request requestID of an interaction with it (ADR-0155 §6), and returns the new state.
// Approving it gives this server's authorization and shows the interaction: a held reply is
// released into its thread (StatusReleased), a boost that came without a request or of a local
// account is recorded, and a quote is approved. The answer to a remote account's interaction
// goes out as an Accept or Reject (InteractionAnswered.Deferred). A request of a status of
// another account fails with ErrNotAuthor, a missing one or one of a deleted status with
// ErrNotFound, and one that is not pending with ErrRequestAnswered.
func (s *Interactions) AnswerInteractionRequest(ctx context.Context, accountID, requestID string, accept bool) (content.InteractionState, error) {
	if s.d.Quotes == nil || !kernel.ValidID(requestID) {
		return content.InteractionState{}, fmt.Errorf("interaction request %s: %w", requestID, ErrNotFound)
	}
	r, err := s.d.Quotes.FindInteractionRequest(ctx, nil, requestID)
	if err != nil {
		return content.InteractionState{}, err
	}
	if r.TargetAccountID != accountID || (r.Direction != content.DirectionInbound && r.Direction != content.DirectionLocal) {
		return content.InteractionState{}, fmt.Errorf("interaction request %s: %w", requestID, ErrNotAuthor)
	}
	target, err := s.localTarget(ctx, r.TargetURI)
	if err != nil {
		return content.InteractionState{}, err
	}
	var out content.InteractionState
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.ensureLive(ctx, tx, target.ID); err != nil {
			return err
		}
		cur, err := s.approvals().state(ctx, tx, r)
		if err != nil {
			return err
		}
		if cur.State != content.InteractionRequestPending {
			return fmt.Errorf("interaction request %s is %s: %w", r.ID, cur.State, ErrRequestAnswered)
		}
		a := answer{kind: content.InteractionEventRejected, deferred: true}
		if accept {
			if a.authorization, err = s.approvals().authorize(ctx, tx, r.Kind, r.InteractingURI, r.InteractingAccountID, target); err != nil {
				return err
			}
			a.kind = content.InteractionEventAccepted
		}
		out, err = s.answerTx(ctx, tx, r, a, target)
		return err
	})
	return out, err
}

// WithdrawRemoteInteraction withdraws the pending requests of the remote interaction
// interactingURI of kind by accountID (ADR-0155 §6): the Undo of a boost, or the Delete of a reply,
// that waits for its approval. It reports whether it withdrew one; a request of another account
// is left as it is. Withdrawing again does nothing.
func (s *Interactions) WithdrawRemoteInteraction(ctx context.Context, kind content.InteractionKind, interactingURI, accountID string) (bool, error) {
	if s.d.Quotes == nil {
		return false, nil
	}
	pending, err := s.d.Quotes.ListPendingInteractionRequestsBy(ctx, nil, kind, interactingURI)
	if err != nil {
		return false, err
	}
	withdrew := false
	for _, r := range pending {
		if r.InteractingAccountID != accountID {
			continue
		}
		if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.approvals().withdraw(ctx, tx, r) }); err != nil {
			return withdrew, err
		}
		withdrew = true
	}
	return withdrew, nil
}

// ReconsiderRequests approves the pending requests of interactions with the local status
// statusID that its rules admit at once now (ADR-0155 §3): after its author widened them. The
// others keep waiting for the author's answer. An approval is answered as the author's would be
// (InteractionAnswered.Deferred).
func (s *Interactions) ReconsiderRequests(ctx context.Context, statusID string) error {
	if s.d.Quotes == nil {
		return nil
	}
	target, err := s.audience().live(ctx, statusID)
	if err != nil || !target.IsLocal {
		return err
	}
	pending, err := s.d.Quotes.ListPendingInteractionRequestsTo(ctx, target.URI)
	if err != nil {
		return err
	}
	for _, r := range pending {
		var d content.InteractionDecision
		switch r.Direction {
		case content.DirectionInbound:
			d, err = s.remoteVerdict(ctx, r.Kind, r.InteractingAccountID, target)
		case content.DirectionLocal:
			d, err = s.interactionReaders().mayInteract(ctx, r.Kind, r.InteractingAccountID, true, nil, target)
		default:
			continue
		}
		if err != nil {
			return err
		}
		if d != content.InteractionAllowed {
			continue
		}
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			cur, err := s.approvals().state(ctx, tx, r)
			if err != nil || cur.State != content.InteractionRequestPending {
				return err
			}
			uri, err := s.approvals().authorize(ctx, tx, r.Kind, r.InteractingURI, r.InteractingAccountID, target)
			if err != nil {
				return err
			}
			_, err = s.answerTx(ctx, tx, r, answer{kind: content.InteractionEventAccepted, authorization: uri, deferred: true}, target)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ReceiveInteractionAnswer records the answer of a remote server to the request requestID of a
// local account's reply or boost of a remote status (ADR-0155 §7): the authorization that the
// federation module verified when accepted, "" when rejected. An approved reply is released into
// its thread and goes out with its Create; an approved boost is recorded, and goes out with its
// Announce. An answer to a request that is not pending changes nothing. A request that is not a
// local account's reply or boost fails with ErrNotFound.
func (s *Interactions) ReceiveInteractionAnswer(ctx context.Context, requestID string, accepted bool, authorizationURI string) (content.InteractionState, bool, error) {
	if s.d.Quotes == nil || !kernel.ValidID(requestID) {
		return content.InteractionState{}, false, fmt.Errorf("interaction request %s: %w", requestID, ErrNotFound)
	}
	r, err := s.d.Quotes.FindInteractionRequest(ctx, nil, requestID)
	if err != nil {
		return content.InteractionState{}, false, err
	}
	if r.Direction != content.DirectionOutbound || r.Kind == content.InteractionQuote {
		return content.InteractionState{}, false, fmt.Errorf("interaction request %s is not a local account's: %w", requestID, ErrNotFound)
	}
	target, err := s.d.Statuses.FindStatusByURI(ctx, r.TargetURI)
	if err != nil {
		return content.InteractionState{}, false, err
	}
	a := answer{kind: content.InteractionEventRejected}
	if accepted {
		a = answer{kind: content.InteractionEventAccepted, authorization: authorizationURI}
	}
	var out content.InteractionState
	changed := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.approvals().state(ctx, tx, r)
		if err != nil || cur.State != content.InteractionRequestPending {
			out = cur
			return err
		}
		out, err = s.answerTx(ctx, tx, r, a, target)
		changed = err == nil
		return err
	})
	return out, changed, err
}

// AcceptRemoteInteraction records that the remote interaction interactingURI of kind with the
// remote status targetURI is approved by the authorization uri, which the federation module
// verified (ADR-0155 §6, a third server's interaction): a held reply is released into its
// thread. It reports whether the state changed; an interaction without a request changes
// nothing.
func (s *Interactions) AcceptRemoteInteraction(ctx context.Context, kind content.InteractionKind, interactingURI, targetURI, uri string) (bool, error) {
	if s.d.Quotes == nil {
		return false, nil
	}
	r, err := s.d.Quotes.FindInteractionRequestFor(ctx, nil, kind, interactingURI, targetURI)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	target, err := s.d.Statuses.FindStatusByURI(ctx, targetURI)
	if err != nil {
		return false, err
	}
	changed := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.approvals().state(ctx, tx, r)
		if err != nil || cur.State != content.InteractionRequestPending {
			return err
		}
		_, err = s.answerTx(ctx, tx, r, answer{kind: content.InteractionEventAccepted, authorization: uri}, target)
		changed = err == nil
		return err
	})
	return changed, err
}

// answerTx records in tx the answer a to the pending request r of an interaction with target,
// and what an approval shows: the release of a held reply, the boost of a request that will see
// no Announce come, and the approval of a quote.
func (s *Interactions) answerTx(ctx context.Context, tx uow.Tx, r content.InteractionRequest, a answer, target content.Status) (content.InteractionState, error) {
	ref := RefOf(target)
	if r.Kind == content.InteractionQuote {
		if quoting, err := s.d.Statuses.FindStatusByURITx(ctx, tx, r.InteractingURI); err == nil {
			if _, err := changeQuote(ctx, tx, s.approvals(), s.d.Statuses, quoting, content.QuoteEventKind(a.kind), a.authorization, a.deferred); err != nil {
				return content.InteractionState{}, err
			}
			return s.approvals().state(ctx, tx, r)
		} else if !errors.Is(err, ErrNotFound) {
			return content.InteractionState{}, err
		}
	}
	st, changed, err := s.approvals().answer(ctx, tx, r, a, &ref)
	if err != nil || !changed || st.State != content.InteractionRequestAccepted {
		return st, err
	}
	switch r.Kind {
	case content.InteractionReply:
		return st, s.release(ctx, tx, r.InteractingURI)
	case content.InteractionAnnounce:
		return st, s.recordApprovedBoost(ctx, tx, r, target)
	}
	return st, nil
}

// release emits in tx the StatusReleased of the stored reply uri, now approved; a reply that is
// not stored (its Create comes later) or is deleted releases nothing.
func (s *Interactions) release(ctx context.Context, tx uow.Tx, uri string) error {
	st, err := s.d.Statuses.FindStatusByURITx(ctx, tx, uri)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	deleted, err := s.d.Statuses.IsDeleted(ctx, tx, st.ID)
	if err != nil || deleted {
		return err
	}
	head, err := s.d.Statuses.Head(ctx, tx, st.ID)
	if err != nil {
		return err
	}
	rev, err := s.d.Statuses.FindRevisionTx(ctx, tx, head.RevisionID)
	if err != nil {
		return err
	}
	return tx.Emit(ctx, StatusReleased{Status: st, Revision: rev})
}

// recordApprovedBoost records in tx the boost that the approved request r asked for, when no
// Announce will bring it: a local account's boost, and a remote boost that came without a
// request. A remote account's AnnounceRequest is followed by its Announce, which names the
// authorization.
func (s *Interactions) recordApprovedBoost(ctx context.Context, tx uow.Tx, r content.InteractionRequest, target content.Status) error {
	local := r.Direction == content.DirectionLocal || r.Direction == content.DirectionOutbound
	if !local && r.ActivityURI != "" {
		return nil
	}
	id := s.d.IDs.NewID()
	if local {
		id = r.InteractingURI[strings.LastIndexByte(r.InteractingURI, '/')+1:]
	}
	vis := r.Visibility
	if !vis.Valid() {
		vis = target.Visibility
	}
	b := content.Boost{
		ID: id, URI: r.InteractingURI, BoosterAccountID: r.InteractingAccountID, StatusID: target.ID,
		Visibility: vis, IsLocal: local, BoostedAt: s.d.Clock.Now(),
	}
	_, err := s.insertBoost(ctx, tx, b, target)
	if errors.Is(err, ErrBoostExists) {
		return nil
	}
	return err
}

// insertBoost records the boost b of st in tx, unless the booster boosts st already, and returns
// the live boost.
func (s *Interactions) insertBoost(ctx context.Context, tx uow.Tx, b content.Boost, st content.Status) (content.Boost, error) {
	repo := s.d.Interactions
	if err := s.ensureLive(ctx, tx, st.ID); err != nil {
		return content.Boost{}, err
	}
	edge, err := repo.BoostEdge(ctx, tx, b.BoosterAccountID, st.ID)
	switch {
	case err == nil:
		return repo.Boost(ctx, tx, edge.BoostID)
	case !errors.Is(err, ErrNotFound):
		return content.Boost{}, err
	}
	if err := repo.InsertBoost(ctx, tx, b); err != nil {
		return content.Boost{}, err
	}
	return b, tx.Emit(ctx, Boosted{Boost: b, Status: RefOf(st)})
}

// InteractionRequestPage is a page of the pending requests of an author.
type InteractionRequestPage struct {
	Requests []content.InteractionRequest
	// StatusIDs are the IDs of the stored statuses among the targets and interactions of
	// Requests, by URI.
	StatusIDs map[string]string
	// Next is the cursor of the next page, "" on the last page.
	Next string
}

// statusIDsOf returns the IDs of the stored statuses among the targets and interactions of rs, by
// URI, with one query.
func (s *Interactions) statusIDsOf(ctx context.Context, rs []content.InteractionRequest) (map[string]string, error) {
	out := map[string]string{}
	var uris []string
	for _, r := range rs {
		uris = append(uris, r.TargetURI)
		if r.Kind != content.InteractionAnnounce {
			uris = append(uris, r.InteractingURI)
		}
	}
	if len(uris) == 0 {
		return out, nil
	}
	sts, err := s.d.Statuses.FindStatusesByURIs(ctx, uris)
	if err != nil {
		return nil, err
	}
	for uri, st := range sts {
		out[uri] = st.ID
	}
	return out, nil
}

// PendingInteractionRequests returns a page of up to limit requests of kinds (every kind when it
// is empty) that wait for the answer of accountID, newest first, below the cursor before ("" for
// the newest), with the IDs of the statuses they name: two queries.
func (s *Interactions) PendingInteractionRequests(ctx context.Context, accountID string, kinds []content.InteractionKind, before string, limit int) (InteractionRequestPage, error) {
	if s.d.Quotes == nil {
		return InteractionRequestPage{}, nil
	}
	if before != "" && !kernel.ValidID(before) {
		before = ""
	}
	rs, err := s.d.Quotes.ListPendingInteractionRequests(ctx, accountID, kinds, before, limit+1)
	if err != nil {
		return InteractionRequestPage{}, err
	}
	out := InteractionRequestPage{Requests: rs}
	if len(rs) > limit {
		out.Requests = rs[:limit]
		out.Next = rs[limit-1].ID
	}
	out.StatusIDs, err = s.statusIDsOf(ctx, out.Requests)
	return out, err
}

// FindInteractionRequest returns the request id when accountID is the author of its target, as a
// page of one, for presenting it; any other fails with ErrNotFound.
func (s *Interactions) FindInteractionRequest(ctx context.Context, accountID, id string) (InteractionRequestPage, content.InteractionState, error) {
	r, st, err := s.findInteractionRequest(ctx, accountID, id)
	if err != nil {
		return InteractionRequestPage{}, content.InteractionState{}, err
	}
	ids, err := s.statusIDsOf(ctx, []content.InteractionRequest{r})
	return InteractionRequestPage{Requests: []content.InteractionRequest{r}, StatusIDs: ids}, st, err
}

func (s *Interactions) findInteractionRequest(ctx context.Context, accountID, id string) (content.InteractionRequest, content.InteractionState, error) {
	if s.d.Quotes == nil || !kernel.ValidID(id) {
		return content.InteractionRequest{}, content.InteractionState{}, fmt.Errorf("interaction request %s: %w", id, ErrNotFound)
	}
	r, err := s.d.Quotes.FindInteractionRequest(ctx, nil, id)
	if err != nil {
		return content.InteractionRequest{}, content.InteractionState{}, err
	}
	if r.TargetAccountID != accountID {
		return content.InteractionRequest{}, content.InteractionState{}, fmt.Errorf("interaction request %s: %w", id, ErrNotFound)
	}
	st, err := s.d.Quotes.FindInteractionState(ctx, nil, id)
	if errors.Is(err, ErrNotFound) {
		st = content.InitialInteractionState(r)
	} else if err != nil {
		return content.InteractionRequest{}, content.InteractionState{}, err
	}
	return r, st, nil
}

// FindRequest returns the request id with its state, for the federation module that answers it;
// one that does not exist fails with ErrNotFound.
func (s *Interactions) FindRequest(ctx context.Context, id string) (content.InteractionRequest, content.InteractionState, error) {
	if s.d.Quotes == nil || !kernel.ValidID(id) {
		return content.InteractionRequest{}, content.InteractionState{}, fmt.Errorf("interaction request %s: %w", id, ErrNotFound)
	}
	r, err := s.d.Quotes.FindInteractionRequest(ctx, nil, id)
	if err != nil {
		return content.InteractionRequest{}, content.InteractionState{}, err
	}
	st, err := s.d.Quotes.FindInteractionState(ctx, nil, id)
	if errors.Is(err, ErrNotFound) {
		return r, content.InitialInteractionState(r), nil
	}
	return r, st, err
}

// FindOutboundRequestOf returns the pending request that asks a remote author to approve the
// local reply or boost interactingURI (ADR-0155 §7), and whether there is one: for an answer that
// names the interaction instead of the request.
func (s *Interactions) FindOutboundRequestOf(ctx context.Context, interactingURI string) (content.InteractionRequest, bool, error) {
	if s.d.Quotes == nil {
		return content.InteractionRequest{}, false, nil
	}
	for _, kind := range []content.InteractionKind{content.InteractionReply, content.InteractionAnnounce} {
		pending, err := s.d.Quotes.ListPendingInteractionRequestsBy(ctx, nil, kind, interactingURI)
		if err != nil {
			return content.InteractionRequest{}, false, err
		}
		for _, r := range pending {
			if r.Direction == content.DirectionOutbound {
				return r, true, nil
			}
		}
	}
	return content.InteractionRequest{}, false, nil
}

// FindInteractionAuthorization returns an approval of kind that this server gave, with its
// revocation, for its ActivityPub object. One that does not exist fails with ErrNotFound.
func (s *Interactions) FindInteractionAuthorization(ctx context.Context, kind content.InteractionKind, id string) (content.InteractionAuthorization, error) {
	if s.d.Quotes == nil || !kernel.ValidID(id) {
		return content.InteractionAuthorization{}, fmt.Errorf("%s authorization %s: %w", kind, id, ErrNotFound)
	}
	a, err := s.d.Quotes.FindInteractionAuthorization(ctx, id)
	if err != nil {
		return content.InteractionAuthorization{}, err
	}
	if a.Kind != kind {
		return content.InteractionAuthorization{}, fmt.Errorf("%s authorization %s: %w", kind, id, ErrNotFound)
	}
	return a, nil
}

// VerifyLocalAuthorization reports whether the authorization id of kind of this server, not
// revoked, approves the interaction interactingURI by interactorID with the local status
// targetStatusID: how a remote interaction that names it is approved without deciding it again
// (ADR-0155 §6). An authorization approves only the interaction of the account it was given to.
func (s *Interactions) VerifyLocalAuthorization(ctx context.Context, kind content.InteractionKind, id, interactingURI, interactorID, targetStatusID string) (bool, error) {
	a, err := s.FindInteractionAuthorization(ctx, kind, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return a.RevokedAt == nil && a.InteractingURI == interactingURI && a.InteractingAccountID == interactorID &&
		a.TargetStatusID == targetStatusID, nil
}

// ListOwnInteractions returns the states of the requests of accountID's replies among uris and of
// its boosts of the statuses among uris, with one query (ADR-0155 §10).
func (s *Interactions) ListOwnInteractions(ctx context.Context, accountID string, uris []string) ([]OwnInteraction, error) {
	if s.d.Quotes == nil || accountID == "" || len(uris) == 0 {
		return nil, nil
	}
	return s.d.Quotes.ListOwnInteractions(ctx, accountID, uris)
}

// approvals returns the writer of the records of approvals.
func (s *Interactions) approvals() approvals {
	return approvals{repo: s.d.Quotes, ids: s.d.IDs, clock: s.d.Clock, host: s.d.Host}
}
