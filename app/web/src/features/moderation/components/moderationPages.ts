// SPDX-License-Identifier: AGPL-3.0-only
// The moderation screens of ADR-0058, which the moderation menus list and whose permissions decide the
// navigation's Moderation entry.
import type { StaffPage } from "./StaffPages";

/** The moderation screens, in the order of the menu, with the permission each needs. */
export const moderationPages: readonly StaffPage[] = [
  {
    href: "/moderation/reports",
    permission: "manage_reports",
    label: "client:admin.reports.title",
    description: "client:admin.reports.summary",
  },
  {
    href: "/moderation/appeals",
    permission: "manage_accounts",
    label: "client:admin.appeals.title",
    description: "client:admin.appeals.summary",
  },
  {
    href: "/moderation/accounts",
    permission: "manage_accounts",
    label: "client:admin.accounts.title",
    description: "client:admin.accounts.summary",
  },
  {
    href: "/moderation/registrations",
    permission: "manage_accounts",
    label: "client:admin.registrations.title",
    description: "client:admin.registrations.summary",
  },
  {
    href: "/moderation/signup-blocks",
    permission: "manage_accounts",
    label: "client:admin.signup_blocks.title",
    description: "client:admin.signup_blocks.summary",
  },
  {
    href: "/moderation/invites",
    permission: "manage_invites",
    label: "client:admin.invites.title",
    description: "client:admin.invites.summary",
  },
  {
    href: "/moderation/domain-actions",
    permission: "manage_domains",
    label: "client:admin.domain_actions.title",
    description: "client:admin.domain_actions.summary",
  },
  {
    href: "/moderation/instances",
    permission: "manage_domains",
    label: "client:admin.instances.title",
    description: "client:admin.instances.summary",
  },
  {
    href: "/moderation/audit-log",
    permission: "view_audit",
    label: "client:admin.audit_log.title",
    description: "client:admin.audit_log.summary",
  },
];
