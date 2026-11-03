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
	base := func(q content.Status, p content.QuotePolicy) content.QuoteFacts {
		return content.QuoteFacts{QuoterAccountID: testID(3), QuoterIsLocal: true, QuoterVisible: true, Quoted: q, Policy: p}
	}
	for _, tc := range []struct {
		name string
		f    func() content.QuoteFacts
		want content.QuoteAllowance
	}{
		{"public local status", func() content.QuoteFacts { return base(local(content.VisibilityPublic), content.QuotePolicyPublic) }, content.QuoteApproved},
		{"local status without a policy takes the default", func() content.QuoteFacts { return base(local(content.VisibilityPublic), "") }, content.QuoteApproved},
		{"not visible", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), content.QuotePolicyPublic)
			f.QuoterVisible = false
			return f
		}, content.QuoteDenied},
		{"nobody", func() content.QuoteFacts { return base(local(content.VisibilityPublic), content.QuotePolicyNobody) }, content.QuoteDenied},
		{"nobody, by the author", func() content.QuoteFacts {
			f := base(local(content.VisibilityPublic), content.QuotePolicyNobody)
			f.QuoterAccountID = localAuthorID
			return f
		}, content.QuoteApproved},
		{"followers, not following", func() content.QuoteFacts {
			return base(local(content.VisibilityUnlisted), content.QuotePolicyFollowers)
		}, content.QuoteDenied},
		{"followers, following", func() content.QuoteFacts {
			f := base(local(content.VisibilityUnlisted), content.QuotePolicyFollowers)
			f.QuoterFollows = true
			return f
		}, content.QuoteApproved},
		{"direct", func() content.QuoteFacts {
			f := base(local(content.VisibilityDirect), content.QuotePolicyPublic)
			f.QuoterAccountID = localAuthorID
			return f
		}, content.QuoteDenied},
		{"private, by someone else", func() content.QuoteFacts {
			f := base(local(content.VisibilityPrivate), content.QuotePolicyPublic)
			f.QuoterFollows = true
			return f
		}, content.QuoteDenied},
		{"private, by the author in a private status", func() content.QuoteFacts {
			f := base(local(content.VisibilityPrivate), content.QuotePolicyNobody)
			f.QuoterAccountID, f.QuotingVisibility = localAuthorID, content.VisibilityPrivate
			return f
		}, content.QuoteApproved},
		{"private, by the author in a public status", func() content.QuoteFacts {
			f := base(local(content.VisibilityPrivate), content.QuotePolicyPublic)
			f.QuoterAccountID, f.QuotingVisibility = localAuthorID, content.VisibilityPublic
			return f
		}, content.QuoteDenied},
		{"local-only, by a remote account", func() content.QuoteFacts {
			f := base(local(content.VisibilityLocal), content.QuotePolicyPublic)
			f.QuoterIsLocal = false
			return f
		}, content.QuoteDenied},
		{"local-only, by a local account", func() content.QuoteFacts { return base(local(content.VisibilityLocal), content.QuotePolicyPublic) }, content.QuoteApproved},
		{"remote status with a policy", func() content.QuoteFacts { return base(remote(content.VisibilityPublic), content.QuotePolicyPublic) }, content.QuoteRequested},
		{"remote status without a policy", func() content.QuoteFacts { return base(remote(content.VisibilityPublic), "") }, content.QuoteLegacy},
		{"remote status of nobody", func() content.QuoteFacts { return base(remote(content.VisibilityPublic), content.QuotePolicyNobody) }, content.QuoteDenied},
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
