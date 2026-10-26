// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"slices"
	"time"
)

// Type is the kind of a notification (notifications.type, FR-205).
type Type string

// Notification types. The notification module generates follow, follow_request,
// follow_accepted, mention, boost, favourite, reaction, quote, and poll_closed from the graph and
// content events, and moderation_notice, appeal_decided, report_resolved, and
// relationships_severed from the moderation events, custom_emoji_request_decided from the
// custom emoji requests (ADR-0070 §9), and supporter_payment_failed,
// supporter_subscription_ended, and supporter_plan_retired from the supporter subscriptions
// (ADR-0072 §16), and account_moved from the migration of the follows of a moved account
// (ADR-0089). service_closure tells every local user of the announcement of the service closure,
// of the change of its dates, and of its withdrawal, and supporter_service_closure a subscriber
// whose subscription the announcement cancels (ADR-0088 §2). voice_room_started and
// voice_room_invited come from the voice chat rooms (ADR-0100 §8). admin_report, admin_sign_up,
// admin_appeal, admin_webhook_disabled, admin_custom_emoji_request, admin_account_moved, and
// admin_service_closure_due go to the staff whose roles grant the permission they need
// (ModeratorPermission).
const (
	TypeFollow         Type = "follow"
	TypeFollowRequest  Type = "follow_request"
	TypeFollowAccepted Type = "follow_accepted"
	TypeMention        Type = "mention"
	TypeBoost          Type = "boost"
	TypeFavourite      Type = "favourite"
	// TypeReaction is an emoji reaction to a status of the recipient (FR-116, ADR-0052 §5).
	TypeReaction Type = "reaction"
	// TypeQuote is a quote of a status of the recipient that is shown: approved, or a legacy
	// quote (FR-114, ADR-0069 §5). StatusID is the quoting status.
	TypeQuote            Type = "quote"
	TypePollClosed       Type = "poll_closed"
	TypeModerationNotice Type = "moderation_notice"
	TypeReportResolved   Type = "report_resolved"
	TypeAdminReport      Type = "admin_report"
	TypeAdminSignUp      Type = "admin_sign_up"
	// TypeRelationshipsSevered tells a local account of the follows that a suspension of a
	// domain or a remote account cut (FR-717).
	TypeRelationshipsSevered Type = "relationships_severed"
	TypeAppealDecided        Type = "appeal_decided"
	TypeAdminAppeal          Type = "admin_appeal"
	// TypeAdminWebhookDisabled tells the holders of manage_webhooks that the server disabled a
	// webhook endpoint that kept failing (ADR-0060).
	TypeAdminWebhookDisabled Type = "admin_webhook_disabled"
	// TypeAdminCustomEmojiRequest tells the holders of manage_custom_emojis of a new request to
	// create a custom emoji or to replace the image of one (ADR-0070 §9).
	TypeAdminCustomEmojiRequest Type = "admin_custom_emoji_request"
	// TypeCustomEmojiRequestDecided tells the requester that their custom emoji request was
	// approved or rejected (ADR-0070 §9).
	TypeCustomEmojiRequestDecided Type = "custom_emoji_request_decided"
	// TypeSupporterPaymentFailed tells a subscriber that a payment of their supporter
	// subscription failed and the grace period began (ADR-0072 §16).
	TypeSupporterPaymentFailed Type = "supporter_payment_failed"
	// TypeSupporterSubscriptionEnded tells a subscriber that their supporter subscription lost
	// the plan's role: it lapsed, expired, or was revoked (ADR-0072 §16).
	TypeSupporterSubscriptionEnded Type = "supporter_subscription_ended"
	// TypeSupporterPlanRetired tells a subscriber that their plan was retired and their
	// subscription ends with its period (ADR-0072 §16).
	TypeSupporterPlanRetired Type = "supporter_plan_retired"
	// TypeAccountMoved tells a local account that an account it followed moved (ADR-0089):
	// SourceAccountID is the moved account, AccountMoveID the move, and AccountMoveOutcome what
	// became of the recipient's follow.
	TypeAccountMoved Type = "account_moved"
	// TypeAdminAccountMoved tells the holders of manage_accounts that an account that moderation
	// limits moved, and that the target does not inherit the limit (ADR-0089).
	TypeAdminAccountMoved Type = "admin_account_moved"
	// TypeServiceClosure tells every local user that the closure of the service was announced,
	// that its dates changed, or that it was withdrawn (ADR-0088 §2).
	TypeServiceClosure Type = "service_closure"
	// TypeAdminServiceClosureDue tells the holders of manage_service_closure that the closing
	// date of the announcement in effect has passed, and that they close the server
	// (ADR-0088 §1).
	TypeAdminServiceClosureDue Type = "admin_service_closure_due"
	// TypeSupporterServiceClosure tells a subscriber that the announcement of the service closure
	// cancels their subscription at the end of its period (ADR-0088 §2).
	TypeSupporterServiceClosure Type = "supporter_service_closure"
	// TypeVoiceRoomStarted tells a local follower of a host who sees the room that the host
	// opened a voice chat room (ADR-0100 §8): SourceAccountID is the host and VoiceRoomID the
	// room.
	TypeVoiceRoomStarted Type = "voice_room_started"
	// TypeVoiceRoomInvited tells a local account that it was invited to a voice chat room
	// (ADR-0100 §7, §8): SourceAccountID is the account that invited it and VoiceRoomID the
	// room.
	TypeVoiceRoomInvited Type = "voice_room_invited"
)

// MoveOutcome is what became of a follow of an account that moved (notifications.account_move_outcome).
type MoveOutcome string

// Move outcomes. With MoveFollowed and MoveRequested the recipient follows the target instead
// of the moved account; with the others the recipient still follows the moved account.
const (
	// MoveFollowed: the recipient follows the target.
	MoveFollowed MoveOutcome = "followed"
	// MoveRequested: the recipient's follow of the target awaits its approval.
	MoveRequested MoveOutcome = "requested"
	// MoveBlocking: the recipient blocks the target.
	MoveBlocking MoveOutcome = "blocking"
	// MoveBlockedBy: the target blocks the recipient.
	MoveBlockedBy MoveOutcome = "blocked_by"
	// MoveDomainMuted: the recipient mutes the domain of the target.
	MoveDomainMuted MoveOutcome = "domain_muted"
)

// ClosureChange is what the service closure event of a service_closure did
// (notifications.service_closure_change).
type ClosureChange string

// What a service_closure tells of.
const (
	// ClosureAnnounced: the closure was announced.
	ClosureAnnounced ClosureChange = "announced"
	// ClosureRescheduled: the dates of the announcement changed.
	ClosureRescheduled ClosureChange = "rescheduled"
	// ClosureWithdrawn: the announcement was withdrawn.
	ClosureWithdrawn ClosureChange = "withdrawn"
)

// Valid reports whether c is a known change.
func (c ClosureChange) Valid() bool {
	return c == ClosureAnnounced || c == ClosureRescheduled || c == ClosureWithdrawn
}

// Valid reports whether o is a known move outcome.
func (o MoveOutcome) Valid() bool {
	switch o {
	case MoveFollowed, MoveRequested, MoveBlocking, MoveBlockedBy, MoveDomainMuted:
		return true
	}
	return false
}

// Types are every notification type, in a stable order.
var Types = []Type{
	TypeFollow, TypeFollowRequest, TypeFollowAccepted, TypeMention, TypeBoost, TypeFavourite,
	TypeReaction, TypeQuote, TypePollClosed, TypeModerationNotice, TypeReportResolved, TypeAdminReport, TypeAdminSignUp,
	TypeRelationshipsSevered,
	TypeAppealDecided, TypeAdminAppeal, TypeAdminWebhookDisabled, TypeAdminCustomEmojiRequest, TypeCustomEmojiRequestDecided,
	TypeSupporterPaymentFailed, TypeSupporterSubscriptionEnded, TypeSupporterPlanRetired,
	TypeAccountMoved, TypeAdminAccountMoved,
	TypeServiceClosure, TypeAdminServiceClosureDue, TypeSupporterServiceClosure,
	TypeVoiceRoomStarted, TypeVoiceRoomInvited,
}

// Valid reports whether t is a known notification type.
func (t Type) Valid() bool {
	switch t {
	case TypeFollow, TypeFollowRequest, TypeFollowAccepted, TypeMention, TypeBoost, TypeFavourite,
		TypeReaction, TypeQuote, TypePollClosed, TypeModerationNotice, TypeReportResolved, TypeAdminReport, TypeAdminSignUp,
		TypeRelationshipsSevered, TypeAppealDecided, TypeAdminAppeal, TypeAdminWebhookDisabled,
		TypeAdminCustomEmojiRequest, TypeCustomEmojiRequestDecided,
		TypeSupporterPaymentFailed, TypeSupporterSubscriptionEnded, TypeSupporterPlanRetired,
		TypeAccountMoved, TypeAdminAccountMoved,
		TypeServiceClosure, TypeAdminServiceClosureDue, TypeSupporterServiceClosure,
		TypeVoiceRoomStarted, TypeVoiceRoomInvited:
		return true
	}
	return false
}

// Notification is one notification to a local account: an event that happened to it
// (notifications, data-model §4.7). It never changes; dismissing it is a separate Dismissal.
//
// DedupeKey is unique per recipient, so that processing the same domain event again, or an
// action that the source repeats, inserts nothing: see the *Key functions.
type Notification struct {
	ID                 string
	RecipientAccountID string
	Type               Type
	SourceAccountID    string // "" for moderation_notice, report_resolved, relationships_severed, and the admin types, which come from the instance
	// SourceDomain is the domain of a remote source account, which the recipient's domain mutes
	// match (FR-304); "" for a local source or none.
	SourceDomain       string
	StatusID           string // "" unless the notification is about a status
	ModerationActionID string // "" unless the notification is a moderation_notice
	ReportID           string // "" unless the notification is a report_resolved or an admin_report
	SubjectUserID      string // "" unless the notification is an admin_sign_up: the user who signed up
	SeveranceID        string // "" unless the notification is a relationships_severed
	AppealID           string // "" unless the notification is an appeal_decided or an admin_appeal
	// CustomEmojiRequestID is the custom emoji request of an admin_custom_emoji_request or a
	// custom_emoji_request_decided, "" for the other types.
	CustomEmojiRequestID string
	// SupporterSubscriptionID is the supporter subscription of a supporter_payment_failed, a
	// supporter_subscription_ended, or a supporter_plan_retired, "" for the other types.
	SupporterSubscriptionID string
	// ReactionEmoji and ReactionCustomEmojiID are the emoji of a reaction: a Unicode emoji or a
	// custom emoji (custom_emojis.id), exactly one of them; both "" for the other types.
	ReactionEmoji         string
	ReactionCustomEmojiID string
	// AccountMoveID is the move of an account_moved or an admin_account_moved, "" for the other
	// types; AccountMoveOutcome is what became of the recipient's follow, for an account_moved
	// only.
	AccountMoveID      string
	AccountMoveOutcome MoveOutcome
	// ServiceClosureEventID is the service closure event of a service_closure, the announcement
	// whose closing date has passed of an admin_service_closure_due, and the announcement that
	// cancels the subscription of a supporter_service_closure; "" for the other types.
	// ServiceClosureChange is what the event did, for a service_closure only.
	ServiceClosureEventID string
	ServiceClosureChange  ClosureChange
	// VoiceRoomID is the voice chat room of a voice_room_started or a voice_room_invited, ""
	// for the other types.
	VoiceRoomID string
	DedupeKey   string
	CreatedAt   time.Time
}

var (
	// ErrSelfNotification reports a notification whose source is its recipient: nobody is
	// notified of their own actions.
	ErrSelfNotification = errors.New("notification: an account is not notified of its own actions")
	// ErrInvalidType reports an unknown notification type.
	ErrInvalidType = errors.New("notification: unknown notification type")
)

// Validate checks the invariants of a new notification.
func (n Notification) Validate() error {
	if n.ID == "" || n.RecipientAccountID == "" || n.DedupeKey == "" {
		return errors.New("notification: a notification needs an ID, a recipient, and a dedupe key")
	}
	if !n.Type.Valid() {
		return ErrInvalidType
	}
	if n.SourceAccountID == n.RecipientAccountID {
		return ErrSelfNotification
	}
	if (n.Type == TypeReportResolved || n.Type == TypeAdminReport) != (n.ReportID != "") {
		return errors.New("notification: a report_resolved or an admin_report, and only they, name a report")
	}
	if (n.Type == TypeModerationNotice) != (n.ModerationActionID != "") {
		return errors.New("notification: a moderation_notice, and only it, names a moderation action")
	}
	if (n.Type == TypeRelationshipsSevered) != (n.SeveranceID != "") || (n.Type == TypeRelationshipsSevered && n.SourceAccountID != "") {
		return errors.New("notification: a relationships_severed, and only it, names a severance, and it has no source account")
	}
	if (n.Type == TypeAppealDecided || n.Type == TypeAdminAppeal) != (n.AppealID != "") {
		return errors.New("notification: an appeal_decided or an admin_appeal, and only they, name an appeal")
	}
	if (n.Type == TypeAdminCustomEmojiRequest || n.Type == TypeCustomEmojiRequestDecided) != (n.CustomEmojiRequestID != "") {
		return errors.New("notification: an admin_custom_emoji_request or a custom_emoji_request_decided, and only they, name a custom emoji request")
	}
	if n.Type.Supporter() != (n.SupporterSubscriptionID != "") || (n.Type.Supporter() && n.SourceAccountID != "") {
		return errors.New("notification: the supporter notifications, and only they, name a supporter subscription, and they have no source account")
	}
	if hasEmoji := n.ReactionEmoji != "" || n.ReactionCustomEmojiID != ""; (n.Type == TypeReaction) != hasEmoji ||
		(n.ReactionEmoji != "" && n.ReactionCustomEmojiID != "") {
		return errors.New("notification: a reaction, and only it, names one emoji")
	}
	moves := n.Type == TypeAccountMoved || n.Type == TypeAdminAccountMoved
	if moves != (n.AccountMoveID != "") || (n.Type == TypeAccountMoved) != (n.AccountMoveOutcome != "") ||
		(n.AccountMoveOutcome != "" && !n.AccountMoveOutcome.Valid()) ||
		(n.Type == TypeAccountMoved && n.SourceAccountID == "") {
		return errors.New("notification: an account_moved or an admin_account_moved, and only they, name a move, and an account_moved has the moved account and an outcome")
	}
	closure := n.Type == TypeServiceClosure || n.Type == TypeAdminServiceClosureDue || n.Type == TypeSupporterServiceClosure
	if closure != (n.ServiceClosureEventID != "") || (n.Type == TypeServiceClosure) != (n.ServiceClosureChange != "") ||
		(n.ServiceClosureChange != "" && !n.ServiceClosureChange.Valid()) || (closure && n.SourceAccountID != "") {
		return errors.New("notification: the service closure notifications, and only they, name a service closure event, a service_closure what it did, and they have no source account")
	}
	if voice := n.Type == TypeVoiceRoomStarted || n.Type == TypeVoiceRoomInvited; voice != (n.VoiceRoomID != "") ||
		(voice && n.SourceAccountID == "") {
		return errors.New("notification: a voice_room_started or a voice_room_invited, and only they, name a voice chat room, and they have a source account")
	}
	switch {
	case n.Type == TypeAdminReport && (n.ReportID == "" || n.SourceAccountID != ""),
		n.Type == TypeAdminAccountMoved && n.SourceAccountID != "",
		n.Type == TypeAdminServiceClosureDue && n.SourceAccountID != "",
		n.Type == TypeAdminAppeal && n.SourceAccountID != "",
		n.Type == TypeAdminWebhookDisabled && n.SourceAccountID != "",
		n.Type == TypeAdminCustomEmojiRequest && n.SourceAccountID != "",
		n.Type == TypeCustomEmojiRequestDecided && n.SourceAccountID != "",
		n.Type == TypeAdminSignUp && (n.SubjectUserID == "" || n.SourceAccountID != ""):
		return errors.New("notification: a moderator notification needs its subject and has no source account")
	}
	return nil
}

// Supporter reports whether t is one of the supporter notifications, which tell a subscriber of
// their supporter subscription (ADR-0072 §16).
func (t Type) Supporter() bool {
	return t == TypeSupporterPaymentFailed || t == TypeSupporterSubscriptionEnded || t == TypeSupporterPlanRetired ||
		t == TypeSupporterServiceClosure
}

// SupporterTransitionKey is the dedupe key of a supporter_payment_failed or a
// supporter_subscription_ended notification: one per transition of the subscription that
// entered the state.
func SupporterTransitionKey(t Type, transitionID string) string {
	return string(t) + ":" + transitionID
}

// SupporterPlanRetiredKey is the dedupe key of a supporter_plan_retired notification: one per
// subscription of the plan, which is retired once.
func SupporterPlanRetiredKey(planID, subscriptionID string) string {
	return string(TypeSupporterPlanRetired) + ":" + planID + ":" + subscriptionID
}

// SupporterServiceClosureKey is the dedupe key of a supporter_service_closure notification: one
// per subscription and announcement that starts the notice period.
func SupporterServiceClosureKey(closureEventID, subscriptionID string) string {
	return string(TypeSupporterServiceClosure) + ":" + closureEventID + ":" + subscriptionID
}

// ServiceClosureKey is the dedupe key of a service_closure notification: one per service
// closure event.
func ServiceClosureKey(closureEventID string) string {
	return string(TypeServiceClosure) + ":" + closureEventID
}

// AdminServiceClosureDueKey is the dedupe key of an admin_service_closure_due notification: one
// per announcement whose closing date has passed.
func AdminServiceClosureDueKey(announcementID string) string {
	return string(TypeAdminServiceClosureDue) + ":" + announcementID
}

// FollowKey is the dedupe key of a follow, follow_request, or follow_accepted notification:
// one per follow and type. Following again after unfollowing is a new follow and notifies again.
func FollowKey(t Type, followID string) string { return string(t) + ":" + followID }

// MentionKey is the dedupe key of a mention notification: one per status, so that an edit
// notifies only the accounts it newly mentions.
func MentionKey(statusID string) string { return string(TypeMention) + ":" + statusID }

// InteractionKey is the dedupe key of a boost or favourite notification: one per status and
// source account, so that undoing and repeating the interaction does not notify again.
func InteractionKey(t Type, statusID, sourceAccountID string) string {
	return string(t) + ":" + statusID + ":" + sourceAccountID
}

// ReactionKey is the dedupe key of a reaction notification: one per status, source account, and
// emoji (the key of the reaction's emoji), so that reacting again with the same emoji does not
// notify again, and reacting with another one does (ADR-0052 §5).
func ReactionKey(statusID, sourceAccountID, emojiKey string) string {
	return string(TypeReaction) + ":" + statusID + ":" + sourceAccountID + ":" + emojiKey
}

// PollClosedKey is the dedupe key of a poll_closed notification: one per poll, so that closing
// the poll again (the job runs at least once) does not notify again.
func PollClosedKey(pollID string) string { return string(TypePollClosed) + ":" + pollID }

// ReportResolvedKey is the dedupe key of a report_resolved notification: one per report, so
// that resolving a reopened report again does not notify the reporter again.
func ReportResolvedKey(reportID string) string { return string(TypeReportResolved) + ":" + reportID }

// ModerationNoticeKey is the dedupe key of a moderation_notice notification: one per action.
func ModerationNoticeKey(actionID string) string {
	return string(TypeModerationNotice) + ":" + actionID
}

// RelationshipsSeveredKey is the dedupe key of a relationships_severed notification: one per
// severance, so that processing the same suspension again does not notify again.
func RelationshipsSeveredKey(severanceID string) string {
	return string(TypeRelationshipsSevered) + ":" + severanceID
}

// Permission is a permission of the roles of the identity module (role_permissions.permission)
// that a moderator notification needs.
type Permission string

// Permissions of the moderator notifications.
const (
	PermissionManageReports  Permission = "manage_reports"
	PermissionManageAccounts Permission = "manage_accounts"
	PermissionManageWebhooks Permission = "manage_webhooks"
	// PermissionManageCustomEmojis is the identity module's manage_custom_emojis.
	PermissionManageCustomEmojis Permission = "manage_custom_emojis"
	// PermissionManageServiceClosure is the identity module's manage_service_closure.
	PermissionManageServiceClosure Permission = "manage_service_closure"
)

// ModeratorPermission returns the permission that the recipient of a notification of type t
// needs, when they get it and while they read it, and whether t needs one: admin_report needs
// manage_reports, admin_sign_up, admin_appeal, and admin_account_moved manage_accounts, admin_webhook_disabled
// manage_webhooks, admin_custom_emoji_request manage_custom_emojis, and
// admin_service_closure_due manage_service_closure. The other types need none.
func ModeratorPermission(t Type) (Permission, bool) {
	switch t {
	case TypeAdminReport:
		return PermissionManageReports, true
	case TypeAdminSignUp, TypeAdminAppeal, TypeAdminAccountMoved:
		return PermissionManageAccounts, true
	case TypeAdminWebhookDisabled:
		return PermissionManageWebhooks, true
	case TypeAdminCustomEmojiRequest:
		return PermissionManageCustomEmojis, true
	case TypeAdminServiceClosureDue:
		return PermissionManageServiceClosure, true
	}
	return "", false
}

// HiddenTypes returns the types that need a permission that granted does not hold: the
// notifications of a recipient with the permissions granted that the list leaves out.
func HiddenTypes(granted []Permission) []Type {
	var out []Type
	for _, t := range Types {
		p, ok := ModeratorPermission(t)
		if !ok {
			continue
		}
		if !slices.Contains(granted, p) {
			out = append(out, t)
		}
	}
	return out
}

// AdminReportKey is the dedupe key of an admin_report notification: one per report.
func AdminReportKey(reportID string) string { return string(TypeAdminReport) + ":" + reportID }

// AdminSignUpKey is the dedupe key of an admin_sign_up notification: one per user who signed up.
func AdminSignUpKey(userID string) string { return string(TypeAdminSignUp) + ":" + userID }

// AppealDecidedKey is the dedupe key of an appeal_decided notification: one per appeal.
func AppealDecidedKey(appealID string) string { return string(TypeAppealDecided) + ":" + appealID }

// AdminWebhookDisabledKey is the dedupe key of an admin_webhook_disabled notification: one per
// state change of the endpoint that disabled it.
func AdminWebhookDisabledKey(stateChangeID string) string {
	return string(TypeAdminWebhookDisabled) + ":" + stateChangeID
}

// AdminCustomEmojiRequestKey is the dedupe key of an admin_custom_emoji_request notification: one
// per request.
func AdminCustomEmojiRequestKey(requestID string) string {
	return string(TypeAdminCustomEmojiRequest) + ":" + requestID
}

// CustomEmojiRequestDecidedKey is the dedupe key of a custom_emoji_request_decided notification:
// one per request, which is decided once.
func CustomEmojiRequestDecidedKey(requestID string) string {
	return string(TypeCustomEmojiRequestDecided) + ":" + requestID
}

// AccountMovedKey is the dedupe key of an account_moved notification: one per move, so that
// running the migration of the follows again does not notify again.
func AccountMovedKey(moveID string) string { return string(TypeAccountMoved) + ":" + moveID }

// AdminAccountMovedKey is the dedupe key of an admin_account_moved notification: one per move.
func AdminAccountMovedKey(moveID string) string { return string(TypeAdminAccountMoved) + ":" + moveID }

// VoiceRoomStartedKey is the dedupe key of a voice_room_started notification: one per room and
// recipient, so that running the fan-out again notifies nobody twice.
func VoiceRoomStartedKey(roomID string) string { return string(TypeVoiceRoomStarted) + ":" + roomID }

// VoiceRoomInvitedKey is the dedupe key of a voice_room_invited notification: one per
// invitation, which an account gets once per room.
func VoiceRoomInvitedKey(invitationID string) string {
	return string(TypeVoiceRoomInvited) + ":" + invitationID
}

// AdminAppealKey is the dedupe key of an admin_appeal notification: one per appeal.
func AdminAppealKey(appealID string) string { return string(TypeAdminAppeal) + ":" + appealID }

// Dismissal is the event that the recipient dismissed a notification (notification_dismissals).
// Dismissed notifications leave the list and the unread count.
type Dismissal struct {
	NotificationID string
	DismissedAt    time.Time
}

// ReadMarker is the newest notification an account has read (notification_read_markers, a
// current value). Notifications with a greater ID are unread.
type ReadMarker struct {
	AccountID  string
	LastReadID string
}

// ExclusionReason is why an exclusion hides a target's notifications from a viewer.
type ExclusionReason string

// Exclusion reasons.
const (
	// ExcludedBlock: the viewer blocks the target.
	ExcludedBlock ExclusionReason = "block"
	// ExcludedBlockedBy: the target blocks the viewer.
	ExcludedBlockedBy ExclusionReason = "blocked_by"
	// ExcludedMute: the viewer mutes the target with hide_notifications.
	ExcludedMute ExclusionReason = "mute"
)

// Exclusion hides the notifications whose source is TargetAccountID from ViewerAccountID while
// it is in force: until ExpiresAt, or for good when ExpiresAt is nil (notification_exclusions,
// a projection of the graph module's blocks and mutes). Reading the list and the unread count
// applies the exclusions in force at that time, so a block or mute also hides the notifications
// that came before it, and lifting it shows them again.
type Exclusion struct {
	ViewerAccountID string
	TargetAccountID string
	Reason          ExclusionReason
	ExpiresAt       *time.Time
}
