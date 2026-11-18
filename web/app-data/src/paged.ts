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
export function pagedQuery<T, P extends Page<T> = Page<T>>(
  queryKey: readonly unknown[],
  fetch: (cursor: string | undefined, signal: AbortSignal) => Promise<P | undefined>,
) {
  return infiniteQueryOptions({
    queryKey,
    queryFn: async ({ pageParam, signal }): Promise<P> => {
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
  /**
   * Whether the last fetch of a next page failed, the pages before it kept (`status` is then `error`, as after any
   * fetch that failed, and `error` says why); it stays true while that page is asked for again, until a fetch succeeds.
   */
  readonly failedNextPage: boolean;
  /** Fetches the next page; does nothing while one is being fetched or when there is none. */
  readonly fetchNextPage: () => void;
  /** Fetches the list again from its first page: the reader's try again after the first page failed. */
  readonly refetch: () => void;
};

/** A list whose pages carry more than items (`P`): the first page, which is the newest read of it, shows that more. */
export type PagedListOf<T, P extends Page<T>> = PagedList<T> & { readonly firstPage: P | undefined };

/** The list of `options` (a `pagedQuery`), for a screen; nothing is fetched while `enabled` is false. */
export function usePagedList<T>(options: ReturnType<typeof pagedQuery<T>>, enabled = true): PagedList<T> {
  const { firstPage: _firstPage, ...list } = usePagedListOf<T, Page<T>>(options, enabled);
  return list;
}

/** The list of `options` whose pages carry more than items, with its first page. */
export function usePagedListOf<T, P extends Page<T>>(
  options: ReturnType<typeof pagedQuery<T, P>>,
  enabled = true,
): PagedListOf<T, P> {
  const query = useInfiniteQuery({ ...options, enabled });
  return {
    items: query.data?.pages.flatMap((page) => page.items) ?? [],
    firstPage: query.data?.pages[0],
    status: query.status,
    error: query.error,
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    failedNextPage: query.isFetchNextPageError,
    fetchNextPage: () => {
      if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
    },
    refetch: () => {
      void query.refetch();
    },
  };
}
