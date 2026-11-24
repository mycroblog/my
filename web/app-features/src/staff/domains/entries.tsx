// SPDX-License-Identifier: AGPL-3.0-only

// The entries of federation and records among the staff's screens (entries.tsx; FR-702, FR-704, FR-706,
// FR-716–FR-718, ADR-0058 §1): under Federation, the actions on remote domains (with Mastodon's CSV, the allowlist and
// the block-list subscriptions under them) and the remote servers; under Records, the audit log.

import { loadAuditLog, loadDomainActions, loadRemoteServers } from "@my/app-data";
import type { StaffEntry } from "../entries";
import { actionQueryOf, DomainActionsScreen } from "./actions";
import { auditQueryOf, AuditLogScreen } from "./audit";
import { RemoteServersScreen, serverQueryOf } from "./servers";

/** The groups these entries add to the root of Moderation. */
export const domainGroups = [
  { id: "federation", area: "moderation", heading: "staff_domains.group.federation" },
  { id: "records", area: "moderation", heading: "staff_audit.group.records" },
] as const;

/** The entries of federation and records, in the root's order within their groups. */
export const domainEntries: readonly StaffEntry[] = [
  {
    // The actions on remote domains (FR-702), with Mastodon's CSV (FR-706) and the allowlist and the block-list
    // subscriptions (ADR-0159) under them.
    path: "domain-actions",
    area: "moderation",
    group: "federation",
    permission: "manage_domains",
    icon: "block",
    name: "staff_domains.entry.domain_actions",
    Screen: DomainActionsScreen,
    load: (data, search) => loadDomainActions(data, actionQueryOf(search)),
  },
  {
    // Every remote server this one knows, with its software and the health of deliveries to it (ADR-0006 §3), and
    // any server's page.
    path: "instances",
    area: "moderation",
    group: "federation",
    permission: "manage_domains",
    icon: "federated",
    name: "staff_domains.entry.instances",
    Screen: RemoteServersScreen,
    load: (data, search) => loadRemoteServers(data, serverQueryOf(search)),
  },
  {
    // The operations of the moderators and the administrators (FR-704).
    path: "audit-log",
    area: "moderation",
    group: "records",
    permission: "view_audit",
    icon: "history",
    name: "staff_audit.entry.audit_log",
    Screen: AuditLogScreen,
    load: (data, search) => loadAuditLog(data, auditQueryOf(search)),
  },
];
