// SPDX-License-Identifier: AGPL-3.0-only
// What the viewer's roles let them reach, for the places of the navigation (ADR-0058 §2, §4, ADR-0070 §1): moderation
// for a permission of its area, admin for a permission of the server's administration, and finance for reading the
// instance's costs; and the count of what awaits the moderators (FR-712, NFR-MOD-4) on Moderation. The server checks
// every operation's permission itself: these only choose what the navigation shows.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { viewerQuery } from "./accounts";
import { useAppData } from "./provider";

/** A permission that a role grants. */
export type Permission = components["schemas"]["Permission"];

/** The places of the navigation the viewer's permissions open. */
export type Rights = { readonly moderation: boolean; readonly admin: boolean; readonly finance: boolean };

/**
 * The permissions of the moderation area (ADR-0058 §4, the server's `identity.CategoryModeration`), whose screens
 * (/moderation/invites among them) the navigation's Moderation leads to.
 */
const moderationPermissions: ReadonlySet<Permission> = new Set([
  "manage_reports",
  "manage_accounts",
  "manage_domains",
  "manage_invites",
  "view_audit",
]);

/**
 * The permissions that belong to neither moderation nor the server's administration (ADR-0070 §1), the server's
 * `identity.CategoryMember`; every permission of neither set is the server's administration, as the server says of a
 * permission of no known area.
 */
const memberPermissions: ReadonlySet<Permission> = new Set([
  "view_finance",
  "create_custom_emojis",
  "start_voice_rooms",
  "create_invites",
  "keep_posts_longer",
  "create_articles",
  "create_events",
]);

/** The places that `permissions` open. */
export function rightsOf(permissions: readonly Permission[]): Rights {
  return {
    moderation: permissions.some((p) => moderationPermissions.has(p)),
    admin: permissions.some((p) => !moderationPermissions.has(p) && !memberPermissions.has(p)),
    finance: permissions.includes("view_finance") || permissions.includes("manage_finance"),
  };
}

const none: Rights = { moderation: false, admin: false, finance: false };

/**
 * The places the viewer's permissions open; none until the server answered, and none fetched while `enabled` is false
 * (a reader without a session).
 */
export function useRights(enabled = true): Rights {
  const viewer = useQuery({ ...viewerQuery(useAppData()), enabled }).data;
  return !enabled || viewer === undefined ? none : rightsOf(viewer.permissions);
}

/** What awaits the moderators that the viewer may see. */
export function moderationCountsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/counts");
}

/**
 * How many things await the viewer as a moderator: the open reports, the registrations to approve and the appeals to
 * decide, of those their permissions show; undefined without the rights of moderation or until the server answered, and
 * nothing fetched while `enabled` is false (a reader without a session).
 */
export function useModerationCount(enabled = true): number | undefined {
  const data = useAppData();
  const { moderation } = useRights(enabled);
  const counts = useQuery({ ...moderationCountsQuery(data), enabled: moderation }).data;
  if (!moderation || counts === undefined) return undefined;
  return (counts.open_reports ?? 0) + (counts.pending_registrations ?? 0) + (counts.pending_appeals ?? 0);
}
