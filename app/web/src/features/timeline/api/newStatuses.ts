// SPDX-License-Identifier: AGPL-3.0-only
// The new entries of the streamed timelines that wait for the reader (web-client/README.md Y7, S10): the home timeline,
// each of the user's lists (ADR-0122 §4), and the local and the federated timelines (ADR-0127 §5). The user's stream
// delivers the first two (`update`, `list_update`) and the timeline's own stream, open while its screen shows it, the
// others (`update`). While a page
// shows the timeline they stay in a Query cache entry of their own until they are inserted at the top: by the reader
// pressing "N new posts" or, when the reader chose automatic insertion (ADR-0077), by useNewPosts as soon as nothing
// keeps them (the reader is not in the feed). While no page shows it, nobody is reading it: the stream inserts them into
// its cached pages at once, so that the timeline shows them from the moment it opens again (a timeline loaded less than
// `staleTime` ago that no write marked stale is not fetched again on opening).
//
// When the waiting entries may not join the loaded ones (a reconnect found a whole page of news, or more
// arrived than are kept), there is a gap; pressing the button then loads the first page again instead,
// the way TanStack Query's guide refetches an infinite query from its first page only
// (https://tanstack.com/query/v5/docs/framework/react/guides/infinite-queries).
import { type QueryClient, queryOptions, skipToken } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { type Status, type StatusPage, type Timeline, timelineQueryOptions } from "./timelines";

/** The timelines whose new entries a stream delivers: the home timeline, the user's lists, the local and the federated. */
export type StreamedTimeline = Extract<Timeline, { kind: "home" | "list" | "local" | "public" }>;

/** The timelines that have a stream of their own, open while their screen shows them (ADR-0127 §5). */
export type PublicStreamedTimeline = Extract<Timeline, { kind: "local" | "public" }>;

/** The new entries the stream delivered, newest first, and whether they may not join the loaded ones. */
export type NewStatuses = { statuses: readonly Status[]; gap: boolean };

/** How many waiting entries are kept; older ones are dropped, and the timeline then has a gap. */
const maxNewStatuses = 200;

/**
 * The waiting entries of a timeline. Only the stream writes them (setQueryData); there is nothing to fetch, and they
 * are kept until they are shown or the user changes (lib/auth/userData.ts clears the cache).
 */
export function newStatusesQueryOptions(timeline: StreamedTimeline) {
  return queryOptions<NewStatuses>({
    // Not an API path in the second place, so that invalidating the lists of a path never reaches it.
    queryKey: newStatusesQueryKey(timeline),
    queryFn: skipToken,
    staleTime: Infinity,
    gcTime: Infinity,
  });
}

function newStatusesQueryKey(timeline: StreamedTimeline) {
  switch (timeline.kind) {
    case "home":
      return ["stream", "home-new-statuses"] as const;
    case "list":
      return ["stream", "list-new-statuses", timeline.id] as const;
    case "local":
      return ["stream", "local-new-statuses"] as const;
    case "public":
      return ["stream", "public-new-statuses"] as const;
  }
}

/** IDs are UUIDv7 strings, which sort in the order they were made: newest first. */
function newestFirst(a: Status, b: Status): number {
  return a.id < b.id ? 1 : a.id > b.id ? -1 : 0;
}

/** The loaded timeline, newest first, or undefined when it has not been loaded. */
function loadedStatuses(queryClient: QueryClient, api: Api, timeline: StreamedTimeline): readonly Status[] | undefined {
  return queryClient.getQueryData(timelineQueryOptions(api, timeline).queryKey)?.pages.flatMap((page) => page.items);
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

/** Adds `statuses` to the waiting entries of `timeline`, without repeating one. */
function addWaiting(
  queryClient: QueryClient,
  timeline: StreamedTimeline,
  statuses: readonly Status[],
  gap: boolean,
): void {
  queryClient.setQueryData(newStatusesQueryOptions(timeline).queryKey, (previous) => {
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
 * Inserts the waiting entries that join the loaded ones at the top of the cached timeline, and clears them. After a
 * gap it changes nothing and returns false: only loading the first page again closes it.
 */
function insertWaiting(queryClient: QueryClient, api: Api, timeline: StreamedTimeline): boolean {
  const waitingKey = newStatusesQueryOptions(timeline).queryKey;
  const waiting = queryClient.getQueryData(waitingKey);
  if (waiting === undefined) return true;
  if (waiting.gap) return false;
  queryClient.setQueryData(waitingKey, { statuses: [], gap: false });
  queryClient.setQueryData(timelineQueryOptions(api, timeline).queryKey, (data) => {
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

/** Whether a page shows the timeline now: its query has observers (the timeline's components). */
function timelineShown(queryClient: QueryClient, api: Api, timeline: StreamedTimeline): boolean {
  const query = queryClient
    .getQueryCache()
    .find({ queryKey: timelineQueryOptions(api, timeline).queryKey, exact: true });
  return (query?.getObserversCount() ?? 0) > 0;
}

/** New entries wait for the reader while a page shows the timeline, and join its cache otherwise. */
function receive(
  queryClient: QueryClient,
  api: Api,
  timeline: StreamedTimeline,
  statuses: readonly Status[],
  gap: boolean,
): void {
  if (statuses.length > 0 || gap) addWaiting(queryClient, timeline, statuses, gap);
  joinWaitingStatuses(queryClient, api, timeline);
}

/**
 * Inserts the waiting entries into the cached timeline unless a page shows it. The routes' loaders call it too, so
 * that entries that arrived while the timeline was shown, but were not shown when the reader left it, are on it when
 * it opens again.
 */
export function joinWaitingStatuses(queryClient: QueryClient, api: Api, timeline: StreamedTimeline): void {
  if (!timelineShown(queryClient, api, timeline)) insertWaiting(queryClient, api, timeline);
}

/** A new home entry from the stream (`update`). */
export function receiveHomeStatus(queryClient: QueryClient, api: Api, status: Status): void {
  receive(queryClient, api, { kind: "home" }, [status], false);
}

/**
 * A new entry of the list `listId` from the stream (`list_update`). Nothing is kept for a list whose timeline is not
 * loaded: it loads fresh when the reader opens it.
 */
export function receiveListStatus(queryClient: QueryClient, api: Api, listId: string, status: Status): void {
  const timeline: StreamedTimeline = { kind: "list", id: listId };
  if (loadedStatuses(queryClient, api, timeline) === undefined) return;
  receive(queryClient, api, timeline, [status], false);
}

/** Reads the first page of `timeline` from the server. */
async function firstPage(api: Api, timeline: StreamedTimeline): Promise<StatusPage> {
  const { data, error } = await (() => {
    switch (timeline.kind) {
      case "home":
        return api.fetch.GET("/timelines/home", {});
      case "list":
        return api.fetch.GET("/timelines/list/{id}", { params: { path: { id: timeline.id } } });
      case "local":
        return api.fetch.GET("/timelines/local", {});
      case "public":
        return api.fetch.GET("/timelines/public", {});
    }
  })();
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Reads the first page of a loaded timeline for the entries the gap missed and adds the new ones to the waiting
 * entries. Nothing is read while the timeline is not loaded: it loads fresh when the reader opens it.
 */
async function catchUp(queryClient: QueryClient, api: Api, timeline: StreamedTimeline): Promise<void> {
  const loaded = loadedStatuses(queryClient, api, timeline);
  if (loaded === undefined) return;
  const page = await firstPage(api, timeline);
  const top = loaded[0]?.id;
  const fresh = unseenStatuses({ statuses: page.items, gap: false }, loaded);
  const oldest = page.items.at(-1)?.id;
  // A full page of news with more behind it may not reach the loaded entries.
  const gap = page.next_cursor !== undefined && oldest !== undefined && (top === undefined || oldest > top);
  receive(queryClient, api, timeline, fresh, gap);
}

/** After the stream reconnected: catches up the home timeline. */
export function catchUpHome(queryClient: QueryClient, api: Api): Promise<void> {
  return catchUp(queryClient, api, { kind: "home" });
}

/** A new entry of the local or the federated timeline from its stream (`update`). */
export function receivePublicTimelineStatus(
  queryClient: QueryClient,
  api: Api,
  timeline: PublicStreamedTimeline,
  status: Status,
): void {
  receive(queryClient, api, timeline, [status], false);
}

/** After the stream of the local or the federated timeline reconnected: catches up that timeline. */
export function catchUpPublicTimeline(
  queryClient: QueryClient,
  api: Api,
  timeline: PublicStreamedTimeline,
): Promise<void> {
  return catchUp(queryClient, api, timeline);
}

/**
 * After the stream reconnected: catches up every list timeline that is loaded (the server replays nothing, so the
 * open lists read their first page again). Resolves once every one is done; rejects with the first failure.
 */
export async function catchUpLists(queryClient: QueryClient, api: Api): Promise<void> {
  const ids = queryClient
    .getQueryCache()
    .findAll({ queryKey: ["get", "/timelines/list/{id}"] })
    .map((query) => {
      const init = query.queryKey[2] as { params?: { path?: { id?: unknown } } } | undefined;
      return init?.params?.path?.id;
    })
    .filter((id): id is string => typeof id === "string");
  await Promise.all([...new Set(ids)].map((id) => catchUp(queryClient, api, { kind: "list", id })));
}

/**
 * Shows the waiting entries: inserts them at the top of the loaded timeline, or, after a gap, loads the first page
 * again and drops the older pages (they follow again on scrolling). Resolves when the timeline shows them; rejects
 * when loading the first page again failed.
 */
export async function showNewStatuses(queryClient: QueryClient, api: Api, timeline: StreamedTimeline): Promise<void> {
  if (insertWaiting(queryClient, api, timeline)) return;
  const key = timelineQueryOptions(api, timeline).queryKey;
  queryClient.setQueryData(newStatusesQueryOptions(timeline).queryKey, { statuses: [], gap: false });
  queryClient.setQueryData(
    key,
    (data) => data && { pages: data.pages.slice(0, 1), pageParams: data.pageParams.slice(0, 1) },
  );
  await queryClient.refetchQueries({ queryKey: key, exact: true }, { throwOnError: true });
}
