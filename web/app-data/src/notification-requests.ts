// SPDX-License-Identifier: AGPL-3.0-only
// The notifications the reader's notification policy holds back (ADR-0158 §7), one request per account that sent them:
// the requests a page at a time, the newest first, and the held-back notifications of one of them. The reader accepts
// a request, which moves its notifications into the list and lets the account's later ones through, or dismisses it,
// which drops them. An answered request keeps its place on the screens open now, with the answer, until they open
// again; the count of the requests and the notifications are read again at once.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { notificationRequestSummaryQuery, notificationsQuery, type NotificationItem } from "./notifications";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** The held-back notifications of one account, named by the account's ID. */
export type NotificationRequest = components["schemas"]["NotificationRequest"];

/** What the reader answered to a request: accepted, or dismissed. */
export type RequestAnswer = "accepted" | "dismissed";

/** The requests a page at a time, the one with the newest notification first. */
export function notificationRequestsQuery({ api, queries }: AppData) {
  return pagedQuery<NotificationRequest>(
    queries.queryOptions("get", "/notifications/requests", { params: { query: {} } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/notifications/requests", {
        params: { query: cursor === undefined ? {} : { cursor } },
        signal,
      });
      return data;
    },
  );
}

/** The request of the account of `id`. */
export function notificationRequestQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/notifications/requests/{id}", { params: { path: { id } } });
}

/** The held-back notifications of the account of `id`, newest first, a page at a time. */
export function requestNotificationsQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<NotificationItem>(
    queries.queryOptions("get", "/notifications/requests/{id}/notifications", {
      params: { path: { id }, query: {} },
    }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/notifications/requests/{id}/notifications", {
        params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
        signal,
      });
      return data;
    },
  );
}

/** Where the answer to the request of `id` is kept for the session. */
const answerKey = (id: string) => ["my", "notification-request-answer", id] as const;

/**
 * Accepts or dismisses the request of the account of `id`, and resolves once the server answered: the answer is kept
 * where the screens read it, the count of the requests and the notifications (which an accepted request joins) are read
 * again, and the requests are read again when their screens next open. It rejects with the server's problem.
 */
async function answerRequest(data: AppData, id: string, answer: RequestAnswer): Promise<void> {
  const path = { params: { path: { id } } };
  if (answer === "accepted") await data.api.POST("/notifications/requests/{id}/accept", path);
  else await data.api.POST("/notifications/requests/{id}/dismiss", path);
  data.queryClient.setQueryData(answerKey(id), answer);
  const { queryClient } = data;
  void queryClient.invalidateQueries({ queryKey: notificationRequestSummaryQuery(data).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: notificationRequestsQuery(data).queryKey, refetchType: "none" });
  void queryClient.invalidateQueries({ queryKey: requestNotificationsQuery(data, id).queryKey, refetchType: "none" });
  if (answer === "accepted")
    for (const filter of ["all", "mentions"] as const)
      void queryClient.invalidateQueries({ queryKey: notificationsQuery(data, filter).queryKey, exact: true });
}

/** The requests, the newest first, those answered here staying in place. */
export function useNotificationRequests(): PagedList<NotificationRequest> {
  return usePagedList(notificationRequestsQuery(useAppData()));
}

/** The request of one account as a screen reads it, and how it came. */
export type OneRequest = {
  readonly request: NotificationRequest | undefined;
  /** `gone` once the server says it lists no request of the account (404); `error` for any other failure. */
  readonly status: "pending" | "success" | "error" | "gone";
  readonly refetch: () => void;
};

/** The request of the account of `id`; undefined until the server answered. */
export function useNotificationRequest(id: string): OneRequest {
  const query = useQuery(notificationRequestQuery(useAppData(), id));
  const { error } = query;
  return {
    request: query.data,
    status:
      query.status === "error"
        ? error instanceof ProblemError && error.status === 404
          ? "gone"
          : "error"
        : query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** The held-back notifications of the account of `id`. */
export function useRequestNotifications(id: string): PagedList<NotificationItem> {
  return usePagedList(requestNotificationsQuery(useAppData(), id));
}

/** What the reader answered here to the request of `id`; undefined while it waits. */
export function useRequestAnswer(id: string): RequestAnswer | undefined {
  return (
    useQuery({
      queryKey: answerKey(id),
      queryFn: () => null as RequestAnswer | null,
      enabled: false,
      staleTime: Infinity,
      gcTime: Infinity,
    }).data ?? undefined
  );
}

/** Accepts or dismisses a request, from a screen (answerRequest). */
export function useAnswerRequest(): (id: string, answer: RequestAnswer) => Promise<void> {
  const data = useAppData();
  return (id, answer) => answerRequest(data, id, answer);
}
