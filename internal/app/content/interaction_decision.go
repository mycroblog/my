// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file gathers what content.MayInteract decides the replies and boosts of a status by
// (ADR-0155 §3): the status's rules, and how the interacting account stands with its author.
// The decision itself is content.MayInteract alone.

var (
	// ErrReplyNotAllowed reports a status whose reply rules do not admit the author of a reply
	// (content.MayInteract).
	ErrReplyNotAllowed = errors.New("content: the author of the status does not allow the account to reply to it")
	// ErrBoostNotAllowed reports a status whose boost rules do not admit the booster
	// (content.MayInteract).
	ErrBoostNotAllowed = errors.New("content: the author of the status does not allow the account to boost it")
)

// interactionReaders are the ports that mayInteract reads.
type interactionReaders struct {
	statuses      Repository
	policies      QuoteRepository // nil: no status has rules
	relationships Relationships
	accounts      Accounts
}

// standing returns how accountID stands with the author of st: whether it actively follows the
// author, the author actively follows it, and the current revision of st mentions it.
func (d interactionReaders) standing(ctx context.Context, accountID string, st content.Status) (interactor, error) {
	if accountID == st.AuthorAccountID {
		return interactor{}, nil
	}
	r, err := d.relationships.Relationship(ctx, accountID, st.AuthorAccountID)
	if err != nil {
		return interactor{}, err
	}
	out := interactor{follows: r.ActivelyFollowing(), followedBy: r.ActivelyFollowedBy()}
	rev, err := d.statuses.FindCurrentRevision(ctx, st.ID)
	switch {
	case errors.Is(err, ErrNotFound): // deleted meanwhile: mentions no one
	case err != nil:
		return interactor{}, err
	default:
		out.mentioned = slices.Contains(rev.MentionedAccountIDs, accountID)
	}
	return out, nil
}

// mayInteract returns whether accountID, which may see st (content.CanView, with no block either
// way), may interact with st by kind (content.MayInteract); isLocal is whether the account is
// local, and in how it stands with the author of st, nil to read it here. It reads the rules of
// st and, only when they do not admit the account at once, the rest: how the account stands with
// the author, whether st replies to a status of the account, and the account's actor IRI. An open
// status costs one query.
func (d interactionReaders) mayInteract(ctx context.Context, kind content.InteractionKind, accountID string, isLocal bool, in *interactor, st content.Status) (content.InteractionDecision, error) {
	v, err := d.verdict(ctx, kind, accountID, isLocal, in, st)
	return v.decision, err
}

// interactionVerdict is what mayInteract decides, with whether the interaction needs the
// approval of the author of the status (content.NeedsAuthorization).
type interactionVerdict struct {
	decision           content.InteractionDecision
	needsAuthorization bool
}

// verdict is mayInteract with whether the interaction needs the author's approval.
func (d interactionReaders) verdict(ctx context.Context, kind content.InteractionKind, accountID string, isLocal bool, in *interactor, st content.Status) (interactionVerdict, error) {
	policy, err := interactionPolicyOf(ctx, d.policies, st.ID)
	if err != nil {
		return interactionVerdict{}, err
	}
	f := content.InteractionFacts{
		Kind: kind, ActorAccountID: accountID, ActorIsLocal: isLocal, ActorVisible: true, Status: st, Policy: policy,
	}
	if in != nil {
		f.Follows, f.FollowedBy, f.Mentioned = in.follows, in.followedBy, in.mentioned
	}
	if decision := content.MayInteract(f); decision == content.InteractionAllowed && !content.NeedsAuthorization(f, decision) {
		return interactionVerdict{decision: decision}, nil
	}
	if in == nil {
		got, err := d.standing(ctx, accountID, st)
		if err != nil {
			return interactionVerdict{}, err
		}
		f.Follows, f.FollowedBy, f.Mentioned = got.follows, got.followedBy, got.mentioned
	}
	if kind == content.InteractionReply && st.InReplyToURI != "" {
		parent, err := d.statuses.FindStatusByURI(ctx, st.InReplyToURI)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return interactionVerdict{}, err
		default:
			f.RepliedTo = parent.AuthorAccountID == accountID
		}
	}
	if namesActors(policy, kind) {
		a, err := d.accounts.FindByID(ctx, accountID)
		switch {
		case err == nil:
			f.ActorURI = a.Account.URI
		case !errors.Is(err, appaccount.ErrNotFound):
			return interactionVerdict{}, err
		}
	}
	decision := content.MayInteract(f)
	return interactionVerdict{decision: decision, needsAuthorization: content.NeedsAuthorization(f, decision)}, nil
}

// localVerdict fails with refused unless the local account accountID, which may see st and
// stands with its author as in (nil to read it), may interact with st by kind, at once or once
// the author approves it, and returns the verdict.
func (d interactionReaders) localVerdict(ctx context.Context, kind content.InteractionKind, accountID string, in *interactor, st content.Status, refused error) (interactionVerdict, error) {
	v, err := d.verdict(ctx, kind, accountID, true, in, st)
	if err != nil {
		return interactionVerdict{}, err
	}
	if v.decision == content.InteractionDenied {
		return interactionVerdict{}, fmt.Errorf("status %s: %w", st.ID, refused)
	}
	return v, nil
}

// requested reports whether a local account's interaction with st that v decides goes through a
// request (ADR-0155 §7): one that waits for the author's approval, and one with a remote status
// that its server must approve for third servers to show it. This server trusts itself with
// the rest of the interactions with local statuses.
func (v interactionVerdict) requested(st content.Status) bool {
	return v.decision == content.InteractionPending || (v.needsAuthorization && !st.IsLocal)
}
