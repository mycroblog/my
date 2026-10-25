// SPDX-License-Identifier: AGPL-3.0-only
// The Web Push subscription of the signed-in grant (FR-206, web-client/README.md S1): one per signed-in app, which the
// server sends the chosen kinds of notifications to.
import { type QueryClient, queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

export type PushSubscriptionInfo = Schemas["PushSubscription"];
export type PushSubscriptionRequest = Schemas["PushSubscriptionRequest"];
export type NotificationType = Schemas["NotificationType"];

/** The grant's subscription (GET /push/subscription); null when it has none (404). */
export function pushSubscriptionQueryOptions(api: Api) {
  return queryOptions({
    queryKey: ["get", "/push/subscription"] as const,
    queryFn: async ({ signal }): Promise<PushSubscriptionInfo | null> => {
      try {
        const { data } = await api.fetch.GET("/push/subscription", { signal });
        return data ?? null;
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },
  });
}

/** Registers the browser's subscription with the notification types to push (PUT /push/subscription). */
export async function savePushSubscription(api: Api, body: PushSubscriptionRequest): Promise<PushSubscriptionInfo> {
  const { data, error } = await api.fetch.PUT("/push/subscription", { body });
  // The middleware throws an ApiError for every error status, so this is only for the types.
  if (data === undefined) throw new Error(error.title);
  return data;
}

/** Deletes the grant's subscription (DELETE /push/subscription). */
export async function deletePushSubscription(api: Api): Promise<void> {
  await api.fetch.DELETE("/push/subscription");
}

/** Puts the grant's subscription as the server answered a change into the cache (web-client/README.md S1). */
export function cachePushSubscription(api: Api, queryClient: QueryClient, saved: PushSubscriptionInfo | null): void {
  queryClient.setQueryData(pushSubscriptionQueryOptions(api).queryKey, saved);
}
