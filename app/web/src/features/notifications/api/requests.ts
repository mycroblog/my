// SPDX-License-Identifier: AGPL-3.0-only
// The notification requests (ADR-0158 §7, web-client/README.md S1): the notifications that the user's notification
// policy held, one request per source account, with their counts; the notifications of one request; and the answers
// to a request, accepting or dismissing it. Keys follow openapi-react-query's `[method, path, init]`, so the prefix
// `["get", "/notifications/requests"]` matches the pages of the list.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { notificationMarkerQueryOptions, notificationsPanelQueryOptions } from "./notifications";

export type NotificationRequest = components["schemas"]["NotificationRequest"];
export type NotificationRequestSummary = components["schemas"]["NotificationRequestSummary"];

/** The page of the requests. */
export const notificationRequestsPath = "/notifications/requests";

/** The page of the notifications of the request of account `accountId`. */
export function notificationRequestPath(accountId: string): string {
  return `/notifications/requests/${encodeURIComponent(accountId)}`;
}

/** The page size of the lists, the API's maximum. */
const pageSize = 40;

/** How many requests wait, and how many notifications are in them (GET /notifications/requests/summary). */
export function notificationRequestSummaryQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/notifications/requests/summary");
}

/** The requests, the one with the newest notification first, a page at a time (GET /notifications/requests). */
export function notificationRequestsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/notifications/requests", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/notifications/requests", {
        params: { query: pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam } },
        signal,
      });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The request of account `accountId` (GET /notifications/requests/{id}); a 404 when there is none. */
export function notificationRequestQueryOptions(api: Api, accountId: string) {
  return api.query.queryOptions("get", "/notifications/requests/{id}", { params: { path: { id: accountId } } });
}

/**
 * The notifications held in the request of account `accountId`, newest first, a page at a time
 * (GET /notifications/requests/{id}/notifications).
 */
export function notificationRequestNotificationsQueryOptions(api: Api, accountId: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/notifications/requests/{id}/notifications", { params: { path: { id: accountId } } }] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/notifications/requests/{id}/notifications", {
        params: {
          path: { id: accountId },
          query: pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam },
        },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** How the user answers a request. */
export type RequestAnswer = "accept" | "dismiss";

/**
 * Accepts or dismisses the request of account `accountId` (POST /notifications/requests/{id}/accept or /dismiss). A
 * second answer is a 404, so a retry after a lost answer fails rather than repeats.
 */
export async function answerNotificationRequest(api: Api, accountId: string, answer: RequestAnswer): Promise<void> {
  const path = answer === "accept" ? "/notifications/requests/{id}/accept" : "/notifications/requests/{id}/dismiss";
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  await api.fetch.POST(path, { params: { path: { id: accountId } } });
}

/**
 * Puts an answer to a request into the cache (S1), but for the request itself, which
 * removeAnsweredRequest takes off: the counts are marked and read again, and the list of requests is marked to be read
 * when it opens next. Accepting moves its notifications into the list, so the lists of notifications, the third
 * column's panel, and the unread count are marked too. Resolves once the queries on screen have been read again, so
 * that the success shows with them.
 */
export async function cacheAnsweredRequest(api: Api, queryClient: QueryClient, answer: RequestAnswer): Promise<void> {
  const marked = [
    queryClient.invalidateQueries({ queryKey: notificationRequestSummaryQueryOptions(api).queryKey, exact: true }),
    // The row on screen leaves without a request (removeAnsweredRequest).
    queryClient.invalidateQueries({ queryKey: notificationRequestsQueryOptions(api).queryKey, refetchType: "none" }),
  ];
  if (answer === "accept") {
    marked.push(
      queryClient.invalidateQueries({ queryKey: notificationMarkerQueryOptions(api).queryKey, exact: true }),
      queryClient.invalidateQueries({ queryKey: notificationsPanelQueryOptions(api).queryKey, exact: true }),
      // Every filter of the notifications page: `["get", "/notifications", init]`.
      queryClient.invalidateQueries({ queryKey: ["get", "/notifications"] }),
    );
  }
  await Promise.all(marked);
}

/**
 * Takes the answered request of account `accountId` off the cached pages of the list, which holds only the requests
 * that wait, and drops its own queries. On the list, its caller moves the focus as the row leaves; on the request's own
 * page, it is called once the page has been left, so that the page does not read the request again.
 */
export function removeAnsweredRequest(api: Api, queryClient: QueryClient, accountId: string): void {
  queryClient.removeQueries({ queryKey: notificationRequestQueryOptions(api, accountId).queryKey, exact: true });
  queryClient.removeQueries({ queryKey: notificationRequestNotificationsQueryOptions(api, accountId).queryKey });
  queryClient.setQueryData(
    notificationRequestsQueryOptions(api).queryKey,
    (data) =>
      data && {
        ...data,
        pages: data.pages.map((page) => ({ ...page, items: page.items.filter((request) => request.id !== accountId) })),
      },
  );
}

/**
 * The counts of the requests from the stream (`notification_requests`, ADR-0158 §7): they replace the cached ones
 * without a request, so the row at the top of the notifications shows, changes, or leaves at once. The list of
 * requests is only marked stale (`refetchType: "none"`): the one on screen does not change under the reader, and the
 * route reads it again when it opens next (web-client/README.md S7).
 */
export function receiveRequestSummary(queryClient: QueryClient, api: Api, summary: NotificationRequestSummary): void {
  queryClient.setQueryData(notificationRequestSummaryQueryOptions(api).queryKey, summary);
  void queryClient.invalidateQueries({ queryKey: notificationRequestsQueryOptions(api).queryKey, refetchType: "none" });
}

/** After the stream reconnected: reads the counts again, and marks the list as a change of them does. */
export function catchUpRequests(queryClient: QueryClient, api: Api): void {
  void queryClient.invalidateQueries({ queryKey: notificationRequestSummaryQueryOptions(api).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: notificationRequestsQueryOptions(api).queryKey, refetchType: "none" });
}
