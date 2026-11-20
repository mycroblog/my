// SPDX-License-Identifier: AGPL-3.0-only
// The audit log as a holder of `view_audit` reads it (FR-704, ADR-0058 §7): the operations of the moderators and the
// administrators, newest first, of one local account's user, of one kind, and on one kind of target or one target, a
// page at a time by the server's cursor. The log cannot be changed: it is only read.
import { type components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { pageOf, type StaffPage } from "./staff-reads";

/** One operation of a moderator or an administrator. */
export type AuditEntry = components["schemas"]["AdminAuditEntry"];
/** The kind of an audited operation. */
export type AuditAction = components["schemas"]["AdminAuditAction"];
/** The kind of what an audited operation acted on. */
export type AuditTargetKind = components["schemas"]["AdminAuditTargetKind"];
/** Who performed an operation: a user, the command line, the server itself, or another server. */
export type AuditActorKind = components["schemas"]["AdminAuditActorKind"];

/** What the audit log shows: the operations of one actor, of one kind, on one kind of target or one target. */
export type AuditQuery = {
  /** The local account of the user who performed them. */
  readonly actor?: string | undefined;
  readonly action?: AuditAction | undefined;
  readonly targetKind?: AuditTargetKind | undefined;
  /** One target, of the kind `targetKind`, which it needs. */
  readonly target?: string | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the audit log `q` asks for. */
export function auditLogQuery({ queries }: AppData, q: AuditQuery) {
  return queries.queryOptions("get", "/admin/audit_log", {
    params: {
      query: {
        ...(q.actor === undefined ? {} : { actor_account_id: q.actor }),
        ...(q.action === undefined ? {} : { action: q.action }),
        ...(q.targetKind === undefined ? {} : { target_kind: q.targetKind }),
        ...(q.targetKind === undefined || q.target === undefined ? {} : { target_id: q.target }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** The page of the audit log `q` asks for; the page shown before stays while another is on its way (`stale`). */
export function useAuditLog(q: AuditQuery): StaffPage<AuditEntry> {
  return pageOf(useQuery({ ...auditLogQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** Reads the page of the audit log `q` asks for, for a route's loader. */
export async function loadAuditLog(data: AppData, q: AuditQuery): Promise<void> {
  await data.queryClient.query(auditLogQuery(data, q)).catch(() => undefined);
}

/** The kinds of target, in the server's order. */
export const auditTargetKinds = [
  "account",
  "status",
  "domain",
  "report",
  "appeal",
  "user",
  "role",
  "setting",
  "rule",
  "signup_block",
  "custom_emoji",
  "webhook",
  "announcement",
  "invite",
  "relay",
  "billing_source",
  "cost_item",
  "supporter_program",
  "supporter_plan",
  "supporter_subscription",
  "custom_emoji_request",
  "payment_provider",
  "oauth_client",
  "oauth_client_host",
  "instance",
  "voice_room",
  "login_provider",
  "link_card_domain",
  "hook_decision",
  "domain_allow",
  "domain_list_subscription",
  "event",
] as const satisfies readonly AuditTargetKind[];

/**
 * The kind of target of each kind of operation, as the API describes `AdminAuditAction`, in the server's order: the
 * filter of the kinds of operation offers those of the kind of target chosen.
 */
export const auditActionTargets = {
  account_action_issued: "account",
  account_action_lifted: "account",
  domain_action_issued: "domain",
  domain_action_lifted: "domain",
  report_assigned: "report",
  report_unassigned: "report",
  report_resolved: "report",
  report_reopened: "report",
  report_note_added: "report",
  report_forwarded: "report",
  registration_approved: "user",
  registration_rejected: "user",
  setting_changed: "setting",
  text_changed: "setting",
  rule_added: "rule",
  rule_revised: "rule",
  rule_moved: "rule",
  rule_retired: "rule",
  role_granted: "role",
  role_revoked: "role",
  role_created: "role",
  role_updated: "role",
  role_deleted: "role",
  appeal_upheld: "appeal",
  appeal_overturned: "appeal",
  account_deletion_canceled: "account",
  revealed_private_data: "account",
  signup_block_added: "signup_block",
  signup_block_lifted: "signup_block",
  hook_decision_applied: "hook_decision",
  hook_decision_overridden: "hook_decision",
  hook_appeal_upheld: "hook_decision",
  two_factor_disabled: "account",
  password_reset_link_issued: "account",
  email_suppression_lifted: "account",
  account_moved: "account",
  custom_emoji_created: "custom_emoji",
  custom_emoji_image_replaced: "custom_emoji",
  custom_emoji_copied: "custom_emoji",
  custom_emoji_updated: "custom_emoji",
  custom_emoji_enabled: "custom_emoji",
  custom_emoji_disabled: "custom_emoji",
  custom_emoji_deleted: "custom_emoji",
  webhook_created: "webhook",
  webhook_updated: "webhook",
  webhook_deleted: "webhook",
  webhook_enabled: "webhook",
  webhook_disabled: "webhook",
  webhook_secret_rotated: "webhook",
  announcement_created: "announcement",
  announcement_updated: "announcement",
  announcement_published: "announcement",
  announcement_withdrawn: "announcement",
  invite_created: "invite",
  invite_revoked: "invite",
  relay_added: "relay",
  relay_followed: "relay",
  relay_unfollowed: "relay",
  relay_removed: "relay",
  relay_accepted: "relay",
  relay_rejected: "relay",
  status_refetch_queued: "status",
  account_statuses_refetch_queued: "account",
  domain_statuses_refetch_queued: "domain",
  billing_import_queued: "billing_source",
  cost_item_created: "cost_item",
  cost_item_relabeled: "cost_item",
  cost_item_price_revised: "cost_item",
  cost_item_end_changed: "cost_item",
  cost_item_deleted: "cost_item",
  supporter_program_enabled: "supporter_program",
  supporter_program_disabled: "supporter_program",
  supporter_tips_enabled: "supporter_program",
  supporter_tips_revised: "supporter_program",
  supporter_tips_disabled: "supporter_program",
  supporter_plan_created: "supporter_plan",
  supporter_plan_revised: "supporter_plan",
  supporter_plan_retired: "supporter_plan",
  supporter_subscription_ended: "supporter_subscription",
  custom_emoji_request_submitted: "custom_emoji_request",
  custom_emoji_request_withdrawn: "custom_emoji_request",
  custom_emoji_request_approved: "custom_emoji_request",
  custom_emoji_request_rejected: "custom_emoji_request",
  payment_reconcile_queued: "payment_provider",
  oauth_client_disabled: "oauth_client",
  oauth_client_enabled: "oauth_client",
  oauth_client_host_denied: "oauth_client_host",
  oauth_client_host_allowed: "oauth_client_host",
  service_closure_announced: "instance",
  service_closure_withdrawn: "instance",
  service_closure_closed: "instance",
  service_closure_delivery_completed: "instance",
  voice_room_ended: "voice_room",
  login_provider_created: "login_provider",
  login_provider_updated: "login_provider",
  login_provider_deleted: "login_provider",
  login_provider_enabled: "login_provider",
  login_provider_disabled: "login_provider",
  login_provider_secret_rotated: "login_provider",
  link_card_domain_fetch_stopped: "link_card_domain",
  link_card_domain_images_stopped: "link_card_domain",
  link_card_domain_rule_removed: "link_card_domain",
  instance_marked_available: "domain",
  delivery_revealed: "domain",
  account_deliveries_viewed: "account",
  domain_allow_added: "domain_allow",
  domain_allow_removed: "domain_allow",
  domain_list_subscription_created: "domain_list_subscription",
  domain_list_subscription_updated: "domain_list_subscription",
  domain_list_subscription_deleted: "domain_list_subscription",
  domain_list_draft_accepted: "domain_list_subscription",
  domain_list_draft_rejected: "domain_list_subscription",
  domain_list_fetch_queued: "domain_list_subscription",
  event_cancelled: "event",
} as const satisfies Readonly<Record<AuditAction, AuditTargetKind>>;

/** Every kind of operation, in the server's order. */
export const auditActions = Object.keys(auditActionTargets) as AuditAction[];
