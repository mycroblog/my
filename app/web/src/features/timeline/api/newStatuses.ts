// SPDX-License-Identifier: AGPL-3.0-only
// The home timeline's new entries that wait for the reader (web-client/README.md Y7, S10): the streaming API
// delivers them, and while a page shows the home timeline they stay in a Query cache entry of their own
// until they are inserted at the top: by the reader pressing "N new posts" or, when the reader chose automatic
// insertion (ADR-0077), by useHomeNewPosts as soon as nothing keeps them (the reader is not in the feed). While no page shows it, nobody
// is reading it: the stream inserts them into its cached pages at once, so that the timeline shows them
// from the moment it opens again (a timeline loaded less than `staleTime` ago that no write marked
// stale is not fetched again on opening).
//
// When the waiting entries may not join the loaded ones (a reconnect found a whole page of news, or more
// arrived than are kept), there is a gap; pressing the button then loads the first page again instead,
// the way TanStack Query's guide refetches an infinite query from its first page only
// (https://tanstack.com/query/v5/docs/framework/react/guides/infinite-queries).
import { type QueryClient, queryOptions, skipToken } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { type Status, type StatusPage, timelineQueryOptions } from "./timelines";

/** The new home entries the stream delivered, newest first, and whether they may not join the loaded ones. */
export type NewStatuses = { statuses: readonly Status[]; gap: boolean };

/** How many waiting entries are kept; older ones are dropped, and the timeline then has a gap. */
const maxNewStatuses = 200;

/**
 * The waiting entries. Only the stream writes them (setQueryData); there is nothing to fetch, and they are
 * kept until they are shown or the user changes (lib/auth/userData.ts clears the cache).
 */
export function newHomeStatusesQueryOptions() {
  return queryOptions<NewStatuses>({
    // Not an API path in the second place, so that invalidating the lists of a path never reaches it.
    queryKey: ["stream", "home-new-statuses"] as const,
    queryFn: skipToken,
    staleTime: Infinity,
    gcTime: Infinity,
  });
}

/** IDs are UUIDv7 strings, which sort in the order they were made: newest first. */
function newestFirst(a: Status, b: Status): number {
  return a.id < b.id ? 1 : a.id > b.id ? -1 : 0;
}

/** The loaded home timeline, newest first, or undefined when it has not been loaded. */
function loadedHome(queryClient: QueryClient, api: Api): readonly Status[] | undefined {
  return queryClient
    .getQueryData(timelineQueryOptions(api, { kind: "home" }).queryKey)
    ?.pages.flatMap((page) => page.items);
}

/**
 * The waiting entries that are still new to the loaded timeline: not in it and newer than its newest. A
 * refetch of the timeline (on navigating back to it) may have loaded some of them already.
 */
export function unseenStatuses(
  waiting: NewStatuses | undefined,
  loaded: readonly Status[] | undefined,
): readonly Status[] {
  if (waiting === undefined) return [];
  if (loaded === undefined) return waiting.statuses;
  const ids = new Set(loaded.map((status) => status.id));
  const top = loaded[0]?.id;
  return waiting.statuses.filter((status) => !ids.has(status.id) && (top === undefined || status.id > top));
}

/** Adds `statuses` to the waiting entries, without repeating one. */
function addWaiting(queryClient: QueryClient, statuses: readonly Status[], gap: boolean): void {
  queryClient.setQueryData(newHomeStatusesQueryOptions().queryKey, (previous) => {
    const known = new Set(previous?.statuses.map((status) => status.id));
    const added = statuses.filter((status) => !known.has(status.id));
    if (added.length === 0 && !gap) return previous;
    const all = [...added, ...(previous?.statuses ?? [])].toSorted(newestFirst);
    return {
      statuses: all.slice(0, maxNewStatuses),
      gap: gap || previous?.gap === true || all.length > maxNewStatuses,
    };
  });
}

/**
 * Inserts the waiting entries that join the loaded ones at the top of the cached home timeline, and
 * clears them. After a gap it changes nothing and returns false: only loading the first page again closes it.
 */
function insertWaiting(queryClient: QueryClient, api: Api): boolean {
  const waitingKey = newHomeStatusesQueryOptions().queryKey;
  const waiting = queryClient.getQueryData(waitingKey);
  if (waiting === undefined) return true;
  if (waiting.gap) return false;
  queryClient.setQueryData(waitingKey, { statuses: [], gap: false });
  queryClient.setQueryData(timelineQueryOptions(api, { kind: "home" }).queryKey, (data) => {
    if (data === undefined) return data;
    const fresh = unseenStatuses(
      waiting,
      data.pages.flatMap((page) => page.items),
    );
    const [first, ...rest] = data.pages;
    if (first === undefined || fresh.length === 0) return data;
    return { ...data, pages: [{ ...first, items: [...fresh, ...first.items] }, ...rest] };
  });
  return true;
}

/** Whether a page shows the home timeline now: its query has observers (the timeline's components). */
function homeShown(queryClient: QueryClient, api: Api): boolean {
  const query = queryClient
    .getQueryCache()
    .find({ queryKey: timelineQueryOptions(api, { kind: "home" }).queryKey, exact: true });
  return (query?.getObserversCount() ?? 0) > 0;
}

/** New entries wait for the reader while a page shows the timeline, and join its cache otherwise. */
function receive(queryClient: QueryClient, api: Api, statuses: readonly Status[], gap: boolean): void {
  if (statuses.length > 0 || gap) addWaiting(queryClient, statuses, gap);
  joinWaitingHomeStatuses(queryClient, api);
}

/**
 * Inserts the waiting entries into the cached home timeline unless a page shows it. The home route's loader
 * calls it too, so that entries that arrived while the timeline was shown, but were not shown when the reader
 * left it, are on it when it opens again.
 */
export function joinWaitingHomeStatuses(queryClient: QueryClient, api: Api): void {
  if (!homeShown(queryClient, api)) insertWaiting(queryClient, api);
}

/** A new home entry from the stream. */
export function receiveHomeStatus(queryClient: QueryClient, api: Api, status: Status): void {
  receive(queryClient, api, [status], false);
}

/**
 * After the stream reconnected: reads the first page of the home timeline for the entries the gap
 * missed and adds the new ones to the waiting entries. Nothing is read while the timeline is not loaded:
 * it loads fresh when the reader opens it.
 */
export async function catchUpHome(queryClient: QueryClient, api: Api): Promise<void> {
  const loaded = loadedHome(queryClient, api);
  if (loaded === undefined) return;
  const { data, error } = await api.fetch.GET("/timelines/home", {});
  if (data === undefined) throw new ApiError(error);
  const page: StatusPage = data;
  const top = loaded[0]?.id;
  const fresh = unseenStatuses({ statuses: page.items, gap: false }, loaded);
  const oldest = page.items.at(-1)?.id;
  // A full page of news with more behind it may not reach the loaded entries.
  const gap = page.next_cursor !== undefined && oldest !== undefined && (top === undefined || oldest > top);
  receive(queryClient, api, fresh, gap);
}

/**
 * Shows the waiting entries: inserts them at the top of the loaded home timeline, or, after a gap,
 * loads the first page again and drops the older pages (they follow again on scrolling). Resolves when
 * the timeline shows them; rejects when loading the first page again failed.
 */
export async function showNewHomeStatuses(queryClient: QueryClient, api: Api): Promise<void> {
  if (insertWaiting(queryClient, api)) return;
  const homeKey = timelineQueryOptions(api, { kind: "home" }).queryKey;
  queryClient.setQueryData(newHomeStatusesQueryOptions().queryKey, { statuses: [], gap: false });
  queryClient.setQueryData(
    homeKey,
    (data) => data && { pages: data.pages.slice(0, 1), pageParams: data.pageParams.slice(0, 1) },
  );
  await queryClient.refetchQueries({ queryKey: homeKey, exact: true }, { throwOnError: true });
}
