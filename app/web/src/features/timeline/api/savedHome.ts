// SPDX-License-Identifier: AGPL-3.0-only
// The home timeline kept on this browser for reading offline (FR-604, web-client/11-pwa.md §11). The Service Worker never
// answers the client API (ADR-0028, ADR-0045), so the app keeps the posts itself: the newest posts of the home
// timeline, without the followers-only and direct ones (nor boosts of them), each time its query loads or changes them while a page shows it, in lib/storage's homeTimelineKey (the
// user's data, removed when the session ends and when another user signs in). When the home timeline cannot be
// fetched for the network and the query cache does not hold it (the page was opened or reloaded offline), the route
// loader puts the kept posts into the cache, dated when they were loaded, and the page says that it is offline.
import { hashKey, type InfiniteData, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { homeTimelineKey } from "../../../lib/storage";
import { type Status, type TimelinePage, timelineQueryOptions } from "./timelines";

/** How many of the newest posts are kept: two pages, a few hundred kilobytes of localStorage at most. */
const maxSavedStatuses = 40;

function homeQueryKey(api: Api) {
  return timelineQueryOptions(api, { kind: "home" }).queryKey;
}

/** Whether a kept item has what every post card reads first; the rest follows the stored version (lib/storage). */
function looksLikeStatus(item: Record<string, unknown>): boolean {
  const account = item["account"];
  return (
    typeof item["id"] === "string" &&
    typeof item["created_at"] === "string" &&
    typeof account === "object" &&
    account !== null
  );
}

/** The visibilities whose posts are never kept on the browser: only their audience may read them. */
const unsavedVisibilities: ReadonlySet<string> = new Set(["private", "direct"]);

/**
 * Whether a post may be kept on the browser: neither it nor the post it boosts is followers-only or direct
 * (decided on #640). A boost carries the boosted post's visibility, and an edit that narrows the visibility
 * rewrites the cached post, which saves the timeline again without it.
 */
function savable(status: Status): boolean {
  return (
    !unsavedVisibilities.has(status.visibility) &&
    (status.boost === undefined || !unsavedVisibilities.has(status.boost.visibility))
  );
}

/** Keeps the newest posts of the cached home timeline that may be kept, unless it holds none. */
function saveHomeTimeline(queryClient: QueryClient, api: Api): void {
  const state = queryClient.getQueryState<InfiniteData<TimelinePage>>(homeQueryKey(api));
  if (state?.data === undefined) return;
  const statuses = state.data.pages.flatMap((page) => page.items).filter(savable);
  const items = statuses.slice(0, maxSavedStatuses);
  // The cursor after the kept posts: the last loaded page's, when every loaded post that may be kept is kept.
  const lastPage = state.data.pages.at(-1);
  const nextCursor = items.length === statuses.length ? (lastPage?.next_cursor ?? null) : null;
  homeTimelineKey.write({ loadedAt: state.dataUpdatedAt, items, nextCursor });
}

/**
 * Puts the kept home timeline into the query cache, dated when it was loaded, unless the cache holds the
 * timeline already. Returns whether the cache holds a home timeline now.
 */
export function restoreSavedHomeTimeline(queryClient: QueryClient, api: Api): boolean {
  const key = homeQueryKey(api);
  if (queryClient.getQueryData(key) !== undefined) return true;
  const saved = homeTimelineKey.read();
  if (saved === null || !saved.items.every(looksLikeStatus)) return false;
  // The items are the Status objects that saveHomeTimeline wrote, of the same stored version.
  const page: TimelinePage = {
    items: (saved.items as Status[]).filter(savable),
    ...(saved.nextCursor !== null && { next_cursor: saved.nextCursor }),
  };
  queryClient.setQueryData(key, { pages: [page], pageParams: [undefined] }, { updatedAt: saved.loadedAt });
  return true;
}

/** Removes the kept home timeline: the server answered that this browser has no session. */
export function forgetSavedHomeTimeline(): void {
  homeTimelineKey.remove();
}

/**
 * Keeps the home timeline whenever its query has new data, from now until the returned function is called.
 * For an Effect of the signed-in home timeline (an external store: the query cache and the browser's storage).
 */
export function keepHomeTimelineSaved(queryClient: QueryClient, api: Api): () => void {
  const hash = hashKey(homeQueryKey(api));
  saveHomeTimeline(queryClient, api);
  return queryClient.getQueryCache().subscribe((event) => {
    // A fetch, or a write to the cache (a new post, a favourite), that succeeded.
    if (event.type === "updated" && event.action.type === "success" && event.query.queryHash === hash)
      saveHomeTimeline(queryClient, api);
  });
}
