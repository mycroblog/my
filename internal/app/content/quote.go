// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
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
	// StatusQuotePolicy is content.StatusQuotePolicy.
	StatusQuotePolicy = content.StatusQuotePolicy
)

// The values of the quotes' types that other modules use.
const (
	QuoteFormFEP044f      = content.QuoteFormFEP044f
	QuoteFormLegacy       = content.QuoteFormLegacy
	QuoteApprovalAccepted = content.QuoteApprovalAccepted
	QuoteApprovalDetached = content.QuoteApprovalDetached
	QuoteEventAccepted    = content.QuoteEventAccepted
)

// QuoteRepository stores the quotes of statuses; the status repositories implement it.
type QuoteRepository interface {
	// InsertQuote records the status that a status quotes. It reports false, writing nothing,
	// when the status has a quote.
	InsertQuote(ctx context.Context, tx uow.Tx, q content.Quote) (bool, error)
	// FindQuote returns the quote of a status, ErrNotFound when it quotes nothing.
	FindQuote(ctx context.Context, statusID string) (content.Quote, error)
	// FindQuoteTx reads as FindQuote does, in tx.
	FindQuoteTx(ctx context.Context, tx uow.Tx, statusID string) (content.Quote, error)
	// ListQuotesAfter returns up to limit quotes whose status ID is above after ("" for the
	// first page), ordered by status ID.
	ListQuotesAfter(ctx context.Context, tx uow.Tx, after string, limit int) ([]content.Quote, error)
	// InsertQuoteEvent records a change of the approval of a quote.
	InsertQuoteEvent(ctx context.Context, tx uow.Tx, e content.QuoteEvent) error
	// ListQuoteEvents returns the events of the quote of a status in the order they occurred.
	ListQuoteEvents(ctx context.Context, tx uow.Tx, statusID string) ([]content.QuoteEvent, error)
	// ListQuotesOf returns up to limit quotes of the status uri whose status ID is below before
	// ("" for the newest), newest first.
	ListQuotesOf(ctx context.Context, uri, before string, limit int) ([]content.Quote, error)

	// UpsertQuotePolicy sets who may quote a status; DeleteQuotePolicy removes the policy of a
	// remote status that declares none any more.
	UpsertQuotePolicy(ctx context.Context, tx uow.Tx, p content.StatusQuotePolicy) error
	DeleteQuotePolicy(ctx context.Context, tx uow.Tx, statusID string) error
	// FindQuotePolicy returns who may quote a status, ErrNotFound when it declares nobody.
	FindQuotePolicy(ctx context.Context, statusID string) (content.StatusQuotePolicy, error)

	// InsertQuoteAuthorization records this server's approval of a quote. It reports false,
	// writing nothing, when the quoting status has one for the quoted status.
	InsertQuoteAuthorization(ctx context.Context, tx uow.Tx, a content.QuoteAuthorization) (bool, error)
	// FindQuoteAuthorization returns an approval with its revocation, ErrNotFound when there is
	// none; FindQuoteAuthorizationFor finds it by the quoting URI and the quoted status.
	FindQuoteAuthorization(ctx context.Context, id string) (content.QuoteAuthorization, error)
	FindQuoteAuthorizationFor(ctx context.Context, quotingURI, quotedStatusID string) (content.QuoteAuthorization, error)
	// InsertQuoteAuthorizationRevocation records the revocation of an approval, and reports
	// false, writing nothing, when it is revoked already.
	InsertQuoteAuthorizationRevocation(ctx context.Context, tx uow.Tx, authorizationID string, at time.Time) (bool, error)

	// Projection, written only by this module's projectors and rebuild.

	UpsertQuoteState(ctx context.Context, tx uow.Tx, s content.QuoteState) error
	// FindQuoteState returns the state of the quote of a status, ErrNotFound when it has none.
	FindQuoteState(ctx context.Context, statusID string) (content.QuoteState, error)
	// FindQuoteStateTx reads as FindQuoteState does, in tx.
	FindQuoteStateTx(ctx context.Context, tx uow.Tx, statusID string) (content.QuoteState, error)
	// ListQuoteStatesByAuthorization returns the states whose QuoteAuthorization is uri,
	// ordered by status ID.
	ListQuoteStatesByAuthorization(ctx context.Context, uri string) ([]content.QuoteState, error)
	// ListQuoteStates returns every state ordered by status ID, for verifying the projection.
	ListQuoteStates(ctx context.Context) ([]content.QuoteState, error)
	DeleteAllQuoteStates(ctx context.Context, tx uow.Tx) error
}

// Event types of the quotes (data-model §4.4):
//   - EventQuoteRecorded: the quote of a status was recorded with the status. This module
//     projects its initial state; the notification module notifies the local author of the
//     quoted status of a legacy quote, and the federation module sends the QuoteRequest of a
//     local status's quote of a remote one.
//   - EventQuoteChanged: the approval of a quote changed. This module projects the new state;
//     the notification module notifies the local author of the quoted status of an approval.
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

// RegisterQuoteProjectors registers the synchronous projector of status_quote_states: the state
// of each quote, derived from its form and events by content.QuoteStateOf. RebuildQuoteStates
// applies the same rule.
func RegisterQuoteProjectors(r *uow.Projectors, repo QuoteRepository) {
	r.Register(EventQuoteRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertQuoteState(ctx, tx, content.InitialQuoteState(e.(QuoteRecorded).Quote))
	})
	r.Register(EventQuoteChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(QuoteChanged)
		cur, err := repo.FindQuoteStateTx(ctx, tx, ev.Quote.StatusID)
		switch {
		case errors.Is(err, ErrNotFound):
			cur = content.InitialQuoteState(ev.Quote)
		case err != nil:
			return err
		}
		return repo.UpsertQuoteState(ctx, tx, cur.Apply(ev.Quote, ev.Event))
	})
}

// quoteRebuildPage is how many quotes RebuildQuoteStates reads at a time.
const quoteRebuildPage = 500

// RebuildQuoteStates empties status_quote_states and derives it again from the quotes and their
// events (data-model §9).
func RebuildQuoteStates(ctx context.Context, u uow.UnitOfWork, repo QuoteRepository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllQuoteStates(ctx, tx); err != nil {
			return err
		}
		for after := ""; ; {
			quotes, err := repo.ListQuotesAfter(ctx, tx, after, quoteRebuildPage)
			if err != nil {
				return err
			}
			for _, q := range quotes {
				events, err := repo.ListQuoteEvents(ctx, tx, q.StatusID)
				if err != nil {
					return err
				}
				if err := repo.UpsertQuoteState(ctx, tx, content.QuoteStateOf(q, events)); err != nil {
					return err
				}
			}
			if len(quotes) < quoteRebuildPage {
				return nil
			}
			after = quotes[len(quotes)-1].StatusID
		}
	})
	if err != nil {
		return fmt.Errorf("rebuild status_quote_states: %w", err)
	}
	return nil
}

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
	return tx.Emit(ctx, QuoteRecorded{Quote: q, Status: RefOf(st), Quoted: quoted, RecordedAt: s.d.Clock.Now()})
}

// changeQuote records in tx an event of kind (with the QuoteAuthorization authorizationURI) of
// the quote of st, unless it changes nothing: the state stays the same. It reports whether the
// state changed. A status that quotes nothing fails with ErrNotFound.
func (s *Service) changeQuote(ctx context.Context, tx uow.Tx, st content.Status, kind content.QuoteEventKind, authorizationURI string) (bool, error) {
	return changeQuote(ctx, tx, s.d.Quotes, s.d.Repository, s.d.IDs, s.d.Clock, st, kind, authorizationURI)
}

// changeQuote is Service.changeQuote on repo, reading the quoted status from statuses.
func changeQuote(ctx context.Context, tx uow.Tx, repo QuoteRepository, statuses Repository, ids kernel.IDGenerator, clock kernel.Clock, st content.Status, kind content.QuoteEventKind, authorizationURI string) (bool, error) {
	q, err := repo.FindQuoteTx(ctx, tx, st.ID)
	if err != nil {
		return false, err
	}
	cur, err := repo.FindQuoteStateTx(ctx, tx, st.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		cur = content.InitialQuoteState(q)
	case err != nil:
		return false, err
	}
	e := content.QuoteEvent{
		ID: ids.NewID(), StatusID: st.ID, Kind: kind, AuthorizationURI: authorizationURI, OccurredAt: clock.Now(),
	}
	if cur.Apply(q, e) == cur {
		return false, nil
	}
	if err := repo.InsertQuoteEvent(ctx, tx, e); err != nil {
		return false, err
	}
	quoted, err := quotedRef(ctx, tx, statuses, q)
	if err != nil {
		return false, err
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
