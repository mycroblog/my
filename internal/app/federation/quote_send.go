// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The approvals of quotes between this server and remote ones (FEP-044f, ADR-0069 §2, §3): the
// QuoteRequest of a local status's quote of a remote one and the answer to it, the answer to a
// remote QuoteRequest of a quote of a local status, and the revocation of an approval this
// server gave.

// quoteRequestSegment is the last path segment of the id of the QuoteRequest of a local status's
// quote: {status IRI}/quote_request. The answer names it, which tells the status without a
// lookup.
const quoteRequestSegment = "quote_request"

// QuoteApprovals is the part of the content module's public API that gives the approvals of
// quotes of local statuses; *appcontent.Interactions implements it.
type QuoteApprovals interface {
	AnswerQuoteRequest(ctx context.Context, in appcontent.QuoteRequestInput) (appcontent.QuoteAuthorization, appcontent.Status, error)
	VerifyLocalQuoteAuthorization(ctx context.Context, id, quotingURI, quotingAccountID, quotedStatusID string) (bool, error)
}

// QuoteStatuses is what the approvals of quotes read of statuses; *appcontent.Service implements
// it.
type QuoteStatuses interface {
	FindStatus(ctx context.Context, id string) (appcontent.Status, error)
	FindStatusByURI(ctx context.Context, uri string) (appcontent.Status, error)
	FindStatusByURITx(ctx context.Context, tx uow.Tx, uri string) (appcontent.Status, error)
}

// QuoteHandlersDeps are the ports of QuoteHandlers.
type QuoteHandlersDeps struct {
	UnitOfWork uow.UnitOfWork
	// Recorder records the activities of the approvals; its Repository stores their documents.
	Recorder  RecorderDeps
	Approvals QuoteApprovals
	Quotes    RemoteQuotes
	// Interactions asks and answers the approvals of replies and boosts (ADR-0155 §6, §7); nil
	// takes no ReplyRequest or AnnounceRequest and sends none.
	Interactions InteractionApprovals
	Statuses     QuoteStatuses
	Accounts     Accounts
	// Fetcher fetches the QuoteAuthorizations that remote servers answer with.
	Fetcher DocumentFetcher
	Clock   kernel.Clock
	Logger  *slog.Logger
}

// QuoteHandlers send and receive the approvals of quotes.
type QuoteHandlers struct {
	d      QuoteHandlersDeps
	rec    recorder
	origin string
}

// NewQuoteHandlers returns the QuoteHandlers on d.
func NewQuoteHandlers(d QuoteHandlersDeps) *QuoteHandlers {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &QuoteHandlers{d: d, rec: recorder{d: d.Recorder}, origin: federation.IRI("https://" + d.Recorder.Host).Origin()}
}

func (h *QuoteHandlers) isLocal(iri federation.IRI) bool { return iri.Origin() == h.origin }

// Register sets the handlers of QuoteRequest, and of ReplyRequest and AnnounceRequest with
// Interactions, on p. The answers to this server's requests arrive as Accept and Reject, which
// the handlers of follows pass on (ReceiveQuoteAnswer).
func (h *QuoteHandlers) Register(p *InboxProcessor) {
	p.Handle(federation.ActivityQuoteRequest, h.quoteRequest)
	if h.d.Interactions != nil {
		p.Handle(federation.ActivityReplyRequest, h.interactionRequest(appcontent.InteractionReply))
		p.Handle(federation.ActivityAnnounceRequest, h.interactionRequest(appcontent.InteractionAnnounce))
	}
}

// RegisterProjectors records the activities that the changes of quotes lead to (ADR-0069 §3):
//   - content.quote_recorded of a local status's FEP-044f quote of a remote one: the QuoteRequest
//     to the quoted author's inbox.
//   - content.quote_changed that approves a local status's quote by a remote
//     QuoteAuthorization: the Update of the status, whose Note names it.
//   - content.interaction_policy_changed of the rules of any kind: the Update of the status, whose
//     interactionPolicy says them (ADR-0155 §9).
//   - content.quote_authorization_revoked of a remote quote: the Delete of the
//     QuoteAuthorization to the quoting author's inbox.
//   - content.interaction_requested of a local account's pending reply or boost of a remote
//     status: the ReplyRequest or AnnounceRequest to the status author's inbox (ADR-0155 §7).
//   - content.interaction_answered of a remote account's interaction with a local status that
//     no handler answered (Deferred): the Accept or Reject to its inbox (ADR-0155 §6); of the
//     withdrawal of a local account's pending reply or boost of a remote status: the Delete of the
//     reply or the Undo of the Announce to the inbox that its request went to.
func (h *QuoteHandlers) RegisterProjectors(r *uow.Projectors) {
	r.Register(appcontent.EventInteractionRequested, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.InteractionRequested)
		if !ok || ev.Request.Direction != appcontent.DirectionOutbound || ev.State.State != appcontent.InteractionRequestPending {
			return nil
		}
		switch ev.Request.Kind {
		case appcontent.InteractionReply:
			return h.recordReplyRequest(ctx, tx, ev.Request)
		case appcontent.InteractionAnnounce:
			return h.recordAnnounceRequest(ctx, tx, ev.Request, ev.Target)
		}
		return nil // a QuoteRequest goes out with its quote (content.quote_recorded)
	})
	r.Register(appcontent.EventInteractionAnswered, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.InteractionAnswered)
		switch {
		case !ok:
			return nil
		case ev.Event.Kind == appcontent.InteractionEventWithdrawn:
			if ev.Request.Direction != appcontent.DirectionOutbound {
				return nil
			}
			return h.recordWithdrawal(ctx, tx, ev.Request)
		case !ev.Deferred || ev.Request.Direction != appcontent.DirectionInbound:
			return nil
		}
		return h.recordDeferredAnswer(ctx, tx, ev)
	})
	r.Register(appcontent.EventQuoteRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.QuoteRecorded)
		if !ok || !ev.Status.IsLocal || ev.Quote.Form != appcontent.QuoteFormFEP044f || h.isLocal(federation.IRI(ev.Quote.QuotedURI)) {
			return nil
		}
		return h.recordQuoteRequest(ctx, tx, ev.Status, ev.Quote)
	})
	r.Register(appcontent.EventQuoteChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.QuoteChanged)
		if !ok || !ev.Status.IsLocal || ev.Event.Kind != appcontent.QuoteEventAccepted || ev.Event.AuthorizationURI == "" ||
			h.isLocal(federation.IRI(ev.Event.AuthorizationURI)) {
			return nil
		}
		return h.recordUpdate(ctx, tx, ev.Status)
	})
	r.Register(appcontent.EventInteractionPolicyChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.InteractionPolicyChanged)
		// The Note carries the rules of every kind (canReply, canAnnounce, canQuote).
		if !ok || !ev.Status.IsLocal || len(ev.Kinds) == 0 {
			return nil
		}
		return h.recordUpdate(ctx, tx, ev.Status)
	})
	r.Register(appcontent.EventQuoteAuthorizationRevoked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.QuoteAuthorizationRevoked)
		if !ok {
			return nil
		}
		return h.recordRevocation(ctx, tx, ev)
	})
}

// recordUpdate records the Update of the current revision of the local status st, whose Note
// presents its quote and policy as they are then.
func (h *QuoteHandlers) recordUpdate(ctx context.Context, tx uow.Tx, st appcontent.StatusRef) error {
	if st.Visibility == appcontent.VisibilityLocal {
		return nil // never federated (ADR-0017)
	}
	return h.rec.record(ctx, tx, federation.OutboundActivity{
		Type: federation.ActivityUpdate, ActorAccountID: st.AuthorAccountID,
		ObjectKind: federation.OutboundStatus, ObjectID: st.ID, CreatedAt: h.d.Clock.Now(),
	}, "")
}

// recordQuoteRequest records the QuoteRequest of the local status st's quote q of a remote
// status, to its author's inbox: object is the quoted status, instrument the quoting one.
func (h *QuoteHandlers) recordQuoteRequest(ctx context.Context, tx uow.Tx, st appcontent.StatusRef, q appcontent.Quote) error {
	quoted, err := h.d.Statuses.FindStatusByURITx(ctx, tx, q.QuotedURI)
	if err != nil {
		return err
	}
	inbox, author, ok, err := h.remoteInbox(ctx, tx, quoted.AuthorAccountID)
	if err != nil || !ok {
		return err
	}
	actor := federation.IRI(appaccount.ActorURI(h.d.Recorder.Host, st.AuthorAccountID))
	act, err := h.rec.insert(ctx, tx, federation.OutboundActivity{
		ActivityURI: federation.IRI(st.URI + "/" + quoteRequestSegment), Type: federation.ActivityQuoteRequest,
		ActorAccountID: st.AuthorAccountID, ObjectKind: federation.OutboundQuote, ObjectID: st.ID, CreatedAt: h.d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	doc := federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, federation.Audience{To: []federation.IRI{author}}, federation.IRI(quoted.URI))
	doc.Instrument = federation.IRI(st.URI)
	return h.store(ctx, tx, act, doc, inbox)
}

// recordRevocation records the Delete of a revoked QuoteAuthorization to the quoting author's
// inbox and the quoted author's followers (ADR-0069 §3); a local quoting author gets none.
func (h *QuoteHandlers) recordRevocation(ctx context.Context, tx uow.Tx, ev appcontent.QuoteAuthorizationRevoked) error {
	inbox, quoter, ok, err := h.remoteInbox(ctx, tx, ev.Authorization.InteractingAccountID)
	if err != nil || !ok {
		return err
	}
	actor := federation.IRI(appaccount.ActorURI(h.d.Recorder.Host, ev.Quoted.AuthorAccountID))
	act, err := h.rec.insert(ctx, tx, federation.OutboundActivity{
		Type: federation.ActivityDelete, ActorAccountID: ev.Quoted.AuthorAccountID,
		ObjectKind: federation.OutboundQuote, ObjectID: ev.Authorization.ID, CreatedAt: h.d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	uri := federation.IRI(appcontent.QuoteAuthorizationURI(h.d.Recorder.Host, ev.Authorization.ID))
	doc := federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt,
		federation.Audience{To: []federation.IRI{quoter}, Cc: []federation.IRI{federation.FollowersOf(actor)}}, uri)
	return h.store(ctx, tx, act, doc, inbox)
}

// remoteInbox returns the inbox of the remote account accountID, its own inbox before its
// shared one, with its actor IRI; ok is false for a local account or one never fetched. It reads
// inside tx, or outside any transaction when tx is nil.
func (h *QuoteHandlers) remoteInbox(ctx context.Context, tx uow.Tx, accountID string) (federation.IRI, federation.IRI, bool, error) {
	findActor, findInbox := h.d.Accounts.FindByID, h.d.Accounts.RemoteInbox
	if tx != nil {
		findActor = func(ctx context.Context, id string) (appaccount.Actor, error) {
			return h.d.Accounts.FindByIDTx(ctx, tx, id)
		}
		findInbox = func(ctx context.Context, id string) (appaccount.RemoteInbox, error) {
			return h.d.Accounts.RemoteInboxTx(ctx, tx, id)
		}
	}
	a, err := findActor(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return "", "", false, nil
	}
	if err != nil || a.Account.IsLocal() {
		return "", "", false, err
	}
	inbox, err := findInbox(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	target := federation.IRI(inbox.InboxURL)
	if target == "" {
		target = federation.IRI(inbox.SharedInboxURL)
	}
	return target, federation.IRI(a.Account.URI), target != "", nil
}

// quoteSubject takes the one inbox of an activity of the approval of a quote from the payload:
// its document is stored already. The Delete of a revoked QuoteAuthorization goes to the quoted
// author's followers too, judged as a public post of the author's: approvals are given only to
// quotes of public and unlisted statuses (content.MayQuote).
func (f *FanOut) quoteSubject(ctx context.Context, s subject, p fanOutPayload) (subject, error) {
	switch s.act.Type {
	case federation.ActivityQuoteRequest, federation.ActivityAnnounceRequest, federation.ActivityAccept, federation.ActivityReject,
		federation.ActivityDelete, federation.ActivityUndo:
	default:
		return subject{}, fmt.Errorf("%w: %s of a quote", federation.ErrInvalidOutbound, s.act.Type)
	}
	// The batches after the first (the followers of a Delete) carry the cursor, not the inbox.
	if p.Inbox == "" && (p.Phase == "" || p.Phase == string(phaseRelay)) {
		return subject{}, fmt.Errorf("%w: %s of quote %s without an inbox", federation.ErrInvalidOutbound, s.act.Type, s.act.ObjectID)
	}
	s.inbox, s.followersToo = federation.IRI(p.Inbox), p.Followers
	if s.followersToo || s.act.ObjectKind == federation.OutboundQuote && s.act.Type == federation.ActivityDelete {
		var err error
		if s.restrictions, err = f.d.Statuses.AuthorRestrictions(ctx, s.act.ActorAccountID); err != nil {
			return subject{}, err
		}
		s.facts = appcontent.StatusFacts{AuthorAccountID: s.act.ActorAccountID, AuthorIsLocal: true, Visibility: appcontent.VisibilityPublic}
		s.visibility = appcontent.EffectiveVisibility(s.facts.Visibility, s.restrictions)
	}
	return s, nil
}

// store stores the document of act in tx and enqueues its fan-out to inbox alone.
func (h *QuoteHandlers) store(ctx context.Context, tx uow.Tx, act federation.OutboundActivity, doc federation.ActivityDocument, inbox federation.IRI) error {
	return h.storeTo(ctx, tx, act, doc, fanOutPayload{Inbox: string(inbox)})
}

// storeTo stores the document of act in tx and enqueues its fan-out with the payload p.
func (h *QuoteHandlers) storeTo(ctx context.Context, tx uow.Tx, act federation.OutboundActivity, doc federation.ActivityDocument, p fanOutPayload) error {
	b, err := doc.Marshal()
	if err != nil {
		return err
	}
	if _, err := h.d.Recorder.Repository.InsertOutboundDocument(ctx, tx, act.ID, b, act.CreatedAt); err != nil {
		return err
	}
	return h.rec.enqueue(ctx, tx, act, p)
}

// answer decides a remote QuoteRequest of the local status quoted by the post quoting: the
// content module's decision, or, when the federation mode does not take the quoting post in
// (ingest, outbound in ADR-0006), a refusal without asking it, since the quote would never be
// shown here. A status that is not a local one fails with appcontent.ErrNotFound.
func (h *QuoteHandlers) answer(ctx context.Context, in *InboxActivity, quoted, quoting federation.IRI) (appcontent.QuoteAuthorization, appcontent.Status, error) {
	if federation.Allows(in.mode(), federation.FlowIngest) {
		return h.d.Approvals.AnswerQuoteRequest(ctx, appcontent.QuoteRequestInput{
			QuotedURI: string(quoted), QuotingURI: string(quoting), QuoterAccountID: in.Actor.Account.ID, ActivityURI: string(in.Activity.ID),
		})
	}
	st, err := h.d.Statuses.FindStatusByURI(ctx, string(quoted))
	switch {
	case err != nil:
		return appcontent.QuoteAuthorization{}, appcontent.Status{}, err
	case !st.IsLocal:
		return appcontent.QuoteAuthorization{}, appcontent.Status{}, fmt.Errorf("status %s: %w", st.ID, appcontent.ErrNotFound)
	}
	return appcontent.QuoteAuthorization{}, st, fmt.Errorf("%w: %w: mode %s", appcontent.ErrQuoteRefused, ErrFederationRestricted, in.mode())
}

// quotedObject returns the status that a QuoteRequest's object names. Bonfire at times names the
// Create of a status in this server's document instead of the status: embedded, whose object's
// IRI alone is taken, or by its IRI, the status's with /activity (createURI, ADR-0018), which
// is read without fetching. The caller checks that the result is a local status.
func (h *QuoteHandlers) quotedObject(ref federation.Ref) federation.IRI {
	if d := ref.Untrusted(); d != nil && d.Activity != nil && d.Activity.Type == federation.ActivityCreate {
		return d.Activity.Object().ID
	}
	if status, ok := strings.CutSuffix(string(ref.ID), "/activity"); ok && h.isLocal(ref.ID) {
		return federation.IRI(status)
	}
	return ref.ID
}

// quoteRequest answers a remote QuoteRequest of a quote of a local status (ADR-0069 §2): the
// content module decides it, and the Accept with this server's QuoteAuthorization as result, or
// the Reject, goes to the actor's inbox. The quoting post (instrument) must be of the actor's
// origin, and the actor's own when it is stored already (the content module refuses it
// otherwise); a stored quoting post's quote is approved here too.
func (h *QuoteHandlers) quoteRequest(ctx context.Context, in *InboxActivity) error {
	act := in.Activity
	quoted, quoting := h.quotedObject(act.Object()), act.Instrument.ID
	switch {
	case quoted == "" || !h.isLocal(quoted):
		return fmt.Errorf("%w: QuoteRequest of %q, which is not a local status", ErrDropActivity, quoted)
	case quoting == "" || !federation.SameOrigin(quoting, act.Actor):
		return fmt.Errorf("%w: QuoteRequest of %q by %s", ErrDropActivity, quoting, act.Actor)
	}
	auth, st, err := h.answer(ctx, in, quoted, quoting)
	accepted := err == nil
	switch {
	case errors.Is(err, appcontent.ErrQuoteAwaitsApproval):
		return nil // the author answers it
	case errors.Is(err, appcontent.ErrQuoteRefused):
	case errors.Is(err, appcontent.ErrNotFound):
		return fmt.Errorf("%w: QuoteRequest of %s: %w", ErrDropActivity, quoted, err)
	case err != nil:
		return err
	}
	inbox, requester, ok, err := h.remoteInbox(ctx, nil, in.Actor.Account.ID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: no inbox for %s", ErrDropActivity, act.Actor)
	}
	request := federation.NewActivityDocument(act.ID, federation.ActivityQuoteRequest, act.Actor, time.Time{}, federation.Audience{}, quoted).Embedded()
	request.Instrument = quoting
	typ, result := federation.ActivityReject, federation.IRI("")
	if accepted {
		typ, result = federation.ActivityAccept, federation.IRI(appcontent.QuoteAuthorizationURI(h.d.Recorder.Host, auth.ID))
	}
	err = h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		first, err := h.rec.firstAnswer(ctx, tx, in.Received.ActivityURI, h.d.Clock.Now())
		if err != nil || !first {
			return err
		}
		out, err := h.rec.insert(ctx, tx, federation.OutboundActivity{
			Type: typ, ActorAccountID: st.AuthorAccountID, ObjectKind: federation.OutboundQuote, ObjectID: st.ID, CreatedAt: h.d.Clock.Now(),
		})
		if err != nil {
			return err
		}
		actor := federation.IRI(appaccount.ActorURI(h.d.Recorder.Host, st.AuthorAccountID))
		doc := federation.NewActivityDocument(out.ActivityURI, typ, actor, out.CreatedAt, federation.Audience{To: []federation.IRI{requester}}, request)
		doc.Result = result
		return h.store(ctx, tx, out, doc, inbox)
	})
	if err != nil || !accepted {
		return err
	}
	stored, err := h.d.Statuses.FindStatusByURI(ctx, string(quoting))
	if errors.Is(err, appcontent.ErrNotFound) {
		return nil // its Create names the QuoteAuthorization (JobVerifyQuote)
	}
	if err != nil {
		return err
	}
	if stored.AuthorAccountID != in.Actor.Account.ID {
		return nil // the approval is the requester's, and approves no quote of another account
	}
	_, err = h.d.Quotes.AcceptQuote(ctx, stored.ID, string(result))
	if errors.Is(err, appcontent.ErrNotFound) {
		return nil
	}
	return err
}

// ReceiveQuoteAnswer takes an Accept or Reject whose object is this server's QuoteRequest
// (ADR-0069 §3), ReplyRequest, or AnnounceRequest (receiveInteractionAnswer), and reports whether
// it was one. The answer must come from the quoted author.
// An Accept must name a QuoteAuthorization of the quoted author's host that approves the quote
// (checkQuoteAuthorization); the quote is then approved, and its approval goes out in an Update
// of the status. A Reject refuses the quote.
func (h *QuoteHandlers) ReceiveQuoteAnswer(ctx context.Context, in *InboxActivity, accepted bool) (bool, error) {
	if handled, err := h.receiveInteractionAnswer(ctx, in, accepted); handled {
		return true, err
	}
	ref := in.Activity.Object()
	statusID, ok := h.quoteRequestStatus(ref.ID)
	if !ok {
		return false, nil
	}
	st, err := h.d.Statuses.FindStatus(ctx, statusID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return true, fmt.Errorf("%w: answer to the QuoteRequest of a missing status %s", ErrDropActivity, statusID)
	}
	if err != nil {
		return true, err
	}
	q, _, err := h.d.Quotes.FindQuote(ctx, st.ID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return true, fmt.Errorf("%w: %s quotes nothing", ErrDropActivity, st.URI)
	}
	if err != nil {
		return true, err
	}
	quoted, err := h.d.Statuses.FindStatusByURI(ctx, q.QuotedURI)
	if err != nil {
		return true, errors.Join(ErrDropActivity, err)
	}
	if quoted.AuthorAccountID != in.Actor.Account.ID {
		return true, fmt.Errorf("%w: %s answered the QuoteRequest of %s, which quotes another's status", ErrDropActivity, in.Activity.Actor, st.URI)
	}
	if !accepted {
		_, err := h.d.Quotes.RejectQuote(ctx, st.ID)
		return true, quoteError(err)
	}
	result := in.Activity.Result
	if err := checkQuoteAuthorization(ctx, h.d.Fetcher, result, federation.IRI(in.Actor.Account.URI), federation.IRI(st.URI), federation.IRI(quoted.URI)); err != nil {
		if isFetchError(err) || errors.Is(err, ErrQuoteUnauthorized) {
			return true, fmt.Errorf("%w: %w", ErrDropActivity, err)
		}
		return true, err
	}
	_, err = h.d.Quotes.AcceptQuote(ctx, st.ID, string(result))
	return true, quoteError(err)
}

// quoteRequestStatus returns the local status whose QuoteRequest id is.
func (h *QuoteHandlers) quoteRequestStatus(id federation.IRI) (string, bool) {
	prefix := h.origin + "/ap/objects/"
	rest, ok := strings.CutPrefix(string(id), prefix)
	if !ok {
		return "", false
	}
	statusID, ok := strings.CutSuffix(rest, "/"+quoteRequestSegment)
	if !ok || !kernel.ValidID(statusID) {
		return "", false
	}
	return statusID, true
}

// quoteError drops an answer about a status that is deleted meanwhile.
func quoteError(err error) error {
	if errors.Is(err, appcontent.ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return err
}
