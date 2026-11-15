// SPDX-License-Identifier: AGPL-3.0-only
// The home timeline: the query a route's loader reads and the hook its column calls.
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  type InfiniteData,
  type UseInfiniteQueryOptions,
} from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import type { Status, StatusPage } from "./statuses";

/**
 * The home timeline, newest first, a page per cursor. Its key is the one openapi-react-query gives the operation, so
 * that a write marks it with the same key as every other query of the API. A route's loader reads it as ADR-0028 §6
 * says: `queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity })`.
 */
export function homeTimelineQuery({ api, queries }: AppData) {
  return infiniteQueryOptions({
    queryKey: queries.queryOptions("get", "/timelines/home").queryKey,
    queryFn: async ({ pageParam, signal }): Promise<StatusPage> => {
      // The client rejects an error answer, so data is set whenever it resolves.
      const { data } = await api.GET("/timelines/home", {
        params: { query: pageParam === undefined ? {} : { cursor: pageParam } },
        signal,
      });
      if (data === undefined) throw new Error("app-data: the home timeline answered without a page");
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => nextCursor(page),
  });
}

/** The cursor of the page after page; undefined on the last page. */
function nextCursor(page: StatusPage): string | undefined {
  return page.next_cursor === undefined || page.next_cursor === "" ? undefined : page.next_cursor;
}

/** What a column of the home timeline shows and can ask for. */
export type HomeTimeline = {
  /** The statuses of the pages fetched so far, newest first; empty until the first page arrives. */
  readonly statuses: readonly Status[];
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
  /** Fetches the timeline again from its first page: the reader's try again after the first page failed. */
  readonly refetch: () => void;
};

/** A public timeline of the instance: its local posts, or the posts it knows from every server. */
export type PublicTimelineKind = "local" | "federated";

/**
 * A public timeline of the instance, newest first, a page per cursor, keyed as its operation; a reader without a
 * session reads it while the instance shows it to everyone. A route's loader reads it as the home timeline's.
 */
export function publicTimelineQuery({ api, queries }: AppData, kind: PublicTimelineKind) {
  const path = kind === "local" ? "/timelines/local" : "/timelines/federated";
  return infiniteQueryOptions({
    queryKey: queries.queryOptions("get", path).queryKey,
    queryFn: async ({ pageParam, signal }): Promise<StatusPage> => {
      const { data } = await api.GET(path, {
        params: { query: pageParam === undefined ? {} : { cursor: pageParam } },
        signal,
      });
      if (data === undefined) throw new Error(`app-data: the ${kind} timeline answered without a page`);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => nextCursor(page),
  });
}

/** A timeline's column of the query `options`. */
function useTimelineColumn<Key extends readonly unknown[]>(
  options: UseInfiniteQueryOptions<StatusPage, Error, InfiniteData<StatusPage>, Key, string | undefined>,
): HomeTimeline {
  const query = useInfiniteQuery(options);
  return {
    statuses: query.data?.pages.flatMap((page) => page.items) ?? [],
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

/** The home timeline of the viewer, for its column. */
export function useHomeTimeline(): HomeTimeline {
  return useTimelineColumn(homeTimelineQuery(useAppData()));
}

/** A public timeline of the instance, for its column. */
export function usePublicTimeline(kind: PublicTimelineKind): HomeTimeline {
  return useTimelineColumn(publicTimelineQuery(useAppData(), kind));
}
