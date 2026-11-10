// SPDX-License-Identifier: AGPL-3.0-only
// The user's notification policy (ADR-0158 §1 to §3, web-client/README.md S1): for each condition on the source of a
// notification, whether its notifications are accepted, held in the source's request, or dropped.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";

export type NotificationPolicy = components["schemas"]["NotificationPolicy"];
export type NotificationPolicyAction = components["schemas"]["NotificationPolicyAction"];

/** A condition on the source of a notification, a field of the policy. */
export type NotificationPolicyCondition = keyof NotificationPolicy;

/** The conditions in the order the settings list them, the closest to the user first. */
export const notificationPolicyConditions = [
  "not_following",
  "not_follower",
  "new_account",
  "unsolicited_private_mention",
  "limited_account",
] as const satisfies readonly NotificationPolicyCondition[];

/** The actions in the order the settings offer them, the mildest first. */
export const notificationPolicyActions = [
  "accept",
  "filter",
  "drop",
] as const satisfies readonly NotificationPolicyAction[];

/** The user's policy (GET /preferences/notification_policy). */
export function notificationPolicyQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences/notification_policy");
}

/** One change of the policy: the action of one condition. */
export type NotificationPolicyChange = { condition: NotificationPolicyCondition; action: NotificationPolicyAction };

/**
 * Changes the action of one condition and keeps the others (PATCH /preferences/notification_policy), as the options
 * of TanStack Query's useMutation; the answer is the policy after the change.
 */
export function updateNotificationPolicyMutation(api: Api) {
  return {
    mutationKey: ["patch", "/preferences/notification_policy"] as const,
    mutationFn: async ({ condition, action }: NotificationPolicyChange): Promise<NotificationPolicy> => {
      const { data, error } = await api.fetch.PATCH("/preferences/notification_policy", {
        body: { [condition]: action },
      });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Puts the policy as the server answered a change into the cache (web-client/README.md S1). */
export function cacheNotificationPolicy(api: Api, queryClient: QueryClient, saved: NotificationPolicy): void {
  queryClient.setQueryData(notificationPolicyQueryOptions(api).queryKey, saved);
}
