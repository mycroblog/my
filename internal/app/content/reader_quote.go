// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// DomainMutes tells whether a local account mutes the domain of a remote account (FR-304);
// *appgraph.Service implements it.
type DomainMutes interface {
	MutesDomain(ctx context.Context, accountID, domain string) (bool, error)
}

// QuoteViewState is the state of a quote as a viewer sees it (ADR-0069 §4, §5): the approval of
// the quote, or why the quoted status is not shown to the viewer. The values are those of
// Mastodon's Quote.state, and legacy.
type QuoteViewState string

// Quote view states.
const (
	// QuoteViewPending is a quote that awaits its approval, or whose quoted status is not
	// fetched yet.
	QuoteViewPending  QuoteViewState = "pending"
	QuoteViewAccepted QuoteViewState = "accepted"
	QuoteViewRejected QuoteViewState = "rejected"
	QuoteViewRevoked  QuoteViewState = "revoked"
	// QuoteViewLegacy is a legacy quote, which needs no approval.
	QuoteViewLegacy QuoteViewState = "legacy"
	// QuoteViewDeleted is a quote of a deleted status.
	QuoteViewDeleted QuoteViewState = "deleted"
	// QuoteViewUnauthorized is a quote of a status that the viewer may not see.
	QuoteViewUnauthorized QuoteViewState = "unauthorized"
	// QuoteViewBlockedAccount is a quote of a status of an account that blocks the viewer or
	// that the viewer blocks; the status is not shown.
	QuoteViewBlockedAccount QuoteViewState = "blocked_account"
	// QuoteViewBlockedDomain is a quote of a status of a domain that the viewer mutes; the status
	// is shown behind the reason.
	QuoteViewBlockedDomain QuoteViewState = "blocked_domain"
	// QuoteViewMutedAccount is a quote of a status of an account that the viewer mutes; the
	// status is shown behind the reason.
	QuoteViewMutedAccount QuoteViewState = "muted_account"
)

// QuoteView is the quote of a status as a viewer sees it.
type QuoteView struct {
	State QuoteViewState
	// Status is the quoted status, set for accepted and legacy quotes and behind a mute
	// (QuoteViewMutedAccount, QuoteViewBlockedDomain). It is presented without its own quote:
	// quotes are shown one level deep (ADR-0069 §5).
	Status *StatusView
}

// quote returns the quote of st as viewer sees it, nil when st quotes nothing or its quote was
// taken away (ADR-0069 §4).
func (r *Reader) quote(ctx context.Context, viewer Viewer, st content.Status) (*QuoteView, error) {
	quoted, shown, final, err := r.quotedStatus(ctx, st)
	if err != nil || shown == "" {
		return final, err
	}
	// A legacy quote of a local status is shown only when its policy would approve the quoting
	// author's quote (ADR-0069 §2): no approval tells otherwise.
	if shown == QuoteViewLegacy && quoted.IsLocal && !st.IsLocal {
		ok, err := r.remoteMayQuote(ctx, st.AuthorAccountID, quoted)
		if err != nil {
			return nil, err
		}
		if !ok {
			return &QuoteView{State: QuoteViewRejected}, nil
		}
	}

	var muted, domainMuted bool
	if viewer.Kind == ViewerLocal && viewer.AccountID != "" && viewer.AccountID != quoted.AuthorAccountID {
		rel, err := r.d.Relationships.Relationship(ctx, viewer.AccountID, quoted.AuthorAccountID)
		if err != nil {
			return nil, err
		}
		if rel.Blocking || rel.BlockedBy {
			return &QuoteView{State: QuoteViewBlockedAccount}, nil
		}
		muted = rel.Muting
	}
	ok, author, err := r.aud.visible(ctx, viewer, quoted)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &QuoteView{State: QuoteViewUnauthorized}, nil
	}
	if !muted && r.d.DomainMutes != nil && viewer.Kind == ViewerLocal && viewer.AccountID != "" && !quoted.IsLocal {
		if domainMuted, err = r.d.DomainMutes.MutesDomain(ctx, viewer.AccountID, author.Account.Domain); err != nil {
			return nil, err
		}
	}
	switch {
	case muted:
		shown = QuoteViewMutedAccount
	case domainMuted:
		shown = QuoteViewBlockedDomain
	}
	return r.presentQuoted(ctx, viewer, quoted, author, shown)
}

// quotedStatus finds the status that st quotes, for whoever may see it, with the state to show it
// under. When there is no status to show it returns the empty state and what to answer instead: a
// QuoteView that says why, or nil when st quotes nothing or its quote was taken away.
func (r *Reader) quotedStatus(ctx context.Context, st content.Status) (content.Status, QuoteViewState, *QuoteView, error) {
	none := func(state QuoteViewState) (content.Status, QuoteViewState, *QuoteView, error) {
		return content.Status{}, "", &QuoteView{State: state}, nil
	}
	if r.d.Quotes == nil {
		return content.Status{}, "", nil, nil
	}
	q, err := r.d.Quotes.FindQuote(ctx, st.ID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, "", nil, nil
	}
	if err != nil {
		return content.Status{}, "", nil, err
	}
	state, err := r.d.Quotes.FindQuoteState(ctx, st.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		state = content.InitialQuoteState(q)
	case err != nil:
		return content.Status{}, "", nil, err
	}
	switch state.Approval {
	case content.QuoteApprovalDetached:
		return content.Status{}, "", nil, nil
	case content.QuoteApprovalPending:
		return none(QuoteViewPending)
	case content.QuoteApprovalRejected:
		return none(QuoteViewRejected)
	case content.QuoteApprovalRevoked:
		return none(QuoteViewRevoked)
	}
	shown := QuoteViewAccepted
	if state.Approval == content.QuoteApprovalLegacy {
		shown = QuoteViewLegacy
	}

	quoted, err := r.d.Statuses.FindStatusByURI(ctx, q.QuotedURI)
	if errors.Is(err, ErrNotFound) {
		return none(QuoteViewPending) // not fetched yet
	}
	if err != nil {
		return content.Status{}, "", nil, err
	}
	if _, err := r.aud.live(ctx, quoted.ID); errors.Is(err, ErrNotFound) {
		return none(QuoteViewDeleted)
	} else if err != nil {
		return content.Status{}, "", nil, err
	}
	return quoted, shown, nil, nil
}

// presentQuoted returns the quote that shows quoted under the state shown.
func (r *Reader) presentQuoted(ctx context.Context, viewer Viewer, quoted content.Status, author appaccount.Actor, shown QuoteViewState) (*QuoteView, error) {
	view, err := r.presentWith(ctx, viewer, quoted, author, false)
	if errors.Is(err, ErrNotFound) {
		return &QuoteView{State: QuoteViewDeleted}, nil // deleted meanwhile
	}
	if err != nil {
		return nil, err
	}
	return &QuoteView{State: shown, Status: &view}, nil
}

// remoteMayQuote reports whether the policy of the local status quoted approves a quote of it by
// the remote account accountID at once, as AnswerQuoteRequest decides.
func (r *Reader) remoteMayQuote(ctx context.Context, accountID string, quoted content.Status) (bool, error) {
	visible, err := r.aud.remoteVisible(ctx, accountID, quoted)
	if err != nil || !visible {
		return false, err
	}
	facts, err := r.quoteFacts(ctx, accountID, quoted)
	if err != nil {
		return false, err
	}
	facts.QuoterIsLocal, facts.QuoterVisible = false, facts.QuoterVisible && visible
	return content.MayQuote(facts) == content.QuoteApproved, nil
}

// quotable reports whether viewer may quote st, which they may see, and returns its interaction
// settings for its local author, nil for anyone else (ADR-0069 §5, ADR-0155 §10).
func (r *Reader) quotable(ctx context.Context, viewer Viewer, st content.Status) (bool, *content.InteractionSettings, error) {
	if r.d.Quotes == nil || viewer.Kind != ViewerLocal || viewer.AccountID == "" {
		return false, nil, nil
	}
	facts, err := r.quoteFacts(ctx, viewer.AccountID, st)
	if err != nil {
		return false, nil, err
	}
	facts.QuoterIsLocal, facts.QuoterVisible = true, true
	allowance := content.MayQuote(facts)
	var own *content.InteractionSettings
	if viewer.AccountID == st.AuthorAccountID && st.IsLocal {
		settings := content.SettingsOf(facts.Policy)
		own = &settings
	}
	return allowance != content.QuoteDenied, own, nil
}

// quoteFacts returns what MayQuote decides a quote of st by accountID by: the policy of st, and
// how the account stands with its author. QuoterVisible is whether neither blocks the other; the
// caller sets whether the account may see st and is local.
func (r *Reader) quoteFacts(ctx context.Context, accountID string, st content.Status) (content.QuoteFacts, error) {
	policy, err := interactionPolicyOf(ctx, r.d.Quotes, st.ID)
	if err != nil {
		return content.QuoteFacts{}, err
	}
	f := content.QuoteFacts{QuoterAccountID: accountID, QuoterVisible: true, Quoted: st, Policy: policy}
	if accountID == st.AuthorAccountID {
		return f, nil
	}
	rel, err := r.d.Relationships.Relationship(ctx, accountID, st.AuthorAccountID)
	if err != nil {
		return content.QuoteFacts{}, err
	}
	f.QuoterVisible = !rel.Blocking && !rel.BlockedBy
	f.QuoterFollows, f.QuoterFollowed = rel.ActivelyFollowing(), rel.ActivelyFollowedBy()
	if f.QuoterMentioned, err = r.d.Interactions.Mentioned(ctx, st.ID, accountID); err != nil {
		return content.QuoteFacts{}, err
	}
	if namesActors(policy, content.InteractionQuote) {
		a, err := r.d.Accounts.FindByID(ctx, accountID)
		switch {
		case err == nil:
			f.QuoterURI = a.Account.URI
		case !errors.Is(err, appaccount.ErrNotFound):
			return content.QuoteFacts{}, err
		}
	}
	return f, nil
}

// QuotesPage is a page of the statuses that quote a status.
type QuotesPage struct {
	Statuses []StatusView
	// Next is the cursor of the next page, "" on the last page.
	Next string
}

// Quotes returns a page of the statuses that quote the local status statusID of the local
// account viewer, newest first (ADR-0069 §5): up to limit quotes older than the cursor before
// ("" for the newest), with the state of each quote. Only the author lists them; another
// account's status, or one hidden from viewer, fails with ErrNotFound, like a missing one.
// Statuses that viewer may not see are left out, so a page may be shorter than limit and still
// have a next page; the last page has none.
func (r *Reader) Quotes(ctx context.Context, viewer Viewer, statusID, before string, limit int) (QuotesPage, error) {
	st, _, err := r.aud.visibleStatus(ctx, viewer, statusID)
	if err != nil {
		return QuotesPage{}, err
	}
	if r.d.Quotes == nil || !st.IsLocal || st.AuthorAccountID != viewer.AccountID {
		return QuotesPage{}, fmt.Errorf("quotes of %s: %w", statusID, ErrNotFound)
	}
	quotes, err := r.d.Quotes.ListQuotesOf(ctx, st.URI, before, limit+1)
	if err != nil {
		return QuotesPage{}, err
	}
	var out QuotesPage
	if len(quotes) > limit {
		quotes = quotes[:limit]
		out.Next = quotes[limit-1].StatusID
	}
	for _, q := range quotes {
		view, err := r.View(ctx, viewer, q.StatusID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return QuotesPage{}, err
		}
		if view.Quote == nil {
			continue // taken away
		}
		out.Statuses = append(out.Statuses, view)
	}
	return out, nil
}
