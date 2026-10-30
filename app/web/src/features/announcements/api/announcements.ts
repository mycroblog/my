// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the announcements (FR-726, ADR-0125, web-client/README.md S1): the announcements that every local
// user sees now with whether the viewer has read each (GET /announcements), the pages of those that have started to
// show, the ended ones included (GET /announcements/history), marking one read, and what the event stream says about
// them (lib/sse): a published or changed announcement replaces or joins the list, and a withdrawn one leaves both.
import { type InfiniteData, infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

export type Announcement = components["schemas"]["Announcement"];
type AnnouncementList = components["schemas"]["AnnouncementList"];
type AnnouncementPage = components["schemas"]["AnnouncementPage"];

/** How many announcements a page of the past ones holds. */
const historyPageSize = 20;

/** The announcements that show now as the cache has them; empty when it has none. */
export function cachedAnnouncements(queryClient: QueryClient, api: Api): readonly Announcement[] {
  return queryClient.getQueryData<AnnouncementList>(announcementsQueryOptions(api).queryKey)?.items ?? [];
}

/** The announcements that the users see now, the latest first (GET /announcements). */
export function announcementsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/announcements");
}

/**
 * The announcements that have started to show, the ended ones included, the latest first (GET
 * /announcements/history), a page at a time.
 */
export function announcementHistoryQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/announcements/history", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query =
        pageParam === undefined ? { limit: historyPageSize } : { limit: historyPageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/announcements/history", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Marks an announcement read (POST /announcements/{id}/read), as the options of useMutation. */
export function markAnnouncementReadMutation(api: Api) {
  return {
    mutationKey: ["post", "/announcements/{id}/read"] as const,
    mutationFn: async (id: string): Promise<Announcement> => {
      const { data, error } = await api.fetch.POST("/announcements/{id}/read", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** The list with `announcement` in place of the one of its ID, or joined in the order of the list. */
function withAnnouncement(list: AnnouncementList, announcement: Announcement): AnnouncementList {
  const others = list.items.filter((item) => item.id !== announcement.id);
  const items = [...others, announcement].toSorted(
    (a, b) => b.published_at.localeCompare(a.published_at) || b.id.localeCompare(a.id),
  );
  return { items };
}

/** The pages of the past announcements with the one of `announcement`'s ID replaced by it, keeping whether it ended. */
function withHistoryItem<Param>(
  pages: InfiniteData<AnnouncementPage, Param>,
  announcement: Announcement,
): InfiniteData<AnnouncementPage, Param> {
  return {
    ...pages,
    pages: pages.pages.map((page) => ({
      ...page,
      items: page.items.map((item) => (item.id === announcement.id ? { ...announcement, ended: item.ended } : item)),
    })),
  };
}

/**
 * Writes an announcement that the response of marking it read or the event stream gave into the list of those shown
 * now, and into the pages of the past ones that already have it. A new one is not placed among the pages: their order
 * is the server's, so they are read again instead.
 */
export function cacheAnnouncement(queryClient: QueryClient, api: Api, announcement: Announcement): void {
  queryClient.setQueryData<AnnouncementList>(announcementsQueryOptions(api).queryKey, (list) =>
    list === undefined ? list : withAnnouncement(list, announcement),
  );
  const historyKey = announcementHistoryQueryOptions(api).queryKey;
  const history = queryClient.getQueryData(historyKey);
  if (history === undefined) return;
  if (history.pages.some((page) => page.items.some((item) => item.id === announcement.id))) {
    queryClient.setQueryData(historyKey, (pages) =>
      pages === undefined ? pages : withHistoryItem(pages, announcement),
    );
  } else {
    void queryClient.invalidateQueries({ queryKey: historyKey, exact: true });
  }
}

/**
 * Takes an announcement that no longer shows off the list of those shown now. It was withdrawn or its window changed,
 * which the event does not tell apart, so the past ones are read again.
 */
export function removeAnnouncement(queryClient: QueryClient, api: Api, id: string): void {
  queryClient.setQueryData<AnnouncementList>(announcementsQueryOptions(api).queryKey, (list) =>
    list === undefined ? list : { items: list.items.filter((item) => item.id !== id) },
  );
  void queryClient.invalidateQueries({ queryKey: announcementHistoryQueryOptions(api).queryKey, exact: true });
}

/** After the stream reconnected: reads the announcements again, since the server replays nothing. */
export function catchUpAnnouncements(queryClient: QueryClient, api: Api): void {
  void queryClient.invalidateQueries({ queryKey: announcementsQueryOptions(api).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: announcementHistoryQueryOptions(api).queryKey, exact: true });
}
