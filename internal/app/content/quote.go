// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the quotes of statuses (FR-114, ADR-0069): the status that a status quotes
// (status_quotes), the changes of its approval (status_quote_events), and the current approval
// (the status_quote_states projection).

// The quotes as part of this module's public API, for the federation module, which does not
// import this module's domain.
type (
	// Quote is content.Quote.
	Quote = content.Quote
	// QuoteForm is content.QuoteForm.
	QuoteForm = content.QuoteForm
	// QuoteState is content.QuoteState.
	QuoteState = content.QuoteState
	// QuoteApproval is content.QuoteApproval.
	QuoteApproval = content.QuoteApproval
)

// The values of the quotes' types that other modules use.
const (
	QuoteFormFEP044f      = content.QuoteFormFEP044f
	QuoteFormLegacy       = content.QuoteFormLegacy
	QuoteApprovalAccepted = content.QuoteApprovalAccepted
	QuoteApprovalDetached = content.QuoteApprovalDetached
	QuoteEventAccepted    = content.QuoteEventAccepted
)

// QuoteRepository stores the quotes of statuses, and the interaction policies that decide who may
// quote them; the status repositories implement it.
type QuoteRepository interface {
	InteractionPolicyRepository
	ApprovalRepository

	// InsertQuote records the status that a status quotes. It reports false, writing nothing,
	// when the status has a quote.
	InsertQuote(ctx context.Context, tx uow.Tx, q content.Quote) (bool, error)
	// FindQuote returns the quote of a status, ErrNotFound when it quotes nothing.
	FindQuote(ctx context.Context, statusID string) (content.Quote, error)
	// FindQuotes and FindQuoteStates return the quotes and their states of the statuses of
	// statusIDs that have them, by status ID, as FindQuote and FindQuoteState return those of
	// one, with one query each.
	FindQuotes(ctx context.Context, statusIDs []string) (map[string]content.Quote, error)
	FindQuoteStates(ctx context.Context, statusIDs []string) (map[string]content.QuoteState, error)
	// FindQuoteTx reads as FindQuote does, in tx.
	FindQuoteTx(ctx context.Context, tx uow.Tx, statusID string) (content.Quote, error)
	// ListQuotesAfter returns up to limit quotes whose status ID is above after ("" for the
	// first page), ordered by status ID.
	ListQuotesAfter(ctx context.Context, tx uow.Tx, after string, limit int) ([]content.Quote, error)
	// ListQuotesOf returns up to limit quotes of the status uri whose status ID is below before
	// ("" for the newest), newest first.
	ListQuotesOf(ctx context.Context, uri, before string, limit int) ([]content.Quote, error)

	// FindQuoteState returns the state of the quote of a status, ErrNotFound when it quotes
	// nothing; FindQuoteStateTx reads it in tx. The state is derived from the quote's form, its
	// detachment, and the state of its request (content.QuoteStateFrom).
	FindQuoteState(ctx context.Context, statusID string) (content.QuoteState, error)
	FindQuoteStateTx(ctx context.Context, tx uow.Tx, statusID string) (content.QuoteState, error)
	// ListQuoteStatesByAuthorization returns the states of the quotes that the QuoteAuthorization
	// uri approves, ordered by status ID.
	ListQuoteStatesByAuthorization(ctx context.Context, uri string) ([]content.QuoteState, error)
}

// Event types of the quotes (data-model §4.4):
//   - EventQuoteRecorded: the quote of a status was recorded with the status, and an FEP-044f
//     quote with its request (InteractionRequested). The notification module notifies the local
//     author of the quoted status of a legacy quote, and the federation module sends the
//     QuoteRequest of a local status's quote of a remote one.
//   - EventQuoteChanged: the approval of a quote changed: an answer to its request
//     (InteractionAnswered), or its detachment. The notification module notifies the local
//     author of the quoted status of an approval.
const (
	EventQuoteRecorded = "content.quote_recorded"
	EventQuoteChanged  = "content.quote_changed"
)

// QuoteRecorded is emitted when the quote of a status is inserted.
type QuoteRecorded struct {
	Quote  content.Quote
	Status StatusRef
	// Quoted is the quoted status, nil while it is not stored.
	Quoted *StatusRef
	// RecordedAt is when the quote was recorded.
	RecordedAt time.Time
}

// EventType implements uow.Event.
func (QuoteRecorded) EventType() string { return EventQuoteRecorded }

// QuoteChanged is emitted when an event of the quote of a status is inserted.
type QuoteChanged struct {
	Quote  content.Quote
	Event  content.QuoteEvent
	Status StatusRef
	// Quoted is the quoted status, nil while it is not stored.
	Quoted *StatusRef
}

// EventType implements uow.Event.
func (QuoteChanged) EventType() string { return EventQuoteChanged }

// RemoteQuoteInput is the quote of a remote post (ADR-0069 §2).
type RemoteQuoteInput struct {
	// URI is the URI of the quoted post.
	URI  string
	Form content.QuoteForm
}

// recordQuote records in tx that st quotes in, when the quotes are stored.
func (s *Service) recordQuote(ctx context.Context, tx uow.Tx, st content.Status, in *RemoteQuoteInput) error {
	if in == nil || s.d.Quotes == nil {
		return nil
	}
	if in.URI == "" || !in.Form.Valid() {
		return fmt.Errorf("%w: quote %+v", ErrInvalidRemoteStatus, *in)
	}
	q := content.Quote{StatusID: st.ID, QuotedURI: in.URI, Form: in.Form}
	inserted, err := s.d.Quotes.InsertQuote(ctx, tx, q)
	if err != nil || !inserted {
		return err
	}
	quoted, err := quotedRef(ctx, tx, s.d.Repository, q)
	if err != nil {
		return err
	}
	if q.Form == content.QuoteFormFEP044f && st.IsLocal && (quoted == nil || !quoted.IsLocal) {
		// A local quote of a remote status asks its server (a QuoteRequest).
		if _, err := quoteRequest(ctx, tx, s.approvals(), st, q, quoted, answer{}); err != nil {
			return err
		}
	}
	return tx.Emit(ctx, QuoteRecorded{Quote: q, Status: RefOf(st), Quoted: quoted, RecordedAt: s.d.Clock.Now()})
}

// quoteRequest records in tx the request of the FEP-044f quote q of st with the answer a, unless
// it has one; it answers the request it has with a, unless a is no answer. It returns the
// request's state.
func quoteRequest(ctx context.Context, tx uow.Tx, p approvals, st content.Status, q content.Quote, quoted *StatusRef, a answer) (content.InteractionState, error) {
	switch req, err := p.repo.FindInteractionRequestFor(ctx, tx, content.InteractionQuote, st.URI, q.QuotedURI); {
	case err == nil:
		if a.kind == "" {
			return p.state(ctx, tx, req)
		}
		state, _, err := p.answer(ctx, tx, req, a, quoted)
		return state, err
	case !errors.Is(err, ErrNotFound):
		return content.InteractionState{}, err
	}
	r := content.InteractionRequest{
		Kind: content.InteractionQuote, TargetURI: q.QuotedURI, InteractingURI: st.URI, InteractingAccountID: st.AuthorAccountID,
		Direction: content.DirectionOf(false, st.IsLocal),
	}
	if quoted != nil {
		r.TargetAccountID, r.Direction = quoted.AuthorAccountID, content.DirectionOf(quoted.IsLocal, st.IsLocal)
	}
	_, state, _, err := p.request(ctx, tx, r, a, quoted)
	return state, err
}

// approvals returns the writer of the records of approvals.
func (s *Service) approvals() approvals {
	return approvals{repo: s.d.Quotes, ids: s.d.IDs, clock: s.d.Clock, host: s.d.Host}
}

// changeQuote records in tx an event of kind (with the QuoteAuthorization authorizationURI) of
// the quote of st, unless it changes nothing: the state stays the same. It reports whether the
// state changed. A status that quotes nothing fails with ErrNotFound.
func (s *Service) changeQuote(ctx context.Context, tx uow.Tx, st content.Status, kind content.QuoteEventKind, authorizationURI string) (bool, error) {
	return changeQuote(ctx, tx, s.approvals(), s.d.Repository, st, kind, authorizationURI, false)
}

// changeQuote is Service.changeQuote with p, reading the quoted status from statuses. The
// detachment of a quote is recorded as such (status_quote_detachments); its other events are
// the answers to its request (interaction_request_events), deferred as InteractionAnswered says.
func changeQuote(ctx context.Context, tx uow.Tx, p approvals, statuses Repository, st content.Status, kind content.QuoteEventKind, authorizationURI string, deferred bool) (bool, error) {
	q, err := p.repo.FindQuoteTx(ctx, tx, st.ID)
	if err != nil {
		return false, err
	}
	cur, err := p.repo.FindQuoteStateTx(ctx, tx, st.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		cur = content.InitialQuoteState(q)
	case err != nil:
		return false, err
	}
	e := content.QuoteEvent{
		ID: p.ids.NewID(), StatusID: st.ID, Kind: kind, AuthorizationURI: authorizationURI, OccurredAt: p.clock.Now(),
	}
	if cur.Apply(q, e) == cur {
		return false, nil
	}
	quoted, err := quotedRef(ctx, tx, statuses, q)
	if err != nil {
		return false, err
	}
	if kind == content.QuoteEventDetached {
		if err := p.repo.InsertQuoteDetachment(ctx, tx, st.ID, e.OccurredAt); err != nil {
			return false, err
		}
	} else {
		a := answer{kind: content.InteractionEventKind(kind), authorization: authorizationURI, deferred: deferred}
		if _, err := quoteRequest(ctx, tx, p, st, q, quoted, a); err != nil {
			return false, err
		}
	}
	return true, tx.Emit(ctx, QuoteChanged{Quote: q, Event: e, Status: RefOf(st), Quoted: quoted})
}

// quotedRef returns the status that q quotes when it is stored and not deleted, nil otherwise.
func quotedRef(ctx context.Context, tx uow.Tx, statuses Repository, q content.Quote) (*StatusRef, error) {
	st, err := statuses.FindLiveStatusByURI(ctx, tx, q.QuotedURI)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ref := RefOf(st)
	return &ref, nil
}

// detachQuote records in tx that a revision of st has no quote: the quote is taken away for
// good (ADR-0069 §3). A status that quotes nothing is left as it is.
func (s *Service) detachQuote(ctx context.Context, tx uow.Tx, st content.Status) error {
	if s.d.Quotes == nil {
		return nil
	}
	_, err := s.changeQuote(ctx, tx, st, content.QuoteEventDetached, "")
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// FindQuote returns the quote of a status with its current state. A status that quotes
// nothing, or a Service without quotes, fails with ErrNotFound.
func (s *Service) FindQuote(ctx context.Context, statusID string) (content.Quote, content.QuoteState, error) {
	if s.d.Quotes == nil {
		return content.Quote{}, content.QuoteState{}, fmt.Errorf("quote of %s: %w", statusID, ErrNotFound)
	}
	q, err := s.d.Quotes.FindQuote(ctx, statusID)
	if err != nil {
		return content.Quote{}, content.QuoteState{}, err
	}
	st, err := s.d.Quotes.FindQuoteState(ctx, statusID)
	switch {
	case errors.Is(err, ErrNotFound):
		st = content.InitialQuoteState(q)
	case err != nil:
		return content.Quote{}, content.QuoteState{}, err
	}
	return q, st, nil
}

// AcceptQuote records that the quote of the live status statusID is approved
// (ADR-0069 §2): by the verified QuoteAuthorization authorizationURI, or with "" for a quote of
// the author's own status. It reports whether the state changed: an approval that is recorded
// already, of a quote taken away, or of a legacy quote changes nothing. A status that is
// missing, deleted, or quotes nothing fails with ErrNotFound.
func (s *Service) AcceptQuote(ctx context.Context, statusID, authorizationURI string) (bool, error) {
	return s.quoteCommand(ctx, statusID, content.QuoteEventAccepted, authorizationURI)
}

func (s *Service) quoteCommand(ctx context.Context, statusID string, kind content.QuoteEventKind, authorizationURI string) (bool, error) {
	if s.d.Quotes == nil {
		return false, fmt.Errorf("quote of %s: %w", statusID, ErrNotFound)
	}
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if err != nil {
		return false, err
	}
	var changed bool
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		deleted, err := s.d.Repository.IsDeleted(ctx, tx, st.ID)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("status %s: %w", st.ID, ErrNotFound)
		}
		changed, err = s.changeQuote(ctx, tx, st, kind, authorizationURI)
		return err
	})
	return changed, err
}

// RevokeQuoteAuthorization records that the QuoteAuthorization uri, deleted by the account
// issuerAccountID, is deleted (ADR-0069 §2): the quotes it approved of statuses of the issuer are
// revoked. An authorization only its issuer, the author of the quoted status, takes back; the
// quotes of statuses of other accounts stay. It returns the statuses whose quotes it revoked.
func (s *Service) RevokeQuoteAuthorization(ctx context.Context, uri, issuerAccountID string) ([]string, error) {
	if s.d.Quotes == nil || uri == "" || issuerAccountID == "" {
		return nil, nil
	}
	states, err := s.d.Quotes.ListQuoteStatesByAuthorization(ctx, uri)
	if err != nil {
		return nil, err
	}
	var revoked []string
	for _, qs := range states {
		switch issued, err := s.quotesStatusOf(ctx, qs.StatusID, issuerAccountID); {
		case err != nil:
			return revoked, err
		case !issued:
			continue
		}
		changed, err := s.quoteCommand(ctx, qs.StatusID, content.QuoteEventRevoked, uri)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return revoked, err
		case changed:
			revoked = append(revoked, qs.StatusID)
		}
	}
	return revoked, nil
}

// quotesStatusOf reports whether the status statusID quotes a status of accountID. A status
// that quotes nothing, or whose quoted status is not stored, does not.
func (s *Service) quotesStatusOf(ctx context.Context, statusID, accountID string) (bool, error) {
	q, err := s.d.Quotes.FindQuote(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	quoted, err := s.d.Repository.FindStatusByURI(ctx, q.QuotedURI)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return quoted.AuthorAccountID == accountID, nil
}
