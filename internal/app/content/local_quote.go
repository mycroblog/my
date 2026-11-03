// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The quotes of local statuses (FR-114, ADR-0069 §3): who may quote a status, and the quote of
// a status that a local author posts.

var (
	// ErrQuoteTargetNotFound reports a quoted status that does not exist, is deleted, or is
	// hidden from the author, alike, so that a quote does not reveal it.
	ErrQuoteTargetNotFound = errors.New("content: the quoted status is not found")
	// ErrQuoteNotAllowed reports a status that the author may not quote (content.MayQuote).
	ErrQuoteNotAllowed = errors.New("content: the status may not be quoted")
	// ErrQuoteRemoteAudience reports a local, private, or direct status that quotes a remote one
	// (content.QuotesRemotePosts).
	ErrQuoteRemoteAudience = errors.New("content: a status of this visibility may not quote a remote status")
	// ErrInvalidQuotePolicy reports a quote policy that is not one of content's.
	ErrInvalidQuotePolicy = errors.New("content: invalid quote policy")
)

// The quote policies as part of this module's public API.
type (
	// QuotePolicy is content.QuotePolicy.
	QuotePolicy = content.QuotePolicy
	// QuoteAuthorization is content.QuoteAuthorization.
	QuoteAuthorization = content.QuoteAuthorization
)

// The values of the quote policies that other modules use.
const (
	QuotePolicyPublic    = content.QuotePolicyPublic
	QuotePolicyFollowers = content.QuotePolicyFollowers
	QuotePolicyNobody    = content.QuotePolicyNobody
)

// QuoteAuthorizationURI returns the ActivityPub id of a QuoteAuthorization of host.
func QuoteAuthorizationURI(host, authorizationID string) string {
	return content.QuoteAuthorizationURI(host, authorizationID)
}

// Events of the quote policies and the approvals this server gives (data-model §4.4):
//   - EventQuotePolicyChanged: the author of a local status changed who may quote it. The
//     federation module delivers an Update of the status, whose interactionPolicy says it.
//   - EventQuoteAuthorizationRevoked: the author of a local status revoked an approval of a quote
//     of it. The federation module delivers the Delete of the QuoteAuthorization to the quoting
//     author when that author is remote.
const (
	EventQuotePolicyChanged        = "content.quote_policy_changed"
	EventQuoteAuthorizationRevoked = "content.quote_authorization_revoked"
)

// QuotePolicyChanged is emitted when the author of a local status changes its quote policy.
type QuotePolicyChanged struct {
	Policy content.StatusQuotePolicy
	Status StatusRef
}

// EventType implements uow.Event.
func (QuotePolicyChanged) EventType() string { return EventQuotePolicyChanged }

// QuoteAuthorizationRevoked is emitted when an approval this server gave is revoked.
type QuoteAuthorizationRevoked struct {
	Authorization content.QuoteAuthorization
	// Quoted is the local status quoted.
	Quoted StatusRef
}

// EventType implements uow.Event.
func (QuoteAuthorizationRevoked) EventType() string { return EventQuoteAuthorizationRevoked }

// localQuote is the quote of a status that a local author is posting.
type localQuote struct {
	quoted    content.Status
	allowance content.QuoteAllowance
}

// quoteTarget returns the status that authorID quotes in a status of visibility vis
// (ADR-0069 §3, §4). A status that does not exist, is deleted, or is hidden from the author fails
// alike with ErrQuoteTargetNotFound; one that the author may not quote with ErrQuoteNotAllowed;
// a remote one that a status of vis may not quote with ErrQuoteRemoteAudience. A refused quote
// records nothing, and the status is not created.
// A quote of a boost is a quote of the boosted status, which the boost's ID does not name.
func (s *Service) quoteTarget(ctx context.Context, authorID, statusID string, vis content.Visibility) (localQuote, error) {
	if s.d.Quotes == nil {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteTargetNotFound)
	}
	quoted, follows, err := s.visibleTo(ctx, authorID, statusID)
	if errors.Is(err, ErrReplyTargetNotFound) {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteTargetNotFound)
	}
	if err != nil {
		return localQuote{}, err
	}
	policy, err := s.quotePolicy(ctx, quoted.ID)
	if err != nil {
		return localQuote{}, err
	}
	allowance := content.MayQuote(content.QuoteFacts{
		QuoterAccountID: authorID, QuoterIsLocal: true, QuoterVisible: true, QuoterFollows: follows,
		QuotingVisibility: vis, Quoted: quoted, Policy: policy,
	})
	if allowance == content.QuoteDenied {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteNotAllowed)
	}
	if !quoted.IsLocal && !content.QuotesRemotePosts(vis) {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteRemoteAudience)
	}
	return localQuote{quoted: quoted, allowance: allowance}, nil
}

// quotePolicy returns who may quote statusID, "" when it declares nobody.
func (s *Service) quotePolicy(ctx context.Context, statusID string) (content.QuotePolicy, error) {
	p, err := s.d.Quotes.FindQuotePolicy(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	return p.Policy, err
}

// recordLocalQuote records in tx that the local status st quotes q (ADR-0069 §3): an approved
// quote of a local status of another author gets this server's QuoteAuthorization at once, a
// quote of the author's own status needs none, a quote of a remote status that declares a policy
// waits for its server's answer to a QuoteRequest, and one of a remote status without one is a
// legacy quote.
func (s *Service) recordLocalQuote(ctx context.Context, tx uow.Tx, st content.Status, q localQuote) error {
	form := content.QuoteFormFEP044f
	if q.allowance == content.QuoteLegacy {
		form = content.QuoteFormLegacy
	}
	if err := s.recordQuote(ctx, tx, st, &RemoteQuoteInput{URI: q.quoted.URI, Form: form}); err != nil {
		return err
	}
	if q.allowance != content.QuoteApproved {
		return nil
	}
	uri := ""
	if q.quoted.AuthorAccountID != st.AuthorAccountID {
		a := content.QuoteAuthorization{
			ID: s.d.IDs.NewID(), QuotingURI: st.URI, QuotingAccountID: st.AuthorAccountID, QuotedStatusID: q.quoted.ID, CreatedAt: s.d.Clock.Now(),
		}
		if _, err := s.d.Quotes.InsertQuoteAuthorization(ctx, tx, a); err != nil {
			return err
		}
		uri = content.QuoteAuthorizationURI(s.d.Host, a.ID)
	}
	_, err := s.changeQuote(ctx, tx, st, content.QuoteEventAccepted, uri)
	return err
}

// newQuotePolicy returns the quote policy of a new status of author: requested, or the author's
// default.
func (s *Service) newQuotePolicy(ctx context.Context, author appaccount.Actor, requested content.QuotePolicy) (content.QuotePolicy, error) {
	if requested != "" {
		if !requested.Valid() {
			return "", fmt.Errorf("%w: %q", ErrInvalidQuotePolicy, requested)
		}
		return requested, nil
	}
	if s.d.DefaultQuotePolicy == nil || author.Local == nil {
		return content.DefaultQuotePolicy, nil
	}
	p, err := s.d.DefaultQuotePolicy(ctx, author.Local.UserID)
	if err != nil || !p.Valid() {
		return content.DefaultQuotePolicy, err
	}
	return p, nil
}

// SetQuotePolicy changes who may quote a local status of accountID (ADR-0069 §1). Approvals
// given before stay. It emits QuotePolicyChanged unless the policy stays the same. A status that
// does not exist or is deleted fails with ErrNotFound, another account's with ErrNotAuthor.
func (s *Service) SetQuotePolicy(ctx context.Context, accountID, statusID string, policy content.QuotePolicy) (content.StatusQuotePolicy, error) {
	if !policy.Valid() {
		return content.StatusQuotePolicy{}, fmt.Errorf("%w: %q", ErrInvalidQuotePolicy, policy)
	}
	if s.d.Quotes == nil {
		return content.StatusQuotePolicy{}, fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	}
	st, err := s.ownStatus(ctx, accountID, statusID)
	if err != nil {
		return content.StatusQuotePolicy{}, err
	}
	p := content.StatusQuotePolicy{StatusID: st.ID, Policy: policy, UpdatedAt: s.d.Clock.Now()}
	if cur, err := s.d.Quotes.FindQuotePolicy(ctx, st.ID); err == nil && cur.Policy == policy {
		return cur, nil
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return content.StatusQuotePolicy{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		deleted, err := s.d.Repository.IsDeleted(ctx, tx, st.ID)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("status %s: %w", st.ID, ErrNotFound)
		}
		if err := s.d.Quotes.UpsertQuotePolicy(ctx, tx, p); err != nil {
			return err
		}
		return tx.Emit(ctx, QuotePolicyChanged{Policy: p, Status: RefOf(st)})
	})
	if err != nil {
		return content.StatusQuotePolicy{}, err
	}
	return p, nil
}

// FindQuotePolicy returns who may quote a status: its author's choice for a local status, what
// a remote status declares. A remote status that declares nobody fails with ErrNotFound.
func (s *Service) FindQuotePolicy(ctx context.Context, statusID string) (content.StatusQuotePolicy, error) {
	if s.d.Quotes == nil {
		return content.StatusQuotePolicy{}, fmt.Errorf("quote policy of %s: %w", statusID, ErrNotFound)
	}
	return s.d.Quotes.FindQuotePolicy(ctx, statusID)
}

// FederatedQuotePage returns the HTML page of the quoted status uri, which the fallback of a quote
// links (ADR-0069 §3): the page of a stored status, or uri itself for one not stored or without
// a page. It reports false when a federated Note must not name the quoted status at all: a
// local-visibility status, whose IRI and author stay within its audience (ADR-0146), and a status
// of this server that is gone, whose visibility is not known any more.
func (s *Service) FederatedQuotePage(ctx context.Context, uri string) (string, bool, error) {
	st, err := s.d.Repository.FindStatusByURI(ctx, uri)
	if errors.Is(err, ErrNotFound) {
		return uri, !strings.HasPrefix(uri, "https://"+s.d.Host+"/"), nil
	}
	if err != nil {
		return "", false, err
	}
	if st.Visibility == content.VisibilityLocal {
		return "", false, nil
	}
	var author appaccount.Actor
	if st.IsLocal {
		if author, err = s.d.Accounts.FindByID(ctx, st.AuthorAccountID); err != nil && !errors.Is(err, appaccount.ErrNotFound) {
			return "", false, err
		}
	}
	if page := StatusPageURL(s.d.Host, st, author); page != "" {
		return page, true, nil
	}
	return uri, true, nil
}

// RejectQuote records that the quoted author's server refused the quote of the live status
// statusID (a Reject of its QuoteRequest). It reports whether the state changed.
func (s *Service) RejectQuote(ctx context.Context, statusID string) (bool, error) {
	return s.quoteCommand(ctx, statusID, content.QuoteEventRejected, "")
}

// RemoteQuotePolicyInput is who may quote a remote post, as its interactionPolicy.canQuote
// declares (ADR-0069 §3); nil declares nobody.
type RemoteQuotePolicyInput struct {
	Policy content.QuotePolicy
}

// recordRemoteQuotePolicy sets in tx who may quote the remote status st: in, or none.
func (s *Service) recordRemoteQuotePolicy(ctx context.Context, tx uow.Tx, st content.Status, in *RemoteQuotePolicyInput) error {
	if s.d.Quotes == nil {
		return nil
	}
	if in == nil || !in.Policy.Valid() {
		return s.d.Quotes.DeleteQuotePolicy(ctx, tx, st.ID)
	}
	return s.d.Quotes.UpsertQuotePolicy(ctx, tx, content.StatusQuotePolicy{StatusID: st.ID, Policy: in.Policy, UpdatedAt: s.d.Clock.Now()})
}

// visibleTo returns the status that the local account accountID refers to, provided that it may
// see it, and whether it actively follows its author. A status that does not exist, is deleted,
// or is hidden from the account fails alike with ErrReplyTargetNotFound.
func (s *Service) visibleTo(ctx context.Context, accountID, statusID string) (content.Status, bool, error) {
	notFound := fmt.Errorf("status %s: %w", statusID, ErrReplyTargetNotFound)
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, false, notFound
	}
	if err != nil {
		return content.Status{}, false, err
	}
	rev, err := s.d.Repository.FindCurrentRevision(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, false, notFound // deleted
	}
	if err != nil {
		return content.Status{}, false, err
	}
	restrictions, err := s.d.Restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return content.Status{}, false, err
	}
	rel := content.Relation{Mentioned: slices.Contains(rev.MentionedAccountIDs, accountID)}
	if st.AuthorAccountID != accountID {
		r, err := s.d.Relationships.Relationship(ctx, accountID, st.AuthorAccountID)
		if err != nil {
			return content.Status{}, false, err
		}
		if r.ActivelyFollowing() {
			rel.Follow = content.FollowActive
		}
		rel.Blocking, rel.BlockedBy = r.Blocking, r.BlockedBy
	}
	viewer := content.Viewer{Kind: content.ViewerLocal, AccountID: accountID}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	if !content.CanView(content.ContextStatus, viewer, facts, restrictions.Author, rel) {
		return content.Status{}, false, notFound
	}
	return st, rel.Follow == content.FollowActive, nil
}
