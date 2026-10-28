// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the announcements (FR-726, web-client/README.md S1): the announcements that every local user sees
// now with whether the viewer has read each (GET /announcements), marking one read, and what the event stream
// says about them (lib/sse): a published or changed announcement replaces or joins the list, and a withdrawn one
// leaves it.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

export type Announcement = components["schemas"]["Announcement"];
type AnnouncementList = components["schemas"]["AnnouncementList"];

/** The announcements that the users see now, the latest first (GET /announcements). */
export function announcementsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/announcements");
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

/** Writes an announcement that the response of marking it read or the event stream gave into the list. */
export function cacheAnnouncement(queryClient: QueryClient, api: Api, announcement: Announcement): void {
  queryClient.setQueryData<AnnouncementList>(announcementsQueryOptions(api).queryKey, (list) =>
    list === undefined ? list : withAnnouncement(list, announcement),
  );
}

/** Takes an announcement that the users no longer see off the list. */
export function removeAnnouncement(queryClient: QueryClient, api: Api, id: string): void {
  queryClient.setQueryData<AnnouncementList>(announcementsQueryOptions(api).queryKey, (list) =>
    list === undefined ? list : { items: list.items.filter((item) => item.id !== id) },
  );
}

/** After the stream reconnected: reads the announcements again, since the server replays nothing. */
export function catchUpAnnouncements(queryClient: QueryClient, api: Api): void {
  void queryClient.invalidateQueries({ queryKey: announcementsQueryOptions(api).queryKey, exact: true });
}
