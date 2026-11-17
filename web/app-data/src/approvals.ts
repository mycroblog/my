// SPDX-License-Identifier: AGPL-3.0-only
// The replies, boosts and quotes of the reader's posts that wait for the reader's approval (ADR-0155 §10), newest
// first: how many wait (the first page's), and the list a page at a time, each answered with Approve or Decline. An
// answered request stays in the cached lists as the server answered it, so that a screen goes on showing the answer;
// an approved reply joins the cached threads of its post at once, and an approved boost counts on it.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";
import { shadowStatus } from "./shadow";
import { cacheReply } from "./thread";

/** A reply, boost or quote of the reader's post that waits for the reader's approval. */
export type InteractionRequest = components["schemas"]["InteractionRequest"];

type RequestPage = components["schemas"]["InteractionRequestPage"];

/** The first page of the requests, by which a count of those that wait is read. */
export function interactionRequestsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/interaction_requests");
}

/** The requests a page at a time, for their screen. */
export function waitingRequestsQuery({ api, queries }: AppData) {
  return pagedQuery<InteractionRequest>(
    queries.queryOptions("get", "/interaction_requests", { params: { query: {} } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/interaction_requests", {
        params: { query: cursor === undefined ? {} : { cursor } },
        signal,
      });
      return data;
    },
  );
}

/** Reads the requests again, after the stream told of a new one or was closed for a while. */
export function receiveInteractionRequests(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: interactionRequestsQuery(data).queryKey });
}

/** Writes an answered request over its copy in each cached list of the requests. */
function cacheAnswer(data: AppData, answered: InteractionRequest): void {
  const swap = (r: InteractionRequest) => (r.id === answered.id ? answered : r);
  data.queryClient.setQueryData<RequestPage>(
    interactionRequestsQuery(data).queryKey,
    (page) => page && { ...page, items: page.items.map(swap) },
  );
  data.queryClient.setQueryData<Pages<InteractionRequest>>(
    waitingRequestsQuery(data).queryKey,
    (cached) => cached && { ...cached, pages: cached.pages.map((p) => ({ ...p, items: p.items.map(swap) })) },
  );
}

/**
 * Approves or declines an interaction request, and resolves once the cache knows it: the request answered in the
 * lists, and an approved reply under its post, an approved boost counted on it. The answer's post counts what the
 * server recorded with the approval, so its counts are taken as they are. It rejects with the server's problem.
 */
async function answerInteractionRequest(data: AppData, id: string, accept: boolean): Promise<void> {
  const path = { params: { path: { id } } };
  const { data: answered } = accept
    ? await data.api.POST("/interaction_requests/{id}/accept", path)
    : await data.api.POST("/interaction_requests/{id}/reject", path);
  if (answered === undefined) {
    await data.queryClient.invalidateQueries({ queryKey: interactionRequestsQuery(data).queryKey });
    return;
  }
  cacheAnswer(data, answered);
  if (answered.state !== "accepted") return;
  const target = answered.status.id;
  if (answered.kind === "reply" && answered.interacting_status !== undefined) {
    const count = answered.status.replies_count;
    shadowStatus(data, target, (s) => ({ ...s, replies_count: Math.max(s.replies_count, count) }), "replies").confirm();
    cacheReply(data.queryClient, answered.interacting_status);
  }
  if (answered.kind === "announce") {
    const count = answered.status.boosts_count;
    shadowStatus(data, target, (s) => ({ ...s, boosts_count: Math.max(s.boosts_count, count) }), "boosts").confirm();
  }
}

/** How many requests wait for the reader's approval, of the first page the server lists; undefined until it answered. */
export function useWaitingCount(): number | undefined {
  const page = useQuery(interactionRequestsQuery(useAppData())).data;
  return page?.items.filter((r) => r.state === "pending").length;
}

/** The requests that wait for the reader's approval, newest first, those answered here with their answer. */
export function useWaitingRequests(): PagedList<InteractionRequest> {
  return usePagedList(waitingRequestsQuery(useAppData()));
}

/** Approves (`accept`) or declines the request of `id`, from a screen. */
export function useAnswerInteraction(): (id: string, accept: boolean) => Promise<void> {
  const data = useAppData();
  return (id, accept) => answerInteractionRequest(data, id, accept);
}
