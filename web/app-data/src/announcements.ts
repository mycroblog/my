// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements (ADR-0125 §2–§5): those the viewer sees now, the unread first, then the read, each group
// newest first, for the column beside the timeline and, unread only, above the home timeline; all that ever showed,
// the ended ones included, newest first, a page at a time, for their list; one of them, for its page; and marking one
// read, which puts the server's answer wherever the announcement is cached.
import type { components } from "@mycroblog/mykit";
import { useQuery, type InfiniteData } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type Page, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** An announcement of the server, as the viewer sees it. */
export type Announcement = components["schemas"]["Announcement"];

/** The announcements the viewer sees now. */
export function announcementsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/announcements");
}

/** Every announcement that showed to the viewer, the ended ones included, newest first. */
export function announcementHistoryQuery({ api, queries }: AppData) {
  return pagedQuery<Announcement>(
    queries.queryOptions("get", "/announcements/history").queryKey,
    async (cursor, signal) =>
      (await api.GET("/announcements/history", { params: { query: cursor === undefined ? {} : { cursor } }, signal }))
        .data,
  );
}

/** One announcement that showed to the viewer. */
export function announcementQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/announcements/{id}", { params: { path: { id } } });
}

/** `items` with the unread first, each group in the server's order (the newest first). */
const unreadFirst = (items: readonly Announcement[]) => [
  ...items.filter((a) => !a.read),
  ...items.filter((a) => a.read),
];

/**
 * The announcements the viewer sees now, the unread first; undefined until the server answered, and empty while
 * `enabled` is false (a reader without a session, who has none).
 */
export function useAnnouncements(enabled = true): readonly Announcement[] | undefined {
  const query = useQuery({ ...announcementsQuery(useAppData()), enabled });
  if (!enabled) return [];
  return query.data === undefined ? undefined : unreadFirst(query.data.items);
}

/** Every announcement that showed to the viewer, newest first, for their list. */
export function useAnnouncementHistory(): PagedList<Announcement> {
  return usePagedList(announcementHistoryQuery(useAppData()));
}

/** The announcement of `id`; undefined until the server answered, and its failure (a ProblemError of mykit). */
export function useAnnouncement(id: string): { announcement: Announcement | undefined; error: Error | null } {
  const query = useQuery(announcementQuery(useAppData(), id));
  return { announcement: query.data, error: query.error as Error | null };
}

/** Puts `read` in place of the announcement of its id, wherever it is cached. */
function cacheRead(data: AppData, read: Announcement): void {
  const { queryClient } = data;
  const swap = (a: Announcement) => (a.id === read.id ? { ...a, read: read.read } : a);
  const now = announcementsQuery(data).queryKey;
  const list = queryClient.getQueryData<components["schemas"]["AnnouncementList"]>(now);
  if (list !== undefined) queryClient.setQueryData(now, { ...list, items: list.items.map(swap) });
  const history = announcementHistoryQuery(data).queryKey;
  const pages = queryClient.getQueryData<InfiniteData<Page<Announcement>, string | undefined>>(history);
  if (pages !== undefined)
    queryClient.setQueryData(history, {
      ...pages,
      pages: pages.pages.map((p) => ({ ...p, items: p.items.map(swap) })),
    });
  const one = announcementQuery(data, read.id).queryKey;
  const shown = queryClient.getQueryData<Announcement>(one);
  if (shown !== undefined) queryClient.setQueryData(one, swap(shown));
}

/**
 * Marks the announcement of `id` read, and puts the server's answer wherever it is cached. It rejects with the problem
 * (one that no longer shows).
 */
export async function markAnnouncementRead(data: AppData, id: string): Promise<void> {
  const { data: read } = await data.api.POST("/announcements/{id}/read", { params: { path: { id } } });
  if (read !== undefined) cacheRead(data, read);
}

/** Marks an announcement read, from a screen (`markAnnouncementRead`). */
export function useMarkAnnouncementRead(): (id: string) => Promise<void> {
  const data = useAppData();
  return (id) => markAnnouncementRead(data, id);
}
