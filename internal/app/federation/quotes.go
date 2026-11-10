// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// JobVerifyQuote fetches the post that a remote post quotes and verifies the approval of the
// quote (ADR-0069 §2).
const JobVerifyQuote = "federation.verify_quote"

// ErrQuoteUnauthorized reports a QuoteAuthorization that does not approve the quote that names
// it.
var ErrQuoteUnauthorized = errors.New("federation: the QuoteAuthorization does not approve the quote")

// RemoteQuotes is the part of the content module's public API that records the approvals of the
// quotes of remote posts; *appcontent.Service implements it.
type RemoteQuotes interface {
	FindQuote(ctx context.Context, statusID string) (appcontent.Quote, appcontent.QuoteState, error)
	AcceptQuote(ctx context.Context, statusID, authorizationURI string) (bool, error)
	RejectQuote(ctx context.Context, statusID string) (bool, error)
	RevokeQuoteAuthorization(ctx context.Context, uri, issuerAccountID string) ([]string, error)
}

// verifyQuotePayload is the payload of JobVerifyQuote.
type verifyQuotePayload struct {
	// URI is the quoting post.
	URI string `json:"uri"`
	// Authorization is the quoteAuthorization that the post names, "" for none.
	Authorization string `json:"authorization,omitempty"`
	// Fetch fetches the quoted post when it is not stored: only for a post that an inbox
	// received, so that a fetched post does not fetch what it quotes in turn (ADR-0069 §2).
	Fetch bool `json:"fetch,omitempty"`
}

// quoteInput maps the quote of a normalized post to what the content module stores.
func quoteInput(q *federation.Quote) *appcontent.RemoteQuoteInput {
	if q == nil {
		return nil
	}
	form := appcontent.QuoteFormLegacy
	if q.Consent {
		form = appcontent.QuoteFormFEP044f
	}
	return &appcontent.RemoteQuoteInput{URI: string(q.Target), Form: form}
}

// interactionPolicyInput maps the interactionPolicy of a post by the author actor, whose
// collections are cols, to the rules the content module stores (ADR-0155 §2): as:Public is
// everyone, the author's followers and following collections are followers and following, the
// author is author, and another IRI is the account of that actor, which an IRI of any other
// collection matches no account of. A kind whose automaticApproval keeps nothing is the author's alone, and a kind the post
// does not declare has no rules.
func interactionPolicyInput(obj *federation.Object, actor federation.IRI, cols authorCollections) appcontent.InteractionPolicy {
	var out appcontent.InteractionPolicy
	for _, c := range []struct {
		kind appcontent.InteractionKind
		can  *federation.CanInteract
	}{{appcontent.InteractionReply, obj.CanReply}, {appcontent.InteractionAnnounce, obj.CanAnnounce}, {appcontent.InteractionQuote, obj.CanQuote}} {
		if c.can == nil {
			continue
		}
		auto := approvalRules(c.kind, appcontent.ApprovalAutomatic, c.can.AutomaticApproval, actor, cols)
		if len(auto) == 0 {
			auto = appcontent.InteractionPolicy{{Kind: c.kind, Approval: appcontent.ApprovalAutomatic, Audience: appcontent.AudienceAuthor}}
		}
		out = append(out, auto...)
		out = append(out, approvalRules(c.kind, appcontent.ApprovalManual, c.can.ManualApproval, actor, cols)...)
	}
	return out
}

// approvalRules maps the IRIs of one approval of a kind to rules, as interactionPolicyInput does.
func approvalRules(kind appcontent.InteractionKind, approval appcontent.InteractionApproval, iris []federation.IRI, actor federation.IRI, cols authorCollections) appcontent.InteractionPolicy {
	var out appcontent.InteractionPolicy
	for _, iri := range iris {
		r := appcontent.InteractionRule{Kind: kind, Approval: approval}
		switch {
		case iri == federation.PublicIRI:
			r.Audience = appcontent.AudienceEveryone
		case cols.followers != "" && iri == cols.followers:
			r.Audience = appcontent.AudienceFollowers
		case cols.following != "" && iri == cols.following:
			r.Audience = appcontent.AudienceFollowing
		case iri == actor:
			r.Audience = appcontent.AudienceAuthor
		default:
			// Another actor, or a collection that is not known, which no account's IRI matches.
			r.Audience, r.ActorURI = appcontent.AudienceAccount, string(iri)
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// verifyQuoteTx enqueues in tx the JobVerifyQuote of the post uri, which quotes q; a post without
// a quote enqueues nothing.
func (h *StatusHandlers) verifyQuoteTx(ctx context.Context, tx uow.Tx, uri federation.IRI, q *federation.Quote, fetch bool) error {
	if q == nil || h.d.Outbox == nil || h.d.Quotes == nil {
		return nil
	}
	payload, err := json.Marshal(verifyQuotePayload{URI: string(uri), Authorization: string(q.Authorization), Fetch: fetch})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	return h.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobVerifyQuote, Payload: payload, IdempotencyKey: "verify_quote:" + hex.EncodeToString(sum[:]),
	})
}

// handleVerifyQuote is the queue.Handler of JobVerifyQuote. A document that cannot be fetched or
// does not approve the quote ends the job: the quote stays pending until an Update names another
// QuoteAuthorization.
func (h *StatusHandlers) handleVerifyQuote(ctx context.Context, j queue.Job) queue.Result {
	var p verifyQuotePayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("federation: verify_quote payload %s: %w", j.Payload, err))
	}
	if p.URI == "" {
		return queue.Fail(fmt.Errorf("federation: verify_quote payload %s has no uri", j.Payload))
	}
	err := h.verifyQuote(ctx, p)
	switch {
	case err == nil:
		return queue.Done()
	case isFetchError(err), errors.Is(err, ErrNotTaken), errors.Is(err, ErrQuoteUnauthorized):
		h.d.Logger.InfoContext(ctx, "quote not verified", slog.String("uri", p.URI), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// verifyQuote does the work of one JobVerifyQuote (ADR-0069 §2): it fetches the quoted post when
// p.Fetch says so, and approves an FEP-044f quote of the author's own post, or one whose
// QuoteAuthorization verifies.
func (h *StatusHandlers) verifyQuote(ctx context.Context, p verifyQuotePayload) error {
	st, err := h.d.Statuses.FindStatusByURI(ctx, p.URI)
	if errors.Is(err, appcontent.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	q, state, err := h.d.Quotes.FindQuote(ctx, st.ID)
	if errors.Is(err, appcontent.ErrNotFound) || err == nil && state.Approval == appcontent.QuoteApprovalDetached {
		return nil
	}
	if err != nil {
		return err
	}
	quoted, err := h.d.Statuses.FindStatusByURI(ctx, q.QuotedURI)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		if !p.Fetch || h.isLocal(federation.IRI(q.QuotedURI)) {
			return nil
		}
		if quoted, _, err = h.fetchStatus(ctx, federation.IRI(q.QuotedURI)); err != nil {
			return err
		}
	case err != nil:
		return err
	}
	if q.Form != appcontent.QuoteFormFEP044f {
		return nil // a legacy quote needs no approval
	}
	if state.Approval == appcontent.QuoteApprovalAccepted && state.AuthorizationURI == p.Authorization {
		return nil
	}
	if quoted.AuthorAccountID == st.AuthorAccountID {
		_, err := h.d.Quotes.AcceptQuote(ctx, st.ID, "")
		return err
	}
	if p.Authorization == "" {
		return nil
	}
	if quoted.IsLocal {
		// The approval of a quote of a local status is one this server gave.
		if h.d.Approvals == nil {
			return nil
		}
		ok, err := h.d.Approvals.VerifyLocalQuoteAuthorization(ctx, lastSegment(p.Authorization), st.URI, st.AuthorAccountID, quoted.ID)
		if err != nil || !ok || !h.isLocal(federation.IRI(p.Authorization)) {
			return errors.Join(err, errNotApproved(ok, p.Authorization))
		}
		_, err = h.d.Quotes.AcceptQuote(ctx, st.ID, p.Authorization)
		return err
	}
	if h.d.Profiles == nil {
		return nil
	}
	author, err := h.d.Profiles.FindByID(ctx, quoted.AuthorAccountID)
	if err != nil {
		return err
	}
	if err := checkQuoteAuthorization(ctx, h.d.Fetcher, federation.IRI(p.Authorization), federation.IRI(author.Account.URI),
		federation.IRI(st.URI), federation.IRI(quoted.URI)); err != nil {
		return err
	}
	_, err = h.d.Quotes.AcceptQuote(ctx, st.ID, p.Authorization)
	return err
}

// errNotApproved returns ErrQuoteUnauthorized for a QuoteAuthorization of this server that does
// not approve a quote, nil for one that does.
func errNotApproved(ok bool, uri string) error {
	if ok {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrQuoteUnauthorized, uri)
}

// lastSegment returns the last path segment of uri: the ID of an object IRI of this server.
func lastSegment(uri string) string { return uri[strings.LastIndexByte(uri, '/')+1:] }

// checkQuoteAuthorization fetches the QuoteAuthorization uri and checks that the quoted author
// issued it for the quote of quoted by quoting (FEP-044f): it is attributed to author, on the
// author's host, and names the two posts.
func checkQuoteAuthorization(ctx context.Context, fetcher DocumentFetcher, uri, author, quoting, quotedURI federation.IRI) error {
	if uri.Host() == "" || uri.Host() != author.Host() {
		return fmt.Errorf("%w: %s is not on the host of %s", ErrQuoteUnauthorized, uri, author)
	}
	got, err := fetcher.Fetch(ctx, string(uri))
	if err != nil {
		return err
	}
	obj := got.Document.Object
	switch {
	case obj == nil || obj.Type != federation.ObjectQuoteAuthorization || obj.ID != uri:
		return fmt.Errorf("%w: %s is not a QuoteAuthorization", ErrQuoteUnauthorized, uri)
	case obj.AttributedTo != author:
		return fmt.Errorf("%w: %s is attributed to %q, not to %s", ErrQuoteUnauthorized, uri, obj.AttributedTo, author)
	case obj.InteractingObject != quoting || obj.InteractionTarget != quotedURI:
		return fmt.Errorf("%w: %s approves %q quoting %q", ErrQuoteUnauthorized, uri, obj.InteractingObject, obj.InteractionTarget)
	}
	return nil
}

// revokeQuoteAuthorization takes the Delete of id by issuer, which is no stored post, as the
// revocation of a QuoteAuthorization (ADR-0069 §2): only of the quotes of the issuer's statuses.
// It reports whether id approved such a quote.
func (h *StatusHandlers) revokeQuoteAuthorization(ctx context.Context, id federation.IRI, issuer appaccount.Actor) (bool, error) {
	if h.d.Quotes == nil {
		return false, nil
	}
	revoked, err := h.d.Quotes.RevokeQuoteAuthorization(ctx, string(id), issuer.Account.ID)
	return len(revoked) > 0, err
}
