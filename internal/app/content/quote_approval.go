// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The approvals that this server gives to quotes of local statuses (FEP-044f, ADR-0069 §2, §3):
// the answer to a remote QuoteRequest, and the author's revocation of an approval.

var (
	// ErrQuoteRefused reports a quote that the policy of the quoted status refuses.
	ErrQuoteRefused = errors.New("content: the quote is refused")
	// ErrQuoteAwaitsApproval reports a quote that a manual rule of the policy of the quoted
	// status includes: its author answers it (ADR-0155 §6).
	ErrQuoteAwaitsApproval = errors.New("content: the quote awaits the author's approval")
)

// QuoteRequestInput is a remote QuoteRequest of a quote of a local status.
type QuoteRequestInput struct {
	// QuotedURI is the local status quoted, QuotingURI the remote status that quotes it.
	QuotedURI  string
	QuotingURI string
	// QuoterAccountID is the remote account that asks: the actor of the QuoteRequest.
	QuoterAccountID string
	// ActivityURI is the id of the QuoteRequest, which the author's later answer embeds.
	ActivityURI string
}

// AnswerQuoteRequest decides a remote QuoteRequest (ADR-0069 §2, ADR-0155 §6), records it, and
// returns this server's QuoteAuthorization of the quote, with the quoted status: the one given
// before when the quote was approved already. The quote is approved when the quoter is a remote
// account that may see the status as an ActivityPub fetch would (content.CanView in
// ContextObjectFetch), is not under the interaction restriction (FR-708), the policy of the
// status includes them automatically (content.MayInteract), and a quoting status that is stored
// already is theirs; a local-only status refuses every remote quoter.
// A refused quote, and one whose approval was revoked, fail with ErrQuoteRefused; one that a
// manual rule includes, with ErrQuoteAwaitsApproval: it waits for the author's answer
// (AnswerInteractionRequest). A quoted status that is not local, missing, or deleted fails with
// ErrNotFound.
func (s *Interactions) AnswerQuoteRequest(ctx context.Context, in QuoteRequestInput) (content.QuoteAuthorization, content.Status, error) {
	if s.d.Quotes == nil {
		return content.QuoteAuthorization{}, content.Status{}, fmt.Errorf("quotes: %w", ErrNotFound)
	}
	quoted, err := s.localTarget(ctx, in.QuotedURI)
	if err != nil {
		return content.QuoteAuthorization{}, content.Status{}, err
	}
	refused := fmt.Errorf("quote of %s by %s: %w", quoted.ID, in.QuoterAccountID, ErrQuoteRefused)
	if in.QuotingURI == "" {
		return content.QuoteAuthorization{}, quoted, refused
	}
	d, err := s.remoteVerdict(ctx, content.InteractionQuote, in.QuoterAccountID, quoted)
	if err != nil {
		return content.QuoteAuthorization{}, quoted, err
	}
	// Only the author of the quoting status asks for its quote.
	switch quoting, err := s.d.Statuses.FindStatusByURI(ctx, in.QuotingURI); {
	case err == nil && quoting.AuthorAccountID != in.QuoterAccountID:
		d = content.InteractionDenied
	case err != nil && !errors.Is(err, ErrNotFound):
		return content.QuoteAuthorization{}, quoted, err
	}
	r := content.InteractionRequest{
		Kind: content.InteractionQuote, Direction: content.DirectionInbound, TargetURI: quoted.URI, TargetAccountID: quoted.AuthorAccountID,
		InteractingURI: in.QuotingURI, InteractingAccountID: in.QuoterAccountID, ActivityURI: in.ActivityURI,
	}
	var state content.InteractionState
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := s.firstAnswer(ctx, tx, d, r, quoted, false)
		if err != nil {
			return err
		}
		ref := RefOf(quoted)
		_, state, _, err = s.approvals().request(ctx, tx, r, a, &ref)
		return err
	})
	switch {
	case err != nil:
		return content.QuoteAuthorization{}, quoted, err
	case state.State == content.InteractionRequestPending:
		return content.QuoteAuthorization{}, quoted, fmt.Errorf("quote of %s by %s: %w", quoted.ID, in.QuoterAccountID, ErrQuoteAwaitsApproval)
	case state.State != content.InteractionRequestAccepted:
		return content.QuoteAuthorization{}, quoted, refused
	}
	// A repeated request gets the approval given before.
	stored, err := s.d.Quotes.FindInteractionAuthorizationFor(ctx, nil, content.InteractionQuote, in.QuotingURI, quoted.ID)
	if err != nil {
		return content.QuoteAuthorization{}, quoted, err
	}
	if stored.RevokedAt != nil {
		return content.QuoteAuthorization{}, quoted, refused
	}
	return stored, quoted, nil
}

// RevokeQuote revokes the approval of the quote quotingStatusID of the local status
// quotedStatusID of accountID (ADR-0069 §3). The quoting status's quote is revoked, and
// QuoteAuthorizationRevoked tells the federation module to deliver the Delete of the
// QuoteAuthorization. A quote without an approval of this server (a legacy quote, or the quote of
// the author's own status), and one revoked already, fail with ErrNotFound; a status of another
// account with ErrNotAuthor.
func (s *Interactions) RevokeQuote(ctx context.Context, accountID, quotedStatusID, quotingStatusID string) error {
	if s.d.Quotes == nil {
		return fmt.Errorf("quotes: %w", ErrNotFound)
	}
	a := s.audience()
	quoted, err := a.live(ctx, quotedStatusID)
	if err != nil {
		return err
	}
	if !quoted.IsLocal || quoted.AuthorAccountID != accountID {
		return fmt.Errorf("status %s: %w", quotedStatusID, ErrNotAuthor)
	}
	quoting, err := s.d.Statuses.FindStatus(ctx, quotingStatusID)
	if err != nil {
		return err
	}
	auth, err := s.d.Quotes.FindInteractionAuthorizationFor(ctx, nil, content.InteractionQuote, quoting.URI, quoted.ID)
	if err != nil {
		return err
	}
	if auth.RevokedAt != nil {
		return fmt.Errorf("quote authorization %s is revoked: %w", auth.ID, ErrNotFound)
	}
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		revoked, err := s.d.Quotes.InsertInteractionAuthorizationRevocation(ctx, tx, auth.ID, now)
		if err != nil || !revoked {
			return err
		}
		auth.RevokedAt = &now
		uri := content.QuoteAuthorizationURI(s.d.Host, auth.ID)
		if _, err := changeQuote(ctx, tx, s.approvals(), s.d.Statuses, quoting, content.QuoteEventRevoked, uri, false); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		return tx.Emit(ctx, QuoteAuthorizationRevoked{Authorization: auth, Quoted: RefOf(quoted)})
	})
}

// FindQuoteAuthorization returns an approval of a quote that this server gave, with its
// revocation, for its ActivityPub object. One that does not exist fails with ErrNotFound.
func (s *Interactions) FindQuoteAuthorization(ctx context.Context, id string) (content.QuoteAuthorization, error) {
	return s.FindInteractionAuthorization(ctx, content.InteractionQuote, id)
}

// VerifyLocalQuoteAuthorization reports whether the QuoteAuthorization id of this server, not
// revoked, approves the quote quotingURI by the account quotingAccountID of the local status
// quotedStatusID (ADR-0069 §2): how a remote quote of a local status that names it is approved.
// An authorization approves only the quote of the account it was given to.
func (s *Interactions) VerifyLocalQuoteAuthorization(ctx context.Context, id, quotingURI, quotingAccountID, quotedStatusID string) (bool, error) {
	return s.VerifyLocalAuthorization(ctx, content.InteractionQuote, id, quotingURI, quotingAccountID, quotedStatusID)
}
