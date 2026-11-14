// SPDX-License-Identifier: AGPL-3.0-only
// A list the API pages with a cursor (a profile's posts, the notifications): the infinite query of one operation, keyed
// as openapi-react-query keys it, and what a screen reads of it, in the shape of the home timeline's (timeline.ts).
import { infiniteQueryOptions, useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";

/** A page of a list: its items and the cursor of the next page. */
export type Page<T> = { readonly items: readonly T[]; readonly next_cursor?: string | undefined };

/** The cached pages of a list, each keyed by the cursor it was fetched with (undefined for the first). */
export type Pages<T> = InfiniteData<Page<T>, string | undefined>;

/** The cursor of the page after page; undefined on the last page. */
const nextCursor = (page: Page<unknown>) =>
  page.next_cursor === undefined || page.next_cursor === "" ? undefined : page.next_cursor;

/**
 * The infinite query of a list whose key is `queryKey` (the operation's, without a cursor), each page read by `fetch`
 * with the cursor of the page before.
 */
export function pagedQuery<T>(
  queryKey: readonly unknown[],
  fetch: (cursor: string | undefined, signal: AbortSignal) => Promise<Page<T> | undefined>,
) {
  return infiniteQueryOptions({
    queryKey,
    queryFn: async ({ pageParam, signal }): Promise<Page<T>> => {
      // The client rejects an error answer, so the page is set whenever it resolves.
      const page = await fetch(pageParam, signal);
      if (page === undefined) throw new Error(`app-data: ${String(queryKey[1])} answered without a page`);
      return page;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => nextCursor(page),
  });
}

/** What a screen shows of a list and can ask for. */
export type PagedList<T> = {
  /** The items of the pages fetched so far, in the list's order; empty until the first page arrives. */
  readonly items: readonly T[];
  /** `pending` until the first page arrives, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** The failure of the last fetch (a ProblemError or NetworkError of mykit), or null. */
  readonly error: Error | null;
  /** Whether there is a page after the last one fetched. */
  readonly hasNextPage: boolean;
  /** Whether the next page is being fetched. */
  readonly isFetchingNextPage: boolean;
  /** Fetches the next page; does nothing while one is being fetched or when there is none. */
  readonly fetchNextPage: () => void;
  /** Fetches the list again from its first page: the reader's try again after the first page failed. */
  readonly refetch: () => void;
};

/** The list of `options` (a `pagedQuery`), for a screen; nothing is fetched while `enabled` is false. */
export function usePagedList<T>(options: ReturnType<typeof pagedQuery<T>>, enabled = true): PagedList<T> {
  const query = useInfiniteQuery({ ...options, enabled });
  return {
    items: query.data?.pages.flatMap((page) => page.items) ?? [],
    status: query.status,
    error: query.error,
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: () => {
      if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
    },
    refetch: () => {
      void query.refetch();
    },
  };
}
