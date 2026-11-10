// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

func TestQuoteStateOf(t *testing.T) {
	quotingID := testID(1)
	fep := content.Quote{StatusID: quotingID, QuotedURI: "https://b.example/n/1", Form: content.QuoteFormFEP044f}
	legacy := content.Quote{StatusID: quotingID, QuotedURI: "https://b.example/n/1", Form: content.QuoteFormLegacy}
	accepted := func(uri string) content.QuoteEvent {
		return content.QuoteEvent{Kind: content.QuoteEventAccepted, AuthorizationURI: uri}
	}
	revoked := func(uri string) content.QuoteEvent {
		return content.QuoteEvent{Kind: content.QuoteEventRevoked, AuthorizationURI: uri}
	}
	detached := content.QuoteEvent{Kind: content.QuoteEventDetached}
	rejected := content.QuoteEvent{Kind: content.QuoteEventRejected}
	for _, tc := range []struct {
		name   string
		q      content.Quote
		events []content.QuoteEvent
		want   content.QuoteState
	}{
		{"FEP-044f starts pending", fep, nil, content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalPending}},
		{"legacy starts legacy", legacy, nil, content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalLegacy}},
		{"accepted", fep, []content.QuoteEvent{accepted("a1")},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalAccepted, AuthorizationURI: "a1"}},
		{"accepted without an authorization (own status)", fep, []content.QuoteEvent{accepted("")},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalAccepted}},
		{"revoked", fep, []content.QuoteEvent{accepted("a1"), revoked("a1")},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalRevoked}},
		{"revoking another authorization", fep, []content.QuoteEvent{accepted("a1"), revoked("a0")},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalAccepted, AuthorizationURI: "a1"}},
		{"approved again after a revocation", fep, []content.QuoteEvent{accepted("a1"), revoked("a1"), accepted("a2")},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalAccepted, AuthorizationURI: "a2"}},
		{"rejected", fep, []content.QuoteEvent{rejected}, content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalRejected}},
		{"detached stays detached", fep, []content.QuoteEvent{accepted("a1"), detached, accepted("a2")},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalDetached}},
		{"legacy ignores approvals", legacy, []content.QuoteEvent{accepted("a1"), rejected},
			content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalLegacy}},
		{"legacy detached", legacy, []content.QuoteEvent{detached}, content.QuoteState{StatusID: quotingID, Approval: content.QuoteApprovalDetached}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := content.QuoteStateOf(tc.q, tc.events); got != tc.want {
				t.Errorf("QuoteStateOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMayQuote(t *testing.T) {
	local := func(v content.Visibility) content.Status {
		return content.Status{ID: testID(2), AuthorAccountID: localAuthorID, IsLocal: true, Visibility: v}
	}
	remote := func(v content.Visibility) content.Status {
		return content.Status{ID: testID(2), AuthorAccountID: localAuthorID, Visibility: v}
	}
	base := func(q content.Status, p content.InteractionPolicy) content.QuoteFacts {
		return content.QuoteFacts{QuoterAccountID: testID(3), QuoterIsLocal: true, QuoterVisible: true, Quoted: q, Policy: p}
	}
	policy := func(rules ...content.InteractionRule) content.InteractionPolicy { return rules }
	auto := func(a content.InteractionAudience) content.InteractionRule {
		return content.InteractionRule{Kind: content.InteractionQuote, Approval: content.ApprovalAutomatic, Audience: a}
	}
	manual := func(a content.InteractionAudience) content.InteractionRule {
		return content.InteractionRule{Kind: content.InteractionQuote, Approval: content.ApprovalManual, Audience: a}
	}
	public, followers, nobody := policy(auto(content.AudienceEveryone)), policy(auto(content.AudienceFollowers)), policy(auto(content.AudienceNobody))
	for _, tc := range []struct {
		name string
		f    func() content.QuoteFacts
		want content.QuoteAllowance
	}{
		{"public local status", func() content.QuoteFacts { return base(local(content.VisibilityPublic), public) }, content.QuoteApproved},
		{"local status without a policy takes the default", func() content.QuoteFacts { return base(local(content.VisibilityPublic), nil) }, content.QuoteApproved},
		{"not visible", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), public)
			f.QuoterVisible = false
			return f
		}, content.QuoteDenied},
		{"nobody", func() content.QuoteFacts { return base(local(content.VisibilityPublic), nobody) }, content.QuoteDenied},
		{"nobody, by the author", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), nobody)
			f.QuoterAccountID = localAuthorID
			return f
		}, content.QuoteApproved},
		{"followers, not following", func() content.QuoteFacts {
			return base(local(content.VisibilityUnlisted), followers)
		}, content.QuoteDenied},
		{"followers, following", func() content.QuoteFacts {
			f := base(local(content.VisibilityUnlisted), followers)
			f.QuoterFollows = true
			return f
		}, content.QuoteApproved},
		{"direct", func() content.QuoteFacts {
			f := base(local(content.VisibilityDirect), public)
			f.QuoterAccountID = localAuthorID
			return f
		}, content.QuoteDenied},
		{"private, by someone else", func() content.QuoteFacts {
			f := base(local(content.VisibilityPrivate), public)
			f.QuoterFollows = true
			return f
		}, content.QuoteDenied},
		{"private, by the author in a private status", func() content.QuoteFacts {
			f := base(local(content.VisibilityPrivate), nobody)
			f.QuoterAccountID, f.QuotingVisibility = localAuthorID, content.VisibilityPrivate
			return f
		}, content.QuoteApproved},
		{"private, by the author in a public status", func() content.QuoteFacts {
			f := base(local(content.VisibilityPrivate), public)
			f.QuoterAccountID, f.QuotingVisibility = localAuthorID, content.VisibilityPublic
			return f
		}, content.QuoteDenied},
		{"local-only, by a remote account", func() content.QuoteFacts {
			f := base(local(content.VisibilityLocal), public)
			f.QuoterIsLocal = false
			return f
		}, content.QuoteDenied},
		{"local-only, by a local account", func() content.QuoteFacts { return base(local(content.VisibilityLocal), public) }, content.QuoteApproved},
		{"remote status with a policy", func() content.QuoteFacts { return base(remote(content.VisibilityPublic), public) }, content.QuoteRequested},
		{"remote status without a policy", func() content.QuoteFacts { return base(remote(content.VisibilityPublic), nil) }, content.QuoteLegacy},
		{"remote status of nobody", func() content.QuoteFacts {
			return base(remote(content.VisibilityPublic), policy(auto(content.AudienceAuthor)))
		}, content.QuoteDenied},
		{"followers, mentioned", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), followers)
			f.QuoterMentioned = true
			return f
		}, content.QuoteApproved},
		{"mentioned, not mentioned", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), policy(auto(content.AudienceMentioned)))
			f.QuoterFollows = true
			return f
		}, content.QuoteDenied},
		{"manual approval of a local status", func() content.QuoteFacts {
			return base(local(content.VisibilityPublic), policy(auto(content.AudienceFollowers), manual(content.AudienceEveryone)))
		}, content.QuotePending},
		{"automatic approval before manual", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), policy(auto(content.AudienceFollowers), manual(content.AudienceEveryone)))
			f.QuoterFollows = true
			return f
		}, content.QuoteApproved},
		{"manual approval of a remote status", func() content.QuoteFacts {
			return base(remote(content.VisibilityPublic), policy(auto(content.AudienceAuthor), manual(content.AudienceEveryone)))
		}, content.QuoteRequested},
		{"remote followers leave the mentioned out", func() content.QuoteFacts {
			f := base(remote(content.VisibilityPublic), followers)
			f.QuoterMentioned = true
			return f
		}, content.QuoteDenied},
		{"remote following", func() content.QuoteFacts {
			f := base(remote(content.VisibilityPublic), policy(auto(content.AudienceFollowing)))
			f.QuoterFollowed = true
			return f
		}, content.QuoteRequested},
		{"remote account", func() content.QuoteFacts {
			f := base(remote(content.VisibilityPublic), policy(content.InteractionRule{
				Kind: content.InteractionQuote, Approval: content.ApprovalAutomatic, Audience: content.AudienceAccount, ActorURI: "https://a.example/users/q",
			}))
			f.QuoterURI = "https://a.example/users/q"
			return f
		}, content.QuoteRequested},
		{"remote reply rules declare no quote rules", func() content.QuoteFacts {
			return base(remote(content.VisibilityPublic), policy(content.InteractionRule{
				Kind: content.InteractionReply, Approval: content.ApprovalAutomatic, Audience: content.AudienceAuthor,
			}))
		}, content.QuoteLegacy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := content.MayQuote(tc.f()); got != tc.want {
				t.Errorf("MayQuote = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestQuotesRemotePosts(t *testing.T) {
	for v, want := range map[content.Visibility]bool{
		content.VisibilityPublic: true, content.VisibilityUnlisted: true,
		content.VisibilityLocal: false, content.VisibilityPrivate: false, content.VisibilityDirect: false,
	} {
		if got := content.QuotesRemotePosts(v); got != want {
			t.Errorf("QuotesRemotePosts(%s) = %v, want %v", v, got, want)
		}
	}
}

func TestQuoteApprovalShown(t *testing.T) {
	for a, want := range map[content.QuoteApproval]bool{
		content.QuoteApprovalAccepted: true, content.QuoteApprovalLegacy: true, content.QuoteApprovalPending: false,
		content.QuoteApprovalRejected: false, content.QuoteApprovalRevoked: false, content.QuoteApprovalDetached: false,
	} {
		if got := a.Shown(); got != want {
			t.Errorf("%s.Shown() = %v, want %v", a, got, want)
		}
	}
}
