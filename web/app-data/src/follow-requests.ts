// SPDX-License-Identifier: AGPL-3.0-only
// The follow requests to the reader (FR-302), newest first: how many wait (the first page's), and the list a page at a
// time, each page with the reader's relationships with its accounts, by which a request answered here shows the answer
// (no longer requested; followed by the account once approved) where it stood.
import { useQuery } from "@tanstack/react-query";
import { cacheRelationship, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { readRelationships } from "./follows";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** The first page of the follow requests, by which a count of those that wait is read. */
export function followRequestCountQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/follow_requests");
}

/** The follow requests a page at a time, for their screen, with the reader's relationships with each account. */
export function followRequestsQuery(data: AppData) {
  const { api, queries } = data;
  return pagedQuery<Account>(
    queries.queryOptions("get", "/follow_requests", { params: { query: {} } }).queryKey,
    async (cursor, signal) => {
      const { data: page } = await api.GET("/follow_requests", {
        params: { query: cursor === undefined ? {} : { cursor } },
        signal,
      });
      if (page !== undefined)
        await readRelationships(
          data,
          page.items.map((a) => a.id),
          signal,
        );
      return page;
    },
  );
}

/** Reads the count again, after the stream told of a new request or was closed for a while. */
export function receiveFollowRequests(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: followRequestCountQuery(data).queryKey, exact: true });
}

/**
 * Approves (`accept`) or declines the follow request of the account of `id`, and resolves once the cache holds the
 * relationship the server answered and the count is read again. It rejects with the server's problem.
 */
async function answerFollowRequest(data: AppData, id: string, accept: boolean): Promise<void> {
  const path = { params: { path: { id } } };
  const { data: relationship } = accept
    ? await data.api.POST("/follow_requests/{id}/accept", path)
    : await data.api.POST("/follow_requests/{id}/reject", path);
  if (relationship !== undefined) await cacheRelationship(data, relationship);
  receiveFollowRequests(data);
}

/** How many follow requests wait, of the first page the server lists; undefined until it answered. */
export function useFollowRequestCount(): number | undefined {
  return useQuery(followRequestCountQuery(useAppData())).data?.items.length;
}

/** The follow requests, newest first, those answered here staying in place. */
export function useFollowRequests(): PagedList<Account> {
  return usePagedList(followRequestsQuery(useAppData()));
}

/** Approves (`accept`) or declines the follow request of the account of `id`, from a screen. */
export function useAnswerFollowRequest(): (id: string, accept: boolean) => Promise<void> {
  const data = useAppData();
  return (id, accept) => answerFollowRequest(data, id, accept);
}
