// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

// The audit log (FR-704, ADR-0037): one append-only entry per operation of a moderator or an
// administrator, written in the transaction of the operation. An entry names who did what to
// which target and copies the target's display name, so that it stays readable after the
// target is erased; reasons and notes stay in the source event.

// AuditActorKind is who performed an audited operation (audit_log_entries.actor_kind).
type AuditActorKind string

// Kinds of actors.
const (
	// AuditActorUser is a moderator or an administrator signed in; the entry names the user.
	AuditActorUser AuditActorKind = "user"
	// AuditActorCLI is the command line, which acts as no user.
	AuditActorCLI AuditActorKind = "cli"
	// AuditActorSystem is the server itself, such as a scheduled job.
	AuditActorSystem AuditActorKind = "system"
	// AuditActorRemote is another server.
	AuditActorRemote AuditActorKind = "remote"
)

// Valid reports whether k is one of the kinds of actors.
func (k AuditActorKind) Valid() bool {
	switch k {
	case AuditActorUser, AuditActorCLI, AuditActorSystem, AuditActorRemote:
		return true
	}
	return false
}

// AuditTargetKind is the kind of what an audited operation acted on
// (audit_log_entries.target_kind).
type AuditTargetKind string

// Kinds of targets.
const (
	AuditTargetAccount     AuditTargetKind = "account"
	AuditTargetStatus      AuditTargetKind = "status"
	AuditTargetDomain      AuditTargetKind = "domain"
	AuditTargetReport      AuditTargetKind = "report"
	AuditTargetAppeal      AuditTargetKind = "appeal"
	AuditTargetUser        AuditTargetKind = "user"
	AuditTargetRole        AuditTargetKind = "role"
	AuditTargetSetting     AuditTargetKind = "setting"
	AuditTargetRule        AuditTargetKind = "rule"
	AuditTargetSignupBlock AuditTargetKind = "signup_block"
	// AuditTargetCustomEmoji is a custom emoji (ADR-0051 §5); its label is :shortcode:, or
	// :shortcode:@domain for a remote one.
	AuditTargetCustomEmoji AuditTargetKind = "custom_emoji"
	// AuditTargetWebhook is a webhook endpoint (ADR-0060); its label is its description and the
	// host name of its URL, never the URL, which may hold a secret.
	AuditTargetWebhook AuditTargetKind = "webhook"
	// AuditTargetAnnouncement is a server-wide announcement (FR-726); its label is the first
	// line of its body.
	AuditTargetAnnouncement AuditTargetKind = "announcement"
	// AuditTargetInvite is an invite (ADR-0058); its label is the first characters of its code,
	// never the whole code, which lets anyone sign up while the invite works.
	AuditTargetInvite AuditTargetKind = "invite"
	// AuditTargetRelay is a relay (ADR-0061); its label is its registered URL.
	AuditTargetRelay AuditTargetKind = "relay"
	// AuditTargetBillingSource is a source of the billing imports (ADR-0068); its ID is the
	// source's name (aws, gcp) and its label the provider's name.
	AuditTargetBillingSource AuditTargetKind = "billing_source"
	// AuditTargetCostItem is a cost item of the instance (ADR-0067 §7); its label is the item's
	// name.
	AuditTargetCostItem AuditTargetKind = "cost_item"
	// AuditTargetSupporterProgram is the supporter program (ADR-0072 §16); its ID is the program
	// change and its label the payment provider's name.
	AuditTargetSupporterProgram AuditTargetKind = "supporter_program"
	// AuditTargetSupporterPlan is a supporter plan; its label is the plan's name.
	AuditTargetSupporterPlan AuditTargetKind = "supporter_plan"
	// AuditTargetSupporterSubscription is a subscription to a supporter plan; its label is the
	// plan's name and the subscriber's handle.
	AuditTargetSupporterSubscription AuditTargetKind = "supporter_subscription"
	// AuditTargetCustomEmojiRequest is a request of a holder of create_custom_emojis (ADR-0070
	// §8); its label is the :shortcode: of the requested emoji.
	AuditTargetCustomEmojiRequest AuditTargetKind = "custom_emoji_request"
	// AuditTargetPaymentProvider is the payment provider of the supporter program (ADR-0074);
	// its ID and its label are the provider's name (stripe).
	AuditTargetPaymentProvider AuditTargetKind = "payment_provider"
	// AuditTargetOAuthClient is an OAuth client (ADR-0090 §1); its ID is oauth_clients.id and
	// its label the client's name and client_id.
	AuditTargetOAuthClient AuditTargetKind = "oauth_client"
	// AuditTargetOAuthClientHost is a host whose Client ID Metadata Documents an administrator
	// denied (ADR-0090 §1); its ID and its label are the host.
	AuditTargetOAuthClientHost AuditTargetKind = "oauth_client_host"
)

var auditTargetKinds = []AuditTargetKind{
	AuditTargetAccount, AuditTargetStatus, AuditTargetDomain, AuditTargetReport, AuditTargetAppeal,
	AuditTargetUser, AuditTargetRole, AuditTargetSetting, AuditTargetRule, AuditTargetSignupBlock,
	AuditTargetCustomEmoji, AuditTargetWebhook, AuditTargetAnnouncement, AuditTargetInvite, AuditTargetRelay,
	AuditTargetBillingSource, AuditTargetCostItem, AuditTargetSupporterProgram, AuditTargetSupporterPlan,
	AuditTargetSupporterSubscription, AuditTargetCustomEmojiRequest, AuditTargetPaymentProvider,
	AuditTargetOAuthClient, AuditTargetOAuthClientHost,
}

// AuditTargetKinds returns every kind of target, in the order of the schema's CHECK.
func AuditTargetKinds() []AuditTargetKind { return slices.Clone(auditTargetKinds) }

// Valid reports whether k is one of the kinds of targets.
func (k AuditTargetKind) Valid() bool { return slices.Contains(auditTargetKinds, k) }

// AuditAction is the kind of an audited operation (audit_log_entries.action). Each action has
// one kind of target (AuditAction.Target).
type AuditAction string

// Audited operations. A new administrative operation adds its action here, to the schema's
// CHECK, and to the API's enum, and emits it from its use case.
const (
	// AuditAccountActionIssued: an action on an account (moderation_actions).
	AuditAccountActionIssued AuditAction = "account_action_issued"
	// AuditAccountActionLifted: the lift of an action on an account (moderation_action_lifts).
	AuditAccountActionLifted AuditAction = "account_action_lifted"
	// AuditDomainActionIssued: an action on a domain (domain_moderation_actions).
	AuditDomainActionIssued AuditAction = "domain_action_issued"
	// AuditDomainActionLifted: the lift of an action on a domain.
	AuditDomainActionLifted AuditAction = "domain_action_lifted"
	// AuditReportAssigned: a moderator took a report, or was given it (report_assignments).
	AuditReportAssigned AuditAction = "report_assigned"
	// AuditReportUnassigned: a report was left to nobody (report_assignments).
	AuditReportUnassigned AuditAction = "report_unassigned"
	// AuditReportResolved: a report was resolved (report_resolutions).
	AuditReportResolved AuditAction = "report_resolved"
	// AuditReportReopened: a resolved report was reopened (report_reopenings).
	AuditReportReopened AuditAction = "report_reopened"
	// AuditReportNoteAdded: a moderator wrote an internal note on a report (report_notes).
	AuditReportNoteAdded AuditAction = "report_note_added"
	// AuditRegistrationApproved: a pending registration was approved (registration_decisions).
	AuditRegistrationApproved AuditAction = "registration_approved"
	// AuditRegistrationRejected: a pending registration was rejected (registration_decisions).
	AuditRegistrationRejected AuditAction = "registration_rejected"
	// AuditSettingChanged: an instance setting got a new value (instance_setting_revisions).
	AuditSettingChanged AuditAction = "setting_changed"
	// AuditTextChanged: an instance text got a new text (instance_text_revisions).
	AuditTextChanged AuditAction = "text_changed"
	// AuditRuleAdded: a new instance rule (instance_rule_revisions).
	AuditRuleAdded AuditAction = "rule_added"
	// AuditRuleRevised: new texts of a rule (instance_rule_revisions).
	AuditRuleRevised AuditAction = "rule_revised"
	// AuditRuleMoved: a rule moved among the rules in effect (instance_rule_revisions).
	AuditRuleMoved AuditAction = "rule_moved"
	// AuditRuleRetired: a rule was retired (instance_rule_retirements).
	AuditRuleRetired AuditAction = "rule_retired"
	// AuditRoleGranted: a role was granted to a user (user_role_grants).
	AuditRoleGranted AuditAction = "role_granted"
	// AuditRoleRevoked: a role was revoked from a user (user_role_revocations).
	AuditRoleRevoked AuditAction = "role_revoked"
	// AuditRoleCreated: a new role (roles, role_permissions); the operation has no row of its own.
	AuditRoleCreated AuditAction = "role_created"
	// AuditRoleUpdated: the position or the permissions of a role changed (roles,
	// role_permissions); the operation has no row of its own.
	AuditRoleUpdated AuditAction = "role_updated"
	// AuditRoleDeleted: a role that nobody held was deleted; the operation has no row of its own.
	AuditRoleDeleted AuditAction = "role_deleted"
	// AuditAppealUpheld: an appeal was decided and its action kept (moderation_appeal_decisions).
	AuditAppealUpheld AuditAction = "appeal_upheld"
	// AuditAppealOverturned: an appeal was decided and its action lifted
	// (moderation_appeal_decisions); the lift has its own account_action_lifted entry.
	AuditAppealOverturned AuditAction = "appeal_overturned"
	// AuditAccountDeletionCancelled: an overturned appeal of a ban cancelled the scheduled
	// deletion of the account (account_lifecycle_events, deletion_cancelled).
	AuditAccountDeletionCancelled AuditAction = "account_deletion_cancelled"
	// AuditRevealedPrivateData: a moderator was shown the email address and the IP addresses of
	// a local account (FR-714); the operation has no row of its own.
	AuditRevealedPrivateData AuditAction = "revealed_private_data"
	// AuditSignupBlockAdded: a new sign-up block (signup_blocks).
	AuditSignupBlockAdded AuditAction = "signup_block_added"
	// AuditSignupBlockLifted: a sign-up block was lifted (signup_block_lifts).
	AuditSignupBlockLifted AuditAction = "signup_block_lifted"
	// AuditTwoFactorDisabled: an administrator removed the two-factor authentication of a local
	// account (totp_enrollment_revocations).
	AuditTwoFactorDisabled AuditAction = "two_factor_disabled"
	// AuditPasswordResetLinkIssued: an administrator issued a password reset link for the user
	// of a local account (user_tokens, purpose password_reset).
	AuditPasswordResetLinkIssued AuditAction = "password_reset_link_issued"
	// AuditCustomEmojiCreated: a local custom emoji was registered (custom_emojis).
	AuditCustomEmojiCreated AuditAction = "custom_emoji_created"
	// AuditCustomEmojiImageReplaced: a local custom emoji got a new image (custom_emoji_images).
	AuditCustomEmojiImageReplaced AuditAction = "custom_emoji_image_replaced"
	// AuditCustomEmojiCopied: a remote custom emoji was copied into a new local one
	// (custom_emojis).
	AuditCustomEmojiCopied AuditAction = "custom_emoji_copied"
	// AuditCustomEmojiUpdated: the category, aliases, or place in the picker of a local custom
	// emoji changed (custom_emoji_settings, custom_emoji_aliases).
	AuditCustomEmojiUpdated AuditAction = "custom_emoji_updated"
	// AuditCustomEmojiEnabled and AuditCustomEmojiDisabled: a custom emoji, local or remote, was
	// enabled or disabled (custom_emoji_settings).
	AuditCustomEmojiEnabled  AuditAction = "custom_emoji_enabled"
	AuditCustomEmojiDisabled AuditAction = "custom_emoji_disabled"
	// AuditCustomEmojiDeleted: a local custom emoji was deleted with its images; the operation
	// has no row of its own.
	AuditCustomEmojiDeleted AuditAction = "custom_emoji_deleted"
	// AuditWebhookCreated: a webhook endpoint was set up (webhook_endpoints).
	AuditWebhookCreated AuditAction = "webhook_created"
	// AuditWebhookUpdated: the URL, description, or events of a webhook endpoint changed
	// (webhook_endpoint_revisions).
	AuditWebhookUpdated AuditAction = "webhook_updated"
	// AuditWebhookDeleted, AuditWebhookEnabled, and AuditWebhookDisabled: a webhook endpoint
	// changed state (webhook_endpoint_state_changes); the server disables one that answers 410
	// Gone or keeps failing, as the system actor.
	AuditWebhookDeleted  AuditAction = "webhook_deleted"
	AuditWebhookEnabled  AuditAction = "webhook_enabled"
	AuditWebhookDisabled AuditAction = "webhook_disabled"
	// AuditWebhookSecretRotated: a webhook endpoint got a new secret (webhook_secrets).
	AuditWebhookSecretRotated AuditAction = "webhook_secret_rotated"
	// AuditAnnouncementCreated: a new announcement, as a draft or published at once
	// (announcements).
	AuditAnnouncementCreated AuditAction = "announcement_created"
	// AuditAnnouncementUpdated: the body or the window of an announcement changed
	// (announcement_revisions).
	AuditAnnouncementUpdated AuditAction = "announcement_updated"
	// AuditAnnouncementPublished and AuditAnnouncementWithdrawn: an announcement was published or
	// withdrawn (announcement_state_changes).
	AuditAnnouncementPublished AuditAction = "announcement_published"
	AuditAnnouncementWithdrawn AuditAction = "announcement_withdrawn"
	// AuditInviteCreated: an invite was created (invites).
	AuditInviteCreated AuditAction = "invite_created"
	// AuditInviteRevoked: an invite was revoked (invite_revocations).
	AuditInviteRevoked AuditAction = "invite_revoked"
	// AuditRelayAdded: a relay was registered (relays).
	AuditRelayAdded AuditAction = "relay_added"
	// AuditRelayFollowed and AuditRelayUnfollowed: the instance actor sent the Follow that joins
	// a relay or the Undo that leaves it (relay_events, follow_sent and left).
	AuditRelayFollowed   AuditAction = "relay_followed"
	AuditRelayUnfollowed AuditAction = "relay_unfollowed"
	// AuditRelayRemoved: the registration of a relay was deleted with its events; the operation
	// has no row of its own.
	AuditRelayRemoved AuditAction = "relay_removed"
	// AuditRelayAccepted and AuditRelayRejected: a relay accepted or rejected the Follow
	// (relay_events, accepted and rejected), as the remote actor.
	AuditRelayAccepted AuditAction = "relay_accepted"
	AuditRelayRejected AuditAction = "relay_rejected"
	// AuditStatusRefetchQueued: the refetch of a remote post from its origin was queued
	// (FR-407); the operation has no row of its own.
	AuditStatusRefetchQueued AuditAction = "status_refetch_queued"
	// AuditAccountStatusesRefetchQueued: the refetch of the posts of a remote account was
	// queued; the operation has no row of its own.
	AuditAccountStatusesRefetchQueued AuditAction = "account_statuses_refetch_queued"
	// AuditDomainStatusesRefetchQueued: the refetch of the posts of the accounts of a remote
	// domain was queued; the operation has no row of its own.
	AuditDomainStatusesRefetchQueued AuditAction = "domain_statuses_refetch_queued"
	// AuditBillingImportQueued: an administrator queued an import of a billing source now
	// (ADR-0068 §2); the operation has no row of its own.
	AuditBillingImportQueued AuditAction = "billing_import_queued"
	// AuditCostItemCreated, AuditCostItemRelabeled, AuditCostItemPriceRevised,
	// AuditCostItemEndChanged, AuditCostItemCanceled: an administrator recorded a cost item,
	// changed its name, provider, category, or memo, revised its price, changed its end, or
	// canceled it (ADR-0067 §7).
	AuditCostItemCreated      AuditAction = "cost_item_created"
	AuditCostItemRelabeled    AuditAction = "cost_item_relabeled"
	AuditCostItemPriceRevised AuditAction = "cost_item_price_revised"
	AuditCostItemEndChanged   AuditAction = "cost_item_end_changed"
	AuditCostItemCanceled     AuditAction = "cost_item_canceled"
	// AuditSupporterProgramEnabled, AuditSupporterProgramDisabled: an administrator enabled or
	// disabled the supporter program (supporter_program_changes, ADR-0072 §2).
	AuditSupporterProgramEnabled  AuditAction = "supporter_program_enabled"
	AuditSupporterProgramDisabled AuditAction = "supporter_program_disabled"
	// AuditSupporterPlanCreated, AuditSupporterPlanRevised, AuditSupporterPlanRetired: an
	// administrator created a supporter plan, changed its terms, or retired it (supporter_plans,
	// supporter_plan_revisions, supporter_plan_retirements).
	AuditSupporterPlanCreated AuditAction = "supporter_plan_created"
	AuditSupporterPlanRevised AuditAction = "supporter_plan_revised"
	AuditSupporterPlanRetired AuditAction = "supporter_plan_retired"
	// AuditSupporterSubscriptionCanceled: an administrator canceled a subscription at once,
	// without a refund (supporter_subscription_transitions, ADR-0072 §10).
	AuditSupporterSubscriptionCanceled AuditAction = "supporter_subscription_canceled"
	// AuditCustomEmojiRequestSubmitted, AuditCustomEmojiRequestWithdrawn,
	// AuditCustomEmojiRequestApproved, and AuditCustomEmojiRequestRejected: a holder of
	// create_custom_emojis requested a custom emoji or a new image of one
	// (custom_emoji_requests), withdrew the request, or a holder of manage_custom_emojis approved
	// or rejected it (custom_emoji_request_outcomes, ADR-0070 §8). An approval also records the
	// custom_emoji_created or custom_emoji_image_replaced of the emoji.
	AuditCustomEmojiRequestSubmitted AuditAction = "custom_emoji_request_submitted"
	AuditCustomEmojiRequestWithdrawn AuditAction = "custom_emoji_request_withdrawn"
	AuditCustomEmojiRequestApproved  AuditAction = "custom_emoji_request_approved"
	AuditCustomEmojiRequestRejected  AuditAction = "custom_emoji_request_rejected"
	// AuditPaymentReconcileQueued: an administrator queued a reconciliation with the payment
	// provider now (ADR-0074 §4); the operation has no row of its own.
	AuditPaymentReconcileQueued AuditAction = "payment_reconcile_queued"
	// AuditOAuthClientDisabled and AuditOAuthClientEnabled: an administrator disabled an OAuth
	// client, which revokes all its tokens, or lifted the disablement (oauth_client_disablements
	// and oauth_client_disablement_lifts, ADR-0090 §1).
	AuditOAuthClientDisabled AuditAction = "oauth_client_disabled"
	AuditOAuthClientEnabled  AuditAction = "oauth_client_enabled"
	// AuditOAuthClientHostDenied and AuditOAuthClientHostAllowed: an administrator denied the
	// Client ID Metadata Documents of a host, which revokes the tokens of its clients, or lifted
	// the denial (oauth_client_host_denials and oauth_client_host_denial_lifts, ADR-0090 §1).
	AuditOAuthClientHostDenied  AuditAction = "oauth_client_host_denied"
	AuditOAuthClientHostAllowed AuditAction = "oauth_client_host_allowed"
)

// auditActions maps each action to the kind of its target, in the order of the schema's CHECK.
var auditActions = []struct {
	action AuditAction
	target AuditTargetKind
}{
	{AuditAccountActionIssued, AuditTargetAccount},
	{AuditAccountActionLifted, AuditTargetAccount},
	{AuditDomainActionIssued, AuditTargetDomain},
	{AuditDomainActionLifted, AuditTargetDomain},
	{AuditReportAssigned, AuditTargetReport},
	{AuditReportUnassigned, AuditTargetReport},
	{AuditReportResolved, AuditTargetReport},
	{AuditReportReopened, AuditTargetReport},
	{AuditReportNoteAdded, AuditTargetReport},
	{AuditRegistrationApproved, AuditTargetUser},
	{AuditRegistrationRejected, AuditTargetUser},
	{AuditSettingChanged, AuditTargetSetting},
	{AuditTextChanged, AuditTargetSetting},
	{AuditRuleAdded, AuditTargetRule},
	{AuditRuleRevised, AuditTargetRule},
	{AuditRuleMoved, AuditTargetRule},
	{AuditRuleRetired, AuditTargetRule},
	{AuditRoleGranted, AuditTargetRole},
	{AuditRoleRevoked, AuditTargetRole},
	{AuditRoleCreated, AuditTargetRole},
	{AuditRoleUpdated, AuditTargetRole},
	{AuditRoleDeleted, AuditTargetRole},
	{AuditAppealUpheld, AuditTargetAppeal},
	{AuditAppealOverturned, AuditTargetAppeal},
	{AuditAccountDeletionCancelled, AuditTargetAccount},
	{AuditRevealedPrivateData, AuditTargetAccount},
	{AuditSignupBlockAdded, AuditTargetSignupBlock},
	{AuditSignupBlockLifted, AuditTargetSignupBlock},
	{AuditTwoFactorDisabled, AuditTargetAccount},
	{AuditPasswordResetLinkIssued, AuditTargetAccount},
	{AuditCustomEmojiCreated, AuditTargetCustomEmoji},
	{AuditCustomEmojiImageReplaced, AuditTargetCustomEmoji},
	{AuditCustomEmojiCopied, AuditTargetCustomEmoji},
	{AuditCustomEmojiUpdated, AuditTargetCustomEmoji},
	{AuditCustomEmojiEnabled, AuditTargetCustomEmoji},
	{AuditCustomEmojiDisabled, AuditTargetCustomEmoji},
	{AuditCustomEmojiDeleted, AuditTargetCustomEmoji},
	{AuditWebhookCreated, AuditTargetWebhook},
	{AuditWebhookUpdated, AuditTargetWebhook},
	{AuditWebhookDeleted, AuditTargetWebhook},
	{AuditWebhookEnabled, AuditTargetWebhook},
	{AuditWebhookDisabled, AuditTargetWebhook},
	{AuditWebhookSecretRotated, AuditTargetWebhook},
	{AuditAnnouncementCreated, AuditTargetAnnouncement},
	{AuditAnnouncementUpdated, AuditTargetAnnouncement},
	{AuditAnnouncementPublished, AuditTargetAnnouncement},
	{AuditAnnouncementWithdrawn, AuditTargetAnnouncement},
	{AuditInviteCreated, AuditTargetInvite},
	{AuditInviteRevoked, AuditTargetInvite},
	{AuditRelayAdded, AuditTargetRelay},
	{AuditRelayFollowed, AuditTargetRelay},
	{AuditRelayUnfollowed, AuditTargetRelay},
	{AuditRelayRemoved, AuditTargetRelay},
	{AuditRelayAccepted, AuditTargetRelay},
	{AuditRelayRejected, AuditTargetRelay},
	{AuditStatusRefetchQueued, AuditTargetStatus},
	{AuditAccountStatusesRefetchQueued, AuditTargetAccount},
	{AuditDomainStatusesRefetchQueued, AuditTargetDomain},
	{AuditBillingImportQueued, AuditTargetBillingSource},
	{AuditCostItemCreated, AuditTargetCostItem},
	{AuditCostItemRelabeled, AuditTargetCostItem},
	{AuditCostItemPriceRevised, AuditTargetCostItem},
	{AuditCostItemEndChanged, AuditTargetCostItem},
	{AuditCostItemCanceled, AuditTargetCostItem},
	{AuditSupporterProgramEnabled, AuditTargetSupporterProgram},
	{AuditSupporterProgramDisabled, AuditTargetSupporterProgram},
	{AuditSupporterPlanCreated, AuditTargetSupporterPlan},
	{AuditSupporterPlanRevised, AuditTargetSupporterPlan},
	{AuditSupporterPlanRetired, AuditTargetSupporterPlan},
	{AuditSupporterSubscriptionCanceled, AuditTargetSupporterSubscription},
	{AuditCustomEmojiRequestSubmitted, AuditTargetCustomEmojiRequest},
	{AuditCustomEmojiRequestWithdrawn, AuditTargetCustomEmojiRequest},
	{AuditCustomEmojiRequestApproved, AuditTargetCustomEmojiRequest},
	{AuditCustomEmojiRequestRejected, AuditTargetCustomEmojiRequest},
	{AuditPaymentReconcileQueued, AuditTargetPaymentProvider},
	{AuditOAuthClientDisabled, AuditTargetOAuthClient},
	{AuditOAuthClientEnabled, AuditTargetOAuthClient},
	{AuditOAuthClientHostDenied, AuditTargetOAuthClientHost},
	{AuditOAuthClientHostAllowed, AuditTargetOAuthClientHost},
}

// AuditActions returns every audited operation, in the order of the schema's CHECK.
func AuditActions() []AuditAction {
	out := make([]AuditAction, len(auditActions))
	for i, a := range auditActions {
		out[i] = a.action
	}
	return out
}

// Target returns the kind of target of the action, and false for an unknown action.
func (a AuditAction) Target() (AuditTargetKind, bool) {
	for _, x := range auditActions {
		if x.action == a {
			return x.target, true
		}
	}
	return "", false
}

// Valid reports whether a is one of the audited operations.
func (a AuditAction) Valid() bool {
	_, ok := a.Target()
	return ok
}

// MaxAuditTargetLabel is the number of characters of an entry's target label that are kept.
const MaxAuditTargetLabel = 512

// ErrInvalidAuditEntry reports an audit log entry with a missing or invalid field.
var ErrInvalidAuditEntry = errors.New("moderation: invalid audit log entry")

// AuditEntry is one entry of the audit log (audit_log_entries, an event). It is never updated;
// only the retention cleanup deletes it (ADR-0038).
type AuditEntry struct {
	ID        string
	ActorKind AuditActorKind
	// ActorUserID is the user who performed the operation when ActorKind is AuditActorUser, and
	// "" otherwise.
	ActorUserID string
	Action      AuditAction
	TargetKind  AuditTargetKind
	TargetID    string
	// TargetLabel is the target's display name when the operation happened: a handle such as
	// @alice@example.com, a domain, or a setting's key.
	TargetLabel string
	// SourceEventID is the ID of the row of the source event (the action, the lift, the
	// revision); "" when the operation has no row of its own.
	SourceEventID string
	OccurredAt    time.Time
}

// AuditActor returns the actor of an operation that a moderator performs, given the user ID
// that the use cases take: the user, or the CLI for "".
func AuditActor(userID string) (AuditActorKind, string) {
	if userID == "" {
		return AuditActorCLI, ""
	}
	return AuditActorUser, userID
}

// TruncateAuditLabel returns the first MaxAuditTargetLabel characters of label.
func TruncateAuditLabel(label string) string {
	if utf8.RuneCountInString(label) <= MaxAuditTargetLabel {
		return label
	}
	return string([]rune(label)[:MaxAuditTargetLabel])
}

// Validate checks the invariants of a new entry: a known actor, action, and target of the
// action's kind, a user exactly for a user actor, and texts within their limits.
func (e AuditEntry) Validate() error {
	target, known := e.Action.Target()
	switch {
	case e.ID == "" || e.TargetID == "" || e.OccurredAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidAuditEntry)
	case !e.ActorKind.Valid():
		return fmt.Errorf("%w: actor kind %q", ErrInvalidAuditEntry, e.ActorKind)
	case (e.ActorKind == AuditActorUser) != (e.ActorUserID != ""):
		return fmt.Errorf("%w: a user actor needs a user, and only a user actor has one", ErrInvalidAuditEntry)
	case !known:
		return fmt.Errorf("%w: action %q", ErrInvalidAuditEntry, e.Action)
	case e.TargetKind != target:
		return fmt.Errorf("%w: %s acts on a %s, not a %s", ErrInvalidAuditEntry, e.Action, target, e.TargetKind)
	case utf8.RuneCountInString(e.TargetLabel) > MaxAuditTargetLabel:
		return fmt.Errorf("%w: the target label is longer than %d characters", ErrInvalidAuditEntry, MaxAuditTargetLabel)
	}
	for _, s := range []string{e.TargetID, e.TargetLabel, e.SourceEventID, e.ActorUserID} {
		if !validText(s) {
			return fmt.Errorf("%w: a text is not valid UTF-8 or contains NUL", ErrInvalidAuditEntry)
		}
	}
	return nil
}
