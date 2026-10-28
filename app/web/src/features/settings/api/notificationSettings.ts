// SPDX-License-Identifier: AGPL-3.0-only
// The user's notification settings (FR-210, ADR-0109 §1, §6; web-client/README.md S1): for each row of notification
// types the user gets, whether the notifications are made in the app and sent by email. Web Push stays the choice of
// each subscription (api/push), and the sound the choice of this device (lib/notificationSound).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";

type Schemas = components["schemas"];

export type NotificationSettings = Schemas["NotificationSettings"];
export type NotificationSettingRow = Schemas["NotificationSettingRow"];
export type NotificationGroup = Schemas["NotificationGroup"];
export type NotificationSettingChange = Schemas["NotificationSettingsUpdate"]["changes"][number];

/** The rows the user gets (GET /preferences/notifications), in the order to show them. */
export function notificationSettingsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences/notifications");
}

/**
 * Saves the changes of one cell, or of a column's cells at once (PATCH /preferences/notifications), as the options of
 * TanStack Query's useMutation.
 */
export function updateNotificationSettingMutation(api: Api) {
  return {
    mutationKey: ["patch", "/preferences/notifications"] as const,
    mutationFn: async (changes: readonly NotificationSettingChange[]): Promise<NotificationSettings> => {
      const { data, error } = await api.fetch.PATCH("/preferences/notifications", { body: { changes: [...changes] } });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
  };
}

/** Puts the settings as the server answered a change into the cache (web-client/README.md S1). */
export function cacheNotificationSettings(api: Api, queryClient: QueryClient, saved: NotificationSettings): void {
  queryClient.setQueryData(notificationSettingsQueryOptions(api).queryKey, saved);
}
