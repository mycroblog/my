// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the requests tab of the custom emoji screen (web-client/README.md S1, ADR-0070 §2, §6,
// manage_custom_emojis): the requests of the holders of create_custom_emojis in one state, newest first (GET
// /admin/custom_emoji_requests, the pending ones by default), approving one (POST .../approve), which makes the emoji
// or gives it the new image, and rejecting one with an optional reason (POST .../reject). A request that cannot be
// decided is a 409 `custom-emoji-request-conflict` problem whose detail says why. Every decision is recorded in the
// audit log; an approval also changes the emoji and what the picker offers.
import { infiniteQueryOptions, type InfiniteData, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type AdminCustomEmojiRequest = Schemas["AdminCustomEmojiRequest"];
export type CustomEmojiRequestState = Schemas["CustomEmojiRequestState"];

/** The states of a request, in the order of the filter: the pending ones, which await a decision, first. */
export const customEmojiRequestStates = ["pending", "approved", "rejected", "withdrawn", "lapsed"] as const satisfies readonly CustomEmojiRequestState[];

/** The longest reason of a rejection (RejectCustomEmojiRequestRequest). */
export const rejectReasonLimit = 500;

/** The requests in `state`, newest first. */
export function adminCustomEmojiRequestsQueryOptions(api: Api, state: CustomEmojiRequestState) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/custom_emoji_requests", { state }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/custom_emoji_requests", { params: { query: { state, ...pageQuery(pageParam) } }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Approves a pending request, as the options of useMutation. Returns the request as decided. */
export function approveCustomEmojiRequestMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/custom_emoji_requests/{id}/approve"] as const,
    mutationFn: async (id: string): Promise<AdminCustomEmojiRequest> => {
      const { data, error } = await api.fetch.POST("/admin/custom_emoji_requests/{id}/approve", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Rejects a pending request with an optional reason, as the options of useMutation. Returns the request as decided. */
export function rejectCustomEmojiRequestMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/custom_emoji_requests/{id}/reject"] as const,
    mutationFn: async ({ id, reason }: { id: string; reason: string }): Promise<AdminCustomEmojiRequest> => {
      const { data, error } = await api.fetch.POST("/admin/custom_emoji_requests/{id}/reject", { params: { path: { id } }, body: reason === "" ? {} : { reason } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

type RequestPages = InfiniteData<{ items: AdminCustomEmojiRequest[]; next_cursor?: string }>;

/**
 * A decided request: it leaves the cached pending list at once, and the lists of the other states gain it, so they
 * are fetched again with the audit log; an approval also changes the emoji and the picker's emoji.
 */
export async function cacheDecidedCustomEmojiRequest(queryClient: QueryClient, request: AdminCustomEmojiRequest): Promise<void> {
  queryClient.setQueriesData<RequestPages>({ queryKey: ["get", "/admin/custom_emoji_requests", { state: "pending" }] }, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.filter((item) => item.id !== request.id) })) },
  );
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/custom_emoji_requests"], predicate: (query) => !isPendingList(query.queryKey) }),
    request.state === "approved" ? queryClient.invalidateQueries({ queryKey: ["get", "/admin/custom_emojis"] }) : undefined,
    request.state === "approved" ? queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis"] }) : undefined,
    refreshAuditLog(queryClient),
  ]);
}

/** Whether a query key is the pending requests' list, which a decision has already updated. */
function isPendingList(key: readonly unknown[]): boolean {
  const filter = key[2];
  return typeof filter === "object" && filter !== null && "state" in filter && filter.state === "pending";
}
