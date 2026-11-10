// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"testing"
	"time"
)

func policyOf(actions map[Condition]Action) Policy {
	var cs []PolicyChoice
	for c, a := range actions {
		cs = append(cs, PolicyChoice{Condition: c, Action: a})
	}
	return NewPolicy(cs)
}

func TestClassifyDefaultsAcceptEverything(t *testing.T) {
	stranger := Facts{NewAccount: true, UnsolicitedPrivateMention: true, LimitedAccount: true}
	for _, typ := range Types {
		if got := Classify(typ, Policy{}, stranger); got != ActionAccept {
			t.Errorf("Classify(%s) with the defaults = %s, want accept", typ, got)
		}
	}
	if (Policy{}).Strict() {
		t.Error("the zero policy is strict")
	}
}

func TestClassifyStrictestWins(t *testing.T) {
	p := policyOf(map[Condition]Action{ConditionNotFollowing: ActionFilter, ConditionNewAccount: ActionDrop})
	if got := Classify(TypeMention, p, Facts{NewAccount: true}); got != ActionDrop {
		t.Errorf("filter and drop matched: %s, want drop", got)
	}
	if got := Classify(TypeMention, p, Facts{}); got != ActionFilter {
		t.Errorf("only not_following matched: %s, want filter", got)
	}
	if got := Classify(TypeMention, p, Facts{Following: true}); got != ActionAccept {
		t.Errorf("a followed source: %s, want accept", got)
	}
}

func TestClassifyFollowingExemptsAllButLimited(t *testing.T) {
	p := policyOf(map[Condition]Action{
		ConditionNotFollower: ActionDrop, ConditionNewAccount: ActionDrop, ConditionUnsolicitedPrivateMention: ActionDrop,
		ConditionLimitedAccount: ActionFilter,
	})
	followed := Facts{Following: true, NewAccount: true, UnsolicitedPrivateMention: true}
	if got := Classify(TypeMention, p, followed); got != ActionAccept {
		t.Errorf("a followed new account: %s, want accept", got)
	}
	followed.LimitedAccount = true
	if got := Classify(TypeMention, p, followed); got != ActionFilter {
		t.Errorf("a followed limited account: %s, want filter", got)
	}
	if got := Classify(TypeFollow, p, Facts{Follower: true}); got != ActionAccept {
		t.Errorf("a follower that the recipient does not follow: %s, want accept", got)
	}
	if got := Classify(TypeFollow, p, Facts{}); got != ActionDrop {
		t.Errorf("a stranger: %s, want drop", got)
	}
}

func TestClassifyAcceptedSourceMeetsNothing(t *testing.T) {
	p := policyOf(map[Condition]Action{ConditionNotFollowing: ActionDrop, ConditionLimitedAccount: ActionDrop})
	if got := Classify(TypeFavourite, p, Facts{Accepted: true, LimitedAccount: true}); got != ActionAccept {
		t.Errorf("an accepted source: %s, want accept", got)
	}
}

func TestClassifyNeverTouchesInstanceNotices(t *testing.T) {
	p := policyOf(map[Condition]Action{ConditionNotFollowing: ActionDrop, ConditionNotFollower: ActionDrop})
	for _, typ := range []Type{
		TypeModerationNotice, TypeReportResolved, TypeAppealDecided, TypeHookAppealDecided, TypeRelationshipsSevered,
		TypeAdminReport, TypeAdminAppeal, TypeAnnouncement, TypeServiceClosure, TypeSupporterPaymentFailed,
		TypeFollowAccepted, TypeStatus, TypePollClosed, TypeAccountMoved, TypeVoiceRoomStarted,
	} {
		if got := Classify(typ, p, Facts{}); got != ActionAccept {
			t.Errorf("Classify(%s) = %s, want accept", typ, got)
		}
	}
	for _, typ := range []Type{TypeFollow, TypeFollowRequest, TypeMention, TypeBoost, TypeFavourite, TypeReaction, TypeQuote, TypeVoiceRoomInvited} {
		if got := Classify(typ, p, Facts{}); got != ActionDrop {
			t.Errorf("Classify(%s) = %s, want drop", typ, got)
		}
	}
}

func TestNewPolicyIgnoresUnknownChoices(t *testing.T) {
	p := NewPolicy([]PolicyChoice{{Condition: "for_bots", Action: ActionDrop}, {Condition: ConditionNewAccount, Action: "block"}})
	if p.Strict() {
		t.Error("unknown choices made the policy strict")
	}
	if err := (PolicyChoice{Condition: ConditionNewAccount, Action: "block"}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("Validate = %v, want ErrInvalidPolicy", err)
	}
}

func TestNewAccountAt(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		created time.Time
		want    bool
	}{
		{now.Add(-NewAccountAge + time.Hour), true},
		{now.Add(-NewAccountAge), false},
		{now.Add(time.Hour), false},
	}
	for _, c := range cases {
		if got := NewAccountAt(c.created, now); got != c.want {
			t.Errorf("NewAccountAt(%s) = %v, want %v", c.created, got, c.want)
		}
	}
}
