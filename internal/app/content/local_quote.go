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
)

// QuoteAuthorization is content.QuoteAuthorization, as part of this module's public API.
type QuoteAuthorization = content.QuoteAuthorization

// QuoteAuthorizationURI returns the ActivityPub id of a QuoteAuthorization of host.
func QuoteAuthorizationURI(host, authorizationID string) string {
	return content.QuoteAuthorizationURI(host, authorizationID)
}

// Event of the approvals this server gives (data-model §4.4):
//   - EventQuoteAuthorizationRevoked: the author of a local status revoked an approval of a quote
//     of it. The federation module delivers the Delete of the QuoteAuthorization to the quoting
//     author when that author is remote.
const EventQuoteAuthorizationRevoked = "content.quote_authorization_revoked"

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
	quoted, rel, err := s.visibleTo(ctx, authorID, statusID)
	if errors.Is(err, ErrReplyTargetNotFound) {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteTargetNotFound)
	}
	if err != nil {
		return localQuote{}, err
	}
	policy, err := interactionPolicyOf(ctx, s.d.Quotes, quoted.ID)
	if err != nil {
		return localQuote{}, err
	}
	facts := content.QuoteFacts{
		QuoterAccountID: authorID, QuoterIsLocal: true, QuoterVisible: true, QuoterFollows: rel.follows,
		QuoterFollowed: rel.followedBy, QuoterMentioned: rel.mentioned, QuotingVisibility: vis, Quoted: quoted, Policy: policy,
	}
	if namesActors(policy, content.InteractionQuote) {
		if facts.QuoterURI, err = s.actorURI(ctx, authorID); err != nil {
			return localQuote{}, err
		}
	}
	allowance := content.MayQuote(facts)
	if allowance == content.QuoteDenied {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteNotAllowed)
	}
	if !quoted.IsLocal && !content.QuotesRemotePosts(vis) {
		return localQuote{}, fmt.Errorf("status %s: %w", statusID, ErrQuoteRemoteAudience)
	}
	return localQuote{quoted: quoted, allowance: allowance}, nil
}

// recordLocalQuote records in tx that the local status st quotes q (ADR-0069 §3): an approved
// quote of a local status of another author gets this server's QuoteAuthorization at once, a
// quote of the author's own status needs none, a quote that a manual rule of a local status
// includes waits for its author (ADR-0155 §6), a quote of a remote status that declares a policy
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

// FederatedQuotePage returns the HTML page of the quoted status uri, which the fallback of a quote
// links (ADR-0069 §3): the page of a stored status, or uri itself for one not stored or without
// a page. It reports false when a federated Note must not name the quoted status at all: a
// local-visibility status, whose IRI and author stay within its audience (ADR-0018 §15), and a status
// of this server that is gone, whose visibility is not known any more.
func (s *Service) FederatedQuotePage(ctx context.Context, uri string) (string, bool, error) {
	st, err := s.d.Repository.FindStatusByURI(ctx, uri)
	if errors.Is(err, ErrNotFound) {
		page, ok := s.federatedQuotePage(uri, nil, appaccount.Actor{})
		return page, ok, nil
	}
	if err != nil {
		return "", false, err
	}
	var author appaccount.Actor
	if st.IsLocal && st.Visibility != content.VisibilityLocal {
		if author, err = s.d.Accounts.FindByID(ctx, st.AuthorAccountID); err != nil && !errors.Is(err, appaccount.ErrNotFound) {
			return "", false, err
		}
	}
	page, ok := s.federatedQuotePage(uri, &st, author)
	return page, ok, nil
}

// federatedQuotePage is FederatedQuotePage of uri from the status stored at it (nil for none) and
// its local author.
func (s *Service) federatedQuotePage(uri string, st *content.Status, author appaccount.Actor) (string, bool) {
	if st == nil {
		return uri, !strings.HasPrefix(uri, "https://"+s.d.Host+"/")
	}
	if st.Visibility == content.VisibilityLocal {
		return "", false
	}
	if page := StatusPageURL(s.d.Host, *st, author); page != "" {
		return page, true
	}
	return uri, true
}

// FederatedQuoteLink is what FederatedQuotePage returns for one quoted status.
type FederatedQuoteLink struct {
	Page string
	OK   bool
}

// NoteQuotes are the quotes of statuses as FindQuote, FindInteractionPolicy, and
// FederatedQuotePage read them one by one: Quotes and States by quoting status, Policies by
// status, and Links by quoted URI. A status missing from a map has none.
type NoteQuotes struct {
	Quotes   map[string]content.Quote
	States   map[string]content.QuoteState
	Policies map[string]content.InteractionPolicy
	Links    map[string]FederatedQuoteLink
}

// NoteQuotesOf reads what the Notes of statusIDs say of quotes, as FindQuote, FindInteractionPolicy,
// and FederatedQuotePage read it for one status, with a number of queries that does not grow with
// statusIDs.
func (s *Service) NoteQuotesOf(ctx context.Context, statusIDs []string) (NoteQuotes, error) {
	out := NoteQuotes{Quotes: map[string]content.Quote{}, States: map[string]content.QuoteState{},
		Policies: map[string]content.InteractionPolicy{}, Links: map[string]FederatedQuoteLink{}}
	if s.d.Quotes == nil || len(statusIDs) == 0 {
		return out, nil
	}
	var err error
	if out.Quotes, err = s.d.Quotes.FindQuotes(ctx, statusIDs); err != nil {
		return NoteQuotes{}, err
	}
	if out.Policies, err = s.d.Quotes.FindInteractionPolicies(ctx, statusIDs); err != nil {
		return NoteQuotes{}, err
	}
	quoting := make([]string, 0, len(out.Quotes))
	var uris []string
	for id, q := range out.Quotes {
		quoting = append(quoting, id)
		if !slices.Contains(uris, q.QuotedURI) {
			uris = append(uris, q.QuotedURI)
		}
	}
	states, err := s.d.Quotes.FindQuoteStates(ctx, quoting)
	if err != nil {
		return NoteQuotes{}, err
	}
	for id, q := range out.Quotes {
		st, ok := states[id]
		if !ok {
			st = content.InitialQuoteState(q)
		}
		out.States[id] = st
	}
	quoted, err := s.d.Repository.FindStatusesByURIs(ctx, uris)
	if err != nil {
		return NoteQuotes{}, err
	}
	var authors []string
	for _, st := range quoted {
		if st.IsLocal && st.Visibility != content.VisibilityLocal && !slices.Contains(authors, st.AuthorAccountID) {
			authors = append(authors, st.AuthorAccountID)
		}
	}
	actors, err := s.d.Accounts.FindByIDs(ctx, authors)
	if err != nil {
		return NoteQuotes{}, err
	}
	for _, uri := range uris {
		var link FederatedQuoteLink
		if st, ok := quoted[uri]; ok {
			link.Page, link.OK = s.federatedQuotePage(uri, &st, actors[st.AuthorAccountID])
		} else {
			link.Page, link.OK = s.federatedQuotePage(uri, nil, appaccount.Actor{})
		}
		out.Links[uri] = link
	}
	return out, nil
}

// RejectQuote records that the quoted author's server refused the quote of the live status
// statusID (a Reject of its QuoteRequest). It reports whether the state changed.
func (s *Service) RejectQuote(ctx context.Context, statusID string) (bool, error) {
	return s.quoteCommand(ctx, statusID, content.QuoteEventRejected, "")
}

// interactor is how an account stands with the author of a status it interacts with: whether it
// actively follows the author, the author actively follows it, and the status mentions it.
type interactor struct {
	follows, followedBy, mentioned bool
}

// visibleTo returns the status that the local account accountID refers to, provided that it may
// see it, and how the account stands with its author. A status that does not exist, is deleted,
// or is hidden from the account fails alike with ErrReplyTargetNotFound.
func (s *Service) visibleTo(ctx context.Context, accountID, statusID string) (content.Status, interactor, error) {
	notFound := fmt.Errorf("status %s: %w", statusID, ErrReplyTargetNotFound)
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, interactor{}, notFound
	}
	if err != nil {
		return content.Status{}, interactor{}, err
	}
	rev, err := s.d.Repository.FindCurrentRevision(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, interactor{}, notFound // deleted
	}
	if err != nil {
		return content.Status{}, interactor{}, err
	}
	restrictions, err := s.d.Restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return content.Status{}, interactor{}, err
	}
	rel := content.Relation{Mentioned: slices.Contains(rev.MentionedAccountIDs, accountID)}
	var followedBy bool
	if st.AuthorAccountID != accountID {
		r, err := s.d.Relationships.Relationship(ctx, accountID, st.AuthorAccountID)
		if err != nil {
			return content.Status{}, interactor{}, err
		}
		if r.ActivelyFollowing() {
			rel.Follow = content.FollowActive
		}
		rel.Blocking, rel.BlockedBy = r.Blocking, r.BlockedBy
		followedBy = r.ActivelyFollowedBy()
	}
	viewer := content.Viewer{Kind: content.ViewerLocal, AccountID: accountID}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	if !content.CanView(content.ContextStatus, viewer, facts, restrictions.Author, rel) {
		return content.Status{}, interactor{}, notFound
	}
	return st, interactor{follows: rel.Follow == content.FollowActive, followedBy: followedBy, mentioned: rel.Mentioned}, nil
}

// namesActors reports whether the rules of kind of p name single actors, whom only the actor
// IRI of an account tells apart.
func namesActors(p content.InteractionPolicy, kind content.InteractionKind) bool {
	return slices.ContainsFunc(p.Of(kind), func(r content.InteractionRule) bool { return r.Audience == content.AudienceAccount })
}

// actorURI returns the actor IRI of an account, "" when it is not known.
func (s *Service) actorURI(ctx context.Context, accountID string) (string, error) {
	a, err := s.d.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return "", nil
	}
	return a.Account.URI, err
}
