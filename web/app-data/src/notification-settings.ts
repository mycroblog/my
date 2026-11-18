// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's choices of which notifications they get (FR-206, ADR-0086): the cells of their notification settings,
// per kind and channel, and the policy that holds back or drops the notifications of accounts that do not know them.
// The rows are read in push.ts, with the Web Push subscription and the sound of this device; a write puts what the
// server answered into the cache, and a new policy marks the held-back notifications.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { notificationSettingsQuery } from "./push";
import { useAppData } from "./provider";

/** The rows of the viewer's notification settings. */
export type NotificationSettings = components["schemas"]["NotificationSettings"];

/** A row of the settings: a kind of notifications, and whether the viewer gets it in the app and by email. */
export type NotificationSettingRow = components["schemas"]["NotificationSettingRow"];

/** The kind of a row of the settings. */
export type NotificationGroup = components["schemas"]["NotificationGroup"];

/** A channel a cell of the settings can change: the app's own notifications, or email. */
export type NotificationChannel = "in_app" | "email";

/** A cell to change: whether the viewer gets the notifications of a row through a channel. */
export type NotificationCellChange = {
  readonly group: NotificationGroup;
  readonly channel: NotificationChannel;
  readonly enabled: boolean;
};

/**
 * Changes the cells of `changes` (the others keep their value), and resolves once the cache holds the settings as the
 * server answered. It rejects with the server's problem (a cell that is fixed, a row the viewer does not get).
 */
export async function changeNotificationSettings(
  data: AppData,
  changes: readonly NotificationCellChange[],
): Promise<void> {
  const { data: saved } = await data.api.PATCH("/preferences/notifications", { body: { changes: [...changes] } });
  if (saved !== undefined) data.queryClient.setQueryData(notificationSettingsQuery(data).queryKey, saved);
}

/** Changes cells of the notification settings, from a screen (`changeNotificationSettings`). */
export function useChangeNotificationSettings(): (changes: readonly NotificationCellChange[]) => Promise<void> {
  const data = useAppData();
  return (changes) => changeNotificationSettings(data, changes);
}

// ---- The policy ----------------------------------------------------------------------------------------------------

/** What the viewer's policy does with the notifications of a source: let them through, hold them back, or drop them. */
export type NotificationPolicyAction = components["schemas"]["NotificationPolicyAction"];

/** The policy: its action for each kind of source. */
export type NotificationPolicy = components["schemas"]["NotificationPolicy"];

/** The viewer's notification policy. */
export function notificationPolicyQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/notification_policy");
}

/** The viewer's notification policy, undefined until the server answered. */
export function useNotificationPolicy(): NotificationPolicy | undefined {
  return useQuery(notificationPolicyQuery(useAppData())).data;
}

/**
 * Changes the actions of `change` (the others stay), and resolves once the cache holds the policy as the server
 * answered and the notifications it holds back, their summary, and the notifications are read again.
 * It rejects with the server's problem.
 */
export async function changeNotificationPolicy(data: AppData, change: Partial<NotificationPolicy>): Promise<void> {
  const { queryClient } = data;
  const { data: saved } = await data.api.PATCH("/preferences/notification_policy", { body: change });
  if (saved !== undefined) queryClient.setQueryData(notificationPolicyQuery(data).queryKey, saved);
  await queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      typeof queryKey[1] === "string" &&
      (queryKey[1] === "/notifications" || queryKey[1].startsWith("/notifications/requests")),
  });
}

/** Changes the notification policy, from a screen (`changeNotificationPolicy`). */
export function useChangeNotificationPolicy(): (change: Partial<NotificationPolicy>) => Promise<void> {
  const data = useAppData();
  return (change) => changeNotificationPolicy(data, change);
}
