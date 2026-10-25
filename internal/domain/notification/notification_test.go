// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"slices"
	"testing"
)

func TestTypes(t *testing.T) {
	for _, typ := range Types {
		if !typ.Valid() {
			t.Errorf("%q is not valid", typ)
		}
	}
	for _, typ := range []Type{"", "poke", "Follow"} {
		if typ.Valid() {
			t.Errorf("%q is valid", typ)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Notification{ID: idN, RecipientAccountID: idA, SourceAccountID: idB, Type: TypeFollow, DedupeKey: FollowKey(TypeFollow, idF)}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate(%+v) = %v", ok, err)
	}
	fromInstance := ok
	fromInstance.SourceAccountID, fromInstance.Type, fromInstance.ModerationActionID = "", TypeModerationNotice, idM
	if err := fromInstance.Validate(); err != nil {
		t.Errorf("Validate(%+v) = %v", fromInstance, err)
	}
	resolved := ok
	resolved.SourceAccountID, resolved.Type, resolved.ReportID = "", TypeReportResolved, idR
	if err := resolved.Validate(); err != nil {
		t.Errorf("Validate(%+v) = %v", resolved, err)
	}
	noReport := resolved
	noReport.ReportID = ""
	strayReport := ok
	strayReport.ReportID = idR
	for _, n := range []Notification{noReport, strayReport} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error for the report", n)
		}
	}
	noAction := fromInstance
	noAction.ModerationActionID = ""
	strayAction := ok
	strayAction.ModerationActionID = idM
	for _, n := range []Notification{noAction, strayAction} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error for the moderation action", n)
		}
	}
	severed := ok
	severed.SourceAccountID, severed.Type, severed.SeveranceID = "", TypeRelationshipsSevered, idS
	if err := severed.Validate(); err != nil {
		t.Errorf("Validate(%+v) = %v", severed, err)
	}
	noSeverance := severed
	noSeverance.SeveranceID = ""
	straySeverance := ok
	straySeverance.SeveranceID = idS
	severedBySomeone := severed
	severedBySomeone.SourceAccountID = idB
	for _, n := range []Notification{noSeverance, straySeverance, severedBySomeone} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error for the severance", n)
		}
	}
	self := ok
	self.SourceAccountID = idA
	if err := self.Validate(); !errors.Is(err, ErrSelfNotification) {
		t.Errorf("Validate(self) = %v, want ErrSelfNotification", err)
	}
	unknown := ok
	unknown.Type = "poke"
	if err := unknown.Validate(); !errors.Is(err, ErrInvalidType) {
		t.Errorf("Validate(poke) = %v, want ErrInvalidType", err)
	}
	for _, n := range []Notification{{RecipientAccountID: idA, Type: TypeFollow, DedupeKey: "k"}, {ID: idN, Type: TypeFollow, DedupeKey: "k"}, {ID: idN, RecipientAccountID: idA, Type: TypeFollow}} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", n)
		}
	}
}

func TestKeys(t *testing.T) {
	for got, want := range map[string]string{
		FollowKey(TypeFollowRequest, idF):       "follow_request:" + idF,
		MentionKey(idS):                         "mention:" + idS,
		InteractionKey(TypeFavourite, idS, idA): "favourite:" + idS + ":" + idA,
		InteractionKey(TypeBoost, idS, idA):     "boost:" + idS + ":" + idA,
		AdminReportKey(idR):                     "admin_report:" + idR,
		RelationshipsSeveredKey(idS):            "relationships_severed:" + idS,
		AdminSignUpKey(idU):                     "admin_sign_up:" + idU,
		ReportResolvedKey(idR):                  "report_resolved:" + idR,
		ModerationNoticeKey(idM):                "moderation_notice:" + idM,
		AppealDecidedKey(idP):                   "appeal_decided:" + idP,
		AdminAppealKey(idP):                     "admin_appeal:" + idP,
		PollClosedKey(idP):                      "poll_closed:" + idP,
	} {
		if got != want {
			t.Errorf("key = %q, want %q", got, want)
		}
	}
}

func TestValidateSupporterNotifications(t *testing.T) {
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterPaymentFailed, SupporterSubscriptionID: idS, DedupeKey: SupporterTransitionKey(TypeSupporterPaymentFailed, idT)},
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterSubscriptionEnded, SupporterSubscriptionID: idS, DedupeKey: SupporterTransitionKey(TypeSupporterSubscriptionEnded, idT)},
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterPlanRetired, SupporterSubscriptionID: idS, DedupeKey: SupporterPlanRetiredKey(idP, idS)},
	} {
		if !n.Type.Valid() || !n.Type.Supporter() {
			t.Errorf("%s: Valid %v, Supporter %v", n.Type, n.Type.Valid(), n.Type.Supporter())
		}
		if err := n.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", n, err)
		}
	}
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterPaymentFailed, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterPlanRetired, SupporterSubscriptionID: idS, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, SupporterSubscriptionID: idS, DedupeKey: "k"},
	} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", n)
		}
	}
	if TypeFollow.Supporter() {
		t.Error("follow is a supporter notification")
	}
}

func TestValidateModeratorNotifications(t *testing.T) {
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminReport, ReportID: idR, DedupeKey: AdminReportKey(idR)},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminSignUp, SubjectUserID: idU, DedupeKey: AdminSignUpKey(idU)},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminAppeal, AppealID: idP, DedupeKey: AdminAppealKey(idP)},
		{ID: idN, RecipientAccountID: idA, Type: TypeAppealDecided, AppealID: idP, DedupeKey: AppealDecidedKey(idP)},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminAccountMoved, AccountMoveID: idR, DedupeKey: AdminAccountMovedKey(idR)},
		{
			ID: idN, RecipientAccountID: idA, Type: TypeAccountMoved, SourceAccountID: idB, AccountMoveID: idR,
			AccountMoveOutcome: MoveBlocking, DedupeKey: AccountMovedKey(idR),
		},
	} {
		if err := n.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", n, err)
		}
	}
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminReport, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminReport, ReportID: idR, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminSignUp, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminAppeal, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminAppeal, AppealID: idP, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAppealDecided, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminWebhookDisabled, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminCustomEmojiRequest, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminCustomEmojiRequest, CustomEmojiRequestID: idR, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeCustomEmojiRequestDecided, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, CustomEmojiRequestID: idR, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, AppealID: idP, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAccountMoved, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAccountMoved, SourceAccountID: idB, AccountMoveID: idR, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAccountMoved, AccountMoveID: idR, AccountMoveOutcome: MoveFollowed, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAccountMoved, SourceAccountID: idB, AccountMoveID: idR, AccountMoveOutcome: "lost", DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminAccountMoved, AccountMoveID: idR, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminAccountMoved, AccountMoveID: idR, AccountMoveOutcome: MoveFollowed, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, AccountMoveID: idR, DedupeKey: "k"},
	} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", n)
		}
	}
}

func TestHiddenTypes(t *testing.T) {
	for _, tc := range []struct {
		granted []Permission
		want    []Type
	}{
		{nil, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved}},
		{[]Permission{PermissionManageReports}, []Type{TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved}},
		{[]Permission{PermissionManageAccounts}, []Type{TypeAdminReport, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest}},
		{[]Permission{PermissionManageWebhooks}, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved}},
		{[]Permission{PermissionManageCustomEmojis}, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminAccountMoved}},
		{[]Permission{PermissionManageAccounts, PermissionManageReports, PermissionManageWebhooks, PermissionManageCustomEmojis, "view_audit"}, nil},
	} {
		if got := HiddenTypes(tc.granted); !slices.Equal(got, tc.want) {
			t.Errorf("HiddenTypes(%v) = %v, want %v", tc.granted, got, tc.want)
		}
	}
	if _, ok := ModeratorPermission(TypeFollow); ok {
		t.Error("ModeratorPermission(follow) needs a permission")
	}
}
