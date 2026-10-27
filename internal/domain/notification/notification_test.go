// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"slices"
	"testing"
	"time"
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

func TestValidateVoiceNotifications(t *testing.T) {
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeVoiceRoomStarted, SourceAccountID: idB, VoiceRoomID: idR, DedupeKey: VoiceRoomStartedKey(idR)},
		{ID: idN, RecipientAccountID: idA, Type: TypeVoiceRoomInvited, SourceAccountID: idB, VoiceRoomID: idR, DedupeKey: VoiceRoomInvitedKey(idP)},
	} {
		if err := n.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", n, err)
		}
	}
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeVoiceRoomStarted, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeVoiceRoomStarted, VoiceRoomID: idR, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeVoiceRoomInvited, VoiceRoomID: idR, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, VoiceRoomID: idR, DedupeKey: "k"},
	} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", n)
		}
	}
	if VoiceRoomStartedKey(idR) == VoiceRoomInvitedKey(idR) {
		t.Error("the dedupe keys of the two voice notifications collide")
	}
}

func TestHiddenTypes(t *testing.T) {
	for _, tc := range []struct {
		granted []Permission
		want    []Type
	}{
		{nil, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved, TypeAdminServiceClosureDue, TypeAdminErrorGroup}},
		{[]Permission{PermissionManageReports}, []Type{TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved, TypeAdminServiceClosureDue, TypeAdminErrorGroup}},
		{[]Permission{PermissionManageAccounts}, []Type{TypeAdminReport, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminServiceClosureDue, TypeAdminErrorGroup}},
		{[]Permission{PermissionManageWebhooks}, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved, TypeAdminServiceClosureDue, TypeAdminErrorGroup}},
		{[]Permission{PermissionManageCustomEmojis}, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminAccountMoved, TypeAdminServiceClosureDue, TypeAdminErrorGroup}},
		{[]Permission{PermissionManageServiceClosure}, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved, TypeAdminErrorGroup}},
		{[]Permission{PermissionManageAccounts, PermissionManageReports, PermissionManageWebhooks, PermissionManageCustomEmojis, PermissionManageServiceClosure, PermissionManageErrors, "view_audit"}, nil},
		{[]Permission{PermissionManageErrors}, []Type{TypeAdminReport, TypeAdminSignUp, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeAdminAccountMoved, TypeAdminServiceClosureDue}},
	} {
		if got := HiddenTypes(tc.granted); !slices.Equal(got, tc.want) {
			t.Errorf("HiddenTypes(%v) = %v, want %v", tc.granted, got, tc.want)
		}
	}
	if _, ok := ModeratorPermission(TypeFollow); ok {
		t.Error("ModeratorPermission(follow) needs a permission")
	}
}

func TestValidateServiceClosureNotifications(t *testing.T) {
	const idA, idB, idN, idE, idS = "a", "b", "n", "e", "s"
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeServiceClosure, ServiceClosureEventID: idE, ServiceClosureChange: ClosureAnnounced, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeServiceClosure, ServiceClosureEventID: idE, ServiceClosureChange: ClosureWithdrawn, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminServiceClosureDue, ServiceClosureEventID: idE, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterServiceClosure, ServiceClosureEventID: idE, SupporterSubscriptionID: idS, DedupeKey: "k"},
	} {
		if err := n.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", n, err)
		}
	}
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeServiceClosure, ServiceClosureChange: ClosureAnnounced, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeServiceClosure, ServiceClosureEventID: idE, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeServiceClosure, ServiceClosureEventID: idE, ServiceClosureChange: "closed", DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeServiceClosure, ServiceClosureEventID: idE, ServiceClosureChange: ClosureAnnounced, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminServiceClosureDue, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminServiceClosureDue, ServiceClosureEventID: idE, ServiceClosureChange: ClosureAnnounced, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterServiceClosure, ServiceClosureEventID: idE, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeSupporterServiceClosure, SupporterSubscriptionID: idS, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, ServiceClosureEventID: idE, DedupeKey: "k"},
	} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", n)
		}
	}
	if p, ok := ModeratorPermission(TypeAdminServiceClosureDue); !ok || p != PermissionManageServiceClosure {
		t.Errorf("ModeratorPermission(admin_service_closure_due) = %s, %v", p, ok)
	}
}

func TestValidateErrorGroupNotifications(t *testing.T) {
	const idA, idB, idN, idG = "a", "b", "n", "g"
	hour := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupCreated, ErrorGroupID: idG, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupRegressed, ErrorGroupID: idG, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupOthers, ErrorGroupHour: hour, DedupeKey: "k"},
	} {
		if err := n.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", n, err)
		}
	}
	for _, n := range []Notification{
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupID: idG, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupCreated, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: "fixed", ErrorGroupID: idG, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupCreated, ErrorGroupID: idG, ErrorGroupHour: hour, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupOthers, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupOthers, ErrorGroupID: idG, ErrorGroupHour: hour, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeAdminErrorGroup, ErrorGroupChange: ErrorGroupCreated, ErrorGroupID: idG, SourceAccountID: idB, DedupeKey: "k"},
		{ID: idN, RecipientAccountID: idA, Type: TypeFollow, SourceAccountID: idB, ErrorGroupID: idG, DedupeKey: "k"},
	} {
		if err := n.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", n)
		}
	}
	if p, ok := ModeratorPermission(TypeAdminErrorGroup); !ok || p != PermissionManageErrors {
		t.Errorf("ModeratorPermission(admin_error_group) = %s, %v", p, ok)
	}
	if AdminErrorGroupKey(ErrorGroupCreated, idG) == AdminErrorGroupKey(ErrorGroupRegressed, idG) ||
		AdminErrorGroupOthersKey(hour) == AdminErrorGroupOthersKey(hour.Add(time.Hour)) {
		t.Error("the dedupe keys of admin_error_group collide")
	}
	if got := ErrorGroupHour(hour.Add(59 * time.Minute)); !got.Equal(hour) {
		t.Errorf("ErrorGroupHour = %v, want %v", got, hour)
	}
}
