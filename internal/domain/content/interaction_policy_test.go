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

func TestHeldReplyReasonOf(t *testing.T) {
	for _, tc := range []struct {
		decision content.InteractionDecision
		want     content.HeldReplyReason
		held     bool
	}{
		{content.InteractionAllowed, "", false},
		{content.InteractionPending, content.HeldReplyPending, true},
		{content.InteractionDenied, content.HeldReplyRefused, true},
	} {
		if got, held := content.HeldReplyReasonOf(tc.decision); got != tc.want || held != tc.held || (held && !got.Valid()) {
			t.Errorf("HeldReplyReasonOf(%v) = %q, %t; want %q, %t", tc.decision, got, held, tc.want, tc.held)
		}
	}
}
