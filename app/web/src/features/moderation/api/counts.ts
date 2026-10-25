// SPDX-License-Identifier: AGPL-3.0-only
// What awaits the moderators (FR-712): the open reports, the registrations awaiting approval, and the appeals
// awaiting a decision (FR-707), for the
// badges of the navigation's admin entry and of the admin menu. The server leaves out a count whose
// permission the viewer's roles lack.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import type { Permission } from "./staff";

export type AdminCounts = components["schemas"]["AdminCounts"];

/** A count of AdminCounts. */
export type AdminCount = keyof AdminCounts;

/** The permission that each count needs; a viewer with none of them has nothing to count. */
export const countPermissions: Readonly<Record<AdminCount, Permission>> = {
  open_reports: "manage_reports",
  pending_registrations: "manage_accounts",
  pending_appeals: "manage_accounts",
};

/**
 * The moderators' counts (GET /admin/counts). Its key is the client's own `["get", "/admin/counts", …]`, which a
 * decision on a registration invalidates (isAdminCountsQuery).
 */
export function adminCountsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/counts");
}

/** The notifications that tell a moderator of one more thing to count: a new report, registration, or appeal. */
const countedNotifications: ReadonlySet<string> = new Set(["admin_report", "admin_sign_up", "admin_appeal"]);

/**
 * What the streaming API changes in the counts (web-client/README.md S10): a notification of a new report or
 * registration, or a reconnect that may have missed one, reads them again.
 */
export function refreshAdminCounts(queryClient: QueryClient, api: Api, notificationType?: string): void {
  if (notificationType !== undefined && !countedNotifications.has(notificationType)) return;
  void queryClient.invalidateQueries({ queryKey: adminCountsQueryOptions(api).queryKey });
}
