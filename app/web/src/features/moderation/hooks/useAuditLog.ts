// SPDX-License-Identifier: AGPL-3.0-only
// The audit log (FR-704): the operations under a filter, newest first, and loading more of them. The route keeps
// the filter and the number of loaded pages in the address (web-client/README.md S3), so that a reload or a shared link
// shows the same entries; the list tells it how many pages it has after each one it loads.
import { useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey } from "../../../lib/i18n";
import {
  type AuditAction,
  type AuditActorKind,
  type AuditEntry,
  type AuditLogFilter,
  auditLogQueryOptions,
  type AuditTargetKind,
} from "../api/staff";
import type { PagedPages } from "./pagedList";

export const auditActionLabels: Readonly<Record<AuditAction, ClientMessageKey>> = {
  account_action_issued: "client:admin.audit_log.action.account_action_issued",
  account_action_lifted: "client:admin.audit_log.action.account_action_lifted",
  revealed_private_data: "client:admin.audit_log.action.revealed_private_data",
  two_factor_disabled: "client:admin.audit_log.action.two_factor_disabled",
  password_reset_link_issued: "client:admin.audit_log.action.password_reset_link_issued",
  email_suppression_lifted: "client:admin.audit_log.action.email_suppression_lifted",
  account_moved: "client:admin.audit_log.action.account_moved",
  domain_action_issued: "client:admin.audit_log.action.domain_action_issued",
  domain_action_lifted: "client:admin.audit_log.action.domain_action_lifted",
  report_assigned: "client:admin.audit_log.action.report_assigned",
  report_unassigned: "client:admin.audit_log.action.report_unassigned",
  report_resolved: "client:admin.audit_log.action.report_resolved",
  report_reopened: "client:admin.audit_log.action.report_reopened",
  report_note_added: "client:admin.audit_log.action.report_note_added",
  report_forwarded: "client:admin.audit_log.action.report_forwarded",
  registration_approved: "client:admin.audit_log.action.registration_approved",
  registration_rejected: "client:admin.audit_log.action.registration_rejected",
  setting_changed: "client:admin.audit_log.action.setting_changed",
  text_changed: "client:admin.audit_log.action.text_changed",
  rule_added: "client:admin.audit_log.action.rule_added",
  rule_revised: "client:admin.audit_log.action.rule_revised",
  rule_moved: "client:admin.audit_log.action.rule_moved",
  rule_retired: "client:admin.audit_log.action.rule_retired",
  role_granted: "client:admin.audit_log.action.role_granted",
  role_revoked: "client:admin.audit_log.action.role_revoked",
  role_created: "client:admin.audit_log.action.role_created",
  role_updated: "client:admin.audit_log.action.role_updated",
  role_deleted: "client:admin.audit_log.action.role_deleted",
  appeal_upheld: "client:admin.audit_log.action.appeal_upheld",
  appeal_overturned: "client:admin.audit_log.action.appeal_overturned",
  account_deletion_canceled: "client:admin.audit_log.action.account_deletion_canceled",
  signup_block_added: "client:admin.audit_log.action.signup_block_added",
  signup_block_lifted: "client:admin.audit_log.action.signup_block_lifted",
  hook_decision_applied: "client:admin.audit_log.action.hook_decision_applied",
  hook_decision_overridden: "client:admin.audit_log.action.hook_decision_overridden",
  hook_appeal_upheld: "client:admin.audit_log.action.hook_appeal_upheld",
  custom_emoji_created: "client:admin.audit_log.action.custom_emoji_created",
  custom_emoji_image_replaced: "client:admin.audit_log.action.custom_emoji_image_replaced",
  custom_emoji_copied: "client:admin.audit_log.action.custom_emoji_copied",
  custom_emoji_updated: "client:admin.audit_log.action.custom_emoji_updated",
  custom_emoji_enabled: "client:admin.audit_log.action.custom_emoji_enabled",
  custom_emoji_disabled: "client:admin.audit_log.action.custom_emoji_disabled",
  custom_emoji_deleted: "client:admin.audit_log.action.custom_emoji_deleted",
  webhook_created: "client:admin.audit_log.action.webhook_created",
  webhook_updated: "client:admin.audit_log.action.webhook_updated",
  webhook_deleted: "client:admin.audit_log.action.webhook_deleted",
  webhook_enabled: "client:admin.audit_log.action.webhook_enabled",
  webhook_disabled: "client:admin.audit_log.action.webhook_disabled",
  webhook_secret_rotated: "client:admin.audit_log.action.webhook_secret_rotated",
  announcement_created: "client:admin.audit_log.action.announcement_created",
  announcement_updated: "client:admin.audit_log.action.announcement_updated",
  announcement_published: "client:admin.audit_log.action.announcement_published",
  announcement_withdrawn: "client:admin.audit_log.action.announcement_withdrawn",
  invite_created: "client:admin.audit_log.action.invite_created",
  invite_revoked: "client:admin.audit_log.action.invite_revoked",
  relay_added: "client:admin.audit_log.action.relay_added",
  relay_followed: "client:admin.audit_log.action.relay_followed",
  relay_unfollowed: "client:admin.audit_log.action.relay_unfollowed",
  relay_removed: "client:admin.audit_log.action.relay_removed",
  relay_accepted: "client:admin.audit_log.action.relay_accepted",
  relay_rejected: "client:admin.audit_log.action.relay_rejected",
  status_refetch_queued: "client:admin.audit_log.action.status_refetch_queued",
  account_statuses_refetch_queued: "client:admin.audit_log.action.account_statuses_refetch_queued",
  domain_statuses_refetch_queued: "client:admin.audit_log.action.domain_statuses_refetch_queued",
  billing_import_queued: "client:admin.audit_log.action.billing_import_queued",
  cost_item_created: "client:admin.audit_log.action.cost_item_created",
  cost_item_relabeled: "client:admin.audit_log.action.cost_item_relabeled",
  cost_item_price_revised: "client:admin.audit_log.action.cost_item_price_revised",
  cost_item_end_changed: "client:admin.audit_log.action.cost_item_end_changed",
  cost_item_deleted: "client:admin.audit_log.action.cost_item_deleted",
  supporter_program_enabled: "client:admin.audit_log.action.supporter_program_enabled",
  supporter_program_disabled: "client:admin.audit_log.action.supporter_program_disabled",
  supporter_plan_created: "client:admin.audit_log.action.supporter_plan_created",
  supporter_plan_revised: "client:admin.audit_log.action.supporter_plan_revised",
  supporter_plan_retired: "client:admin.audit_log.action.supporter_plan_retired",
  supporter_subscription_ended: "client:admin.audit_log.action.supporter_subscription_ended",
  custom_emoji_request_submitted: "client:admin.audit_log.action.custom_emoji_request_submitted",
  custom_emoji_request_withdrawn: "client:admin.audit_log.action.custom_emoji_request_withdrawn",
  custom_emoji_request_approved: "client:admin.audit_log.action.custom_emoji_request_approved",
  custom_emoji_request_rejected: "client:admin.audit_log.action.custom_emoji_request_rejected",
  payment_reconcile_queued: "client:admin.audit_log.action.payment_reconcile_queued",
  oauth_client_disabled: "client:admin.audit_log.action.oauth_client_disabled",
  oauth_client_enabled: "client:admin.audit_log.action.oauth_client_enabled",
  oauth_client_host_denied: "client:admin.audit_log.action.oauth_client_host_denied",
  oauth_client_host_allowed: "client:admin.audit_log.action.oauth_client_host_allowed",
  service_closure_announced: "client:admin.audit_log.action.service_closure_announced",
  service_closure_withdrawn: "client:admin.audit_log.action.service_closure_withdrawn",
  service_closure_closed: "client:admin.audit_log.action.service_closure_closed",
  service_closure_delivery_completed: "client:admin.audit_log.action.service_closure_delivery_completed",
  voice_room_ended: "client:admin.audit_log.action.voice_room_ended",
  login_provider_created: "client:admin.audit_log.action.login_provider_created",
  login_provider_updated: "client:admin.audit_log.action.login_provider_updated",
  login_provider_deleted: "client:admin.audit_log.action.login_provider_deleted",
  login_provider_enabled: "client:admin.audit_log.action.login_provider_enabled",
  login_provider_disabled: "client:admin.audit_log.action.login_provider_disabled",
  login_provider_secret_rotated: "client:admin.audit_log.action.login_provider_secret_rotated",
  link_card_domain_fetch_stopped: "client:admin.audit_log.action.link_card_domain_fetch_stopped",
  link_card_domain_images_stopped: "client:admin.audit_log.action.link_card_domain_images_stopped",
  link_card_domain_rule_removed: "client:admin.audit_log.action.link_card_domain_rule_removed",
  instance_marked_available: "client:admin.audit_log.action.instance_marked_available",
  domain_allow_added: "client:admin.audit_log.action.domain_allow_added",
  domain_allow_removed: "client:admin.audit_log.action.domain_allow_removed",
  domain_list_subscription_created: "client:admin.audit_log.action.domain_list_subscription_created",
  domain_list_subscription_updated: "client:admin.audit_log.action.domain_list_subscription_updated",
  domain_list_subscription_deleted: "client:admin.audit_log.action.domain_list_subscription_deleted",
  domain_list_draft_accepted: "client:admin.audit_log.action.domain_list_draft_accepted",
  domain_list_draft_rejected: "client:admin.audit_log.action.domain_list_draft_rejected",
  domain_list_fetch_queued: "client:admin.audit_log.action.domain_list_fetch_queued",
};

export const auditTargetKindLabels: Readonly<Record<AuditTargetKind, ClientMessageKey>> = {
  account: "client:admin.audit_log.target_kind.account",
  status: "client:admin.audit_log.target_kind.status",
  domain: "client:admin.audit_log.target_kind.domain",
  report: "client:admin.audit_log.target_kind.report",
  appeal: "client:admin.audit_log.target_kind.appeal",
  user: "client:admin.audit_log.target_kind.user",
  role: "client:admin.audit_log.target_kind.role",
  setting: "client:admin.audit_log.target_kind.setting",
  rule: "client:admin.audit_log.target_kind.rule",
  signup_block: "client:admin.audit_log.target_kind.signup_block",
  hook_decision: "client:admin.audit_log.target_kind.hook_decision",
  custom_emoji: "client:admin.audit_log.target_kind.custom_emoji",
  webhook: "client:admin.audit_log.target_kind.webhook",
  announcement: "client:admin.audit_log.target_kind.announcement",
  invite: "client:admin.audit_log.target_kind.invite",
  relay: "client:admin.audit_log.target_kind.relay",
  billing_source: "client:admin.audit_log.target_kind.billing_source",
  cost_item: "client:admin.audit_log.target_kind.cost_item",
  supporter_program: "client:admin.audit_log.target_kind.supporter_program",
  supporter_plan: "client:admin.audit_log.target_kind.supporter_plan",
  supporter_subscription: "client:admin.audit_log.target_kind.supporter_subscription",
  custom_emoji_request: "client:admin.audit_log.target_kind.custom_emoji_request",
  payment_provider: "client:admin.audit_log.target_kind.payment_provider",
  oauth_client: "client:admin.audit_log.target_kind.oauth_client",
  oauth_client_host: "client:admin.audit_log.target_kind.oauth_client_host",
  instance: "client:admin.audit_log.target_kind.instance",
  voice_room: "client:admin.audit_log.target_kind.voice_room",
  login_provider: "client:admin.audit_log.target_kind.login_provider",
  link_card_domain: "client:admin.audit_log.target_kind.link_card_domain",
  domain_allow: "client:admin.audit_log.target_kind.domain_allow",
  domain_list_subscription: "client:admin.audit_log.target_kind.domain_list_subscription",
};

/** Who performed an operation without a user's account to show. */
export const auditActorKindLabels: Readonly<Record<AuditActorKind, ClientMessageKey>> = {
  user: "client:admin.audit_log.actor.deleted_user",
  cli: "client:admin.audit_log.actor.cli",
  system: "client:admin.audit_log.actor.system",
  remote: "client:admin.audit_log.actor.remote",
};

/**
 * The admin screen of an operation's target, where it has one; an account, a report, a domain, a sign-up block, a
 * role, a custom emoji, a webhook, an announcement, an invite, a relay, a billing source or a cost item.
 * The other kinds of target (settings, rules, registrations) have no screen of their own, and a target that is
 * gone (erased past its retention, or deleted) has nothing left to show, so their entries show the label only.
 */
export function auditTargetHref(
  entry: Pick<AuditEntry, "target_kind" | "target_id" | "target_label" | "target_exists">,
): string | undefined {
  if (!entry.target_exists) return undefined;
  const id = encodeURIComponent(entry.target_id);
  switch (entry.target_kind) {
    case "account":
      return `/moderation/accounts/${id}`;
    case "report":
      return `/moderation/reports/${id}`;
    case "domain":
      // Every action on the domain, lifted ones too: the entry may be about one that is no longer in effect.
      return `/moderation/domain-actions?filter=all&domain=${id}`;
    case "hook_decision":
      // A decision has no screen of its own, and its record may have passed its retention.
      return "/moderation/hook-decisions";
    case "signup_block":
      return `/moderation/signup-blocks/${id}`;
    case "custom_emoji":
      return `/admin/custom-emojis/${id}`;
    case "role":
      // A role has no screen of its own: the list of roles shows it.
      return "/admin/roles";
    case "webhook":
      return `/admin/webhooks/${id}`;
    case "announcement":
      return `/admin/announcements/${id}`;
    case "invite":
      // An invite has no screen of its own: the list of invites shows it.
      return "/moderation/invites";
    case "relay":
      // The list, which a deleted relay has left; a relay has no screen of its own.
      return "/admin/relays";
    case "billing_source":
      // The billing imports page of the costs screen shows the state of every billing source.
      return "/admin/finance/imports";
    case "cost_item":
      return `/admin/finance/costs/${id}`;
    case "supporter_plan":
      // Every plan has its page, retired ones too.
      return `/admin/supporters/plans/${id}`;
    case "supporter_subscription":
      return `/admin/supporters/subscriptions/${id}`;
    case "supporter_program":
    case "payment_provider":
      // The first page of the supporters screen shows the program and the state of the payment provider.
      return "/admin/supporters";
    case "login_provider":
      // A provider has no screen of its own: the list of providers shows it.
      return "/admin/login-providers";
    case "link_card_domain":
      // A rule has no screen of its own: the list of rules, which a removed rule has left.
      return "/admin/link-card-domains";
    case "custom_emoji_request":
      // A request has no screen of its own: the requests page of the custom emoji screen, in every state.
      return "/admin/custom-emojis/requests";
    default:
      return undefined;
  }
}

export type AuditLog = PagedPages<AuditEntry> & {
  /** Why the last page failed to load, localized; the list keeps what it has and "show more" tries again. */
  loadMoreError: string | undefined;
  /** The account of the actor of the filter, from the loaded entries; undefined when none shows it. */
  actorAccount: AuditEntry["actor_account"];
  /** The label of the target of the filter, from the loaded entries; undefined when none shows it. */
  targetLabel: string | undefined;
};

/**
 * The audit log under `filter`, newest first. `onPagesLoaded(pages)` is called with the number of loaded pages
 * after loading more, for the route to keep in the address.
 */
export function useAuditLog(filter: AuditLogFilter, onPagesLoaded: (pages: number) => void): AuditLog {
  const api = useApi();
  const query = useSuspenseInfiniteQuery(auditLogQueryOptions(api, filter));
  const items = query.data.pages.flatMap((page) => page.items);
  const actorEntry =
    filter.actor === undefined
      ? undefined
      : items.find((entry) => entry.actor_account_id === filter.actor && entry.actor_account !== undefined);
  const targetEntry =
    filter.target === undefined
      ? undefined
      : items.find((entry) => entry.target_kind === filter.targetKind && entry.target_id === filter.target);
  return {
    items,
    pages: query.data.pages.map((page) => page.items).filter((entries) => entries.length > 0),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A fetch in progress would be overwritten by the page fetched now (TanStack Query, "Infinite Queries").
      if (query.isFetching) return;
      void query.fetchNextPage().then((result) => {
        const pages = result.data?.pages.length;
        if (!result.isFetchNextPageError && pages !== undefined) onPagesLoaded(pages);
      });
    },
    loadMoreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    actorAccount: actorEntry?.actor_account,
    targetLabel: targetEntry?.target_label,
  };
}
