// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { credentialsQueryOptions, instanceQueryOptions, type Permission } from "../api/me";
import type { PreferenceControl } from "./usePreferences";

/**
 * The permissions whose holders get moderator notifications (admin_report, admin_sign_up, admin_appeal,
 * admin_webhook_disabled for manage_webhooks, and admin_custom_emoji_request for manage_custom_emojis).
 */
const moderatorPermissions: readonly Permission[] = [
  "manage_reports",
  "manage_accounts",
  "manage_webhooks",
  "manage_custom_emojis",
];

/** What the preferences show of `email_moderator_notifications`. */
export type ModeratorEmail =
  /** The viewer's roles get no moderator notifications. */
  | { status: "hidden" }
  /** The server sends no email (mail = none), so the preference would change nothing. */
  | { status: "no-email" }
  | { status: "available"; control: PreferenceControl<boolean> };

/**
 * Returns whether and how the preferences offer the moderator notifications by email, on a page whose loader has
 * loaded the viewer's account (it suspends until then). Only a viewer whose roles get moderator notifications
 * sees it; the instance's `sends_email` tells whether the server sends email at all. The instance is the app
 * shell's query: while it is not known, the switch shows, since the server keeps the preference either way.
 */
export function useModeratorEmail(control: PreferenceControl<boolean>): ModeratorEmail {
  const api = useApi();
  const account = useSuspenseQuery(credentialsQueryOptions(api));
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  if (!account.data.permissions.some((p) => moderatorPermissions.includes(p))) return { status: "hidden" };
  if (instance.data?.sends_email === false) return { status: "no-email" };
  return { status: "available", control };
}
