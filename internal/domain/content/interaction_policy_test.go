// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

func TestInteractionSettingValidate(t *testing.T) {
	for _, tc := range []struct {
		set content.InteractionSetting
		ok  bool
	}{
		{content.InteractionSetting{Automatic: content.AudienceEveryone}, true},
		{content.InteractionSetting{Automatic: content.AudienceNobody, Manual: content.AudienceMentioned}, true},
		{content.InteractionSetting{Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone}, true},
		{content.InteractionSetting{}, false},
		{content.InteractionSetting{Automatic: content.AudienceFollowing}, false},
		{content.InteractionSetting{Automatic: content.AudienceEveryone, Manual: content.AudienceEveryone}, false},
		{content.InteractionSetting{Automatic: content.AudienceFollowers, Manual: content.AudienceMentioned}, false},
		{content.InteractionSetting{Automatic: content.AudienceNobody, Manual: content.AudienceAccount}, false},
	} {
		err := tc.set.Validate()
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, content.ErrInvalidInteractionPolicy)) {
			t.Errorf("%+v.Validate() = %v, want ok %v", tc.set, err, tc.ok)
		}
	}
}

func TestInteractionSettingsPolicy(t *testing.T) {
	s := content.DefaultInteractionSettings.
		With(content.InteractionReply, content.InteractionSetting{Automatic: content.AudienceMentioned, Manual: content.AudienceFollowers}).
		With(content.InteractionQuote, content.InteractionSetting{Automatic: content.AudienceNobody})
	p := s.Policy()
	for _, k := range content.InteractionKinds {
		if !p.Declares(k) {
			t.Errorf("the policy declares no rules of %s", k)
		}
	}
	want := content.InteractionPolicy{
		{Kind: content.InteractionReply, Approval: content.ApprovalAutomatic, Audience: content.AudienceMentioned},
		{Kind: content.InteractionReply, Approval: content.ApprovalManual, Audience: content.AudienceFollowers},
		{Kind: content.InteractionAnnounce, Approval: content.ApprovalAutomatic, Audience: content.AudienceEveryone},
		{Kind: content.InteractionQuote, Approval: content.ApprovalAutomatic, Audience: content.AudienceNobody},
	}
	if !slices.Equal(p, want) {
		t.Errorf("Policy = %+v, want %+v", p, want)
	}
	if got := content.SettingsOf(p); got != s {
		t.Errorf("SettingsOf(Policy) = %+v, want %+v", got, s)
	}
	if got := content.SettingsOf(nil); got != content.DefaultInteractionSettings {
		t.Errorf("SettingsOf(nil) = %+v, want the defaults", got)
	}
}

func TestMayInteract(t *testing.T) {
	st := content.Status{ID: testID(2), AuthorAccountID: localAuthorID, IsLocal: true, Visibility: content.VisibilityPublic}
	closed := content.DefaultInteractionSettings.
		With(content.InteractionReply, content.InteractionSetting{Automatic: content.AudienceNobody}).
		With(content.InteractionAnnounce, content.InteractionSetting{Automatic: content.AudienceNobody, Manual: content.AudienceEveryone}).
		Policy()
	facts := func(kind content.InteractionKind) content.InteractionFacts {
		return content.InteractionFacts{Kind: kind, ActorAccountID: testID(3), ActorIsLocal: true, ActorVisible: true, Status: st, Policy: closed}
	}
	for _, tc := range []struct {
		name string
		f    func() content.InteractionFacts
		want content.InteractionDecision
	}{
		{"reply refused", func() content.InteractionFacts { return facts(content.InteractionReply) }, content.InteractionDenied},
		{"reply by the mentioned", func() content.InteractionFacts {
			f := facts(content.InteractionReply)
			f.Mentioned = true
			return f
		}, content.InteractionAllowed},
		{"reply by the replied-to", func() content.InteractionFacts {
			f := facts(content.InteractionReply)
			f.RepliedTo = true
			return f
		}, content.InteractionAllowed},
		{"boost pending", func() content.InteractionFacts { return facts(content.InteractionAnnounce) }, content.InteractionPending},
		{"boost of a followers-only status", func() content.InteractionFacts {
			f := facts(content.InteractionAnnounce)
			f.Status.Visibility = content.VisibilityPrivate
			return f
		}, content.InteractionDenied},
		{"boost of a local-only status by a remote account", func() content.InteractionFacts {
			f := facts(content.InteractionAnnounce)
			f.Status.Visibility, f.ActorIsLocal, f.Policy = content.VisibilityLocal, false, nil
			return f
		}, content.InteractionDenied},
		{"quote by default", func() content.InteractionFacts { return facts(content.InteractionQuote) }, content.InteractionAllowed},
		{"remote status without rules", func() content.InteractionFacts {
			f := facts(content.InteractionReply)
			f.Status.IsLocal, f.Policy = false, nil
			return f
		}, content.InteractionAllowed},
		{"the author", func() content.InteractionFacts {
			f := facts(content.InteractionReply)
			f.ActorAccountID = localAuthorID
			return f
		}, content.InteractionAllowed},
		{"not visible", func() content.InteractionFacts {
			f := facts(content.InteractionQuote)
			f.ActorVisible = false
			return f
		}, content.InteractionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := content.MayInteract(tc.f()); got != tc.want {
				t.Errorf("MayInteract = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestInteractionStateOf(t *testing.T) {
	r := content.InteractionRequest{ID: "r", Kind: content.InteractionReply, TargetAccountID: "author"}
	ev := func(k content.InteractionEventKind, uri string) content.InteractionEvent {
		return content.InteractionEvent{RequestID: r.ID, Kind: k, AuthorizationURI: uri}
	}
	for _, tc := range []struct {
		name   string
		events []content.InteractionEvent
		want   content.InteractionState
	}{
		{"no answer", nil, content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestPending}},
		{"accepted", []content.InteractionEvent{ev(content.InteractionEventAccepted, "a")},
			content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestAccepted, AuthorizationURI: "a"}},
		{"rejected", []content.InteractionEvent{ev(content.InteractionEventRejected, "")},
			content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestRejected}},
		{"revoked by its authorization", []content.InteractionEvent{ev(content.InteractionEventAccepted, "a"), ev(content.InteractionEventRevoked, "a")},
			content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestRevoked}},
		{"withdrawn while pending", []content.InteractionEvent{ev(content.InteractionEventWithdrawn, ""), ev(content.InteractionEventAccepted, "a")},
			content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestWithdrawn}},
		{"withdrawn after an answer", []content.InteractionEvent{ev(content.InteractionEventRejected, ""), ev(content.InteractionEventWithdrawn, "")},
			content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestRejected}},
		{"revoked by another authorization", []content.InteractionEvent{ev(content.InteractionEventAccepted, "a"), ev(content.InteractionEventRevoked, "b")},
			content.InteractionState{RequestID: "r", Kind: content.InteractionReply, TargetAccountID: "author", State: content.InteractionRequestAccepted, AuthorizationURI: "a"}},
	} {
		if got := content.InteractionStateOf(r, tc.events); got != tc.want {
			t.Errorf("%s: InteractionStateOf = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestQuoteStateFrom(t *testing.T) {
	fep := content.Quote{StatusID: "s", QuotedURI: "q", Form: content.QuoteFormFEP044f}
	legacy := content.Quote{StatusID: "s", QuotedURI: "q", Form: content.QuoteFormLegacy}
	accepted := &content.InteractionState{State: content.InteractionRequestAccepted, AuthorizationURI: "a"}
	for _, tc := range []struct {
		name     string
		q        content.Quote
		detached bool
		req      *content.InteractionState
		want     content.QuoteState
	}{
		{"no request", fep, false, nil, content.QuoteState{StatusID: "s", Approval: content.QuoteApprovalPending}},
		{"accepted", fep, false, accepted, content.QuoteState{StatusID: "s", Approval: content.QuoteApprovalAccepted, AuthorizationURI: "a"}},
		{"rejected", fep, false, &content.InteractionState{State: content.InteractionRequestRejected}, content.QuoteState{StatusID: "s", Approval: content.QuoteApprovalRejected}},
		{"revoked", fep, false, &content.InteractionState{State: content.InteractionRequestRevoked}, content.QuoteState{StatusID: "s", Approval: content.QuoteApprovalRevoked}},
		{"detached", fep, true, accepted, content.QuoteState{StatusID: "s", Approval: content.QuoteApprovalDetached}},
		{"legacy", legacy, false, accepted, content.QuoteState{StatusID: "s", Approval: content.QuoteApprovalLegacy}},
	} {
		if got := content.QuoteStateFrom(tc.q, tc.detached, tc.req); got != tc.want {
			t.Errorf("%s: QuoteStateFrom = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestAdmitsEveryone(t *testing.T) {
	open := content.DefaultInteractionSettings.Policy()
	followers := content.InteractionSettings{
		Reply:    content.InteractionSetting{Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone},
		Announce: content.InteractionSetting{Automatic: content.AudienceEveryone},
		Quote:    content.InteractionSetting{Automatic: content.AudienceEveryone},
	}.Policy()
	for _, tc := range []struct {
		name    string
		p       content.InteractionPolicy
		isLocal bool
		want    bool
	}{
		{"everyone", open, true, true},
		{"followers with manual everyone", followers, true, false},
		{"a local status without rules", nil, true, true},
		{"a remote status that declares none", nil, false, true},
	} {
		if got := content.AdmitsEveryone(tc.p, content.InteractionReply, tc.isLocal); got != tc.want {
			t.Errorf("%s: AdmitsEveryone = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestNeedsAuthorization(t *testing.T) {
	followersOnly := content.InteractionSettings{
		Reply:    content.InteractionSetting{Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone},
		Announce: content.InteractionSetting{Automatic: content.AudienceEveryone},
		Quote:    content.InteractionSetting{Automatic: content.AudienceEveryone},
	}.Policy()
	st := content.Status{ID: "s", AuthorAccountID: "author", IsLocal: false}
	f := content.InteractionFacts{Kind: content.InteractionReply, ActorAccountID: "a", ActorVisible: true, Status: st, Policy: followersOnly}
	for _, tc := range []struct {
		name string
		f    func(content.InteractionFacts) content.InteractionFacts
		d    content.InteractionDecision
		want bool
	}{
		{"pending", func(f content.InteractionFacts) content.InteractionFacts { return f }, content.InteractionPending, true},
		{"denied", func(f content.InteractionFacts) content.InteractionFacts { return f }, content.InteractionDenied, false},
		{"a follower", func(f content.InteractionFacts) content.InteractionFacts { f.Follows = true; return f }, content.InteractionAllowed, true},
		{"mentioned", func(f content.InteractionFacts) content.InteractionFacts { f.Mentioned = true; return f }, content.InteractionAllowed, false},
		{"the author", func(f content.InteractionFacts) content.InteractionFacts { f.ActorAccountID = "author"; return f }, content.InteractionAllowed, false},
		{"a boost open to everyone", func(f content.InteractionFacts) content.InteractionFacts {
			f.Kind = content.InteractionAnnounce
			return f
		}, content.InteractionAllowed, false},
	} {
		if got := content.NeedsAuthorization(tc.f(f), tc.d); got != tc.want {
			t.Errorf("%s: NeedsAuthorization = %t, want %t", tc.name, got, tc.want)
		}
	}
}
